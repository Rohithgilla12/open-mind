package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimit429(t *testing.T) {
	s, rc, _ := testDeps(t)
	h := newSrv(t, s, rc, "")
	srv := httptest.NewServer(h)
	defer srv.Close()
	var last int
	for i := 0; i < 12; i++ {
		resp, err := http.Get(srv.URL + "/search?q=x")
		if err != nil {
			t.Fatal(err)
		}
		last = resp.StatusCode
		resp.Body.Close()
	}
	if last != 429 {
		t.Errorf("12th rapid request = %d, want 429", last)
	}
}

// TestWrongTokenGuessesThrottled proves that rapid wrong-token probes against
// GET /items are throttled to 429 (limiter runs before bearer auth), rather
// than yielding unlimited 401 guesses.
func TestWrongTokenGuessesThrottled(t *testing.T) {
	s, rc, _ := testDeps(t)
	h := newSrv(t, s, rc, "sekret")
	srv := httptest.NewServer(h)
	defer srv.Close()
	var last int
	for i := 0; i < 12; i++ {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/items", nil)
		req.Header.Set("Authorization", "Bearer wrong-guess")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		last = resp.StatusCode
		resp.Body.Close()
	}
	if last != 429 {
		t.Errorf("12th rapid wrong-token request = %d, want 429 (throttled)", last)
	}
}
