package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimit throttles the write/search/list endpoints per client IP. It guards
// POST /items, GET /items, and GET /search; all other routes pass through
// untouched. GET /items is the login-probe target, so guarding it (with the
// limiter ahead of bearer auth) throttles token brute-force attempts. Each
// client gets a token-bucket limiter (rps refill, burst ceiling).
func rateLimit(rps rate.Limit, burst int) func(http.Handler) http.Handler {
	var (
		mu      sync.Mutex
		clients = make(map[string]*ipLimiter)
		lookups int
	)

	getLimiter := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()

		lookups++
		if lookups%100 == 0 {
			cutoff := time.Now().Add(-10 * time.Minute)
			for k, v := range clients {
				if v.lastSeen.Before(cutoff) {
					delete(clients, k)
				}
			}
		}

		c, ok := clients[ip]
		if !ok {
			c = &ipLimiter{limiter: rate.NewLimiter(rps, burst)}
			clients[ip] = c
		}
		c.lastSeen = time.Now()
		return c.limiter
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			guarded := (r.Method == http.MethodPost && r.URL.Path == "/items") ||
				(r.Method == http.MethodGet && r.URL.Path == "/items") ||
				(r.Method == http.MethodGet && r.URL.Path == "/search") ||
					(r.Method == http.MethodGet && r.URL.Path == "/export")
			if !guarded {
				next.ServeHTTP(w, r)
				return
			}
			if !getLimiter(clientIP(r)).Allow() {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// clientIP returns the first hop of X-Forwarded-For when present, else the host
// portion of RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
