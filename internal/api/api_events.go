package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/sheets"
)

// GET /api/events?date=YYYY-MM-DD
func (h *Handler) GetEvents(c *echo.Context) error {
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
	events, err := svc.GetEventsByDate(r.Context(), date)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if events == nil {
		events = []sheets.Event{}
	}
	return c.JSON(http.StatusOK, events)
}

// POST /api/events
func (h *Handler) PostEvent(c *echo.Context) error {
	session := auth.SessionFrom(c)
	r := c.Request()
	var req sheets.Event
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid body")
	}
	if req.Date == "" {
		req.Date = sheets.DateString(LocalNow(r))
	}
	if req.Time == "" {
		req.Time = sheets.TimeString(LocalNow(r))
	}
	if req.Kind == "" {
		return writeErr(c, http.StatusBadRequest, "kind required")
	}
	if req.ID == "" {
		req.ID = uuid.NewString()
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if err := svc.AppendEvent(r.Context(), req); err != nil {
		return h.writeAPIErr(c, err)
	}
	h.cacheInvalidate(session.SpreadsheetID)
	return c.JSON(http.StatusOK, req)
}

// PATCH /api/events/:id
func (h *Handler) PatchEvent(c *echo.Context) error {
	session := auth.SessionFrom(c)
	id := c.Param("id")
	if id == "" {
		return writeErr(c, http.StatusBadRequest, "id required")
	}
	r := c.Request()
	var req sheets.Event
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return writeErr(c, http.StatusBadRequest, "invalid body")
	}
	req.ID = id
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if err := svc.UpdateEvent(r.Context(), id, req); err != nil {
		return h.writeAPIErr(c, err)
	}
	h.cacheInvalidate(session.SpreadsheetID)
	return c.JSON(http.StatusOK, req)
}

// DELETE /api/events/:id
func (h *Handler) DeleteEvent(c *echo.Context) error {
	session := auth.SessionFrom(c)
	id := c.Param("id")
	if id == "" {
		return writeErr(c, http.StatusBadRequest, "id required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if err := svc.DeleteEvent(c.Request().Context(), id); err != nil {
		return h.writeAPIErr(c, err)
	}
	h.cacheInvalidate(session.SpreadsheetID)
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
