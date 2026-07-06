// Openmind mobile design tokens — warm editorial palette.
// Inlined (not imported from @openmind/ui) because apps/mobile is a standalone
// Expo app outside the pnpm workspace. Values mirror docs/design/README.md.

export const colors = {
  canvas: "#E4DDCD", // outermost background
  paper: "#F4F0E6", // main surfaces / header
  cardSurface: "#FCFBF6", // default card
  ink: "#1C1A16", // primary text
  inkMuted: "#57534A", // secondary text
  inkFaint: "#A39C8B", // mono metadata
  cobalt: "#1B3FD1", // primary accent
  gold: "#E0B23A", // Drift accent
  hairline: "rgba(28,26,22,0.12)", // borders
  danger: "#B3261E", // errors / destructive
} as const;

export const fonts = {
  // System sans for UI/body; platform serif for editorial titles.
  // (Newsreader via expo-font is a later enhancement.)
  sans: "System",
  serif: "serif",
  mono: "monospace",
} as const;

export const radius = {
  card: 11,
  button: 10,
  pill: 20,
  overlay: 16,
} as const;

export const spacing = {
  xs: 4,
  sm: 8,
  md: 12,
  lg: 16,
  xl: 24,
  xxl: 32,
} as const;

export const theme = { colors, fonts, radius, spacing } as const;

export type Theme = typeof theme;
