package main

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"golang.org/x/time/rate"

	"foodtracker/internal/api"
	"foodtracker/internal/auth"
	"foodtracker/internal/ratelimit"
)

const (
	defaultBodyLimit int64 = 1 << 20  // 1 MB
	chatBodyLimit    int64 = 20 << 20 // 20 MB
)

func NewRouter(cfg Config, authHandler *auth.Handler, apiHandler *api.Handler, frontendFS fs.FS) *echo.Echo {
	e := echo.New()
	e.Logger = slog.Default()

	// --- Global middleware (outermost first) ---
	e.Use(middleware.Recover())
	e.Use(requestLogger())
	blCfg := middleware.BodyLimitConfig{
		LimitBytes: defaultBodyLimit,
		Skipper: func(c *echo.Context) bool {
			return c.Request().URL.Path == "/api/agent"
		},
	}
	blMw, _ := blCfg.ToMiddleware()
	e.Use(blMw)

	// Security headers
	secureMw, _ := middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: false,
		HSTSPreloadEnabled:    false,
		ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob: data:; connect-src 'self'; form-action 'self' https://accounts.google.com; frame-ancestors 'none'; base-uri 'self'",
		ReferrerPolicy:        "same-origin",
		Skipper: func(c *echo.Context) bool {
			return !cfg.CookieSecure // skip HSTS in dev
		},
	}.ToMiddleware()
	e.Use(secureMw)

	// CSRF via Sec-Fetch-Site (same approach as the old CrossOriginProtection).
	// In dev, Vite serves from :5173 and proxies API to :8080 — skip CSRF locally.
	if cfg.CookieSecure {
		csrfMw, _ := middleware.CSRFConfig{
			Skipper: func(c *echo.Context) bool {
				// Only protect state-changing methods.
				switch c.Request().Method {
				case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
					return true
				}
				return false
			},
		}.ToMiddleware()
		e.Use(csrfMw)
	}

	// --- Rate limiters ---
	// Per-IP for unauthenticated /auth/* routes.
	ipRL := ratelimit.New(rate.Limit(1), 10, 10*time.Minute)
	ipLimit := ipRL.Middleware(ratelimit.IPKey)

	// Per-user for authenticated /api/* routes.
	userRL := ratelimit.New(rate.Limit(5), 20, 10*time.Minute)
	userLimit := userRL.Middleware(ratelimit.UserKey)

	// --- Health check ---
	e.GET("/api/healthz", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok\n")
	})

	// --- Auth routes (per-IP rate limited) ---
	authGroup := e.Group("/auth", ipLimit)
	authGroup.GET("/login", authHandler.Login)
	authGroup.GET("/callback", authHandler.Callback)
	authGroup.GET("/logout", authHandler.Logout)
	authGroup.GET("/check", func(c *echo.Context) error {
		if _, err := authHandler.GetSession(c.Request()); err != nil {
			return c.NoContent(http.StatusUnauthorized)
		}
		return c.NoContent(http.StatusOK)
	})

	// --- API routes (auth required + per-user rate limited) ---
	apiGroup := e.Group("/api", authHandler.AuthMiddleware(), apiHandler.EnsureSpreadsheetMiddleware(), userLimit)

	apiGroup.GET("/log", apiHandler.GetLog)
	apiGroup.GET("/events", apiHandler.GetEvents)
	apiGroup.POST("/events", apiHandler.PostEvent)
	apiGroup.PATCH("/events/:id", apiHandler.PatchEvent)
	apiGroup.DELETE("/events/:id", apiHandler.DeleteEvent)
	apiGroup.POST("/chat/confirm", apiHandler.ConfirmChat)
	apiGroup.POST("/agent", apiHandler.Agent, middleware.BodyLimit(chatBodyLimit))
	apiGroup.POST("/coach/chat", apiHandler.CoachChat)
	apiGroup.GET("/insights", apiHandler.GetStoredInsights)
	apiGroup.POST("/insights", apiHandler.Insights)
	apiGroup.GET("/insights/day", apiHandler.GetStoredDayInsights)
	apiGroup.POST("/insights/day", apiHandler.DayInsights)
	apiGroup.GET("/insights/snapshots", apiHandler.GetInsightSnapshots)
	apiGroup.GET("/insights/by-trigger", apiHandler.GetInsightByTrigger)
	apiGroup.GET("/suggestions/day", apiHandler.GetStoredDaySuggestions)
	apiGroup.POST("/suggestions/day", apiHandler.DaySuggestions)
	apiGroup.GET("/suggestions/meal", apiHandler.GetStoredMealSuggestion)
	apiGroup.POST("/suggestions/meal", apiHandler.MealSuggestion)
	apiGroup.GET("/suggestions/week", apiHandler.GetStoredWeekSuggestions)
	apiGroup.POST("/suggestions/week", apiHandler.WeekSuggestions)
	apiGroup.PATCH("/entries/:id", apiHandler.PatchEntry)
	apiGroup.DELETE("/entries/:id", apiHandler.DeleteEntry)
	apiGroup.GET("/profile", apiHandler.GetProfile)
	apiGroup.PUT("/profile", apiHandler.PutProfile)
	apiGroup.GET("/system-prompt", apiHandler.GetSystemPrompt)
	apiGroup.GET("/favorites", apiHandler.GetFavorites)
	apiGroup.POST("/favorites", apiHandler.AddFavorite)
	apiGroup.DELETE("/favorites/:id", apiHandler.DeleteFavorite)

	// --- Serve Svelte SPA ---
	// Serve static files first; fall back to index.html for client-side routes.
	fileServer := http.FileServerFS(frontendFS)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			p := c.Request().URL.Path
			// Let API and auth routes pass through.
			if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/auth/") {
				return next(c)
			}
			// Try serving a real file (JS, CSS, favicon, etc.).
			if f, err := frontendFS.Open(strings.TrimPrefix(p, "/")); err == nil {
				f.Close()
				fileServer.ServeHTTP(c.Response(), c.Request())
				return nil
			}
			// SPA fallback: serve index.html for client-side routes.
			c.Request().URL.Path = "/"
			fileServer.ServeHTTP(c.Response(), c.Request())
			return nil
		}
	})

	return e
}

// requestLogger logs method, path, status, and latency — skipping healthz.
func requestLogger() echo.MiddlewareFunc {
	cfg := middleware.RequestLoggerConfig{
		LogMethod:  true,
		LogURIPath: true,
		LogStatus:  true,
		LogLatency: true,
		Skipper: func(c *echo.Context) bool {
			return c.Request().URL.Path == "/api/healthz"
		},
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			slog.Info("request",
				"method", v.Method,
				"path", v.URIPath,
				"status", v.Status,
				"latency", v.Latency,
			)
			return nil
		},
	}
	mw, _ := cfg.ToMiddleware()
	return mw
}
