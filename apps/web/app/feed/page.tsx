import { tokens } from "@openmind/ui";
import { Shell } from "../../components/Shell";
import { FeedRiver } from "../../components/FeedRiver";

const { color } = tokens;

export default async function FeedPage() {
  return (
    <Shell activeFeed>
      {/* Terracotta 2px hairline — mirrors the Desk's gold rule, this river's own signature. */}
      <div
        style={{
          height: 2,
          background: `linear-gradient(90deg,${color.terracotta},${color.terracotta} 40%,transparent)`,
        }}
      />
      <div
        style={{
          padding: "26px var(--gutter) 20px",
          borderBottom: `1px solid ${color.hairline}`,
          background: color.header,
        }}
      >
        <h1
          className="serif"
          style={{ fontSize: 27, fontWeight: 600, letterSpacing: "-.02em", color: color.ink, margin: 0 }}
        >
          Feed
        </h1>
        <div
          className="meta"
          style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt, marginTop: 8 }}
        >
          Reverse-chron river of everything your subscriptions brought in
        </div>
      </div>

      <div style={{ position: "relative", flex: 1 }}>
        <div className="paper-texture" style={{ position: "absolute", inset: 0, pointerEvents: "none" }} />
        <div style={{ position: "relative", padding: "28px var(--gutter) 48px" }}>
          <FeedRiver />
        </div>
      </div>
    </Shell>
  );
}
