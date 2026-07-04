// Canonical warm palette — mirrors docs/design/README.md (Design Tokens).
// If these diverge from the design docs, the docs win.
export const tokens = {
  color: {
    canvas: "#E4DDCD", // outermost app background, behind panels
    paper: "#F4F0E6", // main content surface
    header: "#F1ECE1", // topbar / header surface
    panel: "#EBE5D7", // sidebar, rails, desk background
    cardSurface: "#FCFBF6", // default card surface
    surface: "#FCFBF6", // alias of cardSurface for existing consumers
    ink: "#1C1A16", // primary text
    inkMuted: "#57534A", // secondary text
    inkFaint: "#A39C8B", // mono metadata
    inkFaintAlt: "#8A8578", // secondary caption text (account subtext, storage caption)
    hairline: "rgba(28,26,22,.11)", // card & panel borders
    line: "rgba(28,26,22,.11)", // alias of hairline for existing consumers
    cobalt: "#1B3FD1", // primary accent — buttons, links, active states
    cobaltDeep: "#17206b", // gradients, article palette
    terracotta: "#C24A2E", // ledger margin rule, editorial red hairline
    gold: "#E0B23A", // Drift accent, keep
    green: "#2E7D5B", // product palette, storage meter
    noteSurface: "#FBF4D8", // note cards
    danger: "#B3261E",
  },
  font: {
    sans: "var(--font-instrument-sans), 'Instrument Sans', system-ui, sans-serif",
    mono: "var(--font-jetbrains-mono), 'JetBrains Mono', monospace",
    quote: "var(--font-newsreader), 'Newsreader', serif",
  },
} as const;
