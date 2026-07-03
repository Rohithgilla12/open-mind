package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuth(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, "sekret"))
	defer srv.Close()
	tests := []struct {
		name, path, header string
		want               int
	}{
		{"no token", "/items", "", 401},
		{"wrong token", "/items", "Bearer nope", 401},
		{"right token", "/items", "Bearer sekret", 200},
		{"healthz exempt", "/healthz", "", 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, srv.URL+tt.path, nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Errorf("%s = %d, want %d", tt.path, resp.StatusCode, tt.want)
			}
		})
	}
}

func TestAuthDisabledWhenTokenEmpty(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/items")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /items with empty token = %d, want 200", resp.StatusCode)
	}
}
