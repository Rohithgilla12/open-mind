import { useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { Alert } from "react-native";
import { deleteItem, setKept, setPinned, type Item } from "./api";
import { confirmDelete } from "./item-actions";
import { filterCachedItems, mapCachedItems } from "./paged-cache";
import { queryKeys } from "./query";

/** Patch an item across every list cache that might hold it. */
function patchItemInCaches(
  qc: ReturnType<typeof useQueryClient>,
  id: string,
  patch: Partial<Item>,
) {
  const apply = (it: Item): Item => (it.id === id ? { ...it, ...patch } : it);

  qc.setQueriesData({ queryKey: ["feed"] }, (prev) => mapCachedItems<Item>(prev, apply));
  qc.setQueriesData({ queryKey: ["items"] }, (prev) => mapCachedItems<Item>(prev, apply));
  qc.setQueriesData({ queryKey: ["search"] }, (prev) => mapCachedItems<Item>(prev, apply));
  qc.setQueriesData<Item[]>({ queryKey: queryKeys.desk() }, (prev) => {
    if (!prev) return prev;
    // Unpinning removes from desk immediately.
    if (patch.pinnedAt === null) return prev.filter((it) => it.id !== id);
    // Pinning: if the item isn't on desk yet, leave lists to invalidate
    // (we may not have the full item here). Just patch badge fields if present.
    return prev.map(apply);
  });
  qc.setQueryData(queryKeys.item(id), (prev: Item | undefined) =>
    prev ? { ...prev, ...patch } : prev,
  );
}

/** Invalidate list caches after a mutation so Library / Feed / Desk stay in sync. */
export function useInvalidateLists() {
  const qc = useQueryClient();
  return useCallback(() => {
    // ["items"] and ["feed"] are infinite queries: invalidating them normally
    // would refetch every loaded page in sequence (v5 dropped refetchPage), and
    // we used to dodge that by trimming the cache to one page before
    // invalidating. That trim was wrong here — it runs on every pin/keep/delete,
    // so a reader scrolled to item 120 would see their list collapse back to
    // page one the moment the mutation resolved. Instead, mark them stale with
    // refetchType: "none": no refetch fires now, so nothing collapses.
    // patchItemInCaches/filterCachedItems above already keep the visible items
    // correct optimistically; the one gap they can't cover — e.g. a keep making
    // an item newly eligible for a list that doesn't have it yet — reconciles
    // next time that tab regains focus, via useSoftFocusRefetch, which already
    // passes trimToFirstPage as its onBeforeRefetch so that eventual refetch is
    // still a single request. Do not restore the trim here.
    void qc.invalidateQueries({ queryKey: ["items"], refetchType: "none" });
    void qc.invalidateQueries({ queryKey: ["feed"], refetchType: "none" });
    // search and desk are single-page caches, so the fan-out problem never
    // applied to them; refetch immediately as before.
    void qc.invalidateQueries({ queryKey: ["search"] });
    void qc.invalidateQueries({ queryKey: queryKeys.desk() });
  }, [qc]);
}

/** Pin toggle with optimistic cache patch + list invalidation. */
export function usePinItem() {
  const invalidate = useInvalidateLists();
  const qc = useQueryClient();

  return useCallback(
    async (item: Item) => {
      const next = !item.pinnedAt;
      const pinnedAt = next ? new Date().toISOString() : null;
      patchItemInCaches(qc, item.id, { pinnedAt });
      // When pinning, prepend onto desk cache so Desk updates without waiting.
      if (next) {
        qc.setQueriesData<Item[]>({ queryKey: queryKeys.desk() }, (prev) => {
          if (!prev) return prev;
          if (prev.some((it) => it.id === item.id)) {
            return prev.map((it) => (it.id === item.id ? { ...it, pinnedAt } : it));
          }
          return [{ ...item, pinnedAt }, ...prev];
        });
      }
      const res = await setPinned(item.id, next);
      if (!res.ok) {
        patchItemInCaches(qc, item.id, { pinnedAt: item.pinnedAt ?? null });
        if (next) {
          // Roll back the optimistic desk prepend.
          qc.setQueriesData<Item[]>({ queryKey: queryKeys.desk() }, (prev) =>
            prev?.filter((it) => it.id !== item.id),
          );
        }
        Alert.alert("Couldn't update pin", "Please try again.");
        return;
      }
      invalidate();
    },
    [invalidate, qc],
  );
}

/** Keep toggle with optimistic cache patch + list invalidation. */
export function useKeepItem() {
  const invalidate = useInvalidateLists();
  const qc = useQueryClient();

  return useCallback(
    async (item: Item) => {
      const next = !item.keptAt;
      const keptAt = next ? new Date().toISOString() : null;
      patchItemInCaches(qc, item.id, { keptAt });
      const res = await setKept(item.id, next);
      if (!res.ok) {
        patchItemInCaches(qc, item.id, { keptAt: item.keptAt ?? null });
        Alert.alert("Couldn't update", "Please try again.");
        return;
      }
      invalidate();
    },
    [invalidate, qc],
  );
}

/** Confirm + delete, then drop from list caches immediately. */
export function useDeleteItem() {
  const invalidate = useInvalidateLists();
  const qc = useQueryClient();

  return useCallback(
    (item: Item, after?: () => void) => {
      confirmDelete(item, async () => {
        const res = await deleteItem(item.id);
        if (!res.ok) {
          Alert.alert("Couldn't delete", "Please try again.");
          return;
        }
        void qc.removeQueries({ queryKey: queryKeys.item(item.id) });
        // Drop from list caches immediately so back-nav doesn't flash the gone card.
        const keep = (it: Item) => it.id !== item.id;
        qc.setQueriesData({ queryKey: ["feed"] }, (prev) => filterCachedItems<Item>(prev, keep));
        qc.setQueriesData({ queryKey: ["items"] }, (prev) => filterCachedItems<Item>(prev, keep));
        qc.setQueriesData({ queryKey: ["search"] }, (prev) => filterCachedItems<Item>(prev, keep));
        // Desk stays a flat Item[] (it isn't paginated), so a plain filter is
        // simplest and matches the other Desk-specific branches in this file.
        qc.setQueriesData<Item[]>({ queryKey: queryKeys.desk() }, (prev) => prev?.filter(keep));
        invalidate();
        after?.();
      });
    },
    [invalidate, qc],
  );
}
