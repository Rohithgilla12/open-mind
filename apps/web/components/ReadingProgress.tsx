"use client";

import { tokens } from "@openmind/ui";
import { useEffect, useRef } from "react";

/**
 * Thin terracotta reading-progress bar pinned to the top of the viewport.
 * Driven by a transform (never layout) and throttled to one update per frame,
 * so scrolling a long read stays cheap. Purely decorative — hidden from the
 * accessibility tree.
 */
export function ReadingProgress() {
  const barRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let frame = 0;
    const update = () => {
      frame = 0;
      const bar = barRef.current;
      if (!bar) return;
      const doc = document.documentElement;
      const max = doc.scrollHeight - window.innerHeight;
      const progress = max > 0 ? Math.min(1, Math.max(0, window.scrollY / max)) : 0;
      bar.style.transform = `scaleX(${progress})`;
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    update();
    window.addEventListener("scroll", schedule, { passive: true });
    window.addEventListener("resize", schedule);
    return () => {
      if (frame) cancelAnimationFrame(frame);
      window.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", schedule);
    };
  }, []);

  return (
    <div
      ref={barRef}
      aria-hidden
      style={{
        position: "fixed",
        top: 0,
        left: 0,
        right: 0,
        height: 2,
        background: tokens.color.terracotta,
        transform: "scaleX(0)",
        transformOrigin: "0 50%",
        zIndex: 20,
        pointerEvents: "none",
      }}
    />
  );
}
