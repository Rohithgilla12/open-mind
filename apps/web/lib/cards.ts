import { tokens } from "@openmind/ui";
import type { Item } from "./types";

const { color } = tokens;

/**
 * The card types, straight from the OpenAPI contract rather than re-typed here.
 * Every per-type map below is a `Record<CardKind, …>`, so adding a type to
 * `openapi.yaml` and regenerating breaks compilation until each map — including
 * the home-page filter chips — accounts for it.
 */
export type CardKind = NonNullable<Item["cardType"]>;

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
  repo: "Repo",
};

/** Plural form for the filter chips, where the label names a collection. */
export const chipLabel: Record<CardKind, string> = {
  article: "Articles",
  quote: "Quotes",
  image: "Images",
  product: "Products",
  note: "Notes",
  video: "Video",
  tweet: "Posts",
  book: "Books",
  recipe: "Recipes",
  repo: "Repos",
};

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
  repo: `linear-gradient(135deg, ${color.gold}, ${color.green})`,
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
  repo: color.gold,
};

/** Every known kind, derived from the label map so the two can't disagree. */
const KNOWN_KINDS = Object.keys(typeLabel) as readonly CardKind[];

/**
 * Chip order for the home-page filter strip — curated for the eye (commonest
 * types first), not the contract's enum order. `all` carries no `type` param
 * and so shows everything; the page maps `type` → API `types` and filters
 * server-side. `typeFilters` covers every kind, enforced by a test.
 */
const CHIP_ORDER: readonly CardKind[] = [
  "article",
  "image",
  "quote",
  "product",
  "video",
  "tweet",
  "recipe",
  "note",
  "book",
  "repo",
];

export const typeFilters: readonly { label: string; type: string }[] = [
  { label: "All", type: "all" },
  ...CHIP_ORDER.map((kind) => ({ label: chipLabel[kind], type: kind })),
];

/** Normalise a raw cardType into a known kind; unknown/absent → article. */
export function cardKind(cardType: string | undefined): CardKind {
  if (cardType && (KNOWN_KINDS as readonly string[]).includes(cardType)) {
    return cardType as CardKind;
  }
  return "article";
}

/**
 * Text-forward types worth treating as long-form reading material: opening in
 * distraction-free reader mode (item detail page) and painting highlights over
 * (reader page). A repo's README-style body reads like an article's.
 */
export function isTextForward(kind: CardKind): boolean {
  return (
    kind === "article" ||
    kind === "product" ||
    kind === "book" ||
    kind === "recipe" ||
    kind === "note" ||
    kind === "repo"
  );
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

/**
 * A failed extraction leaves an item with no title, summary or tags, so its card
 * would otherwise render as an empty shell and read as a broken app. Recover the
 * most human part of the url — the last path segment, else the hostname — so the
 * card still says what was saved.
 */
export function fallbackTitle(url: string | undefined): string | null {
  const host = domainOf(url);
  if (!host || !url) return null;
  let path: string;
  try {
    path = new URL(url).pathname;
  } catch {
    return null;
  }
  const segment = path.split("/").filter(Boolean).pop();
  if (!segment) return host;
  const words = decodeURIComponent(segment)
    .replace(/\.[a-z0-9]{1,5}$/i, "")
    .replace(/[-_]+/g, " ")
    .trim();
  return words || null;
}

/**
 * An item that came through a feed and has not been kept: searchable, but not
 * part of the Mind until the reader keeps it. Both the server-rendered search
 * and the live one show these below a divider rather than among library
 * matches, so the distinction survives however the results arrived.
 */
export function isFeedOnly(item: Item): boolean {
  return Boolean(item.feedId) && !item.keptAt;
}
