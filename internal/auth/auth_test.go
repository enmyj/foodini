package auth_test

import (
	"net/http/httptest"
	"testing"

	"foodtracker/internal/auth"
)

func TestSessionRoundTrip(t *testing.T) {
	cfg := auth.Config{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		CookieSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	h := auth.NewHandler(cfg)

	session := &auth.Session{
		UserEmail:     "test@example.com",
		RefreshToken:  "refresh-tok",
		SpreadsheetID: "sheet-123",
	}

	w := httptest.NewRecorder()
	if err := h.SetSession(w, session); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}

	got, err := h.GetSession(req)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.UserEmail != session.UserEmail {
		t.Errorf("UserEmail: got %q, want %q", got.UserEmail, session.UserEmail)
	}
	if got.RefreshToken != session.RefreshToken {
		t.Errorf("RefreshToken: got %q, want %q", got.RefreshToken, session.RefreshToken)
	}
	if got.SpreadsheetID != session.SpreadsheetID {
		t.Errorf("SpreadsheetID: got %q, want %q", got.SpreadsheetID, session.SpreadsheetID)
	}
}

func TestGetSession_NoCookie(t *testing.T) {
	cfg := auth.Config{CookieSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	h := auth.NewHandler(cfg)
	req := httptest.NewRequest("GET", "/", nil)
	_, err := h.GetSession(req)
	if err == nil {
		t.Error("expected error for missing cookie")
	}
}
