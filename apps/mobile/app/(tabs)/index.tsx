import { Redirect, useFocusEffect, useRouter } from "expo-router";
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
import { ItemRow } from "@/components/ItemRow";
import { listItems, type Item } from "@/lib/api";
import { useSettingsContext } from "@/lib/settings-context";
import { colors, fonts, radius, spacing } from "@/lib/theme";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; items: Item[] }
  | { kind: "unreachable" }
  | { kind: "rejected" }
  | { kind: "error" };

export default function LibraryScreen() {
  const router = useRouter();
  const { settings, configured, loading } = useSettingsContext();
  const [state, setState] = useState<LoadState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(
    async (isRefresh: boolean) => {
      if (!settings) return;
      if (isRefresh) setRefreshing(true);
      else setState({ kind: "loading" });
      const res = await listItems(50);
      if (res.ok) {
        setState({ kind: "ready", items: res.items });
      } else if (res.status === 0) {
        setState({ kind: "unreachable" });
      } else if (res.status === 401) {
        setState({ kind: "rejected" });
      } else {
        setState({ kind: "error" });
      }
      if (isRefresh) setRefreshing(false);
    },
    [settings],
  );

  useFocusEffect(
    useCallback(() => {
      if (settings) void load(false);
    }, [settings, load]),
  );

  const onOpen = useCallback(
    (item: Item) => {
      router.push(`/item/${item.id}`);
    },
    [router],
  );

  // Unconfigured guard: with no token stored, land on Settings.
  if (!loading && !configured) return <Redirect href="/settings" />;

  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      <View style={styles.header}>
        <Text style={styles.title}>Library</Text>
        <Text style={styles.subtitle}>Your gatherings, organised by the machine</Text>
      </View>
      <Body
        state={state}
        refreshing={refreshing}
        onRefresh={() => void load(true)}
        onRetry={() => void load(false)}
        onOpen={onOpen}
      />
    </SafeAreaView>
  );
}

type BodyProps = {
  state: LoadState;
  refreshing: boolean;
  onRefresh: () => void;
  onRetry: () => void;
  onOpen: (item: Item) => void;
};

function Body({ state, refreshing, onRefresh, onRetry, onOpen }: BodyProps) {
  if (state.kind === "loading") {
    return (
      <View style={styles.centre}>
        <ActivityIndicator color={colors.cobalt} />
      </View>
    );
  }

  if (state.kind === "unreachable") {
    return (
      <Message
        text="Instance unreachable — check your connection or the URL in Settings."
        onRetry={onRetry}
      />
    );
  }
  if (state.kind === "rejected") {
    return <Message text="Token rejected — check Settings." onRetry={onRetry} />;
  }
  if (state.kind === "error") {
    return <Message text="Couldn't load your library." onRetry={onRetry} />;
  }

  return (
    <FlatList
      data={state.items}
      keyExtractor={(item) => item.id}
      renderItem={({ item }) => <ItemRow item={item} onPress={onOpen} />}
      contentContainerStyle={styles.list}
      ItemSeparatorComponent={() => <View style={styles.separator} />}
      refreshControl={
        <RefreshControl
          refreshing={refreshing}
          onRefresh={onRefresh}
          tintColor={colors.cobalt}
        />
      }
      ListEmptyComponent={<Message text="Nothing saved yet — capture something." />}
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
  safe: { flex: 1, backgroundColor: colors.paper },
  header: { paddingHorizontal: spacing.xl, paddingTop: spacing.lg, paddingBottom: spacing.md },
  title: { fontFamily: fonts.serif, fontSize: 27, fontWeight: "600", color: colors.ink },
  subtitle: {
    fontFamily: fonts.mono,
    fontSize: 12,
    color: colors.inkFaint,
    marginTop: spacing.xs,
  },
  list: { paddingHorizontal: spacing.xl, paddingBottom: spacing.xxl, flexGrow: 1 },
  separator: { height: spacing.md },
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
});
