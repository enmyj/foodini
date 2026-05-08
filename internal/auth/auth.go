package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/labstack/echo/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	cookieName        = "ft_session"
	sessionCookieAge  = 180 * 24 * 3600
	missingRefreshMsg = "Google sign-in did not return offline access. Please try signing in again."
	tsCacheTTL        = 1 * time.Hour
	tsCacheJanitor    = 15 * time.Minute
)

var scopes = []string{
	"openid",
	"email",
	"profile",
	"https://www.googleapis.com/auth/drive.file",
}

type Config struct {
	ClientID     string
	ClientSecret string
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
	tsMu     sync.Mutex
	tsCache  map[string]*tsEntry
}

type tsEntry struct {
	ts       oauth2.TokenSource
	lastUsed time.Time
}

func NewHandler(cfg Config) *Handler {
	secret := []byte(cfg.CookieSecret)
	if len(secret) < 64 {
		panic("COOKIE_SECRET must be at least 64 bytes (use: openssl rand -hex 32 gives 64 hex chars)")
	}
	hashKey := secret[:32]
	encKey := secret[32:64]
	h := &Handler{
		oauthCfg: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Scopes:       scopes,
			Endpoint:     google.Endpoint,
		},
		sc:      securecookie.New(hashKey, encKey),
		secure:  cfg.Secure,
		tsCache: make(map[string]*tsEntry),
	}
	go h.tsCacheJanitor(tsCacheJanitor)
	return h
}

func (h *Handler) tsCacheJanitor(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for now := range t.C {
		cutoff := now.Add(-tsCacheTTL)
		h.tsMu.Lock()
		for k, e := range h.tsCache {
			if e.lastUsed.Before(cutoff) {
				delete(h.tsCache, k)
			}
		}
		h.tsMu.Unlock()
	}
}

// redirectURL derives the OAuth callback URL from the incoming request.
func redirectURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + r.Host + "/auth/callback"
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (h *Handler) Login(c *echo.Context) error {
	r := c.Request()
	state, err := generateState()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "state generation failed"})
	}
	c.SetCookie(&http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   300,
		SameSite: http.SameSiteLaxMode,
	})
	prompt := "consent"
	if c.QueryParam("consent") == "1" {
		prompt = "consent select_account"
	}
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", prompt),
	}
	opts = append(opts, oauth2.SetAuthURLParam("redirect_uri", redirectURL(r)))
	url := h.oauthCfg.AuthCodeURL(state, opts...)
	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *Handler) Callback(c *echo.Context) error {
	r := c.Request()

	stateCookie, err := c.Cookie("oauth_state")
	if err != nil || c.QueryParam("state") != stateCookie.Value {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid oauth state"})
	}
	c.SetCookie(&http.Cookie{Name: "oauth_state", Value: "", MaxAge: -1, Path: "/"})

	code := c.QueryParam("code")
	if code == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing code"})
	}

	ctx := r.Context()
	token, err := h.oauthCfg.Exchange(ctx, code, oauth2.SetAuthURLParam("redirect_uri", redirectURL(r)))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "token exchange failed"})
	}

	if token.RefreshToken == "" {
		return c.String(http.StatusBadRequest, missingRefreshMsg)
	}

	client := h.oauthCfg.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "userinfo fetch failed"})
	}
	defer resp.Body.Close()

	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "userinfo decode failed"})
	}

	session := &Session{
		UserEmail:    info.Email,
		RefreshToken: token.RefreshToken,
	}
	if err := h.SetSession(c, session); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "session save failed"})
	}
	return c.Redirect(http.StatusTemporaryRedirect, "/")
}

func (h *Handler) Logout(c *echo.Context) error {
	if s, err := h.GetSession(c.Request()); err == nil {
		h.dropTokenSource(s.RefreshToken)
	}
	h.ClearSession(c)
	return c.Redirect(http.StatusTemporaryRedirect, "/")
}

func (h *Handler) ClearSession(c *echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (h *Handler) SetSession(c *echo.Context, session *Session) error {
	encoded, err := h.sc.Encode(cookieName, session)
	if err != nil {
		return err
	}
	c.SetCookie(&http.Cookie{
		Name:     cookieName,
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   sessionCookieAge,
	})
	return nil
}

func (h *Handler) GetSession(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := h.sc.Decode(cookieName, cookie.Value, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// TokenSource returns a cached oauth2.TokenSource for the given session's refresh token.
func (h *Handler) TokenSource(_ context.Context, session *Session) oauth2.TokenSource {
	h.tsMu.Lock()
	defer h.tsMu.Unlock()
	if e, ok := h.tsCache[session.RefreshToken]; ok {
		e.lastUsed = time.Now()
		return e.ts
	}
	base := &oauth2.Token{RefreshToken: session.RefreshToken}
	ts := oauth2.ReuseTokenSource(nil, h.oauthCfg.TokenSource(context.Background(), base))
	h.tsCache[session.RefreshToken] = &tsEntry{ts: ts, lastUsed: time.Now()}
	return ts
}

// dropTokenSource removes a refresh token's cached TokenSource, e.g. on logout.
func (h *Handler) dropTokenSource(refreshToken string) {
	if refreshToken == "" {
		return
	}
	h.tsMu.Lock()
	delete(h.tsCache, refreshToken)
	h.tsMu.Unlock()
}

// AuthMiddleware returns Echo middleware that requires a valid session.
func (h *Handler) AuthMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			session, err := h.GetSession(c.Request())
			if err != nil {
				h.ClearSession(c)
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			c.Set("session", session)
			return next(c)
		}
	}
}

// SessionFrom retrieves the session stored in the Echo context by AuthMiddleware.
func SessionFrom(c *echo.Context) *Session {
	s, _ := c.Get("session").(*Session)
	return s
}
