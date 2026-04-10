package api

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/sheets"
)

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
	return c.JSON(http.StatusOK, dayLog)
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
	return c.JSON(http.StatusOK, req)
}
