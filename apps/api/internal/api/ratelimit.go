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
	return perIPRateLimit(rps, burst, guarded)
}

// claimRateLimit throttles POST /device-links/claim with a small, strict
// per-IP bucket (5/min, burst 5) distinct from the general limiter: the
// device code is the only credential on that route, so it's the one place a
// stranger can cheaply guess at a secret, and it deserves a tighter ceiling
// than the rest of the API.
func claimRateLimit() func(http.Handler) http.Handler {
	return perIPRateLimit(rate.Limit(5.0/60.0), 5, func(method, path string) bool {
		return method == http.MethodPost && path == "/device-links/claim"
	})
}

// perIPRateLimit throttles requests matching match(method, path) with a
// per-client-IP token-bucket limiter (rps refill, burst ceiling). Limiters are
// created lazily per IP and evicted after 10 minutes of inactivity.
func perIPRateLimit(rps rate.Limit, burst int, match func(method, path string) bool) func(http.Handler) http.Handler {
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
			if !match(r.Method, r.URL.Path) {
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

// guarded reports whether a request to (method, path) is subject to the rate
// limiter. It covers the write/search/list endpoints plus POST /assets uploads.
// GET /assets/<id> reads are deliberately NOT guarded: image loads are proxied
// server-side from the single web-container IP with no X-Forwarded-For, so a
// per-IP burst limiter would break any view with more images than the burst
// ceiling. Serving is already bearer-gated, user-scoped, and UUID-keyed.
func guarded(method, path string) bool {
	return (method == http.MethodPost && path == "/items") ||
		(method == http.MethodGet && path == "/items") ||
		(method == http.MethodPatch && strings.HasPrefix(path, "/items/")) ||
		(method == http.MethodGet && path == "/search") ||
		(method == http.MethodGet && path == "/desk") ||
		(method == http.MethodGet && path == "/drift") ||
		(method == http.MethodPost && strings.HasPrefix(path, "/drift/")) ||
		(method == http.MethodGet && path == "/export") ||
		(method == http.MethodPost && path == "/assets") ||
		(method == http.MethodPost && path == "/feeds") ||
		// Kindle sends enqueue an SMTP delivery per call — unthrottled they'd be
		// an email-amplification vector, so both are guarded like other writes.
		(method == http.MethodPost && strings.HasPrefix(path, "/items/") && strings.HasSuffix(path, "/kindle")) ||
		(method == http.MethodPost && strings.HasPrefix(path, "/lenses/") && strings.HasSuffix(path, "/kindle")) ||
		// The MCP endpoint (any method: POST for JSON-RPC, GET for the SSE
		// stream) is guarded so that, like the REST API, failed token guesses
		// are throttled before bearer auth and authed tool calls have a ceiling.
		strings.HasPrefix(path, "/mcp") ||
		// API keys and device links mint/revoke credentials; guard them like
		// other writes. /device-links/claim also sits behind its own, much
		// stricter claimRateLimit bucket since the code is the credential.
		strings.HasPrefix(path, "/api-keys") ||
		strings.HasPrefix(path, "/device-links")
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
