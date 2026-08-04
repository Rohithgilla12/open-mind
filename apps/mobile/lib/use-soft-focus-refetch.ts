import { useFocusEffect } from "expo-router";
import { useCallback, useRef } from "react";
import type { UseQueryResult } from "@tanstack/react-query";
import { QUERY_STALE_MS } from "./query";

/**
 * Soft refetch when a tab/screen gains focus — only if the query is stale
 * (or has never loaded). Does not clear cached data, so returning from a
 * detail screen or switching tabs never flashes a full-screen spinner.
 */
export function useSoftFocusRefetch(
  query: Pick<UseQueryResult, "isStale" | "isPending" | "refetch" | "dataUpdatedAt">,
  extra?: () => void,
  onBeforeRefetch?: () => void,
): void {
  const extraRef = useRef(extra);
  extraRef.current = extra;
  const beforeRef = useRef(onBeforeRefetch);
  beforeRef.current = onBeforeRefetch;

  useFocusEffect(
    useCallback(() => {
      extraRef.current?.();
      // Refetch when stale or never fetched. `isStale` alone can be true before
      // the first successful fetch completes; refetch is a no-op while pending.
      if (query.isStale || query.dataUpdatedAt === 0) {
        // An infinite query refetches every loaded page, so callers trim to the
        // first page here to keep a focus refetch to one request.
        beforeRef.current?.();
        void query.refetch();
      }
      // Intentionally omit query object identity — depend on freshness signals.
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [query.isStale, query.dataUpdatedAt, query.refetch]),
  );
}

export { QUERY_STALE_MS };
