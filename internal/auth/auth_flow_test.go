package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func testHandler() *Handler {
	return NewHandler(Config{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		CookieSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
}

func TestLoginRequestsOfflineConsent(t *testing.T) {
	h := testHandler()

	req := httptest.NewRequest(http.MethodGet, "http://example.com/auth/login", nil)
	rec := httptest.NewRecorder()
	c := echo.NewContext(req, rec)

	if err := h.Login(c); err != nil {
		t.Fatalf("Login: %v", err)
	}

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}

	loc := rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse redirect URL: %v", err)
	}
	q := u.Query()

	if got := q.Get("prompt"); got != "consent" {
		t.Fatalf("prompt: got %q, want %q", got, "consent")
	}
	if got := q.Get("access_type"); got != "offline" {
		t.Fatalf("access_type: got %q, want %q", got, "offline")
	}
	if got := q.Get("redirect_uri"); got != "http://example.com/auth/callback" {
		t.Fatalf("redirect_uri: got %q", got)
	}

	foundStateCookie := false
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "oauth_state" && cookie.Value != "" {
			foundStateCookie = true
			break
		}
	}
	if !foundStateCookie {
		t.Fatal("expected oauth_state cookie")
	}
}

func TestCallbackWithoutRefreshTokenReturnsError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-token","token_type":"Bearer"}`))
	}))
	defer tokenServer.Close()

	h := testHandler()
	h.oauthCfg.Endpoint.TokenURL = tokenServer.URL

	req := httptest.NewRequest(http.MethodGet, "http://example.com/auth/callback?state=test-state&code=test-code", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	rec := httptest.NewRecorder()
	c := echo.NewContext(req, rec)

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != missingRefreshMsg {
		t.Fatalf("body: got %q, want %q", got, missingRefreshMsg)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("unexpected redirect to %q", loc)
	}
}

func TestSetSessionUsesLongLivedCookie(t *testing.T) {
	h := testHandler()

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rec := httptest.NewRecorder()
	c := echo.NewContext(req, rec)

	if err := h.SetSession(c, &Session{UserEmail: "test@example.com", RefreshToken: "refresh-token"}); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == cookieName {
			if cookie.MaxAge != sessionCookieAge {
				t.Fatalf("MaxAge: got %d, want %d", cookie.MaxAge, sessionCookieAge)
			}
			return
		}
	}

	t.Fatal("expected session cookie")
}
