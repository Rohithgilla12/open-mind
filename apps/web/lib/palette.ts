/**
 * derivedPalette returns a small, deterministic set of hex colours from a seed
 * string (typically an item's title + tags).
 *
 * It is a DOCUMENTED PLACEHOLDER, used only when the server has not extracted a
 * real palette (`item.palette` is empty) — e.g. text-only cards, or items whose
 * lead image is an external URL whose bytes the pipeline does not fetch. The same
 * seed always yields the same colours, so a card's dots stay stable across
 * renders. Colours are generated in HSL within a constrained saturation and
 * lightness band so they harmonise with the warm editorial theme.
 */
export function derivedPalette(seed: string): string[] {
  const h = hash(seed);
  const count = 2 + (h % 2); // 2 or 3 dots
  const base = h % 360;
  const out: string[] = [];
  for (let i = 0; i < count; i++) {
    const hue = (base + i * 47) % 360;
    const sat = 42 + ((h >>> (i + 3)) % 18); // 42–59%
    const light = 52 + ((h >>> (i + 5)) % 12); // 52–63%
    out.push(hslToHex(hue, sat, light));
  }
  return out;
}

// hash is a 32-bit FNV-1a over the seed, returned unsigned.
function hash(s: string): number {
  let h = 2166136261 >>> 0;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619) >>> 0;
  }
  return h >>> 0;
}

// hslToHex converts an HSL colour (h in [0,360), s/l in [0,100]) to "#rrggbb".
function hslToHex(h: number, s: number, l: number): string {
  const sn = s / 100;
  const ln = l / 100;
  const c = (1 - Math.abs(2 * ln - 1)) * sn;
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
  const m = ln - c / 2;
  let r = 0;
  let g = 0;
  let b = 0;
  if (h < 60) [r, g, b] = [c, x, 0];
  else if (h < 120) [r, g, b] = [x, c, 0];
  else if (h < 180) [r, g, b] = [0, c, x];
  else if (h < 240) [r, g, b] = [0, x, c];
  else if (h < 300) [r, g, b] = [x, 0, c];
  else [r, g, b] = [c, 0, x];
  const to = (v: number) =>
    Math.round((v + m) * 255)
      .toString(16)
      .padStart(2, "0");
  return `#${to(r)}${to(g)}${to(b)}`;
}
