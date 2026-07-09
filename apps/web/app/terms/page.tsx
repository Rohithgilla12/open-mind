// Public terms of service — linked from Clerk's sign-up flow.
// Exempted from the auth middleware in both modes.
export const metadata = { title: "Terms — Openmind" };

const updated = "2026-07-09";

export default function TermsPage() {
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
        Terms of service
      </h1>

      <p>
        Openmind is a personal commonplace book for saving and finding links, notes, and
        images. This instance is a privately operated service; by creating an account you
        agree to these terms.
      </p>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>Your content</h2>
      <p>
        Everything you save remains yours. We claim no ownership or licence over it beyond
        what is technically required to store, enrich, and show it back to you. You can
        export your full library as JSON or delete items — or your whole account — at any
        time (see the <a href="/privacy">privacy policy</a>).
      </p>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>Acceptable use</h2>
      <ul>
        <li>Don&apos;t save or distribute content that is illegal to possess or share.</li>
        <li>
          Don&apos;t abuse the service — no attempts to access other users&apos; data,
          circumvent rate limits, or disrupt the instance.
        </li>
        <li>Keep your device keys private; revoke any key you no longer use.</li>
      </ul>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>Availability</h2>
      <p>
        This is a personal project provided <strong>as-is</strong>, with no uptime guarantee
        and no warranty of any kind. Back up anything you can&apos;t afford to lose using the
        export feature. The service, or your access to it, may be changed, suspended, or
        discontinued — for accounts violating these terms, without notice; otherwise with
        reasonable notice and time to export your data.
      </p>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>Liability</h2>
      <p>
        To the maximum extent permitted by law, the operator is not liable for any indirect,
        incidental, or consequential damages arising from the use of, or inability to use,
        this service.
      </p>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>Changes</h2>
      <p>
        These terms may be updated; the date above reflects the latest revision. Continued
        use after a change constitutes acceptance.
      </p>

      <h2 style={{ fontSize: 20, marginTop: 32 }}>Contact</h2>
      <p>
        This instance is operated by Rohith Gilla:{" "}
        <a href="mailto:gillarohith1@gmail.com">gillarohith1@gmail.com</a>.
      </p>
    </main>
  );
}
