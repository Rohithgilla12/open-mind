import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { Redirect, useRouter } from "expo-router";
import { useCallback, useState } from "react";
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  RefreshControl,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { ApiError, listFeedItems, type Item } from "@/lib/api";
import { showItemActions, useAndroidActionSheet } from "@/lib/item-actions";
import { useKeepItem, usePinItem } from "@/lib/mutations";
import { trimToFirstPage } from "@/lib/paged-cache";
import { queryKeys } from "@/lib/query";
import { useSettingsContext } from "@/lib/settings-context";
import { colors, fonts, radius, spacing } from "@/lib/theme";
import { stripMarkdown } from "@/lib/text";
import { useSoftFocusRefetch } from "@/lib/use-soft-focus-refetch";

const LIST_LIMIT = 50;

/** Coarse relative-time label ("just now", "3h ago", "5d ago") from an ISO timestamp. */
function relativeTime(iso: string | undefined): string {
  if (!iso) return "";
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const seconds = Math.max(0, Math.floor((Date.now() - then) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;
  const weeks = Math.floor(days / 7);
  if (weeks < 5) return `${weeks}w ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;
  const years = Math.floor(days / 365);
  return `${years}y ago`;
}

export default function FeedScreen() {
  const router = useRouter();
  const { settings, configured, loading } = useSettingsContext();
  const [pendingKeepIds, setPendingKeepIds] = useState<Set<string>>(new Set());
  const keepItem = useKeepItem();
  const pinItem = usePinItem();
  const { present, node: actionSheet } = useAndroidActionSheet();

  const queryClient = useQueryClient();

  const feedQuery = useInfiniteQuery({
    queryKey: queryKeys.feed(LIST_LIMIT),
    enabled: !!settings && configured,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const res = await listFeedItems(LIST_LIMIT, pageParam);
      if (!res.ok) throw new ApiError(res.status);
      return { items: res.items, nextCursor: res.nextCursor };
    },
    getNextPageParam: (last) => last.nextCursor,
  });

  const trimToFirst = useCallback(() => {
    queryClient.setQueryData(queryKeys.feed(LIST_LIMIT), (prev: unknown) => trimToFirstPage(prev));
  }, [queryClient]);

  useSoftFocusRefetch(feedQuery, undefined, trimToFirst);

  const onEndReached = useCallback(() => {
    if (feedQuery.hasNextPage && !feedQuery.isFetchingNextPage) {
      void feedQuery.fetchNextPage();
    }
  }, [feedQuery]);

  const onToggleKeep = useCallback(
    async (item: Item) => {
      setPendingKeepIds((prev) => new Set(prev).add(item.id));
      try {
        await keepItem(item);
      } finally {
        setPendingKeepIds((prev) => {
          const next = new Set(prev);
          next.delete(item.id);
          return next;
        });
      }
    },
    [keepItem],
  );

  const onOpen = useCallback(
    (item: Item) => {
      router.push(`/item/${item.id}`);
    },
    [router],
  );

  const onLongPress = useCallback(
    (item: Item) => {
      showItemActions(
        item,
        {
          onOpen,
          onPin: (it) => void pinItem(it),
          onKeep: (it) => void onToggleKeep(it),
        },
        present,
      );
    },
    [onOpen, pinItem, onToggleKeep, present],
  );

  if (!loading && !configured) return <Redirect href="/settings" />;

  const items = feedQuery.data?.pages.flatMap((p) => p.items) ?? [];
  const count = items.length;
  const errStatus = feedQuery.error instanceof ApiError ? feedQuery.error.status : undefined;

  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      {actionSheet}
      <View style={styles.topHairline} />
      <View style={styles.header}>
        <Text style={styles.title}>Feed</Text>
        <Text style={styles.subtitle}>
          {/* A count over a partial list asserts a feed size that isn't true yet —
              carry the "+" suffix (as the Library header does) while more pages
              remain, and force the plural since a "+" total is always more than one. */}
          {count}
          {feedQuery.hasNextPage ? "+" : ""}{" "}
          {feedQuery.hasNextPage || count !== 1 ? "items" : "item"} · from your subscribed feeds
        </Text>
      </View>
      <Body
        isPending={feedQuery.isPending && !feedQuery.data}
        isError={feedQuery.isError}
        errStatus={errStatus}
        items={items}
        refreshing={feedQuery.isRefetching && !feedQuery.isFetchingNextPage}
        pendingKeepIds={pendingKeepIds}
        isFetchingNextPage={feedQuery.isFetchingNextPage}
        onEndReached={onEndReached}
        onRefresh={() => {
          trimToFirst();
          void feedQuery.refetch();
        }}
        onRetry={() => void feedQuery.refetch()}
        onToggleKeep={onToggleKeep}
        onOpen={onOpen}
        onLongPress={onLongPress}
      />
    </SafeAreaView>
  );
}

type BodyProps = {
  isPending: boolean;
  isError: boolean;
  errStatus?: number;
  items: Item[];
  refreshing: boolean;
  pendingKeepIds: Set<string>;
  isFetchingNextPage: boolean;
  onEndReached: () => void;
  onRefresh: () => void;
  onRetry: () => void;
  onToggleKeep: (item: Item) => void;
  onOpen: (item: Item) => void;
  onLongPress: (item: Item) => void;
};

function Body({
  isPending,
  isError,
  errStatus,
  items,
  refreshing,
  pendingKeepIds,
  isFetchingNextPage,
  onEndReached,
  onRefresh,
  onRetry,
  onToggleKeep,
  onOpen,
  onLongPress,
}: BodyProps) {
  if (isPending) {
    return (
      <View style={styles.centre}>
        <ActivityIndicator color={colors.cobalt} />
      </View>
    );
  }

  if (isError && items.length === 0) {
    if (errStatus === 0) {
      return (
        <Message
          text="Instance unreachable — check your connection or the URL in Settings."
          onRetry={onRetry}
        />
      );
    }
    if (errStatus === 401) {
      return <Message text="Token rejected — check Settings." onRetry={onRetry} />;
    }
    return <Message text="Couldn't load your feed." onRetry={onRetry} />;
  }

  return (
    <FlatList
      data={items}
      keyExtractor={(item) => item.id}
      renderItem={({ item }) => (
        <FeedRow
          item={item}
          pending={pendingKeepIds.has(item.id)}
          onToggleKeep={() => onToggleKeep(item)}
          onOpen={() => onOpen(item)}
          onLongPress={() => onLongPress(item)}
        />
      )}
      contentContainerStyle={styles.list}
      ItemSeparatorComponent={() => <View style={styles.separator} />}
      refreshControl={
        <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={colors.cobalt} />
      }
      ListEmptyComponent={
        <Message text="Nothing in your feed yet — subscribe to feeds from the web app's Feeds page." />
      }
      onEndReached={onEndReached}
      onEndReachedThreshold={0.6}
      ListFooterComponent={
        isFetchingNextPage ? (
          <View style={styles.footer}>
            <ActivityIndicator color={colors.inkFaint} />
          </View>
        ) : null
      }
    />
  );
}

function FeedRow({
  item,
  pending,
  onToggleKeep,
  onOpen,
  onLongPress,
}: {
  item: Item;
  pending: boolean;
  onToggleKeep: () => void;
  onOpen: () => void;
  onLongPress: () => void;
}) {
  const kept = !!item.keptAt;
  const pinned = !!item.pinnedAt;
  const title = stripMarkdown(item.title?.trim()) || item.url || "Untitled";
  const summary = stripMarkdown(item.summary);
  return (
    <Pressable
      style={({ pressed }) => [styles.row, pressed && styles.rowPressed]}
      onPress={onOpen}
      onLongPress={onLongPress}
      delayLongPress={350}
    >
      <View style={styles.rowText}>
        <View style={styles.rowTitleRow}>
          <Text style={styles.rowTitle} numberOfLines={2}>
            {title}
          </Text>
          {pinned ? <Text style={styles.pinMark}>◆</Text> : null}
        </View>
        {summary ? (
          <Text style={styles.rowSummary} numberOfLines={2}>
            {summary}
          </Text>
        ) : null}
        <Text style={styles.rowMeta}>{relativeTime(item.createdAt)}</Text>
      </View>
      <Pressable
        style={({ pressed }) => [
          styles.keepButton,
          kept && styles.keepButtonActive,
          pressed && styles.keepButtonPressed,
        ]}
        onPress={onToggleKeep}
        disabled={pending}
      >
        <Text style={[styles.keepButtonText, kept && styles.keepButtonTextActive]}>
          {kept ? "Kept" : "Keep"}
        </Text>
      </Pressable>
    </Pressable>
  );
}

function Message({ text, onRetry }: { text: string; onRetry?: () => void }) {
  return (
    <View style={styles.centre}>
      <Text style={styles.messageText}>{text}</Text>
      {onRetry ? (
        <Pressable
          style={({ pressed }) => [styles.retryButton, pressed && styles.retryButtonPressed]}
          onPress={onRetry}
        >
          <Text style={styles.retryButtonText}>Try again</Text>
        </Pressable>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.canvas },
  topHairline: { height: 2, backgroundColor: colors.gold },
  header: { paddingHorizontal: spacing.xl, paddingTop: spacing.lg, paddingBottom: spacing.md },
  title: { fontFamily: fonts.serifBold, fontSize: 27, color: colors.ink },
  subtitle: {
    fontFamily: fonts.mono,
    fontSize: 12,
    color: colors.inkFaint,
    marginTop: spacing.xs,
  },
  list: { paddingHorizontal: spacing.xl, paddingBottom: spacing.xxl, flexGrow: 1 },
  separator: { height: 14 },
  footer: { paddingVertical: spacing.xl, alignItems: "center" },
  centre: { flex: 1, alignItems: "center", justifyContent: "center", padding: spacing.xl },
  messageText: {
    fontSize: 14,
    color: colors.inkMuted,
    lineHeight: 20,
    textAlign: "center",
  },
  retryButton: {
    marginTop: spacing.lg,
    borderWidth: 1,
    borderColor: colors.cobalt,
    borderRadius: radius.button,
    paddingHorizontal: spacing.xl,
    paddingVertical: spacing.sm,
  },
  retryButtonPressed: { opacity: 0.7 },
  retryButtonText: { color: colors.cobalt, fontSize: 14, fontWeight: "600" },
  row: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.md,
    borderRadius: radius.card,
    borderWidth: 1,
    borderColor: colors.hairline,
    backgroundColor: colors.cardSurface,
    padding: spacing.md,
  },
  rowPressed: { opacity: 0.85 },
  rowText: { flex: 1, gap: 3 },
  rowTitleRow: { flexDirection: "row", alignItems: "flex-start", gap: spacing.sm },
  rowTitle: { flex: 1, fontFamily: fonts.serifBold, fontSize: 16, color: colors.ink, lineHeight: 21 },
  pinMark: { color: colors.gold, fontSize: 12, marginTop: 3 },
  rowSummary: { fontFamily: fonts.sans, fontSize: 12.5, color: colors.inkMuted, lineHeight: 17 },
  rowMeta: {
    fontFamily: fonts.mono,
    fontSize: 10.5,
    color: colors.inkFaint,
    marginTop: 2,
  },
  keepButton: {
    borderWidth: 1,
    borderColor: colors.cobalt,
    borderRadius: radius.button,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
  },
  keepButtonActive: { backgroundColor: colors.cobalt },
  keepButtonPressed: { opacity: 0.7 },
  keepButtonText: { fontFamily: fonts.sansMedium, fontSize: 12.5, color: colors.cobalt },
  keepButtonTextActive: { color: colors.paper },
});
