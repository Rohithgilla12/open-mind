import type { CSSProperties } from "react";

/**
 * Palette renders a row of colour dots — the signature brand detail. Each hex in
 * `colors` becomes one `.dot` span (styled in globals.css). It renders bare spans
 * (a fragment) so it drops into any flex container. Pass `size` to scale the dots
 * up for the detail-page swatches; omit it for the default card dot size.
 */
export function Palette({ colors, size }: { colors: string[]; size?: number }) {
  const style = (c: string): CSSProperties =>
    size !== undefined
      ? { background: c, width: size, height: size, borderRadius: size / 2 }
      : { background: c };
  return (
    <>
      {colors.map((c, i) => (
        <span key={`${c}-${i}`} className="dot" style={style(c)} />
      ))}
    </>
  );
}
