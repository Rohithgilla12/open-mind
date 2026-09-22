// Package jev is a hand-rolled client for TypeSafe's Jev model
// (POST /v1/systemone). There is no official Go SDK.
//
// The client is optional. With no API key, Evaluate returns ErrDisabled and
// makes no request. Skip is true for ErrDisabled and for any API or transport
// failure, so a caller can proceed without decisions in one branch.
//
// This package does not run on the save path. Capture callers that later opt
// in should put CaptureTimeout on the context and treat a timeout like any
// other skip: the save must already have returned.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultModel is the pinned Jev version. Aliases such as jev-latest move
	// when TypeSafe ships a new release; the response model field is the
	// version that actually answered, and Result.Model records that.
	DefaultModel = "jev-1.13.0"

	// DefaultBaseURL is the TypeSafe API origin.
	DefaultBaseURL = "https://api.typesafe.ai"

	// APIKeyEnv is the environment variable that supplies the API key.
	// Empty leaves the client disabled.
	APIKeyEnv = "OPENMIND_TYPESAFE_API_KEY"

	// CaptureTimeout is the deadline a future capture caller should set.
	// Evaluate does not apply it itself; rerank and drift have other budgets.
	CaptureTimeout = 2 * time.Second

	systemOnePath = "/v1/systemone"

	// statusOverloaded is TypeSafe's "temporarily overloaded" status.
	// It is not in the net/http status constants.
	statusOverloaded = 529

	defaultMaxRetries   = 2
	defaultBackoff      = 500 * time.Millisecond
	defaultBackoffMax   = 5 * time.Second
	defaultJitter       = 0.25
	defaultMaxRetryWait = 60 * time.Second

	maxResponseBody = 1 << 20

	requestIDHeader = "x-typesafe-request-id"
)

// ErrDisabled means no API key is configured. Evaluate returns it before any
// request. Callers treat it the same as an API failure: see Skip.
var ErrDisabled = errors.New("jev disabled: no API key")

// APIError is a failed call to the TypeSafe API. Status is 0 when the failure
// happened before an HTTP response (dial, timeout, cancelled request).
// RequestID is the x-typesafe-request-id header when the server sent one.
type APIError struct {
	Status    int
	Body      string
	RequestID string
	Err       error
}

// Error implements error.
func (e *APIError) Error() string {
	if e.Err != nil && e.Status == 0 {
		return e.Err.Error()
	}
	msg := clipBytes(strings.TrimSpace(e.Body), 256)
	if e.RequestID != "" && msg != "" {
		return fmt.Sprintf("http %d (request %s): %s", e.Status, e.RequestID, msg)
	}
	if e.RequestID != "" {
		return fmt.Sprintf("http %d (request %s)", e.Status, e.RequestID)
	}
	if msg != "" {
		return fmt.Sprintf("http %d: %s", e.Status, msg)
	}
	return fmt.Sprintf("http %d", e.Status)
}

// Unwrap exposes the transport error, when there is one.
func (e *APIError) Unwrap() error { return e.Err }

// Skip reports whether the caller should proceed without decisions.
// It is true for ErrDisabled, for any API or transport failure, and for a
// cancelled or deadline-exceeded context. It is false for nil and for caller
// mistakes such as evaluating an empty question set.
func Skip(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrDisabled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var api *APIError
	return errors.As(err, &api)
}

// Question is one typed judgment. Type is "noul", "choice", or "score".
// Instructions is a string, or an object whose fields the question names in
// backticks. Criteria is a choice map, a score level list, or noul yes/no
// text; it is omitted when empty.
type Question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

const (
	// TypeNoul is a yes/no probability.
	TypeNoul = "noul"
	// TypeChoice picks one option from a closed set.
	TypeChoice = "choice"
	// TypeScore places the state on an ordered rubric.
	TypeScore = "score"
)

// noulCriteria is the wire shape for a Noul's optional yes/no descriptions.
type noulCriteria struct {
	True  string `json:"true"`
	False string `json:"false"`
}

// Noul builds a yes/no question. yes and no describe the two poles; pass
// empty strings to omit criteria.
func Noul(instructions string, yes, no string) Question {
	q := Question{Type: TypeNoul, Instructions: instructions}
	if yes != "" || no != "" {
		q.Criteria = noulCriteria{True: yes, False: no}
	}
	return q
}

// Choice builds a closed-set question. options maps each option id to a
// description. The API allows at most 255 options.
func Choice(instructions string, options map[string]string) Question {
	return Question{Type: TypeChoice, Instructions: instructions, Criteria: options}
}

// Score builds an ordered-rubric question. levels is lowest to highest.
// The API accepts 2 to 10 levels.
func Score(instructions string, levels []string) Question {
	return Question{Type: TypeScore, Instructions: instructions, Criteria: levels}
}

// Answer is one question's result. Which fields are set depends on Type.
// Noul has no confidence; a nil Confidence on a choice or score means the
// field was absent.
type Answer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
}

// Result is one evaluation. Model is the version the response reports, which
// can differ from the id sent in the request when the request used an alias.
type Result struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Usage is token accounting for one call. Output tokens are unpriced.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type evaluateRequest struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

// Client calls POST /v1/systemone. A zero API key disables it.
type Client struct {
	apiKey         string
	baseURL        string
	model          string
	http           *http.Client
	maxRetries     int
	backoffInitial time.Duration
	backoffMax     time.Duration
	jitter         float64
	maxRetryAfter  time.Duration
	sleep          func(ctx context.Context, d time.Duration) error
	randFloat      func() float64
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API origin. Tests point this at httptest.
func WithBaseURL(base string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(base, "/") }
}

// WithModel overrides the pinned model id. An empty id keeps DefaultModel.
func WithModel(id string) Option {
	return func(c *Client) { c.model = id }
}

// WithHTTPClient sets the HTTP client. The request context is the deadline;
// the client itself should not add a shorter timeout that ignores it.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// New builds a client. An empty or whitespace apiKey leaves it disabled.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:         strings.TrimSpace(apiKey),
		baseURL:        DefaultBaseURL,
		model:          DefaultModel,
		http:           &http.Client{},
		maxRetries:     defaultMaxRetries,
		backoffInitial: defaultBackoff,
		backoffMax:     defaultBackoffMax,
		jitter:         defaultJitter,
		maxRetryAfter:  defaultMaxRetryWait,
		sleep:          sleepContext,
		randFloat:      rand.Float64,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.model = strings.TrimSpace(c.model)
	if c.model == "" {
		c.model = DefaultModel
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	c.baseURL = strings.TrimRight(c.baseURL, "/")
	if c.http == nil {
		c.http = &http.Client{}
	}
	if c.sleep == nil {
		c.sleep = sleepContext
	}
	if c.randFloat == nil {
		c.randFloat = rand.Float64
	}
	if c.maxRetries < 0 {
		c.maxRetries = 0
	}
	return c
}

// FromEnv builds a client from APIKeyEnv. A missing key returns a disabled
// client, not an error.
func FromEnv() *Client {
	return New(os.Getenv(APIKeyEnv))
}

// Evaluate posts state and questions and returns the parsed answers.
// Model on the result is the version the response reports.
func (c *Client) Evaluate(ctx context.Context, state any, questions map[string]Question) (*Result, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, ErrDisabled
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("evaluating questions: %w", err)
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("evaluating questions: no questions")
	}

	body, err := encodeRequest(evaluateRequest{
		State:     state,
		Model:     c.model,
		Questions: questions,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	var (
		last           error
		serverDelay    time.Duration
		useServerDelay bool
	)
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("evaluating questions: %w", err)
		}
		if attempt > 0 {
			delay := c.backoff(attempt - 1)
			if useServerDelay {
				delay = serverDelay
			}
			if err := c.sleep(ctx, delay); err != nil {
				return nil, fmt.Errorf("waiting to retry: %w", err)
			}
		}
		result, delay, honor, err := c.post(ctx, body)
		if err == nil {
			return result, nil
		}
		last = err
		if !retryable(err) || attempt == c.maxRetries {
			return nil, err
		}
		serverDelay, useServerDelay = delay, honor
	}
	return nil, last
}

func retryable(err error) bool {
	var api *APIError
	if !errors.As(err, &api) {
		return false
	}
	return api.Status == http.StatusTooManyRequests || api.Status == statusOverloaded
}

func (c *Client) post(ctx context.Context, body []byte) (result *Result, retryAfter time.Duration, honor bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+systemOnePath, bytes.NewReader(body))
	if err != nil {
		return nil, 0, false, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, false, fmt.Errorf("evaluating questions: %w", &APIError{Err: err})
	}
	defer resp.Body.Close()

	reqID := resp.Header.Get(requestIDHeader)
	raw, err := readLimited(resp.Body)
	if err != nil {
		return nil, 0, false, fmt.Errorf("reading response: %w", &APIError{Status: resp.StatusCode, RequestID: reqID, Err: err})
	}
	if resp.StatusCode != http.StatusOK {
		apiErr := &APIError{
			Status:    resp.StatusCode,
			Body:      strings.TrimSpace(string(raw)),
			RequestID: reqID,
		}
		delay, ok := retryDelayFrom(resp.Header, c.maxRetryAfter)
		return nil, delay, ok, fmt.Errorf("evaluating questions: %w", apiErr)
	}

	var out Result
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, 0, false, fmt.Errorf("decoding response: %w", &APIError{
			Status:    resp.StatusCode,
			Body:      strings.TrimSpace(string(raw)),
			RequestID: reqID,
			Err:       err,
		})
	}
	if out.Answers == nil {
		out.Answers = map[string]Answer{}
	}
	return &out, 0, false, nil
}

func encodeRequest(payload evaluateRequest) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func readLimited(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxResponseBody+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBody {
		return nil, errors.New("response too large")
	}
	return raw, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// backoff is the delay before retry number retry (0 is the first retry).
// The base delay doubles from backoffInitial up to backoffMax, then a
// fraction of it — up to jitter — is subtracted.
func (c *Client) backoff(retry int) time.Duration {
	d := c.backoffInitial
	if d < 0 {
		d = 0
	}
	if c.backoffMax > 0 {
		for i := 0; i < retry; i++ {
			if d >= c.backoffMax/2 {
				d = c.backoffMax
				break
			}
			d *= 2
		}
		if d > c.backoffMax {
			d = c.backoffMax
		}
	}
	j := c.jitter
	if j < 0 {
		j = 0
	}
	if j > 1 {
		j = 1
	}
	if j > 0 && d > 0 {
		f := 0.0
		if c.randFloat != nil {
			f = c.randFloat()
		}
		if f < 0 {
			f = 0
		}
		if f > 1 {
			f = 1
		}
		d = time.Duration(float64(d) * (1 - f*j))
	}
	if d < 0 {
		return 0
	}
	return d
}

func retryDelayFrom(h http.Header, maxWait time.Duration) (time.Duration, bool) {
	d, ok := parseRetryAfter(h)
	if !ok {
		return 0, false
	}
	if maxWait > 0 && d > maxWait {
		return 0, false
	}
	return d, true
}

func parseRetryAfter(h http.Header) (time.Duration, bool) {
	if ms := strings.TrimSpace(h.Get("retry-after-ms")); ms != "" {
		n, err := strconv.Atoi(ms)
		if err == nil && n >= 0 {
			return time.Duration(n) * time.Millisecond, true
		}
	}
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Second, true
	}
	when, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	d := time.Until(when)
	if d < 0 {
		return 0, true
	}
	return d, true
}

func clipBytes(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}
