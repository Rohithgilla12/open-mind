# AI Fallback Chain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** OpenAI-compatible provider + ordered fallback chain with per-provider RPM limits and retryable-failure fallover. Spec: `docs/superpowers/specs/20260704-ai-fallback-chain-design.md` — read it; it is the authority on semantics.

**Architecture:** All inside `apps/api/internal/ai`. New `openai.go` (stdlib HTTP), `chain.go` (Provider-implementing orchestrator), `retry.go` (retryable classification), `FromEnv` rewrite with `AI_PROVIDERS`/`AI_PROVIDER` precedence. Consumers (pipeline, search, cmd) untouched.

**Tech Stack:** stdlib + existing deps only (`golang.org/x/time/rate` already in go.mod).

## Global Constraints

- Spec's chain semantics verbatim (ErrNotSupported silent-next; retryable warn-next; non-retryable error-next; exhaustion returns last error; `AI_PROVIDER` compat = single provider, no implicit noop).
- Embed dimension stays `ai.EmbedDims` (768) everywhere; OpenAI embeddings request `dimensions: 768` and length-check the response.
- Cheap models only: `OPENAI_MODEL` has no default — listed-but-unconfigured provider is a startup error from FromEnv.
- No live API calls in tests (httptest fakes). TDD. `go test -p 1 ./... && golangci-lint run ./...` green. No banner comments; errors `%w`.

---

### Task 1: Retryable classification + OpenAI-compatible provider

**Files:**
- Create: `apps/api/internal/ai/retry.go`, `apps/api/internal/ai/openai.go`
- Test: `apps/api/internal/ai/retry_test.go`, `apps/api/internal/ai/openai_test.go`

**Interfaces:**
- Produces:
```go
// retry.go
type RetryableError struct{ Status int; Err error } // Error(), Unwrap()
func Retryable(err error) bool // true for RetryableError with status 429 or >=500, context.DeadlineExceeded, net timeouts
// openai.go
type OpenAI struct{ /* baseURL, apiKey, model, embedModel string; client *http.Client */ }
func NewOpenAI(baseURL, apiKey, model, embedModel string) *OpenAI // implements Provider; Name() "openai"
type OpenAIOption // WithHTTPClient(c) for tests (or export a settable client field — pick one, document)
```
- Behaviour: Summarise → chat/completions (system prompt mirroring gemini's, temperature 0.3); Tag → chat/completions with `response_format: {"type":"json_object"}`, prompt demanding `{"tags": [...]}`, parse+lowercase; Embed → embeddings endpoint with `dimensions: EmbedDims` (omit field when embedModel doesn't support it? NO — always send; servers ignoring it get caught by the length check), error `%w` + RetryableError wrapping by status; ParseQuery passthrough. embedModel == "" → Embed returns ErrNotSupported.

- [ ] Steps: failing tests (httptest fake asserting Authorization Bearer, model field, JSON-mode tag parse, embeddings dimensions field, 768-length response OK + wrong-length error, 429 wrapped Retryable, 500 Retryable, 401 non-retryable) → implement → `go test -p 1 ./internal/ai/` green → commit `feat(ai): openai-compatible provider + retryable error classification`.

---

### Task 2: Chain + FromEnv + wiring

**Files:**
- Create: `apps/api/internal/ai/chain.go`
- Modify: `apps/api/internal/ai/ai.go` (FromEnv), `docker-compose.yml` (env passthrough), `.env.example`
- Test: `apps/api/internal/ai/chain_test.go`, extend `ai` FromEnv tests

**Interfaces:**
- Produces:
```go
type Chain struct{ /* entries []chainEntry{name string; p Provider; limiter *rate.Limiter} */ }
func NewChain(entries ...ChainEntry) *Chain // ChainEntry{Name string; Provider Provider; RPM int}
// implements Provider; Name() == "chain(gemini,openai,noop)"
```
- Per-call algorithm per spec: limiter wait bounded at 2s (ctx with timeout) → saturated = treat as retryable fallover; ErrNotSupported → silent next; Retryable(err) → slog.Warn next; other error → slog.Error next; success → return. Exhausted → last error (or ErrNotSupported if every provider said so).
- FromEnv: `AI_PROVIDERS` CSV wins; else `AI_PROVIDER` single; unknown name → slog.Warn + skip; zero usable providers → noop with warn (never a dead app); "openai" listed without OPENAI_MODEL or OPENAI_API_KEY → error; `AI_RPM_<UPPER(NAME)>` parsed. Single-entry chains return the bare provider (no wrapper overhead) EXCEPT when an RPM is configured for it.

- [ ] Steps: failing chain tests (scripted fakes: fallover-on-429, skip-on-notsupported, first-success-wins, exhaustion, limiter fallover with RPM=1 second call) + FromEnv precedence tests (env via t.Setenv) → implement → full suite + lint green → update compose (`AI_PROVIDERS`, `OPENAI_BASE_URL/API_KEY/MODEL/EMBED_MODEL`, `AI_RPM_*` passthrough with `:-` defaults) + .env.example → commit `feat(ai): ordered fallback chain with per-provider rate limits`.

---

### Task 3: Docs + e2e evidence + wrap-up

**Files:**
- Modify: `docs/self-hosting.md` (providers section rewrite: table of envs, example chains incl. Ollama via OPENAI_BASE_URL=http://host:11434/v1), `TODO.md`

- [ ] Local e2e: compose up api with `AI_PROVIDERS=openai,noop` + `OPENAI_BASE_URL` pointed at a tiny local fake (run the httptest-style fake as a real listener via `go run` scratch script in the scratchpad, or simpler: set OPENAI_BASE_URL to an unreachable port and prove graceful chain fallover to noop — item still enriches, status enriched, warn logged). Record evidence. Stop api after.
- [ ] TODO.md: AI adapter item → Done (dated). self-hosting.md providers section.
- [ ] Commit `feat(ai): chain e2e evidence + provider docs`. Controller merges, pushes, redeploys (deployed box keeps `AI_PROVIDER=gemini` — compat path — no .env change needed; optionally switch to `AI_PROVIDERS=gemini,noop`).
