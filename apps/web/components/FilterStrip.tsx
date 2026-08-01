import Link from "next/link";
import { tokens } from "@openmind/ui";

const { color } = tokens;

// Chip → cardType. "all" carries no `type` param (shows everything).
// The page maps `type` → API `types` and filters server-side.
const FILTERS: readonly { label: string; type: string }[] = [
  { label: "All", type: "all" },
  { label: "Articles", type: "article" },
  { label: "Images", type: "image" },
  { label: "Quotes", type: "quote" },
  { label: "Products", type: "product" },
  { label: "Video", type: "video" },
  { label: "Posts", type: "tweet" },
  { label: "Recipes", type: "recipe" },
  { label: "Notes", type: "note" },
  { label: "Books", type: "book" },
];

/**
 * Type filter strip. Chips are links to `?type=<cardType>` (preserving any
 * active q / colour / domains); the page forwards filters to the API.
 * The active chip is ink-filled.
 */
export function FilterStrip({
  active = "all",
  q,
  color: colorParam,
  domains,
}: {
  active?: string;
  q?: string;
  color?: string;
  domains?: string;
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
        if (domains) params.set("domains", domains);
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
