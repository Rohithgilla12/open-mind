// Structured content for the public /architecture deep-dive page.
//
// KEEP THIS UPDATED: when the architecture changes (a new pipeline stage, a new
// client, a swapped core dependency), edit the data here and bump LAST_UPDATED.
// Keeping content as data — rather than buried in JSX — is what makes the page
// cheap to maintain, and lets us unit-test it without a DOM.

export const LAST_UPDATED = "2026-08-03";

export type Principle = { title: string; body: string };
export type PipelineStage = { name: string; note: string };
export type Client = { name: string; stack: string; role: string };
export type StackRow = { layer: string; choice: string; why: string };

export const principles: Principle[] = [
  {
    title: "Capture is sacred",
    body: "Save paths return instantly. Enrichment is always async — a save never waits on an AI call.",
  },
  {
    title: "Idempotent & retryable",
    body: "Every enrichment job is safe to re-run. Failures never block or corrupt a save.",
  },
  {
    title: "Single-binary self-hosting",
    body: "docker compose up — Postgres plus one Go binary — is the whole deployment. No Redis, no sidecars.",
  },
  {
    title: "Multi-tenant from day one",
    body: "Every table has a user_id; every query is scoped in the store layer. Single-user mode is just an auto-provisioned account.",
  },
  {
    title: "AI is pluggable, never assumed",
    body: "All AI goes through one adapter interface. The noop provider must always keep the app fully functional.",
  },
  {
    title: "Cheap models only",
    body: "The pipeline defaults to budget tiers (Gemini Flash-Lite, DeepSeek, Groq, Ollama). No flagship model in enrichment.",
  },
];

export const pipelineStages: PipelineStage[] = [
  { name: "extract", note: "readability · trafilatura · domdistiller; PDF via go-pdfium on wazero WASM" },
  { name: "classify", note: "card type — article, product, book, recipe, video, tweet, image, note, quote, repo" },
  { name: "summarise", note: "AI adapter — short summary + tags, cheap tier only" },
  { name: "embed", note: "pgvector embedding for semantic + colour search" },
];

export const clients: Client[] = [
  { name: "Web", stack: "Next.js 15 · React 19", role: "Full reader & library UI; talks only through the generated API client." },
  { name: "Extension", stack: "WXT · React", role: "One-click capture from any page. Thin — no business logic." },
  { name: "Mobile", stack: "Expo", role: "Share-sheet-first capture with an offline queue." },
  { name: "Dock", stack: "Tauri", role: "Floating desktop capture + Desk/Recents, global hotkey." },
];

export const stack: StackRow[] = [
  { layer: "Language (server)", choice: "Go 1.25", why: "One static binary, great concurrency, standard-library-first." },
  { layer: "HTTP router", choice: "chi/v5", why: "Small, idiomatic, net/http-compatible." },
  { layer: "Jobs", choice: "River (on Postgres)", why: "Durable queues with priority lanes — no extra broker." },
  { layer: "Database", choice: "Postgres + pgx/v5", why: "The only required service; FTS and pgvector live here too." },
  { layer: "Queries", choice: "sqlc", why: "Type-safe Go from plain SQL; no ORM, no inline SQL in handlers." },
  { layer: "Vectors", choice: "pgvector", why: "Semantic search without a separate vector DB." },
  { layer: "API contract", choice: "oapi-codegen", why: "openapi.yaml generates the Go server and the TS client." },
  { layer: "Extraction", choice: "readability · trafilatura · domdistiller", why: "Layered fallbacks for clean article text." },
  { layer: "PDF", choice: "go-pdfium + wazero", why: "PDFium compiled to WASM — no C toolchain at build time." },
  { layer: "AI", choice: "Gemini · OpenAI-compatible · noop", why: "Ordered fallback chain behind one adapter interface." },
  { layer: "Auth", choice: "Clerk or bearer device keys", why: "AUTH_MODE picks one; token mode keeps self-hosting free of any third party." },
  { layer: "Notifications", choice: "Postgres outbox · Expo push · e-mail", why: "At-least-once delivery with no broker; every channel is opt-in." },
  { layer: "Geocoding", choice: "Google Places or Nominatim (optional)", why: "Turns places named in a video into map pins; unset means places store by name only." },
  { layer: "Reel media", choice: "yt-dlp + ffmpeg (opt-in build)", why: "Samples video frames for on-screen place names; off unless the image is built with it." },
  { layer: "Agents", choice: "MCP (go-sdk)", why: "Openmind exposes an MCP server so assistants can read your library." },
  { layer: "Web", choice: "Next.js 15 · React 19", why: "App Router; warm design tokens from @openmind/ui." },
  { layer: "Tasks", choice: "Taskfile", why: "dev, generate, test, lint, migrate — codegen no-ops when inputs are unchanged." },
];
