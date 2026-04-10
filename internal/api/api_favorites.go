package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"foodtracker/internal/auth"
	"foodtracker/internal/sheets"
)

// GET /api/favorites
func (h *Handler) GetFavorites(c *echo.Context) error {
	session := auth.SessionFrom(c)

	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	ctx := c.Request().Context()
	favs, err := svc.GetFavorites(ctx)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if favs == nil {
		favs = []sheets.FavoriteEntry{}
	}
	return c.JSON(http.StatusOK, map[string]any{"favorites": favs})
}

// POST /api/favorites
func (h *Handler) AddFavorite(c *echo.Context) error {
	session := auth.SessionFrom(c)

	r := c.Request()
	ctx := r.Context()
	var req struct {
		Description string `json:"description"`
		MealType    string `json:"meal_type"`
		Calories    int    `json:"calories"`
		Protein     int    `json:"protein"`
		Carbs       int    `json:"carbs"`
		Fat         int    `json:"fat"`
		Fiber       int    `json:"fiber"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Description == "" {
		return writeErr(c, http.StatusBadRequest, "invalid request body")
	}
	fav := sheets.FavoriteEntry{
		ID:          uuid.NewString(),
		Description: req.Description,
		MealType:    req.MealType,
		Calories:    req.Calories,
		Protein:     req.Protein,
		Carbs:       req.Carbs,
		Fat:         req.Fat,
		Fiber:       req.Fiber,
		CreatedAt:   sheets.DateString(LocalNow(r)),
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	existing, err := svc.GetFavorites(ctx)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	key := sheets.NormalizeFavoriteKey(fav.Description)
	for _, e := range existing {
		if sheets.NormalizeFavoriteKey(e.Description) == key {
			return writeErr(c, http.StatusConflict, "favorite_exists")
		}
	}
	if err := svc.AddFavorite(ctx, fav); err != nil {
		return h.writeAPIErr(c, err)
	}
	return c.JSON(http.StatusOK, fav)
}

// DELETE /api/favorites/:id
func (h *Handler) DeleteFavorite(c *echo.Context) error {
	session := auth.SessionFrom(c)
	id := c.Param("id")
	if id == "" {
		return writeErr(c, http.StatusBadRequest, "missing id")
	}
	svc, err := h.sheetsSvc(c, session)
	if err != nil {
		return h.writeAPIErr(c, err)
	}
	if err := svc.DeleteFavorite(c.Request().Context(), id); err != nil {
		return h.writeAPIErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
