package api

import (
	"net/http"

	"foodtracker/internal/auth"
)

type Handler struct{}

func NewHandler(ah *auth.Handler) *Handler { return &Handler{} }

func (h *Handler) Authenticated(next http.HandlerFunc) http.HandlerFunc { return next }

func (h *Handler) GetLog(w http.ResponseWriter, r *http.Request)      {}
func (h *Handler) GetActivity(w http.ResponseWriter, r *http.Request) {}
func (h *Handler) PutActivity(w http.ResponseWriter, r *http.Request) {}
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request)        {}
func (h *Handler) PatchEntry(w http.ResponseWriter, r *http.Request)  {}
