import Link from "next/link";
import { tokens } from "@openmind/ui";

const { color } = tokens;

// Chip → cardType. "all" carries no `type` param (shows everything).
const FILTERS: readonly { label: string; type: string }[] = [
  { label: "All", type: "all" },
  { label: "Articles", type: "article" },
  { label: "Images", type: "image" },
  { label: "Quotes", type: "quote" },
  { label: "Products", type: "product" },
  { label: "Video", type: "video" },
  { label: "Notes", type: "note" },
  { label: "Books", type: "book" },
];

/**
 * Type filter strip. Chips are links to `?type=<cardType>` (preserving any
 * active `q`); the page filters items server-side. The active chip is ink-filled.
 */
export function FilterStrip({
  active = "all",
  q,
  color: colorParam,
}: {
  active?: string;
  q?: string;
  color?: string;
}) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        flexWrap: "wrap",
        gap: 8,
        padding: "12px 28px",
        borderBottom: "1px solid rgba(28,26,22,.08)",
        background: color.header,
      }}
    >
      {FILTERS.map((f) => {
        const isActive = f.type === active;
        const params = new URLSearchParams();
        if (q) params.set("q", q);
        if (colorParam) params.set("color", colorParam);
        if (f.type !== "all") params.set("type", f.type);
        const qs = params.toString();
        return (
          <Link
            key={f.type}
            href={qs ? `/?${qs}` : "/"}
            className="chip"
            aria-current={isActive ? "page" : undefined}
            style={
              isActive
                ? { background: color.ink, color: color.paper, borderColor: color.ink }
                : undefined
            }
          >
            {f.label}
          </Link>
        );
      })}
      <span className="meta" style={{ marginLeft: "auto" }}>
        Sorted · recently saved
      </span>
    </div>
  );
}
