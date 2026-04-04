package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/securecookie"
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
// It honours X-Forwarded-Proto (set by Cloud Run and most reverse proxies) so
// the correct scheme is used in both local dev (http) and production (https).
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
	opts := []oauth2.AuthCodeOption{oauth2.AccessTypeOffline}
	if r.URL.Query().Get("consent") == "1" {
		// Force the consent screen to guarantee a refresh token (used when the
		// default flow didn't return one, or when re-authorizing after scope errors).
		opts = append(opts, oauth2.ApprovalForce)
	} else {
		// Show only the account picker; skip the permissions review if the user
		// has already authorized. The callback falls back to ?consent=1 if Google
		// doesn't issue a refresh token.
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	opts = append(opts, oauth2.SetAuthURLParam("redirect_uri", redirectURL(r)))
	url := h.oauthCfg.AuthCodeURL(state, opts...)
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
	token, err := h.oauthCfg.Exchange(ctx, code, oauth2.SetAuthURLParam("redirect_uri", redirectURL(r)))
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	if token.RefreshToken == "" {
		// Google didn't issue a refresh token (already granted this client before).
		// Redirect to the consent flow to force one.
		http.Redirect(w, r, "/auth/login?consent=1", http.StatusTemporaryRedirect)
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
	h.ClearSession(w)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (h *Handler) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
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

// TokenSource returns a cached oauth2.TokenSource for the given session's refresh token.
// Caching ensures concurrent requests share one source and reuse the cached access token
// rather than racing to exchange the same refresh token multiple times.
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

// Authenticated wraps a handler requiring a valid session.
// The session is injected into the request context.
func (h *Handler) Authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := h.GetSession(r)
		if err != nil {
			h.ClearSession(w)
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
