package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"foodtracker/internal/api"
	"foodtracker/internal/auth"
)

//go:embed frontend/dist
var frontendDist embed.FS

func main() {
	cfg := auth.Config{
		ClientID:     requireEnv("GOOGLE_CLIENT_ID"),
		ClientSecret: requireEnv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  requireEnv("REDIRECT_URL"),
		CookieSecret: requireEnv("COOKIE_SECRET"),
		Secure:       os.Getenv("COOKIE_SECURE") == "true",
	}

	authHandler := auth.NewHandler(cfg)
	apiHandler := api.NewHandler(authHandler, requireEnv("GEMINI_API_KEY"))

	mux := http.NewServeMux()

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
	mux.HandleFunc("PATCH /api/entries/{id}", apiHandler.Authenticated(apiHandler.PatchEntry))

	// Serve Svelte SPA
	distFS, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(distFS)))

	port := getEnv("PORT", "8080")
	log.Printf("Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
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
