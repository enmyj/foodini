package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/kelseyhightower/envconfig"

	"foodtracker/internal/api"
	"foodtracker/internal/auth"
)

//go:embed frontend/dist
var frontendDist embed.FS

type Config struct {
	GoogleClientID     string `envconfig:"GOOGLE_CLIENT_ID" required:"true"`
	GoogleClientSecret string `envconfig:"GOOGLE_CLIENT_SECRET" required:"true"`
	CookieSecret       string `envconfig:"COOKIE_SECRET" required:"true"`
	CookieSecure       bool   `envconfig:"COOKIE_SECURE" default:"false"`
	Port               string `envconfig:"PORT" default:"8080"`
	GeminiAPIKey       string `envconfig:"GEMINI_API_KEY" required:"true"`
}

func main() {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	// Singletons
	authHandler := auth.NewHandler(auth.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		CookieSecret: cfg.CookieSecret,
		Secure:       cfg.CookieSecure,
	})
	apiHandler := api.NewHandler(authHandler, cfg.GeminiAPIKey)

	distFS, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	e := NewRouter(cfg, authHandler, apiHandler, distFS)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           e,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
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

	log.Printf("Listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-shutdownDone
	log.Printf("bye")
}
