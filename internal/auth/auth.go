package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/securecookie"
	"github.com/labstack/echo/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const cookieName = "ft_session"

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
	tsCache  map[string]oauth2.TokenSource
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
			Scopes:       scopes,
			Endpoint:     google.Endpoint,
		},
		sc:      securecookie.New(hashKey, encKey),
		secure:  cfg.Secure,
		tsCache: make(map[string]oauth2.TokenSource),
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

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) Login(c *echo.Context) error {
	r := c.Request()
	state := generateState()
	c.SetCookie(&http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   300,
		SameSite: http.SameSiteLaxMode,
	})
	opts := []oauth2.AuthCodeOption{oauth2.AccessTypeOffline}
	if c.QueryParam("consent") == "1" {
		opts = append(opts, oauth2.ApprovalForce)
	} else {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
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
		return c.Redirect(http.StatusTemporaryRedirect, "/auth/login?consent=1")
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
		MaxAge:   30 * 24 * 3600,
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
	if ts, ok := h.tsCache[session.RefreshToken]; ok {
		return ts
	}
	base := &oauth2.Token{RefreshToken: session.RefreshToken}
	ts := oauth2.ReuseTokenSource(nil, h.oauthCfg.TokenSource(context.Background(), base))
	h.tsCache[session.RefreshToken] = ts
	return ts
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
