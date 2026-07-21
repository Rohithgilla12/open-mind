"use client";
// Full-bleed MapLibre map of every geocoded place. Client-side only — OSM
// raster tiles, no server component. Pins open a popup linking to the item.
import { useEffect, useRef } from "react";
import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { tokens } from "@openmind/ui";
import type { MapPlace } from "../lib/types";

export type { MapPlace };

// Minimal geometry shape we read off clustered features. Kept inline rather
// than importing GeoJSON types so it doesn't depend on @types/geojson being
// resolvable in the web workspace (it isn't in CI).
type PointGeometry = { coordinates: [number, number] };

const OSM_STYLE = {
  version: 8 as const,
  sources: {
    osm: {
      type: "raster" as const,
      tiles: ["https://tile.openstreetmap.org/{z}/{x}/{y}.png"],
      tileSize: 256,
      attribution: "© OpenStreetMap contributors",
    },
  },
  layers: [{ id: "osm", type: "raster" as const, source: "osm" }],
};

export function PlacesMap({ places }: { places: MapPlace[] }) {
  const container = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!container.current) return;
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
        const [lng, lat] = (f.geometry as unknown as PointGeometry).coordinates;
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
      // cluster_id is stable per index/zoom-bucket, so this cache is bounded by
      // the cluster-tree size — no pruning needed; it's torn down with the map.
      const clusterMarkers = new Map<number, maplibregl.Marker>();
      let onScreen = new Map<number, maplibregl.Marker>();
      const syncClusters = () => {
        if (!map.isSourceLoaded("places")) return;
        const next = new Map<number, maplibregl.Marker>();
        for (const f of map.querySourceFeatures("places")) {
          const props = f.properties as { cluster?: boolean; cluster_id?: number; point_count?: number };
          if (!props.cluster || props.cluster_id == null) continue;
          const id = props.cluster_id;
          const [lng, lat] = (f.geometry as unknown as PointGeometry).coordinates;
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
            el.setAttribute("role", "button");
            el.setAttribute("tabindex", "0");
            el.setAttribute("aria-label", `${props.point_count ?? 0} places, activate to expand`);
            el.addEventListener("click", () => {
              const src = map.getSource("places") as maplibregl.GeoJSONSource | undefined;
              if (!src) return;
              src
                .getClusterExpansionZoom(id)
                .then((zoom) => {
                  if (map.getSource("places")) map.easeTo({ center: [lng, lat], zoom });
                })
                .catch((err) => console.warn("cluster expansion zoom lookup failed", err));
            });
            el.addEventListener("keydown", (ev) => {
              if (ev.key === "Enter" || ev.key === " ") {
                ev.preventDefault();
                el.click();
              }
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
  }, [places]);

  return <div ref={container} style={{ position: "absolute", inset: 0 }} />;
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
}
