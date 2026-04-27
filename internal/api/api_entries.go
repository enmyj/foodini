package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/sheets"
)

// POST /api/chat/confirm — persist a list of entries (e.g. logging a favorite).
func (h *Handler) ConfirmChat(c *echo.Context) error {
	session := auth.SessionFrom(c)

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Entries []sheets.FoodEntry `json:"entries"`
		Date    string             `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Entries) == 0 {
		return writeErr(c, http.StatusBadRequest, "invalid request body")
	}

	targetDate := req.Date
	if targetDate == "" {
		targetDate = sheets.DateString(LocalNow(r))
	}

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}

	now := LocalNow(r)
	saved := make([]sheets.FoodEntry, 0, len(req.Entries))
	for _, e := range req.Entries {
		saved = append(saved, sheets.FoodEntry{
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
		})
	}
	if err := svc.AppendFoods(ctx, saved); err != nil {
		return h.writeAPIErr(c, fmt.Errorf("sheet write: %w", err))
	}

	h.cacheInvalidate(session.SpreadsheetID)
	return c.JSON(http.StatusOK, map[string]any{"done": true, "entries": saved})
}

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
