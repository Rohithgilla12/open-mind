import type { CSSProperties } from "react";
import { tokens } from "@openmind/ui";

/** Three-line cobalt mark from the web sidebar wordmark. */
export function LogoMark({ size = 24, style }: { size?: number; style?: CSSProperties }) {
  const radius = Math.max(4, Math.round(size * 0.25));
  return (
    <div
      aria-hidden
      style={{
        width: size,
        height: size,
        borderRadius: radius,
        background: tokens.color.cobalt,
        position: "relative",
        flex: "none",
        ...style,
      }}
    >
      <div
        style={{
          position: "absolute",
          inset: `${Math.round(size * 0.25)}px ${Math.round(size * 0.25)}px auto ${Math.round(size * 0.25)}px`,
          height: 2,
          background: tokens.color.paper,
        }}
      />
      <div
        style={{
          position: "absolute",
          inset: `${Math.round(size * 0.46)}px ${Math.round(size * 0.25)}px auto ${Math.round(size * 0.25)}px`,
          height: 2,
          background: "rgba(244,240,230,.6)",
        }}
      />
      <div
        style={{
          position: "absolute",
          inset: `${Math.round(size * 0.67)}px ${Math.round(size * 0.38)}px auto ${Math.round(size * 0.25)}px`,
          height: 2,
          background: "rgba(244,240,230,.4)",
        }}
      />
    </div>
  );
}
