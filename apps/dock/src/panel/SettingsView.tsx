import { useState } from "react";
import type { CSSProperties, KeyboardEvent } from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { tokens } from "@openmind/ui";
import { checkToken } from "../lib/api";
import { setSettings, type Settings } from "../lib/settings";

type Status =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "valid" }
  | { kind: "invalid" }
  | { kind: "saved-unconfirmed"; reason: "unreachable" | "server"; code?: number }
  | { kind: "save-failed" }
  | { kind: "incomplete" };

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

  const checking = status.kind === "checking";

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
