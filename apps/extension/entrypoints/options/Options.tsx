import { useEffect, useState } from "react";
import type { CSSProperties } from "react";
import { tokens } from "@openmind/ui";
import { getSettings, setSettings } from "../../lib/storage";
import { checkToken, claimDeviceCode } from "../../lib/save";

type ValidationState =
  | "idle"
  | "checking"
  | "valid"
  | "invalid"
  | "unreachable";

const VALIDATION_LABEL: Record<ValidationState, string> = {
  idle: "",
  checking: "Checking…",
  valid: "Token is valid",
  invalid: "Token rejected — check the value",
  unreachable: "Instance unreachable",
};

type ConnectState = "idle" | "invalid" | "unreachable";

const CONNECT_LABEL: Record<ConnectState, string> = {
  idle: "",
  invalid: "Invalid or expired code",
  unreachable: "Instance unreachable",
};

export function Options() {
  const [instanceUrl, setInstanceUrl] = useState("");
  const [token, setToken] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [saved, setSaved] = useState(false);
  const [validation, setValidation] = useState<ValidationState>("idle");
  const [deviceCode, setDeviceCode] = useState("");
  const [connecting, setConnecting] = useState(false);
  const [connectState, setConnectState] = useState<ConnectState>("idle");

  useEffect(() => {
    void getSettings().then((s) => {
      setInstanceUrl(s.instanceUrl);
      setToken(s.token);
      setLoaded(true);
    });
  }, []);

  async function handleSave() {
    await setSettings({ instanceUrl: instanceUrl.trim(), token: token.trim() });
    setSaved(true);
    window.setTimeout(() => setSaved(false), 2000);
  }

  async function handleValidate() {
    // Persist first so checkToken reads the current inputs.
    await setSettings({ instanceUrl: instanceUrl.trim(), token: token.trim() });
    setValidation("checking");
    const status = await checkToken();
    if (status === 200) {
      setValidation("valid");
    } else if (status === 401 || status === 429) {
      setValidation("invalid");
    } else {
      // 0 (network failure) or 502+ server errors.
      setValidation("unreachable");
    }
  }

  async function handleConnect() {
    const url = instanceUrl.trim();
    const code = deviceCode.trim();
    if (!url || !code) {
      setConnectState("invalid");
      return;
    }
    setConnecting(true);
    setConnectState("idle");
    const result = await claimDeviceCode(url, code, "Extension");
    setConnecting(false);
    if (!result.ok || !result.key) {
      setConnectState(result.status === 0 ? "unreachable" : "invalid");
      return;
    }
    await setSettings({ instanceUrl: url, token: result.key });
    setInstanceUrl(url);
    setToken(result.key);
    setDeviceCode("");
    setValidation("idle");
    setSaved(true);
    window.setTimeout(() => setSaved(false), 2000);
  }

  const validationColor =
    validation === "valid"
      ? tokens.color.cobalt
      : validation === "idle" || validation === "checking"
        ? tokens.color.ink
        : tokens.color.danger;

  if (!loaded) {
    return <div style={styles.page} />;
  }

  return (
    <div style={styles.page}>
      <main style={styles.card}>
        <h1 style={styles.heading}>Openmind settings</h1>
        <p style={styles.subtitle}>
          Connect this extension to your Openmind instance.
        </p>

        <label style={styles.label}>
          Instance URL
          <input
            style={styles.input}
            type="url"
            value={instanceUrl}
            placeholder="https://openmind.gilla.fun"
            onChange={(e) => {
              setInstanceUrl(e.target.value);
              setValidation("idle");
            }}
          />
        </label>

        <label style={styles.label}>
          Token
          <input
            style={styles.input}
            type="password"
            value={token}
            placeholder="Paste your access token"
            onChange={(e) => {
              setToken(e.target.value);
              setValidation("idle");
            }}
          />
        </label>

        <div style={styles.actions}>
          <button
            type="button"
            style={styles.primaryButton}
            onClick={handleSave}
          >
            Save settings
          </button>
          <button
            type="button"
            style={styles.secondaryButton}
            onClick={handleValidate}
            disabled={validation === "checking"}
          >
            Validate
          </button>
        </div>

        {saved && <p style={styles.savedNote}>Saved.</p>}
        {validation !== "idle" && (
          <p style={{ ...styles.validation, color: validationColor }}>
            {VALIDATION_LABEL[validation]}
          </p>
        )}

        <hr style={styles.divider} />

        <h2 style={styles.sectionHeading}>Connect with a code</h2>
        <p style={styles.subtitle}>
          Generate a code on a signed-in device, then enter it here.
        </p>

        <label style={styles.label}>
          Connect code
          <input
            style={styles.input}
            type="text"
            value={deviceCode}
            placeholder="ABCD-EFGH"
            autoCapitalize="off"
            autoCorrect="off"
            spellCheck={false}
            onChange={(e) => {
              setDeviceCode(e.target.value);
              setConnectState("idle");
            }}
          />
        </label>

        <div style={styles.actions}>
          <button
            type="button"
            style={{
              ...styles.primaryButton,
              ...(connecting ? styles.disabled : {}),
            }}
            onClick={() => void handleConnect()}
            disabled={connecting}
          >
            {connecting ? "Connecting…" : "Connect"}
          </button>
        </div>

        {connectState !== "idle" && (
          <p style={{ ...styles.validation, color: tokens.color.danger }}>
            {CONNECT_LABEL[connectState]}
          </p>
        )}
      </main>
    </div>
  );
}

const styles: Record<string, CSSProperties> = {
  page: {
    minHeight: "100vh",
    margin: 0,
    padding: "48px 24px",
    boxSizing: "border-box",
    background: tokens.color.paper,
    color: tokens.color.ink,
    fontFamily: tokens.font.sans,
    display: "flex",
    justifyContent: "center",
  },
  card: {
    width: "100%",
    maxWidth: 480,
    background: tokens.color.surface,
    border: `1px solid ${tokens.color.line}`,
    borderRadius: 12,
    padding: 32,
    boxSizing: "border-box",
  },
  heading: {
    margin: "0 0 4px",
    fontSize: 22,
    fontWeight: 600,
  },
  subtitle: {
    margin: "0 0 24px",
    fontSize: 14,
    opacity: 0.7,
  },
  label: {
    display: "flex",
    flexDirection: "column",
    gap: 6,
    marginBottom: 18,
    fontSize: 13,
    fontWeight: 500,
  },
  input: {
    fontFamily: tokens.font.mono,
    fontSize: 14,
    padding: "10px 12px",
    border: `1px solid ${tokens.color.line}`,
    borderRadius: 8,
    background: tokens.color.paper,
    color: tokens.color.ink,
  },
  actions: {
    display: "flex",
    gap: 12,
    marginTop: 8,
  },
  primaryButton: {
    fontFamily: tokens.font.sans,
    fontSize: 14,
    fontWeight: 500,
    padding: "10px 18px",
    border: "none",
    borderRadius: 8,
    background: tokens.color.cobalt,
    color: tokens.color.surface,
    cursor: "pointer",
  },
  secondaryButton: {
    fontFamily: tokens.font.sans,
    fontSize: 14,
    fontWeight: 500,
    padding: "10px 18px",
    border: `1px solid ${tokens.color.line}`,
    borderRadius: 8,
    background: tokens.color.surface,
    color: tokens.color.ink,
    cursor: "pointer",
  },
  savedNote: {
    marginTop: 16,
    fontSize: 13,
    color: tokens.color.cobalt,
  },
  validation: {
    marginTop: 16,
    fontSize: 13,
    fontWeight: 500,
  },
  divider: {
    border: "none",
    borderTop: `1px solid ${tokens.color.line}`,
    margin: "28px 0 20px",
  },
  sectionHeading: {
    margin: "0 0 4px",
    fontSize: 16,
    fontWeight: 600,
  },
  disabled: {
    opacity: 0.6,
    cursor: "default",
  },
};
