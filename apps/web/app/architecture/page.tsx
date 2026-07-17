// Public architecture & tech-stack deep dive — a contributor-facing tour of how
// Openmind is built. Exempted from auth in both modes (see middleware.ts).
//
// KEEP THIS UPDATED: when the architecture changes (a new pipeline stage, a new
// client, a swapped dependency), edit the structured data below and bump
// `LAST_UPDATED`. Content lives as typed data at the top of the file so updates
// stay readable — no JSX archaeology required.
import type { CSSProperties, ReactNode } from "react";
import { tokens } from "@openmind/ui";
import {
  LAST_UPDATED,
  clients,
  pipelineStages,
  principles,
  stack,
} from "../../lib/architecture";

export const metadata = {
  title: "Architecture — Openmind",
  description:
    "How Openmind is built: a single Go binary, an OpenAPI contract, an async enrichment pipeline, a pluggable AI chain, and hybrid Postgres search.",
};

const c = tokens.color;

// --- Layout primitives ------------------------------------------------------

const maxW = 780;

function Kicker({ children }: { children: ReactNode }) {
  return (
    <p className="meta" style={{ marginBottom: 8 }}>
      {children}
    </p>
  );
}

function Section({
  kicker,
  title,
  children,
}: {
  kicker: string;
  title: string;
  children: ReactNode;
}) {
  return (
    <section style={{ marginTop: 56 }}>
      <Kicker>{kicker}</Kicker>
      <h2 className="serif" style={{ fontSize: 26, fontStyle: "italic", margin: "0 0 14px" }}>
        {title}
      </h2>
      {children}
    </section>
  );
}

const boxStyle: CSSProperties = {
  background: c.cardSurface,
  border: `1px solid ${c.hairline}`,
  borderRadius: 10,
  padding: "10px 14px",
  fontFamily: tokens.font.mono,
  fontSize: 12,
  fontWeight: 600,
  color: c.ink,
  whiteSpace: "nowrap",
};

function Arrow() {
  return (
    <span aria-hidden style={{ color: c.inkFaint, fontSize: 16, lineHeight: 1 }}>
      →
    </span>
  );
}

function FlowRow({ items }: { items: string[] }) {
  return (
    <div
      style={{
        display: "flex",
        flexWrap: "wrap",
        alignItems: "center",
        gap: 10,
        padding: "16px 0",
      }}
    >
      {items.map((item, i) => (
        <span key={item} style={{ display: "inline-flex", alignItems: "center", gap: 10 }}>
          <span style={boxStyle}>{item}</span>
          {i < items.length - 1 && <Arrow />}
        </span>
      ))}
    </div>
  );
}

// --- Page -------------------------------------------------------------------

export default function ArchitecturePage() {
  return (
    <main
      style={{
        maxWidth: maxW,
        margin: "0 auto",
        padding: "56px 20px 112px",
        lineHeight: 1.7,
        fontSize: 15.5,
      }}
    >
      <p className="meta">Openmind · Architecture · updated {LAST_UPDATED}</p>
      <h1
        className="serif"
        style={{ fontSize: 40, fontStyle: "italic", lineHeight: 1.15, margin: "10px 0 20px" }}
      >
        How Openmind is built.
      </h1>
      <p style={{ fontSize: 17, maxWidth: "60ch" }}>
        Openmind is an open-source, self-hostable commonplace book: save any link, note, image, or
        quote, and it quietly enriches and organises each one so you can find it again by a
        fragment — a colour, a word, a half-remembered vibe. This page is the contributor&apos;s
        tour: the shape of the system, the decisions that hold it together, and the stack that runs
        it.
      </p>

      {/* Principles */}
      <Section kicker="First principles" title="Six things that never bend">
        <p style={{ marginTop: 0 }}>
          Every design decision answers to these. When a feature and a principle disagree, the
          principle wins.
        </p>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
            gap: 14,
            marginTop: 18,
          }}
        >
          {principles.map((p) => (
            <div
              key={p.title}
              style={{
                background: c.cardSurface,
                border: `1px solid ${c.hairline}`,
                borderRadius: 12,
                padding: "16px 16px 18px",
              }}
            >
              <h3 style={{ fontSize: 15, margin: "0 0 6px", fontWeight: 600 }}>{p.title}</h3>
              <p style={{ margin: 0, fontSize: 14, color: c.inkMuted, lineHeight: 1.55 }}>
                {p.body}
              </p>
            </div>
          ))}
        </div>
      </Section>

      {/* The shape */}
      <Section kicker="The shape" title="A monorepo, one binary">
        <p>
          The repository is a pnpm + Turborepo monorepo. The server is a single Go program under{" "}
          <code>apps/api</code> that runs in one of four modes —{" "}
          <code>serve</code>, <code>work</code>, <code>all</code>, or <code>migrate</code> — so the
          API and the background workers are the same binary, just started differently.
        </p>
        <ul style={{ margin: "14px 0", paddingLeft: 20, display: "grid", gap: 6 }}>
          <li>
            <code>apps/api</code> — Go API + River workers. Inside: <code>internal/api</code>{" "}
            (handlers), <code>internal/enrich</code> (pipeline), <code>internal/ai</code>{" "}
            (adapters + fallback chain), <code>internal/search</code> (FTS + vector fusion),{" "}
            <code>internal/store</code> (sqlc), <code>internal/jobs</code> (River jobs).
          </li>
          <li>
            <code>apps/web</code>, <code>apps/extension</code>, <code>apps/mobile</code>,{" "}
            <code>apps/dock</code> — the clients.
          </li>
          <li>
            <code>packages/api-client</code> — the TypeScript client, generated from{" "}
            <code>openapi.yaml</code>, never hand-edited. <code>packages/ui</code> — shared React
            components + design tokens.
          </li>
        </ul>
      </Section>

      {/* The contract */}
      <Section kicker="The contract" title="openapi.yaml is the spine">
        <p>
          The API contract is the single source of truth. To change an endpoint you edit{" "}
          <code>openapi.yaml</code>, regenerate, then implement — never the other way round.
          Hand-writing API types in TypeScript, or adding a Go route that isn&apos;t in the spec,
          is a bug.
        </p>
        <FlowRow items={["edit openapi.yaml", "task generate", "Go server + TS client", "implement handler"]} />
        <p style={{ marginTop: 0, fontSize: 14, color: c.inkMuted }}>
          <code>task generate</code> runs oapi-codegen for both the Go server stubs and the TS
          client, plus sqlc for the store layer. It checksums its inputs and no-ops when nothing
          changed.
        </p>
      </Section>

      {/* Capture */}
      <Section kicker="The save path" title="Capture is sacred">
        <p>
          A save writes the item and returns — immediately. Everything expensive happens afterwards,
          on a durable queue, so the moment of capture is never held hostage to a network call or an
          AI provider having a bad day.
        </p>
        <FlowRow items={["capture", "write item + 202", "enqueue River job", "async enrichment"]} />
      </Section>

      {/* Pipeline */}
      <Section kicker="Enrichment" title="An idempotent pipeline">
        <p>
          Enrichment runs as River jobs on Postgres, with priority lanes so interactive work jumps
          ahead of backfills. Every job fetches fresh state, keeps its payload minimal (IDs, not
          blobs), and is safe to run twice — the second run produces the same result.
        </p>
        <div style={{ margin: "8px 0 6px" }}>
          {pipelineStages.map((s, i) => (
            <div
              key={s.name}
              style={{
                display: "flex",
                gap: 14,
                alignItems: "baseline",
                padding: "12px 0",
                borderTop: i === 0 ? "none" : `1px solid ${c.hairline}`,
              }}
            >
              <span
                style={{
                  ...boxStyle,
                  whiteSpace: "nowrap",
                  minWidth: 96,
                  textAlign: "center",
                }}
              >
                {s.name}
              </span>
              <span style={{ fontSize: 14, color: c.inkMuted }}>{s.note}</span>
            </div>
          ))}
        </div>
        <p style={{ fontSize: 14, color: c.inkMuted }}>
          Beyond the core four, dedicated jobs handle lead-image fetching, colour-palette
          extraction, place detection from reels, feed polling, Kindle delivery, and the Drift
          digest.
        </p>
      </Section>

      {/* AI */}
      <Section kicker="Intelligence" title="Pluggable AI, cheap by default">
        <p>
          All AI goes through one adapter interface — <code>Summarise</code>, <code>Tag</code>,{" "}
          <code>Embed</code>, <code>ParseQuery</code> — behind an ordered fallback chain. A 429 is
          treated as &ldquo;fall over to the next provider&rdquo;, not a failure. Providers only ever
          receive the content they need to enrich; they never receive your identity.
        </p>
        <FlowRow items={["Gemini Flash-Lite", "OpenAI-compatible", "noop"]} />
        <p style={{ marginTop: 0, fontSize: 14, color: c.inkMuted }}>
          The <code>noop</code> provider is the floor: with no AI configured at all, the app stays
          fully usable — manual tags and full-text search still work. The pipeline defaults to
          budget model tiers; a flagship model is never wired into enrichment.
        </p>
      </Section>

      {/* Search */}
      <Section kicker="Retrieval" title="Hybrid search">
        <p>
          Search fuses two signals living in the same Postgres database: full-text search for exact
          words, and pgvector similarity for meaning. Their rankings are combined by rank fusion, an
          optional reranker can reorder the top results, and a dedicated colour index lets you find
          things by hue.
        </p>
        <FlowRow items={["query", "FTS + pgvector", "rank fusion", "optional rerank", "results"]} />
      </Section>

      {/* Data */}
      <Section kicker="Data" title="Multi-tenant to the core">
        <p>
          Every table carries a <code>user_id</code>, and every store method takes a{" "}
          <code>ctx</code> and scopes by it — a query without a <code>user_id</code> predicate is a
          bug unless explicitly justified. All SQL goes through sqlc in{" "}
          <code>internal/store</code>; there is no inline SQL in handlers or jobs. Self-hosted
          single-user mode is simply an account that provisions itself on first run.
        </p>
      </Section>

      {/* Clients */}
      <Section kicker="The edges" title="Thin clients, one brain">
        <p>
          Enrichment logic lives on the server, only. The clients capture and display — nothing
          more — and every one of them fetches through the generated API client.
        </p>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))",
            gap: 14,
            marginTop: 16,
          }}
        >
          {clients.map((cl) => (
            <div
              key={cl.name}
              style={{
                background: c.cardSurface,
                border: `1px solid ${c.hairline}`,
                borderRadius: 12,
                padding: "16px",
              }}
            >
              <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 8 }}>
                <h3 style={{ fontSize: 15, margin: 0, fontWeight: 600 }}>{cl.name}</h3>
                <span className="meta" style={{ margin: 0 }}>
                  {cl.stack}
                </span>
              </div>
              <p style={{ margin: "8px 0 0", fontSize: 14, color: c.inkMuted, lineHeight: 1.55 }}>
                {cl.role}
              </p>
            </div>
          ))}
        </div>
      </Section>

      {/* Self-hosting */}
      <Section kicker="Running it" title="One command to self-host">
        <p>
          The entire deployment is <code>docker compose up</code>: Postgres and the one Go binary.
          There is no required Redis, no Python sidecar, no message broker — Postgres carries the
          data, the jobs, the full-text index, and the vectors. Every optional integration (AI
          providers, email, Kindle) sits behind configuration and degrades gracefully when absent.
        </p>
      </Section>

      {/* Stack table */}
      <Section kicker="The stack" title="What runs where, and why">
        <div style={{ overflowX: "auto", marginTop: 8 }}>
          <table style={{ borderCollapse: "collapse", width: "100%", fontSize: 14 }}>
            <thead>
              <tr>
                {["Layer", "Choice", "Why"].map((h) => (
                  <th
                    key={h}
                    className="meta"
                    style={{
                      textAlign: "left",
                      padding: "8px 12px 8px 0",
                      borderBottom: `1px solid ${c.hairline}`,
                    }}
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {stack.map((row) => (
                <tr key={row.layer}>
                  <td
                    style={{
                      padding: "10px 12px 10px 0",
                      borderBottom: `1px solid ${c.hairline}`,
                      verticalAlign: "top",
                      color: c.inkMuted,
                      whiteSpace: "nowrap",
                    }}
                  >
                    {row.layer}
                  </td>
                  <td
                    style={{
                      padding: "10px 12px 10px 0",
                      borderBottom: `1px solid ${c.hairline}`,
                      verticalAlign: "top",
                      fontWeight: 600,
                    }}
                  >
                    {row.choice}
                  </td>
                  <td
                    style={{
                      padding: "10px 0",
                      borderBottom: `1px solid ${c.hairline}`,
                      verticalAlign: "top",
                      color: c.inkMuted,
                    }}
                  >
                    {row.why}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      <p className="meta" style={{ marginTop: 64 }}>
        <a href="/welcome" style={{ color: "inherit" }}>
          Home
        </a>
        {"  ·  "}
        <a href="/privacy" style={{ color: "inherit" }}>
          Privacy
        </a>
        {"  ·  "}
        <a href="/terms" style={{ color: "inherit" }}>
          Terms
        </a>
      </p>
    </main>
  );
}
