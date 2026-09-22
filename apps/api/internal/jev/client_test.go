package jev

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEvaluate_CaptureRoundTrip(t *testing.T) {
	tags := []string{"go", "postgres", "search", "notes", "type-theory"}
	questions := CaptureQuestions(tags)
	state := BuildCaptureState(
		"https://go.dev/doc/effective_go",
		"Effective Go",
		"go.dev",
		"Go is a new language. Effective Go is a document. See <code>gofmt</code>.",
	)

	var gotBody evaluateRequest
	var gotMethod, gotPath, gotAuth, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request: %v", err)
			http.Error(w, "read", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("decoding request: %v", err)
			http.Error(w, "json", http.StatusBadRequest)
			return
		}
		if strings.Contains(string(raw), `\u003c`) {
			t.Errorf("request escaped HTML: %s", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "jev-1.13.0",
			"answers": {
				"content_type": {
					"type": "choice",
					"choice": "docs",
					"probabilities": {"article": 0.1, "docs": 0.8, "paper": 0.0, "tool": 0.05, "video": 0.0, "discussion": 0.0, "other": 0.05},
					"confidence": 0.72
				},
				"depth": {
					"type": "score",
					"score": 1.2,
					"legend": {"0": "glance", "1": "working", "2": "keeper"},
					"probabilities": {"0": 0.1, "1": 0.6, "2": 0.3},
					"confidence": 0.41
				},
				"is_evergreen": {"type": "noul", "noul": 0.93},
				"tag:go": {"type": "noul", "noul": 0.91},
				"tag:postgres": {"type": "noul", "noul": 0.62},
				"tag:search": {"type": "noul", "noul": 0.2},
				"tag:notes": {"type": "noul", "noul": 0.5},
				"tag:type-theory": {"type": "noul", "noul": 0.85}
			},
			"usage": {"input_tokens": 700, "output_tokens": 40}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	res, err := c.Evaluate(context.Background(), state, questions)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s", gotMethod)
	}
	if gotPath != "/v1/systemone" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type = %q", gotContentType)
	}
	if gotBody.Model != DefaultModel {
		t.Fatalf("request model = %q, want %q", gotBody.Model, DefaultModel)
	}
	gotState, ok := gotBody.State.(map[string]any)
	if !ok {
		t.Fatalf("state type %T", gotBody.State)
	}
	for _, key := range []string{"url", "title", "site", "excerpt"} {
		if _, exists := gotState[key]; !exists {
			t.Errorf("state missing %s", key)
		}
	}
	if gotState["excerpt"] != state.Excerpt {
		t.Fatalf("excerpt = %#v", gotState["excerpt"])
	}
	for id, q := range questions {
		sent, exists := gotBody.Questions[id]
		if !exists {
			t.Errorf("request missing question %s", id)
			continue
		}
		if sent.Type != q.Type {
			t.Errorf("%s type = %s, want %s", id, sent.Type, q.Type)
		}
	}
	content, _ := gotBody.Questions[QContentType].Criteria.(map[string]any)
	if len(content) != 7 {
		t.Fatalf("content_type criteria = %#v", gotBody.Questions[QContentType].Criteria)
	}
	depthLevels, _ := gotBody.Questions[QDepth].Criteria.([]any)
	if len(depthLevels) != 3 {
		t.Fatalf("depth criteria = %#v", gotBody.Questions[QDepth].Criteria)
	}
	ever, _ := gotBody.Questions[QEvergreen].Criteria.(map[string]any)
	if ever["true"] == nil || ever["false"] == nil {
		t.Fatalf("evergreen criteria = %#v", ever)
	}
	tagIns, _ := gotBody.Questions["tag:go"].Instructions.(map[string]any)
	if tagIns["tag"] != "go" || tagIns["question"] == "" {
		t.Fatalf("tag instructions = %#v", gotBody.Questions["tag:go"].Instructions)
	}

	if res.Model != "jev-1.13.0" {
		t.Fatalf("response model = %q", res.Model)
	}
	if res.Usage.InputTokens != 700 || res.Usage.OutputTokens != 40 {
		t.Fatalf("usage = %+v", res.Usage)
	}

	ct := res.Answers[QContentType]
	if ct.Type != TypeChoice || ct.Choice != "docs" || ct.Confidence == nil || *ct.Confidence != 0.72 {
		t.Fatalf("content_type = %+v", ct)
	}
	if !AcceptContentType(*ct.Confidence) {
		t.Fatal("expected content type to clear the provisional threshold")
	}
	if ct.Probabilities["docs"] != 0.8 {
		t.Fatalf("docs probability = %v", ct.Probabilities["docs"])
	}

	depth := res.Answers[QDepth]
	if depth.Type != TypeScore || depth.Score == nil || *depth.Score != 1.2 {
		t.Fatalf("depth = %+v", depth)
	}
	if depth.Legend["1"] != "working" {
		t.Fatalf("legend = %+v", depth.Legend)
	}
	if depth.Probabilities["2"] != 0.3 {
		t.Fatalf("depth probabilities = %+v", depth.Probabilities)
	}

	evergreen := res.Answers[QEvergreen]
	if evergreen.Type != TypeNoul || evergreen.Noul == nil || *evergreen.Noul != 0.93 {
		t.Fatalf("is_evergreen = %+v", evergreen)
	}
	if evergreen.Confidence != nil {
		t.Fatal("noul answers have no confidence")
	}

	wantTags := map[string]TagDecision{
		"tag:go":          ActionApplied,
		"tag:postgres":    ActionSuggested,
		"tag:search":      ActionSkipped,
		"tag:notes":       ActionSuggested,
		"tag:type-theory": ActionSuggested,
	}
	for id, want := range wantTags {
		a := res.Answers[id]
		if a.Noul == nil {
			t.Fatalf("%s missing noul", id)
		}
		if got := DecideTag(*a.Noul); got != want {
			t.Errorf("DecideTag(%s=%v) = %s, want %s", id, *a.Noul, got, want)
		}
	}
}

func TestEvaluate_RecordsReportedModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body evaluateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		if body.Model != "jev-latest" {
			t.Errorf("sent model %q", body.Model)
		}
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.5}},"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, WithModel("jev-latest"))
	res, err := c.Evaluate(context.Background(), "state", map[string]Question{
		"q": Noul("Is this true?", "", ""),
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Model != "jev-1.13.0" {
		t.Fatalf("recorded model %q, want the version the response reported", res.Model)
	}
}

func TestEvaluate_Disabled(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tests := []struct {
		name string
		key  string
	}{
		{name: "empty", key: ""},
		{name: "whitespace", key: "  \t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.key, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
			_, err := c.Evaluate(context.Background(), "state", map[string]Question{
				"q": Noul("Is this true?", "", ""),
			})
			if !errors.Is(err, ErrDisabled) {
				t.Fatalf("err = %v, want ErrDisabled", err)
			}
			if !Skip(err) {
				t.Fatal("Skip(ErrDisabled) = false")
			}
		})
	}
	if hits != 0 {
		t.Fatalf("disabled client made %d requests", hits)
	}
}

func TestEvaluate_Retry429And529(t *testing.T) {
	statuses := []int{http.StatusTooManyRequests, statusOverloaded, http.StatusOK}
	var sleeps []time.Duration
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		code := statuses[calls]
		calls++
		if code == http.StatusOK {
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":1}},"usage":{"input_tokens":3,"output_tokens":1}}`))
			return
		}
		w.Header().Set(requestIDHeader, "req_test")
		http.Error(w, "slow down", code)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	c.backoffInitial = 500 * time.Millisecond
	c.backoffMax = 5 * time.Second
	c.jitter = 0.25
	c.randFloat = func() float64 { return 1 } // subtract the full jitter fraction
	c.sleep = func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}

	res, err := c.Evaluate(context.Background(), "state", map[string]Question{
		"q": Noul("Is this true?", "", ""),
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
	if res.Answers["q"].Noul == nil || *res.Answers["q"].Noul != 1 {
		t.Fatalf("answer = %+v", res.Answers["q"])
	}
	want := []time.Duration{
		time.Duration(float64(500*time.Millisecond) * 0.75),
		time.Duration(float64(1000*time.Millisecond) * 0.75),
	}
	if len(sleeps) != len(want) {
		t.Fatalf("sleeps = %v, want %v", sleeps, want)
	}
	for i := range want {
		if sleeps[i] != want[i] {
			t.Errorf("sleep %d = %s, want %s", i, sleeps[i], want[i])
		}
	}
}

func TestEvaluate_RetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		header string
		value  string
		want   time.Duration
	}{
		{name: "delta seconds", header: "Retry-After", value: "2", want: 2 * time.Second},
		{name: "milliseconds", header: "retry-after-ms", value: "1500", want: 1500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			var slept time.Duration
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					w.Header().Set(tt.header, tt.value)
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"error":"rate limit"}`))
					return
				}
				_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0}},"usage":{"input_tokens":1,"output_tokens":1}}`))
			}))
			defer srv.Close()

			c := newTestClient(t, srv)
			c.sleep = func(ctx context.Context, d time.Duration) error {
				slept = d
				return nil
			}
			if _, err := c.Evaluate(context.Background(), "s", oneQuestion()); err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if slept != tt.want {
				t.Fatalf("slept %s, want %s", slept, tt.want)
			}
		})
	}
}

func TestEvaluate_RetryAfterTooLongUsesBackoff(t *testing.T) {
	calls := 0
	var slept time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(statusOverloaded)
			return
		}
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.1}},"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	c.backoffInitial = 20 * time.Millisecond
	c.jitter = 0
	c.maxRetryAfter = 60 * time.Second
	c.sleep = func(ctx context.Context, d time.Duration) error {
		slept = d
		return nil
	}
	if _, err := c.Evaluate(context.Background(), "s", oneQuestion()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if slept != 20*time.Millisecond {
		t.Fatalf("slept %s, want backoff instead of 120s", slept)
	}
}

func TestEvaluate_NoRetry(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "401", status: http.StatusUnauthorized},
		{name: "422", status: http.StatusUnprocessableEntity},
		{name: "500", status: http.StatusInternalServerError},
		{name: "400", status: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set(requestIDHeader, "req_abc")
				http.Error(w, "nope", tt.status)
			}))
			defer srv.Close()

			c := newTestClient(t, srv)
			_, err := c.Evaluate(context.Background(), "s", oneQuestion())
			if err == nil {
				t.Fatal("expected error")
			}
			if calls != 1 {
				t.Fatalf("calls = %d, want 1", calls)
			}
			if !Skip(err) {
				t.Fatal("Skip(api error) = false")
			}
			if errors.Is(err, ErrDisabled) {
				t.Fatal("api error must not be ErrDisabled")
			}
			var api *APIError
			if !errors.As(err, &api) {
				t.Fatalf("err = %v, want APIError", err)
			}
			if api.Status != tt.status {
				t.Fatalf("status = %d", api.Status)
			}
			if api.RequestID != "req_abc" {
				t.Fatalf("request id = %q", api.RequestID)
			}
		})
	}
}

func TestEvaluate_RetriesExhausted(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("busy"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	c.maxRetries = 2
	_, err := c.Evaluate(context.Background(), "s", oneQuestion())
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
	if !Skip(err) {
		t.Fatal("exhausted retries should skip")
	}
	var api *APIError
	if !errors.As(err, &api) || api.Status != http.StatusTooManyRequests {
		t.Fatalf("err = %v", err)
	}
}

func TestEvaluate_Context(t *testing.T) {
	t.Run("already cancelled", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
		}))
		defer srv.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c := newTestClient(t, srv)
		_, err := c.Evaluate(ctx, "s", oneQuestion())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
		if !Skip(err) {
			t.Fatal("cancelled context should skip")
		}
		if calls != 0 {
			t.Fatalf("calls = %d", calls)
		}
	})

	t.Run("cancelled during backoff", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(statusOverloaded)
		}))
		defer srv.Close()
		ctx, cancel := context.WithCancel(context.Background())
		c := newTestClient(t, srv)
		c.sleep = func(ctx context.Context, d time.Duration) error {
			cancel()
			return ctx.Err()
		}
		_, err := c.Evaluate(ctx, "s", oneQuestion())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
		if !Skip(err) {
			t.Fatal("Skip = false")
		}
		if calls != 1 {
			t.Fatalf("calls = %d, want 1", calls)
		}
	})

	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()
		time.Sleep(time.Millisecond)
		c := New("k")
		_, err := c.Evaluate(ctx, "s", oneQuestion())
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v", err)
		}
		if !Skip(err) {
			t.Fatal("deadline should skip")
		}
	})
}

func TestEvaluate_EmptyQuestions(t *testing.T) {
	c := New("key")
	_, err := c.Evaluate(context.Background(), "s", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if Skip(err) {
		t.Fatal("empty questions is a caller mistake, not an API failure")
	}
	if errors.Is(err, ErrDisabled) {
		t.Fatal("did not expect ErrDisabled")
	}
}

func TestEvaluate_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.Evaluate(context.Background(), "s", oneQuestion())
	if err == nil {
		t.Fatal("expected error")
	}
	if !Skip(err) {
		t.Fatal("malformed success body should skip")
	}
}

func TestEvaluate_NullAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":null,"usage":{"input_tokens":1,"output_tokens":0}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	res, err := c.Evaluate(context.Background(), "s", oneQuestion())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Answers == nil {
		t.Fatal("nil answers should decode to an empty map")
	}
	if res.Model != "jev-1.13.0" {
		t.Fatalf("model = %q", res.Model)
	}
}

func TestSkip(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "disabled", err: ErrDisabled, want: true},
		{name: "wrapped disabled", err: errors.Join(ErrDisabled), want: true},
		{name: "api", err: &APIError{Status: 401, Body: "no"}, want: true},
		{name: "transport", err: &APIError{Err: errors.New("dial")}, want: true},
		{name: "deadline", err: context.DeadlineExceeded, want: true},
		{name: "canceled", err: context.Canceled, want: true},
		{name: "plain", err: errors.New("no questions"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Skip(tt.err); got != tt.want {
				t.Fatalf("Skip = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBackoff(t *testing.T) {
	c := New("k")
	c.backoffInitial = 500 * time.Millisecond
	c.backoffMax = 5 * time.Second
	c.jitter = 0.25
	c.randFloat = func() float64 { return 1 }
	tests := []struct {
		retry int
		want  time.Duration
	}{
		{retry: 0, want: time.Duration(float64(500*time.Millisecond) * 0.75)},
		{retry: 1, want: time.Duration(float64(time.Second) * 0.75)},
		{retry: 2, want: time.Duration(float64(2*time.Second) * 0.75)},
		{retry: 3, want: time.Duration(float64(4*time.Second) * 0.75)},
		{retry: 4, want: time.Duration(float64(5*time.Second) * 0.75)},
		{retry: 8, want: time.Duration(float64(5*time.Second) * 0.75)},
	}
	for _, tt := range tests {
		if got := c.backoff(tt.retry); got != tt.want {
			t.Errorf("backoff(%d) = %s, want %s", tt.retry, got, tt.want)
		}
	}

	c.jitter = 0
	c.randFloat = func() float64 { return 1 }
	if got := c.backoff(0); got != 500*time.Millisecond {
		t.Fatalf("zero jitter backoff = %s", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	soon := time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat)
	tests := []struct {
		name    string
		header  http.Header
		want    time.Duration
		wantOK  bool
		approx  bool
	}{
		{
			name:   "ms preferred over seconds",
			header: http.Header{"Retry-After-Ms": []string{"250"}, "Retry-After": []string{"9"}},
			want:   250 * time.Millisecond,
			wantOK: true,
		},
		{
			name:   "seconds",
			header: http.Header{"Retry-After": []string{"3"}},
			want:   3 * time.Second,
			wantOK: true,
		},
		{
			name:   "past http date",
			header: http.Header{"Retry-After": []string{past}},
			want:   0,
			wantOK: true,
		},
		{
			name:   "future http date",
			header: http.Header{"Retry-After": []string{soon}},
			want:   5 * time.Second,
			wantOK: true,
			approx: true,
		},
		{
			name:   "garbage",
			header: http.Header{"Retry-After": []string{"tomorrow"}},
			wantOK: false,
		},
		{
			name:   "absent",
			header: http.Header{},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRetryAfter(tt.header)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (delay %s)", ok, tt.wantOK, got)
			}
			if !ok {
				return
			}
			if tt.approx {
				if got < 3*time.Second || got > 6*time.Second {
					t.Fatalf("delay = %s, want about %s", got, tt.want)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("delay = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestSleepContext(t *testing.T) {
	if err := sleepContext(context.Background(), 0); err != nil {
		t.Fatalf("zero sleep: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	start := time.Now()
	if err := sleepContext(context.Background(), 5*time.Millisecond); err != nil {
		t.Fatalf("short sleep: %v", err)
	}
	if time.Since(start) < 5*time.Millisecond {
		t.Fatal("sleep returned early")
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv(APIKeyEnv, "")
	c := FromEnv()
	if _, err := c.Evaluate(context.Background(), "s", oneQuestion()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("unset key: %v", err)
	}

	t.Setenv(APIKeyEnv, "sek")
	c = FromEnv()
	if c.apiKey != "sek" {
		t.Fatalf("apiKey = %q", c.apiKey)
	}
	if c.model != DefaultModel {
		t.Fatalf("model = %q", c.model)
	}
}

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{name: "status", err: &APIError{Status: 529}, want: "http 529"},
		{name: "body", err: &APIError{Status: 422, Body: "missing state"}, want: "http 422: missing state"},
		{name: "request id", err: &APIError{Status: 429, RequestID: "req_1", Body: "slow"}, want: "http 429 (request req_1): slow"},
		{name: "transport", err: &APIError{Err: errors.New("dial tcp")}, want: "dial tcp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func newTestClient(t *testing.T, srv *httptest.Server, opts ...Option) *Client {
	t.Helper()
	all := make([]Option, 0, len(opts)+2)
	all = append(all, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	all = append(all, opts...)
	c := New("test-key", all...)
	c.sleep = func(ctx context.Context, d time.Duration) error { return nil }
	c.randFloat = func() float64 { return 0 }
	return c
}

func oneQuestion() map[string]Question {
	return map[string]Question{"q": Noul("Is this true?", "", "")}
}
