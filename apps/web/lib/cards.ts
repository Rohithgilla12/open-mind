import { tokens } from "@openmind/ui";

export type CardKind =
  | "article"
  | "quote"
  | "image"
  | "product"
  | "note"
  | "video"
  | "tweet"
  | "book"
  | "recipe";

const KNOWN_KINDS: readonly CardKind[] = [
  "article",
  "quote",
  "image",
  "product",
  "note",
  "video",
  "tweet",
  "book",
  "recipe",
];

/** Normalise a raw cardType into a known kind; unknown/absent → article. */
export function cardKind(cardType: string | undefined): CardKind {
  if (cardType && (KNOWN_KINDS as readonly string[]).includes(cardType)) {
    return cardType as CardKind;
  }
  return "article";
}

/** Human hostname for the meta line; null for uploads/notes or unparseable urls. */
export function domainOf(url: string | undefined): string | null {
  if (!url || url.startsWith("/assets/")) return null;
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return null;
  }
}

const { color } = tokens;

/**
 * Per-type gradient used as the underlay behind a lead image, so a missing or
 * broken image reveals the gradient instead of a broken-image glyph.
 */
export const typeGradient: Record<CardKind, string> = {
  article: `linear-gradient(120deg, ${color.cobalt}, ${color.cobaltDeep})`,
  quote: `linear-gradient(135deg, ${color.ink}, ${color.cobaltDeep})`,
  image: `linear-gradient(150deg, ${color.terracotta} 0%, ${color.gold} 55%, ${color.paper} 100%)`,
  product: `linear-gradient(135deg, ${color.green}, ${color.ink})`,
  note: `linear-gradient(135deg, ${color.gold}, ${color.noteSurface})`,
  video: `linear-gradient(135deg, ${color.ink}, rgba(0,0,0,1))`,
  tweet: `linear-gradient(135deg, ${color.cobalt}, ${color.green})`,
  book: `linear-gradient(160deg, ${color.terracotta}, ${color.ink})`,
  recipe: `linear-gradient(135deg, ${color.terracotta}, ${color.gold})`,
};

/**
 * Per-type accent colour, used as a thin top rule on text-forward cards
 * (imageless article/product/book/recipe) as a subtle nod to the type.
 */
export const typeAccent: Record<CardKind, string> = {
  article: color.cobalt,
  quote: color.gold,
  image: color.terracotta,
  product: color.green,
  note: color.gold,
  video: color.ink,
  tweet: color.cobalt,
  book: color.terracotta,
  recipe: color.gold,
};

/** Meta-line label per type (matches mockup: tweet reads "Post"). */
export const typeLabel: Record<CardKind, string> = {
  article: "Article",
  quote: "Quote",
  image: "Image",
  product: "Product",
  note: "Note",
  video: "Video",
  tweet: "Post",
  book: "Book",
  recipe: "Recipe",
};
