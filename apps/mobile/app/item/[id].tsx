// Native item detail — the enriched card rendered in-app so a tap never needs
// a web session (the API is reached with the device key, and "Open original"
// goes to the public source URL). Mirrors the web reader's shape: kicker,
// serif title, summary lead, archived body, tags.
import { LinearGradient } from "expo-linear-gradient";
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
import { colors, fonts, radius, spacing, typeGradients, type CardKind } from "@/lib/theme";

type State =
  | { kind: "loading" }
  | { kind: "ready"; item: ItemDetail }
  | { kind: "error"; message: string };

const KNOWN_KINDS: readonly CardKind[] = [
  "article",
  "quote",
  "image",
  "product",
  "note",
  "video",
  "tweet",
  "book",
  "recipe",
];

function cardKind(cardType: string | undefined): CardKind {
  if (cardType && (KNOWN_KINDS as readonly string[]).includes(cardType)) {
    return cardType as CardKind;
  }
  return "article";
}

/** Types that get a gradient hero wash on the detail screen. */
const HERO_KINDS: readonly CardKind[] = ["article", "image", "product", "book", "recipe", "video", "tweet"];

function hostOf(url?: string): string {
  if (!url) return "";
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return "";
  }
}

/** Palette dots — same signature detail as ItemCard. Max 5, 9px, hairline ring. */
function PaletteDots({ dots }: { dots: string[] }) {
  if (dots.length === 0) return null;
  return (
    <View style={styles.dotsRow}>
      {dots.slice(0, 5).map((c, i) => (
        <View key={`${c}-${i}`} style={[styles.dot, { backgroundColor: c }]} />
      ))}
    </View>
  );
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
  const kind = cardKind(item.cardType);
  const dots = item.palette ?? [];
  const kicker = [item.cardType, host].filter(Boolean).join(" · ").toUpperCase();
  const tags = [...new Set([...(item.tags ?? []), ...(item.userTags ?? [])])];
  const paragraphs = (item.body ?? "")
    .split(/\n{2,}/)
    .map((p) => p.trim())
    .filter(Boolean);

  if (kind === "quote") {
    return (
      <ScrollView contentContainerStyle={styles.container}>
        <View style={styles.quoteCard}>
          {kicker ? <Text style={styles.quoteKicker}>{kicker}</Text> : null}
          <Text style={styles.quoteGlyph}>&ldquo;</Text>
          <Text style={styles.quoteText}>{item.summary || item.title || ""}</Text>
          {item.title && item.summary ? (
            <Text style={styles.quoteAttribution}>— {item.title}</Text>
          ) : null}
          <PaletteDots dots={dots} />
        </View>
        {item.url ? <OpenOriginalPill url={item.url} /> : null}
        {tags.length > 0 ? <TagsRow tags={tags} /> : null}
      </ScrollView>
    );
  }

  const showHero = HERO_KINDS.includes(kind);
  const gradientColors: [string, string] = dots.length >= 2 ? [dots[0], dots[1]] : typeGradients[kind];

  return (
    <ScrollView contentContainerStyle={styles.container}>
      {showHero ? (
        <View style={styles.hero}>
          <LinearGradient colors={gradientColors} style={StyleSheet.absoluteFill} />
        </View>
      ) : null}
      {kicker ? <Text style={styles.kicker}>{kicker}</Text> : null}
      <PaletteDots dots={dots} />
      <Text style={styles.title}>{item.title || host || "Untitled"}</Text>
      {item.summary ? <Text style={styles.summary}>{item.summary}</Text> : null}
      {item.url ? <OpenOriginalPill url={item.url} /> : null}
      {tags.length > 0 ? <TagsRow tags={tags} /> : null}
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

function OpenOriginalPill({ url }: { url: string }) {
  return (
    <Pressable onPress={() => void openBrowserAsync(url)} hitSlop={8} style={styles.openOriginalPill}>
      <Text style={styles.openOriginalText}>Open original ↗</Text>
    </Pressable>
  );
}

function TagsRow({ tags }: { tags: string[] }) {
  return (
    <View style={styles.tagsRow}>
      {tags.map((t) => (
        <View key={t} style={styles.tag}>
          <Text style={styles.tagText}>{t}</Text>
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.paper },
  topbar: { paddingHorizontal: spacing.xl, paddingVertical: spacing.md },
  back: { color: colors.cobalt, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  container: { paddingHorizontal: spacing.xl, paddingBottom: spacing.xxl },
  centre: { flex: 1, alignItems: "center", justifyContent: "center", padding: spacing.xl },
  errorText: { fontFamily: fonts.sans, fontSize: 14, color: colors.inkMuted, textAlign: "center", lineHeight: 20 },
  hero: {
    height: 120,
    borderRadius: radius.card,
    overflow: "hidden",
    marginBottom: spacing.lg,
  },
  kicker: {
    fontFamily: fonts.mono,
    fontSize: 10,
    letterSpacing: 0.8,
    color: colors.inkFaint,
    marginBottom: spacing.sm,
  },
  dotsRow: { flexDirection: "row", gap: 5, marginBottom: spacing.md },
  dot: {
    width: 9,
    height: 9,
    borderRadius: 4.5,
    borderWidth: 1,
    borderColor: colors.hairline,
  },
  title: {
    fontFamily: fonts.serifBold,
    fontSize: 27,
    color: colors.ink,
    lineHeight: 34,
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
  openOriginalPill: {
    alignSelf: "flex-start",
    backgroundColor: colors.cobalt,
    borderRadius: radius.pill,
    paddingHorizontal: spacing.lg,
    paddingVertical: spacing.sm,
    marginBottom: spacing.lg,
  },
  openOriginalText: { color: colors.paper, fontFamily: fonts.sansSemiBold, fontSize: 13.5 },
  tagsRow: { flexDirection: "row", flexWrap: "wrap", gap: spacing.sm, marginBottom: spacing.lg },
  tag: {
    backgroundColor: colors.canvas,
    borderRadius: radius.button,
    paddingHorizontal: spacing.md,
    paddingVertical: 4,
  },
  tagText: { fontFamily: fonts.mono, fontSize: 11, color: colors.inkMuted },
  bodyBlock: { gap: spacing.md },
  paragraph: { fontFamily: fonts.sans, fontSize: 15.5, lineHeight: 25, color: colors.ink },
  noBody: { fontFamily: fonts.sans, fontSize: 14, color: colors.inkFaint, fontStyle: "italic" },
  quoteCard: {
    backgroundColor: colors.ink,
    borderRadius: radius.card,
    padding: spacing.xl,
    marginBottom: spacing.lg,
  },
  quoteKicker: {
    fontFamily: fonts.monoMedium,
    fontSize: 10,
    letterSpacing: 0.8,
    color: colors.inkFaint,
    marginBottom: spacing.sm,
  },
  quoteGlyph: { fontFamily: fonts.serifBold, fontSize: 44, color: colors.gold, lineHeight: 44 },
  quoteText: {
    fontFamily: fonts.serif,
    fontStyle: "italic",
    fontSize: 20,
    lineHeight: 28,
    color: colors.paper,
    marginTop: spacing.sm,
  },
  quoteAttribution: {
    fontFamily: fonts.mono,
    fontSize: 11,
    color: colors.inkFaint,
    marginTop: spacing.lg,
  },
});
