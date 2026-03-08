package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"foodtracker/internal/api"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	api.WriteJSON(w, http.StatusOK, map[string]string{"hello": "world"})
	if w.Code != 200 {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q", ct)
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["hello"] != "world" {
		t.Errorf("body: %v", got)
	}
}

func TestWriteJSON_Status(t *testing.T) {
	w := httptest.NewRecorder()
	api.WriteJSON(w, http.StatusCreated, map[string]string{"ok": "yes"})
	if w.Code != 201 {
		t.Errorf("status: got %d, want 201", w.Code)
	}
}

func TestLocalNow_WithTimezone(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Timezone", "America/New_York")
	got := api.LocalNow(req)
	if got.Location().String() != "America/New_York" {
		t.Errorf("location: got %q, want America/New_York", got.Location())
	}
}

func TestLocalNow_InvalidTimezone(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Timezone", "Not/AReal/Zone")
	got := api.LocalNow(req)
	if got.IsZero() {
		t.Error("expected non-zero time for invalid timezone")
	}
}

func TestLocalNow_NoHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := api.LocalNow(req)
	if got.IsZero() {
		t.Error("expected non-zero time with no header")
	}
}
