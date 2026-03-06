package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"foodtracker/internal/auth"
	"foodtracker/internal/gemini"
	"foodtracker/internal/sheets"

	"github.com/google/uuid"
)

// Handler holds references to auth and gemini services.
// The sheets service is created per-request using the user's token source.
type Handler struct {
	auth   *auth.Handler
	gemini *gemini.Service
}

func NewHandler(authHandler *auth.Handler) *Handler {
	return &Handler{
		auth:   authHandler,
		gemini: gemini.NewService(),
	}
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

func (h *Handler) sheetsSvc(r *http.Request, session *auth.Session) (*sheets.Service, error) {
	ts := h.auth.TokenSource(r.Context(), session)
	return sheets.NewService(r.Context(), ts, session.SpreadsheetID)
}

// ensureSpreadsheet creates the user's spreadsheet on first login.
// Updates the session cookie with the new spreadsheet ID.
// Returns false and writes an error response if it fails.
func (h *Handler) ensureSpreadsheet(w http.ResponseWriter, r *http.Request, session *auth.Session) bool {
	if session.SpreadsheetID != "" {
		return true
	}
	ts := h.auth.TokenSource(r.Context(), session)
	id, err := sheets.CreateSpreadsheet(r.Context(), ts, session.UserEmail)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create spreadsheet: "+err.Error())
		return false
	}
	session.SpreadsheetID = id
	if err := h.auth.SetSession(w, session); err != nil {
		writeErr(w, http.StatusInternalServerError, "session save failed")
		return false
	}
	return true
}

// GET /api/log?date=YYYY-MM-DD   → today's entries grouped with activity note
// GET /api/log?week=true          → last 7 days aggregated
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

	if r.URL.Query().Get("week") == "true" {
		start := sheets.DateString(time.Now().AddDate(0, 0, -6))
		entries, err := svc.GetFoodByDateRange(r.Context(), start, today)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"entries": entries,
			"start":   start,
			"end":     today,
		})
		return
	}

	date := r.URL.Query().Get("date")
	if date == "" {
		date = today
	}
	entries, err := svc.GetFoodByDate(r.Context(), date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	activity, _ := svc.GetActivity(r.Context(), date)
	WriteJSON(w, http.StatusOK, map[string]any{
		"entries":  entries,
		"activity": activity,
		"date":     date,
	})
}

// POST /api/chat — body: {"message": "..."}
// Returns {"done": false, "message": "..."} for clarifying questions.
// Returns {"done": true, "entries": [...]} when entries are logged.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ts := h.auth.TokenSource(r.Context(), session)
	responseText, entries, err := h.gemini.Chat(r.Context(), ts, session.UserEmail, req.Message)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "gemini error: "+err.Error())
		return
	}

	// Clarifying question — nothing to write to Sheets yet
	if len(entries) == 0 {
		WriteJSON(w, http.StatusOK, map[string]any{"done": false, "message": responseText})
		return
	}

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	now := time.Now()
	var saved []sheets.FoodEntry
	for _, e := range entries {
		fe := sheets.FoodEntry{
			ID:          uuid.NewString(),
			Date:        sheets.DateString(now),
			Time:        sheets.TimeString(now),
			MealType:    e.MealType,
			Description: e.Description,
			Calories:    e.Calories,
			Protein:     e.Protein,
			Carbs:       e.Carbs,
			Fat:         e.Fat,
		}
		if err := svc.AppendFood(r.Context(), fe); err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("sheet write: %v", err))
			return
		}
		saved = append(saved, fe)
	}

	WriteJSON(w, http.StatusOK, map[string]any{"done": true, "entries": saved})
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
	notes, err := svc.GetActivity(r.Context(), date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"date": date, "notes": notes})
}

// PUT /api/activity — body: {"date": "YYYY-MM-DD", "notes": "..."}
func (h *Handler) PutActivity(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	var req struct {
		Date  string `json:"date"`
		Notes string `json:"notes"`
	}
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
	if err := svc.SetActivity(r.Context(), req.Date, req.Notes); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"date": req.Date, "notes": req.Notes})
}
