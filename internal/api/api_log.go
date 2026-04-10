package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/sheets"
)

// GET /api/log?date=YYYY-MM-DD or ?days=N
func (h *Handler) GetLog(c *echo.Context) error {
	session := auth.SessionFrom(c)

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
