"use client";

import { tokens } from "@openmind/ui";
import { useEffect, useRef } from "react";

const { color, font } = tokens;

/** Visually hidden, still read aloud. */
const srOnly = {
  position: "absolute",
  width: 1,
  height: 1,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
} as const;

/**
 * The load-more affordance shared by the Mind and the Feed river.
 *
 * The button is the control and is always rendered; the IntersectionObserver
 * merely presses it early. Infinite scroll whose only trigger is a scroll event
 * is unreachable by keyboard and invisible to a screen reader.
 *
 * `announcement` is required rather than optional on purpose: a screen reader
 * gets no other signal that a page arrived, and making it mandatory means the
 * type checker asks every river for one instead of trusting us to remember.
 */
export function LoadMore({
  onLoad,
  loading,
  error,
  label,
  announcement,
}: {
  onLoad: () => void;
  loading: boolean;
  error: boolean;
  label: string;
  announcement: string;
}) {
  const button = useRef<HTMLButtonElement | null>(null);
  const onLoadRef = useRef(onLoad);
  onLoadRef.current = onLoad;

  useEffect(() => {
    // The button itself is the sentinel. A separate absolutely-positioned
    // marker relied on static-position resolution to land anywhere useful —
    // correct per spec, but if it ever stopped intersecting, auto-load would
    // silently degrade to button-only and nothing would report it.
    const node = button.current;
    // Never auto-load into a failure: after an error the reader presses Retry.
    if (!node || loading || error) return;
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) onLoadRef.current();
      },
      { rootMargin: "600px 0px" },
    );
    io.observe(node);
    return () => io.disconnect();
  }, [loading, error]);

  return (
    <div style={{ display: "flex", justifyContent: "center", padding: "28px 0 8px" }}>
      <p aria-live="polite" style={srOnly}>
        {announcement}
      </p>
      <button
        ref={button}
        type="button"
        onClick={onLoad}
        disabled={loading}
        style={{
          font: `500 11px/1 ${font.mono}`,
          letterSpacing: ".04em",
          color: error ? color.danger : color.inkFaint,
          background: "none",
          border: `1px solid ${error ? color.danger : color.hairline}`,
          borderRadius: 20,
          padding: "10px 18px",
          cursor: loading ? "default" : "pointer",
        }}
      >
        {error ? "Couldn't load more — retry" : loading ? "Loading…" : label}
      </button>
    </div>
  );
}
