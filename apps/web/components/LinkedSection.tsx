"use client";

import { tokens } from "@openmind/ui";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type CSSProperties } from "react";
import { cardKind, domainOf, typeLabel } from "../lib/cards";
import type { Item } from "../lib/types";

const { color, font } = tokens;

const rowStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 8,
  padding: "7px 0",
};

const rowTextBtn: CSSProperties = {
  flex: 1,
  minWidth: 0,
  display: "flex",
  flexDirection: "column",
  gap: 2,
  background: "none",
  border: "none",
  padding: 0,
  textAlign: "left",
  cursor: "pointer",
};

const rowTitle: CSSProperties = {
  fontFamily: font.sans,
  fontSize: 13,
  color: color.ink,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const removeBtn: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: 16,
  height: 16,
  borderRadius: "50%",
  border: "none",
  background: "none",
  color: color.inkFaint,
  fontFamily: font.mono,
  fontSize: 13,
  lineHeight: 1,
  cursor: "pointer",
  padding: 0,
  flexShrink: 0,
};

const addToggle: CSSProperties = {
  font: `500 10px/1 ${font.mono}`,
  letterSpacing: ".02em",
  color: color.cobalt,
  background: "none",
  border: "none",
  padding: 0,
  cursor: "pointer",
};

/** Best label for a linked item: title, else the domain, else the raw URL. */
function labelFor(item: Item): string {
  return item.title || domainOf(item.url) || item.url;
}

export function LinkedSection({ itemId }: { itemId: string }) {
  const router = useRouter();
  const [links, setLinks] = useState<Item[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [candidates, setCandidates] = useState<Item[] | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    let cancelled = false;
    fetch(`/api/items/${itemId}/links`)
      .then((res) => (res.ok ? res.json() : Promise.reject(res.status)))
      .then((data: Item[]) => {
        if (!cancelled) setLinks(data);
      })
      .catch(() => {
        if (!cancelled) setLinks([]);
      });
    return () => {
      cancelled = true;
    };
  }, [itemId]);

  function openPicker() {
    setPickerOpen(true);
    setQuery("");
    if (candidates === null) {
      fetch("/api/items?limit=50")
        .then((res) => (res.ok ? res.json() : Promise.reject(res.status)))
        .then((data: Item[]) => setCandidates(data))
        .catch(() => setCandidates([]));
    }
    setTimeout(() => inputRef.current?.focus(), 0);
  }

  function closePicker() {
    setPickerOpen(false);
    setQuery("");
  }

  async function removeLink(toId: string) {
    if (!links) return;
    const prev = links;
    setLinks(links.filter((l) => l.id !== toId));
    setError(null);
    try {
      const res = await fetch(`/api/items/${itemId}/links/${toId}`, { method: "DELETE" });
      if (res.status !== 204) throw new Error("delete failed");
    } catch {
      setLinks(prev);
      setError("Could not remove link. Please try again.");
    }
  }

  async function addLink(toId: string) {
    setError(null);
    try {
      const res = await fetch(`/api/items/${itemId}/links`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ toId }),
      });
      if (!res.ok) throw new Error("link failed");
      const data = (await res.json()) as Item[];
      setLinks(data);
      closePicker();
    } catch {
      setError("Could not add link. Please try again.");
    }
  }

  const linkedIds = new Set((links ?? []).map((l) => l.id));
  const needle = query.trim().toLowerCase();
  const filtered = (candidates ?? []).filter((c) => {
    if (c.id === itemId || linkedIds.has(c.id)) return false;
    if (!needle) return true;
    return c.title?.toLowerCase().includes(needle) || c.url.toLowerCase().includes(needle);
  });

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <div className="meta" style={{ color: color.inkFaintAlt }}>
          Links
        </div>
        <button type="button" onClick={pickerOpen ? closePicker : openPicker} style={addToggle}>
          {pickerOpen ? "cancel" : "+ link"}
        </button>
      </div>

      {links === null ? null : links.length === 0 && !pickerOpen ? (
        <p
          className="meta"
          style={{ color: color.inkFaint, textTransform: "none", letterSpacing: ".02em", margin: "9px 0 0" }}
        >
          No links yet — weave a thread.
        </p>
      ) : (
        <div style={{ marginTop: 6 }}>
          {(links ?? []).map((item) => (
            <div key={item.id} style={rowStyle}>
              <button type="button" style={rowTextBtn} onClick={() => router.push(`/item/${item.id}`)}>
                <span style={rowTitle}>{labelFor(item)}</span>
                <span className="meta">{typeLabel[cardKind(item.cardType)]}</span>
              </button>
              <button
                type="button"
                onClick={() => removeLink(item.id)}
                aria-label={`Remove link to ${labelFor(item)}`}
                style={removeBtn}
              >
                ×
              </button>
            </div>
          ))}
        </div>
      )}

      {pickerOpen ? (
        <div style={{ marginTop: 9 }}>
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="search your library…"
            aria-label="Search items to link"
            style={{
              width: "100%",
              boxSizing: "border-box",
              font: `500 11px/1 ${font.mono}`,
              letterSpacing: ".02em",
              color: color.ink,
              background: color.paper,
              border: `1px solid ${color.hairline}`,
              borderRadius: 8,
              padding: "7px 9px",
              outline: "none",
            }}
          />
          <div style={{ marginTop: 6, maxHeight: 220, overflowY: "auto" }}>
            {candidates === null ? (
              <p className="meta" style={{ color: color.inkFaint, margin: "8px 0 0" }}>
                Loading…
              </p>
            ) : filtered.length === 0 ? (
              <p
                className="meta"
                style={{ color: color.inkFaint, textTransform: "none", letterSpacing: ".02em", margin: "8px 0 0" }}
              >
                No matches.
              </p>
            ) : (
              filtered.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => addLink(item.id)}
                  style={{ ...rowTextBtn, width: "100%", padding: "6px 0" }}
                >
                  <span style={rowTitle}>{labelFor(item)}</span>
                  <span className="meta">{typeLabel[cardKind(item.cardType)]}</span>
                </button>
              ))
            )}
          </div>
        </div>
      ) : null}

      {error ? (
        <p
          aria-live="polite"
          style={{ fontFamily: font.mono, fontSize: "0.72rem", color: color.danger, margin: "8px 0 0" }}
        >
          {error}
        </p>
      ) : null}
    </div>
  );
}
