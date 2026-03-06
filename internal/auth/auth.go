package auth

import "net/http"

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	CookieSecret string
}

type Handler struct{}

func NewHandler(cfg Config) *Handler { return &Handler{} }

func (h *Handler) Login(w http.ResponseWriter, r *http.Request)    {}
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {}
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request)   {}
