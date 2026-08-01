import Link from "next/link";
import { tokens } from "@openmind/ui";
import { Shell } from "../../components/Shell";
import { ImportForm } from "../../components/ImportForm";
import { RaindropImportForm } from "../../components/RaindropImportForm";

const { color, font } = tokens;

const SOURCES = [
  "Browser bookmarks (Chrome, Firefox, Safari) — export as HTML",
  "Pocket — HTML or CSV export",
  "Raindrop.io — connect directly with a test token below (no export file needed), or upload an HTML/CSV export; collections become tags either way",
  "Pinboard / Instapaper — HTML export",
  "Omnivore — zip export (labels become your tags; archived article bodies aren't used yet, so dead links import as failed cards)",
  "A plain .txt list of URLs, one per line",
  "PDF (drop it on the home-page capture box)",
];

export default function ImportPage() {
  return (
    <Shell>
      <div
        style={{
          height: 2,
          background: `linear-gradient(90deg,${color.terracotta},${color.terracotta} 40%,transparent)`,
        }}
      />
      <div style={{ padding: "24px 28px", borderBottom: `1px solid ${color.hairline}`, background: color.header }}>
        <Link
          href="/"
          className="meta"
          style={{ textTransform: "none", letterSpacing: ".02em", color: color.cobalt, textDecoration: "none" }}
        >
          ← The Mind
        </Link>
        <h1 className="serif" style={{ fontSize: 27, fontWeight: 600, letterSpacing: "-.02em", margin: "10px 0 4px", color: color.ink }}>
          Import
        </h1>
        <p className="meta" style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt }}>
          Bring your saves in from anywhere. Each link becomes a card and enriches itself — re-importing is safe.
        </p>
      </div>
      <div style={{ padding: "26px 28px 40px", display: "flex", flexDirection: "column", gap: 26 }}>
        <ImportForm />
        <RaindropImportForm />
        <div>
          <div className="meta" style={{ marginBottom: 10 }}>
            Supported sources
          </div>
          <ul style={{ margin: 0, paddingLeft: 18, display: "flex", flexDirection: "column", gap: 6 }}>
            {SOURCES.map((s) => (
              <li key={s} style={{ fontFamily: font.sans, fontSize: 13.5, lineHeight: 1.5, color: color.inkMuted }}>
                {s}
              </li>
            ))}
          </ul>
        </div>
      </div>
    </Shell>
  );
}
