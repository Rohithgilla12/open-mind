import { LinearGradient } from "expo-linear-gradient";
import { useEffect, useRef, useState } from "react";
import { AccessibilityInfo, Animated, Image, Pressable, StyleSheet, Text, View } from "react-native";
import type { Item } from "@/lib/api";
import type { MorphRect } from "@/lib/morph";
import { cardKind, typeLabel } from "@/lib/cards";
import { leadImageSource } from "@/lib/lead-image";
import type { Settings } from "@/lib/settings";
import { colors, fonts, radius, spacing, type, typeGradients, type CardKind } from "@/lib/theme";
import { stripMarkdown } from "@/lib/text";

/** Extract a bare host (no scheme, no www., no path) from a URL, or null. */
function hostOf(url?: string): string | null {
  if (!url) return null;
  const match = /^[a-z]+:\/\/([^/?#]+)/i.exec(url.trim());
  if (!match) return null;
  return match[1].replace(/^www\./i, "");
}

/** Palette dots — the signature brand detail. Max 5, 9px, inset hairline ring. */
function PaletteDots({ dots, onPickColor }: { dots: string[]; onPickColor?: (hex: string) => void }) {
  if (dots.length === 0) return null;
  return (
    <View style={styles.dotsRow}>
      {dots.slice(0, 5).map((c, i) =>
        onPickColor ? (
          <Pressable
            key={`${c}-${i}`}
            hitSlop={6}
            accessibilityRole="button"
            accessibilityLabel={`Find saves matching ${c}`}
            onPress={() => onPickColor(c)}
            style={[styles.dot, { backgroundColor: c }]}
          />
        ) : (
          <View key={`${c}-${i}`} style={[styles.dot, { backgroundColor: c }]} />
        ),
      )}
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
  innerRef,
}: {
  kind: CardKind;
  dots: string[];
  source?: { uri: string; headers?: Record<string, string> };
  height: number;
  innerRef?: React.Ref<View>;
}) {
  const gradientColors: [string, string] = dots.length >= 2 ? [dots[0], dots[1]] : typeGradients[kind];
  return (
    <View ref={innerRef} style={[styles.hero, { height }]}>
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
  /** Long-press for the action sheet — omit to disable. */
  onLongPress?: (item: Item) => void;
  /** Tapping a palette dot filters the Library by that colour — omit to disable. */
  onPickColor?: (hex: string) => void;
  /** When set, press measures the card's window rect and reports it for the
   * card→detail morph (the handler also navigates). Falls back to onPress. */
  onMorphPress?: (item: Item, rect: MorphRect) => void;
};

export function ItemCard({ item, settings, onPress, onLongPress, onPickColor, onMorphPress }: ItemCardProps) {
  const heroRef = useRef<View>(null);
  const kind = cardKind(item.cardType);
  const pending = item.status === "pending";
  const pinned = !!item.pinnedAt;
  const domain = hostOf(item.url);
  const dots = item.palette ?? [];
  const source = leadImageSource(item.leadImageUrl, settings);
  const rawTitle = stripMarkdown(item.title?.trim());
  const title = rawTitle || domain || item.url || "Untitled";
  const summary = stripMarkdown(item.summary);

  // Quote/note cards have no gradient hero on the detail screen, so only
  // hero-bearing kinds morph; the rest navigate plainly.
  const morphable = kind !== "quote" && kind !== "note";
  const press = () => {
    if (onMorphPress && morphable && heroRef.current) {
      heroRef.current.measureInWindow((x, y, width, height) => {
        onMorphPress(item, { x, y, width, height });
      });
      return;
    }
    onPress(item);
  };
  const wrap = (children: React.ReactNode) => (
    <Pressable
      style={({ pressed }) => [styles.card, pressed && styles.cardPressed]}
      onPress={press}
      onLongPress={onLongPress ? () => onLongPress(item) : undefined}
      delayLongPress={350}
    >
      {children}
      {pinned ? (
        <View style={styles.pinBadge} accessibilityLabel="On desk">
          <Text style={styles.pinBadgeText}>◆</Text>
        </View>
      ) : null}
    </Pressable>
  );

  if (kind === "quote") {
    const text = summary || rawTitle;
    const attribution = summary && rawTitle ? rawTitle : null;
    return wrap(
      <View style={[styles.body, { backgroundColor: colors.ink, borderRadius: radius.card }]}>
        <Text style={styles.quoteGlyph}>&ldquo;</Text>
        <Text style={styles.quoteText} numberOfLines={5}>
          {text}
        </Text>
        <Text style={styles.quoteAttribution}>{attribution ? `${attribution} — Quote` : "Quote"}</Text>
        <PaletteDots dots={dots} onPickColor={onPickColor} />
        {pending ? <EnrichingLabel /> : null}
      </View>,
    );
  }

  if (kind === "note") {
    const text = summary || rawTitle || "Untitled note";
    return wrap(
      <View style={[styles.body, { backgroundColor: colors.note, borderRadius: radius.card }]}>
        <Text style={styles.noteKicker}>NOTE</Text>
        <Text style={styles.noteText} numberOfLines={8}>
          {text}
        </Text>
        <View style={styles.footerRow}>
          <PaletteDots dots={dots} onPickColor={onPickColor} />
          <MetaLine label={typeLabel.note} domain={domain} />
        </View>
        {pending ? <EnrichingLabel /> : null}
      </View>,
    );
  }

  if (kind === "image") {
    return wrap(
      <View>
        <Hero kind={kind} dots={dots} source={source} height={180} innerRef={heroRef} />
        <View style={styles.body}>
          {rawTitle ? (
            <Text style={styles.cardTitle} numberOfLines={2}>
              {rawTitle}
            </Text>
          ) : null}
          <View style={styles.footerRow}>
            <PaletteDots dots={dots} onPickColor={onPickColor} />
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
      <Hero kind={kind} dots={dots} source={source} height={84} innerRef={heroRef} />
      <View style={styles.body}>
        <Text style={styles.cardTitle} numberOfLines={2}>
          {title}
        </Text>
        {summary ? (
          <Text style={styles.summary} numberOfLines={2}>
            {summary}
          </Text>
        ) : null}
        <View style={styles.footerRow}>
          <PaletteDots dots={dots} onPickColor={onPickColor} />
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
  pinBadge: {
    position: "absolute",
    top: spacing.sm,
    right: spacing.sm,
    width: 22,
    height: 22,
    borderRadius: 11,
    backgroundColor: colors.paper,
    borderWidth: 1,
    borderColor: colors.hairline,
    alignItems: "center",
    justifyContent: "center",
  },
  pinBadgeText: { fontSize: 11, color: colors.gold, lineHeight: 14 },
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
