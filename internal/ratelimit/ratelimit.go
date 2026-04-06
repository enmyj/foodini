// Package ratelimit provides a simple HTTP rate limiter built on
// golang.org/x/time/rate (the canonical Go token-bucket limiter).
//
// A single Limiter holds one token bucket per key. A key-extractor function
// lets callers partition by user, IP, API token, etc. The limiter evicts
// idle keys periodically so the map stays bounded.
package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"foodtracker/internal/auth"
)

// KeyFunc extracts a rate-limit key from a request. Returning "" bypasses
// limiting for that request (useful when a session is unavailable and
// auth will reject the request anyway).
type KeyFunc func(*http.Request) string

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

// Middleware returns an http middleware that enforces the limit keyed by keyFn.
func (l *Limiter) Middleware(keyFn KeyFunc) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			key := keyFn(r)
			if key == "" {
				next(w, r)
				return
			}
			if !l.get(key).Allow() {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next(w, r)
		}
	}
}

// UserKey keys rate-limit buckets by the authenticated user's email.
// Must be used inside auth.Authenticated so the session is in context.
func UserKey(r *http.Request) string {
	s := auth.SessionFromContext(r.Context())
	if s == nil || s.UserEmail == "" {
		return ""
	}
	return "u:" + s.UserEmail
}

// IPKey keys rate-limit buckets by the client IP.
//
// Deployment assumption: this app runs on Cloud Run behind Cloudflare.
//   - Cloudflare (when proxied) sets CF-Connecting-IP to the real client.
//   - Cloud Run's front end sets X-Forwarded-For; the leftmost entry is
//     the client that hit the front end (which, when proxied, is
//     Cloudflare — so we prefer CF-Connecting-IP when present).
//   - r.RemoteAddr on Cloud Run is a Google front-end address and is
//     useless as a limiter key (everyone would share it).
//
// Caveat: Cloud Run services are also reachable directly at their
// *.run.app URL, bypassing Cloudflare. A determined attacker could hit
// that URL and set CF-Connecting-IP to anything. To fully close that
// gap, lock Cloud Run ingress to Cloudflare's IP ranges (or put Cloud
// Run behind an internal LB).
func IPKey(r *http.Request) string {
	if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
		return "ip:" + cf
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Leftmost entry = original client.
		first, _, _ := strings.Cut(xff, ",")
		return "ip:" + strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return "ip:" + host
}
