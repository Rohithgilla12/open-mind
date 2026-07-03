"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";

export function LoginForm() {
  const router = useRouter();
  const [token, setToken] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const res = await fetch("/api/auth", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ token }),
      });
      if (res.ok) {
        router.push("/");
        return;
      }
      setError("Invalid token");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        backgroundColor: tokens.color.paper,
      }}
    >
      <form
        onSubmit={handleSubmit}
        style={{
          display: "flex",
          flexDirection: "column",
          gap: "1rem",
          width: "100%",
          maxWidth: "360px",
          padding: "2rem",
          border: `1px solid ${tokens.color.line}`,
          borderRadius: "8px",
          backgroundColor: tokens.color.surface,
        }}
      >
        <h1
          style={{
            color: tokens.color.ink,
            fontFamily: tokens.font.sans,
            fontSize: "1.25rem",
            margin: 0,
          }}
        >
          Sign in to Openmind
        </h1>
        <input
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="API token"
          autoFocus
          style={{
            padding: "0.6rem 0.75rem",
            fontFamily: tokens.font.mono,
            border: `1px solid ${tokens.color.line}`,
            borderRadius: "6px",
            color: tokens.color.ink,
          }}
        />
        {error ? (
          <p style={{ color: tokens.color.danger, fontFamily: tokens.font.sans, fontSize: "0.875rem", margin: 0 }}>
            {error}
          </p>
        ) : null}
        <button
          type="submit"
          disabled={submitting || !token}
          style={{
            padding: "0.6rem 0.75rem",
            fontFamily: tokens.font.sans,
            fontWeight: 600,
            color: tokens.color.surface,
            backgroundColor: tokens.color.cobalt,
            border: "none",
            borderRadius: "6px",
            cursor: submitting || !token ? "not-allowed" : "pointer",
            opacity: submitting || !token ? 0.6 : 1,
          }}
        >
          {submitting ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </div>
  );
}
