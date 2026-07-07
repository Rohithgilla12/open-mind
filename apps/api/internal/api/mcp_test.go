package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMCPMountedAndGuarded asserts /mcp is reachable through NewServer's chi
// router and inherits the same Bearer-auth middleware as the REST routes.
// initialize is a POST of JSON-RPC and doesn't touch the Backend, so a
// Server with nil store/river is safe to build here.
func TestMCPMountedAndGuarded(t *testing.T) {
	h := NewServer(nil, nil, nil, AuthConfig{Mode: AuthModeToken, LegacyToken: "secret"}, nil, 0, nil, false)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer secret")
	req2.Header.Set("Accept", "application/json, text/event-stream")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("with token: want 200 initialize, got %d (body: %s)", rec2.Code, rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), `"serverInfo"`) {
		t.Fatalf("initialize response missing serverInfo: %s", rec2.Body.String())
	}
}
