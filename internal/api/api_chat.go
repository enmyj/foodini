package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/gemini"
	"foodtracker/internal/sheets"
)

type chatRequest struct {
	Message string
	Date    string
	Meal    string
	Images  []gemini.ImageData
}

// POST /api/chat
func (h *Handler) Chat(c *echo.Context) error {
	session := auth.SessionFrom(c)

	r := c.Request()
	ctx := r.Context()
	req, err := parseChatRequest(r)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return writeErr(c, http.StatusRequestEntityTooLarge, "upload_too_large")
		}
		return writeErr(c, http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.Message) == "" && len(req.Images) == 0 {
		return writeErr(c, http.StatusBadRequest, "message or image required")
	}

	targetDate := req.Date
	if targetDate == "" {
		targetDate = sheets.DateString(LocalNow(r))
	}

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		svc, err := h.sheetsSvc(c, session)
		if err != nil {
			return h.writeAPIErr(c, err)
		}
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	message := req.Message
	if req.Meal != "" {
		message = "(meal type: " + req.Meal + ") " + message
	}

	responseText, entries, err := h.gemini.Chat(ctx, session.UserEmail, targetDate, message, profileCtx, req.Images)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}

	if len(entries) == 0 {
		return c.JSON(http.StatusOK, map[string]any{"done": false, "message": responseText})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"done":    false,
		"pending": true,
		"entries": entries,
		"message": responseText,
	})
}

func parseChatRequest(r *http.Request) (chatRequest, error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return parseMultipartChatRequest(r)
	}
	return parseJSONChatRequest(r)
}

func parseJSONChatRequest(r *http.Request) (chatRequest, error) {
	var req struct {
		Message string `json:"message"`
		Date    string `json:"date"`
		Meal    string `json:"meal"`
		Images  []struct {
			MIMEType string `json:"mime_type"`
			Data     string `json:"data"`
		} `json:"images"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return chatRequest{}, err
	}

	parsed := chatRequest{
		Message: req.Message,
		Date:    req.Date,
		Meal:    req.Meal,
	}
	for _, img := range req.Images {
		decoded, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			return chatRequest{}, err
		}
		parsed.Images = append(parsed.Images, gemini.ImageData{MIMEType: img.MIMEType, Data: decoded})
	}
	return parsed, nil
}

func parseMultipartChatRequest(r *http.Request) (chatRequest, error) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return chatRequest{}, err
	}

	req := chatRequest{
		Message: r.FormValue("message"),
		Date:    r.FormValue("date"),
		Meal:    r.FormValue("meal"),
	}
	for _, field := range []string{"images", "image"} {
		files := r.MultipartForm.File[field]
		for _, fh := range files {
			file, err := fh.Open()
			if err != nil {
				return chatRequest{}, err
			}
			data, readErr := io.ReadAll(file)
			closeErr := file.Close()
			if readErr != nil {
				return chatRequest{}, readErr
			}
			if closeErr != nil {
				return chatRequest{}, closeErr
			}
			if len(data) == 0 {
				continue
			}

			mimeType := strings.TrimSpace(fh.Header.Get("Content-Type"))
			if mimeType == "" {
				mimeType = http.DetectContentType(data)
			}
			if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
				return chatRequest{}, errors.New("invalid image upload")
			}
			req.Images = append(req.Images, gemini.ImageData{MIMEType: mimeType, Data: data})
		}
	}
	return req, nil
}

// POST /api/chat/confirm
func (h *Handler) ConfirmChat(c *echo.Context) error {
	session := auth.SessionFrom(c)

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Entries []sheets.FoodEntry `json:"entries"`
		Date    string             `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Entries) == 0 {
		return writeErr(c, http.StatusBadRequest, "invalid request body")
	}

	targetDate := req.Date
	if targetDate == "" {
		targetDate = sheets.DateString(LocalNow(r))
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	now := LocalNow(r)
	var saved []sheets.FoodEntry
	for _, e := range req.Entries {
		fe := sheets.FoodEntry{
			ID:          uuid.NewString(),
			Date:        targetDate,
			Time:        sheets.TimeString(now),
			MealType:    e.MealType,
			Description: e.Description,
			Calories:    e.Calories,
			Protein:     e.Protein,
			Carbs:       e.Carbs,
			Fat:         e.Fat,
			Fiber:       e.Fiber,
		}
		if err := svc.AppendFood(ctx, fe); err != nil {
			return h.writeAPIErr(c, fmt.Errorf("sheet write: %w", err))
		}
		saved = append(saved, fe)
	}

	h.gemini.ClearConversation(session.UserEmail, targetDate)
	h.cacheInvalidate(session.SpreadsheetID)
	return c.JSON(http.StatusOK, map[string]any{"done": true, "entries": saved})
}
