// Public landing served at "/" for anonymous visitors (middleware rewrite).
// Exists so the homepage explains the product without requiring sign-in —
// needed by humans and by Google's OAuth branding verification alike.
export const metadata = {
  title: "Openmind — a home for everything worth keeping",
  description:
    "Save links, notes, and images. Openmind enriches and organises them, so you can find anything by a fragment — a colour, a word, a vibe.",
};

export default function WelcomePage() {
  return (
    <main
      style={{
        maxWidth: 680,
        margin: "0 auto",
        padding: "72px 20px 96px",
        lineHeight: 1.7,
        fontSize: 15.5,
      }}
    >
      <p className="meta">OPENMIND</p>
      <h1
        className="serif"
        style={{ fontSize: 40, fontStyle: "italic", lineHeight: 1.15, margin: "12px 0 20px" }}
      >
        A home for everything worth keeping.
      </h1>
      <p style={{ fontSize: 17, maxWidth: "56ch" }}>
        Openmind is a commonplace book for the internet age: save any link, note, or image in
        under a second, and it quietly enriches each one — a summary, tags, the colours of the
        page — so you can find it again by a fragment: a word, a hue, a half-remembered vibe.
      </p>

      <ul style={{ margin: "28px 0", paddingLeft: 20, display: "grid", gap: 10 }}>
        <li>
          <strong>Capture from anywhere</strong> — web, browser extension, iPhone share sheet,
          or a desktop hotkey.
        </li>
        <li>
          <strong>The machine organises</strong> — no folders, no filing; search finds things
          by meaning, tags, even colour.
        </li>
        <li>
          <strong>Yours, always</strong> — export everything as JSON any time; open-source and
          self-hostable.
        </li>
      </ul>

      <p>
        <a
          href="/login"
          style={{
            display: "inline-block",
            background: "#1B3FD1",
            color: "#F4F0E6",
            padding: "12px 26px",
            borderRadius: 8,
            fontWeight: 600,
            textDecoration: "none",
          }}
        >
          Sign in
        </a>
      </p>

      <p className="meta" style={{ marginTop: 48 }}>
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
