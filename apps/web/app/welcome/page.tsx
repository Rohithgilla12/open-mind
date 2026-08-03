// Public landing served at "/" for anonymous visitors (middleware rewrite).
// Exists so the homepage explains the product without requiring sign-in —
// needed by humans and by Google's OAuth branding verification alike.
//
// Deliberately a static server component: no client JS, no interactivity
// beyond links, so an anonymous first paint stays cheap. Layout-only styles
// that need media queries live in globals.css under the `.lp-` prefix.
//
// Every claim here is meant to describe shipped behaviour. If a feature is
// gated on configuration (Send to Kindle needs SMTP; AI needs a provider), the
// copy says so rather than implying it works out of the box.
import { tokens } from "@openmind/ui";

export const metadata = {
  title: "Openmind — the self-hosted commonplace book",
  description:
    "Save links, notes, quotes, and images. Openmind enriches and organises them, so you can find anything by a fragment — a colour, a word, a vibe. Lenses, Drift, RSS, maps, Send to Kindle, and an MCP server for your AI agents. Open source: Postgres plus one binary.",
};

const REPO = "https://github.com/Rohithgilla12/open-mind";

function Mark({ size = 30 }: { size?: number }) {
  return (
    <img
      src="/marketing/mark.svg"
      width={size}
      height={size}
      alt=""
      style={{ display: "block", borderRadius: size * 0.235 }}
    />
  );
}

function Act({ label, title }: { label: string; title: string }) {
  return (
    <div className="lp-act">
      <p className="meta">{label}</p>
      <h2
        className="serif"
        style={{
          fontSize: 30,
          fontWeight: 600,
          fontStyle: "italic",
          letterSpacing: "-.015em",
          margin: "10px 0 0",
        }}
      >
        {title}
      </h2>
    </div>
  );
}

function Feature({
  eyebrow,
  title,
  body,
  shot,
  alt,
  flip = false,
}: {
  eyebrow: string;
  title: string;
  body: React.ReactNode;
  shot?: string;
  alt?: string;
  flip?: boolean;
}) {
  // Without a screenshot the two-column grid would leave half the row empty, so
  // imageless features run full width with the prose constrained for line length.
  if (!shot) {
    return (
      <section style={{ padding: "52px 0" }}>
        <p className="meta" style={{ marginBottom: 10 }}>
          {eyebrow}
        </p>
        <h3
          className="serif"
          style={{
            fontSize: 24,
            fontWeight: 600,
            letterSpacing: "-.01em",
            margin: "0 0 12px",
          }}
        >
          {title}
        </h3>
        <div style={{ ...prose, maxWidth: "68ch" }}>{body}</div>
      </section>
    );
  }

  return (
    <section className={`lp-feature${flip ? " lp-feature-flip" : ""}`}>
      <div className="lp-feature-copy">
        <p className="meta" style={{ marginBottom: 10 }}>
          {eyebrow}
        </p>
        <h3
          className="serif"
          style={{
            fontSize: 24,
            fontWeight: 600,
            letterSpacing: "-.01em",
            margin: "0 0 12px",
          }}
        >
          {title}
        </h3>
        <div style={prose}>{body}</div>
      </div>
      <img className="lp-shot" src={shot} alt={alt ?? ""} loading="lazy" />
    </section>
  );
}

function Panel({
  label,
  title,
  children,
}: {
  label: string;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="lp-panel">
      <p className="meta" style={{ marginBottom: 8 }}>
        {label}
      </p>
      <h4
        style={{
          fontFamily: tokens.font.sans,
          fontSize: 14.5,
          fontWeight: 600,
          margin: "0 0 8px",
        }}
      >
        {title}
      </h4>
      <div style={{ ...prose, fontSize: 14 }}>{children}</div>
    </div>
  );
}

const MCP_TOOLS = [
  "save_item",
  "search_items",
  "list_recent",
  "get_item",
  "related_items",
  "set_user_tags",
  "pin_item",
  "delete_item",
  "get_desk",
  "get_drift",
  "list_lenses",
  "run_lens",
  "create_lens",
  "delete_lens",
];

export default function WelcomePage() {
  return (
    <>
      <header
        style={{
          borderBottom: `1px solid ${tokens.color.hairline}`,
          background: tokens.color.header,
        }}
      >
        <div
          className="lp-wrap"
          style={{ display: "flex", alignItems: "center", gap: 10, height: 62 }}
        >
          <Mark />
          <span
            className="serif"
            style={{ fontSize: 19, fontWeight: 600, letterSpacing: "-.01em" }}
          >
            Openmind
          </span>
          <span style={{ flex: 1 }} />
          <a href={REPO} className="meta" style={{ color: tokens.color.inkMuted, textDecoration: "none" }}>
            GitHub
          </a>
          <a href="/login" className="meta" style={{ color: tokens.color.cobalt, textDecoration: "none" }}>
            Sign in
          </a>
        </div>
      </header>

      <main>
        {/* ---------------------------------------------------------- hero */}
        <div className="lp-wrap" style={{ padding: "72px 24px 8px" }}>
          <p className="meta">The self-hosted commonplace book</p>
          <h1 className="serif lp-hero-title" style={{ marginTop: 14 }}>
            A home for everything
            <br />
            worth keeping.
          </h1>
          <p className="lp-lede">
            Save any link, note, quote, or image in under a second. Openmind
            quietly enriches each one — a summary, tags, the colours of the page
            — so you can find it again by a fragment: a word, a hue, a
            half-remembered vibe.
          </p>
          <div style={{ display: "flex", gap: 12, flexWrap: "wrap", marginBottom: 54 }}>
            <a href="/login" className="lp-cta">
              Sign in
            </a>
            <a href={`${REPO}#quickstart`} className="lp-cta-ghost">
              Self-host it →
            </a>
          </div>
          <img
            className="lp-shot"
            src="/marketing/the-mind.jpg"
            alt="Openmind's main library view: a masonry grid of saved articles, quotes, books and images, each card showing the colours extracted from it"
          />
        </div>

        {/* ------------------------------------------------------- finding */}
        <div className="lp-wrap">
          <Act label="Act one" title="Finding things again" />

          <Feature
            eyebrow="Search"
            title="Find it by a fragment"
            body={
              <>
                <p style={p}>
                  You rarely remember the title. You remember it was blue, or
                  about latency, or something a friend linked in March. Search
                  fuses Postgres full-text with pgvector semantic similarity and
                  fuses the two rankings, so a phrase you half-remember still
                  lands.
                </p>
                <p style={p}>
                  Every save keeps the colours extracted from it, so colour is a
                  real filter rather than decoration — tap a swatch to narrow the
                  library to things that look like it.
                </p>
              </>
            }
            shot="/marketing/search-colour.jpg"
            alt="Colour search: a cobalt swatch is selected and the library has filtered to items whose palette contains that blue"
          />

          <Feature
            eyebrow="Lenses"
            title="Saved queries that stay live"
            body={
              <>
                <p style={p}>
                  A Lens is a query, not a folder. Save a search you keep
                  running — a topic, a colour, a card type — and new saves fall
                  into it automatically. Nothing to file, nothing to maintain.
                </p>
                <p style={p}>
                  Any Lens can also be put on a schedule: daily, or weekly on a
                  chosen day. A scheduled digest only contains what's{" "}
                  <em>new since the last one</em>, so a quiet week simply sends
                  nothing.
                </p>
              </>
            }
            shot="/marketing/lens.jpg"
            alt="A Lens named Typography showing the items its saved rule currently matches"
            flip
          />
        </div>

        {/* ---------------------------------------------------- resurfacing */}
        <div className="lp-wrap">
          <Act label="Act two" title="The opposite of a backlog" />

          <Feature
            eyebrow="Drift"
            title="A daily ritual, not a queue"
            body={
              <>
                <p style={p}>
                  Most read-later apps turn saving into debt. Drift resurfaces a
                  few forgotten items a day, one at a time — keep it on your
                  Desk, or let it go.
                </p>
                <p style={p}>
                  No streaks, no unread count, no badge demanding attention.
                  Items you've already drifted past step aside so the rotation
                  keeps reaching further back.
                </p>
              </>
            }
            shot="/marketing/drift.jpg"
            alt="Drift mode: a single resurfaced card on a dark backdrop, with Let go and Keep on my desk actions"
          />

          <Feature
            eyebrow="Desk"
            title="What you're thinking about now"
            body={
              <p style={p}>
                A pinboard for the handful of things currently live in your head.
                Pin from anywhere — the grid, the reader, Drift, even an AI agent
                — and unpin when it stops mattering. The Desk is deliberately
                small; it's a working surface, not storage.
              </p>
            }
            shot="/marketing/desk.jpg"
            alt="The Desk: a pinboard of pinned cards including a quote, a note, an article and a colour study"
            flip
          />
        </div>

        {/* --------------------------------------------------------- reading */}
        <div className="lp-wrap">
          <Act label="Act three" title="Reading, and taking it with you" />

          <Feature
            eyebrow="Reader"
            title="Without the web attached"
            body={
              <>
                <p style={p}>
                  Articles are archived on save and open in a clean reader — no
                  banners, no consent walls, no layout shift. The text is stored
                  in your database, so it survives the page going away.
                </p>
                <p style={p}>
                  Highlight a passage and it becomes its own quote card, linked
                  back to the source. The line outlives the article.
                </p>
              </>
            }
            shot="/marketing/reader.jpg"
            alt="The reader view showing an article with its summary, tags, highlighted passages and related items"
          />

          <Feature
            eyebrow="Send to Kindle"
            title="Read it on e-ink instead"
            body={
              <>
                <p style={p}>
                  Screens are where you save things; e-ink is where you actually
                  read them. Openmind builds a proper EPUB — cover page, and a
                  hero image where the article had one — and mails it to your
                  Kindle.
                </p>
                <p style={p}>
                  Delivery is queued and runs in the background, so the button
                  returns instantly rather than blocking on SMTP. Everyone sets
                  their own Kindle address, so one instance can serve a household.
                </p>
                <div className="lp-grid3" style={{ margin: "22px 0 16px" }}>
                  <Panel label="One item" title="Send this article">
                    A single save becomes a single EPUB. Straight from the reader
                    toolbar.
                  </Panel>
                  <Panel label="A whole Lens" title="Your own periodical">
                    A digest with one chapter per item — up to 25 — assembled
                    from whatever the Lens currently matches.
                  </Panel>
                  <Panel label="On a schedule" title="Daily or weekly">
                    Pick a cadence and it arrives on its own, containing only
                    what's new since the last one. A quiet week sends nothing.
                  </Panel>
                </div>
                <p style={{ ...p, color: tokens.color.inkFaintAlt, marginBottom: 0 }}>
                  Needs outbound SMTP configured on your instance, plus your
                  sender address approved in Amazon's personal-document settings.
                </p>
              </>
            }
          />
        </div>

        {/* ------------------------------------------------ bringing things in */}
        <div className="lp-wrap">
          <Act label="Act four" title="Bringing the world in" />

          <Feature
            eyebrow="Feeds"
            title="RSS without an inbox"
            body={
              <>
                <p style={p}>
                  Subscribe to RSS or Atom and new items land in a{" "}
                  <strong>river</strong> — searchable straight away, but not part
                  of your library until you keep one. Nothing accumulates unread,
                  because nothing was ever addressed to you.
                </p>
                <p style={p}>
                  Feeds re-poll automatically every half hour from inside the
                  same binary — no cron, no extra service — and entries are
                  matched against what you've already saved, so re-polling never
                  double-saves.
                </p>
              </>
            }
            shot="/marketing/feed.jpg"
            alt="The feed river showing new unread items from subscribed feeds, each with a Keep action"
          />

          <Feature
            eyebrow="Places"
            title="Saved somewhere, put on a map"
            body={
              <>
                <p style={p}>
                  Save a video or post about somewhere and Openmind reads the
                  place names out of the caption, geocodes them, and pins them on
                  a map. A restaurant reel becomes three pins you can actually
                  navigate to.
                </p>
                <p style={p}>
                  Everything you've saved with a location collects into one map
                  of the places you've been meaning to go.
                </p>
              </>
            }
            shot="/marketing/places.jpg"
            alt="A map with cobalt pins marking coffee bars extracted from a saved reel's caption"
            flip
          />

          <Feature
            eyebrow="Import"
            title="Bring your existing library"
            body={
              <>
                <p style={p}>
                  Browser bookmarks, Pocket, Raindrop, Pinboard, Instapaper, an
                  Omnivore zip, a CSV, or just a list of URLs. Raindrop.io also
                  connects directly with an API test token — no export file
                  needed, and collections become tags. Each new URL becomes a
                  pending item and enriches in the background.
                </p>
                <p style={p}>
                  URLs already in your library are skipped, so re-running an
                  import is safe. And you can leave the same way you came: one
                  click exports every item as JSON, archived text included.
                </p>
              </>
            }
            shot="/marketing/import.jpg"
            alt="The import screen listing supported formats including browser bookmarks, Pocket, Raindrop and Omnivore"
          />
        </div>

        {/* --------------------------------------------------------- capture */}
        <div className="lp-wrap">
          <Act label="Act five" title="Save from wherever you are" />
          <p style={{ ...prose, maxWidth: "62ch", margin: "14px 0 24px" }}>
            <span style={p}>
              Saves return instantly on every surface — enrichment always happens
              after, never in the request. Each client is a thin capture layer;
              all the logic lives server-side.
            </span>
          </p>
          <div className="lp-grid4">
            <Panel label="Browser" title="Extension">
              One-click page save, or right-click a selection or an image.
              Quick-tag without leaving the page. Chrome, Edge, Brave, Firefox.
            </Panel>
            <Panel label="Phone" title="Share sheet">
              Share anything into Openmind from any app. Photos queue offline and
              upload when you're back.
            </Panel>
            <Panel label="Desktop" title="Floating dock">
              A hotkey for quick save and quick find, without switching windows.
              Still in progress.
            </Panel>
            <Panel label="Anything else" title="HTTP API">
              It's one <code style={code}>POST /api/items</code>. Scripts, shortcuts,
              a watch complication — whatever you like.
            </Panel>
          </div>
        </div>

        {/* ------------------------------------------------------------- MCP */}
        <div className="lp-wrap">
          <Act label="Act six" title="Your agents get a key too" />
          <div className="lp-grid2" style={{ marginTop: 22, alignItems: "start" }}>
            <div>
              <div style={prose}>
                <p style={p}>
                  Openmind speaks the{" "}
                  <a href="https://modelcontextprotocol.io" style={link}>
                    Model Context Protocol
                  </a>
                  , so Claude — or any MCP client — can work against your library
                  directly. It's served by the same binary as everything else, at{" "}
                  <code style={code}>/mcp</code>, with no extra process to run.
                </p>
                <p style={p}>
                  So you can just ask:
                </p>
                <p
                  className="serif"
                  style={{
                    fontStyle: "italic",
                    fontSize: 17,
                    lineHeight: 1.5,
                    color: tokens.color.ink,
                    borderLeft: `2px solid ${tokens.color.gold}`,
                    paddingLeft: 14,
                    margin: "0 0 14px",
                  }}
                >
                  “Find that essay about latency I saved in March, pin it to my
                  desk, and tag it reference.”
                </p>
                <p style={p}>
                  Destructive tools are guarded — deleting an item refuses
                  without explicit confirmation, so an agent has to check with
                  you first. Reading Drift candidates doesn't consume your daily
                  Drift.
                </p>
              </div>
            </div>
            <div>
              <pre className="lp-pre" style={{ marginTop: 0 }}>
                <code>{`claude mcp add --transport http openmind \\
  https://your-instance.example.com/mcp \\
  --header "Authorization: Bearer $TOKEN"`}</code>
              </pre>
              <p className="meta" style={{ margin: "18px 0 9px" }}>
                14 tools
              </p>
              <ul className="lp-chips">
                {MCP_TOOLS.map((t) => (
                  <li key={t}>{t}</li>
                ))}
              </ul>
              <p style={{ ...p, fontSize: 13.5, marginTop: 14, color: tokens.color.inkFaintAlt }}>
                Plus a resource template for attaching an item's text without a
                tool call, and a prompt that walks a client through search → read
                → summarise.
              </p>
            </div>
          </div>
        </div>

        {/* ------------------------------------------------------ self-hosting */}
        <div className="lp-wrap">
          <Act label="Act seven" title="Postgres, and one binary" />
          <div className="lp-grid2" style={{ marginTop: 22, alignItems: "start" }}>
            <div style={prose}>
              <p style={p}>
                No Redis, no message broker, no Python sidecar — and there never
                will be. The API and its background workers are the same Go
                binary; jobs live in Postgres. <code style={code}>docker compose up</code>{" "}
                is the whole deployment.
              </p>
              <p style={p}>
                Every table is scoped by user from the first migration, so a
                single-user install is just an account that provisions itself.
              </p>
              <p style={p}>
                <strong>AI is pluggable and never assumed.</strong> The default
                provider does nothing at all, and the app stays fully usable —
                manual tags, full-text search. Point it at a cheap hosted model,
                any OpenAI-compatible endpoint, or a local Ollama when you want
                summaries and semantic search. Providers form a fallback chain, so
                a rate-limited one steps aside instead of failing the job.
              </p>
            </div>
            <div>
              <pre className="lp-pre" style={{ marginTop: 0 }}>
                <code>{`git clone ${REPO.replace("https://", "")}
cd open-mind && docker compose up -d
open http://localhost:3000`}</code>
              </pre>
              <div className="lp-grid2" style={{ marginTop: 18, gap: 14 }}>
                <Panel label="Licence" title="AGPL-3.0">
                  Run it, modify it, keep it. If you offer it to others as a
                  service, share your changes.
                </Panel>
                <Panel label="Your data" title="Export any time">
                  Every item as JSON, archived text included. No lock-in by
                  design.
                </Panel>
              </div>
            </div>
          </div>
          <p style={{ ...prose, margin: "26px 0 0" }}>
            <a href={`${REPO}#quickstart`} className="lp-cta">
              Read the self-hosting guide →
            </a>
          </p>
        </div>

        <footer
          style={{
            borderTop: `1px solid ${tokens.color.hairline}`,
            marginTop: 56,
            padding: "26px 0 44px",
          }}
        >
          <div
            className="lp-wrap"
            style={{ display: "flex", gap: 18, flexWrap: "wrap", alignItems: "center" }}
          >
            <Mark size={22} />
            <a href="/architecture" className="meta" style={{ color: "inherit" }}>
              Architecture
            </a>
            <a href="/privacy" className="meta" style={{ color: "inherit" }}>
              Privacy
            </a>
            <a href="/terms" className="meta" style={{ color: "inherit" }}>
              Terms
            </a>
            <a href={REPO} className="meta" style={{ color: "inherit" }}>
              Source
            </a>
            <span style={{ flex: 1 }} />
            <span className="meta">AGPL-3.0</span>
          </div>
        </footer>
      </main>
    </>
  );
}

const prose = {
  fontSize: 15.5,
  lineHeight: 1.65,
  color: tokens.color.inkMuted,
} as const;

const p = { margin: "0 0 12px" } as const;

const code = {
  fontFamily: tokens.font.mono,
  fontSize: 13,
  background: "rgba(28,26,22,.055)",
  padding: "2px 5px",
  borderRadius: 4,
} as const;

const link = { color: tokens.color.cobalt, textDecoration: "underline" } as const;
