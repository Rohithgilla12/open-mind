// Native item detail — the enriched card rendered in-app so a tap never needs
// a web session (the API is reached with the device key, and "Open original"
// goes to the public source URL). Mirrors the web reader's shape: kicker,
// serif title, summary lead, archived body, tags.
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { openBrowserAsync } from "expo-web-browser";
import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { getItem, type ItemDetail } from "@/lib/api";
import { colors, fonts, radius, spacing } from "@/lib/theme";

type State =
  | { kind: "loading" }
  | { kind: "ready"; item: ItemDetail }
  | { kind: "error"; message: string };

function hostOf(url?: string): string {
  if (!url) return "";
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return "";
  }
}

export default function ItemScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const [state, setState] = useState<State>({ kind: "loading" });

  useEffect(() => {
    if (typeof id !== "string") return;
    let cancelled = false;
    void (async () => {
      const res = await getItem(id);
      if (cancelled) return;
      if (res.ok && res.item) {
        setState({ kind: "ready", item: res.item });
      } else if (res.status === 0) {
        setState({ kind: "error", message: "Instance unreachable — check your connection." });
      } else if (res.status === 404) {
        setState({ kind: "error", message: "This item no longer exists." });
      } else {
        setState({ kind: "error", message: "Couldn't load this item." });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id]);

  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      <Stack.Screen options={{ headerShown: false }} />
      <View style={styles.topbar}>
        <Pressable onPress={() => router.back()} hitSlop={12}>
          <Text style={styles.back}>‹ Back</Text>
        </Pressable>
      </View>
      <Body state={state} />
    </SafeAreaView>
  );
}

function Body({ state }: { state: State }) {
  if (state.kind === "loading") {
    return (
      <View style={styles.centre}>
        <ActivityIndicator color={colors.cobalt} />
      </View>
    );
  }
  if (state.kind === "error") {
    return (
      <View style={styles.centre}>
        <Text style={styles.errorText}>{state.message}</Text>
      </View>
    );
  }

  const item = state.item;
  const host = hostOf(item.url);
  const kicker = [item.cardType, host].filter(Boolean).join(" · ").toUpperCase();
  const tags = [...new Set([...(item.tags ?? []), ...(item.userTags ?? [])])];
  const paragraphs = (item.body ?? "")
    .split(/\n{2,}/)
    .map((p) => p.trim())
    .filter(Boolean);

  return (
    <ScrollView contentContainerStyle={styles.container}>
      {kicker ? <Text style={styles.kicker}>{kicker}</Text> : null}
      <Text style={styles.title}>{item.title || host || "Untitled"}</Text>
      {item.summary ? <Text style={styles.summary}>{item.summary}</Text> : null}
      {item.url ? (
        <Pressable onPress={() => void openBrowserAsync(item.url)} hitSlop={8}>
          <Text style={styles.openOriginal}>Open original ↗</Text>
        </Pressable>
      ) : null}
      {tags.length > 0 ? (
        <View style={styles.tagsRow}>
          {tags.map((t) => (
            <View key={t} style={styles.tag}>
              <Text style={styles.tagText}>{t}</Text>
            </View>
          ))}
        </View>
      ) : null}
      {paragraphs.length > 0 ? (
        <View style={styles.bodyBlock}>
          {paragraphs.map((p, i) => (
            <Text key={i} style={styles.paragraph}>
              {p}
            </Text>
          ))}
        </View>
      ) : (
        <Text style={styles.noBody}>
          No archived text for this item{item.status === "pending" ? " yet — still enriching" : ""}.
        </Text>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.paper },
  topbar: { paddingHorizontal: spacing.xl, paddingVertical: spacing.md },
  back: { color: colors.cobalt, fontSize: 15, fontWeight: "600" },
  container: { paddingHorizontal: spacing.xl, paddingBottom: spacing.xxl },
  centre: { flex: 1, alignItems: "center", justifyContent: "center", padding: spacing.xl },
  errorText: { fontSize: 14, color: colors.inkMuted, textAlign: "center", lineHeight: 20 },
  kicker: {
    fontFamily: fonts.mono,
    fontSize: 10,
    letterSpacing: 0.8,
    color: colors.inkFaint,
    marginBottom: spacing.sm,
  },
  title: {
    fontFamily: fonts.serif,
    fontSize: 26,
    fontWeight: "600",
    color: colors.ink,
    lineHeight: 33,
    marginBottom: spacing.md,
  },
  summary: {
    fontFamily: fonts.serif,
    fontStyle: "italic",
    fontSize: 16.5,
    lineHeight: 25,
    color: colors.inkMuted,
    marginBottom: spacing.md,
  },
  openOriginal: { color: colors.cobalt, fontSize: 14.5, fontWeight: "600", marginBottom: spacing.lg },
  tagsRow: { flexDirection: "row", flexWrap: "wrap", gap: spacing.sm, marginBottom: spacing.lg },
  tag: {
    backgroundColor: colors.canvas,
    borderRadius: radius.button,
    paddingHorizontal: spacing.md,
    paddingVertical: 4,
  },
  tagText: { fontFamily: fonts.mono, fontSize: 11, color: colors.inkMuted },
  bodyBlock: { gap: spacing.md },
  paragraph: { fontSize: 15.5, lineHeight: 25, color: colors.ink },
  noBody: { fontSize: 14, color: colors.inkFaint, fontStyle: "italic" },
});
