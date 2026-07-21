// Pure clustering helpers over supercluster, shared by the Places map. Kept free
// of react-native-maps rendering so the math is unit-testable without a map.
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

export function buildIndex(places: ClusterInput[]): Supercluster<PointProps> {
  const index = new Supercluster<PointProps>({ radius: 50, maxZoom: 16 });
  index.load(
    places.map((p) => ({
      type: "Feature" as const,
      properties: { id: p.id, name: p.name, itemId: p.itemId, itemTitle: p.itemTitle },
      geometry: { type: "Point" as const, coordinates: [p.lng, p.lat] },
    })),
  );
  return index;
}

export function zoomForRegion(region: Region): number {
  const z = Math.round(Math.log2(360 / Math.max(region.longitudeDelta, 1e-6)));
  return Math.min(Math.max(z, 0), 20);
}

export function clustersForRegion(index: Supercluster<PointProps>, region: Region): ClusterFeature[] {
  const { latitude, longitude, latitudeDelta, longitudeDelta } = region;
  const bbox: [number, number, number, number] = [
    longitude - longitudeDelta / 2,
    latitude - latitudeDelta / 2,
    longitude + longitudeDelta / 2,
    latitude + latitudeDelta / 2,
  ];
  return index.getClusters(bbox, zoomForRegion(region)).map((f): ClusterFeature => {
    const [lng, lat] = f.geometry.coordinates;
    const props = f.properties as { cluster?: boolean; cluster_id?: number; point_count?: number } & Partial<PointProps>;
    if (props.cluster) {
      return { kind: "cluster", id: `cluster-${props.cluster_id}`, longitude: lng, latitude: lat, count: props.point_count ?? 0, clusterId: props.cluster_id! };
    }
    return { kind: "point", id: props.id!, longitude: lng, latitude: lat, name: props.name ?? "", itemId: props.itemId ?? "", itemTitle: props.itemTitle ?? "" };
  });
}

export function expansionRegion(
  index: Supercluster<PointProps>,
  clusterId: number,
  longitude: number,
  latitude: number,
): Region {
  const zoom = Math.min(index.getClusterExpansionZoom(clusterId), 20);
  const delta = 360 / Math.pow(2, zoom);
  return { longitude, latitude, longitudeDelta: delta, latitudeDelta: delta };
}
