"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";

/**
 * The sidebar account row in single-user token mode.
 *
 * There is no identity provider here, so there is no name or avatar to show —
 * saying "Self-hosted" is the honest answer rather than inventing a person.
 * Clerk's UserButton can't be used (no ClerkProvider in this mode), so this
 * supplies the one auth action that does exist: dropping the session cookie.
 */
export function TokenAccountRow({ usage }: { usage?: string }) {
  const router = useRouter();
  const [signingOut, setSigningOut] = useState(false);

  async function signOut() {
    setSigningOut(true);
    try {
      await fetch("/api/auth", { method: "DELETE" });
      // Replace rather than push so Back can't land on a now-unauthorised page.
      router.replace("/login");
      router.refresh();
    } catch {
      // The cookie may still be set; let the user retry rather than pretend.
      setSigningOut(false);
    }
  }

  return (
    <div style={row}>
      <div style={{ minWidth: 0, flex: 1 }}>
        <div style={nameStyle}>Self-hosted</div>
        <div className="meta" style={captionStyle}>
          {usage ?? "Single-user mode"}
        </div>
      </div>
      <button
        type="button"
        onClick={() => void signOut()}
        disabled={signingOut}
        title="Sign out"
        aria-label="Sign out"
        style={{
          ...signOutStyle,
          cursor: signingOut ? "default" : "pointer",
          opacity: signingOut ? 0.5 : 1,
        }}
      >
        {signingOut ? "…" : "Sign out"}
      </button>
    </div>
  );
}

const row = {
  marginTop: "auto",
  display: "flex",
  alignItems: "center",
  gap: 8,
  padding: 8,
  borderRadius: 11,
  border: `1px solid ${tokens.color.hairline}`,
  background: tokens.color.paper,
} as const;

const nameStyle = {
  fontFamily: tokens.font.sans,
  fontSize: 12.5,
  fontWeight: 600,
  lineHeight: 1.1,
} as const;

const captionStyle = {
  textTransform: "none",
  letterSpacing: ".02em",
  color: tokens.color.inkFaintAlt,
  marginTop: 3,
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
} as const;

const signOutStyle = {
  flex: "none",
  border: `1px solid ${tokens.color.hairline}`,
  background: tokens.color.surface,
  color: tokens.color.inkMuted,
  font: "500 9.5px/1 var(--font-jetbrains-mono), monospace",
  letterSpacing: ".06em",
  textTransform: "uppercase",
  padding: "6px 7px",
  borderRadius: 7,
} as const;
