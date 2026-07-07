import { useState } from "react";
import type { CSSProperties, KeyboardEvent } from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { tokens } from "@openmind/ui";
import { checkToken, claimDeviceCode } from "../lib/api";
import { setSettings, type Settings } from "../lib/settings";

type Status =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "valid" }
  | { kind: "invalid" }
  | { kind: "saved-unconfirmed"; reason: "unreachable" | "server"; code?: number }
  | { kind: "save-failed" }
  | { kind: "incomplete" };

type ConnectStatus =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "connected" }
  | { kind: "code-invalid" }
  | { kind: "rate-limited" }
  | { kind: "unreachable" }
  | { kind: "incomplete" }
  | { kind: "save-failed" };

export function SettingsView({
  initial,
  onSaved,
  onCancel,
}: {
  initial: Settings | null;
  onSaved: (settings: Settings) => void;
  onCancel?: () => void;
}) {
  const [instanceUrl, setInstanceUrl] = useState(initial?.instanceUrl ?? "");
  const [token, setToken] = useState(initial?.token ?? "");
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [deviceCode, setDeviceCode] = useState("");
  const [connectStatus, setConnectStatus] = useState<ConnectStatus>({ kind: "idle" });

  const checking = status.kind === "checking";
  const connecting = connectStatus.kind === "checking";

  async function onValidateAndSave() {
    const url = instanceUrl.trim().replace(/\/+$/, "");
    const tok = token.trim();
    if (!url || !tok) {
      setStatus({ kind: "incomplete" });
      return;
    }
    setStatus({ kind: "checking" });
    const settings: Settings = { instanceUrl: url, token: tok };
    const code = await checkToken(settings);
    // 401 is the only definitive "wrong token" — never persist it.
    if (code === 401) {
      setStatus({ kind: "invalid" });
      return;
    }
    // Every other result (200 confirmed; 0 unreachable; 429/5xx busy) still
    // persists the settings, so the panel is usable once the instance
    // recovers instead of forcing the user to re-enter the token.
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
    const url = instanceUrl.trim().replace(/\/+$/, "");
    const code = deviceCode.trim();
    if (!url || !code) {
      setConnectStatus({ kind: "incomplete" });
      return;
    }
    setConnectStatus({ kind: "checking" });
    const result = await claimDeviceCode(url, code, "Mac dock");
    if (!result.ok) {
      setConnectStatus(
        result.status === 0
          ? { kind: "unreachable" }
          : result.status === 429
            ? { kind: "rate-limited" }
            : { kind: "code-invalid" },
      );
      return;
    }
    const settings: Settings = { instanceUrl: url, token: result.key };
    try {
      await setSettings(settings);
    } catch {
      setConnectStatus({ kind: "save-failed" });
      return;
    }
    setInstanceUrl(url);
    setToken(result.key);
    setDeviceCode("");
    setConnectStatus({ kind: "connected" });
    onSaved(settings);
  }

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key === "Escape") {
      e.preventDefault();
      void getCurrentWindow().hide();
    }
  }

  return (
    <div style={styles.page} onKeyDown={onKeyDown}>
      <div style={styles.header}>
        <h1 style={styles.title}>Settings</h1>
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
          onChange={(e) => setInstanceUrl(e.target.value)}
          placeholder="https://openmind.example.com"
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
        />
      </label>

      <label style={styles.field}>
        <span style={styles.label}>API Token</span>
        <input
          style={styles.input}
          value={token}
          onChange={(e) => setToken(e.target.value)}
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

      <hr style={styles.divider} />

      <p style={styles.subtitle}>Or connect with a code from another device</p>

      <label style={styles.field}>
        <span style={styles.label}>Connect code</span>
        <input
          style={styles.input}
          value={deviceCode}
          onChange={(e) => setDeviceCode(e.target.value)}
          placeholder="ABCD-EFGH"
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
        />
      </label>

      <ConnectStatusMessage status={connectStatus} />

      <button
        type="button"
        style={{ ...styles.primaryButton, ...(connecting ? styles.disabled : {}) }}
        onClick={() => void onConnect()}
        disabled={connecting}
      >
        {connecting ? "Connecting…" : "Connect"}
      </button>
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

function ConnectStatusMessage({ status }: { status: ConnectStatus }) {
  switch (status.kind) {
    case "connected":
      return <p style={{ ...styles.status, color: tokens.color.cobalt }}>Connected — saved.</p>;
    case "code-invalid":
      return <p style={{ ...styles.status, color: tokens.color.danger }}>Invalid or expired code.</p>;
    case "rate-limited":
      return (
        <p style={{ ...styles.status, color: tokens.color.danger }}>
          Too many attempts — wait a moment and try again.
        </p>
      );
    case "unreachable":
      return <p style={{ ...styles.status, color: tokens.color.danger }}>Couldn't reach the instance.</p>;
    case "save-failed":
      return <p style={{ ...styles.status, color: tokens.color.danger }}>Couldn't save to the keychain — try again.</p>;
    case "incomplete":
      return <p style={{ ...styles.status, color: tokens.color.danger }}>Enter both an instance URL and a code.</p>;
    default:
      return null;
  }
}

const styles: Record<string, CSSProperties> = {
  page: {
    display: "flex",
    flexDirection: "column",
    gap: 4,
    padding: 20,
    fontFamily: tokens.font.sans,
    color: tokens.color.ink,
  },
  header: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
  },
  title: {
    fontFamily: tokens.font.quote,
    fontStyle: "italic",
    fontSize: 22,
    margin: 0,
  },
  closeButton: {
    border: "none",
    background: "none",
    fontSize: 20,
    lineHeight: 1,
    color: tokens.color.inkFaint,
    cursor: "pointer",
    padding: 4,
  },
  subtitle: {
    fontFamily: tokens.font.mono,
    fontSize: 11,
    color: tokens.color.inkFaint,
    margin: "2px 0 12px",
  },
  field: {
    display: "flex",
    flexDirection: "column",
    gap: 6,
    marginBottom: 12,
  },
  label: {
    fontFamily: tokens.font.mono,
    fontSize: 10,
    letterSpacing: "0.05em",
    textTransform: "uppercase",
    color: tokens.color.inkMuted,
  },
  input: {
    border: `1px solid ${tokens.color.hairline}`,
    borderRadius: 8,
    background: tokens.color.cardSurface,
    color: tokens.color.ink,
    padding: "9px 10px",
    fontSize: 14,
    fontFamily: tokens.font.sans,
  },
  status: {
    fontSize: 12,
    margin: "4px 0 10px",
  },
  divider: {
    border: "none",
    borderTop: `1px solid ${tokens.color.hairline}`,
    margin: "16px 0",
  },
  primaryButton: {
    border: "none",
    borderRadius: 8,
    background: tokens.color.cobalt,
    color: tokens.color.paper,
    padding: "10px 16px",
    fontSize: 14,
    fontWeight: 600,
    cursor: "pointer",
    marginTop: 4,
  },
  disabled: {
    opacity: 0.6,
    cursor: "default",
  },
};
