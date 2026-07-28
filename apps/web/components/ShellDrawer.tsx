"use client";
// Responsive wrapper for the Shell sidebar. Desktop (>900px) renders the
// aside statically, exactly as before. On narrow screens the aside becomes an
// off-canvas drawer: hidden by default, opened from a floating hamburger,
// closed by the overlay, the close button, or navigating (pathname change).
import { type ReactNode, useEffect, useState } from "react";
import { usePathname } from "next/navigation";
import { tokens } from "@openmind/ui";

export function ShellDrawer({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const pathname = usePathname();

  // Navigating (link tap inside the drawer) closes it.
  useEffect(() => {
    setOpen(false);
  }, [pathname]);

  return (
    <>
      <button
        type="button"
        className="shell-hamburger"
        aria-label={open ? "Close menu" : "Open menu"}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        style={{
          border: `1px solid ${tokens.color.hairline}`,
          background: tokens.color.cardSurface,
          color: tokens.color.ink,
          borderRadius: 10,
          width: 40,
          height: 40,
          fontSize: 18,
          lineHeight: 1,
          cursor: "pointer",
          boxShadow: "0 4px 14px -6px rgba(28,26,22,.4)",
        }}
      >
        {open ? "✕" : "☰"}
      </button>
      {/* Always mounted: an unmounted scrim can't fade out, and React removes
          the node before any exit transition could run. Closed state is inert
          via pointer-events:none in globals.css. */}
      <div
        className={`shell-overlay${open ? " shell-overlay-open" : ""}`}
        onClick={() => setOpen(false)}
        aria-hidden
      />
      <div className={`shell-aside${open ? " shell-aside-open" : ""}`}>{children}</div>
    </>
  );
}
