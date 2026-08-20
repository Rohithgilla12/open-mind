import { tokens } from "@openmind/ui";

/**
 * The rule that separates library matches from unkept feed matches. Shared by
 * the server-rendered search and the live one so a query returns the same
 * shape either way.
 */
export function FeedDivider({ count }: { count: number }) {
  return (
    <div
      role="separator"
      aria-label="Matches from your feeds"
      style={{ display: "flex", alignItems: "center", gap: 12, margin: "30px 0 20px" }}
    >
      <span aria-hidden style={{ flex: 1, height: 1, background: tokens.color.hairline }} />
      <span className="meta">From your feeds · {count} — not yet in your Mind</span>
      <span aria-hidden style={{ flex: 1, height: 1, background: tokens.color.hairline }} />
    </div>
  );
}
