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

	"foodtracker/internal/auth"
	"foodtracker/internal/gemini"
	"foodtracker/internal/sheets"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

// Handler holds references to auth and gemini services.
// The sheets service is created per-request using the user's token source.
type Handler struct {
	auth    *auth.Handler
	gemini  *gemini.Service
	cacheMu sync.RWMutex
	cache   map[string]cacheItem
	// migratedIDs tracks spreadsheet IDs that have been confirmed at
	// CurrentSchemaVersion this server lifetime, so we only pay the
	// GetSchemaVersion cost once per restart for already-current sheets.
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

// cacheInvalidate removes all log cache entries for a spreadsheet.
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

// Authenticated delegates to the auth handler's middleware.
func (h *Handler) Authenticated(next http.HandlerFunc) http.HandlerFunc {
	return h.auth.Authenticated(next)
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
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

// writeAPIErr maps auth and Google API failures to stable frontend-facing errors.
func (h *Handler) writeAPIErr(w http.ResponseWriter, err error) {
	if isSessionExpiredErr(err) {
		h.auth.ClearSession(w)
		writeErr(w, http.StatusUnauthorized, "session_expired")
		return
	}
	if isInsufficientScopesErr(err) {
		writeErr(w, http.StatusForbidden, "insufficient_scopes")
		return
	}
	writeErr(w, http.StatusInternalServerError, err.Error())
}

// LocalNow returns the current time in the user's local timezone.
// It reads the IANA timezone name from the X-Timezone request header.
// Falls back to server time if the header is missing or invalid.
func LocalNow(r *http.Request) time.Time {
	tz := r.Header.Get("X-Timezone")
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return time.Now().In(loc)
		}
	}
	return time.Now()
}

// formatProfileContext builds the profile preamble injected into Gemini's system prompt.
// Returns "" if all profile fields are empty. currentYear is used to compute
// age from the stored birth year.
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

func (h *Handler) sheetsSvc(r *http.Request, session *auth.Session) (*sheets.Service, error) {
	ts := h.auth.TokenSource(r.Context(), session)
	return sheets.NewService(r.Context(), ts, session.SpreadsheetID)
}

// ensureSpreadsheet finds or creates the user's spreadsheet.
// Updates the session cookie with the spreadsheet ID.
// Returns false and writes an error response if it fails.
func (h *Handler) ensureSpreadsheet(w http.ResponseWriter, r *http.Request, session *auth.Session) bool {
	if session.SpreadsheetID != "" {
		// Fast path: if we've already confirmed this spreadsheet is at the
		// current schema version this server lifetime, skip the version check.
		h.migratedMu.RLock()
		done := h.migratedIDs[session.SpreadsheetID]
		h.migratedMu.RUnlock()
		if done {
			return true
		}
		// Slow path: check version and run any missing migrations.
		ts := h.auth.TokenSource(r.Context(), session)
		svc, err := sheets.NewService(r.Context(), ts, session.SpreadsheetID)
		if err != nil {
			h.writeAPIErr(w, err)
			return false
		}
		version, err := svc.GetSchemaVersion(r.Context())
		if err != nil {
			h.writeAPIErr(w, err)
			return false
		}
		if version < sheets.CurrentSchemaVersion {
			if !h.runMigrations(w, r, ts, session.SpreadsheetID, version) {
				return false
			}
		}
		h.migratedMu.Lock()
		h.migratedIDs[session.SpreadsheetID] = true
		h.migratedMu.Unlock()
		return true
	}
	ts := h.auth.TokenSource(r.Context(), session)

	// Search Drive for an existing spreadsheet from a previous session
	id, err := sheets.FindExistingSpreadsheet(r.Context(), ts, session.UserEmail)
	if err != nil {
		h.writeAPIErr(w, err)
		return false
	}

	if id != "" {
		// Found an existing spreadsheet — check its schema version
		svc, err := sheets.NewService(r.Context(), ts, id)
		if err != nil {
			h.writeAPIErr(w, err)
			return false
		}
		version, err := svc.GetSchemaVersion(r.Context())
		if err != nil {
			h.writeAPIErr(w, err)
			return false
		}
		if version < 1 {
			writeErr(w, http.StatusConflict, "incompatible_spreadsheet")
			return false
		}
		if !h.runMigrations(w, r, ts, id, version) {
			return false
		}
		session.SpreadsheetID = id
	} else {
		// No existing spreadsheet — create a new one
		id, err = sheets.CreateSpreadsheet(r.Context(), ts, session.UserEmail)
		if err != nil {
			h.writeAPIErr(w, err)
			return false
		}
		session.SpreadsheetID = id
	}

	if err := h.auth.SetSession(w, session); err != nil {
		writeErr(w, http.StatusInternalServerError, "session save failed")
		return false
	}
	return true
}

// runMigrations applies all schema upgrades from version → CurrentSchemaVersion.
// Returns false and writes an error response if any step fails.
func (h *Handler) runMigrations(w http.ResponseWriter, r *http.Request, ts oauth2.TokenSource, spreadsheetID string, version int) bool {
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
			if err := s.migrate(r.Context(), ts, spreadsheetID); err != nil {
				writeErr(w, http.StatusInternalServerError, "migration failed: "+err.Error())
				return false
			}
			version++
		}
	}
	return true
}

// GET /api/log?date=YYYY-MM-DD   → today's entries grouped with activity note
// GET /api/log?days=N             → last N days (1-365, default 30)
func (h *Handler) GetLog(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}

	today := sheets.DateString(LocalNow(r))

	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		days, err := strconv.Atoi(daysStr)
		if err != nil || days < 1 || days > 365 {
			days = 30
		}
		cacheKey := session.SpreadsheetID + "|days|" + strconv.Itoa(days)
		if cached, ok := h.cacheGet(cacheKey); ok {
			w.Header().Set("Content-Type", "application/json")
			w.Write(cached)
			return
		}
		start := sheets.DateString(LocalNow(r).AddDate(0, 0, -(days - 1)))
		entries, err := svc.GetFoodByDateRange(r.Context(), start, today)
		if err != nil {
			h.writeAPIErr(w, err)
			return
		}
		dailyLogs, err := svc.GetActivityByDateRange(r.Context(), start, today)
		if err != nil {
			h.writeAPIErr(w, err)
			return
		}
		data, _ := json.Marshal(map[string]any{
			"entries":         entries,
			"daily_logs":      dailyLogs,
			"start":           start,
			"end":             today,
			"spreadsheet_url": "https://docs.google.com/spreadsheets/d/" + session.SpreadsheetID,
		})
		h.cacheSet(cacheKey, data)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
		return
	}

	date := r.URL.Query().Get("date")
	if date == "" {
		date = today
	}
	cacheKey := session.SpreadsheetID + "|date|" + date
	if cached, ok := h.cacheGet(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Write(cached)
		return
	}
	entries, err := svc.GetFoodByDate(r.Context(), date)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	dayLog, err := svc.GetActivity(r.Context(), date)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	data, _ := json.Marshal(map[string]any{
		"entries":         entries,
		"day_log":         dayLog,
		"date":            date,
		"spreadsheet_url": "https://docs.google.com/spreadsheets/d/" + session.SpreadsheetID,
	})
	h.cacheSet(cacheKey, data)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// POST /api/chat — accepts JSON or multipart/form-data with optional images.
// Returns {"done": false, "message": "..."} for clarifying questions.
// Returns {"done": false, "pending": true, "entries": [...]} when entries are ready for confirmation.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}

	req, err := parseChatRequest(r)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeErr(w, http.StatusRequestEntityTooLarge, "upload_too_large")
			return
		}
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Message) == "" && len(req.Images) == 0 {
		writeErr(w, http.StatusBadRequest, "message or image required")
		return
	}

	targetDate := req.Date
	if targetDate == "" {
		targetDate = sheets.DateString(LocalNow(r))
	}

	// Fetch user profile for Gemini context (cached; invalidated on profile save)
	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		svc, err := h.sheetsSvc(r, session)
		if err != nil {
			h.writeAPIErr(w, err)
			return
		}
		profile, _ := svc.GetProfile(r.Context())
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	message := req.Message
	if req.Meal != "" {
		message = "(meal type: " + req.Meal + ") " + message
	}

	responseText, entries, err := h.gemini.Chat(r.Context(), session.UserEmail, targetDate, message, profileCtx, req.Images)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "gemini error: "+err.Error())
		return
	}

	if len(entries) == 0 {
		WriteJSON(w, http.StatusOK, map[string]any{"done": false, "message": responseText})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
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

// POST /api/chat/confirm — body: {"entries": [...], "date": "YYYY-MM-DD"}
// Saves confirmed entries returned from a pending chat response.
func (h *Handler) ConfirmChat(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}

	var req struct {
		Entries []sheets.FoodEntry `json:"entries"`
		Date    string             `json:"date"` // optional; defaults to today
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Entries) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	targetDate := req.Date
	if targetDate == "" {
		targetDate = sheets.DateString(LocalNow(r))
	}

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
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
		if err := svc.AppendFood(r.Context(), fe); err != nil {
			h.writeAPIErr(w, fmt.Errorf("sheet write: %w", err))
			return
		}
		saved = append(saved, fe)
	}

	h.gemini.ClearConversation(session.UserEmail, targetDate)
	h.cacheInvalidate(session.SpreadsheetID)
	WriteJSON(w, http.StatusOK, map[string]any{"done": true, "entries": saved})
}

// DELETE /api/entries/{id}
func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "missing id")
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	if err := svc.DeleteFood(r.Context(), id); err != nil {
		h.writeAPIErr(w, err)
		return
	}
	h.cacheInvalidate(session.SpreadsheetID)
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/profile
func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	p, err := svc.GetProfile(r.Context())
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, p)
}

// PUT /api/profile — body: {gender, age, height, weight, notes, goals, dietary_restrictions}
func (h *Handler) PutProfile(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	var req sheets.UserProfile
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	if err := svc.SetProfile(r.Context(), req); err != nil {
		h.writeAPIErr(w, err)
		return
	}
	h.cacheInvalidate(session.SpreadsheetID)
	WriteJSON(w, http.StatusOK, req)
}

// GET /api/favorites
func (h *Handler) GetFavorites(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	favs, err := svc.GetFavorites(r.Context())
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	if favs == nil {
		favs = []sheets.FavoriteEntry{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"favorites": favs})
}

// POST /api/favorites — body: {description, meal_type, calories, protein, carbs, fat, fiber}
func (h *Handler) AddFavorite(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}
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
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
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
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	existing, err := svc.GetFavorites(r.Context())
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	key := sheets.NormalizeFavoriteKey(fav.Description)
	for _, e := range existing {
		if sheets.NormalizeFavoriteKey(e.Description) == key {
			writeErr(w, http.StatusConflict, "favorite_exists")
			return
		}
	}
	if err := svc.AddFavorite(r.Context(), fav); err != nil {
		h.writeAPIErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, fav)
}

// DELETE /api/favorites/{id}
func (h *Handler) DeleteFavorite(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "missing id")
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	if err := svc.DeleteFavorite(r.Context(), id); err != nil {
		h.writeAPIErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PATCH /api/entries/{id} — body: FoodEntry JSON (all fields)
func (h *Handler) PatchEntry(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "missing id")
		return
	}
	var entry sheets.FoodEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	entry.ID = id

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	if err := svc.UpdateFood(r.Context(), id, entry); err != nil {
		h.writeAPIErr(w, err)
		return
	}
	h.cacheInvalidate(session.SpreadsheetID)
	WriteJSON(w, http.StatusOK, entry)
}

// GET /api/activity?date=YYYY-MM-DD
func (h *Handler) GetActivity(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		date = sheets.DateString(LocalNow(r))
	}
	dayLog, err := svc.GetActivity(r.Context(), date)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, dayLog)
}

// POST /api/insights — body: {"start": "YYYY-MM-DD", "end": "YYYY-MM-DD"}
// Returns {"insight": "..."} — a free-form Gemini analysis of the week.
func (h *Handler) Insights(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}

	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Start == "" || req.End == "" {
		writeErr(w, http.StatusBadRequest, "start and end dates required")
		return
	}
	if _, err := time.Parse("2006-01-02", req.Start); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid start date")
		return
	}
	if _, err := time.Parse("2006-01-02", req.End); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid end date")
		return
	}
	startT, _ := time.Parse("2006-01-02", req.Start)
	endT, _ := time.Parse("2006-01-02", req.End)
	if endT.Sub(startT) > 31*24*time.Hour {
		writeErr(w, http.StatusBadRequest, "date range too large")
		return
	}

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}

	entries, err := svc.GetFoodByDateRange(r.Context(), req.Start, req.End)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	dailyLogs, err := svc.GetActivityByDateRange(r.Context(), req.Start, req.End)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}

	summary := buildWeekSummary(req.Start, req.End, entries, dailyLogs)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(r.Context())
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	insight, err := h.gemini.Insights(r.Context(), summary, profileCtx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "gemini error: "+err.Error())
		return
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(r.Context(), sheets.InsightRecord{
		Type:        "week",
		StartDate:   req.Start,
		EndDate:     req.End,
		GeneratedAt: generatedAt,
		Insight:     insight,
	})
	WriteJSON(w, http.StatusOK, map[string]any{"insight": insight, "generated_at": generatedAt})
}

// POST /api/insights/day — body: {"date": "YYYY-MM-DD"}
// Returns {"insight": "..."} — a single-day Gemini analysis.
func (h *Handler) DayInsights(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}

	var req struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Date == "" {
		writeErr(w, http.StatusBadRequest, "date required")
		return
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid date")
		return
	}

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}

	entries, err := svc.GetFoodByDateRange(r.Context(), req.Date, req.Date)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	dailyLogs, err := svc.GetActivityByDateRange(r.Context(), req.Date, req.Date)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}

	if len(entries) == 0 && len(dailyLogs) == 0 {
		writeErr(w, http.StatusBadRequest, "no data for this day")
		return
	}

	today := sheets.DateString(LocalNow(r))
	summary := buildDaySummary(req.Date, entries, dailyLogs, req.Date == today)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(r.Context())
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	insight, err := h.gemini.DayInsights(r.Context(), summary, profileCtx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "gemini error: "+err.Error())
		return
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(r.Context(), sheets.InsightRecord{
		Type:        "day",
		StartDate:   req.Date,
		EndDate:     req.Date,
		GeneratedAt: generatedAt,
		Insight:     insight,
	})
	WriteJSON(w, http.StatusOK, map[string]any{"insight": insight, "generated_at": generatedAt})
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
	// Reuse buildWeekSummary's per-day logic but with a single-day header.
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

// GET /api/insights?start=YYYY-MM-DD&end=YYYY-MM-DD — returns most recent stored week insight or null
func (h *Handler) GetStoredInsights(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		writeErr(w, http.StatusBadRequest, "start and end required")
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	rec, err := svc.GetInsight(r.Context(), "week", start, end)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	if rec == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"insight": nil, "generated_at": nil})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"insight": rec.Insight, "generated_at": rec.GeneratedAt})
}

// GET /api/insights/day?date=YYYY-MM-DD — returns most recent stored day insight or null
func (h *Handler) GetStoredDayInsights(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		writeErr(w, http.StatusBadRequest, "date required")
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	rec, err := svc.GetInsight(r.Context(), "day", date, date)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	if rec == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"insight": nil, "generated_at": nil})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"insight": rec.Insight, "generated_at": rec.GeneratedAt})
}

// POST /api/suggestions/day — body: {"date": "YYYY-MM-DD"}
// Generates meal suggestions. If B/L/D all present → next-day suggestions; otherwise → remaining meal suggestions.
// Returns {"suggestions": "...", "type": "remaining"|"next-day", "generated_at": "..."}
func (h *Handler) DaySuggestions(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}

	var req struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Date == "" {
		writeErr(w, http.StatusBadRequest, "date required")
		return
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid date")
		return
	}

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}

	entries, err := svc.GetFoodByDateRange(r.Context(), req.Date, req.Date)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}

	// Determine which meals are present
	hasMeal := map[string]bool{}
	for _, e := range entries {
		hasMeal[e.MealType] = true
	}
	complete := hasMeal["breakfast"] && hasMeal["lunch"] && hasMeal["dinner"]

	// Get previous day's entries for variety
	prevDate := addDaysStr(req.Date, -1)
	prevEntries, _ := svc.GetFoodByDateRange(r.Context(), prevDate, prevDate)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(r.Context())
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	// Fetch existing day insights to inform suggestions
	var insightText string
	if rec, _ := svc.GetInsight(r.Context(), "day", req.Date, req.Date); rec != nil {
		insightText = rec.Insight
	}

	summary := buildMealSuggestionSummary(req.Date, entries, prevEntries, complete, insightText)

	suggestions, err := h.gemini.MealSuggestions(r.Context(), summary, profileCtx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "gemini error: "+err.Error())
		return
	}

	sugType := "remaining"
	if complete {
		sugType = "next-day"
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(r.Context(), sheets.InsightRecord{
		Type:        "day-suggestions",
		StartDate:   req.Date,
		EndDate:     req.Date,
		GeneratedAt: generatedAt,
		Insight:     sugType + "\n" + suggestions,
	})
	WriteJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions, "type": sugType, "generated_at": generatedAt})
}

// GET /api/suggestions/day?date=YYYY-MM-DD — returns most recent stored day suggestions or null
func (h *Handler) GetStoredDaySuggestions(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		writeErr(w, http.StatusBadRequest, "date required")
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	rec, err := svc.GetInsight(r.Context(), "day-suggestions", date, date)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	if rec == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"suggestions": nil, "type": nil, "generated_at": nil})
		return
	}
	// Parse type from stored format: "type\nsuggestions"
	sugType := "remaining"
	sugText := rec.Insight
	if parts := strings.SplitN(rec.Insight, "\n", 2); len(parts) == 2 {
		sugType = parts[0]
		sugText = parts[1]
	}
	WriteJSON(w, http.StatusOK, map[string]any{"suggestions": sugText, "type": sugType, "generated_at": rec.GeneratedAt})
}

// POST /api/suggestions/week — body: {"start": "YYYY-MM-DD", "end": "YYYY-MM-DD"}
func (h *Handler) WeekSuggestions(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}

	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Start == "" || req.End == "" {
		writeErr(w, http.StatusBadRequest, "start and end dates required")
		return
	}
	if _, err := time.Parse("2006-01-02", req.Start); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid start date")
		return
	}
	if _, err := time.Parse("2006-01-02", req.End); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid end date")
		return
	}

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}

	entries, err := svc.GetFoodByDateRange(r.Context(), req.Start, req.End)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	dailyLogs, err := svc.GetActivityByDateRange(r.Context(), req.Start, req.End)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}

	summary := buildWeekSummary(req.Start, req.End, entries, dailyLogs)

	// Fetch existing week insights to inform suggestions
	if rec, _ := svc.GetInsight(r.Context(), "week", req.Start, req.End); rec != nil {
		summary += "\nInsights for this week:\n" + rec.Insight + "\n"
	}

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(r.Context())
		profileCtx = formatProfileContext(profile, LocalNow(r).Year())
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	suggestions, err := h.gemini.WeekMealSuggestions(r.Context(), summary, profileCtx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "gemini error: "+err.Error())
		return
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = svc.SaveInsight(r.Context(), sheets.InsightRecord{
		Type:        "week-suggestions",
		StartDate:   req.Start,
		EndDate:     req.End,
		GeneratedAt: generatedAt,
		Insight:     suggestions,
	})
	WriteJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions, "generated_at": generatedAt})
}

// GET /api/suggestions/week?start=YYYY-MM-DD&end=YYYY-MM-DD
func (h *Handler) GetStoredWeekSuggestions(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		writeErr(w, http.StatusBadRequest, "start and end required")
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	rec, err := svc.GetInsight(r.Context(), "week-suggestions", start, end)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	if rec == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"suggestions": nil, "generated_at": nil})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"suggestions": rec.Insight, "generated_at": rec.GeneratedAt})
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
		// Figure out missing meals
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

	// Include insights if available — suggestions should address these
	if insightText != "" {
		fmt.Fprintf(&b, "Nutrition insights for today (factor these into suggestions):\n%s\n\n", insightText)
	}

	// What was eaten today
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

	// Yesterday's meals (for variety)
	if len(prevEntries) > 0 {
		fmt.Fprintf(&b, "Yesterday's meals (avoid repeating):\n")
		for _, e := range prevEntries {
			fmt.Fprintf(&b, "  - [%s] %s\n", e.MealType, e.Description)
		}
	}

	return b.String()
}

// PUT /api/activity — body: {"date": "YYYY-MM-DD", "activity": "...", "feeling_score": 0, "feeling_notes": "..."}
func (h *Handler) PutActivity(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	var req sheets.DayLog
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Date == "" {
		req.Date = sheets.DateString(LocalNow(r))
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		h.writeAPIErr(w, err)
		return
	}
	if err := svc.SetActivity(r.Context(), req); err != nil {
		h.writeAPIErr(w, err)
		return
	}
	h.cacheInvalidate(session.SpreadsheetID)
	WriteJSON(w, http.StatusOK, req)
}
