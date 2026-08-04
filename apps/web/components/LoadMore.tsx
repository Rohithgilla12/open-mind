"use client";

import { tokens } from "@openmind/ui";
import { useEffect, useRef } from "react";

const { color, font } = tokens;

/**
 * The load-more affordance shared by the Mind and the Feed river.
 *
 * The button is the control and is always rendered; the IntersectionObserver
 * merely presses it early. Infinite scroll whose only trigger is a scroll event
 * is unreachable by keyboard and invisible to a screen reader.
 */
export function LoadMore({
  onLoad,
  loading,
  error,
  label,
}: {
  onLoad: () => void;
  loading: boolean;
  error: boolean;
  label: string;
}) {
  const sentinel = useRef<HTMLDivElement | null>(null);
  const onLoadRef = useRef(onLoad);
  onLoadRef.current = onLoad;

  useEffect(() => {
    const node = sentinel.current;
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
      <div ref={sentinel} aria-hidden style={{ position: "absolute", height: 1, width: 1 }} />
      <button
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
