import Link from "next/link";
import { tokens } from "@openmind/ui";
import { SearchBox } from "./SearchBox";

const { color, font } = tokens;

/**
 * Editorial topbar: terracotta hairline, title + live count subline, the search
 * pill (SearchBox), a "Save something" affordance that jumps to the capture row,
 * and an Export JSON link. Server component — the only interactive piece is the
 * client SearchBox; the Save button is an in-page anchor (no client needed).
 */
export function Topbar({ count, q }: { count: number; q?: string }) {
  const noun = count === 1 ? "gathering" : "gatherings";

  return (
    <>
      <div
        style={{
          height: 2,
          background: `linear-gradient(90deg,${color.terracotta},${color.terracotta} 40%,transparent)`,
        }}
      />
      <div
        style={{
          display: "flex",
          alignItems: "center",
          flexWrap: "wrap",
          gap: "12px 16px",
          padding: "18px 28px 16px",
          borderBottom: `1px solid ${color.hairline}`,
          background: color.header,
        }}
      >
        <div style={{ flex: "none" }}>
          <div
            className="serif"
            style={{
              fontSize: 27,
              fontWeight: 600,
              letterSpacing: "-.02em",
              lineHeight: 1,
              color: color.ink,
            }}
          >
            The Mind
          </div>
          <div
            className="meta"
            style={{
              marginTop: 5,
              textTransform: "none",
              letterSpacing: ".02em",
              color: color.inkFaintAlt,
            }}
          >
            {count.toLocaleString("en-GB")} {noun} · organised by the machine
          </div>
        </div>

        <SearchBox initial={q} />

        <a href="#capture" className="savebtn" style={{ flex: "none", textDecoration: "none" }}>
          Save something
        </a>

        <Link
          href="/feeds"
          style={{
            flex: "none",
            fontFamily: font.mono,
            fontSize: 11,
            letterSpacing: ".04em",
            color: color.cobalt,
            textDecoration: "none",
          }}
        >
          Feeds
        </Link>

        <Link
          href="/import"
          style={{
            flex: "none",
            fontFamily: font.mono,
            fontSize: 11,
            letterSpacing: ".04em",
            color: color.cobalt,
            textDecoration: "none",
          }}
        >
          Import
        </Link>

        <Link
          href="/settings/devices"
          style={{
            flex: "none",
            fontFamily: font.mono,
            fontSize: 11,
            letterSpacing: ".04em",
            color: color.cobalt,
            textDecoration: "none",
          }}
        >
          Devices
        </Link>

        <a
          href="/api/export"
          style={{
            flex: "none",
            fontFamily: font.mono,
            fontSize: 11,
            letterSpacing: ".04em",
            color: color.cobalt,
            textDecoration: "none",
          }}
        >
          Export JSON
        </a>
      </div>
    </>
  );
}
