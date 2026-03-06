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
