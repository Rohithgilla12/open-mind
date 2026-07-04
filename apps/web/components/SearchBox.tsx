"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";

const { color, font } = tokens;

export function SearchBox({ initial }: { initial?: string }) {
  const router = useRouter();
  const [q, setQ] = useState(initial ?? "");
  const [focused, setFocused] = useState(false);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = q.trim();
    router.push(trimmed ? `/?q=${encodeURIComponent(trimmed)}` : "/");
  }

  return (
    <form onSubmit={handleSubmit} style={{ flex: "1 1 150px", minWidth: 130, maxWidth: 400 }}>
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
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
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
