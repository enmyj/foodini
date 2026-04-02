package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"google.golang.org/api/googleapi"
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

// writeGoogleErr checks if err is a Google API 403 (insufficient scopes) and
// writes the appropriate response. Falls back to a 500 for other errors.
func writeGoogleErr(w http.ResponseWriter, err error) {
	var ge *googleapi.Error
	if errors.As(err, &ge) && ge.Code == 403 {
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
	if len(parts) == 0 && p.Notes == "" && p.Goals == "" {
		return ""
	}
	ctx := "User profile: " + strings.Join(parts, ", ")
	if p.Notes != "" {
		ctx += ". " + p.Notes
	}
	if p.Goals != "" {
		ctx += ". Goals: " + p.Goals
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
		writeGoogleErr(w, err)
		return false
	}

	if id != "" {
		// Found an existing spreadsheet — check its schema version
		svc, err := sheets.NewService(r.Context(), ts, id)
		if err != nil {
			writeGoogleErr(w, err)
			return false
		}
		version, err := svc.GetSchemaVersion(r.Context())
		if err != nil {
			writeGoogleErr(w, err)
			return false
		}
		if version == 1 {
			// Migrate v1 → v2: add poop columns to Activity sheet header
			if err := sheets.MigrateV1toV2(r.Context(), ts, id); err != nil {
				writeErr(w, http.StatusInternalServerError, "migration failed: "+err.Error())
				return false
			}
			version = 2
		}
		if version == 2 {
			// Migrate v2 → v3: add hydration column to Activity sheet header
			if err := sheets.MigrateV2toV3(r.Context(), ts, id); err != nil {
				writeErr(w, http.StatusInternalServerError, "migration failed: "+err.Error())
				return false
			}
			version = 3
		}
		if version == 3 {
			// Migrate v3 → v4: add goals column to Profile sheet header
			if err := sheets.MigrateV3toV4(r.Context(), ts, id); err != nil {
				writeErr(w, http.StatusInternalServerError, "migration failed: "+err.Error())
				return false
			}
		}
		if version < 1 {
			writeErr(w, http.StatusConflict, "incompatible_spreadsheet")
			return false
		}
		session.SpreadsheetID = id
	} else {
		// No existing spreadsheet — create a new one
		id, err = sheets.CreateSpreadsheet(r.Context(), ts, session.UserEmail)
		if err != nil {
			writeGoogleErr(w, err)
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
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		dailyLogs, err := svc.GetActivityByDateRange(r.Context(), start, today)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	dayLog, err := svc.GetActivity(r.Context(), date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		Date    string `json:"date"`   // optional; defaults to today
		Meal    string `json:"meal"`   // optional; hints the meal type to Gemini
		Image   *struct {
			MIMEType string `json:"mime_type"`
			Data     string `json:"data"` // base64-encoded
		} `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Message) == "" && req.Image == nil {
		writeErr(w, http.StatusBadRequest, "message or image required")
		return
	}

	var imgData *gemini.ImageData
	if req.Image != nil {
		decoded, err := base64.StdEncoding.DecodeString(req.Image.Data)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid image data")
			return
		}
		imgData = &gemini.ImageData{MIMEType: req.Image.MIMEType, Data: decoded}
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
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		profile, _ := svc.GetProfile(r.Context())
		profileCtx = formatProfileContext(profile)
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	message := req.Message
	if req.Meal != "" {
		message = "(meal type: " + req.Meal + ") " + message
	}

	responseText, entries, err := h.gemini.Chat(r.Context(), session.UserEmail, targetDate, message, profileCtx, imgData)
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
		targetDate = sheets.DateString(LocalNow(r))
	}

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	h.cacheInvalidate(session.SpreadsheetID)
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
		date = sheets.DateString(LocalNow(r))
	}
	dayLog, err := svc.GetActivity(r.Context(), date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	entries, err := svc.GetFoodByDateRange(r.Context(), req.Start, req.End)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	dailyLogs, err := svc.GetActivityByDateRange(r.Context(), req.Start, req.End)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	summary := buildWeekSummary(req.Start, req.End, entries, dailyLogs)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(r.Context())
		profileCtx = formatProfileContext(profile)
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	insight, err := h.gemini.Insights(r.Context(), summary, profileCtx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "gemini error: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"insight": insight})
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	entries, err := svc.GetFoodByDateRange(r.Context(), req.Date, req.Date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	dailyLogs, err := svc.GetActivityByDateRange(r.Context(), req.Date, req.Date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(entries) == 0 && len(dailyLogs) == 0 {
		writeErr(w, http.StatusBadRequest, "no data for this day")
		return
	}

	summary := buildDaySummary(req.Date, entries, dailyLogs)

	profileCacheKey := session.SpreadsheetID + "|profile"
	var profileCtx string
	if cached, ok := h.cacheGet(profileCacheKey); ok {
		profileCtx = string(cached)
	} else {
		profile, _ := svc.GetProfile(r.Context())
		profileCtx = formatProfileContext(profile)
		h.cacheSet(profileCacheKey, []byte(profileCtx))
	}

	insight, err := h.gemini.DayInsights(r.Context(), summary, profileCtx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "gemini error: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"insight": insight})
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

func buildDaySummary(date string, entries []sheets.FoodEntry, dailyLogs []sheets.DayLog) string {
	// Reuse buildWeekSummary's per-day logic but with a single-day header.
	logByDate := map[string]sheets.DayLog{}
	for _, l := range dailyLogs {
		logByDate[l.Date] = l
	}
	t, _ := time.Parse("2006-01-02", date)
	log := logByDate[date]

	var b strings.Builder
	fmt.Fprintf(&b, "Day: %s (%s)\n\n", date, t.Weekday())
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
