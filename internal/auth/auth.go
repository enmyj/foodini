package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/securecookie"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const cookieName = "ft_session"

var scopes = []string{
	"openid",
	"email",
	"profile",
	"https://www.googleapis.com/auth/spreadsheets",
	"https://www.googleapis.com/auth/drive.file",
	"https://www.googleapis.com/auth/generative-language",
}

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	CookieSecret string
	Secure       bool
}

type Session struct {
	UserEmail     string `json:"user_email"`
	RefreshToken  string `json:"refresh_token"`
	SpreadsheetID string `json:"spreadsheet_id"`
}

type Handler struct {
	oauthCfg *oauth2.Config
	sc       *securecookie.SecureCookie
	secure   bool
}

func NewHandler(cfg Config) *Handler {
	secret := []byte(cfg.CookieSecret)
	if len(secret) < 64 {
		panic("COOKIE_SECRET must be at least 64 bytes (use: openssl rand -hex 32 gives 64 hex chars)")
	}
	hashKey := secret[:32]
	encKey := secret[32:64]
	return &Handler{
		oauthCfg: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       scopes,
			Endpoint:     google.Endpoint,
		},
		sc:     securecookie.New(hashKey, encKey),
		secure: cfg.Secure,
	}
}

// generateState returns a cryptographically random hex string for OAuth CSRF protection.
func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	state := generateState()
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   300, // 5 minutes
		SameSite: http.SameSiteLaxMode,
	})
	url := h.oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	// Verify CSRF state
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: "", MaxAge: -1, Path: "/"})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	token, err := h.oauthCfg.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	if token.RefreshToken == "" {
		http.Error(w, "no refresh token: re-authorize with prompt=consent", http.StatusInternalServerError)
		return
	}

	client := h.oauthCfg.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		http.Error(w, "userinfo fetch failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		http.Error(w, "userinfo decode failed", http.StatusInternalServerError)
		return
	}

	session := &Session{
		UserEmail:    info.Email,
		RefreshToken: token.RefreshToken,
	}
	if err := h.SetSession(w, session); err != nil {
		http.Error(w, "session save failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (h *Handler) SetSession(w http.ResponseWriter, session *Session) error {
	encoded, err := h.sc.Encode(cookieName, session)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
	return nil
}

func (h *Handler) GetSession(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil, errors.New("no session cookie")
	}
	var session Session
	if err := h.sc.Decode(cookieName, cookie.Value, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// TokenSource returns an oauth2.TokenSource for the given session's refresh token.
func (h *Handler) TokenSource(ctx context.Context, session *Session) oauth2.TokenSource {
	token := &oauth2.Token{RefreshToken: session.RefreshToken}
	return h.oauthCfg.TokenSource(ctx, token)
}

// Authenticated wraps a handler requiring a valid session.
// The session is injected into the request context.
func (h *Handler) Authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := h.GetSession(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), sessionKey{}, session))
		next(w, r)
	}
}

type sessionKey struct{}

func SessionFromContext(ctx context.Context) *Session {
	s, _ := ctx.Value(sessionKey{}).(*Session)
	return s
}
