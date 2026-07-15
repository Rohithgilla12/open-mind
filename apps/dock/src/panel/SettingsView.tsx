import { useEffect, useState } from "react";
import type { CSSProperties, KeyboardEvent } from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { disable, enable, isEnabled } from "@tauri-apps/plugin-autostart";
import { tokens } from "@openmind/ui";
import { checkToken, claimDeviceCode } from "../lib/api";
import { clearSettings, setSettings, type Settings } from "../lib/settings";
import { DragRegion } from "../components/DragRegion";

type Status =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "valid" }
  | { kind: "invalid" }
  | { kind: "saved-unconfirmed"; reason: "unreachable" | "server"; code?: number }
  | { kind: "save-failed" }
  | { kind: "incomplete" };

type ClaimStatus = { kind: "idle" } | { kind: "claiming" } | { kind: "error"; message: string };

function resolveInstanceUrl(typed: string, saved?: string | null): string {
  return (typed || saved || "").trim().replace(/\/+$/, "");
}

export function SettingsView({
  initial,
  onSaved,
  onSignedOut,
  onCancel,
}: {
  initial: Settings | null;
  onSaved: (settings: Settings) => void;
  onSignedOut?: () => void;
  onCancel?: () => void;
}) {
  const [instanceUrl, setInstanceUrl] = useState(initial?.instanceUrl ?? "");
  const [token, setToken] = useState(initial?.token ?? "");
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [deviceCode, setDeviceCode] = useState("");
  const [claimStatus, setClaimStatus] = useState<ClaimStatus>({ kind: "idle" });
  const [launchAtLogin, setLaunchAtLogin] = useState(false);
  const [launchBusy, setLaunchBusy] = useState(false);
  const [launchError, setLaunchError] = useState<string | null>(null);

  const checking = status.kind === "checking";
  const claiming = claimStatus.kind === "claiming";

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const on = await isEnabled();
        if (!cancelled) setLaunchAtLogin(on);
      } catch {
        // Plugin unavailable in non-Tauri contexts (vitest) — leave off.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function onToggleLaunchAtLogin() {
    setLaunchBusy(true);
    setLaunchError(null);
    try {
      if (launchAtLogin) {
        await disable();
        setLaunchAtLogin(false);
      } else {
        await enable();
        setLaunchAtLogin(true);
      }
    } catch {
      setLaunchError("Couldn't update login item — try again.");
    } finally {
      setLaunchBusy(false);
    }
  }

  async function onValidateAndSave() {
    const url = resolveInstanceUrl(instanceUrl);
    const tok = token.trim();
    if (!url || !tok) {
      setStatus({ kind: "incomplete" });
      return;
    }
    setStatus({ kind: "checking" });
    const settings: Settings = { instanceUrl: url, token: tok };
    const code = await checkToken(settings);
    if (code === 401) {
      setStatus({ kind: "invalid" });
      return;
    }
    try {
      await setSettings(settings);
    } catch {
      setStatus({ kind: "save-failed" });
      return;
    }
    if (code === 200) {
      setStatus({ kind: "valid" });
    } else if (code === 0) {
      setStatus({ kind: "saved-unconfirmed", reason: "unreachable" });
    } else {
      setStatus({ kind: "saved-unconfirmed", reason: "server", code });
    }
    onSaved(settings);
  }

  async function onConnect() {
    const url = resolveInstanceUrl(instanceUrl, initial?.instanceUrl);
    const codeInput = deviceCode.trim();
    if (!url) {
      setClaimStatus({
        kind: "error",
        message: "Enter your instance URL first — the code only works with a specific server.",
      });
      return;
    }
    if (!codeInput) {
      setClaimStatus({
        kind: "error",
        message: "Enter the connect code from your Openmind web app.",
      });
      return;
    }
    setClaimStatus({ kind: "claiming" });
    const result = await claimDeviceCode(url, codeInput, "Mac dock");
    if (!result.ok) {
      setClaimStatus({
        kind: "error",
        message:
          result.status === 0
            ? "Couldn't reach that instance — check the URL."
            : result.status === 429
              ? "Too many attempts — wait a moment and try again."
              : "Invalid, expired, or already-used code.",
      });
      return;
    }
    const settings: Settings = { instanceUrl: url, token: result.key };
    try {
      await setSettings(settings);
    } catch {
      setClaimStatus({ kind: "error", message: "Couldn't save to the keychain — try again." });
      return;
    }
    setInstanceUrl(url);
    setToken(result.key);
    setDeviceCode("");
    setClaimStatus({ kind: "idle" });
    setStatus({ kind: "valid" });
    onSaved(settings);
  }

  async function onSignOut() {
    try {
      await clearSettings();
    } catch {
      setStatus({ kind: "save-failed" });
      return;
    }
    setInstanceUrl("");
    setToken("");
    setDeviceCode("");
    setStatus({ kind: "idle" });
    setClaimStatus({ kind: "idle" });
    onSignedOut?.();
  }

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key === "Escape") {
      e.preventDefault();
      if (onCancel) {
        onCancel();
      } else {
        void getCurrentWindow().hide();
      }
    }
  }

  return (
    <div style={styles.page} onKeyDown={onKeyDown}>
      <div style={styles.scroll}>
        <div style={styles.header}>
          <DragRegion style={styles.headerDrag}>
            <h1 style={styles.title}>Settings</h1>
          </DragRegion>
          {onCancel ? (
            <button type="button" style={styles.closeButton} onClick={onCancel} aria-label="Close settings">
              ×
            </button>
          ) : null}
        </div>
        <p style={styles.subtitle}>Connect to your Openmind instance</p>

        <label style={styles.field}>
          <span style={styles.label}>Instance URL</span>
          <input
            style={styles.input}
            value={instanceUrl}
            onChange={(e) => {
              setInstanceUrl(e.target.value);
              if (claimStatus.kind === "error") setClaimStatus({ kind: "idle" });
            }}
            placeholder="https://openmind.example.com"
            autoCapitalize="off"
            autoCorrect="off"
            spellCheck={false}
            inputMode="url"
          />
        </label>

        <h2 style={styles.sectionHeading}>Connect with code</h2>
        <p style={styles.sectionHint}>
          On the web app, open Settings → Devices & keys, generate a code, then enter it here.
        </p>

        <label style={styles.field}>
          <span style={styles.label}>Connect code</span>
          <input
            style={styles.input}
            value={deviceCode}
            onChange={(e) => {
              setDeviceCode(e.target.value);
              if (claimStatus.kind === "error") setClaimStatus({ kind: "idle" });
            }}
            placeholder="ABCD-EFGH"
            autoCapitalize="characters"
            autoCorrect="off"
            spellCheck={false}
          />
        </label>

        <ClaimStatusMessage status={claimStatus} />

        <button
          type="button"
          style={{ ...styles.outlineButton, ...(claiming ? styles.disabled : {}) }}
          onClick={() => void onConnect()}
          disabled={claiming}
        >
          {claiming ? "Connecting…" : "Connect"}
        </button>

        <p style={styles.divider}>Or connect manually</p>

        <label style={styles.field}>
          <span style={styles.label}>API token</span>
          <input
            style={styles.input}
            value={token}
            onChange={(e) => {
              setToken(e.target.value);
              if (status.kind !== "idle" && status.kind !== "checking") setStatus({ kind: "idle" });
            }}
            placeholder="Paste your API token"
            type="password"
            autoCapitalize="off"
            autoCorrect="off"
            spellCheck={false}
          />
        </label>

        <StatusMessage status={status} />

        <button
          type="button"
          style={{ ...styles.primaryButton, ...(checking ? styles.disabled : {}) }}
          onClick={() => void onValidateAndSave()}
          disabled={checking}
        >
          {checking ? "Checking…" : "Validate & save"}
        </button>

        <div style={styles.launchRow}>
          <div style={styles.launchCopy}>
            <span style={styles.launchTitle}>Launch at login</span>
            <span style={styles.launchHint}>Start the dock in the menu bar when you sign in to macOS.</span>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={launchAtLogin}
            aria-label="Launch at login"
            disabled={launchBusy}
            onClick={() => void onToggleLaunchAtLogin()}
            style={{
              ...styles.switchTrack,
              ...(launchAtLogin ? styles.switchTrackOn : {}),
              ...(launchBusy ? styles.disabled : {}),
            }}
          >
            <span
              style={{
                ...styles.switchThumb,
                ...(launchAtLogin ? styles.switchThumbOn : {}),
              }}
            />
          </button>
        </div>
        {launchError ? <p style={{ ...styles.status, color: tokens.color.danger }}>{launchError}</p> : null}

        {initial ? (
          <button type="button" style={styles.signOutButton} onClick={() => void onSignOut()}>
            Sign out
          </button>
        ) : null}
      </div>
    </div>
  );
}

function StatusMessage({ status }: { status: Status }) {
  switch (status.kind) {
    case "valid":
      return <p style={{ ...styles.status, color: tokens.color.cobalt }}>Token valid — saved.</p>;
    case "invalid":
      return <p style={{ ...styles.status, color: tokens.color.danger }}>Invalid token (401).</p>;
    case "saved-unconfirmed":
      return (
        <p style={{ ...styles.status, color: tokens.color.gold }}>
          {status.reason === "unreachable"
            ? "Saved — but couldn't reach the instance to confirm. It'll work once the instance is reachable."
            : `Saved — but the instance was busy${status.code ? ` (${status.code})` : ""}, so the token isn't confirmed yet.`}
        </p>
      );
    case "save-failed":
      return <p style={{ ...styles.status, color: tokens.color.danger }}>Couldn't save to the keychain — try again.</p>;
    case "incomplete":
      return <p style={{ ...styles.status, color: tokens.color.danger }}>Enter both an instance URL and a token.</p>;
    default:
      return null;
  }
}

function ClaimStatusMessage({ status }: { status: ClaimStatus }) {
  if (status.kind !== "error") return null;
  return <p style={{ ...styles.status, color: tokens.color.danger }}>{status.message}</p>;
}

const styles: Record<string, CSSProperties> = {
  page: {
    display: "flex",
    flexDirection: "column",
    height: "100%",
    minHeight: 0,
    fontFamily: tokens.font.sans,
    color: tokens.color.ink,
  },
  scroll: {
    flex: 1,
    overflowY: "auto",
    padding: "20px 22px 24px",
    display: "flex",
    flexDirection: "column",
    gap: 0,
  },
  header: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    marginBottom: 2,
    gap: 8,
  },
  headerDrag: {
    flex: 1,
    minWidth: 0,
  },
  title: {
    fontFamily: tokens.font.quote,
    fontStyle: "italic",
    fontWeight: 600,
    fontSize: 27,
    margin: 0,
    letterSpacing: "-0.02em",
  },
  closeButton: {
    border: "none",
    background: "none",
    fontSize: 22,
    lineHeight: 1,
    color: tokens.color.inkFaint,
    cursor: "pointer",
    padding: 4,
  },
  subtitle: {
    fontFamily: tokens.font.mono,
    fontSize: 12,
    color: tokens.color.inkFaint,
    margin: "0 0 20px",
  },
  sectionHeading: {
    fontSize: 15,
    fontWeight: 600,
    margin: "4px 0 6px",
    fontFamily: tokens.font.sans,
  },
  sectionHint: {
    fontSize: 13,
    lineHeight: 1.45,
    color: tokens.color.inkMuted,
    margin: "0 0 14px",
  },
  field: {
    display: "flex",
    flexDirection: "column",
    gap: 6,
    marginBottom: 14,
  },
  label: {
    fontFamily: tokens.font.mono,
    fontSize: 10,
    letterSpacing: "0.08em",
    textTransform: "uppercase",
    color: tokens.color.inkMuted,
  },
  input: {
    border: `1px solid ${tokens.color.hairline}`,
    borderRadius: 10,
    background: tokens.color.cardSurface,
    color: tokens.color.ink,
    padding: "11px 12px",
    fontSize: 15,
    fontFamily: tokens.font.sans,
  },
  status: {
    fontSize: 13,
    lineHeight: 1.4,
    margin: "0 0 12px",
  },
  divider: {
    fontFamily: tokens.font.mono,
    fontSize: 10,
    letterSpacing: "0.08em",
    textTransform: "uppercase",
    color: tokens.color.inkFaint,
    textAlign: "center",
    margin: "20px 0 18px",
  },
  primaryButton: {
    border: "none",
    borderRadius: 10,
    background: tokens.color.cobalt,
    color: tokens.color.paper,
    padding: "12px 16px",
    fontSize: 15,
    fontWeight: 600,
    cursor: "pointer",
    marginTop: 4,
  },
  outlineButton: {
    border: `1px solid ${tokens.color.cobalt}`,
    borderRadius: 10,
    background: "transparent",
    color: tokens.color.cobalt,
    padding: "12px 16px",
    fontSize: 15,
    fontWeight: 600,
    cursor: "pointer",
    marginTop: 2,
  },
  launchRow: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 16,
    marginTop: 22,
    padding: "14px 0 4px",
    borderTop: `1px solid ${tokens.color.hairline}`,
  },
  launchCopy: {
    display: "flex",
    flexDirection: "column",
    gap: 4,
    minWidth: 0,
  },
  launchTitle: {
    fontSize: 15,
    fontWeight: 600,
  },
  launchHint: {
    fontSize: 12,
    lineHeight: 1.4,
    color: tokens.color.inkMuted,
  },
  switchTrack: {
    width: 44,
    height: 26,
    borderRadius: 999,
    border: `1px solid ${tokens.color.hairline}`,
    background: tokens.color.panel,
    padding: 2,
    cursor: "pointer",
    flex: "none",
    position: "relative",
  },
  switchTrackOn: {
    background: tokens.color.cobalt,
    borderColor: tokens.color.cobalt,
  },
  switchThumb: {
    display: "block",
    width: 20,
    height: 20,
    borderRadius: 999,
    background: tokens.color.cardSurface,
    transform: "translateX(0)",
    transition: "transform 120ms ease",
  },
  switchThumbOn: {
    transform: "translateX(18px)",
  },
  signOutButton: {
    border: `1px solid ${tokens.color.hairline}`,
    borderRadius: 10,
    background: "transparent",
    color: tokens.color.danger,
    padding: "12px 16px",
    fontSize: 15,
    fontWeight: 600,
    cursor: "pointer",
    marginTop: 14,
  },
  disabled: {
    opacity: 0.65,
    cursor: "default",
  },
};
