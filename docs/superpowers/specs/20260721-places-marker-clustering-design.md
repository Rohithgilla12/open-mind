# Places marker clustering — web (MapLibre) + mobile (supercluster)

Date: 2026-07-21. The Places map plots one marker per geocoded place on both
web (`apps/web/components/PlacesMap.tsx`, MapLibre) and mobile
(`apps/mobile/app/places.tsx`, react-native-maps). In dense areas the pins pile
on top of each other and become unreadable. This adds marker clustering to both,
using the same algorithm (supercluster) on each platform so they behave
identically.

## Goal

Nearby pins collapse into a count bubble when zoomed out; tapping a cluster
zooms in to expand it; individual pins keep their existing behaviour (web popup
with an item link, mobile callout that opens the item). Clean and functional for
v1 — no spiderfy, no bespoke animations.

## Non-goals

- No spiderfy / expansion-in-place — tap always zoom-to-expand.
- No animated count transitions or bounce effects (v1 stays clean).
- No API/contract change — both maps consume the existing `/places` response.
- No change to the unpinned-places list, FABs, or geolocate controls.

## Current state (verified)

- **Web** `PlacesMap.tsx`: builds one `maplibregl.Marker` (cobalt) per pinned
  place with a `Popup` (name / address / item link); `fitBounds` over all pins;
  Navigation + Geolocate controls. `maplibre-gl@^5.24.0`. The map's `MapPlace`
  type is hand-written locally.
- **Mobile** `places.tsx`: one `<Marker>` per pinned place (title/description,
  `onCalloutPress` → open item); unpinned places listed below; Back + locate
  FABs. `react-native-maps@1.27.2`. No clustering library present.
- A jest-expo unit-test harness exists (added 2026-07-21) for mobile pure-logic
  tests.

## Web — native MapLibre clustering

MapLibre clusters natively via a GeoJSON source (supercluster under the hood),
so no dependency is added. Rework `PlacesMap.tsx`:

- Build a GeoJSON `FeatureCollection` from pinned places; each feature's
  `properties` carry `{ name, address, itemId, itemTitle }`.
- Add it as a source with `cluster: true`, `clusterRadius: 50`,
  `clusterMaxZoom: 14`.
- Three layers:
  - **clusters** — `circle`, filtered to `has point_count`; cobalt fill
    (`tokens.color.cobalt`), radius stepped by `point_count`
    (e.g. `["step", ["get","point_count"], 16, 10, 22, 50, 30]`).
  - **cluster-count** — `symbol`, `text-field: {point_count_abbreviated}`, paper
    text (`#F4F0E6`).
  - **unclustered-point** — `circle`, filtered to `!has point_count`; cobalt dot.
- Interactions:
  - click **cluster** → `source.getClusterExpansionZoom(clusterId)` →
    `map.easeTo({ center, zoom })`.
  - click **unclustered-point** → the existing popup (name / address / item
    link), built from the feature's `properties`; `escapeHtml` retained.
  - `mouseenter`/`mouseleave` on both layers toggles the pointer cursor.
- `fitBounds` on load, Navigation + Geolocate controls unchanged.

Also fold in the logged cleanup: derive the map's place type from the generated
`@openmind/api-client` (`/places` response) instead of the hand-written local
`MapPlace` (keep a thin local alias if the map needs a narrower shape).

## Mobile — supercluster

Add the small, well-maintained `supercluster` dependency (+ `@types/supercluster`
if the package ships no types). Put the clustering math in a **pure, testable
helper**, keeping `places.tsx` a thin renderer.

- New `apps/mobile/lib/cluster.ts`:
  - `buildIndex(points)` → a `Supercluster` index loaded with pinned places as
    GeoJSON point features carrying `{ id, name, itemId, itemTitle }`.
  - `clustersForRegion(index, region)` → derives zoom from the region
    (`zoom = round(log2(360 / longitudeDelta))`), computes the bbox from
    centre ± deltas/2, returns `index.getClusters(bbox, zoom)`.
  - `expansionRegion(index, clusterId, longitude, latitude)` → uses
    `getClusterExpansionZoom` to produce the target `Region` for a tap.
- `places.tsx`:
  - build the index with `useMemo` over `pinned`;
  - hold the current `Region` in state, seeded from `initialRegion`, updated in
    `onRegionChangeComplete`; derive `clusters` with `useMemo`.
  - render each returned feature: a **cluster** (`properties.cluster === true`) →
    a custom `<Marker>` whose child view is a warm-palette pill showing
    `point_count`; `onPress` → `animateToRegion(expansionRegion(...))`. A
    **point** → the existing place `<Marker>` (callout → open item).
  - unpinned list, Back/locate FABs, permission flow unchanged.

## Shared styling

Clusters read as one system across platforms: cobalt fill, paper count text,
size scaling with count. No hardcoded colours — web uses `@openmind/ui`
`tokens`, mobile uses `lib/theme` (`colors.cobalt`, `colors.paper`, etc.).

## Testing

- Mobile: unit tests for `lib/cluster.ts` under jest-expo — points collapse into
  a cluster when zoomed out, separate into individual points when zoomed in, a
  lone point stays a point, and `expansionRegion` returns a tighter (higher-zoom)
  region than its cluster. A small fixed fixture of coordinates; no map needed.
- Web MapLibre clustering is engine-internal (no bespoke logic to unit-test);
  verified by running the web app.
- Manual: web dev server and an iOS build to eyeball clustering + tap-to-expand
  on both platforms.

## Rollout

Client-only; no migration, env var, or contract change. Web ships in the normal
build; the mobile `supercluster` dependency ships in the next dev/EAS build.
