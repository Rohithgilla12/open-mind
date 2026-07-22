// Native item detail — the enriched card rendered in-app so a tap never needs
// a web session (the API is reached with the device key, and "Open original"
// goes to the public source URL). Mirrors the web reader's shape: kicker,
// serif title, summary lead, archived body, tags.
import { useQuery } from "@tanstack/react-query";
import { LinearGradient } from "expo-linear-gradient";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { openBrowserAsync } from "expo-web-browser";
import { useCallback, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Image,
  Linking,
  Platform,
  Pressable,
  ScrollView,
  Share,
  StyleSheet,
  Text,
  View,
} from "react-native";
import MapView, { Marker } from "react-native-maps";
import { SafeAreaView } from "react-native-safe-area-context";
import * as Clipboard from "expo-clipboard";
import { ApiError, getItem, getItemPlaces, sendItemToKindle, type ItemDetail, type Place } from "@/lib/api";
import { cardKind } from "@/lib/cards";
import { leadImageSource } from "@/lib/lead-image";
import { useDeleteItem, useKeepItem, usePinItem } from "@/lib/mutations";
import { queryKeys } from "@/lib/query";
import { useSettingsContext } from "@/lib/settings-context";
import type { Settings } from "@/lib/settings";
import { colors, fonts, radius, spacing, typeGradients, type CardKind } from "@/lib/theme";
import { stripMarkdown } from "@/lib/text";

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
  const { settings } = useSettingsContext();
  const pinItem = usePinItem();
  const keepItem = useKeepItem();
  const deleteItemFn = useDeleteItem();
  const [busy, setBusy] = useState<"pin" | "keep" | "kindle" | null>(null);

  const itemId = typeof id === "string" ? id : "";

  const itemQuery = useQuery({
    queryKey: queryKeys.item(itemId),
    enabled: itemId.length > 0,
    queryFn: async () => {
      const res = await getItem(itemId);
      if (!res.ok || !res.item) throw new ApiError(res.status);
      return res.item;
    },
    staleTime: 30_000,
  });

  const item = itemQuery.data;

  const placesQuery = useQuery({
    queryKey: queryKeys.itemPlaces(itemId),
    enabled: itemId.length > 0,
    queryFn: async () => {
      const res = await getItemPlaces(itemId);
      if (!res.ok) throw new ApiError(res.status);
      return res.places;
    },
    staleTime: 30_000,
  });

  const onPin = useCallback(() => {
    if (!item) return;
    void (async () => {
      setBusy("pin");
      // pinItem already patches the item + list caches (and rolls back on failure).
      await pinItem(item);
      setBusy(null);
    })();
  }, [item, pinItem]);

  const onKeep = useCallback(() => {
    if (!item) return;
    void (async () => {
      setBusy("keep");
      // keepItem already patches caches; do not re-flip from a stale closure.
      await keepItem(item);
      setBusy(null);
    })();
  }, [item, keepItem]);

  const onDelete = useCallback(() => {
    if (!item) return;
    deleteItemFn(item, () => {
      router.back();
    });
  }, [item, deleteItemFn, router]);

  const onCopyLink = useCallback(() => {
    if (!item?.url) return;
    void Clipboard.setStringAsync(item.url).then(() => {
      Alert.alert("Copied", "Link copied to clipboard.");
    });
  }, [item]);

  const onShare = useCallback(() => {
    if (!item?.url) return;
    void Share.share({
      message: item.title?.trim() ? `${item.title.trim()}\n${item.url}` : item.url,
      url: item.url,
    });
  }, [item]);

  const onKindle = useCallback(() => {
    if (!item) return;
    void (async () => {
      setBusy("kindle");
      const res = await sendItemToKindle(item.id);
      setBusy(null);
      if (res.ok) {
        Alert.alert("Queued", "On its way to your Kindle.");
      } else if (res.status === 409) {
        Alert.alert(
          "Kindle not set up",
          "Add your Kindle address in the web app's Settings first.",
        );
      } else if (res.status === 422) {
        Alert.alert("Nothing to send", "This item has no archived text to build an EPUB from.");
      } else {
        Alert.alert("Couldn't send", "Sending to Kindle failed — try again in a moment.");
      }
    })();
  }, [item]);

  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      <Stack.Screen options={{ headerShown: false, animation: "fade", animationDuration: 220 }} />
      <View style={styles.topbar}>
        <Pressable onPress={() => router.back()} hitSlop={12}>
          <Text style={styles.back}>‹ Back</Text>
        </Pressable>
        {item ? (
          <View style={styles.topActions}>
            <Pressable onPress={onPin} hitSlop={8} disabled={busy === "pin"}>
              <Text style={[styles.pinAction, item.pinnedAt ? styles.pinActionActive : null]}>
                {busy === "pin" ? "…" : item.pinnedAt ? "◆ Desk" : "◇ Pin"}
              </Text>
            </Pressable>
            {item.feedId ? (
              <Pressable onPress={onKeep} hitSlop={8} disabled={busy === "keep"}>
                <Text style={styles.keepAction}>
                  {busy === "keep" ? "…" : item.keptAt ? "Unkeep" : "Keep"}
                </Text>
              </Pressable>
            ) : null}
            <Pressable onPress={onDelete} hitSlop={8}>
              <Text style={styles.deleteAction}>Delete</Text>
            </Pressable>
          </View>
        ) : null}
      </View>
      <Body
        isPending={itemQuery.isPending && !item}
        error={
          itemQuery.isError
            ? itemQuery.error instanceof ApiError
              ? itemQuery.error.status === 0
                ? "Instance unreachable — check your connection."
                : itemQuery.error.status === 404
                  ? "This item no longer exists."
                  : "Couldn't load this item."
              : "Couldn't load this item."
            : null
        }
        item={item}
        settings={settings}
        places={placesQuery.data ?? []}
        onCopyLink={onCopyLink}
        onShare={onShare}
        onKindle={onKindle}
        kindleBusy={busy === "kindle"}
      />
    </SafeAreaView>
  );
}

function googleMapsSearchUrl(p: Place): string {
  const q = [p.name, p.hint].filter(Boolean).join(" ");
  return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(q)}`;
}

function PlacesSection({ places }: { places: Place[] }) {
  if (places.length === 0) return null;
  const pinned = places.filter(
    (p): p is Place & { lat: number; lng: number } =>
      typeof p.lat === "number" && typeof p.lng === "number",
  );
  const first = pinned[0];

  return (
    <View style={styles.placesSection}>
      {first ? (
        <MapView
          style={styles.placesMap}
          initialRegion={{
            latitude: first.lat,
            longitude: first.lng,
            latitudeDelta: 0.08,
            longitudeDelta: 0.08,
          }}
          scrollEnabled={false}
          zoomEnabled={false}
          pitchEnabled={false}
          rotateEnabled={false}
        >
          {pinned.map((p) => (
            <Marker key={p.id} coordinate={{ latitude: p.lat, longitude: p.lng }} title={p.name} />
          ))}
        </MapView>
      ) : null}
      {places.map((p) => {
        const hasCoords = typeof p.lat === "number" && typeof p.lng === "number";
        const onPress = () => {
          if (hasCoords) {
            const url =
              Platform.OS === "ios"
                ? `maps:0,0?q=${encodeURIComponent(p.name)}@${p.lat},${p.lng}`
                : `geo:${p.lat},${p.lng}?q=${encodeURIComponent(p.name)}`;
            void Linking.openURL(url);
          } else {
            void Linking.openURL(googleMapsSearchUrl(p));
          }
        };
        return (
          <Pressable
            key={p.id}
            style={({ pressed }) => [styles.placeRow, pressed && styles.placeRowPressed]}
            onPress={onPress}
          >
            <Text style={styles.placeName}>{p.name}</Text>
            {p.address ? <Text style={styles.placeAddress}>{p.address}</Text> : null}
          </Pressable>
        );
      })}
    </View>
  );
}

function Body({
  isPending,
  error,
  item,
  settings,
  places,
  onCopyLink,
  onShare,
  onKindle,
  kindleBusy,
}: {
  isPending: boolean;
  error: string | null;
  item?: ItemDetail;
  settings: Settings | null;
  places: Place[];
  onCopyLink: () => void;
  onShare: () => void;
  onKindle: () => void;
  kindleBusy: boolean;
}) {
  if (isPending) {
    return (
      <View style={styles.centre}>
        <ActivityIndicator color={colors.cobalt} />
      </View>
    );
  }
  if (error || !item) {
    return (
      <View style={styles.centre}>
        <Text style={styles.errorText}>{error ?? "Couldn't load this item."}</Text>
      </View>
    );
  }

  const host = hostOf(item.url);
  const kind = cardKind(item.cardType);
  const dots = item.palette ?? [];
  const kicker = [item.cardType, host].filter(Boolean).join(" · ").toUpperCase();
  const tags = [...new Set([...(item.tags ?? []), ...(item.userTags ?? [])])];
  const title = stripMarkdown(item.title);
  const summary = stripMarkdown(item.summary);
  const paragraphs = (item.body ?? "")
    .split(/\n{2,}/)
    .map((p) => stripMarkdown(p))
    .filter(Boolean);
  const imageSrc = leadImageSource(item.leadImageUrl, settings);

  if (kind === "quote") {
    return (
      <ScrollView contentContainerStyle={styles.container}>
        <View style={styles.quoteCard}>
          {kicker ? <Text style={styles.quoteKicker}>{kicker}</Text> : null}
          <Text style={styles.quoteGlyph}>&ldquo;</Text>
          <Text style={styles.quoteText}>{summary || title || ""}</Text>
          {title && summary ? <Text style={styles.quoteAttribution}>— {title}</Text> : null}
          <PaletteDots dots={dots} />
        </View>
        {item.url ? <OpenOriginalPill url={item.url} /> : null}
        {item.url ? (
          <SecondaryActions onCopyLink={onCopyLink} onShare={onShare} onKindle={onKindle} kindleBusy={kindleBusy} />
        ) : null}
        {tags.length > 0 ? <TagsRow tags={tags} /> : null}
        <PlacesSection places={places} />
      </ScrollView>
    );
  }

  const showHero = HERO_KINDS.includes(kind);
  const gradientColors: [string, string] = dots.length >= 2 ? [dots[0], dots[1]] : typeGradients[kind];

  return (
    <ScrollView contentContainerStyle={styles.container}>
      {imageSrc ? (
        <View style={styles.heroImageWrap}>
          <Image source={imageSrc} style={styles.heroImage} resizeMode="cover" accessibilityLabel={title || "Saved image"} />
        </View>
      ) : showHero ? (
        <View style={styles.hero}>
          <LinearGradient colors={gradientColors} style={StyleSheet.absoluteFill} />
        </View>
      ) : null}
      {kicker ? <Text style={styles.kicker}>{kicker}</Text> : null}
      <PaletteDots dots={dots} />
      <Text style={styles.title} numberOfLines={3}>
        {title || host || "Untitled"}
      </Text>
      {summary ? <Text style={styles.summary}>{summary}</Text> : null}
      {item.url ? <OpenOriginalPill url={item.url} /> : null}
      {item.url ? (
        <SecondaryActions onCopyLink={onCopyLink} onShare={onShare} onKindle={onKindle} kindleBusy={kindleBusy} />
      ) : null}
      {tags.length > 0 ? <TagsRow tags={tags} /> : null}
      <PlacesSection places={places} />
      {paragraphs.length > 0 ? (
        <View style={styles.bodyBlock}>
          {paragraphs.map((p, i) => (
            <Text key={i} style={styles.paragraph}>
              {p}
            </Text>
          ))}
        </View>
      ) : kind === "image" && imageSrc ? null : (
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

function SecondaryActions({
  onCopyLink,
  onShare,
  onKindle,
  kindleBusy,
}: {
  onCopyLink: () => void;
  onShare: () => void;
  onKindle: () => void;
  kindleBusy: boolean;
}) {
  return (
    <View style={styles.secondaryRow}>
      <Pressable onPress={onCopyLink} hitSlop={8} style={styles.secondaryPill}>
        <Text style={styles.secondaryText}>Copy link</Text>
      </Pressable>
      <Pressable onPress={onKindle} hitSlop={8} style={styles.secondaryPill} disabled={kindleBusy}>
        <Text style={styles.secondaryText}>{kindleBusy ? "Sending…" : "Send to Kindle"}</Text>
      </Pressable>
      <Pressable onPress={onShare} hitSlop={8} style={styles.secondaryPill}>
        <Text style={styles.secondaryText}>Share</Text>
      </Pressable>
    </View>
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
  topbar: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingHorizontal: spacing.xl,
    paddingVertical: spacing.md,
  },
  topActions: { flexDirection: "row", alignItems: "center", gap: spacing.md },
  back: { color: colors.cobalt, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  pinAction: { color: colors.inkMuted, fontFamily: fonts.mono, fontSize: 13 },
  pinActionActive: { color: colors.gold },
  deleteAction: { color: colors.danger, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  keepAction: { color: colors.cobalt, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  container: { paddingHorizontal: spacing.xl, paddingBottom: spacing.xxl },
  centre: { flex: 1, alignItems: "center", justifyContent: "center", padding: spacing.xl },
  errorText: { fontFamily: fonts.sans, fontSize: 14, color: colors.inkMuted, textAlign: "center", lineHeight: 20 },
  hero: {
    height: 120,
    borderRadius: radius.card,
    overflow: "hidden",
    marginBottom: spacing.lg,
  },
  heroImageWrap: {
    borderRadius: radius.card,
    overflow: "hidden",
    marginBottom: spacing.lg,
    backgroundColor: colors.canvas,
  },
  heroImage: {
    width: "100%",
    aspectRatio: 4 / 3,
    backgroundColor: colors.canvas,
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
    marginBottom: spacing.sm,
  },
  openOriginalText: { color: colors.paper, fontFamily: fonts.sansSemiBold, fontSize: 13.5 },
  secondaryRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: spacing.sm,
    marginBottom: spacing.lg,
  },
  secondaryPill: {
    borderWidth: 1,
    borderColor: colors.hairline,
    borderRadius: radius.pill,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    backgroundColor: colors.canvas,
  },
  secondaryText: { fontFamily: fonts.sansMedium, fontSize: 13, color: colors.inkMuted },
  tagsRow: { flexDirection: "row", flexWrap: "wrap", gap: spacing.sm, marginBottom: spacing.lg },
  placesSection: { marginBottom: spacing.lg },
  placesMap: { height: 160, borderRadius: radius.card, marginBottom: spacing.md },
  placeRow: {
    borderRadius: radius.card,
    borderWidth: 1,
    borderColor: colors.hairline,
    backgroundColor: colors.cardSurface,
    padding: spacing.md,
    marginBottom: spacing.sm,
  },
  placeRowPressed: { opacity: 0.7 },
  placeName: { fontFamily: fonts.sansSemiBold, fontSize: 14, color: colors.ink },
  placeAddress: { fontFamily: fonts.sans, fontSize: 12, color: colors.inkMuted, marginTop: 2 },
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
