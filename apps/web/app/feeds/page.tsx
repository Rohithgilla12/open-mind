import Link from "next/link";
import { tokens } from "@openmind/ui";
import { getFeeds } from "../../lib/feeds";
import { Shell } from "../../components/Shell";
import { AddFeed } from "./AddFeed";
import { DeleteFeedButton } from "./DeleteFeedButton";

const { color, font } = tokens;

function hostOf(u: string): string {
  try {
    return new URL(u).host.replace(/^www\./, "");
  } catch {
    return u;
  }
}

function relativeTime(iso?: string): string {
  if (!iso) return "never polled";
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "never polled";
  const secs = Math.round((Date.now() - then) / 1000);
  if (secs < 45) return "polled just now";
  const mins = Math.round(secs / 60);
  if (mins < 60) return `polled ${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `polled ${hrs}h ago`;
  const days = Math.round(hrs / 24);
  if (days < 30) return `polled ${days}d ago`;
  const months = Math.round(days / 30);
  return months < 12 ? `polled ${months}mo ago` : `polled ${Math.round(months / 12)}y ago`;
}

function StatusPill({ status }: { status: string }) {
  const isError = status.startsWith("error");
  const isPending = !isError && status.trim() === "";
  const label = isError ? status.replace(/^error:?\s*/, "").trim() || "error" : "ok";
  const tone = isError ? color.danger : isPending ? color.inkFaint : color.green;
  const text = isError ? `error — ${label}` : isPending ? "pending" : "ok";
  return (
    <span
      className="meta"
      title={isError ? status : undefined}
      style={{
        display: "inline-block",
        maxWidth: 260,
        overflow: "hidden",
        textOverflow: "ellipsis",
        whiteSpace: "nowrap",
        color: tone,
        background: `color-mix(in srgb, ${tone} 12%, transparent)`,
        border: `1px solid color-mix(in srgb, ${tone} 32%, transparent)`,
        borderRadius: 999,
        padding: "3px 9px",
      }}
    >
      {text}
    </span>
  );
}

export default async function FeedsPage() {
  const feeds = await getFeeds();

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
        <h1
          className="serif"
          style={{ fontSize: 27, fontWeight: 600, letterSpacing: "-.02em", margin: "10px 0 4px", color: color.ink }}
        >
          Feeds
        </h1>
        <p className="meta" style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt }}>
          Subscribe to a blog or news feed — new posts are saved and enriched automatically.
        </p>
      </div>

      <div style={{ padding: "26px 28px 40px", display: "flex", flexDirection: "column", gap: 28 }}>
        <AddFeed />

        {feeds.length === 0 ? (
          <div
            style={{
              maxWidth: 560,
              padding: "26px 22px",
              background: color.cardSurface,
              border: `1.5px dashed ${color.hairline}`,
              borderRadius: 12,
            }}
          >
            <div className="serif" style={{ fontSize: 18, fontWeight: 600, color: color.ink, marginBottom: 8 }}>
              No feeds yet
            </div>
            <p style={{ fontFamily: font.sans, fontSize: 13.5, lineHeight: 1.55, color: color.inkMuted, margin: 0 }}>
              Paste a feed URL above to subscribe. Openmind checks it regularly and turns each new post into a card that
              enriches itself — no reading list to babysit.
            </p>
          </div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 10, maxWidth: 720 }}>
            {feeds.map((feed) => {
              const host = hostOf(feed.siteUrl || feed.url);
              const displayTitle = feed.title?.trim() || host;
              return (
                <li
                  key={feed.id}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 16,
                    padding: "14px 18px",
                    background: color.cardSurface,
                    border: `1px solid ${color.hairline}`,
                    borderRadius: 12,
                  }}
                >
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div
                      className="serif"
                      style={{
                        fontSize: 16,
                        fontWeight: 600,
                        color: color.ink,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {displayTitle}
                    </div>
                    <div
                      className="meta"
                      style={{
                        marginTop: 4,
                        textTransform: "none",
                        letterSpacing: ".02em",
                        color: color.inkFaintAlt,
                        display: "flex",
                        gap: 10,
                        flexWrap: "wrap",
                      }}
                    >
                      <span>{host}</span>
                      <span aria-hidden>·</span>
                      <span>{relativeTime(feed.lastPolledAt)}</span>
                    </div>
                  </div>
                  <StatusPill status={feed.lastStatus ?? ""} />
                  <DeleteFeedButton id={feed.id} title={displayTitle} />
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </Shell>
  );
}
