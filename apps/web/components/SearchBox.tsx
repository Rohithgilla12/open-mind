"use client";

import { useEffect, useRef, useState } from "react";
import { tokens } from "@openmind/ui";
import { useLiveSearch } from "./LiveSearch";

const { color, font } = tokens;

/**
 * The Mind's search pill.
 *
 * Typing does not navigate: each keystroke is answered from the local index
 * (see LiveSearch), and the server's ranking replaces that answer when it
 * arrives. Enter promotes the query to a real ?q= URL — the shareable,
 * server-rendered version with the understood-query echo — and Escape steps
 * back out of a live query, or out of a committed one when there is nothing
 * live to drop.
 */
export function SearchBox() {
  const { query, setQuery, commit, clear, warm } = useLiveSearch();
  const input = useRef<HTMLInputElement | null>(null);
  const [focused, setFocused] = useState(false);

  // The ⌘K badge in this pill was decorative for a long time. It isn't now.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        warm();
        input.current?.focus();
        input.current?.select();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [warm]);

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        commit();
      }}
      style={{ flex: "1 1 150px", minWidth: 130, maxWidth: 400 }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 10,
          padding: "10px 14px",
          background: color.cardSurface,
          border: `1px solid ${focused ? color.cobalt : "rgba(27,63,209,.35)"}`,
          borderRadius: 10,
          boxShadow: focused ? "0 0 0 3px rgba(27,63,209,.15)" : "none",
          transition: ".15s",
        }}
      >
        <span aria-hidden style={{ color: color.inkFaint, fontSize: 14, flex: "none" }}>
          ⌕
        </span>
        <input
          ref={input}
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onFocus={() => {
            setFocused(true);
            // Search intent: build the worker and start indexing now, so the
            // first keystroke has more than the visible page to match against.
            warm();
          }}
          onBlur={() => setFocused(false)}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              clear();
            }
          }}
          placeholder="Search a colour, a word, a vibe…"
          aria-label="Search your library"
          className="search-input"
          style={{
            flex: 1,
            minWidth: 0,
            border: "none",
            outline: "none",
            background: "transparent",
            fontFamily: font.sans,
            fontSize: 13,
            color: color.ink,
          }}
        />
        <span aria-hidden className="kbd">
          ⌘K
        </span>
      </div>
    </form>
  );
}
