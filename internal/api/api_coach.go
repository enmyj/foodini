package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/gemini"
	"foodtracker/internal/sheets"
)

// POST /api/coach/chat
func (h *Handler) CoachChat(c *echo.Context) error {
	session := auth.SessionFrom(c)
	r := c.Request()
	ctx := r.Context()

	var req struct {
		Messages []gemini.CoachMessage `json:"messages"`
		Date     string                `json:"date"`
		Days     int                   `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid request body")
	}
	if len(req.Messages) == 0 {
		return writeErr(c, http.StatusBadRequest, "messages required")
	}

	days := req.Days
	if days <= 0 {
		days = 7
	}
	if days > 28 {
		days = 28
	}

	endDate := req.Date
	if endDate == "" {
		endDate = sheets.DateString(LocalNow(r))
	}
	endT, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid date")
	}
	startDate := endT.AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
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

	summary, err := h.buildCoachContext(ctx, svc, startDate, endDate)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	stream, err := h.gemini.CoachStream(ctx, req.Messages, summary, profileCtx)
	if err != nil {
		return writeErr(c, http.StatusInternalServerError, "gemini error: "+err.Error())
	}

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	writeEvent := func(event, data string) {
		if event != "" {
			fmt.Fprintf(w, "event: %s\n", event)
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	for chunk, err := range stream {
		if err != nil {
			payload, _ := json.Marshal(map[string]string{"error": err.Error()})
			writeEvent("error", string(payload))
			return nil
		}
		payload, _ := json.Marshal(map[string]string{"text": chunk})
		writeEvent("", string(payload))
	}
	writeEvent("done", "{}")
	return nil
}

func (h *Handler) buildCoachContext(ctx context.Context, svc *sheets.Service, start, end string) (string, error) {
	entries, err := svc.GetFoodByDateRange(ctx, start, end)
	if err != nil {
		return "", err
	}
	dailyLogs, err := svc.GetEventsByDateRange(ctx, start, end)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Food + activity log from %s to %s:\n\n", start, end)
	b.WriteString(buildWeekSummary(start, end, entries, dailyLogs))

	// Pull stored day insights for each of the last 7 days, if any.
	cur, _ := time.Parse("2006-01-02", start)
	endT, _ := time.Parse("2006-01-02", end)
	var insightSection strings.Builder
	for !cur.After(endT) {
		date := cur.Format("2006-01-02")
		if rec, _ := svc.GetInsight(ctx, "day", date, date); rec != nil && strings.TrimSpace(rec.Insight) != "" {
			fmt.Fprintf(&insightSection, "\n%s insights:\n%s\n", date, rec.Insight)
		}
		cur = cur.AddDate(0, 0, 1)
	}
	if insightSection.Len() > 0 {
		b.WriteString("\nDay-level nutrition insights from this period:\n")
		b.WriteString(insightSection.String())
	}

	return b.String(), nil
}
