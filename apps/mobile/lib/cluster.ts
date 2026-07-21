// Pure clustering helpers over supercluster, used by the mobile Places screen
// (web clusters via MapLibre, not this file). Kept free of react-native-maps
// rendering so the math is unit-testable without a map.
import Supercluster from "supercluster";
import type { Region } from "react-native-maps";

export type ClusterInput = {
  id: string;
  name: string;
  itemId: string;
  itemTitle: string;
  lat: number;
  lng: number;
};

export type ClusterFeature =
  | { kind: "cluster"; id: string; longitude: number; latitude: number; count: number; clusterId: number }
  | { kind: "point"; id: string; longitude: number; latitude: number; name: string; itemId: string; itemTitle: string };

type PointProps = { id: string; name: string; itemId: string; itemTitle: string };

export function buildIndex(places: ClusterInput[]): Supercluster<PointProps, PointProps> {
  const index = new Supercluster<PointProps, PointProps>({ radius: 50, maxZoom: 14 });
  index.load(
    places.map((p) => ({
      type: "Feature" as const,
      properties: { id: p.id, name: p.name, itemId: p.itemId, itemTitle: p.itemTitle },
      geometry: { type: "Point" as const, coordinates: [p.lng, p.lat] },
    })),
  );
  return index;
}

// Approximates zoom from longitude span alone (aspect ratio ignored) — good
// enough for supercluster's zoom buckets, not a precise MapView zoom.
export function zoomForRegion(region: Region): number {
  const delta = region.longitudeDelta;
  if (!Number.isFinite(delta) || delta <= 0) return 0;
  const z = Math.round(Math.log2(360 / delta));
  return Math.min(Math.max(z, 0), 20);
}

export function clustersForRegion(
  index: Supercluster<PointProps, PointProps>,
  region: Region,
): ClusterFeature[] {
  const { latitude, longitude, latitudeDelta, longitudeDelta } = region;
  const bbox: [number, number, number, number] = [
    longitude - longitudeDelta / 2,
    latitude - latitudeDelta / 2,
    longitude + longitudeDelta / 2,
    latitude + latitudeDelta / 2,
  ];
  return index.getClusters(bbox, zoomForRegion(region)).map((f): ClusterFeature => {
    const [lng, lat] = f.geometry.coordinates;
    if ("cluster" in f.properties && f.properties.cluster) {
      const { cluster_id, point_count } = f.properties;
      return { kind: "cluster", id: `cluster-${cluster_id}`, longitude: lng, latitude: lat, count: point_count, clusterId: cluster_id };
    }
    const { id, name, itemId, itemTitle } = f.properties;
    return { kind: "point", id, longitude: lng, latitude: lat, name, itemId, itemTitle };
  });
}

export function expansionRegion(
  index: Supercluster<PointProps, PointProps>,
  clusterId: number,
  longitude: number,
  latitude: number,
): Region {
  const zoom = Math.min(index.getClusterExpansionZoom(clusterId), 20);
  const delta = 360 / Math.pow(2, zoom);
  return { longitude, latitude, longitudeDelta: delta, latitudeDelta: delta };
}
