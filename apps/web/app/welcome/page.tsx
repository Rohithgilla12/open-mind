// Public landing served at "/" for anonymous visitors (middleware rewrite).
// Exists so the homepage explains the product without requiring sign-in —
// needed by humans and by Google's OAuth branding verification alike.
//
// Deliberately a static server component: no client JS, no interactivity
// beyond links, so an anonymous first paint stays cheap. Layout-only styles
// that need media queries live in globals.css under the `.lp-` prefix.
import { tokens } from "@openmind/ui";

export const metadata = {
  title: "Openmind — the self-hosted commonplace book",
  description:
    "Save links, notes, quotes, and images. Openmind enriches and organises them, so you can find anything by a fragment — a colour, a word, a vibe. Open source, self-hostable: Postgres plus one binary.",
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
  shot: string;
  alt: string;
  flip?: boolean;
}) {
  return (
    <section className={`lp-feature${flip ? " lp-feature-flip" : ""}`}>
      <div className="lp-feature-copy">
        <p className="meta" style={{ marginBottom: 10 }}>
          {eyebrow}
        </p>
        <h2
          className="serif"
          style={{
            fontSize: 27,
            fontWeight: 600,
            letterSpacing: "-.01em",
            margin: "0 0 12px",
          }}
        >
          {title}
        </h2>
        <p
          style={{
            fontSize: 15.5,
            lineHeight: 1.65,
            color: tokens.color.inkMuted,
            margin: 0,
          }}
        >
          {body}
        </p>
      </div>
      <img className="lp-shot" src={shot} alt={alt} loading="lazy" />
    </section>
  );
}

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
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            height: 62,
          }}
        >
          <Mark />
          <span
            className="serif"
            style={{ fontSize: 19, fontWeight: 600, letterSpacing: "-.01em" }}
          >
            Openmind
          </span>
          <span style={{ flex: 1 }} />
          <a
            href={REPO}
            className="meta"
            style={{ color: tokens.color.inkMuted, textDecoration: "none" }}
          >
            GitHub
          </a>
          <a
            href="/login"
            className="meta"
            style={{ color: tokens.color.cobalt, textDecoration: "none" }}
          >
            Sign in
          </a>
        </div>
      </header>

      <main>
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
          <div
            style={{
              display: "flex",
              gap: 12,
              flexWrap: "wrap",
              marginBottom: 54,
            }}
          >
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

        <div className="lp-wrap">
          <Feature
            eyebrow="Search"
            title="Find it by a fragment"
            body="You rarely remember the title. You remember it was blue, or about latency, or something a friend linked in March. Search fuses full-text with semantic similarity, and every save keeps its palette — so colour is a filter, not a decoration."
            shot="/marketing/search-colour.jpg"
            alt="Colour search: a cobalt swatch is selected and the library has filtered to items whose palette contains that blue"
          />

          <Feature
            eyebrow="Drift"
            title="The opposite of a backlog"
            body="Saved things shouldn't turn into a queue you owe. Drift resurfaces a few forgotten items a day, one at a time — keep it on your Desk, or let it go. No streaks, no unread count, no guilt."
            shot="/marketing/drift.jpg"
            alt="Drift mode: a single resurfaced card on a dark backdrop, with Let go and Keep on my desk actions"
            flip
          />

          <Feature
            eyebrow="Read"
            title="Without the web attached"
            body="Articles open in a clean reader. Highlight a passage and it becomes its own quote card, linked back to where it came from — so the line survives even if the page doesn't."
            shot="/marketing/reader.jpg"
            alt="The reader view showing an article with its summary, tags, highlighted passages and related items"
          />
        </div>

        <div className="lp-wrap" style={{ padding: "44px 24px 8px" }}>
          <div className="lp-grid3">
            <div className="lp-panel">
              <p className="meta" style={{ marginBottom: 8 }}>
                Capture
              </p>
              <p style={{ fontSize: 14.5, lineHeight: 1.6, margin: 0 }}>
                Web, a browser extension, your phone's share sheet, or a plain{" "}
                <code style={{ fontFamily: tokens.font.mono, fontSize: 13 }}>
                  POST
                </code>
                . Saves return instantly — enrichment always happens after.
              </p>
            </div>
            <div className="lp-panel">
              <p className="meta" style={{ marginBottom: 8 }}>
                Organise
              </p>
              <p style={{ fontSize: 14.5, lineHeight: 1.6, margin: 0 }}>
                No folders, no filing. Lenses are saved queries that stay live;
                the Desk holds what you're thinking about right now.
              </p>
            </div>
            <div className="lp-panel">
              <p className="meta" style={{ marginBottom: 8 }}>
                Own
              </p>
              <p style={{ fontSize: 14.5, lineHeight: 1.6, margin: 0 }}>
                Export everything as JSON whenever you like. AGPL-3.0, and it
                runs on hardware you control.
              </p>
            </div>
          </div>
        </div>

        <div className="lp-wrap" style={{ padding: "60px 24px 20px" }}>
          <p className="meta">Self-hosting is the product</p>
          <h2
            className="serif"
            style={{
              fontSize: 27,
              fontWeight: 600,
              letterSpacing: "-.01em",
              margin: "12px 0 14px",
            }}
          >
            Postgres, and one binary.
          </h2>
          <p
            style={{
              fontSize: 15.5,
              lineHeight: 1.65,
              color: tokens.color.inkMuted,
              maxWidth: "62ch",
              margin: "0 0 22px",
            }}
          >
            No Redis, no message broker, no Python sidecar — and there never will
            be. AI is pluggable and never assumed: the default provider does
            nothing at all, and the app stays fully usable with manual tags and
            full-text search. Point it at Gemini Flash-Lite, any
            OpenAI-compatible endpoint, or a local Ollama when you want more.
          </p>
          <pre className="lp-pre">
            <code>{`git clone ${REPO.replace("https://", "")}
cd open-mind && docker compose up -d
open http://localhost:3000`}</code>
          </pre>
        </div>

        <footer
          style={{
            borderTop: `1px solid ${tokens.color.hairline}`,
            marginTop: 50,
            padding: "26px 0 44px",
          }}
        >
          <div
            className="lp-wrap"
            style={{
              display: "flex",
              gap: 18,
              flexWrap: "wrap",
              alignItems: "center",
            }}
          >
            <Mark size={22} />
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
