package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/unrolled/secure"
	"golang.org/x/time/rate"

	"foodtracker/internal/api"
	"foodtracker/internal/auth"
	"foodtracker/internal/ratelimit"
)

//go:embed frontend/dist
var frontendDist embed.FS

const (
	// Most API routes only exchange small JSON payloads.
	maxRequestBody = 1 << 20
	// Chat accepts direct image uploads. Keep this generous enough for a
	// normal phone photo while still bounding server memory and Gemini's
	// inline upload path.
	maxChatRequestBody = 20 << 20
)

func main() {
	cfg := auth.Config{
		ClientID:     requireEnv("GOOGLE_CLIENT_ID"),
		ClientSecret: requireEnv("GOOGLE_CLIENT_SECRET"),
		CookieSecret: requireEnv("COOKIE_SECRET"),
		Secure:       os.Getenv("COOKIE_SECURE") == "true",
	}
	isLocal := !cfg.Secure

	authHandler := auth.NewHandler(cfg)
	apiHandler := api.NewHandler(authHandler, requireEnv("GEMINI_API_KEY"))

	// Per-user rate limiter for authenticated API routes:
	// 5 req/s steady, burst 20, evict idle users after 10m.
	userRL := ratelimit.New(rate.Limit(5), 20, 10*time.Minute)
	userLimit := userRL.Middleware(ratelimit.UserKey)
	protect := func(h http.HandlerFunc) http.HandlerFunc {
		return apiHandler.Authenticated(userLimit(h))
	}

	// Per-IP rate limiter for the unauthenticated /auth/* routes:
	// 1 req/s steady, burst 10 — slows down signup abuse (each new user
	// costs us a Drive spreadsheet creation).
	authRL := ratelimit.New(rate.Limit(1), 10, 10*time.Minute)
	ipLimit := authRL.Middleware(ratelimit.IPKey)

	mux := http.NewServeMux()

	// Health check (no rate limit; used by orchestrators).
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "ok")
	})

	// Auth routes (per-IP rate limited).
	mux.HandleFunc("GET /auth/login", ipLimit(authHandler.Login))
	mux.HandleFunc("GET /auth/callback", ipLimit(authHandler.Callback))
	mux.HandleFunc("GET /auth/logout", ipLimit(authHandler.Logout))

	// API routes (all require auth + per-user rate limit).
	mux.HandleFunc("GET /api/log", protect(apiHandler.GetLog))
	mux.HandleFunc("GET /api/activity", protect(apiHandler.GetActivity))
	mux.HandleFunc("PUT /api/activity", protect(apiHandler.PutActivity))
	mux.HandleFunc("POST /api/chat", protect(apiHandler.Chat))
	mux.HandleFunc("POST /api/chat/confirm", protect(apiHandler.ConfirmChat))
	mux.HandleFunc("GET /api/insights", protect(apiHandler.GetStoredInsights))
	mux.HandleFunc("POST /api/insights", protect(apiHandler.Insights))
	mux.HandleFunc("GET /api/insights/day", protect(apiHandler.GetStoredDayInsights))
	mux.HandleFunc("POST /api/insights/day", protect(apiHandler.DayInsights))
	mux.HandleFunc("GET /api/suggestions/day", protect(apiHandler.GetStoredDaySuggestions))
	mux.HandleFunc("POST /api/suggestions/day", protect(apiHandler.DaySuggestions))
	mux.HandleFunc("GET /api/suggestions/week", protect(apiHandler.GetStoredWeekSuggestions))
	mux.HandleFunc("POST /api/suggestions/week", protect(apiHandler.WeekSuggestions))
	mux.HandleFunc("PATCH /api/entries/{id}", protect(apiHandler.PatchEntry))
	mux.HandleFunc("DELETE /api/entries/{id}", protect(apiHandler.DeleteEntry))
	mux.HandleFunc("GET /api/profile", protect(apiHandler.GetProfile))
	mux.HandleFunc("PUT /api/profile", protect(apiHandler.PutProfile))
	mux.HandleFunc("GET /api/favorites", protect(apiHandler.GetFavorites))
	mux.HandleFunc("POST /api/favorites", protect(apiHandler.AddFavorite))
	mux.HandleFunc("DELETE /api/favorites/{id}", protect(apiHandler.DeleteFavorite))

	// Serve Svelte SPA.
	distFS, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(distFS)))

	// --- Middleware stack (outermost first) ---
	//
	//   recoverPanics       — last line of defense; catches anything below
	//   logRequests         — logs status/latency
	//   maxBytes            — caps request body size
	//   secureMw            — HSTS, X-Content-Type-Options, frame-deny, CSP, ...
	//   csrf                — Go 1.25 Sec-Fetch-Site / Origin CSRF protection
	//   mux                 — routes
	var handler http.Handler = mux

	// CSRF protection via Go 1.25 CrossOriginProtection. It only blocks
	// unsafe methods (POST/PUT/PATCH/DELETE) coming from a cross-origin
	// browser context. In dev, Vite serves the frontend from :5173 while
	// the API lives on :8080, so we trust that origin explicitly.
	// Frontend and API share an origin in prod (Go binary embeds the SPA),
	// so same-origin requests pass COP automatically. In local dev Vite
	// serves the frontend from :5173 and hits the API on :8080 — rather
	// than maintaining a trusted-origin list, just skip COP locally.
	if !isLocal {
		handler = http.NewCrossOriginProtection().Handler(handler)
	}

	// Security headers (HSTS, nosniff, frame-deny, referrer, CSP).
	secureMw := secure.New(secure.Options{
		BrowserXssFilter:      true,
		ContentTypeNosniff:    true,
		FrameDeny:             true,
		ReferrerPolicy:        "same-origin",
		ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob: data:; connect-src 'self'; form-action 'self' https://accounts.google.com; frame-ancestors 'none'; base-uri 'self'",
		STSSeconds:            31536000, // 1 year
		STSIncludeSubdomains:  true,
		STSPreload:            false,
		// In dev (no HTTPS) skip HSTS + don't force SSL redirects.
		IsDevelopment: isLocal,
	})
	handler = secureMw.Handler(handler)

	handler = maxBytes(maxRequestBody, handler)
	handler = logRequests(handler)
	handler = recoverPanics(handler)

	port := getEnv("PORT", "8080")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout must exceed the slowest handler. Gemini calls can be
		// slow, so give them headroom.
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	shutdownDone := make(chan struct{})
	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
		<-sigc
		log.Printf("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
		close(shutdownDone)
	}()

	log.Printf("Listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-shutdownDone
	log.Printf("bye")
}

// recoverPanics is the outermost middleware: it logs and returns 500 instead
// of letting a panic take down the connection (and leak a stack trace).
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// Avoid leaking the panic value to the client.
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// maxBytes caps request body size to n bytes. Reads past the limit return an
// error and close the connection.
func maxBytes(n int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := n
		if r.Method == http.MethodPost && r.URL.Path == "/api/chat" {
			limit = maxChatRequestBody
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
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
