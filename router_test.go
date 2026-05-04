package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"foodtracker/internal/api"
	"foodtracker/internal/auth"
)

func testRouter() http.Handler {
	authHandler := auth.NewHandler(auth.Config{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		CookieSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	apiHandler := api.NewHandler(authHandler, "")
	return NewRouter(Config{}, authHandler, apiHandler, fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><div id=\"app\"></div>")},
	})
}

func TestDynamicRoutesAreNotCacheable(t *testing.T) {
	for _, path := range []string{"/api/healthz", "/auth/check"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			testRouter().ServeHTTP(rec, req)

			if got := rec.Header().Get("Cache-Control"); got != "no-store, max-age=0" {
				t.Fatalf("Cache-Control: got %q, want no-store, max-age=0", got)
			}
			if got := rec.Header().Get("Pragma"); got != "no-cache" {
				t.Fatalf("Pragma: got %q, want no-cache", got)
			}
			if got := rec.Header().Get("Expires"); got != "0" {
				t.Fatalf("Expires: got %q, want 0", got)
			}
		})
	}
}
