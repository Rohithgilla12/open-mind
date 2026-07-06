// Client-side mirror of the backend colour vocabulary (apps/api/internal/search/color.go).
// Used to render a swatch for a colour term the search understood or a colour
// the user is filtering by — the term may arrive as a hex string or a name.
// If this diverges from the Go map, the Go map wins (it does the actual ranking).

const NAMED_COLORS: Record<string, string> = {
  // Openmind accents.
  cobalt: "#1B3FD1",
  terracotta: "#C24A2E",
  gold: "#E0B23A",
  green: "#2E7D5B",
  // Common colours.
  red: "#D1291B",
  orange: "#E07B2A",
  yellow: "#E8C33A",
  lime: "#7DC24A",
  teal: "#2E7D7D",
  cyan: "#2AB6E0",
  blue: "#1B54D1",
  navy: "#16255C",
  indigo: "#3A2EE0",
  purple: "#7D2EC2",
  magenta: "#C22E9E",
  pink: "#E07BA6",
  brown: "#7D5A2E",
  beige: "#D8C9A8",
  cream: "#F4F0E6",
  black: "#1C1A16",
  white: "#FCFBF6",
  grey: "#8A857A",
  gray: "#8A857A",
};

/** Swatches offered as one-tap colour filters (order = display order). */
export const COLOR_SWATCHES: readonly string[] = [
  "cobalt",
  "terracotta",
  "gold",
  "green",
  "red",
  "blue",
  "purple",
  "black",
];

const HEX_RE = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i;

/** Expand a 3-digit hex to 6 digits and ensure a leading '#'. */
function normaliseHex(raw: string): string {
  let s = raw.trim().toLowerCase().replace(/^#/, "");
  if (s.length === 3) s = s.replace(/(.)/g, "$1$1");
  return `#${s}`;
}

/**
 * Resolve a colour term (hex or recognised name) to a CSS hex string, or null
 * if it is neither. Mirrors parseColor in the backend so the swatch we paint
 * matches the colour actually searched.
 */
export function resolveColor(term: string | undefined): string | null {
  if (!term) return null;
  const t = term.trim().toLowerCase();
  if (HEX_RE.test(t)) return normaliseHex(t);
  return NAMED_COLORS[t] ?? null;
}
