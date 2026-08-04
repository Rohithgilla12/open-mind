/**
 * Cursor-paged list state. Pages are kept separate rather than flattened
 * because the Mind renders one CSS multi-column block per page: appending into
 * a single .mind-col makes the browser rebalance every column, which moves
 * cards the reader has already passed.
 */
export interface PagedState<T> {
  pages: T[][];
  /** Cursor for the next page; undefined once the list is exhausted. */
  cursor?: string;
}

export function initialPagedState<T>(items: T[], cursor?: string): PagedState<T> {
  return { pages: [items], cursor };
}

export function appendPage<T>(
  state: PagedState<T>,
  page: { items: T[]; nextCursor?: string },
): PagedState<T> {
  // An empty page would render as an empty block; the API's lookahead makes
  // this rare, but a concurrent delete can still produce one.
  const pages = page.items.length > 0 ? [...state.pages, page.items] : state.pages;
  return { pages, cursor: page.nextCursor };
}

export function mapPagedItems<T>(state: PagedState<T>, fn: (item: T) => T): PagedState<T> {
  return { ...state, pages: state.pages.map((page) => page.map(fn)) };
}
