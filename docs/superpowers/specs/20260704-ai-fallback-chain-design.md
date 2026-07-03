# AI Fallback Chain + OpenAI-Compatible Provider — Design

Date: 2026-07-04 · Status: Designed autonomously (authorised overnight run) · Closes Milestone 1's "AI adapter: OpenAI-compatible client + fallback chain" item

## Goal

Make AI genuinely pluggable per CLAUDE.md principle 5/6: an OpenAI-compatible provider (covers DeepSeek, Groq, Cerebras, Together, Ollama's `/v1` endpoint) and an ordered fallback chain where 429/quota/5xx fails over to the next provider instead of failing the job. noop remains the guaranteed floor.

## Configuration

- `AI_PROVIDERS` (new, comma-ordered, e.g. `gemini,openai,noop`) supersedes `AI_PROVIDER`; when only `AI_PROVIDER` is set it behaves as a one-element chain (backward compatible — deployed box keeps working).
- OpenAI-compatible provider env: `OPENAI_BASE_URL` (default `https://api.openai.com/v1`), `OPENAI_API_KEY`, `OPENAI_MODEL` (required when provider listed; cheap default `gpt-5-mini` is NOT assumed — model must be explicit to honour "cheap models only"), `OPENAI_EMBED_MODEL` (optional; empty → provider returns `ai.ErrNotSupported` for Embed; embeddings must be 768-dim — provider passes `dimensions: 768` when the API supports it and errors on mismatch).
- Per-provider rate limit: `AI_RPM_<NAME>` (e.g. `AI_RPM_GEMINI=15`) → client-side `x/time/rate` limiter inside the chain; unset → no proactive limit. Limiter waits up to a short bound (e.g. 2s) then falls over rather than blocking the worker.

## Chain semantics (`ai.Chain implements Provider`)

Per call (Summarise/Tag/Embed/ParseQuery), try providers in order:
- `ErrNotSupported` → next provider silently.
- Retryable failure (HTTP 429, 5xx, quota/rate errors, timeouts) → `slog.Warn` + next provider.
- Non-retryable (4xx auth/config) → `slog.Error` + next provider (config errors shouldn't kill enrichment either).
- All exhausted → return last error (River retries the job as today). If noop is in the chain it never errors, so chains ending in noop always succeed (possibly with empty results) — recommended default stays `gemini,noop` semantics via `AI_PROVIDER=gemini` compat (single provider, no implicit noop appended: an explicit chain is the operator's choice; compat mode preserves today's behaviour exactly).
- `Name()` = `chain(<names>)`.

## OpenAI-compatible provider (`internal/ai/openai.go`)

- stdlib `net/http` client (no SDK dep — YAGNI): `POST {base}/chat/completions` (JSON mode via `response_format: {type:"json_object"}` for Tag; plain for Summarise), `POST {base}/embeddings` with `dimensions: 768`. 30s timeout. Errors carry status codes for the chain's retryable classification (`ai.RetryableError{Status}` sentinel-ish type shared with gemini classification).
- Gemini provider errors likewise classified (googleapi 429/5xx → retryable) — smallest viable mapping: string/status inspection isolated in one function per provider.

## Wiring

`ai.FromEnv(ctx)` builds the chain; `cmd/openmind` and tests unchanged in signature. Pipeline/search code untouched (they already treat Provider generically). docker-compose/.env.example/self-hosting.md document the new envs.

## Testing

- Chain unit tests with scripted fake providers: fallover on retryable, skip on ErrNotSupported, stop on success, exhaustion returns last error, RPM limiter falls over when saturated (fake clock not required — use tiny limits + elapsed assertions kept loose).
- OpenAI provider tests against `httptest` fake implementing chat/completions + embeddings (asserts auth header, model, dimensions, JSON-mode parse; 429 → classified retryable). No live API in tests.
- Compat: `AI_PROVIDER=gemini` still yields a working provider (unit test on FromEnv parsing incl. precedence and unknown names → warn+skip).

## Out of scope

Provider health caching/circuit breakers, request batching, cost accounting, per-op provider routing, UI for provider status.
