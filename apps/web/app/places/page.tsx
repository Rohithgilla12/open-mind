import { tokens } from "@openmind/ui";
import { Shell } from "../../components/Shell";
import { PlacesMap } from "../../components/PlacesMap";
import { apiFetch } from "../../lib/api";
import type { MapPlace } from "../../lib/types";

const { color, font } = tokens;

async function getPlaces(): Promise<MapPlace[]> {
  try {
    const res = await apiFetch("/places");
    if (!res.ok) return [];
    return ((await res.json()) as MapPlace[]) ?? [];
  } catch {
    return [];
  }
}

export default async function PlacesPage() {
  const places = await getPlaces();
  const pinned = places.filter((p) => p.lat != null && p.lng != null);
  const unpinned = places.filter((p) => p.lat == null || p.lng == null);
  const subline = `${places.length.toLocaleString("en-GB")} places · ${pinned.length.toLocaleString("en-GB")} on the map`;

  return (
    <Shell activePlaces>
      <div
        style={{
          padding: "18px 28px 16px",
          borderBottom: `1px solid ${color.hairline}`,
          background: color.header,
        }}
      >
        <h1
          className="serif"
          style={{ fontSize: 27, fontWeight: 600, letterSpacing: "-.02em", color: color.ink, margin: 0 }}
        >
          Places
        </h1>
        <div
          className="meta"
          style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt, marginTop: 6 }}
        >
          {subline}
        </div>
      </div>

      {places.length === 0 ? (
        <div style={{ padding: "22px 28px 40px" }}>
          <p
            style={{
              fontFamily: font.quote,
              fontStyle: "italic",
              fontSize: "1.25rem",
              lineHeight: 1.5,
              color: color.inkMuted,
              maxWidth: "48ch",
              marginTop: "2rem",
            }}
          >
            Save something with a place in it and it&apos;ll show up here, pinned on a map.
          </p>
        </div>
      ) : (
        <>
          {pinned.length > 0 && (
            <div style={{ position: "relative", flex: 1 }}>
              <PlacesMap places={pinned} />
            </div>
          )}

          {unpinned.length > 0 && (
            <div style={{ padding: "16px 28px 32px", borderTop: pinned.length > 0 ? `1px solid ${color.hairline}` : undefined }}>
              {pinned.length === 0 && (
                <div className="meta" style={{ color: color.inkFaint, marginBottom: 16 }}>
                  None of your places have map coordinates yet.
                </div>
              )}
              {pinned.length > 0 && (
                <div className="meta" style={{ color: color.inkFaint, marginBottom: 10 }}>
                  Not on the map
                </div>
              )}
              <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 8 }}>
            {unpinned.map((p) => {
              const query = encodeURIComponent(`${p.name} ${p.hint}`.trim());
              return (
                <li
                  key={p.id}
                  style={{
                    fontFamily: font.sans,
                    fontSize: 13,
                    color: color.ink,
                    display: "flex",
                    alignItems: "baseline",
                    gap: 10,
                  }}
                >
                  <span style={{ fontWeight: 500 }}>{p.name}</span>
                  <span style={{ color: color.inkMuted }}>{p.hint}</span>
                  <a
                    href={`https://www.openstreetmap.org/search?query=${query}`}
                    target="_blank"
                    rel="noreferrer"
                    style={{ color: color.cobalt }}
                  >
                    OSM
                  </a>
                  <a
                    href={`https://www.google.com/maps/search/${query}`}
                    target="_blank"
                    rel="noreferrer"
                    style={{ color: color.cobalt }}
                  >
                    Google Maps
                  </a>
                </li>
              );
            })}
              </ul>
            </div>
          )}
        </>
      )}
    </Shell>
  );
}
