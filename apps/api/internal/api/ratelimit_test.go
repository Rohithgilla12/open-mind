package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/api"
)

func TestRateLimit429(t *testing.T) {
	s, rc, _ := testDeps(t)
	h := api.NewServer(s, rc, ai.NewNoop(), "")
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
