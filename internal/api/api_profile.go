package api

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/sheets"
)

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
	return c.JSON(http.StatusOK, p)
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
	return c.JSON(http.StatusOK, req)
}
