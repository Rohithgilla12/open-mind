"use client";

import { tokens } from "@openmind/ui";
import QRCode from "qrcode";
import { useEffect, useRef, useState, type CSSProperties } from "react";
import { diffNotifySettings, type ChannelPref, type NotifyFormValues } from "../lib/notify-settings-diff";
import { composeQuietHours, parseQuietHours } from "../lib/quiet-hours";
import type { ApiKey, ApiKeyCreated, DeviceLinkCreated, Settings } from "../lib/types";

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

const fieldLabel: CSSProperties = {
  display: "block",
  fontFamily: font.sans,
  fontSize: 12.5,
  color: color.inkMuted,
  marginBottom: 6,
};

const fieldGroup: CSSProperties = {
  flex: "1 1 220px",
  minWidth: 200,
};

const CHANNEL_OPTIONS: { value: ChannelPref; label: string }[] = [
  { value: "off", label: "Off" },
  { value: "push", label: "Push" },
  { value: "email", label: "Email" },
  { value: "both", label: "Both" },
];

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

// Maps a fetched Settings response onto the six form fields, applying the
// same display defaults the server documents for an absent row (push / off
// / push / no quiet hours / 10) — except timezone, which prefills from the
// browser rather than "UTC" so nobody has to look up their own IANA name.
// This is also the "loaded" snapshot handleSave diffs the form against, so
// a save only sends fields the user actually changed.
function toFormValues(data: Settings): NotifyFormValues {
  return {
    digest: (data.notifyDigest as ChannelPref | undefined) ?? "push",
    feedRiver: (data.notifyFeedRiver as ChannelPref | undefined) ?? "off",
    lifecycle: (data.notifyLifecycle as ChannelPref | undefined) ?? "push",
    quietHours: data.notifyQuietHours ?? "",
    timezone: data.notifyTimezone ?? Intl.DateTimeFormat().resolvedOptions().timeZone,
    dailyCap: data.notifyDailyCap ?? 10,
  };
}

function NotificationsSection() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [loaded, setLoaded] = useState<NotifyFormValues | null>(null);
  const [digest, setDigest] = useState<ChannelPref>("push");
  const [feedRiver, setFeedRiver] = useState<ChannelPref>("off");
  const [lifecycle, setLifecycle] = useState<ChannelPref>("push");
  const [quietStart, setQuietStart] = useState("");
  const [quietEnd, setQuietEnd] = useState("");
  const [timezone, setTimezone] = useState("");
  const [dailyCap, setDailyCap] = useState("10");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);

  function applyToForm(values: NotifyFormValues) {
    setDigest(values.digest);
    setFeedRiver(values.feedRiver);
    setLifecycle(values.lifecycle);
    const { start, end } = parseQuietHours(values.quietHours);
    setQuietStart(start);
    setQuietEnd(end);
    setTimezone(values.timezone);
    setDailyCap(String(values.dailyCap));
  }

  useEffect(() => {
    let cancelled = false;
    setLoadFailed(false);
    fetch("/api/settings")
      .then((res) => (res.ok ? res.json() : Promise.reject(res.status)))
      .then((data: Settings) => {
        if (cancelled) return;
        setSettings(data);
        const values = toFormValues(data);
        setLoaded(values);
        applyToForm(values);
      })
      .catch(() => {
        if (!cancelled) setLoadFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [loadAttempt]);

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSaved(false);

    const cap = Number.parseInt(dailyCap, 10);
    if (!Number.isInteger(cap) || cap < 0 || cap > 200) {
      setError("Daily cap must be a whole number between 0 and 200.");
      return;
    }
    if (!loaded) return;

    const current: NotifyFormValues = {
      digest,
      feedRiver,
      lifecycle,
      quietHours: composeQuietHours(quietStart, quietEnd),
      timezone: timezone.trim(),
      dailyCap: cap,
    };
    const patch = diffNotifySettings(loaded, current);
    if (Object.keys(patch).length === 0) {
      setSaved(true);
      return;
    }

    setBusy(true);
    try {
      const res = await fetch("/api/settings", {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(patch),
      });
      if (res.status === 400) {
        const payload = (await res.json().catch(() => null)) as { error?: string } | null;
        setError(payload?.error ?? "Couldn't save — check the values above.");
        return;
      }
      if (!res.ok) throw new Error("save failed");
      const data = (await res.json()) as Settings;
      setSettings(data);
      const values = toFormValues(data);
      setLoaded(values);
      applyToForm(values);
      setSaved(true);
    } catch {
      setError("Couldn't save. Try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={card}>
      <div style={sectionTitle}>Notifications</div>
      <p
        className="meta"
        style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt, margin: "6px 0 4px" }}
      >
        This is where notifications are configured — push notifications arrive on your linked mobile
        devices, not in this browser. The web app never shows a notification itself.
      </p>

      {loadFailed && settings === null ? (
        <button
          type="button"
          onClick={() => setLoadAttempt((n) => n + 1)}
          style={{
            display: "block",
            margin: "6px 0 16px",
            font: `500 11px/1 ${font.mono}`,
            letterSpacing: ".02em",
            color: color.terracotta,
            background: color.noteSurface,
            border: `1px solid ${color.terracotta}`,
            borderRadius: 20,
            padding: "6px 12px",
            cursor: "pointer",
          }}
        >
          Couldn&apos;t load settings — retry
        </button>
      ) : settings === null ? null : (
        <form onSubmit={handleSave} style={{ marginTop: 14 }}>
          <div style={{ display: "flex", gap: 14, flexWrap: "wrap", marginBottom: 16 }}>
            <div style={fieldGroup}>
              <label style={fieldLabel} htmlFor="notify-digest">
                Lens digests
              </label>
              <select
                id="notify-digest"
                value={digest}
                onChange={(e) => setDigest(e.target.value as ChannelPref)}
                style={{ ...inputStyle, width: "100%" }}
              >
                {CHANNEL_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>

            <div style={fieldGroup}>
              <label style={fieldLabel} htmlFor="notify-feed-river">
                Feed activity
              </label>
              <select
                id="notify-feed-river"
                value={feedRiver}
                onChange={(e) => setFeedRiver(e.target.value as ChannelPref)}
                style={{ ...inputStyle, width: "100%" }}
              >
                {CHANNEL_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>

            <div style={fieldGroup}>
              <label style={fieldLabel} htmlFor="notify-lifecycle">
                Save failures
              </label>
              <select
                id="notify-lifecycle"
                value={lifecycle}
                onChange={(e) => setLifecycle(e.target.value as ChannelPref)}
                style={{ ...inputStyle, width: "100%" }}
              >
                {CHANNEL_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div style={{ display: "flex", gap: 14, flexWrap: "wrap", marginBottom: 16 }}>
            <div style={fieldGroup}>
              <label style={fieldLabel} htmlFor="notify-quiet-start">
                Quiet hours
              </label>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <input
                  id="notify-quiet-start"
                  type="time"
                  value={quietStart}
                  onChange={(e) => setQuietStart(e.target.value)}
                  aria-label="Quiet hours start"
                  style={{ ...inputStyle, flex: "1 1 auto" }}
                />
                <span style={{ fontFamily: font.mono, fontSize: 12, color: color.inkFaintAlt }}>to</span>
                <input
                  type="time"
                  value={quietEnd}
                  onChange={(e) => setQuietEnd(e.target.value)}
                  aria-label="Quiet hours end"
                  style={{ ...inputStyle, flex: "1 1 auto" }}
                />
              </div>
              <p style={{ fontFamily: font.sans, fontSize: 11.5, color: color.inkFaintAlt, margin: "6px 0 0" }}>
                Leave either side blank to turn quiet hours off.
              </p>
            </div>

            <div style={fieldGroup}>
              <label style={fieldLabel} htmlFor="notify-timezone">
                Timezone
              </label>
              <input
                id="notify-timezone"
                value={timezone}
                onChange={(e) => setTimezone(e.target.value)}
                placeholder="Europe/London"
                style={{ ...inputStyle, width: "100%" }}
              />
            </div>

            <div style={fieldGroup}>
              <label style={fieldLabel} htmlFor="notify-daily-cap">
                Daily cap
              </label>
              <input
                id="notify-daily-cap"
                type="number"
                min={0}
                max={200}
                value={dailyCap}
                onChange={(e) => setDailyCap(e.target.value)}
                style={{ ...inputStyle, width: "100%" }}
              />
              <p style={{ fontFamily: font.sans, fontSize: 11.5, color: color.inkFaintAlt, margin: "6px 0 0" }}>
                Save-failure alerts always get through, regardless of this cap.
              </p>
            </div>
          </div>

          <button type="submit" className="savebtn" disabled={busy} style={{ opacity: busy ? 0.6 : 1 }}>
            {busy ? "Saving…" : "Save"}
          </button>
        </form>
      )}

      {error ? (
        <p role="alert" style={errorStyle}>
          {error}
        </p>
      ) : saved ? (
        <p style={{ fontFamily: font.mono, fontSize: 12, color: color.green, margin: "10px 0 0" }}>Saved.</p>
      ) : null}
    </div>
  );
}

function KindleSection() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoadFailed(false);
    fetch("/api/settings")
      .then((res) => (res.ok ? res.json() : Promise.reject(res.status)))
      .then((data: Settings) => {
        if (cancelled) return;
        setSettings(data);
        setValue(data.kindleEmail ?? "");
      })
      .catch(() => {
        if (!cancelled) setLoadFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [loadAttempt]);

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      const trimmed = value.trim();
      const res = await fetch("/api/settings", {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ kindleEmail: trimmed }),
      });
      if (res.status === 400) {
        setError("That doesn't look like a valid e-mail address.");
        return;
      }
      if (!res.ok) throw new Error("save failed");
      const data = (await res.json()) as Settings;
      setSettings(data);
      setValue(data.kindleEmail ?? "");
      setSaved(true);
    } catch {
      setError("Couldn't save. Try again.");
    } finally {
      setBusy(false);
    }
  }

  const configured = Boolean(settings?.kindleEmail);

  return (
    <div style={card}>
      <div style={sectionTitle}>Send to Kindle</div>
      <p
        className="meta"
        style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt, margin: "6px 0 4px" }}
      >
        Items and Lens digests can be e-mailed to your Kindle as an EPUB.
      </p>
      {loadFailed && settings === null ? (
        <button
          type="button"
          onClick={() => setLoadAttempt((n) => n + 1)}
          style={{
            display: "block",
            margin: "6px 0 16px",
            font: `500 11px/1 ${font.mono}`,
            letterSpacing: ".02em",
            color: color.terracotta,
            background: color.noteSurface,
            border: `1px solid ${color.terracotta}`,
            borderRadius: 20,
            padding: "6px 12px",
            cursor: "pointer",
          }}
        >
          Couldn&apos;t load settings — retry
        </button>
      ) : settings === null ? null : (
        <p style={{ fontFamily: font.mono, fontSize: 12, color: color.inkFaintAlt, margin: "6px 0 16px" }}>
          {configured ? `Current address: ${settings?.kindleEmail}` : "Not set"}
        </p>
      )}

      <form onSubmit={handleSave} style={{ display: "flex", gap: 10, flexWrap: "wrap", marginBottom: 6 }}>
        <input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="you@kindle.com"
          aria-label="Kindle e-mail address"
          type="email"
          style={inputStyle}
        />
        <button type="submit" className="savebtn" disabled={busy} style={{ opacity: busy ? 0.6 : 1 }}>
          {busy ? "Saving…" : "Save"}
        </button>
      </form>

      {error ? (
        <p role="alert" style={errorStyle}>
          {error}
        </p>
      ) : saved ? (
        <p style={{ fontFamily: font.mono, fontSize: 12, color: color.green, margin: "10px 0 0" }}>
          Saved{configured ? "" : " — Kindle delivery is now off"}.
        </p>
      ) : null}

      <p style={{ fontFamily: font.sans, fontSize: 12.5, color: color.inkFaintAlt, margin: "14px 0 0" }}>
        In Amazon, go to <strong>Accounts &amp; Lists</strong> → <strong>Content &amp; Devices</strong> →{" "}
        <strong>Preferences</strong> → <strong>Personal Document Settings</strong>. Approve the sender address under{" "}
        <strong>Approved Personal Document E-mail List</strong>, then find your device&rsquo;s{" "}
        <strong>@kindle.com</strong> address under <strong>Send-to-Kindle E-mail Settings</strong> and enter it above.
      </p>
    </div>
  );
}

export function DevicesKeys() {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 24, maxWidth: 720 }}>
      <ConnectDeviceSection />
      <KeysSection />
      <AIOrganisationSection />
      <NotificationsSection />
      <KindleSection />
    </div>
  );
}

function AIOrganisationSection() {
  const [on, setOn] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoadFailed(false);
    fetch("/api/settings")
      .then((res) => (res.ok ? res.json() : Promise.reject(res.status)))
      .then((data: Settings) => {
        if (cancelled) return;
        setOn(Boolean(data.aiAssistedOrganisation));
        setLoaded(true);
      })
      .catch(() => {
        if (!cancelled) setLoadFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [loadAttempt]);

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSaved(false);
    setBusy(true);
    try {
      const res = await fetch("/api/settings", {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ aiAssistedOrganisation: on }),
      });
      if (!res.ok) throw new Error("save failed");
      const data = (await res.json()) as Settings;
      setOn(Boolean(data.aiAssistedOrganisation));
      setSaved(true);
    } catch {
      setError("Couldn't save. Try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={card}>
      <div style={sectionTitle}>AI-assisted organisation</div>
      <p
        className="meta"
        style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt, margin: "6px 0 14px" }}
      >
        When on (and the server has <code>OPENMIND_TYPESAFE_API_KEY</code>), each save may send the URL,
        title, site, and a short excerpt (~1.5–2k characters) to TypeSafe Jev for tagging judgments.
        Never the full document. Phase 1 only logs decisions — nothing user-visible changes yet.
      </p>
      {loadFailed && !loaded ? (
        <button type="button" onClick={() => setLoadAttempt((n) => n + 1)} style={{ cursor: "pointer" }}>
          Couldn&apos;t load settings — retry
        </button>
      ) : (
        <form onSubmit={handleSave} style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <label style={{ display: "flex", alignItems: "center", gap: 10, fontFamily: font.sans, fontSize: 13 }}>
            <input
              type="checkbox"
              checked={on}
              disabled={!loaded || busy}
              onChange={(e) => setOn(e.target.checked)}
            />
            Enable AI-assisted organisation
          </label>
          <button type="submit" className="savebtn" disabled={!loaded || busy} style={{ alignSelf: "flex-start", opacity: busy ? 0.6 : 1 }}>
            {busy ? "Saving…" : "Save"}
          </button>
          {error ? (
            <p role="alert" style={errorStyle}>
              {error}
            </p>
          ) : saved ? (
            <p style={{ fontFamily: font.mono, fontSize: 12, color: color.green, margin: 0 }}>Saved.</p>
          ) : null}
        </form>
      )}
    </div>
  );
}
