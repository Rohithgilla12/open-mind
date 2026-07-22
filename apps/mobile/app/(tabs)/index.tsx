import { useQuery } from "@tanstack/react-query";
import { Redirect, useRouter } from "expo-router";
import { useCallback, useEffect, useState } from "react";
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  RefreshControl,
  SectionList,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { ItemCard } from "@/components/ItemCard";
import { ApiError, listItems, searchItems, type Item, type UnderstoodQuery } from "@/lib/api";
import { cardKind, KNOWN_KINDS, typeLabelPlural } from "@/lib/cards";
import { useCaptureQueue } from "@/lib/capture-queue-context";
import { showItemActions, useAndroidActionSheet } from "@/lib/item-actions";
import { useDeleteItem, usePinItem } from "@/lib/mutations";
import { queryKeys } from "@/lib/query";
import { useSettingsContext } from "@/lib/settings-context";
import { colors, fonts, radius, spacing, typeGradients, type CardKind } from "@/lib/theme";
import { useMorph, type MorphRect } from "@/lib/morph";
import { useSoftFocusRefetch } from "@/lib/use-soft-focus-refetch";

/** Group items by cardType, in a stable KNOWN_KINDS order, dropping empty groups. */
function groupByKind(items: Item[]): { kind: CardKind; items: Item[] }[] {
  const byKind = new Map<CardKind, Item[]>();
  for (const item of items) {
    const k = cardKind(item.cardType);
    const list = byKind.get(k);
    if (list) list.push(item);
    else byKind.set(k, [item]);
  }
  return KNOWN_KINDS.filter((k) => byKind.has(k)).map((k) => ({ kind: k, items: byKind.get(k)! }));
}

/** An unkept feed-river item: searchable, but not part of the Mind until kept. */
function isFeedOnly(item: Item): boolean {
  return Boolean(item.feedId) && !item.keptAt;
}

/** The two hero-gradient colours for an item — mirrors ItemCard's Hero so the
 * morph overlay starts on the exact colours the card is showing. */
function heroColors(item: Item): [string, string] {
  const dots = item.palette ?? [];
  return dots.length >= 2 ? [dots[0], dots[1]] : typeGradients[cardKind(item.cardType)];
}

const SEARCH_DEBOUNCE_MS = 300;
const LIST_LIMIT = 50;

type LibraryData = { items: Item[]; understood?: UnderstoodQuery };

export default function LibraryScreen() {
  const router = useRouter();
  const { settings, configured, loading } = useSettingsContext();
  const { pendingCount, flush } = useCaptureQueue();
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [searchFocused, setSearchFocused] = useState(false);
  const [grouped, setGrouped] = useState(false);
  const [colorFilter, setColorFilter] = useState<string | null>(null);
  const pinItem = usePinItem();
  const deleteItem = useDeleteItem();
  const { present, node: actionSheet } = useAndroidActionSheet();

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query.trim()), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [query]);

  const searching = debouncedQuery.length > 0;
  const filteringByColor = !searching && !!colorFilter;

  const listQuery = useQuery({
    queryKey: searching
      ? queryKeys.search(debouncedQuery)
      : filteringByColor
        ? queryKeys.search(`color:${colorFilter}`)
        : queryKeys.items(LIST_LIMIT),
    enabled: !!settings && configured,
    queryFn: async (): Promise<LibraryData> => {
      if (searching) {
        const res = await searchItems({ q: debouncedQuery, parse: true });
        if (!res.ok) throw new ApiError(res.status);
        return {
          items: res.results.map((r) => r.item),
          understood: res.understood,
        };
      }
      if (filteringByColor) {
        const res = await searchItems({ color: colorFilter });
        if (!res.ok) throw new ApiError(res.status);
        return { items: res.results.map((r) => r.item) };
      }
      const res = await listItems(LIST_LIMIT);
      if (!res.ok) throw new ApiError(res.status);
      return { items: res.items };
    },
    // Keep prior results visible while a new search key loads.
    placeholderData: (prev) => prev,
  });

  useSoftFocusRefetch(listQuery, () => {
    void flush();
  });

  const morph = useMorph();

  const onOpen = useCallback(
    (item: Item) => {
      router.push(`/item/${item.id}`);
    },
    [router],
  );

  // Morph open: spring the card's hero into the detail hero, then navigate
  // underneath. onMorphPress in ItemCard supplies the measured card rect.
  const onMorphOpen = useCallback(
    (item: Item, rect: MorphRect) => {
      morph.begin(rect, { colors: heroColors(item) });
      router.push(`/item/${item.id}`);
    },
    [morph, router],
  );

  const onLongPress = useCallback(
    (item: Item) => {
      showItemActions(
        item,
        {
          onOpen,
          onPin: (it) => void pinItem(it),
          onDelete: (it) => deleteItem(it),
        },
        present,
      );
    },
    [onOpen, pinItem, deleteItem, present],
  );

  if (!loading && !configured) return <Redirect href="/sign-in" />;

  const items = listQuery.data?.items ?? [];
  const count = items.length;
  const errStatus = listQuery.error instanceof ApiError ? listQuery.error.status : undefined;

  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      {actionSheet}
      <View style={styles.topHairline} />
      <View style={styles.header}>
        <View style={styles.titleRow}>
          <Text style={styles.title}>The Mind</Text>
          <View style={styles.headerActions}>
            <Pressable onPress={() => router.push("/places")} hitSlop={8}>
              <Text style={styles.mapAction}>⌖ Map</Text>
            </Pressable>
            {!searching && !filteringByColor ? (
              <Pressable
                style={({ pressed }) => [styles.groupToggle, pressed && styles.groupTogglePressed]}
                onPress={() => setGrouped((g) => !g)}
              >
                <Text style={styles.groupToggleText}>{grouped ? "List" : "Group by type"}</Text>
              </Pressable>
            ) : null}
          </View>
        </View>
        <Text style={styles.subtitle}>
          {searching || filteringByColor
            ? `${count} ${count === 1 ? "match" : "matches"}`
            : `${count} ${count === 1 ? "gathering" : "gatherings"} · organised by the machine`}
          {pendingCount > 0 ? ` · ${pendingCount} queued` : ""}
        </Text>
        <View style={styles.searchCard}>
          <TextInput
            style={[styles.searchInput, searchFocused && styles.searchInputFocused]}
            value={query}
            onChangeText={(t) => {
              setQuery(t);
              if (colorFilter) setColorFilter(null);
            }}
            onFocus={() => setSearchFocused(true)}
            onBlur={() => setSearchFocused(false)}
            placeholder="Search by fragment, vibe, colour…"
            placeholderTextColor={colors.inkFaint}
            autoCapitalize="none"
            autoCorrect={false}
            returnKeyType="search"
            clearButtonMode="while-editing"
          />
        </View>
        {listQuery.data?.understood ? (
          <UnderstoodRow understood={listQuery.data.understood} q={debouncedQuery} />
        ) : null}
        {colorFilter ? (
          <View style={styles.colorChip}>
            <View style={[styles.colorChipSwatch, { backgroundColor: colorFilter }]} />
            <Text style={styles.colorChipText}>Colour</Text>
            <Pressable
              onPress={() => setColorFilter(null)}
              hitSlop={8}
              accessibilityRole="button"
              accessibilityLabel="Clear colour filter"
            >
              <Text style={styles.colorChipClear}>×</Text>
            </Pressable>
          </View>
        ) : null}
      </View>
      <Body
        isPending={listQuery.isPending && !listQuery.data}
        isError={listQuery.isError}
        errStatus={errStatus}
        searching={searching || filteringByColor}
        items={items}
        settings={settings}
        refreshing={listQuery.isRefetching && !listQuery.isPending}
        grouped={grouped && !searching && !filteringByColor}
        onRefresh={() => void listQuery.refetch()}
        onRetry={() => void listQuery.refetch()}
        onOpen={onOpen}
        onMorphPress={onMorphOpen}
        onLongPress={onLongPress}
        onPickColor={(hex) => {
          setColorFilter(hex);
          setQuery("");
        }}
      />
    </SafeAreaView>
  );
}

function UnderstoodRow({ understood, q }: { understood: UnderstoodQuery; q: string }) {
  const chips: string[] = [];
  const text = understood.text?.trim();
  if (text && text !== q) chips.push(text);
  if (understood.color) chips.push(understood.color);
  for (const t of understood.types ?? []) chips.push(t);
  if (chips.length === 0) return null;
  return (
    <View style={styles.understoodRow}>
      <Text style={styles.understoodLabel}>understood</Text>
      {chips.map((chip) => (
        <View key={chip} style={styles.chip}>
          <Text style={styles.chipText}>{chip}</Text>
        </View>
      ))}
    </View>
  );
}

type BodyProps = {
  isPending: boolean;
  isError: boolean;
  errStatus?: number;
  searching: boolean;
  items: Item[];
  settings: ReturnType<typeof useSettingsContext>["settings"];
  refreshing: boolean;
  grouped: boolean;
  onRefresh: () => void;
  onRetry: () => void;
  onOpen: (item: Item) => void;
  onMorphPress: (item: Item, rect: MorphRect) => void;
  onLongPress: (item: Item) => void;
  onPickColor: (hex: string) => void;
};

function Body({
  isPending,
  isError,
  errStatus,
  searching,
  items,
  settings,
  refreshing,
  grouped,
  onRefresh,
  onRetry,
  onOpen,
  onMorphPress,
  onLongPress,
  onPickColor,
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
          text={
            searching
              ? "Search needs a connection."
              : "Instance unreachable — check your connection or the URL in Settings."
          }
          onRetry={onRetry}
        />
      );
    }
    if (errStatus === 401) {
      return <Message text="Token rejected — check Settings." onRetry={onRetry} />;
    }
    return (
      <Message
        text={searching ? "Couldn't run that search." : "Couldn't load your library."}
        onRetry={onRetry}
      />
    );
  }

  const emptyMessage = (
    <Message
      text={searching ? "No matches — try a different fragment." : "Nothing saved yet — capture something."}
    />
  );
  const refreshControl = (
    <RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={colors.cobalt} />
  );

  // Grouped browsing sections by card type; searching sections library
  // matches ahead of unkept feed-river matches (results arrive in that order
  // from the API, the header just makes the split visible).
  let sections: { title: string; data: Item[] }[] | null = null;
  if (grouped) {
    sections = groupByKind(items).map(({ kind, items: sectionItems }) => ({
      title: `${typeLabelPlural[kind]} · ${sectionItems.length}`,
      data: sectionItems,
    }));
  } else if (searching) {
    const feedMatches = items.filter(isFeedOnly);
    if (feedMatches.length > 0) {
      const mindMatches = items.filter((i) => !isFeedOnly(i));
      sections = [
        ...(mindMatches.length > 0
          ? [{ title: `In your Mind · ${mindMatches.length}`, data: mindMatches }]
          : []),
        { title: `From your feeds · ${feedMatches.length}`, data: feedMatches },
      ];
    }
  }

  if (sections) {
    return (
      <SectionList
        sections={sections}
        keyExtractor={(item) => item.id}
        renderItem={({ item }) => (
          <ItemCard item={item} settings={settings} onPress={onOpen} onMorphPress={onMorphPress} onLongPress={onLongPress} onPickColor={onPickColor} />
        )}
        renderSectionHeader={({ section }) => (
          <Text style={styles.sectionHeader}>{section.title}</Text>
        )}
        contentContainerStyle={styles.list}
        ItemSeparatorComponent={() => <View style={styles.separator} />}
        refreshControl={refreshControl}
        ListEmptyComponent={emptyMessage}
      />
    );
  }

  return (
    <FlatList
      data={items}
      keyExtractor={(item) => item.id}
      renderItem={({ item }) => (
        <ItemCard item={item} settings={settings} onPress={onOpen} onMorphPress={onMorphPress} onLongPress={onLongPress} onPickColor={onPickColor} />
      )}
      contentContainerStyle={styles.list}
      ItemSeparatorComponent={() => <View style={styles.separator} />}
      refreshControl={refreshControl}
      ListEmptyComponent={emptyMessage}
    />
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
  topHairline: { height: 2, backgroundColor: colors.terracotta },
  header: { paddingHorizontal: spacing.xl, paddingTop: spacing.lg, paddingBottom: spacing.md },
  titleRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  title: { fontFamily: fonts.serifBold, fontSize: 27, color: colors.ink },
  headerActions: { flexDirection: "row", alignItems: "center", gap: spacing.md },
  mapAction: { fontFamily: fonts.sansSemiBold, fontSize: 15, color: colors.cobalt },
  groupToggle: {
    borderWidth: 1,
    borderColor: colors.hairline,
    borderRadius: radius.pill,
    paddingHorizontal: spacing.md,
    paddingVertical: 6,
    backgroundColor: colors.paper,
  },
  groupTogglePressed: { opacity: 0.7 },
  groupToggleText: { fontFamily: fonts.mono, fontSize: 11, color: colors.inkMuted },
  sectionHeader: {
    fontFamily: fonts.monoMedium,
    fontSize: 11,
    letterSpacing: 0.6,
    textTransform: "uppercase",
    color: colors.inkFaint,
    backgroundColor: colors.canvas,
    paddingTop: spacing.md,
    paddingBottom: spacing.sm,
  },
  subtitle: {
    fontFamily: fonts.mono,
    fontSize: 12,
    color: colors.inkFaint,
    marginTop: spacing.xs,
  },
  searchCard: {
    marginTop: spacing.md,
    backgroundColor: colors.paper,
    borderRadius: radius.card,
    padding: 3,
  },
  searchInput: {
    borderWidth: 1,
    borderColor: colors.hairline,
    borderRadius: radius.button,
    backgroundColor: colors.cardSurface,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm + 2,
    fontFamily: fonts.sans,
    fontSize: 15,
    color: colors.ink,
  },
  searchInputFocused: { borderColor: colors.cobalt, borderWidth: 1.5 },
  understoodRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    alignItems: "center",
    gap: spacing.sm,
    marginTop: spacing.sm,
  },
  understoodLabel: {
    fontFamily: fonts.mono,
    fontSize: 10,
    letterSpacing: 0.6,
    textTransform: "uppercase",
    color: colors.inkFaint,
  },
  chip: {
    borderWidth: 1,
    borderColor: colors.hairline,
    borderRadius: radius.pill,
    paddingHorizontal: spacing.sm,
    paddingVertical: 2,
    backgroundColor: colors.paper,
  },
  chipText: { fontFamily: fonts.mono, fontSize: 11, color: colors.inkMuted },
  colorChip: {
    flexDirection: "row",
    alignItems: "center",
    alignSelf: "flex-start",
    gap: spacing.xs,
    marginTop: spacing.sm,
    borderWidth: 1,
    borderColor: colors.hairline,
    borderRadius: radius.pill,
    paddingHorizontal: spacing.sm,
    paddingVertical: 4,
    backgroundColor: colors.paper,
  },
  colorChipSwatch: {
    width: 12,
    height: 12,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.hairline,
  },
  colorChipText: { fontFamily: fonts.mono, fontSize: 11, color: colors.inkMuted },
  colorChipClear: { fontFamily: fonts.sans, fontSize: 15, color: colors.inkFaint, lineHeight: 16 },
  list: { paddingHorizontal: spacing.xl, paddingBottom: spacing.xxl, flexGrow: 1 },
  separator: { height: 14 },
  centre: { flex: 1, alignItems: "center", justifyContent: "center", padding: spacing.xl },
  messageText: {
    fontFamily: fonts.sans,
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
  retryButtonText: { color: colors.cobalt, fontFamily: fonts.sansSemiBold, fontSize: 14 },
});
