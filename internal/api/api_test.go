package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"foodtracker/internal/api"
)

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
