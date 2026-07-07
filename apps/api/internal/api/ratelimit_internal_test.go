package api

import (
	"net/http"
	"testing"
)

// TestGuardedPredicate proves the rate-limit predicate covers write/search/list
// and POST /assets, but deliberately excludes GET /assets/<id> reads: image
// loads are proxied server-side from a single web-container IP with no
// X-Forwarded-For, so guarding them would break any view with more images than
// the burst ceiling. Serving is already bearer-gated, user-scoped, and
// UUID-keyed, so throttling reads adds little.
func TestGuardedPredicate(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{http.MethodPost, "/items", true},
		{http.MethodGet, "/items", true},
		{http.MethodGet, "/search", true},
		{http.MethodGet, "/export", true},
		{http.MethodPost, "/assets", true},
		{http.MethodGet, "/assets/3f1a2b4c-0000-0000-0000-000000000000", false},
		{http.MethodGet, "/assets/", false},
		{http.MethodGet, "/healthz", false},
		{http.MethodPost, "/mcp", true},
		{http.MethodGet, "/mcp", true},
		{http.MethodPost, "/mcp/", true},
		{http.MethodPost, "/items/3f1a2b4c-0000-0000-0000-000000000000/kindle", true},
		{http.MethodPost, "/lenses/3f1a2b4c-0000-0000-0000-000000000000/kindle", true},
		{http.MethodGet, "/api-keys", true},
		{http.MethodPost, "/api-keys", true},
		{http.MethodDelete, "/api-keys/3f1a2b4c-0000-0000-0000-000000000000", true},
		{http.MethodPost, "/device-links", true},
		{http.MethodPost, "/device-links/claim", true},
	}
	for _, c := range cases {
		if got := guarded(c.method, c.path); got != c.want {
			t.Errorf("guarded(%q, %q) = %v, want %v", c.method, c.path, got, c.want)
		}
	}
}
