/**
 * The Mind's local search index.
 *
 * Search on the server is a hybrid of Postgres FTS and pgvector, and it is the
 * better ranking — but it is also a network round trip, and this deployment
 * pays a ~310 ms detour that is a billing question rather than an engineering
 * one (see TODO.md). So the client keeps its own index of the library and
 * answers every keystroke from memory, then lets the server's ranking replace
 * it when it lands. This file is that index: pure functions, no DOM, no fetch,
 * so it runs identically in a Web Worker and in vitest.
 *
 * It is a LINEAR SCAN, deliberately. An inverted index with a prefix trie would
 * be the textbook answer, but a library is thousands of items, not millions:
 * scoring 2,000 items against a query costs well under a millisecond, the code
 * stays small enough to test exhaustively, and there is no index to keep in
 * sync as pages stream in. If a library ever grows past ~50k items, revisit —
 * the seam is `queryLocal`, and nothing outside this file assumes a scan.
 */
import { colourTerm, resolveColor } from "./colors";
import type { Item } from "./types";

/**
 * How many matches a local query returns. Also bounds the cost of the grid's
 * view-transition morph, which snapshots one element per rendered card.
 */
export const LOCAL_RESULT_LIMIT = 48;

/**
 * Score per matched term, by where the match landed. Title beats tags beats
 * summary because that is the order a reader recognises their own save in; a
 * bare substring scores low so it can rescue a typo mid-word without
 * outranking a real word match.
 */
const SCORE = {
  titlePrefix: 100,
  titleWord: 70,
  tagExact: 60,
  tagWord: 45,
  domain: 40,
  summaryWord: 25,
  substring: 12,
} as const;

/** Awarded once when the whole query appears verbatim in the title. */
const PHRASE_BONUS = 80;

/**
 * Colour terms score on perceptual distance: a term resolving to a colour
 * (a name from the shared vocabulary, or a hex string) is compared against the
 * item's extracted palette. Beyond COLOUR_MAX_DELTA the colours read as
 * unrelated and the term simply does not match.
 *
 * The server applies no cutoff — /search?color= ranks the whole library by
 * distance — because there the colour arrived as an explicit filter. Here the
 * term is one word of free text ANDed with the others, so an unbounded match
 * would let "red" quietly match everything.
 */
const COLOUR_MAX_DELTA = 55;
const COLOUR_FLOOR = 20;
const COLOUR_RANGE = 40;

/** A CIELAB colour. Mirrors `lab` in apps/api/internal/search/color.go. */
interface Lab {
  l: number;
  a: number;
  b: number;
}

/** One library item, prepared for scanning. */
export interface Indexed {
  item: Item;
  /** Normalised haystacks: lowercased, unaccented, punctuation spaced out. */
  title: string;
  summary: string;
  domain: string;
  tags: string[];
  /**
   * The item's extracted palette in Lab. Only ever the real server-extracted
   * palette — never `derivedPalette`, which invents harmonious colours from a
   * title hash for cards that have none. Those are decoration; matching a
   * colour search against them would return items that are not that colour.
   */
  palette: Lab[];
  created: number;
}

/**
 * Lowercase, strip diacritics, and reduce every run of non-alphanumerics to a
 * single space, so "Kyōto—in Autumn" becomes "kyoto in autumn" and word-prefix
 * tests are a plain `startsWith` / `includes(" " + term)`.
 */
export function normalise(raw: string | undefined): string {
  if (!raw) return "";
  return raw
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, " ")
    .trim();
}

/**
 * Colour targets for a query, keyed by the normalised term they belong to.
 *
 * Derived from the RAW tokens, because `normalise` strips '#' — so by the time
 * a term reaches the scorer, "#1b3fd1" and the word "facade" are both bare hex
 * and indistinguishable. `colourTerm` is applied before that happens.
 */
function colourTargets(raw: string): Map<string, Lab> {
  const out = new Map<string, Lab>();
  for (const token of raw.trim().split(/\s+/)) {
    const term = colourTerm(token);
    if (!term) continue;
    const lab = hexToLab(term);
    if (lab) out.set(normalise(token), lab);
  }
  return out;
}

/** Split a raw query into normalised terms. */
export function queryTerms(raw: string): string[] {
  const n = normalise(raw);
  return n ? n.split(" ") : [];
}

/** True when `hay` contains a word starting with `term`. */
function hasWordPrefix(hay: string, term: string): boolean {
  return hay.startsWith(term) || hay.includes(` ${term}`);
}

/** Host of a URL, minus "www.", or "" for asset paths and unparseable URLs. */
function hostOf(url: string | undefined): string {
  if (!url || url.startsWith("/")) return "";
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return "";
  }
}

/** sRGB → CIELAB (D65). Mirrors rgb.toLab in the backend's color.go. */
function hexToLab(hex: string): Lab | null {
  const resolved = resolveColor(hex);
  if (!resolved) return null;
  const int = Number.parseInt(resolved.slice(1), 16);
  const lin = (ch: number) => {
    const v = ch / 255;
    return v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
  };
  const r = lin((int >> 16) & 0xff);
  const g = lin((int >> 8) & 0xff);
  const b = lin(int & 0xff);
  const x = (r * 0.4124 + g * 0.3576 + b * 0.1805) / 0.95047;
  const y = r * 0.2126 + g * 0.7152 + b * 0.0722;
  const z = (r * 0.0193 + g * 0.1192 + b * 0.9505) / 1.08883;
  const f = (t: number) => (t > 0.008856 ? Math.cbrt(t) : 7.787 * t + 16 / 116);
  const fx = f(x);
  const fy = f(y);
  const fz = f(z);
  return { l: 116 * fy - 16, a: 500 * (fx - fy), b: 200 * (fy - fz) };
}

/** ΔE*76 — Euclidean distance in CIELAB. Mirrors deltaE in color.go. */
function deltaE(a: Lab, b: Lab): number {
  const dl = a.l - b.l;
  const da = a.a - b.a;
  const db = a.b - b.b;
  return Math.sqrt(dl * dl + da * da + db * db);
}

/** Prepare one item for scanning. */
export function indexItem(item: Item): Indexed {
  const tags = [...(item.tags ?? []), ...(item.userTags ?? [])]
    .map(normalise)
    .filter(Boolean);
  const created = item.createdAt ? Date.parse(item.createdAt) : Number.NaN;
  return {
    item,
    title: normalise(item.title),
    summary: normalise(item.summary),
    domain: normalise(hostOf(item.url)),
    tags: [...new Set(tags)],
    palette: (item.palette ?? []).map(hexToLab).filter((c): c is Lab => c !== null),
    created: Number.isNaN(created) ? 0 : created,
  };
}

export function indexItems(items: Item[]): Indexed[] {
  return items.map(indexItem);
}

/** Best score for one term against one item, or 0 when the term misses. */
function scoreTerm(entry: Indexed, term: string, target: Lab | null): number {
  let best = 0;
  if (entry.title.startsWith(term)) best = SCORE.titlePrefix;
  else if (hasWordPrefix(entry.title, term)) best = SCORE.titleWord;

  for (const tag of entry.tags) {
    if (tag === term) best = Math.max(best, SCORE.tagExact);
    else if (hasWordPrefix(tag, term)) best = Math.max(best, SCORE.tagWord);
  }

  if (best < SCORE.domain && entry.domain.includes(term)) best = Math.max(best, SCORE.domain);
  if (hasWordPrefix(entry.summary, term)) best = Math.max(best, SCORE.summaryWord);

  if (target && entry.palette.length > 0) {
    let closest = Number.POSITIVE_INFINITY;
    for (const c of entry.palette) closest = Math.min(closest, deltaE(target, c));
    if (closest <= COLOUR_MAX_DELTA) {
      best = Math.max(best, COLOUR_FLOOR + COLOUR_RANGE * (1 - closest / COLOUR_MAX_DELTA));
    }
  }

  if (best === 0 && (entry.title.includes(term) || entry.summary.includes(term))) {
    best = SCORE.substring;
  }
  return best;
}

/**
 * Score `raw` against the index and return the best matches, most relevant
 * first. Terms are ANDed: every term must match somewhere, which is what makes
 * typing feel like narrowing rather than churning.
 *
 * Ties break by newest-created then id descending — the same tiebreak the
 * backend's colour ranking uses, so a local ordering never disagrees with the
 * server's for reasons the reader can see.
 */
export function queryLocal(
  index: Indexed[],
  raw: string,
  limit: number = LOCAL_RESULT_LIMIT,
): Item[] {
  const terms = queryTerms(raw);
  if (terms.length === 0) return [];
  const targets = colourTargets(raw);
  const phrase = terms.join(" ");

  const hits: { entry: Indexed; score: number }[] = [];
  for (const entry of index) {
    let total = 0;
    let matchedAll = true;
    for (let i = 0; i < terms.length; i++) {
      const s = scoreTerm(entry, terms[i], targets.get(terms[i]) ?? null);
      if (s === 0) {
        matchedAll = false;
        break;
      }
      total += s;
    }
    if (!matchedAll) continue;
    if (terms.length > 1 && entry.title.includes(phrase)) total += PHRASE_BONUS;
    hits.push({ entry, score: total });
  }

  hits.sort((a, b) => {
    if (b.score !== a.score) return b.score - a.score;
    if (b.entry.created !== a.entry.created) return b.entry.created - a.entry.created;
    return b.entry.item.id.localeCompare(a.entry.item.id);
  });

  return hits.slice(0, limit).map((h) => h.entry.item);
}
