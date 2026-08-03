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
  cobaltDeep: "#17206b", // gradients, article palette
  terracotta: "#C24A2E", // ledger margin rule, editorial red hairline
  gold: "#E0B23A", // Drift accent, keep
  green: "#2E7D5B", // product palette, storage meter
  note: "#FBF4D8", // note card surface
  hairline: "rgba(28,26,22,0.12)", // borders
  danger: "#B3261E", // errors / destructive
} as const;

export const fonts = {
  // Brand fonts loaded via expo-font + @expo-google-fonts/*; see app/_layout.tsx.
  sans: "InstrumentSans_400Regular",
  sansMedium: "InstrumentSans_500Medium",
  sansSemiBold: "InstrumentSans_600SemiBold",
  serif: "Newsreader_500Medium_Italic",
  serifBold: "Newsreader_600SemiBold_Italic",
  mono: "JetBrainsMono_400Regular",
  monoMedium: "JetBrainsMono_500Medium",
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

// Type scale (px) per docs/superpowers/specs/20260707-mobile-design-pass.md Move 1.
export const type = {
  title: { fontSize: 27, fontFamily: fonts.serifBold },
  cardTitle: { fontSize: 17.5, fontFamily: fonts.serifBold },
  body: { fontSize: 15.5, fontFamily: fonts.sans },
  meta: { fontSize: 10.5, fontFamily: fonts.mono },
  kicker: { fontSize: 10, fontFamily: fonts.monoMedium, letterSpacing: 0.8 },
} as const;

export type CardKind =
  | "article"
  | "quote"
  | "image"
  | "product"
  | "note"
  | "video"
  | "tweet"
  | "book"
  | "recipe"
  | "repo";

/**
 * Per-type gradient fallback (used when the item has no extracted palette),
 * mirrored from apps/web/lib/cards.ts `typeGradient` so mobile and web read as
 * the same brand: article/tweet cobalt-tinted, product/green organic, book/
 * recipe terracotta editorial, note/quote gold-warm, image terracotta→gold,
 * video near-black.
 */
export const typeGradients: Record<CardKind, [string, string]> = {
  article: [colors.cobalt, colors.cobaltDeep],
  quote: [colors.ink, colors.cobaltDeep],
  image: [colors.terracotta, colors.gold],
  product: [colors.green, colors.ink],
  note: [colors.gold, colors.note],
  video: [colors.ink, "#000000"],
  tweet: [colors.cobalt, colors.green],
  book: [colors.terracotta, colors.ink],
  recipe: [colors.terracotta, colors.gold],
  repo: [colors.gold, colors.green],
} as const;

export const theme = { colors, fonts, radius, spacing, type, typeGradients } as const;

export type Theme = typeof theme;
