package ai

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/genai"
)

func TestClassifyGeminiErr_APIErrorRetryable(t *testing.T) {
	err := fmt.Errorf("gemini summarise: %w", classifyGeminiErr(genai.APIError{Code: 429, Message: "rate limit exceeded"}))
	if !Retryable(err) {
		t.Fatalf("expected 429 genai.APIError to be retryable, got %v", err)
	}
}

func TestClassifyGeminiErr_APIErrorNonRetryable(t *testing.T) {
	err := fmt.Errorf("gemini summarise: %w", classifyGeminiErr(genai.APIError{Code: 400, Message: "bad request"}))
	if Retryable(err) {
		t.Fatalf("expected 400 genai.APIError to be non-retryable, got %v", err)
	}
}

func TestClassifyGeminiErr_StringFallbackRetryable(t *testing.T) {
	err := fmt.Errorf("gemini summarise: %w", classifyGeminiErr(errors.New("googleapi: Error 429: rate limit exceeded")))
	if !Retryable(err) {
		t.Fatalf("expected string-fallback 429 error to be retryable, got %v", err)
	}
}

func TestClassifyGeminiErr_StringFallbackNonRetryable(t *testing.T) {
	err := fmt.Errorf("gemini summarise: %w", classifyGeminiErr(errors.New("invalid argument")))
	if Retryable(err) {
		t.Fatalf("expected unrelated error to be non-retryable, got %v", err)
	}
}
