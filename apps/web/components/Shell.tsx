import type { ReactNode } from "react";
import Link from "next/link";
import { tokens } from "@openmind/ui";
import { getLenses } from "../lib/lenses";
import { lensDot } from "../lib/lens-format";
import { formatBytes, getAccount } from "../lib/account";
import { authMode } from "../lib/auth-mode";
import { ClerkAccountRow } from "./ClerkAccountRow";
import { TokenAccountRow } from "./TokenAccountRow";
import { ShellDrawer } from "./ShellDrawer";

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

export async function Shell({
  children,
  activeLensId,
  activeDesk,
  activeDrift,
  activeFeed,
  activePlaces,
}: {
  children: ReactNode;
  activeLensId?: string;
  activeDesk?: boolean;
  activeDrift?: boolean;
  activeFeed?: boolean;
  activePlaces?: boolean;
}) {
  // Both are independent and neither throws, so fetch them concurrently rather
  // than making the sidebar wait on two sequential round trips.
  const [lenses, account] = await Promise.all([getLenses(), getAccount()]);
  const mindActive = !activeLensId && !activeDesk && !activeDrift && !activeFeed && !activePlaces;
  return (
    <div style={{ display: "flex", height: "100vh", overflow: "hidden" }}>
      <ShellDrawer>
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

        {/* The Mind — the home library. Active unless viewing a lens, feed, drift or the desk. */}
        <Link
          href="/"
          style={{
            ...navBase,
            textDecoration: "none",
            background: mindActive ? "rgba(27,63,209,.1)" : "transparent",
            color: mindActive ? tokens.color.cobalt : tokens.color.ink,
          }}
        >
          <span style={{ fontSize: 15, width: 16 }}>◧</span> The Mind
        </Link>

        {/* Feed — the reverse-chron river of everything your subscriptions brought in. */}
        <Link
          href="/feed"
          style={{
            ...navBase,
            textDecoration: "none",
            background: activeFeed ? "rgba(27,63,209,.1)" : "transparent",
            color: activeFeed ? tokens.color.cobalt : tokens.color.ink,
          }}
        >
          <span style={{ fontSize: 15, width: 16 }}>≋</span> Feed
        </Link>

        {/* Desk — the pinboard of what you're working with. */}
        <Link
          href="/desk"
          style={{
            ...navBase,
            textDecoration: "none",
            background: activeDesk ? "rgba(27,63,209,.1)" : "transparent",
            color: activeDesk ? tokens.color.cobalt : tokens.color.ink,
          }}
        >
          <span style={{ fontSize: 15, width: 16 }}>◵</span> Desk
        </Link>

        {/* Drift — calm, finite resurfacing of forgotten saves. */}
        <Link
          href="/drift"
          style={{
            ...navBase,
            textDecoration: "none",
            background: activeDrift ? "rgba(27,63,209,.1)" : "transparent",
            color: activeDrift ? tokens.color.cobalt : tokens.color.ink,
          }}
        >
          <span style={{ fontSize: 15, width: 16 }}>❍</span> Drift
        </Link>

        {/* Places — every spot your saves mention, pinned on a map. */}
        <Link
          href="/places"
          style={{
            ...navBase,
            textDecoration: "none",
            background: activePlaces ? "rgba(27,63,209,.1)" : "transparent",
            color: activePlaces ? tokens.color.cobalt : tokens.color.ink,
          }}
        >
          <span style={{ fontSize: 15, width: 16 }}>⌖</span> Places
        </Link>

        <div style={{ ...softDivider, margin: "16px 8px" }} />

        <div
          className="meta"
          style={{ display: "flex", alignItems: "center", padding: "2px 10px 8px" }}
        >
          Lenses
        </div>
        {lenses.map((lens) => {
          const active = lens.id === activeLensId;
          return (
            <Link
              key={lens.id}
              href={`/lens/${lens.id}`}
              title={lens.name}
              style={{
                ...navBase,
                textDecoration: "none",
                background: active ? "rgba(27,63,209,.1)" : "transparent",
                color: active ? tokens.color.cobalt : tokens.color.ink,
              }}
            >
              <span className="dot" style={{ background: lensDot(lens.rule, tokens.color.cobalt) }} />
              <span
                style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}
              >
                {lens.name}
              </span>
            </Link>
          );
        })}
        <Link href="/lens/new" style={{ ...navBase, textDecoration: "none", color: tokens.color.inkMuted }}>
          <span style={{ fontSize: 15, width: 16 }}>+</span> New lens
        </Link>

        {/* Account row. Clerk mode delegates the avatar and every account
            action to Clerk's UserButton; token mode has no identity provider,
            so it shows a neutral label plus a cookie sign-out. Neither invents
            a name — the old hardcoded one shipped to every self-hoster. */}
        {authMode === "clerk" ? <ClerkAccountRow /> : <TokenAccountRow />}

        {/* Real usage, not a meter. A progress bar needs a denominator, and
            self-hosting has no storage quota — the old fixed 34% bar against a
            made-up 9 GB ceiling was showing every self-hoster invented numbers.
            Counts come from GET /account; omitted entirely if it's unreachable
            rather than falling back to placeholders. */}
        {account && (
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
              className="meta"
              style={{
                textTransform: "none",
                letterSpacing: ".02em",
                color: tokens.color.inkFaintAlt,
              }}
            >
              {account.itemCount.toLocaleString("en-GB")}
              {account.itemCount === 1 ? " item" : " items"}
              {account.assetBytes > 0 && ` · ${formatBytes(account.assetBytes)}`}
            </div>
          </div>
        )}
      </aside>
      </ShellDrawer>

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
