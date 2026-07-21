# Places Marker Clustering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cluster nearby pins on the Places map — web (MapLibre) and mobile (react-native-maps + supercluster) — so dense areas stay readable; tap a cluster to zoom in and expand.

**Architecture:** Mobile uses the `supercluster` library behind a pure, testable helper (`lib/cluster.ts`), with `places.tsx` a thin renderer. Web uses MapLibre's native GeoJSON clustering (supercluster under the hood — same algorithm, no npm dep) with a circle layer for single points and custom HTML markers for cluster-count bubbles (avoiding a symbol text layer, which would need an external glyphs/font server). Clean, functional v1: tap-to-zoom only, no spiderfy, no bespoke animation.

**Tech Stack:** TypeScript, MapLibre GL JS 5.24.0 (web), react-native-maps 1.27.2 + supercluster (mobile), jest-expo (mobile unit tests), `@openmind/ui` tokens (web) / `lib/theme` (mobile).

## Global Constraints

- Clean/functional v1: **tap → zoom-to-expand only**. No spiderfy, no expansion-in-place, no animated count transitions or bounce.
- **No new external runtime dependency on web.** MapLibre clustering is native (GeoJSON `cluster: true`). Do NOT add a `symbol` text layer for counts — it requires an external glyphs/font server; use HTML markers instead.
- Mobile adds exactly one runtime dep: `supercluster` (+ `@types/supercluster` dev dep if the package ships no types). Package manager for `apps/mobile` is **npm**; `npx` is shimmed → use `./node_modules/.bin/...`, never `npx`.
- Both platforms cluster with the same algorithm (supercluster; radius 50).
- No API/contract change; both maps consume the existing `/places` response. No migration.
- Do NOT hardcode colours: web uses `@openmind/ui` `tokens.color` (`cobalt` `#1B3FD1`, `paper` `#F4F0E6`, `ink` `#1C1A16`); mobile uses `lib/theme` `colors` (`cobalt`, `paper`, `cardSurface`, `hairline`, `ink`, `inkMuted`).
- No comment banner blocks (`// ======`); match the terse style of the file being edited.
- `maplibre-gl@5.24.0`: `source.getClusterExpansionZoom(clusterId)` returns `Promise<number>` — `await` it.
- Preserve existing behaviour: web popup (name/address/item link) + Navigation/Geolocate controls + `fitBounds`; mobile callout → open item, unpinned list, Back/locate FABs, geolocate flow.

---

### Task 1: Mobile clustering helper `lib/cluster.ts` (pure, tested)

Isolate all supercluster math behind a pure module so `places.tsx` stays a thin renderer and the logic is unit-testable under jest-expo.

**Files:**
- Modify: `apps/mobile/package.json` (add `supercluster`)
- Create: `apps/mobile/lib/cluster.ts`
- Create: `apps/mobile/lib/__tests__/cluster.test.ts`

**Interfaces:**
- Consumes: `supercluster`; `react-native-maps` `Region` type.
- Produces (used by Task 2):
  - `type ClusterInput = { id: string; name: string; itemId: string; itemTitle: string; lat: number; lng: number }`
  - `type ClusterFeature = { kind: "cluster"; id: string; longitude: number; latitude: number; count: number; clusterId: number } | { kind: "point"; id: string; longitude: number; latitude: number; name: string; itemId: string; itemTitle: string }`
  - `buildIndex(places: ClusterInput[]): Supercluster`
  - `clustersForRegion(index: Supercluster, region: Region): ClusterFeature[]`
  - `expansionRegion(index: Supercluster, clusterId: number, longitude: number, latitude: number): Region`
  - `zoomForRegion(region: Region): number`

- [ ] **Step 1: Install supercluster**

Run (from `apps/mobile/`):
```bash
npm install supercluster
npm install -D @types/supercluster
```
Expected: both resolve. (If `supercluster` already bundles its own types, the `@types` install is harmless.) If a peer conflict appears, re-run with `--legacy-peer-deps`.

- [ ] **Step 2: Write the failing test**

`apps/mobile/lib/__tests__/cluster.test.ts`:

```ts
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
});

test("a lone point is never a cluster", () => {
  const feats = clustersForRegion(buildIndex([A]), WORLD);
  expect(feats).toEqual([
    expect.objectContaining({ kind: "point", id: "a" }),
  ]);
});

test("expansionRegion zooms tighter than the region the cluster came from", () => {
  const index = buildIndex([A, B, C, L]);
  const cluster = clustersForRegion(index, WORLD).find((f) => f.kind === "cluster")!;
  const region = expansionRegion(index, cluster.clusterId!, cluster.longitude, cluster.latitude);
  expect(region.longitudeDelta).toBeLessThan(WORLD.longitudeDelta);
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `npm test -- cluster`
Expected: FAIL — cannot find module `../cluster`.

- [ ] **Step 4: Implement `lib/cluster.ts`**

```ts
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `npm test -- cluster`
Expected: PASS (5 tests). Then `npm test` (full) and `npm run typecheck`.
If typecheck reports missing supercluster types, confirm `@types/supercluster` is installed (Step 1).

- [ ] **Step 6: Commit**

```bash
git add apps/mobile/package.json apps/mobile/package-lock.json apps/mobile/lib/cluster.ts apps/mobile/lib/__tests__/cluster.test.ts
git commit -m "feat(mobile): supercluster clustering helper for the Places map"
```

---

### Task 2: Mobile — render clusters in `places.tsx`

Wire the helper into the map: cluster pills that zoom-to-expand on tap; single points keep the existing marker + callout.

**Files:**
- Modify: `apps/mobile/app/places.tsx`

**Interfaces:**
- Consumes: `buildIndex`, `clustersForRegion`, `expansionRegion`, `ClusterInput` from `../lib/cluster` (Task 1).

- [ ] **Step 1: Import the helper and MapView Region tracking**

At the top of `apps/mobile/app/places.tsx`, add to the imports:
```ts
import { buildIndex, clustersForRegion, expansionRegion } from "@/lib/cluster";
```

- [ ] **Step 2: Replace the marker rendering in `MapBody`**

In `MapBody`, build the index and track the region, then render clustered features. Replace the `<MapView ...> {pinned.map(...)} </MapView>` block with:

```tsx
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
                onPress={() =>
                  mapRef.current?.animateToRegion(
                    expansionRegion(index, f.clusterId, f.longitude, f.latitude),
                  )
                }
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
```

(Keep the two `<Pressable>` FABs and the closing tags that follow exactly as they were.)

- [ ] **Step 3: Add the cluster pill styles**

In the `StyleSheet.create({...})` block, add:
```ts
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
```

- [ ] **Step 4: Confirm `useMemo`/`useState`/`Region` are imported**

`useMemo`, `useState`, `useRef` are already imported from `react` (verify); `Region` is already imported from `react-native-maps`. Add any that are missing.

- [ ] **Step 5: Typecheck + full test suite**

Run: `npm run typecheck` → no errors.
Run: `npm test` → all prior tests still pass (no new test for this renderer file).

- [ ] **Step 6: Manual verification (dev build or Expo Go)**

Open Places with several nearby pins: they show as a cobalt count pill; tapping it animates in and expands; zooming in fully shows individual pins whose callout opens the item. Unpinned list + FABs unchanged.

- [ ] **Step 7: Commit**

```bash
git add "apps/mobile/app/places.tsx"
git commit -m "feat(mobile): cluster Places pins, tap to zoom-and-expand"
```

---

### Task 3: Web — native MapLibre clustering in `PlacesMap.tsx`

Replace per-place DOM markers with a native clustered GeoJSON source: a circle layer for single points (popup on click) and custom HTML markers for cluster-count bubbles (tap → zoom to expand).

**Files:**
- Modify: `apps/web/components/PlacesMap.tsx`

**Interfaces:**
- Consumes: `maplibre-gl` (`Map`, `Marker`, `Popup`, `NavigationControl`, `GeolocateControl`, `LngLatBounds`, `GeoJSONSource`); `@openmind/ui` `tokens`.

- [ ] **Step 1: Replace the marker/popup body of the effect**

In `apps/web/components/PlacesMap.tsx`, keep the imports, `MapPlace` type, `OSM_STYLE`, `escapeHtml`, and the component/`container` shell. Replace the body of the `useEffect` (from building `pinned` through the `return () => map.remove()`) with:

```tsx
    const pinned = places.filter(
      (p): p is MapPlace & { lat: number; lng: number } => p.lat != null && p.lng != null,
    );
    const map = new maplibregl.Map({
      container: container.current,
      style: OSM_STYLE,
      center: pinned.length ? [pinned[0].lng, pinned[0].lat] : [0, 20],
      zoom: pinned.length ? 11 : 1.5,
    });
    map.addControl(new maplibregl.NavigationControl(), "top-right");
    map.addControl(
      new maplibregl.GeolocateControl({
        positionOptions: { enableHighAccuracy: true },
        trackUserLocation: false,
        showAccuracyCircle: false,
      }),
      "top-right",
    );

    map.on("load", () => {
      map.addSource("places", {
        type: "geojson",
        cluster: true,
        clusterRadius: 50,
        clusterMaxZoom: 14,
        data: {
          type: "FeatureCollection",
          features: pinned.map((p) => ({
            type: "Feature" as const,
            geometry: { type: "Point" as const, coordinates: [p.lng, p.lat] },
            properties: { name: p.name, address: p.address, itemId: p.itemId, itemTitle: p.itemTitle },
          })),
        },
      });

      // Single (unclustered) points: a cobalt dot with the existing popup.
      map.addLayer({
        id: "point",
        type: "circle",
        source: "places",
        filter: ["!", ["has", "point_count"]],
        paint: {
          "circle-color": tokens.color.cobalt,
          "circle-radius": 7,
          "circle-stroke-width": 2,
          "circle-stroke-color": tokens.color.paper,
        },
      });

      map.on("click", "point", (e) => {
        const f = e.features?.[0];
        if (!f) return;
        const p = f.properties as { name: string; address: string; itemId: string; itemTitle: string };
        const [lng, lat] = (f.geometry as GeoJSON.Point).coordinates;
        new maplibregl.Popup({ offset: 18 })
          .setLngLat([lng, lat])
          .setHTML(
            `<strong>${escapeHtml(p.name)}</strong><br/>${escapeHtml(p.address)}<br/><a href="/item/${encodeURIComponent(p.itemId)}">${escapeHtml(p.itemTitle || "View item")}</a>`,
          )
          .addTo(map);
      });
      map.on("mouseenter", "point", () => (map.getCanvas().style.cursor = "pointer"));
      map.on("mouseleave", "point", () => (map.getCanvas().style.cursor = ""));

      // Cluster count bubbles: HTML markers synced to the source on each render
      // (a symbol text layer would need an external glyphs/font server).
      const clusterMarkers = new Map<number, maplibregl.Marker>();
      let onScreen = new Map<number, maplibregl.Marker>();
      const syncClusters = () => {
        if (!map.isSourceLoaded("places")) return;
        const next = new Map<number, maplibregl.Marker>();
        for (const f of map.querySourceFeatures("places")) {
          const props = f.properties as { cluster?: boolean; cluster_id?: number; point_count?: number };
          if (!props.cluster || props.cluster_id == null) continue;
          const id = props.cluster_id;
          const [lng, lat] = (f.geometry as GeoJSON.Point).coordinates;
          let marker = clusterMarkers.get(id);
          if (!marker) {
            const el = document.createElement("div");
            el.textContent = String(props.point_count ?? "");
            Object.assign(el.style, {
              minWidth: "34px", height: "34px", padding: "0 8px", borderRadius: "17px",
              background: tokens.color.cobalt, color: tokens.color.paper,
              border: `2px solid ${tokens.color.paper}`, display: "flex",
              alignItems: "center", justifyContent: "center", cursor: "pointer",
              font: "600 13px system-ui, sans-serif",
            } as Partial<CSSStyleDeclaration>);
            el.addEventListener("click", () => {
              const src = map.getSource("places") as maplibregl.GeoJSONSource;
              void src.getClusterExpansionZoom(id).then((zoom) => map.easeTo({ center: [lng, lat], zoom }));
            });
            marker = new maplibregl.Marker({ element: el }).setLngLat([lng, lat]);
            clusterMarkers.set(id, marker);
          } else {
            marker.getElement().textContent = String(props.point_count ?? "");
          }
          next.set(id, marker);
          if (!onScreen.has(id)) marker.addTo(map);
        }
        for (const [id, marker] of onScreen) if (!next.has(id)) marker.remove();
        onScreen = next;
      };
      map.on("render", syncClusters);

      const bounds = new maplibregl.LngLatBounds();
      for (const p of pinned) bounds.extend([p.lng, p.lat]);
      if (pinned.length > 1) map.fitBounds(bounds, { padding: 64, maxZoom: 13 });
    });

    return () => map.remove();
```

- [ ] **Step 2: Typecheck**

Run (from `apps/web/`): `pnpm turbo run typecheck --filter=web` (or the repo's web typecheck). Expected: no errors. If `GeoJSON.Point` is unresolved, it is provided by `@types/geojson` (a transitive dep of maplibre-gl); import type via `import type { Point } from "geojson"` and use `Point` instead.

- [ ] **Step 3: Lint + build**

Run: `pnpm turbo run lint build --filter=web`. Expected: pass (mirrors the "Web (lint + build)" CI check).

- [ ] **Step 4: Manual verification**

Run the web dev server (ask the user first — they usually have one running). Open `/places` with several nearby geocoded items: nearby pins show as a cobalt count bubble; clicking it eases in and expands; clicking a single dot shows the popup with a working item link; nav + geolocate controls present; `fitBounds` frames all pins on load.

- [ ] **Step 5: Commit**

```bash
git add apps/web/components/PlacesMap.tsx
git commit -m "feat(web): cluster Places pins with native MapLibre clustering"
```

---

### Task 4: Web — derive the place type from the generated api-client

The logged cleanup: `PlacesMap.tsx` hand-writes `MapPlace`, and `places/page.tsx` casts `/places` JSON to a local `PlaceWithItem`. Derive both from the generated `@openmind/api-client` schema so the map type tracks the contract.

**Files:**
- Modify: `apps/web/components/PlacesMap.tsx`
- Modify: `apps/web/app/places/page.tsx`

**Interfaces:**
- Consumes: `paths` from `@openmind/api-client`.

- [ ] **Step 1: Derive the response element type in `PlacesMap.tsx`**

Confirm the export first:
Run: `grep -n "listPlaces\|/places" packages/api-client/src/schema.d.ts | head`
Expected: `operations["listPlaces"]` and `paths["/places"]` exist.

Replace the hand-written `MapPlace` type in `apps/web/components/PlacesMap.tsx` with a derivation from the contract:
```ts
import type { paths } from "@openmind/api-client";

// One element of GET /places, straight from the OpenAPI contract.
export type MapPlace =
  paths["/places"]["get"]["responses"][200]["content"]["application/json"][number];
```
(If the generated array type resolves to `never[]` or the `200` key differs, fall back to `keyof`-inspecting the generated `operations["listPlaces"]["responses"]` and use the correct status key; report the exact shape found. The map only reads `id`, `name`, `address`, `lat`, `lng`, `itemId`, `itemTitle` — all present on the `/places` schema.)

- [ ] **Step 2: Use the derived type in `places/page.tsx`**

In `apps/web/app/places/page.tsx`, drop the local `type PlaceWithItem = MapPlace & {...}` and type `getPlaces` against the derived `MapPlace`:
```ts
async function getPlaces(): Promise<MapPlace[]> {
  try {
    const res = await apiFetch("/places");
    if (!res.ok) return [];
    return ((await res.json()) as MapPlace[]) ?? [];
  } catch {
    return [];
  }
}
```
Keep the `import { PlacesMap, type MapPlace } from "../../components/PlacesMap"` line. The `pinned`/`unpinned` filters and `<PlacesMap places={pinned} />` are unchanged (the schema type still carries `lat`/`lng`).

- [ ] **Step 3: Typecheck + lint + build**

Run: `pnpm turbo run typecheck lint build --filter=web`. Expected: pass. If the generated type is structurally identical to the old hand-written one, there is no behavioural change — this is purely a source-of-truth cleanup.

- [ ] **Step 4: Commit**

```bash
git add apps/web/components/PlacesMap.tsx apps/web/app/places/page.tsx
git commit -m "refactor(web): derive Places map type from the api-client contract"
```

---

## Self-Review

**Spec coverage:**
- Web native MapLibre clustering (source + point circle layer + cluster HTML markers + click-to-expand + popup + controls + fitBounds) → Task 3. Deviation from the spec's "symbol count layer": HTML markers instead, to honour the spec's "no dependency" goal (a symbol text layer needs an external glyphs/font server) — documented in Global Constraints and Task 3 Step 1.
- Mobile supercluster behind a pure helper + tests → Task 1; renderer wiring + cluster pills + tap-to-expand → Task 2.
- Shared warm-palette styling (tokens/theme, no hardcoded colours) → Tasks 2–3.
- Type fold-in (derive from api-client) → Task 4.
- Testing: mobile `cluster.ts` unit tests (collapse/expand/singleton/expansion) → Task 1; web verified by lint+build+manual → Task 3.

No gaps.

**Placeholder scan:** No TBD/TODO; every code step shows complete code; no "similar to Task N".

**Type consistency:** `ClusterInput`/`ClusterFeature`/`buildIndex`/`clustersForRegion`/`expansionRegion`/`zoomForRegion` are identical across Tasks 1–2. `MapPlace` stays the exported name in Tasks 3–4 (Task 4 changes only its definition, not its name, so Task 3's usage and `page.tsx`'s import are unaffected). Web layer ids (`point`, cluster markers keyed by `cluster_id`) and the source id (`places`) are consistent within Task 3.
