/**
 * Cache-shape helpers for query caches that may be flat or paged.
 *
 * Library and Feed are infinite queries ({ pages, pageParams }); Desk is a flat
 * array and search is { items, understood }. One mutation touches all four, so
 * the patch helper has to handle every shape rather than assume one.
 */

export type InfiniteCache<T> = {
  pages: { items: T[]; nextCursor?: string }[];
  pageParams: unknown[];
};

export function isInfiniteCache(v: unknown): v is InfiniteCache<unknown> {
  return (
    !!v &&
    typeof v === "object" &&
    Array.isArray((v as { pages?: unknown }).pages) &&
    Array.isArray((v as { pageParams?: unknown }).pageParams)
  );
}

/** Apply fn to every item in a cache, whatever shape it has. Never mutates. */
export function mapCachedItems<T>(cache: unknown, fn: (item: T) => T): unknown {
  if (!cache) return cache;
  if (isInfiniteCache(cache)) {
    const c = cache as InfiniteCache<T>;
    return { ...c, pages: c.pages.map((p) => ({ ...p, items: p.items.map(fn) })) };
  }
  if (Array.isArray(cache)) return (cache as T[]).map(fn);
  const obj = cache as { items?: unknown };
  if (Array.isArray(obj.items)) return { ...obj, items: (obj.items as T[]).map(fn) };
  return cache;
}

/**
 * Keep only the items matching predicate, whatever shape the cache has. Never
 * mutates. Used for removal (delete) where mapCachedItems's transform-in-place
 * shape can't express "drop this entry".
 */
export function filterCachedItems<T>(cache: unknown, predicate: (item: T) => boolean): unknown {
  if (!cache) return cache;
  if (isInfiniteCache(cache)) {
    const c = cache as InfiniteCache<T>;
    return { ...c, pages: c.pages.map((p) => ({ ...p, items: p.items.filter(predicate) })) };
  }
  if (Array.isArray(cache)) return (cache as T[]).filter(predicate);
  const obj = cache as { items?: unknown };
  if (Array.isArray(obj.items)) return { ...obj, items: (obj.items as T[]).filter(predicate) };
  return cache;
}

/**
 * Drop every page but the first.
 *
 * TanStack Query v5 removed refetchPage, so refetching an infinite query
 * re-requests every loaded page in sequence — ten loaded pages would mean ten
 * requests on one pull-to-refresh. Trimming first makes it one request, and
 * scrolling back down re-pages naturally. Data is never cleared, so there is no
 * spinner flash.
 */
export function trimToFirstPage(cache: unknown): unknown {
  if (!isInfiniteCache(cache)) return cache;
  return { ...cache, pages: cache.pages.slice(0, 1), pageParams: cache.pageParams.slice(0, 1) };
}
