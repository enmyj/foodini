package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"foodtracker/internal/auth"
	"foodtracker/internal/gemini"
	"foodtracker/internal/sheets"

	"github.com/google/uuid"
)

// Handler holds references to auth and gemini services.
// The sheets service is created per-request using the user's token source.
type Handler struct {
	auth    *auth.Handler
	gemini  *gemini.Service
	cacheMu sync.RWMutex
	cache   map[string]cacheItem
}

type cacheItem struct {
	data    []byte
	expires time.Time
}

const cacheTTL = 60 * time.Second

func NewHandler(authHandler *auth.Handler, geminiAPIKey string) *Handler {
	return &Handler{
		auth:   authHandler,
		gemini: gemini.NewService(geminiAPIKey),
		cache:  make(map[string]cacheItem),
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

// formatProfileContext builds the profile preamble injected into Gemini's system prompt.
// Returns "" if all profile fields are empty.
func formatProfileContext(p sheets.UserProfile) string {
	var parts []string
	if p.Gender != "" {
		parts = append(parts, p.Gender)
	}
	if p.Height != "" {
		parts = append(parts, p.Height)
	}
	if p.Weight != "" {
		parts = append(parts, p.Weight)
	}
	if len(parts) == 0 && p.Notes == "" {
		return ""
	}
	ctx := "User profile: " + strings.Join(parts, ", ")
	if p.Notes != "" {
		ctx += ". " + p.Notes
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
		return true
	}
	ts := h.auth.TokenSource(r.Context(), session)

	// Search Drive for an existing spreadsheet from a previous session
	id, err := sheets.FindExistingSpreadsheet(r.Context(), ts, session.UserEmail)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to search for spreadsheet: "+err.Error())
		return false
	}

	if id != "" {
		// Found an existing spreadsheet — check its schema version
		svc, err := sheets.NewService(r.Context(), ts, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return false
		}
		version, err := svc.GetSchemaVersion(r.Context())
		if err != nil || version < sheets.CurrentSchemaVersion {
			writeErr(w, http.StatusConflict, "incompatible_spreadsheet")
			return false
		}
		session.SpreadsheetID = id
	} else {
		// No existing spreadsheet — create a new one
		id, err = sheets.CreateSpreadsheet(r.Context(), ts, session.UserEmail)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to create spreadsheet: "+err.Error())
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

// GET /api/log?date=YYYY-MM-DD   → today's entries grouped with activity note
// GET /api/log?days=N             → last N days (1-365, default 30)
func (h *Handler) GetLog(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	today := sheets.DateString(time.Now())

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
		start := sheets.DateString(time.Now().AddDate(0, 0, -(days - 1)))
		entries, err := svc.GetFoodByDateRange(r.Context(), start, today)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		dailyLogs, err := svc.GetActivityByDateRange(r.Context(), start, today)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		data, _ := json.Marshal(map[string]any{
			"entries":    entries,
			"daily_logs": dailyLogs,
			"start":      start,
			"end":        today,
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	dayLog, err := svc.GetActivity(r.Context(), date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	data, _ := json.Marshal(map[string]any{
		"entries": entries,
		"day_log": dayLog,
		"date":    date,
	})
	h.cacheSet(cacheKey, data)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// POST /api/chat — body: {"message": "...", "date": "YYYY-MM-DD"}
// Returns {"done": false, "message": "..."} for clarifying questions.
// Returns {"done": false, "pending": true, "entries": [...]} when entries are ready for confirmation.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}

	var req struct {
		Message string `json:"message"`
		Date    string `json:"date"` // optional; defaults to today
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	targetDate := req.Date
	if targetDate == "" {
		targetDate = sheets.DateString(time.Now())
	}

	// Fetch user profile for Gemini context (ignore errors — use empty profile)
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	profile, _ := svc.GetProfile(r.Context())
	profileCtx := formatProfileContext(profile)

	responseText, entries, err := h.gemini.Chat(r.Context(), session.UserEmail, targetDate, req.Message, profileCtx)
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
		targetDate = sheets.DateString(time.Now())
	}

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	now := time.Now()
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
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("sheet write: %v", err))
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := svc.DeleteFood(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, err := svc.GetProfile(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, p)
}

// PUT /api/profile — body: {gender, height, weight, notes}
func (h *Handler) PutProfile(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	var req sheets.UserProfile
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := svc.SetProfile(r.Context(), req); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, req)
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := svc.UpdateFood(r.Context(), id, entry); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		date = sheets.DateString(time.Now())
	}
	dayLog, err := svc.GetActivity(r.Context(), date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, dayLog)
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
		req.Date = sheets.DateString(time.Now())
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := svc.SetActivity(r.Context(), req); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.cacheInvalidate(session.SpreadsheetID)
	WriteJSON(w, http.StatusOK, req)
}
