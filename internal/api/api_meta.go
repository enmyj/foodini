package api

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"foodtracker/internal/gemini"
)

func (h *Handler) GetSystemPrompt(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"prompt": gemini.InsightsSystemPrompt,
	})
}
