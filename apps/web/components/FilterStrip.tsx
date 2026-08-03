import Link from "next/link";
import { tokens } from "@openmind/ui";
import { typeFilters } from "../lib/cards";

const { color } = tokens;

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
      {typeFilters.map((f) => {
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
