import { tokens } from "@openmind/ui";
import { Grid } from "../../components/Grid";
import { Shell } from "../../components/Shell";
import { getDesk } from "../../lib/desk";

const { color, font } = tokens;

export default async function DeskPage() {
  const items = await getDesk();
  const count = items.length;
  const subline = `${count.toLocaleString("en-GB")} pinned · what you're working with`;

  return (
    <Shell activeDesk>
      {/* Gold 2px hairline — the Desk's signature top rule. */}
      <div
        style={{
          height: 2,
          background: `linear-gradient(90deg,${color.gold},${color.gold} 40%,transparent)`,
        }}
      />
      <div
        style={{
          padding: "18px 28px 16px",
          borderBottom: `1px solid ${color.hairline}`,
          background: color.header,
        }}
      >
        <h1
          className="serif"
          style={{ fontSize: 27, fontWeight: 600, letterSpacing: "-.02em", color: color.ink, margin: 0 }}
        >
          Your desk
        </h1>
        <div
          className="meta"
          style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt, marginTop: 6 }}
        >
          {subline}
        </div>
      </div>

      <div style={{ position: "relative", flex: 1 }}>
        <div className="paper-texture" style={{ position: "absolute", inset: 0, pointerEvents: "none" }} />
        <div style={{ position: "relative", padding: "22px 28px 40px" }}>
          {count === 0 ? (
            <p
              style={{
                fontFamily: font.quote,
                fontStyle: "italic",
                fontSize: "1.25rem",
                lineHeight: 1.5,
                color: color.inkMuted,
                maxWidth: "48ch",
                marginTop: "2rem",
              }}
            >
              Pin anything to keep it close — it&apos;ll wait here while the rest of your mind stays
              out of the way.
            </p>
          ) : (
            <Grid items={items} />
          )}
        </div>
      </div>
    </Shell>
  );
}
