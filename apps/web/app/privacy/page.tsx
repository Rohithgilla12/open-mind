// Public privacy policy — linked from Clerk's sign-up flow and OAuth consent.
// Exempted from the auth middleware in both modes.
export const metadata = { title: "Privacy — Openmind" };

const updated = "2026-07-09";

export default function PrivacyPage() {
  return (
    <main
      style={{
        maxWidth: 680,
        margin: "0 auto",
        padding: "56px 20px 96px",
        lineHeight: 1.7,
        fontSize: 15,
      }}
    >
      <p className="meta">Openmind · updated {updated}</p>
      <h1 className="serif" style={{ fontSize: 32, fontStyle: "italic", margin: "10px 0 24px" }}>
        Privacy policy
      </h1>

      <p>
        Openmind is a personal commonplace book: you save links, notes, and images, and the
        service organises them for you. This instance is operated privately; it is not an
        advertising business, and your data is never sold or shared with third parties for
        marketing.
      </p>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>What we store</h2>
      <ul>
        <li>
          <strong>Things you save</strong> — URLs, extracted article text, notes, uploaded
          images (with location and camera metadata stripped on upload), tags, and links
          between items. These are stored in this instance&apos;s database and belong to your
          account alone; other users can never see them.
        </li>
        <li>
          <strong>Account details</strong> — your email address and sign-in identity, handled
          by our authentication provider, Clerk (see{" "}
          <a href="https://clerk.com/legal/privacy">Clerk&apos;s privacy policy</a>). We store
          only the identifier Clerk gives us and your email.
        </li>
        <li>
          <strong>Device keys</strong> — when you connect the mobile app, browser extension,
          or desktop dock, each device gets its own key. We store a hash of the key, its
          name, and when it was last used; you can revoke any of them at any time from
          Settings → Devices.
        </li>
      </ul>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>How saved content is processed</h2>
      <p>
        To summarise, tag, and make your saves searchable, the text of items you save is sent
        to the AI provider this instance is configured with. Providers only receive the
        content needed for enrichment; they are not given your identity. Search and storage
        otherwise happen entirely on this instance&apos;s own server.
      </p>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>Your data, your controls</h2>
      <ul>
        <li>Export everything you&apos;ve saved as JSON at any time from the app.</li>
        <li>Delete any item permanently; deleting an item also removes its links and files.</li>
        <li>
          To delete your entire account and library, contact the operator (below) — removal is
          permanent and complete.
        </li>
      </ul>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>Cookies</h2>
      <p>
        We use only the session cookies required to keep you signed in. No analytics trackers,
        no advertising cookies.
      </p>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>Contact</h2>
      <p>
        This instance is operated by Rohith Gilla. Questions or deletion requests:{" "}
        <a href="mailto:gillarohith1@gmail.com">gillarohith1@gmail.com</a>.
      </p>
    </main>
  );
}
