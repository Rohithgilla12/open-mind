package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestOpenAI(t *testing.T, srv *httptest.Server, model, embedModel string) *OpenAI {
	t.Helper()
	return NewOpenAI(srv.URL, "test-key", model, embedModel, WithHTTPClient(srv.Client()))
}

func TestOpenAI_Name(t *testing.T) {
	p := NewOpenAI("http://x", "k", "m", "")
	if p.Name() != "openai" {
		t.Fatalf("Name() = %q, want openai", p.Name())
	}
}

func TestOpenAI_Summarise(t *testing.T) {
	var gotPath, gotAuth, gotModel string
	var gotTemp float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Model       string  `json:"model"`
			Temperature float64 `json:"temperature"`
			Messages    []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		gotTemp = body.Temperature
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"a short summary."}}]}`)
	}))
	defer srv.Close()

	p := newTestOpenAI(t, srv, "gpt-test", "")
	out, err := p.Summarise(context.Background(), "Title", "Body")
	if err != nil {
		t.Fatal(err)
	}
	if out != "a short summary." {
		t.Fatalf("summary = %q", out)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotModel != "gpt-test" {
		t.Fatalf("model = %q", gotModel)
	}
	if gotTemp != 0.3 {
		t.Fatalf("temperature = %v, want 0.3", gotTemp)
	}
}

func TestOpenAI_Tag_JSONMode(t *testing.T) {
	var gotFormatType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotFormatType = body.ResponseFormat.Type
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"tags\":[\"Go\",\"HTTP\",\"Testing\"]}"}}]}`)
	}))
	defer srv.Close()

	p := newTestOpenAI(t, srv, "gpt-test", "")
	tags, err := p.Tag(context.Background(), "Title", "Body")
	if err != nil {
		t.Fatal(err)
	}
	if gotFormatType != "json_object" {
		t.Fatalf("response_format.type = %q, want json_object", gotFormatType)
	}
	want := []string{"go", "http", "testing"}
	if len(tags) != len(want) {
		t.Fatalf("tags = %v", tags)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Fatalf("tags = %v, want %v (lowercased)", tags, want)
		}
	}
}

func TestOpenAI_Embed_DimensionsAndLength(t *testing.T) {
	var gotDims int
	var gotModel string
	makeVec := func(n int) []float32 {
		v := make([]float32, n)
		for i := range v {
			v[i] = float32(i)
		}
		return v
	}
	newSrv := func(n int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Model      string `json:"model"`
				Dimensions int    `json:"dimensions"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotDims = body.Dimensions
			gotModel = body.Model
			resp := map[string]any{"data": []map[string]any{{"embedding": makeVec(n)}}}
			_ = json.NewEncoder(w).Encode(resp)
		}))
	}

	t.Run("correct length", func(t *testing.T) {
		srv := newSrv(EmbedDims)
		defer srv.Close()
		p := newTestOpenAI(t, srv, "gpt-test", "embed-test")
		vec, err := p.Embed(context.Background(), "hello")
		if err != nil {
			t.Fatal(err)
		}
		if len(vec) != EmbedDims {
			t.Fatalf("len(vec) = %d", len(vec))
		}
		if gotDims != EmbedDims {
			t.Fatalf("dimensions field = %d, want %d", gotDims, EmbedDims)
		}
		if gotModel != "embed-test" {
			t.Fatalf("embed model = %q", gotModel)
		}
	})

	t.Run("wrong length errors", func(t *testing.T) {
		srv := newSrv(512)
		defer srv.Close()
		p := newTestOpenAI(t, srv, "gpt-test", "embed-test")
		if _, err := p.Embed(context.Background(), "hello"); err == nil {
			t.Fatal("expected error on wrong-length embedding")
		}
	})
}

func TestOpenAI_Embed_NotSupported(t *testing.T) {
	p := NewOpenAI("http://x", "k", "m", "")
	if _, err := p.Embed(context.Background(), "x"); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported", err)
	}
}

func TestOpenAI_ParseQuery(t *testing.T) {
	var gotReq chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		// Unknown card type and noisy domain must be sanitised.
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"text\":\"shoes\",\"color\":\"\",\"types\":[\"tweet\",\"poster\"],\"domains\":[\"https://www.x.com/a\",\"x.com\",\"not a host\"]}"}}]}`)
	}))
	defer srv.Close()

	p := newTestOpenAI(t, srv, "m", "")
	out, err := p.ParseQuery(context.Background(), "posts from x.com about shoes")
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "shoes" || out.Color != "" {
		t.Fatalf("ParseQuery = %+v, want text=shoes color=", out)
	}
	if len(out.Types) != 1 || out.Types[0] != "tweet" {
		t.Fatalf("Types = %v, want [tweet] (unknown types dropped)", out.Types)
	}
	if len(out.Domains) != 1 || out.Domains[0] != "x.com" {
		t.Fatalf("Domains = %v, want [x.com] (normalised + deduped)", out.Domains)
	}
	if gotReq.ResponseFormat == nil || gotReq.ResponseFormat.Type != "json_object" {
		t.Fatalf("request response_format = %+v, want json_object", gotReq.ResponseFormat)
	}
}

func TestSanitiseDomains(t *testing.T) {
	got := sanitiseDomains([]string{"x.com", "https://www.x.com/a", "twitter.com", "", "not a host", "  "})
	want := []string{"x.com", "twitter.com"}
	if len(got) != len(want) {
		t.Fatalf("sanitiseDomains = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sanitiseDomains[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if sanitiseDomains(nil) != nil {
		t.Errorf("sanitiseDomains(nil) = %v, want nil", sanitiseDomains(nil))
	}
}

func TestOpenAI_ErrorClassification(t *testing.T) {
	tests := []struct {
		status    int
		retryable bool
	}{
		{429, true},
		{500, true},
		{503, true},
		{401, false},
		{400, false},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"error":{"message":"nope"}}`)
			}))
			defer srv.Close()
			p := newTestOpenAI(t, srv, "gpt-test", "")
			_, err := p.Summarise(context.Background(), "t", "b")
			if err == nil {
				t.Fatal("expected error")
			}
			var re *RetryableError
			isRetryable := errors.As(err, &re)
			if Retryable(err) != tt.retryable {
				t.Fatalf("Retryable(%v) = %v, want %v", err, Retryable(err), tt.retryable)
			}
			if !isRetryable {
				t.Fatalf("expected error to carry *RetryableError, got %v", err)
			}
			if re.Status != tt.status {
				t.Fatalf("status = %d, want %d", re.Status, tt.status)
			}
			if !strings.Contains(err.Error(), "nope") {
				t.Logf("error body not surfaced (ok): %v", err)
			}
		})
	}
}
