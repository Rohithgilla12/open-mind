package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// RetryableError wraps a provider failure with its HTTP status code so the
// fallback chain can decide whether to fail over to the next provider. A nil
// Err is permitted (Status alone carries the classification).
type RetryableError struct {
	Status int
	Err    error
}

// Error implements the error interface.
func (e *RetryableError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("ai: http %d: %v", e.Status, e.Err)
	}
	return fmt.Sprintf("ai: http %d", e.Status)
}

// Unwrap exposes the underlying error for errors.Is/As.
func (e *RetryableError) Unwrap() error { return e.Err }

// Retryable reports whether err represents a transient failure that should
// trigger fail-over to the next provider in the chain: a RetryableError with
// status 429 or >=500, a deadline-exceeded, or a network timeout. Context
// cancellation is not retryable.
func Retryable(err error) bool {
	if err == nil {
		return false
	}

	var re *RetryableError
	if errors.As(err, &re) {
		return re.Status == 429 || re.Status >= 500
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}

	return false
}
