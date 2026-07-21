// Stack screen: every place the pipeline has extracted, plotted on a map with
// coordinate-less places listed below (opened via a Google Maps search URL).
import { useQuery } from "@tanstack/react-query";
import * as Location from "expo-location";
import { Stack, useRouter } from "expo-router";
import { useMemo, useRef, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Linking,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from "react-native";
import MapView, { Marker, type Region } from "react-native-maps";
import { SafeAreaView, useSafeAreaInsets } from "react-native-safe-area-context";
import { ApiError, listPlaces, type PlaceWithItem } from "@/lib/api";
import { buildIndex, clustersForRegion, expansionRegion } from "@/lib/cluster";
import { queryKeys } from "@/lib/query";
import { colors, fonts, radius, spacing } from "@/lib/theme";

const WORLD_REGION: Region = {
  latitude: 20,
  longitude: 0,
  latitudeDelta: 90,
  longitudeDelta: 90,
};

const NEAR_ME_DELTA = 0.05;

function hasCoords(p: PlaceWithItem): p is PlaceWithItem & { lat: number; lng: number } {
  return typeof p.lat === "number" && typeof p.lng === "number";
}

function googleMapsSearchUrl(p: PlaceWithItem): string {
  const q = [p.name, p.hint].filter(Boolean).join(" ");
  return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(q)}`;
}

export default function PlacesScreen() {
  const router = useRouter();

  const placesQuery = useQuery({
    queryKey: queryKeys.places,
    queryFn: async () => {
      const res = await listPlaces();
      if (!res.ok) throw new ApiError(res.status);
      return res.places;
    },
  });

  const places = placesQuery.data ?? [];
  const pinned = useMemo(() => places.filter(hasCoords), [places]);
  const unpinned = useMemo(() => places.filter((p) => !hasCoords(p)), [places]);

  const initialRegion: Region = pinned.length
    ? { latitude: pinned[0].lat, longitude: pinned[0].lng, latitudeDelta: 0.3, longitudeDelta: 0.3 }
    : WORLD_REGION;

  const errStatus = placesQuery.error instanceof ApiError ? placesQuery.error.status : undefined;

  return (
    <SafeAreaView style={styles.safe} edges={["left", "right", "bottom"]}>
      <Stack.Screen options={{ headerShown: false }} />
      <Body
        isPending={placesQuery.isPending && !placesQuery.data}
        isError={placesQuery.isError}
        errStatus={errStatus}
        pinned={pinned}
        unpinned={unpinned}
        initialRegion={initialRegion}
        onRetry={() => void placesQuery.refetch()}
        onOpenItem={(id) => router.push(`/item/${id}`)}
        onBack={() => router.back()}
      />
    </SafeAreaView>
  );
}

function Body({
  isPending,
  isError,
  errStatus,
  pinned,
  unpinned,
  initialRegion,
  onRetry,
  onOpenItem,
  onBack,
}: {
  isPending: boolean;
  isError: boolean;
  errStatus?: number;
  pinned: (PlaceWithItem & { lat: number; lng: number })[];
  unpinned: PlaceWithItem[];
  initialRegion: Region;
  onRetry: () => void;
  onOpenItem: (id: string) => void;
  onBack: () => void;
}) {
  if (isPending) {
    return (
      <View style={styles.flex}>
        <TopBar onBack={onBack} />
        <View style={styles.centre}>
          <ActivityIndicator color={colors.cobalt} />
        </View>
      </View>
    );
  }

  if (isError && pinned.length === 0 && unpinned.length === 0) {
    const text =
      errStatus === 0
        ? "Instance unreachable — check your connection or the URL in Settings."
        : errStatus === 401
          ? "Token rejected — check Settings."
          : "Couldn't load your places.";
    return (
      <View style={styles.flex}>
        <TopBar onBack={onBack} />
        <Message text={text} onRetry={onRetry} />
      </View>
    );
  }

  if (pinned.length === 0 && unpinned.length === 0) {
    return (
      <View style={styles.flex}>
        <TopBar onBack={onBack} />
        <Message text="No places saved yet — capture something with a location." />
      </View>
    );
  }

  return (
    <MapBody
      pinned={pinned}
      unpinned={unpinned}
      initialRegion={initialRegion}
      onOpenItem={onOpenItem}
      onBack={onBack}
    />
  );
}

function TopBar({ onBack }: { onBack: () => void }) {
  const insets = useSafeAreaInsets();
  return (
    <View style={[styles.topbar, { paddingTop: insets.top + spacing.sm }]}>
      <Pressable onPress={onBack} hitSlop={12} accessibilityRole="button" accessibilityLabel="Back">
        <Text style={styles.back}>‹ Back</Text>
      </Pressable>
    </View>
  );
}

function MapBody({
  pinned,
  unpinned,
  initialRegion,
  onOpenItem,
  onBack,
}: {
  pinned: (PlaceWithItem & { lat: number; lng: number })[];
  unpinned: PlaceWithItem[];
  initialRegion: Region;
  onOpenItem: (id: string) => void;
  onBack: () => void;
}) {
  const mapRef = useRef<MapView>(null);
  const insets = useSafeAreaInsets();
  const [locating, setLocating] = useState(false);
  const [region, setRegion] = useState<Region>(initialRegion);
  const index = useMemo(
    () =>
      buildIndex(
        pinned.map((p) => ({
          id: p.id,
          name: p.name,
          itemId: p.itemId,
          itemTitle: p.itemTitle,
          lat: p.lat,
          lng: p.lng,
        })),
      ),
    [pinned],
  );
  const features = useMemo(() => clustersForRegion(index, region), [index, region]);

  async function goToMyLocation() {
    setLocating(true);
    try {
      const { status } = await Location.requestForegroundPermissionsAsync();
      if (status !== "granted") {
        Alert.alert("Location needed", "Allow location access to centre the map on you.");
        return;
      }
      const pos = await Location.getCurrentPositionAsync({
        accuracy: Location.Accuracy.Balanced,
      });
      mapRef.current?.animateToRegion({
        latitude: pos.coords.latitude,
        longitude: pos.coords.longitude,
        latitudeDelta: NEAR_ME_DELTA,
        longitudeDelta: NEAR_ME_DELTA,
      });
    } catch {
      Alert.alert("Couldn't find you", "Try again in a moment.");
    } finally {
      setLocating(false);
    }
  }

  return (
    <View style={styles.flex}>
      <View style={styles.mapWrap}>
        <MapView
          ref={mapRef}
          style={styles.map}
          initialRegion={initialRegion}
          onRegionChangeComplete={setRegion}
          showsUserLocation
          showsMyLocationButton={false}
        >
          {features.map((f) =>
            f.kind === "cluster" ? (
              <Marker
                key={f.id}
                coordinate={{ latitude: f.latitude, longitude: f.longitude }}
                onPress={() => {
                  try {
                    mapRef.current?.animateToRegion(
                      expansionRegion(index, f.clusterId, f.longitude, f.latitude),
                    );
                  } catch {
                    mapRef.current?.animateToRegion({
                      latitude: f.latitude,
                      longitude: f.longitude,
                      latitudeDelta: region.latitudeDelta / 4,
                      longitudeDelta: region.longitudeDelta / 4,
                    });
                  }
                }}
                accessibilityRole="button"
                accessibilityLabel={`${f.count} places, tap to expand`}
              >
                <View style={styles.cluster}>
                  <Text style={styles.clusterText}>{f.count}</Text>
                </View>
              </Marker>
            ) : (
              <Marker
                key={f.id}
                coordinate={{ latitude: f.latitude, longitude: f.longitude }}
                title={f.name}
                description={f.itemTitle}
                onCalloutPress={() => onOpenItem(f.itemId)}
              />
            ),
          )}
        </MapView>
        <Pressable
          style={[styles.backFab, { top: insets.top + spacing.sm }]}
          onPress={onBack}
          hitSlop={8}
          accessibilityRole="button"
          accessibilityLabel="Back"
        >
          <Text style={styles.backFabText}>‹ Back</Text>
        </Pressable>
        <Pressable
          style={[styles.locateFab, locating && styles.locateFabBusy]}
          onPress={() => void goToMyLocation()}
          disabled={locating}
          hitSlop={8}
          accessibilityRole="button"
          accessibilityLabel="Current location"
        >
          <Text style={styles.locateFabText}>{locating ? "…" : "⌖"}</Text>
        </Pressable>
      </View>
      {unpinned.length > 0 ? (
        <ScrollView style={styles.unpinnedList} contentContainerStyle={styles.unpinnedContent}>
          <Text style={styles.sectionHeader}>Without coordinates · {unpinned.length}</Text>
          {unpinned.map((p) => (
            <Pressable
              key={p.id}
              style={({ pressed }) => [styles.row, pressed && styles.rowPressed]}
              onPress={() => void Linking.openURL(googleMapsSearchUrl(p))}
            >
              <Text style={styles.rowTitle} numberOfLines={1}>
                {p.name}
              </Text>
              {p.address ? (
                <Text style={styles.rowSubtitle} numberOfLines={1}>
                  {p.address}
                </Text>
              ) : null}
            </Pressable>
          ))}
        </ScrollView>
      ) : null}
    </View>
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
  flex: { flex: 1 },
  mapWrap: { flex: 1 },
  map: { position: "absolute", top: 0, right: 0, bottom: 0, left: 0 },
  topbar: {
    paddingHorizontal: spacing.xl,
    paddingBottom: spacing.sm,
    backgroundColor: colors.paper,
  },
  back: { color: colors.cobalt, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  backFab: {
    position: "absolute",
    left: spacing.lg,
    backgroundColor: colors.cardSurface,
    borderRadius: radius.button,
    borderWidth: 1,
    borderColor: colors.hairline,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
  },
  backFabText: { color: colors.cobalt, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  locateFab: {
    position: "absolute",
    right: spacing.lg,
    bottom: spacing.lg,
    width: 44,
    height: 44,
    borderRadius: radius.button,
    backgroundColor: colors.cardSurface,
    borderWidth: 1,
    borderColor: colors.hairline,
    alignItems: "center",
    justifyContent: "center",
  },
  locateFabBusy: { opacity: 0.6 },
  locateFabText: { color: colors.cobalt, fontFamily: fonts.sansSemiBold, fontSize: 20, lineHeight: 24 },
  cluster: {
    minWidth: 34,
    height: 34,
    borderRadius: 17,
    paddingHorizontal: spacing.sm,
    backgroundColor: colors.cobalt,
    borderWidth: 2,
    borderColor: colors.paper,
    alignItems: "center",
    justifyContent: "center",
  },
  clusterText: { color: colors.paper, fontFamily: fonts.sansSemiBold, fontSize: 13 },
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
  unpinnedList: { maxHeight: "35%", backgroundColor: colors.paper },
  unpinnedContent: { paddingHorizontal: spacing.xl, paddingVertical: spacing.md },
  sectionHeader: {
    fontFamily: fonts.monoMedium,
    fontSize: 11,
    letterSpacing: 0.6,
    textTransform: "uppercase",
    color: colors.inkFaint,
    marginBottom: spacing.sm,
  },
  row: {
    borderRadius: radius.card,
    borderWidth: 1,
    borderColor: colors.hairline,
    backgroundColor: colors.cardSurface,
    padding: spacing.md,
    marginBottom: spacing.sm,
  },
  rowPressed: { opacity: 0.7 },
  rowTitle: { fontFamily: fonts.sansSemiBold, fontSize: 14, color: colors.ink },
  rowSubtitle: { fontFamily: fonts.sans, fontSize: 12, color: colors.inkMuted, marginTop: 2 },
});
