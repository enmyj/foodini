package api

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/sheets"
)

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
	return c.JSON(http.StatusOK, entry)
}
