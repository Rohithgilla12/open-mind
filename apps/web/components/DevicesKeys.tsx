"use client";

import { tokens } from "@openmind/ui";
import QRCode from "qrcode";
import { useEffect, useRef, useState, type CSSProperties } from "react";
import type { ApiKey, ApiKeyCreated, DeviceLinkCreated } from "../lib/types";

const { color, font } = tokens;

const sectionTitle: CSSProperties = {
  fontFamily: font.sans,
  fontSize: 13,
  fontWeight: 600,
  color: color.ink,
};

const card: CSSProperties = {
  background: color.cardSurface,
  border: `1px solid ${color.hairline}`,
  borderRadius: 12,
  padding: "20px 22px",
};

const errorStyle: CSSProperties = {
  fontFamily: font.mono,
  fontSize: 12,
  color: color.danger,
  margin: "10px 0 0",
};

const inputStyle: CSSProperties = {
  flex: "1 1 220px",
  fontFamily: font.mono,
  fontSize: 13,
  color: color.ink,
  background: color.paper,
  border: `1px solid ${color.hairline}`,
  borderRadius: 10,
  padding: "10px 12px",
};

function formatDate(iso?: string): string {
  if (!iso) return "never";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "never";
  return d.toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
}

function KeyRow({ apiKey, onRevoke }: { apiKey: ApiKey; onRevoke: (id: string) => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const revoked = Boolean(apiKey.revokedAt);

  async function handleRevoke() {
    if (!window.confirm(`Revoke “${apiKey.name}”? Anything using this key will stop working.`)) return;
    setBusy(true);
    try {
      await onRevoke(apiKey.id);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "12px 0",
        borderBottom: `1px solid ${color.hairline}`,
      }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontFamily: font.sans, fontSize: 13.5, color: color.ink }}>{apiKey.name}</div>
        <div
          className="meta"
          style={{
            marginTop: 3,
            textTransform: "none",
            letterSpacing: ".02em",
            color: color.inkFaintAlt,
            display: "flex",
            gap: 8,
            flexWrap: "wrap",
          }}
        >
          <span style={{ fontFamily: font.mono }}>{apiKey.prefix}</span>
          <span aria-hidden>·</span>
          <span>created {formatDate(apiKey.createdAt)}</span>
          <span aria-hidden>·</span>
          <span>last used {apiKey.lastUsedAt ? formatDate(apiKey.lastUsedAt) : "never"}</span>
        </div>
      </div>
      {revoked ? (
        <span
          className="meta"
          style={{
            color: color.danger,
            background: `color-mix(in srgb, ${color.danger} 12%, transparent)`,
            border: `1px solid color-mix(in srgb, ${color.danger} 32%, transparent)`,
            borderRadius: 999,
            padding: "3px 9px",
          }}
        >
          revoked
        </span>
      ) : (
        <button
          type="button"
          onClick={handleRevoke}
          disabled={busy}
          style={{
            flex: "none",
            fontFamily: font.mono,
            fontSize: 11,
            letterSpacing: ".04em",
            color: color.inkFaintAlt,
            background: "transparent",
            border: `1px solid ${color.hairline}`,
            borderRadius: 8,
            padding: "6px 10px",
            cursor: busy ? "default" : "pointer",
            opacity: busy ? 0.6 : 1,
          }}
        >
          {busy ? "Revoking…" : "Revoke"}
        </button>
      )}
    </div>
  );
}

function KeysSection() {
  const [keys, setKeys] = useState<ApiKey[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<ApiKeyCreated | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/api-keys")
      .then((res) => (res.ok ? res.json() : Promise.reject(res.status)))
      .then((data: ApiKey[]) => {
        if (!cancelled) setKeys(data);
      })
      .catch(() => {
        if (!cancelled) setKeys([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) {
      setError("Enter a name for the key");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const res = await fetch("/api/api-keys", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ name: trimmed }),
      });
      if (!res.ok) throw new Error("create failed");
      const data = (await res.json()) as ApiKeyCreated;
      setCreated(data);
      setCopied(false);
      setName("");
      setKeys((prev) => [
        { id: data.id, name: data.name, prefix: data.prefix, createdAt: data.createdAt },
        ...(prev ?? []),
      ]);
    } catch {
      setError("Couldn't create key. Try again.");
    } finally {
      setBusy(false);
    }
  }

  async function handleRevoke(id: string) {
    const prev = keys;
    setKeys((cur) =>
      (cur ?? []).map((k) => (k.id === id ? { ...k, revokedAt: new Date().toISOString() } : k)),
    );
    setError(null);
    try {
      const res = await fetch(`/api/api-keys/${id}`, { method: "DELETE" });
      if (res.status !== 204) throw new Error("revoke failed");
    } catch {
      setKeys(prev ?? null);
      setError("Couldn't revoke key. Try again.");
    }
  }

  async function copyKey() {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.key);
      setCopied(true);
    } catch {
      setError("Couldn't copy — select and copy manually.");
    }
  }

  return (
    <div style={card}>
      <div style={sectionTitle}>API keys</div>
      <p
        className="meta"
        style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt, margin: "6px 0 16px" }}
      >
        Keys authenticate scripts, the CLI, and third-party integrations.
      </p>

      {created ? (
        <div
          style={{
            marginBottom: 18,
            padding: "16px 18px",
            background: color.paper,
            border: `1.5px dashed ${color.gold}`,
            borderRadius: 10,
          }}
        >
          <div className="meta" style={{ color: color.inkFaintAlt, marginBottom: 8 }}>
            New key — “{created.name}”
          </div>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 10,
              flexWrap: "wrap",
            }}
          >
            <code
              style={{
                fontFamily: font.mono,
                fontSize: 13.5,
                color: color.ink,
                background: color.cardSurface,
                border: `1px solid ${color.hairline}`,
                borderRadius: 8,
                padding: "8px 12px",
                overflowWrap: "anywhere",
              }}
            >
              {created.key}
            </code>
            <button
              type="button"
              onClick={copyKey}
              className="savebtn"
              style={{ flex: "none" }}
            >
              {copied ? "Copied" : "Copy"}
            </button>
          </div>
          <p style={{ fontFamily: font.sans, fontSize: 12.5, color: color.danger, margin: "10px 0 0" }}>
            You won&rsquo;t see this key again — copy it now.
          </p>
        </div>
      ) : null}

      <form onSubmit={handleCreate} style={{ display: "flex", gap: 10, flexWrap: "wrap", marginBottom: 6 }}>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Key name, e.g. laptop CLI"
          aria-label="Key name"
          style={inputStyle}
        />
        <button type="submit" className="savebtn" disabled={busy} style={{ opacity: busy ? 0.6 : 1 }}>
          {busy ? "Creating…" : "New key"}
        </button>
      </form>

      {error ? (
        <p role="alert" style={errorStyle}>
          {error}
        </p>
      ) : null}

      {keys === null ? null : keys.length === 0 ? (
        <p
          className="meta"
          style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaint, margin: "14px 0 0" }}
        >
          No keys yet.
        </p>
      ) : (
        <div style={{ marginTop: 14 }}>
          {keys.map((k) => (
            <KeyRow key={k.id} apiKey={k} onRevoke={handleRevoke} />
          ))}
        </div>
      )}
    </div>
  );
}

function useCountdown(expiresAt: string | null): string {
  const [label, setLabel] = useState("--:--");

  useEffect(() => {
    if (!expiresAt) {
      setLabel("--:--");
      return;
    }
    const target = new Date(expiresAt).getTime();
    function tick() {
      const remaining = Math.max(0, Math.round((target - Date.now()) / 1000));
      const mins = Math.floor(remaining / 60);
      const secs = remaining % 60;
      setLabel(`${String(mins).padStart(2, "0")}:${String(secs).padStart(2, "0")}`);
    }
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [expiresAt]);

  return label;
}

function ConnectDeviceSection() {
  const [link, setLink] = useState<DeviceLinkCreated | null>(null);
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const countdown = useCountdown(link?.expiresAt ?? null);
  const cancelledRef = useRef(false);

  useEffect(() => {
    return () => {
      cancelledRef.current = true;
    };
  }, []);

  async function generate() {
    setBusy(true);
    setError(null);
    try {
      const res = await fetch("/api/device-links", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ deviceHint: "web" }),
      });
      if (!res.ok) throw new Error("device link failed");
      const data = (await res.json()) as DeviceLinkCreated;
      const payload = `openmind://link?code=${data.code}&url=${window.location.origin}`;
      const dataUrl = await QRCode.toDataURL(payload, { width: 200, margin: 2 });
      if (cancelledRef.current) return;
      setLink(data);
      setQrDataUrl(dataUrl);
    } catch {
      if (!cancelledRef.current) setError("Couldn't generate a code. Try again.");
    } finally {
      if (!cancelledRef.current) setBusy(false);
    }
  }

  const expired = link ? new Date(link.expiresAt).getTime() <= Date.now() : false;

  return (
    <div style={card}>
      <div style={sectionTitle}>Connect a device</div>
      <p
        className="meta"
        style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt, margin: "6px 0 16px" }}
      >
        Scan the code from another device to link it — the code works once and expires quickly.
      </p>

      {!link ? (
        <button type="button" onClick={generate} className="savebtn" disabled={busy} style={{ opacity: busy ? 0.6 : 1 }}>
          {busy ? "Generating…" : "Generate code"}
        </button>
      ) : (
        <div style={{ display: "flex", gap: 24, flexWrap: "wrap", alignItems: "center" }}>
          {qrDataUrl ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={qrDataUrl}
              width={200}
              height={200}
              alt="Scan to connect a device"
              style={{ borderRadius: 10, border: `1px solid ${color.hairline}` }}
            />
          ) : null}
          <div>
            <div
              style={{
                fontFamily: font.mono,
                fontSize: 40,
                fontWeight: 600,
                letterSpacing: ".04em",
                color: color.ink,
              }}
            >
              {link.code}
            </div>
            <div
              className="meta"
              style={{
                marginTop: 8,
                textTransform: "none",
                letterSpacing: ".02em",
                color: expired ? color.danger : color.inkFaintAlt,
              }}
            >
              {expired ? "Expired" : `Expires in ${countdown}`}
            </div>
            <button
              type="button"
              onClick={generate}
              disabled={busy}
              style={{
                marginTop: 14,
                flex: "none",
                fontFamily: font.mono,
                fontSize: 11,
                letterSpacing: ".04em",
                color: color.cobalt,
                background: "transparent",
                border: `1px solid ${color.hairline}`,
                borderRadius: 8,
                padding: "8px 12px",
                cursor: busy ? "default" : "pointer",
                opacity: busy ? 0.6 : 1,
              }}
            >
              {busy ? "Generating…" : "Generate new code"}
            </button>
          </div>
        </div>
      )}

      {error ? (
        <p role="alert" style={errorStyle}>
          {error}
        </p>
      ) : null}
    </div>
  );
}

export function DevicesKeys() {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 24, maxWidth: 720 }}>
      <ConnectDeviceSection />
      <KeysSection />
    </div>
  );
}
