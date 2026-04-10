package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"foodtracker/internal/auth"
	"foodtracker/internal/gemini"
	"foodtracker/internal/sheets"
)

// Handler holds references to auth and gemini services.
type Handler struct {
	auth    *auth.Handler
	gemini  *gemini.Service
	cacheMu sync.RWMutex
	cache   map[string]cacheItem
	// migratedIDs tracks spreadsheet IDs confirmed at CurrentSchemaVersion.
	migratedMu  sync.RWMutex
	migratedIDs map[string]bool
}

type cacheItem struct {
	data    []byte
	expires time.Time
}

type chatRequest struct {
	Message string
	Date    string
	Meal    string
	Images  []gemini.ImageData
}

const cacheTTL = 60 * time.Second

func NewHandler(authHandler *auth.Handler, geminiAPIKey string) *Handler {
	return &Handler{
		auth:        authHandler,
		gemini:      gemini.NewService(geminiAPIKey),
		cache:       make(map[string]cacheItem),
		migratedIDs: make(map[string]bool),
	}
}

func (h *Handler) cacheGet(key string) ([]byte, bool) {
	h.cacheMu.RLock()
	item, ok := h.cache[key]
	h.cacheMu.RUnlock()
	if !ok || time.Now().After(item.expires) {
		return nil, false
	}
	return item.data, true
}

func (h *Handler) cacheSet(key string, data []byte) {
	h.cacheMu.Lock()
	h.cache[key] = cacheItem{data: data, expires: time.Now().Add(cacheTTL)}
	h.cacheMu.Unlock()
}

func (h *Handler) cacheInvalidate(spreadsheetID string) {
	prefix := spreadsheetID + "|"
	h.cacheMu.Lock()
	for k := range h.cache {
		if strings.HasPrefix(k, prefix) {
			delete(h.cache, k)
		}
	}
	h.cacheMu.Unlock()
}

func writeJSON(c *echo.Context, status int, v any) error {
	return c.JSON(status, v)
}

func writeErr(c *echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]string{"error": msg})
}

func isSessionExpiredErr(err error) bool {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		if re.ErrorCode == "invalid_grant" || re.ErrorCode == "invalid_token" {
			return true
		}
		if re.Response != nil && (re.Response.StatusCode == http.StatusBadRequest || re.Response.StatusCode == http.StatusUnauthorized) {
			return true
		}
	}
	var ge *googleapi.Error
	return errors.As(err, &ge) && ge.Code == http.StatusUnauthorized
}

func isInsufficientScopesErr(err error) bool {
	var ge *googleapi.Error
	if !errors.As(err, &ge) || ge.Code != http.StatusForbidden {
		return false
	}
	if hasInsufficientScopesText(ge.Message) || hasInsufficientScopesDetails(ge.Details) {
		return true
	}
	for _, item := range ge.Errors {
		if isInsufficientScopesReason(item.Reason) || hasInsufficientScopesText(item.Message) {
			return true
		}
	}
	return false
}

func isInsufficientScopesReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "insufficientpermissions", "access_token_scope_insufficient":
		return true
	default:
		return false
	}
}

func hasInsufficientScopesText(msg string) bool {
	msg = strings.ToLower(strings.TrimSpace(msg))
	return strings.Contains(msg, "insufficient authentication scopes")
}

func hasInsufficientScopesDetails(details []interface{}) bool {
	for _, detail := range details {
		if detailHasInsufficientScopes(detail) {
			return true
		}
	}
	return false
}

func detailHasInsufficientScopes(detail any) bool {
	switch v := detail.(type) {
	case map[string]any:
		for key, value := range v {
			if strings.EqualFold(key, "reason") {
				if reason, ok := value.(string); ok && isInsufficientScopesReason(reason) {
					return true
				}
			}
			if detailHasInsufficientScopes(value) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if detailHasInsufficientScopes(item) {
				return true
			}
		}
	case string:
		return isInsufficientScopesReason(v) || hasInsufficientScopesText(v)
	}
	return false
}

func (h *Handler) writeAPIErr(c *echo.Context, err error) error {
	if isSessionExpiredErr(err) {
		h.auth.ClearSession(c)
		return writeErr(c, http.StatusUnauthorized, "session_expired")
	}
	if isInsufficientScopesErr(err) {
		return writeErr(c, http.StatusForbidden, "insufficient_scopes")
	}
	return writeErr(c, http.StatusInternalServerError, err.Error())
}

// LocalNow returns the current time in the user's local timezone.
func LocalNow(r *http.Request) time.Time {
	tz := r.Header.Get("X-Timezone")
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return time.Now().In(loc)
		}
	}
	return time.Now()
}

func formatProfileContext(p sheets.UserProfile, currentYear int) string {
	var parts []string
	if p.BirthYear != "" {
		if by, err := strconv.Atoi(strings.TrimSpace(p.BirthYear)); err == nil && by > 1900 && by <= currentYear {
			parts = append(parts, fmt.Sprintf("age %d", currentYear-by))
		}
	}
	if p.Gender != "" {
		parts = append(parts, p.Gender)
	}
	if p.Height != "" {
		parts = append(parts, p.Height)
	}
	if p.Weight != "" {
		parts = append(parts, p.Weight)
	}
	if len(parts) == 0 && p.Notes == "" && p.Goals == "" && p.DietaryRestrictions == "" {
		return ""
	}
	ctx := "User profile: " + strings.Join(parts, ", ")
	if p.Notes != "" {
		ctx += ". " + p.Notes
	}
	if p.Goals != "" {
		ctx += ". Goals: " + p.Goals
	}
	if p.DietaryRestrictions != "" {
		ctx += ". Dietary restrictions: " + p.DietaryRestrictions
	}
	return ctx
}

func (h *Handler) sheetsSvc(c *echo.Context, session *auth.Session) (*sheets.Service, error) {
	ctx := c.Request().Context()
	ts := h.auth.TokenSource(ctx, session)
	return sheets.NewService(ctx, ts, session.SpreadsheetID)
}

// ensureSpreadsheet finds or creates the user's spreadsheet.
// Returns false if it fails (error response already written).
func (h *Handler) ensureSpreadsheet(c *echo.Context, session *auth.Session) bool {
	r := c.Request()
	if session.SpreadsheetID != "" {
		h.migratedMu.RLock()
		done := h.migratedIDs[session.SpreadsheetID]
		h.migratedMu.RUnlock()
		if done {
			return true
		}
		ts := h.auth.TokenSource(r.Context(), session)
		svc, err := sheets.NewService(r.Context(), ts, session.SpreadsheetID)
		if err != nil {
			h.writeAPIErr(c, err)
			return false
		}
		version, err := svc.GetSchemaVersion(r.Context())
		if err != nil {
			h.writeAPIErr(c, err)
			return false
		}
		if version < sheets.CurrentSchemaVersion {
			if !h.runMigrations(c, ts, session.SpreadsheetID, version) {
				return false
			}
		}
		h.migratedMu.Lock()
		h.migratedIDs[session.SpreadsheetID] = true
		h.migratedMu.Unlock()
		return true
	}
	ts := h.auth.TokenSource(r.Context(), session)

	id, err := sheets.FindExistingSpreadsheet(r.Context(), ts, session.UserEmail)
	if err != nil {
		h.writeAPIErr(c, err)
		return false
	}

	if id != "" {
		svc, err := sheets.NewService(r.Context(), ts, id)
		if err != nil {
			h.writeAPIErr(c, err)
			return false
		}
		version, err := svc.GetSchemaVersion(r.Context())
		if err != nil {
			h.writeAPIErr(c, err)
			return false
		}
		if version < 1 {
			writeErr(c, http.StatusConflict, "incompatible_spreadsheet")
			return false
		}
		if !h.runMigrations(c, ts, id, version) {
			return false
		}
		session.SpreadsheetID = id
	} else {
		id, err = sheets.CreateSpreadsheet(r.Context(), ts, session.UserEmail)
		if err != nil {
			h.writeAPIErr(c, err)
			return false
		}
		session.SpreadsheetID = id
	}

	if err := h.auth.SetSession(c, session); err != nil {
		writeErr(c, http.StatusInternalServerError, "session save failed")
		return false
	}
	return true
}

func (h *Handler) runMigrations(c *echo.Context, ts oauth2.TokenSource, spreadsheetID string, version int) bool {
	ctx := c.Request().Context()
	type step struct {
		from    int
		migrate func(context.Context, oauth2.TokenSource, string) error
	}
	steps := []step{
		{1, sheets.MigrateV1toV2},
		{2, sheets.MigrateV2toV3},
		{3, sheets.MigrateV3toV4},
		{4, sheets.MigrateV4toV5},
		{5, sheets.MigrateV5toV6},
		{6, sheets.MigrateV6toV7},
		{7, sheets.MigrateV7toV8},
		{8, sheets.MigrateV8toV9},
	}
	for _, s := range steps {
		if version == s.from {
			if err := s.migrate(ctx, ts, spreadsheetID); err != nil {
				writeErr(c, http.StatusInternalServerError, "migration failed: "+err.Error())
				return false
			}
			version++
		}
	}
	return true
}

// GET /api/log?date=YYYY-MM-DD or ?days=N
func (h *Handler) GetLog(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	r := c.Request()
	ctx := r.Context()
	today := sheets.DateString(LocalNow(r))

	if daysStr := c.QueryParam("days"); daysStr != "" {
		days, err := strconv.Atoi(daysStr)
		if err != nil || days < 1 || days > 365 {
			days = 30
		}
		cacheKey := session.SpreadsheetID + "|days|" + strconv.Itoa(days)
		if cached, ok := h.cacheGet(cacheKey); ok {
			return c.Blob(http.StatusOK, "application/json", cached)
		}
		start := sheets.DateString(LocalNow(r).AddDate(0, 0, -(days - 1)))
		entries, err := svc.GetFoodByDateRange(ctx, start, today)
		if err != nil {
			return h.writeAPIErr(c, err)
		}
		dailyLogs, err := svc.GetActivityByDateRange(ctx, start, today)
		if err != nil {
			return h.writeAPIErr(c, err)
		}
		data, _ := json.Marshal(map[string]any{
			"entries":         entries,
			"daily_logs":      dailyLogs,
			"start":           start,
			"end":             today,
			"spreadsheet_url": "https://docs.google.com/spreadsheets/d/" + session.SpreadsheetID,
		})
		h.cacheSet(cacheKey, data)
		return c.Blob(http.StatusOK, "application/json", data)
	}

	date := c.QueryParam("date")
	if date == "" {
		date = today
	}
	cacheKey := session.SpreadsheetID + "|date|" + date
	if cached, ok := h.cacheGet(cacheKey); ok {
		return c.Blob(http.StatusOK, "application/json", cached)
	}
	entries, err := svc.GetFoodByDate(ctx, date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	dayLog, err := svc.GetActivity(ctx, date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	data, _ := json.Marshal(map[string]any{
		"entries":         entries,
		"day_log":         dayLog,
		"date":            date,
		"spreadsheet_url": "https://docs.google.com/spreadsheets/d/" + session.SpreadsheetID,
	})
	h.cacheSet(cacheKey, data)
	return c.Blob(http.StatusOK, "application/json", data)
}

// POST /api/chat
func (h *Handler) Chat(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}

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
		return writeJSON(c, http.StatusOK, map[string]any{"done": false, "message": responseText})
	}

	return writeJSON(c, http.StatusOK, map[string]any{
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
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}

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
	return writeJSON(c, http.StatusOK, map[string]any{"done": true, "entries": saved})
}

// DELETE /api/entries/:id
func (h *Handler) DeleteEntry(c *echo.Context) error {
	session := auth.SessionFrom(c)
	id := c.Param("id")
	if id == "" {
		return writeErr(c, http.StatusBadRequest, "missing id")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if err := svc.DeleteFood(c.Request().Context(), id); err != nil {
		return h.writeAPIErr(c, err)
	}
	h.cacheInvalidate(session.SpreadsheetID)
	return c.NoContent(http.StatusNoContent)
}

// GET /api/profile
func (h *Handler) GetProfile(c *echo.Context) error {
	session := auth.SessionFrom(c)
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	p, err := svc.GetProfile(c.Request().Context())
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	return writeJSON(c, http.StatusOK, p)
}

// PUT /api/profile
func (h *Handler) PutProfile(c *echo.Context) error {
	session := auth.SessionFrom(c)
	var req sheets.UserProfile
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid body")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if err := svc.SetProfile(c.Request().Context(), req); err != nil {
		return h.writeAPIErr(c, err)
	}
	h.cacheInvalidate(session.SpreadsheetID)
	return writeJSON(c, http.StatusOK, req)
}

// GET /api/favorites
func (h *Handler) GetFavorites(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	ctx := c.Request().Context()
	favs, err := svc.GetFavorites(ctx)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if favs == nil {
		favs = []sheets.FavoriteEntry{}
	}
	return writeJSON(c, http.StatusOK, map[string]any{"favorites": favs})
}

// POST /api/favorites
func (h *Handler) AddFavorite(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}
	r := c.Request()
	ctx := r.Context()
	var req struct {
		Description string `json:"description"`
		MealType    string `json:"meal_type"`
		Calories    int    `json:"calories"`
		Protein     int    `json:"protein"`
		Carbs       int    `json:"carbs"`
		Fat         int    `json:"fat"`
		Fiber       int    `json:"fiber"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Description == "" {
		return writeErr(c, http.StatusBadRequest, "invalid request body")
	}
	fav := sheets.FavoriteEntry{
		ID:          uuid.NewString(),
		Description: req.Description,
		MealType:    req.MealType,
		Calories:    req.Calories,
		Protein:     req.Protein,
		Carbs:       req.Carbs,
		Fat:         req.Fat,
		Fiber:       req.Fiber,
		CreatedAt:   sheets.DateString(LocalNow(r)),
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	existing, err := svc.GetFavorites(ctx)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	key := sheets.NormalizeFavoriteKey(fav.Description)
	for _, e := range existing {
		if sheets.NormalizeFavoriteKey(e.Description) == key {
			return writeErr(c, http.StatusConflict, "favorite_exists")
		}
	}
	if err := svc.AddFavorite(ctx, fav); err != nil {
		return h.writeAPIErr(c, err)
	}
	return writeJSON(c, http.StatusOK, fav)
}

// DELETE /api/favorites/:id
func (h *Handler) DeleteFavorite(c *echo.Context) error {
	session := auth.SessionFrom(c)
	id := c.Param("id")
	if id == "" {
		return writeErr(c, http.StatusBadRequest, "missing id")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if err := svc.DeleteFavorite(c.Request().Context(), id); err != nil {
		return h.writeAPIErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// PATCH /api/entries/:id
func (h *Handler) PatchEntry(c *echo.Context) error {
	session := auth.SessionFrom(c)
	id := c.Param("id")
	if id == "" {
		return writeErr(c, http.StatusBadRequest, "missing id")
	}
	var entry sheets.FoodEntry
	if err := json.NewDecoder(c.Request().Body).Decode(&entry); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid body")
	}
	entry.ID = id

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if err := svc.UpdateFood(c.Request().Context(), id, entry); err != nil {
		return h.writeAPIErr(c, err)
	}
	h.cacheInvalidate(session.SpreadsheetID)
	return writeJSON(c, http.StatusOK, entry)
}

// GET /api/activity?date=YYYY-MM-DD
func (h *Handler) GetActivity(c *echo.Context) error {
	session := auth.SessionFrom(c)
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	r := c.Request()
	date := c.QueryParam("date")
	if date == "" {
		date = sheets.DateString(LocalNow(r))
	}
	dayLog, err := svc.GetActivity(r.Context(), date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	return writeJSON(c, http.StatusOK, dayLog)
}

// PUT /api/activity
func (h *Handler) PutActivity(c *echo.Context) error {
	session := auth.SessionFrom(c)
	r := c.Request()
	var req sheets.DayLog
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid body")
	}
	if req.Date == "" {
		req.Date = sheets.DateString(LocalNow(r))
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if err := svc.SetActivity(r.Context(), req); err != nil {
		return h.writeAPIErr(c, err)
	}
	h.cacheInvalidate(session.SpreadsheetID)
	return writeJSON(c, http.StatusOK, req)
}

// POST /api/insights
func (h *Handler) Insights(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Start == "" || req.End == "" {
		return writeErr(c, http.StatusBadRequest, "start and end dates required")
	}
	if _, err := time.Parse("2006-01-02", req.Start); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid start date")
	}
	if _, err := time.Parse("2006-01-02", req.End); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid end date")
	}
	startT, _ := time.Parse("2006-01-02", req.Start)
	endT, _ := time.Parse("2006-01-02", req.End)
	if endT.Sub(startT) > 31*24*time.Hour {
		return writeErr(c, http.StatusBadRequest, "date range too large")
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	entries, err := svc.GetFoodByDateRange(ctx, req.Start, req.End)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	dailyLogs, err := svc.GetActivityByDateRange(ctx, req.Start, req.End)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	summary := buildWeekSummary(req.Start, req.End, entries, dailyLogs)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	insight, err := h.gemini.Insights(ctx, summary, profileCtx)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(ctx, sheets.InsightRecord{
		Type:        "week",
		StartDate:   req.Start,
		EndDate:     req.End,
		GeneratedAt: generatedAt,
		Insight:     insight,
	})
	return writeJSON(c, http.StatusOK, map[string]any{"insight": insight, "generated_at": generatedAt})
}

// POST /api/insights/day
func (h *Handler) DayInsights(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Date == "" {
		return writeErr(c, http.StatusBadRequest, "date required")
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid date")
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	entries, err := svc.GetFoodByDateRange(ctx, req.Date, req.Date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	dailyLogs, err := svc.GetActivityByDateRange(ctx, req.Date, req.Date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	if len(entries) == 0 && len(dailyLogs) == 0 {
		return writeErr(c, http.StatusBadRequest, "no data for this day")
	}

	today := sheets.DateString(LocalNow(r))
	summary := buildDaySummary(req.Date, entries, dailyLogs, req.Date == today)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	insight, err := h.gemini.DayInsights(ctx, summary, profileCtx)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(ctx, sheets.InsightRecord{
		Type:        "day",
		StartDate:   req.Date,
		EndDate:     req.Date,
		GeneratedAt: generatedAt,
		Insight:     insight,
	})
	return writeJSON(c, http.StatusOK, map[string]any{"insight": insight, "generated_at": generatedAt})
}

func buildWeekSummary(start, end string, entries []sheets.FoodEntry, dailyLogs []sheets.DayLog) string {
	byDate := map[string][]sheets.FoodEntry{}
	for _, e := range entries {
		byDate[e.Date] = append(byDate[e.Date], e)
	}
	logByDate := map[string]sheets.DayLog{}
	for _, l := range dailyLogs {
		logByDate[l.Date] = l
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Week %s to %s:\n\n", start, end)
	cur, _ := time.Parse("2006-01-02", start)
	endT, _ := time.Parse("2006-01-02", end)
	for !cur.After(endT) {
		date := cur.Format("2006-01-02")
		dayEntries := byDate[date]
		log := logByDate[date]
		fmt.Fprintf(&b, "%s (%s):\n", date, cur.Weekday())
		if len(dayEntries) == 0 {
			fmt.Fprintf(&b, "  No food logged\n")
		} else {
			totalCal, totalProt, totalCarb, totalFat, totalFiber := 0, 0, 0, 0, 0
			for _, e := range dayEntries {
				totalCal += e.Calories
				totalProt += e.Protein
				totalCarb += e.Carbs
				totalFat += e.Fat
				totalFiber += e.Fiber
			}
			fmt.Fprintf(&b, "  Totals: %d cal, %dg protein, %dg carbs, %dg fat, %dg fiber\n", totalCal, totalProt, totalCarb, totalFat, totalFiber)
			for _, e := range dayEntries {
				fmt.Fprintf(&b, "  - [%s] %s: %d cal\n", e.MealType, e.Description, e.Calories)
			}
		}
		if log.Activity != "" {
			fmt.Fprintf(&b, "  Activity: %s\n", log.Activity)
		}
		if log.Poop {
			fmt.Fprintf(&b, "  Bowel movement: yes\n")
			if log.PoopNotes != "" {
				fmt.Fprintf(&b, "  Notes: %s\n", log.PoopNotes)
			}
		}
		if log.Hydration > 0 {
			fmt.Fprintf(&b, "  Water: %.1fL\n", log.Hydration)
		}
		if log.FeelingScore > 0 {
			fmt.Fprintf(&b, "  Feeling: %d/10\n", log.FeelingScore)
		}
		fmt.Fprintln(&b)
		cur = cur.AddDate(0, 0, 1)
	}
	return b.String()
}

func buildDaySummary(date string, entries []sheets.FoodEntry, dailyLogs []sheets.DayLog, inProgress bool) string {
	logByDate := map[string]sheets.DayLog{}
	for _, l := range dailyLogs {
		logByDate[l.Date] = l
	}
	t, _ := time.Parse("2006-01-02", date)
	log := logByDate[date]

	var b strings.Builder
	fmt.Fprintf(&b, "Day: %s (%s)\n", date, t.Weekday())
	if inProgress {
		fmt.Fprintf(&b, "Status: TODAY — day is still in progress; more meals may be logged later.\n")
	} else {
		fmt.Fprintf(&b, "Status: past day — complete log.\n")
	}
	b.WriteString("\n")
	if len(entries) == 0 {
		fmt.Fprintf(&b, "  No food logged\n")
	} else {
		totalCal, totalProt, totalCarb, totalFat, totalFiber := 0, 0, 0, 0, 0
		for _, e := range entries {
			totalCal += e.Calories
			totalProt += e.Protein
			totalCarb += e.Carbs
			totalFat += e.Fat
			totalFiber += e.Fiber
		}
		fmt.Fprintf(&b, "Totals: %d cal, %dg protein, %dg carbs, %dg fat, %dg fiber\n", totalCal, totalProt, totalCarb, totalFat, totalFiber)
		for _, e := range entries {
			fmt.Fprintf(&b, "  - [%s] %s: %d cal\n", e.MealType, e.Description, e.Calories)
		}
	}
	if log.Activity != "" {
		fmt.Fprintf(&b, "Activity: %s\n", log.Activity)
	}
	if log.Poop {
		fmt.Fprintf(&b, "Bowel movement: yes\n")
		if log.PoopNotes != "" {
			fmt.Fprintf(&b, "Notes: %s\n", log.PoopNotes)
		}
	}
	if log.Hydration > 0 {
		fmt.Fprintf(&b, "Water: %.1fL\n", log.Hydration)
	}
	if log.FeelingScore > 0 {
		fmt.Fprintf(&b, "Feeling: %d/10\n", log.FeelingScore)
		if log.FeelingNotes != "" {
			fmt.Fprintf(&b, "Feeling notes: %s\n", log.FeelingNotes)
		}
	}
	return b.String()
}

// GET /api/insights?start=...&end=...
func (h *Handler) GetStoredInsights(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}
	start := c.QueryParam("start")
	end := c.QueryParam("end")
	if start == "" || end == "" {
		return writeErr(c, http.StatusBadRequest, "start and end required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	rec, err := svc.GetInsight(c.Request().Context(), "week", start, end)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if rec == nil {
		return writeJSON(c, http.StatusOK, map[string]any{"insight": nil, "generated_at": nil})
	}
	return writeJSON(c, http.StatusOK, map[string]any{"insight": rec.Insight, "generated_at": rec.GeneratedAt})
}

// GET /api/insights/day?date=...
func (h *Handler) GetStoredDayInsights(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}
	date := c.QueryParam("date")
	if date == "" {
		return writeErr(c, http.StatusBadRequest, "date required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	rec, err := svc.GetInsight(c.Request().Context(), "day", date, date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if rec == nil {
		return writeJSON(c, http.StatusOK, map[string]any{"insight": nil, "generated_at": nil})
	}
	return writeJSON(c, http.StatusOK, map[string]any{"insight": rec.Insight, "generated_at": rec.GeneratedAt})
}

// POST /api/suggestions/day
func (h *Handler) DaySuggestions(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Date == "" {
		return writeErr(c, http.StatusBadRequest, "date required")
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid date")
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	entries, err := svc.GetFoodByDateRange(ctx, req.Date, req.Date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	hasMeal := map[string]bool{}
	for _, e := range entries {
		hasMeal[e.MealType] = true
	}
	complete := hasMeal["breakfast"] && hasMeal["lunch"] && hasMeal["dinner"]

	prevDate := addDaysStr(req.Date, -1)
	prevEntries, _ := svc.GetFoodByDateRange(ctx, prevDate, prevDate)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	var insightText string
	if rec, _ := svc.GetInsight(ctx, "day", req.Date, req.Date); rec != nil {
		insightText = rec.Insight
	}

	summary := buildMealSuggestionSummary(req.Date, entries, prevEntries, complete, insightText)

	suggestions, err := h.gemini.MealSuggestions(ctx, summary, profileCtx)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}

	sugType := "remaining"
	if complete {
		sugType = "next-day"
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(ctx, sheets.InsightRecord{
		Type:        "day-suggestions",
		StartDate:   req.Date,
		EndDate:     req.Date,
		GeneratedAt: generatedAt,
		Insight:     sugType + "\n" + suggestions,
	})
	return writeJSON(c, http.StatusOK, map[string]any{"suggestions": suggestions, "type": sugType, "generated_at": generatedAt})
}

// GET /api/suggestions/day?date=...
func (h *Handler) GetStoredDaySuggestions(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}
	date := c.QueryParam("date")
	if date == "" {
		return writeErr(c, http.StatusBadRequest, "date required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	rec, err := svc.GetInsight(c.Request().Context(), "day-suggestions", date, date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if rec == nil {
		return writeJSON(c, http.StatusOK, map[string]any{"suggestions": nil, "type": nil, "generated_at": nil})
	}
	sugType := "remaining"
	sugText := rec.Insight
	if parts := strings.SplitN(rec.Insight, "\n", 2); len(parts) == 2 {
		sugType = parts[0]
		sugText = parts[1]
	}
	return writeJSON(c, http.StatusOK, map[string]any{"suggestions": sugText, "type": sugType, "generated_at": rec.GeneratedAt})
}

// POST /api/suggestions/week
func (h *Handler) WeekSuggestions(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Start == "" || req.End == "" {
		return writeErr(c, http.StatusBadRequest, "start and end dates required")
	}
	if _, err := time.Parse("2006-01-02", req.Start); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid start date")
	}
	if _, err := time.Parse("2006-01-02", req.End); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid end date")
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	entries, err := svc.GetFoodByDateRange(ctx, req.Start, req.End)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	dailyLogs, err := svc.GetActivityByDateRange(ctx, req.Start, req.End)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	summary := buildWeekSummary(req.Start, req.End, entries, dailyLogs)

	if rec, _ := svc.GetInsight(ctx, "week", req.Start, req.End); rec != nil {
		summary += "\nInsights for this week:\n" + rec.Insight + "\n"
	}

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(ctx)
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	suggestions, err := h.gemini.WeekMealSuggestions(ctx, summary, profileCtx)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(ctx, sheets.InsightRecord{
		Type:        "week-suggestions",
		StartDate:   req.Start,
		EndDate:     req.End,
		GeneratedAt: generatedAt,
		Insight:     suggestions,
	})
	return writeJSON(c, http.StatusOK, map[string]any{"suggestions": suggestions, "generated_at": generatedAt})
}

// GET /api/suggestions/week?start=...&end=...
func (h *Handler) GetStoredWeekSuggestions(c *echo.Context) error {
	session := auth.SessionFrom(c)
	if !h.ensureSpreadsheet(c, session) {
		return nil
	}
	start := c.QueryParam("start")
	end := c.QueryParam("end")
	if start == "" || end == "" {
		return writeErr(c, http.StatusBadRequest, "start and end required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	rec, err := svc.GetInsight(c.Request().Context(), "week-suggestions", start, end)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if rec == nil {
		return writeJSON(c, http.StatusOK, map[string]any{"suggestions": nil, "generated_at": nil})
	}
	return writeJSON(c, http.StatusOK, map[string]any{"suggestions": rec.Insight, "generated_at": rec.GeneratedAt})
}

func addDaysStr(dateStr string, n int) string {
	t, _ := time.Parse("2006-01-02", dateStr)
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func buildMealSuggestionSummary(date string, entries, prevEntries []sheets.FoodEntry, complete bool, insightText string) string {
	var b strings.Builder

	if complete {
		fmt.Fprintf(&b, "Today (%s) is complete. Suggest meals for tomorrow.\n\n", date)
	} else {
		hasMeal := map[string]bool{}
		for _, e := range entries {
			hasMeal[e.MealType] = true
		}
		var missing []string
		for _, m := range []string{"breakfast", "lunch", "dinner"} {
			if !hasMeal[m] {
				missing = append(missing, m)
			}
		}
		fmt.Fprintf(&b, "Suggest meals for: %s\n\n", strings.Join(missing, ", "))
	}

	if insightText != "" {
		fmt.Fprintf(&b, "Nutrition insights for today (factor these into suggestions):\n%s\n\n", insightText)
	}

	if len(entries) > 0 {
		fmt.Fprintf(&b, "Already eaten today:\n")
		for _, e := range entries {
			fmt.Fprintf(&b, "  - [%s] %s: %d cal, %dg protein\n", e.MealType, e.Description, e.Calories, e.Protein)
		}
		totalCal, totalProt := 0, 0
		for _, e := range entries {
			totalCal += e.Calories
			totalProt += e.Protein
		}
		fmt.Fprintf(&b, "  Today's totals so far: %d cal, %dg protein\n\n", totalCal, totalProt)
	}

	if len(prevEntries) > 0 {
		fmt.Fprintf(&b, "Yesterday's meals (avoid repeating):\n")
		for _, e := range prevEntries {
			fmt.Fprintf(&b, "  - [%s] %s\n", e.MealType, e.Description)
		}
	}

	return b.String()
}
