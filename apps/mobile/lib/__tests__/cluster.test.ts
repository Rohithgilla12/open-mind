import type { Region } from "react-native-maps";
import { buildIndex, clustersForRegion, expansionRegion, zoomForRegion, type ClusterInput } from "../cluster";

const mk = (id: string, lat: number, lng: number): ClusterInput => ({
  id, name: `n-${id}`, itemId: `i-${id}`, itemTitle: `t-${id}`, lat, lng,
});
// Three points ~0.1° apart near NY, plus London far away.
const A = mk("a", 40.0, -74.0);
const B = mk("b", 40.1, -74.1);
const C = mk("c", 40.05, -74.05);
const L = mk("l", 51.5, -0.1);

const WORLD: Region = { latitude: 45, longitude: -37, latitudeDelta: 40, longitudeDelta: 140 };
const NY: Region = { latitude: 40.05, longitude: -74.05, latitudeDelta: 0.3, longitudeDelta: 0.3 };

test("zoomForRegion maps wide→low and narrow→high zoom", () => {
  expect(zoomForRegion(WORLD)).toBeLessThan(4);
  expect(zoomForRegion(NY)).toBeGreaterThan(8);
});

test("zoomForRegion clamps at the boundaries and guards NaN", () => {
  expect(zoomForRegion({ ...WORLD, longitudeDelta: 400 })).toBe(0);
  expect(zoomForRegion({ ...WORLD, longitudeDelta: 0.0000001 })).toBe(20);
  expect(zoomForRegion({ ...WORLD, longitudeDelta: NaN })).toBe(0);
});

test("clustersForRegion on an empty index returns no features", () => {
  expect(clustersForRegion(buildIndex([]), WORLD)).toEqual([]);
});

test("zoomed out: nearby NY points collapse into one cluster; London stays a point", () => {
  const feats = clustersForRegion(buildIndex([A, B, C, L]), WORLD);
  const clusters = feats.filter((f) => f.kind === "cluster");
  const points = feats.filter((f) => f.kind === "point");
  expect(clusters).toHaveLength(1);
  expect(clusters[0].kind === "cluster" && clusters[0].count).toBe(3);
  expect(points).toHaveLength(1);
  expect(points[0].kind === "point" && points[0].id).toBe("l");
});

test("zoomed in over NY: the three points separate into individual points", () => {
  const feats = clustersForRegion(buildIndex([A, B, C, L]), NY);
  expect(feats.every((f) => f.kind === "point")).toBe(true);
  expect(feats).toHaveLength(3); // London is outside the NY bbox
  expect(feats).toContainEqual(
    expect.objectContaining({ name: "n-a", itemId: "i-a", itemTitle: "t-a" }),
  );
});

test("a lone point is never a cluster", () => {
  const feats = clustersForRegion(buildIndex([A]), WORLD);
  expect(feats).toEqual([
    expect.objectContaining({ kind: "point", id: "a" }),
  ]);
});

test("expansionRegion zooms tighter than the region the cluster came from", () => {
  const index = buildIndex([A, B, C, L]);
  const feats = clustersForRegion(index, WORLD);
  const cluster = feats.find((f): f is Extract<typeof feats[number], { kind: "cluster" }> => f.kind === "cluster")!;
  const region = expansionRegion(index, cluster.clusterId, cluster.longitude, cluster.latitude);
  expect(region.longitudeDelta).toBeLessThan(WORLD.longitudeDelta);
  expect(region.longitude).toBe(cluster.longitude);
  expect(region.latitude).toBe(cluster.latitude);
  expect(region.longitudeDelta).toBe(region.latitudeDelta);
});
