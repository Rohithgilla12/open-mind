"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";

export function SearchBox({ initial }: { initial?: string }) {
  const router = useRouter();
  const [q, setQ] = useState(initial ?? "");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = q.trim();
    router.push(trimmed ? `/?q=${encodeURIComponent(trimmed)}` : "/");
  }

  return (
    <form onSubmit={handleSubmit} style={{ marginBottom: "1.5rem" }}>
      <input
        type="search"
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder="Search by keyword, colour, vibe…"
        style={{
          width: "100%",
          padding: "0.6rem 0.75rem",
          fontFamily: tokens.font.sans,
          fontSize: "0.95rem",
          border: `1px solid ${tokens.color.line}`,
          borderRadius: 8,
          backgroundColor: tokens.color.surface,
          color: tokens.color.ink,
        }}
      />
    </form>
  );
}
