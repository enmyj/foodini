package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"foodtracker/internal/auth"
	"foodtracker/internal/sheets"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(auth.NewHandler(auth.Config{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
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

func TestFormatProfileContextIncludesAge(t *testing.T) {
	got := formatProfileContext(sheets.UserProfile{
		BirthYear:           "1990",
		Gender:              "female",
		Height:              "165cm",
		Weight:              "60kg",
		Goals:               "build muscle",
		DietaryRestrictions: "vegetarian",
	}, 2024)

	want := "User profile: age 34, female, 165cm, 60kg. Goals: build muscle. Dietary restrictions: vegetarian"
	if got != want {
		t.Fatalf("formatProfileContext: got %q, want %q", got, want)
	}
}

func TestParseChatRequestJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{
		"message":"banana and yogurt",
		"date":"2026-04-06",
		"meal":"breakfast",
		"images":[{"mime_type":"image/jpeg","data":"`+base64.StdEncoding.EncodeToString([]byte("jpeg-bytes"))+`"}]
	}`))
	req.Header.Set("Content-Type", "application/json")

	got, err := parseChatRequest(req)
	if err != nil {
		t.Fatalf("parseChatRequest: %v", err)
	}
	if got.Message != "banana and yogurt" || got.Date != "2026-04-06" || got.Meal != "breakfast" {
		t.Fatalf("unexpected request: %+v", got)
	}
	if len(got.Images) != 1 {
		t.Fatalf("images len: got %d, want 1", len(got.Images))
	}
	if got.Images[0].MIMEType != "image/jpeg" {
		t.Fatalf("mime: got %q, want image/jpeg", got.Images[0].MIMEType)
	}
	if string(got.Images[0].Data) != "jpeg-bytes" {
		t.Fatalf("image data: got %q", string(got.Images[0].Data))
	}
}

func TestParseChatRequestMultipart(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("message", "salad"); err != nil {
		t.Fatalf("WriteField message: %v", err)
	}
	if err := writer.WriteField("date", "2026-04-06"); err != nil {
		t.Fatalf("WriteField date: %v", err)
	}
	if err := writer.WriteField("meal", "lunch"); err != nil {
		t.Fatalf("WriteField meal: %v", err)
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="images"; filename="meal.jpg"`)
	header.Set("Content-Type", "image/jpeg")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte("fake-jpeg")); err != nil {
		t.Fatalf("part.Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/chat", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	got, err := parseChatRequest(req)
	if err != nil {
		t.Fatalf("parseChatRequest: %v", err)
	}
	if got.Message != "salad" || got.Date != "2026-04-06" || got.Meal != "lunch" {
		t.Fatalf("unexpected request: %+v", got)
	}
	if len(got.Images) != 1 {
		t.Fatalf("images len: got %d, want 1", len(got.Images))
	}
	if string(got.Images[0].Data) != "fake-jpeg" {
		t.Fatalf("image data: got %q", string(got.Images[0].Data))
	}
	if got.Images[0].MIMEType == "" {
		t.Fatal("expected MIME type to be populated")
	}
}

func TestParseChatRequestMultipartRejectsNonImage(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="images"; filename="notes.txt"`)
	header.Set("Content-Type", "text/plain")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte("plain text")); err != nil {
		t.Fatalf("part.Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/chat", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if _, err := parseChatRequest(req); err == nil {
		t.Fatal("expected non-image multipart upload to fail")
	}
}
