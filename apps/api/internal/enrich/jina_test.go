package enrich_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/enrich"
)

func TestJinaImplementsExtractor(t *testing.T) {
	var _ enrich.Extractor = enrich.NewJina(nil, "")
}

func TestJinaExtract(t *testing.T) {
	var gotAuth, gotAccept, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"title":   "Commonplace Books",
				"content": "Readers kept commonplace books full of marginalia.",
				"images": map[string]string{
					"Fig 1": "https://example.com/lead.jpg",
				},
			},
		})
	}))
	defer srv.Close()

	ex := enrich.NewJina(srv.Client(), "secret-key", enrich.WithJinaBaseURL(srv.URL))
	if ex.Name() != "jina" {
		t.Errorf("name = %q", ex.Name())
	}
	got, err := ex.Extract(context.Background(), "https://example.com/essay")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Errorf("accept header = %q", gotAccept)
	}
	if !strings.Contains(gotPath, "https://example.com/essay") {
		t.Errorf("path = %q, want target url appended", gotPath)
	}
	if got.Title != "Commonplace Books" {
		t.Errorf("title = %q", got.Title)
	}
	if !strings.Contains(got.Body, "marginalia") {
		t.Errorf("body = %q", got.Body)
	}
	if got.LeadImageURL != "https://example.com/lead.jpg" {
		t.Errorf("lead image = %q", got.LeadImageURL)
	}
}

func TestJinaNoKeyOmitsAuth(t *testing.T) {
	hadAuth := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			hadAuth = true
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"title": "T", "content": "C"},
		})
	}))
	defer srv.Close()

	ex := enrich.NewJina(srv.Client(), "", enrich.WithJinaBaseURL(srv.URL))
	if _, err := ex.Extract(context.Background(), "https://example.com/x"); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if hadAuth {
		t.Errorf("authorization header should be absent without key")
	}
}
