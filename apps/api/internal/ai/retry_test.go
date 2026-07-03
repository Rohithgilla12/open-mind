package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestRetryableError_ErrorAndUnwrap(t *testing.T) {
	inner := errors.New("boom")
	re := &RetryableError{Status: 429, Err: inner}
	if !errors.Is(re, inner) {
		t.Fatalf("expected Unwrap to expose inner error")
	}
	if re.Error() == "" {
		t.Fatalf("expected non-empty Error() string")
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429", &RetryableError{Status: 429}, true},
		{"500", &RetryableError{Status: 500}, true},
		{"503", &RetryableError{Status: 503}, true},
		{"401", &RetryableError{Status: 401}, false},
		{"400", &RetryableError{Status: 400}, false},
		{"wrapped 429", fmt.Errorf("outer: %w", &RetryableError{Status: 429}), true},
		{"wrapped 401", fmt.Errorf("outer: %w", &RetryableError{Status: 401}), false},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"wrapped deadline", fmt.Errorf("x: %w", context.DeadlineExceeded), true},
		{"net timeout", timeoutErr{}, true},
		{"canceled", context.Canceled, false},
		{"plain error", errors.New("nope"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Retryable(tt.err); got != tt.want {
				t.Fatalf("Retryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// ensure net.Error timeouts (e.g. from a real deadline) are classified.
func TestRetryable_NetError(t *testing.T) {
	var ne net.Error = timeoutErr{}
	if !Retryable(ne) {
		t.Fatalf("expected net.Error timeout to be retryable")
	}
}
