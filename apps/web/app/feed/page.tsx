import { tokens } from "@openmind/ui";
import { Shell } from "../../components/Shell";
import { FeedRiver } from "../../components/FeedRiver";
import { getFeeds } from "../../lib/feeds";

const { color } = tokens;

export default async function FeedPage() {
  // Fetched here rather than in the client: the subscription list sizes the
  // filter strip and names every row's source, so fetching it after mount made
  // the strip appear late and shove the river down mid-read.
  const feeds = await getFeeds();

  return (
    <Shell activeFeed>
      {/* Terracotta 2px hairline — mirrors the Desk's gold rule, this river's own signature. */}
      <div
        style={{
          height: 2,
          background: `linear-gradient(90deg,${color.terracotta},${color.terracotta} 40%,transparent)`,
        }}
      />
      {/* Same masthead rhythm as the Desk and Places. This block used to reach
          for a `var(--gutter)` that is defined nowhere — an undefined custom
          property invalidates the whole `padding` shorthand, so the header and
          the river below it were rendering with no padding at all. */}
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
          Feed
        </h1>
        <div
          className="meta"
          style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt, marginTop: 6 }}
        >
          Reverse-chron river of everything your subscriptions brought in
        </div>
      </div>

      {/* The per-feed filter and the river share client state, so FeedRiver owns
          both bands: the filter strip sits in the page chrome flush under the
          header (as the Mind's FilterStrip does), the river on the paper below. */}
      <FeedRiver feeds={feeds} />
    </Shell>
  );
}
