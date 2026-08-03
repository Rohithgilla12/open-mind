package ai

import (
	"context"
	"testing"
)

func TestFromEnvDefaultsToNoop(t *testing.T) {
	// No AI_PROVIDER / AI_PROVIDERS set.
	t.Setenv("AI_PROVIDER", "")
	t.Setenv("AI_PROVIDERS", "")

	p, err := FromEnv(context.Background())
	if err != nil {
		t.Fatalf("FromEnv err = %v", err)
	}
	if p.Name() != "noop" {
		t.Fatalf("Name() = %q, want noop", p.Name())
	}
}

func TestFromEnvUnknownProviderSkipped(t *testing.T) {
	t.Setenv("AI_PROVIDERS", "bogus")

	p, err := FromEnv(context.Background())
	if err != nil {
		t.Fatalf("FromEnv err = %v", err)
	}
	// All names unknown → zero usable providers → noop floor.
	if p.Name() != "noop" {
		t.Fatalf("Name() = %q, want noop", p.Name())
	}
}

func TestFromEnvSingleOpenAIBareProvider(t *testing.T) {
	t.Setenv("AI_PROVIDERS", "openai")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-5-mini")

	p, err := FromEnv(context.Background())
	if err != nil {
		t.Fatalf("FromEnv err = %v", err)
	}
	// Single entry without RPM → bare provider, no chain wrapper.
	if p.Name() != "openai" {
		t.Fatalf("Name() = %q, want openai (bare)", p.Name())
	}
}

func TestFromEnvSingleOpenAIWithRPMWrapsChain(t *testing.T) {
	t.Setenv("AI_PROVIDERS", "openai")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-5-mini")
	t.Setenv("AI_RPM_OPENAI", "10")

	p, err := FromEnv(context.Background())
	if err != nil {
		t.Fatalf("FromEnv err = %v", err)
	}
	if p.Name() != "chain(openai)" {
		t.Fatalf("Name() = %q, want chain(openai) (RPM configured)", p.Name())
	}
}

func TestFromEnvChainOfOpenAIAndNoop(t *testing.T) {
	t.Setenv("AI_PROVIDERS", "openai,noop")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-5-mini")

	p, err := FromEnv(context.Background())
	if err != nil {
		t.Fatalf("FromEnv err = %v", err)
	}
	if p.Name() != "chain(openai,noop)" {
		t.Fatalf("Name() = %q, want chain(openai,noop)", p.Name())
	}
}

func TestFromEnvProvidersPrecedenceOverProvider(t *testing.T) {
	// AI_PROVIDERS wins over AI_PROVIDER.
	t.Setenv("AI_PROVIDER", "gemini")
	t.Setenv("AI_PROVIDERS", "noop")

	p, err := FromEnv(context.Background())
	if err != nil {
		t.Fatalf("FromEnv err = %v", err)
	}
	if p.Name() != "noop" {
		t.Fatalf("Name() = %q, want noop (AI_PROVIDERS precedence)", p.Name())
	}
}

func TestFromEnvOpenAIMissingConfigErrors(t *testing.T) {
	t.Setenv("AI_PROVIDERS", "openai")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")

	if _, err := FromEnv(context.Background()); err == nil {
		t.Fatal("FromEnv err = nil, want error for openai without key/model")
	}
}

func TestFromEnvGeminiMissingKeyErrors(t *testing.T) {
	t.Setenv("AI_PROVIDER", "gemini")
	t.Setenv("AI_PROVIDERS", "")
	t.Setenv("GEMINI_API_KEY", "")

	if _, err := FromEnv(context.Background()); err == nil {
		t.Fatal("FromEnv err = nil, want error for gemini without key")
	}
}

func TestSanitiseTypesKeepsRepo(t *testing.T) {
	got := sanitiseTypes([]string{"repo", "REPO", "gizmo", "article"})
	want := []string{"repo", "article"}
	if len(got) != len(want) {
		t.Fatalf("sanitiseTypes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sanitiseTypes[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
