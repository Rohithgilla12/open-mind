package enrich_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/enrich"
)

func TestReadabilityImplementsExtractor(t *testing.T) {
	var _ enrich.Extractor = enrich.NewReadability(nil)
}

func TestReadabilityExtract(t *testing.T) {
	html, err := os.ReadFile("testdata/article.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(html)
	}))
	defer srv.Close()

	ex := enrich.NewReadability(srv.Client())
	if ex.Name() != "readability" {
		t.Errorf("name = %q", ex.Name())
	}
	got, err := ex.Extract(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(got.Title, "Commonplace Books") {
		t.Errorf("title = %q", got.Title)
	}
	if !strings.Contains(got.Body, "marginalia") {
		t.Errorf("body missing article text")
	}
	if strings.Contains(got.Body, "Subscribe to our newsletter") {
		t.Errorf("body contains boilerplate")
	}
}
