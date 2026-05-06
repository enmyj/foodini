package api

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
)

// DELETE /api/fueling/:id
func (h *Handler) DeleteFueling(c *echo.Context) error {
	session := auth.SessionFrom(c)
	id := c.Param("id")
	if id == "" {
		return writeErr(c, http.StatusBadRequest, "id required")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if err := svc.DeleteFueling(c.Request().Context(), id); err != nil {
		return h.writeAPIErr(c, err)
	}
	h.cacheInvalidate(session.SpreadsheetID)
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
