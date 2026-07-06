import Link from "next/link";
import { notFound } from "next/navigation";
import { tokens } from "@openmind/ui";
import { Shell } from "../../../components/Shell";
import { Grid } from "../../../components/Grid";
import { DeleteLensButton } from "../../../components/DeleteLensButton";
import { KindleButton } from "../../../components/KindleButton";
import { getLens, getLensItems } from "../../../lib/lenses";
import { lensDot, lensSummary } from "../../../lib/lens-format";

const { color, font } = tokens;

export default async function LensPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const lens = await getLens(id);
  if (!lens) notFound();

  const items = await getLensItems(id);
  const summary = lensSummary(lens.rule);
  const count = items.length;
  const noun = count === 1 ? "gathering" : "gatherings";

  return (
    <Shell activeLensId={id}>
      <div
        style={{
          height: 2,
          background: `linear-gradient(90deg,${color.terracotta},${color.terracotta} 40%,transparent)`,
        }}
      />
      <div
        style={{
          display: "flex",
          alignItems: "flex-start",
          flexWrap: "wrap",
          gap: "10px 16px",
          padding: "18px 28px 16px",
          borderBottom: `1px solid ${color.hairline}`,
          background: color.header,
        }}
      >
        <div style={{ flex: "1 1 auto", minWidth: 0 }}>
          <Link
            href="/"
            className="meta"
            style={{ textTransform: "none", letterSpacing: ".02em", color: color.cobalt, textDecoration: "none" }}
          >
            ← The Mind
          </Link>
          <div style={{ display: "flex", alignItems: "center", gap: 10, margin: "9px 0 4px" }}>
            <span className="dot" style={{ background: lensDot(lens.rule, color.cobalt), width: 11, height: 11 }} />
            <h1 className="serif" style={{ fontSize: 27, fontWeight: 600, letterSpacing: "-.02em", color: color.ink, margin: 0 }}>
              {lens.name}
            </h1>
          </div>
          <div className="meta" style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt }}>
            {count.toLocaleString("en-GB")} {noun}
            {summary ? ` · ${summary}` : ""} · live view
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 16, flex: "none", paddingTop: 4 }}>
          <Link
            href={{
              pathname: "/lens/new",
              query: {
                ...(lens.rule?.q ? { q: lens.rule.q } : {}),
                ...(lens.rule?.color ? { color: lens.rule.color } : {}),
                ...(lens.rule?.types?.length ? { types: lens.rule.types.join(",") } : {}),
              },
            }}
            style={{ fontFamily: font.mono, fontSize: 11, letterSpacing: ".04em", color: color.cobalt, textDecoration: "none" }}
          >
            duplicate
          </Link>
          <KindleButton target="lens" id={id} />
          <DeleteLensButton id={id} name={lens.name} />
        </div>
      </div>

      <div style={{ position: "relative", flex: 1 }}>
        <div className="paper-texture" style={{ position: "absolute", inset: 0, pointerEvents: "none" }} />
        <div style={{ position: "relative", padding: "22px 28px 40px" }}>
          <Grid items={items} />
        </div>
      </div>
    </Shell>
  );
}
