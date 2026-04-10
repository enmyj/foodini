// Package ratelimit provides a simple HTTP rate limiter built on
// golang.org/x/time/rate (the canonical Go token-bucket limiter).
//
// A single Limiter holds one token bucket per key. A key-extractor function
// lets callers partition by user, IP, etc. The limiter evicts idle keys
// periodically so the map stays bounded.
package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"golang.org/x/time/rate"

	"foodtracker/internal/auth"
)

// KeyFunc extracts a rate-limit key from an echo context. Returning "" bypasses
// limiting for that request.
type KeyFunc func(c *echo.Context) string

// Limiter holds a token bucket per key.
type Limiter struct {
	limit rate.Limit
	burst int

	mu      sync.Mutex
	buckets map[string]*entry
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// New creates a Limiter allowing `limit` events/sec steady-state with
// bursts up to `burst`. A background janitor evicts keys idle beyond idleTTL.
func New(limit rate.Limit, burst int, idleTTL time.Duration) *Limiter {
	l := &Limiter{
		limit:   limit,
		burst:   burst,
		buckets: make(map[string]*entry),
	}
	go l.janitor(idleTTL)
	return l
}

func (l *Limiter) janitor(idleTTL time.Duration) {
	t := time.NewTicker(idleTTL)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-idleTTL)
		l.mu.Lock()
		for k, e := range l.buckets {
			if e.lastSeen.Before(cutoff) {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}

func (l *Limiter) get(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.buckets[key]
	if !ok {
		e = &entry{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.buckets[key] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// Middleware returns an Echo middleware that enforces the limit keyed by keyFn.
func (l *Limiter) Middleware(keyFn KeyFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			key := keyFn(c)
			if key == "" {
				return next(c)
			}
			if !l.get(key).Allow() {
				c.Response().Header().Set("Retry-After", "1")
				return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			}
			return next(c)
		}
	}
}

// UserKey keys rate-limit buckets by the authenticated user's email.
// Must be used after auth middleware so the session is in context.
func UserKey(c *echo.Context) string {
	s := auth.SessionFrom(c)
	if s == nil || s.UserEmail == "" {
		return ""
	}
	return "u:" + s.UserEmail
}

// IPKey keys rate-limit buckets by the client IP.
//
// Deployment assumption: this app runs on Cloud Run behind Cloudflare.
//   - Cloudflare sets CF-Connecting-IP to the real client.
//   - Cloud Run sets X-Forwarded-For; the leftmost entry is the client.
//   - r.RemoteAddr on Cloud Run is a Google front-end address.
func IPKey(c *echo.Context) string {
	r := c.Request()
	if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
		return "ip:" + cf
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return "ip:" + strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return "ip:" + host
}
