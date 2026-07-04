import type { ReactNode } from "react";
import { tokens } from "@openmind/ui";

const navBase = {
  display: "flex",
  alignItems: "center",
  gap: 10,
  padding: "7px 10px",
  borderRadius: 8,
  fontFamily: tokens.font.sans,
  fontSize: 13,
  fontWeight: 500,
  lineHeight: 1,
} as const;

const softDivider = { height: 1, background: "rgba(28,26,22,.09)" } as const;

// A small "soon" badge sits beside features that aren't built yet, so the nav
// reads as a roadmap rather than a set of dead links.
function SoonTag() {
  return (
    <span
      style={{
        marginLeft: "auto",
        fontFamily: tokens.font.mono,
        fontSize: 8.5,
        fontWeight: 500,
        lineHeight: 1,
        letterSpacing: ".08em",
        textTransform: "uppercase",
        color: tokens.color.inkFaint,
        border: `1px solid ${tokens.color.hairline}`,
        borderRadius: 5,
        padding: "3px 5px",
      }}
    >
      soon
    </span>
  );
}

// Not-yet-built nav rows: muted, non-interactive, no link — never a dead click.
// Rows with a `count` (lenses) show it in the `.navk` mono style instead of
// the "soon" badge, matching the mockup; both are equally non-clickable.
function MutedNav({
  glyph,
  label,
  dot,
  count,
  trailing = true,
}: {
  glyph?: string;
  label: string;
  dot?: string;
  count?: string;
  trailing?: boolean;
}) {
  return (
    <div style={{ ...navBase, color: tokens.color.inkFaint, cursor: "default" }}>
      {dot ? (
        <span className="dot" style={{ background: dot }} />
      ) : (
        <span style={{ fontSize: 15, width: 16 }}>{glyph}</span>
      )}
      {label}
      {count ? (
        <span
          style={{
            marginLeft: "auto",
            fontFamily: tokens.font.mono,
            fontSize: 10,
            fontWeight: 500,
            lineHeight: 1,
            color: tokens.color.inkFaint,
          }}
        >
          {count}
        </span>
      ) : trailing ? (
        <SoonTag />
      ) : null}
    </div>
  );
}

export function Shell({ children }: { children: ReactNode }) {
  return (
    <div style={{ display: "flex", height: "100vh", overflow: "hidden" }}>
      <aside
        style={{
          width: 230,
          flex: "none",
          borderRight: `1px solid ${tokens.color.hairline}`,
          background: tokens.color.panel,
          display: "flex",
          flexDirection: "column",
          padding: "22px 16px",
        }}
      >
        {/* Wordmark + 3-line cobalt logo mark */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 9,
            padding: "0 6px 24px",
          }}
        >
          <div
            style={{
              width: 24,
              height: 24,
              borderRadius: 6,
              background: tokens.color.cobalt,
              position: "relative",
              flex: "none",
            }}
          >
            <div
              style={{
                position: "absolute",
                inset: "6px 6px auto 6px",
                height: 2,
                background: tokens.color.paper,
              }}
            />
            <div
              style={{
                position: "absolute",
                inset: "11px 6px auto 6px",
                height: 2,
                background: "rgba(244,240,230,.6)",
              }}
            />
            <div
              style={{
                position: "absolute",
                inset: "16px 9px auto 6px",
                height: 2,
                background: "rgba(244,240,230,.4)",
              }}
            />
          </div>
          <span
            className="serif"
            style={{ fontSize: 20, fontWeight: 600, letterSpacing: "-.01em" }}
          >
            Openmind
          </span>
        </div>

        <MutedNav glyph="◵" label="Desk" />

        {/* The Mind — the one live screen, shown active */}
        <div
          style={{
            ...navBase,
            background: "rgba(27,63,209,.1)",
            color: tokens.color.cobalt,
          }}
        >
          <span style={{ fontSize: 15, width: 16 }}>◧</span> The Mind
          <span
            style={{
              marginLeft: "auto",
              fontFamily: tokens.font.mono,
              fontSize: 12,
              fontWeight: 500,
              lineHeight: 1,
              color: tokens.color.inkFaint,
            }}
          >
            1,284
          </span>
        </div>

        <MutedNav glyph="❍" label="Drift" />

        <div style={{ ...softDivider, margin: "16px 8px" }} />

        <div
          className="meta"
          style={{ display: "flex", alignItems: "center", padding: "2px 10px 8px" }}
        >
          Lenses
          <SoonTag />
        </div>
        <MutedNav dot={tokens.color.cobalt} label="Design inspiration" count="214" />
        <MutedNav dot={tokens.color.terracotta} label="Distributed systems" count="88" />
        <MutedNav dot={tokens.color.green} label="Running & gear" count="37" />
        <MutedNav dot="#8A7A3A" label="Books to read" count="52" />
        <MutedNav glyph="+" label="New lens" trailing={false} />

        {/* Account row */}
        <div
          style={{
            marginTop: "auto",
            display: "flex",
            alignItems: "center",
            gap: 10,
            padding: 8,
            borderRadius: 11,
            border: `1px solid ${tokens.color.hairline}`,
            background: tokens.color.paper,
          }}
        >
          <div
            style={{
              width: 30,
              height: 30,
              borderRadius: "50%",
              background: `linear-gradient(135deg,${tokens.color.cobalt},${tokens.color.green})`,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: tokens.color.paper,
              fontFamily: tokens.font.sans,
              fontSize: 13,
              fontWeight: 600,
              lineHeight: 1,
              flex: "none",
            }}
          >
            R
          </div>
          <div style={{ minWidth: 0, flex: 1 }}>
            <div
              style={{
                fontFamily: tokens.font.sans,
                fontSize: 12.5,
                fontWeight: 600,
                lineHeight: 1.1,
                whiteSpace: "nowrap",
                overflow: "hidden",
                textOverflow: "ellipsis",
              }}
            >
              Rohith Gilla
            </div>
            <div
              className="meta"
              style={{
                textTransform: "none",
                letterSpacing: ".02em",
                color: tokens.color.inkFaintAlt,
                marginTop: 3,
              }}
            >
              Owner · signed in
            </div>
          </div>
        </div>

        {/* Storage meter — static; local, self-hosted */}
        <div
          style={{
            padding: "14px 10px 2px",
            marginTop: 12,
            borderTop: "1px solid rgba(28,26,22,.09)",
          }}
        >
          <div className="meta" style={{ marginBottom: 7 }}>
            Local · self-hosted
          </div>
          <div
            style={{
              height: 5,
              borderRadius: 3,
              background: "rgba(28,26,22,.1)",
              overflow: "hidden",
            }}
          >
            <div
              style={{ width: "34%", height: "100%", background: tokens.color.green }}
            />
          </div>
          <div
            className="meta"
            style={{
              marginTop: 6,
              textTransform: "none",
              letterSpacing: ".02em",
              color: tokens.color.inkFaintAlt,
            }}
          >
            3.1 GB / 9 GB archived
          </div>
        </div>
      </aside>

      {/* Fluid main column */}
      <div
        style={{
          flex: 1,
          display: "flex",
          flexDirection: "column",
          minWidth: 0,
          overflow: "auto",
          background: tokens.color.paper,
        }}
      >
        {children}
      </div>
    </div>
  );
}
