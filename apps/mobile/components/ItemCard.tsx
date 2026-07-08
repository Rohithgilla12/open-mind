import { LinearGradient } from "expo-linear-gradient";
import { useEffect, useRef, useState } from "react";
import { AccessibilityInfo, Animated, Image, Pressable, StyleSheet, Text, View } from "react-native";
import type { Item } from "@/lib/api";
import type { Settings } from "@/lib/settings";
import { colors, fonts, radius, spacing, type, typeGradients, type CardKind } from "@/lib/theme";

/** Extract a bare host (no scheme, no www., no path) from a URL, or null. */
function hostOf(url?: string): string | null {
  if (!url) return null;
  const match = /^[a-z]+:\/\/([^/?#]+)/i.exec(url.trim());
  if (!match) return null;
  return match[1].replace(/^www\./i, "");
}

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

/** Normalise a raw cardType into a known kind; unknown/absent → article. */
function cardKind(cardType: string | undefined): CardKind {
  if (cardType && (KNOWN_KINDS as readonly string[]).includes(cardType)) {
    return cardType as CardKind;
  }
  return "article";
}

const typeLabel: Record<CardKind, string> = {
  article: "Article",
  quote: "Quote",
  image: "Image",
  product: "Product",
  note: "Note",
  video: "Video",
  tweet: "Post",
  book: "Book",
  recipe: "Recipe",
};

/**
 * Resolve an item's leadImageUrl into an <Image> source. An instance-relative
 * path (`/assets/<id>`) needs the instance URL prefix plus a bearer header;
 * an absolute URL (http...) is already public and renders as-is.
 */
function imageSource(
  leadImageUrl: string | undefined,
  settings: Settings | null,
): { uri: string; headers?: Record<string, string> } | undefined {
  if (!leadImageUrl) return undefined;
  if (leadImageUrl.startsWith("/assets/")) {
    if (!settings) return undefined;
    return {
      uri: `${settings.instanceUrl}${leadImageUrl}`,
      headers: { Authorization: `Bearer ${settings.token}` },
    };
  }
  return { uri: leadImageUrl };
}

/** Palette dots — the signature brand detail. Max 5, 9px, inset hairline ring. */
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

/** "ENRICHING…" pending indicator with a soft opacity pulse (skipped under reduce-motion). */
function EnrichingLabel() {
  const opacity = useRef(new Animated.Value(1)).current;

  useEffect(() => {
    let cancelled = false;
    let loop: Animated.CompositeAnimation | null = null;

    void AccessibilityInfo.isReduceMotionEnabled().then((reduced) => {
      if (cancelled || reduced) return;
      loop = Animated.loop(
        Animated.sequence([
          Animated.timing(opacity, { toValue: 0.55, duration: 700, useNativeDriver: true }),
          Animated.timing(opacity, { toValue: 1, duration: 700, useNativeDriver: true }),
        ]),
      );
      loop.start();
    });

    return () => {
      cancelled = true;
      loop?.stop();
    };
  }, [opacity]);

  return (
    <Animated.Text style={[styles.enriching, { opacity }]}>ENRICHING…</Animated.Text>
  );
}

function MetaLine({ label, domain }: { label: string; domain: string | null }) {
  const text = domain ? `${label.toUpperCase()} · ${domain}` : label.toUpperCase();
  return <Text style={styles.meta}>{text}</Text>;
}

function Hero({
  kind,
  dots,
  source,
  height,
}: {
  kind: CardKind;
  dots: string[];
  source?: { uri: string; headers?: Record<string, string> };
  height: number;
}) {
  const gradientColors: [string, string] = dots.length >= 2 ? [dots[0], dots[1]] : typeGradients[kind];
  return (
    <View style={[styles.hero, { height }]}>
      <LinearGradient colors={gradientColors} style={StyleSheet.absoluteFill} />
      {source ? (
        <Image source={source} style={StyleSheet.absoluteFill} resizeMode="cover" />
      ) : null}
    </View>
  );
}

type ItemCardProps = {
  item: Item;
  settings: Settings | null;
  onPress: (item: Item) => void;
};

export function ItemCard({ item, settings, onPress }: ItemCardProps) {
  const kind = cardKind(item.cardType);
  const pending = item.status === "pending";
  const domain = hostOf(item.url);
  const dots = item.palette ?? [];
  const source = imageSource(item.leadImageUrl, settings);
  const title = item.title?.trim() || domain || item.url || "Untitled";

  const press = () => onPress(item);
  const wrap = (children: React.ReactNode) => (
    <Pressable
      style={({ pressed }) => [styles.card, pressed && styles.cardPressed]}
      onPress={press}
    >
      {children}
    </Pressable>
  );

  if (kind === "quote") {
    const text = item.summary ?? item.title ?? "";
    const attribution = item.summary && item.title ? item.title : null;
    return wrap(
      <View style={[styles.body, { backgroundColor: colors.ink, borderRadius: radius.card }]}>
        <Text style={styles.quoteGlyph}>&ldquo;</Text>
        <Text style={styles.quoteText} numberOfLines={5}>
          {text}
        </Text>
        <Text style={styles.quoteAttribution}>{attribution ? `${attribution} — Quote` : "Quote"}</Text>
        <PaletteDots dots={dots} />
        {pending ? <EnrichingLabel /> : null}
      </View>,
    );
  }

  if (kind === "note") {
    const text = item.summary ?? item.title ?? "Untitled note";
    return wrap(
      <View style={[styles.body, { backgroundColor: colors.note, borderRadius: radius.card }]}>
        <Text style={styles.noteKicker}>NOTE</Text>
        <Text style={styles.noteText} numberOfLines={8}>
          {text}
        </Text>
        <View style={styles.footerRow}>
          <PaletteDots dots={dots} />
          <MetaLine label={typeLabel.note} domain={domain} />
        </View>
        {pending ? <EnrichingLabel /> : null}
      </View>,
    );
  }

  if (kind === "image") {
    return wrap(
      <View>
        <Hero kind={kind} dots={dots} source={source} height={180} />
        <View style={styles.body}>
          {item.title ? (
            <Text style={styles.cardTitle} numberOfLines={2}>
              {item.title}
            </Text>
          ) : null}
          <View style={styles.footerRow}>
            <PaletteDots dots={dots} />
            <MetaLine label={typeLabel.image} domain={domain} />
          </View>
          {pending ? <EnrichingLabel /> : null}
        </View>
      </View>,
    );
  }

  // article / product / book / recipe / video / tweet — hero + serif title + summary.
  return wrap(
    <View>
      <Hero kind={kind} dots={dots} source={source} height={84} />
      <View style={styles.body}>
        <Text style={styles.cardTitle} numberOfLines={2}>
          {title}
        </Text>
        {item.summary ? (
          <Text style={styles.summary} numberOfLines={2}>
            {item.summary}
          </Text>
        ) : null}
        <View style={styles.footerRow}>
          <PaletteDots dots={dots} />
          <MetaLine label={typeLabel[kind]} domain={domain} />
        </View>
        {pending ? <EnrichingLabel /> : null}
      </View>
    </View>,
  );
}

const styles = StyleSheet.create({
  card: {
    borderRadius: radius.card,
    borderWidth: 1,
    borderColor: colors.hairline,
    backgroundColor: colors.cardSurface,
    overflow: "hidden",
    shadowColor: colors.ink,
    shadowOpacity: 0.08,
    shadowRadius: 8,
    shadowOffset: { width: 0, height: 3 },
    elevation: 2,
  },
  cardPressed: { opacity: 0.85 },
  hero: { width: "100%", overflow: "hidden" },
  body: { padding: spacing.md, gap: 4 },
  cardTitle: { fontFamily: fonts.serifBold, fontSize: type.cardTitle.fontSize, color: colors.ink, lineHeight: 22 },
  summary: { fontFamily: fonts.sans, fontSize: 12.5, color: colors.inkMuted, lineHeight: 17 },
  meta: {
    fontFamily: fonts.mono,
    fontSize: type.meta.fontSize,
    color: colors.inkFaint,
    letterSpacing: 0.4,
    marginLeft: "auto",
  },
  footerRow: { flexDirection: "row", alignItems: "center", gap: spacing.sm, marginTop: 6 },
  dotsRow: { flexDirection: "row", gap: 5 },
  dot: {
    width: 9,
    height: 9,
    borderRadius: 4.5,
    borderWidth: 1,
    borderColor: colors.hairline,
  },
  enriching: {
    fontFamily: fonts.mono,
    fontSize: 10,
    letterSpacing: 0.4,
    color: colors.cobalt,
    marginTop: 6,
  },
  quoteGlyph: { fontFamily: fonts.serifBold, fontSize: 34, color: colors.gold, lineHeight: 34 },
  quoteText: {
    fontFamily: fonts.serif,
    fontSize: 18,
    fontStyle: "italic",
    lineHeight: 24,
    color: colors.paper,
    marginTop: 6,
  },
  quoteAttribution: { fontFamily: fonts.mono, fontSize: 10, color: colors.inkFaint, marginTop: 12 },
  noteKicker: { fontFamily: fonts.monoMedium, fontSize: 10, letterSpacing: 0.8, color: colors.gold },
  noteText: { fontFamily: fonts.serif, fontSize: 15, lineHeight: 21, color: colors.ink, marginTop: 6 },
});
