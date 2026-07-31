import { useEffect, useState } from "react";
import type { CSSProperties } from "react";
import { tokens } from "@openmind/ui";
import {
  HOSTED_INSTANCE_URL,
  getSettings,
  setSettings,
} from "../../lib/storage";
import { checkToken, claimDeviceCode } from "../../lib/save";
import {
  hasOriginAccess,
  originPattern,
  requestOriginAccess,
  revokeOriginAccess,
} from "../../lib/permissions";

type ValidationState =
  | "idle"
  | "checking"
  | "valid"
  | "invalid"
  | "unreachable"
  | "denied"
  | "bad-url";

const VALIDATION_LABEL: Record<ValidationState, string> = {
  idle: "",
  checking: "Checking…",
  valid: "Token is valid",
  invalid: "Token rejected — check the value",
  unreachable: "Instance unreachable",
  denied: "Access declined — Openmind can't reach that instance",
  "bad-url": "Enter a full URL, e.g. https://openmind.example.com",
};

type ConnectState = "idle" | "invalid" | "rate-limited" | "unreachable";

const CONNECT_LABEL: Record<ConnectState, string> = {
  idle: "",
  invalid: "Invalid or expired code",
  "rate-limited": "Too many attempts — wait a moment and try again",
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
  const [savedUrl, setSavedUrl] = useState("");

  useEffect(() => {
    void getSettings().then((s) => {
      setInstanceUrl(s.instanceUrl);
      setToken(s.token);
      setSavedUrl(s.instanceUrl);
      setLoaded(true);
    });
  }, []);

  /**
   * Ensure host access to `url` before any fetch against it. Must be reached
   * from a click: Chrome only shows the permission prompt during a user
   * gesture. Sets the matching validation state and returns false on failure.
   */
  async function ensureAccess(url: string): Promise<boolean> {
    if (!originPattern(url)) {
      setValidation("bad-url");
      return false;
    }
    if (await hasOriginAccess(url)) return true;
    if (await requestOriginAccess(url)) return true;
    setValidation("denied");
    return false;
  }

  /**
   * Persist the pair and drop the host grant for the origin we just moved away
   * from, so switching instances doesn't accumulate access to old servers.
   */
  async function persist(url: string, nextToken: string) {
    await setSettings({ instanceUrl: url, token: nextToken });
    if (savedUrl && originPattern(savedUrl) !== originPattern(url)) {
      await revokeOriginAccess(savedUrl);
    }
    setSavedUrl(url);
  }

  function flashSaved() {
    setSaved(true);
    window.setTimeout(() => setSaved(false), 2000);
  }

  async function handleSave() {
    const url = instanceUrl.trim();
    if (!(await ensureAccess(url))) return;
    await persist(url, token.trim());
    flashSaved();
  }

  async function handleUseHosted() {
    setInstanceUrl(HOSTED_INSTANCE_URL);
    setValidation("idle");
    if (!(await ensureAccess(HOSTED_INSTANCE_URL))) return;
    await persist(HOSTED_INSTANCE_URL, token.trim());
    flashSaved();
  }

  async function handleValidate() {
    const url = instanceUrl.trim();
    if (!(await ensureAccess(url))) return;
    // Persist first so checkToken reads the current inputs.
    await persist(url, token.trim());
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
    if (!(await ensureAccess(url))) return;
    setConnecting(true);
    setConnectState("idle");
    const result = await claimDeviceCode(url, code, "Extension");
    setConnecting(false);
    if (!result.ok || !result.key) {
      setConnectState(
        result.status === 0 ? "unreachable" : result.status === 429 ? "rate-limited" : "invalid",
      );
      return;
    }
    await persist(url, result.key);
    setInstanceUrl(url);
    setToken(result.key);
    setDeviceCode("");
    setValidation("idle");
    flashSaved();
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
            placeholder="https://openmind.example.com"
            onChange={(e) => {
              setInstanceUrl(e.target.value);
              setValidation("idle");
            }}
          />
        </label>

        <p style={styles.hostedHint}>
          Self-hosting your own Openmind? Paste its URL above. Otherwise you can{" "}
          <button
            type="button"
            style={styles.linkButton}
            onClick={() => void handleUseHosted()}
          >
            use the hosted instance
          </button>{" "}
          ({new URL(HOSTED_INSTANCE_URL).host}) — your saves are stored there,
          so only opt in if you trust it.
        </p>

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
  hostedHint: {
    margin: "-8px 0 20px",
    fontSize: 12,
    lineHeight: 1.5,
    color: tokens.color.inkMuted,
  },
  linkButton: {
    border: "none",
    background: "none",
    padding: 0,
    font: "inherit",
    fontWeight: 500,
    color: tokens.color.cobalt,
    cursor: "pointer",
    textDecoration: "underline",
  },
};
