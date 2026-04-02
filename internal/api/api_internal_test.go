package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"foodtracker/internal/auth"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(auth.NewHandler(auth.Config{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost/callback",
		CookieSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}), "")
}

func TestWriteAPIErrSessionExpiredClearsCookie(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()

	h.writeAPIErr(w, fmt.Errorf("token refresh failed: %w", &oauth2.RetrieveError{
		Response:         &http.Response{StatusCode: http.StatusBadRequest},
		ErrorCode:        "invalid_grant",
		ErrorDescription: "refresh token revoked",
	}))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "session_expired" {
		t.Fatalf("error body: got %q, want %q", body["error"], "session_expired")
	}
	if got := w.Header().Get("Set-Cookie"); !strings.Contains(got, "ft_session=") {
		t.Fatalf("Set-Cookie: got %q, want cleared ft_session cookie", got)
	}
}

func TestWriteAPIErrInsufficientScopes(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()

	h.writeAPIErr(w, &googleapi.Error{
		Code: http.StatusForbidden,
		Errors: []googleapi.ErrorItem{{
			Reason:  "insufficientPermissions",
			Message: "Request had insufficient authentication scopes.",
		}},
	})

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusForbidden)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "insufficient_scopes" {
		t.Fatalf("error body: got %q, want %q", body["error"], "insufficient_scopes")
	}
}

func TestWriteAPIErrInsufficientScopesFromDetails(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()

	h.writeAPIErr(w, &googleapi.Error{
		Code:    http.StatusForbidden,
		Message: "Permission denied",
		Details: []interface{}{
			map[string]any{
				"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
				"reason": "ACCESS_TOKEN_SCOPE_INSUFFICIENT",
			},
		},
	})

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusForbidden)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "insufficient_scopes" {
		t.Fatalf("error body: got %q, want %q", body["error"], "insufficient_scopes")
	}
}

func TestWriteAPIErrOtherGoogle403StaysInternal(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()

	h.writeAPIErr(w, &googleapi.Error{
		Code:    http.StatusForbidden,
		Message: "Rate Limit Exceeded",
		Errors: []googleapi.ErrorItem{{
			Reason:  "rateLimitExceeded",
			Message: "Rate Limit Exceeded",
		}},
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] == "insufficient_scopes" {
		t.Fatalf("error body: unexpectedly rewrote non-scope 403 to insufficient_scopes")
	}
}
