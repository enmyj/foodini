package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
	_ "time/tzdata"

	"foodtracker/internal/api"
	"foodtracker/internal/auth"
)

//go:embed frontend/dist
var frontendDist embed.FS

func main() {
	cfg := auth.Config{
		ClientID:     requireEnv("GOOGLE_CLIENT_ID"),
		ClientSecret: requireEnv("GOOGLE_CLIENT_SECRET"),
		CookieSecret: requireEnv("COOKIE_SECRET"),
		Secure:       os.Getenv("COOKIE_SECURE") == "true",
	}

	authHandler := auth.NewHandler(cfg)
	apiHandler := api.NewHandler(authHandler, requireEnv("GEMINI_API_KEY"))

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "ok")
	})

	// Auth routes
	mux.HandleFunc("GET /auth/login", authHandler.Login)
	mux.HandleFunc("GET /auth/callback", authHandler.Callback)
	mux.HandleFunc("GET /auth/logout", authHandler.Logout)

	// API routes (all require auth)
	mux.HandleFunc("GET /api/log", apiHandler.Authenticated(apiHandler.GetLog))
	mux.HandleFunc("GET /api/activity", apiHandler.Authenticated(apiHandler.GetActivity))
	mux.HandleFunc("PUT /api/activity", apiHandler.Authenticated(apiHandler.PutActivity))
	mux.HandleFunc("POST /api/chat", apiHandler.Authenticated(apiHandler.Chat))
	mux.HandleFunc("POST /api/chat/confirm", apiHandler.Authenticated(apiHandler.ConfirmChat))
	mux.HandleFunc("GET /api/insights", apiHandler.Authenticated(apiHandler.GetStoredInsights))
	mux.HandleFunc("POST /api/insights", apiHandler.Authenticated(apiHandler.Insights))
	mux.HandleFunc("GET /api/insights/day", apiHandler.Authenticated(apiHandler.GetStoredDayInsights))
	mux.HandleFunc("POST /api/insights/day", apiHandler.Authenticated(apiHandler.DayInsights))
	mux.HandleFunc("GET /api/suggestions/day", apiHandler.Authenticated(apiHandler.GetStoredDaySuggestions))
	mux.HandleFunc("POST /api/suggestions/day", apiHandler.Authenticated(apiHandler.DaySuggestions))
	mux.HandleFunc("GET /api/suggestions/week", apiHandler.Authenticated(apiHandler.GetStoredWeekSuggestions))
	mux.HandleFunc("POST /api/suggestions/week", apiHandler.Authenticated(apiHandler.WeekSuggestions))
	mux.HandleFunc("PATCH /api/entries/{id}", apiHandler.Authenticated(apiHandler.PatchEntry))
	mux.HandleFunc("DELETE /api/entries/{id}", apiHandler.Authenticated(apiHandler.DeleteEntry))
	mux.HandleFunc("GET /api/profile", apiHandler.Authenticated(apiHandler.GetProfile))
	mux.HandleFunc("PUT /api/profile", apiHandler.Authenticated(apiHandler.PutProfile))

	// Serve Svelte SPA
	distFS, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(distFS)))

	port := getEnv("PORT", "8080")
	log.Printf("Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, logRequests(mux)))
}

// logRequests logs method, path, status code, and latency for every request.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		if r.URL.Path != "/api/healthz" {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start))
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s not set", key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
