// Shared card-type helpers — single source of truth for the cardType enum
// normalisation and display labels used by ItemCard, the item detail screen,
// and the Library screen's group-by-type view.
import type { CardKind } from "./theme";

export const KNOWN_KINDS: readonly CardKind[] = [
  "article",
  "quote",
  "image",
  "product",
  "note",
  "video",
  "tweet",
  "book",
  "recipe",
  "repo",
];

/** Normalise a raw cardType into a known kind; unknown/absent → article. */
export function cardKind(cardType: string | undefined): CardKind {
  if (cardType && (KNOWN_KINDS as readonly string[]).includes(cardType)) {
    return cardType as CardKind;
  }
  return "article";
}

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

/** Plural label for group-by-type section headers. */
export const typeLabelPlural: Record<CardKind, string> = {
  article: "Articles",
  quote: "Quotes",
  image: "Images",
  product: "Products",
  note: "Notes",
  video: "Videos",
  tweet: "Posts",
  book: "Books",
  recipe: "Recipes",
  repo: "Repos",
};
