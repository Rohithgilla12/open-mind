import { useEffect, useState } from "react";
import type { CSSProperties } from "react";
import { browser } from "wxt/browser";
import { tokens } from "@openmind/ui";
import { getSettings } from "../../lib/storage";
import { saveItem } from "../../lib/save";

type SaveState = "idle" | "saving" | "saved" | "error" | "needs-settings";

interface ActiveTab {
  title: string;
  url: string;
}

export function Popup() {
  const [tab, setTab] = useState<ActiveTab | null>(null);
  const [hasToken, setHasToken] = useState<boolean>(true);
  const [state, setState] = useState<SaveState>("idle");
  const [errorText, setErrorText] = useState<string>("");

  useEffect(() => {
    void (async () => {
      const [active] = await browser.tabs.query({
        active: true,
        currentWindow: true,
      });
      if (active?.url) {
        setTab({ title: active.title ?? active.url, url: active.url });
      }
      const { token } = await getSettings();
      setHasToken(token.trim().length > 0);
    })();
  }, []);

  function openSettings() {
    void browser.runtime.openOptionsPage();
  }

  async function handleSave() {
    if (!tab) return;
    setState("saving");
    const res = await saveItem({ url: tab.url });
    if (res.ok) {
      setState("saved");
    } else if (res.status === 401) {
      setState("needs-settings");
    } else if (res.status === 0) {
      setState("error");
      setErrorText("Instance unreachable.");
    } else {
      setState("error");
      setErrorText(`Save failed (error ${res.status}).`);
    }
  }

  const canSave =
    hasToken && tab !== null && state !== "saving" && state !== "saved";

  return (
    <div style={styles.page}>
      <h1 style={styles.heading}>Save to Openmind</h1>

      {tab ? (
        <div style={styles.tabCard}>
          <div style={styles.tabTitle}>{tab.title}</div>
          <div style={styles.tabUrl}>{tab.url}</div>
        </div>
      ) : (
        <p style={styles.muted}>No active tab to save.</p>
      )}

      {!hasToken || state === "needs-settings" ? (
        <>
          <p style={styles.error}>
            {state === "needs-settings"
              ? "Token invalid or missing."
              : "No token configured yet."}
          </p>
          <button
            type="button"
            style={styles.primaryButton}
            onClick={openSettings}
          >
            Open settings
          </button>
        </>
      ) : (
        <>
          <button
            type="button"
            style={{
              ...styles.primaryButton,
              ...(canSave ? {} : styles.disabledButton),
            }}
            onClick={handleSave}
            disabled={!canSave}
          >
            {state === "saving"
              ? "Saving…"
              : state === "saved"
                ? "Saved ✓"
                : "Save page"}
          </button>
          {state === "error" && <p style={styles.error}>{errorText}</p>}
        </>
      )}
    </div>
  );
}

const styles: Record<string, CSSProperties> = {
  page: {
    width: 320,
    boxSizing: "border-box",
    margin: 0,
    padding: 16,
    background: tokens.color.paper,
    color: tokens.color.ink,
    fontFamily: tokens.font.sans,
  },
  heading: {
    margin: "0 0 12px",
    fontSize: 16,
    fontWeight: 600,
  },
  tabCard: {
    background: tokens.color.surface,
    border: `1px solid ${tokens.color.line}`,
    borderRadius: 8,
    padding: 12,
    marginBottom: 12,
  },
  tabTitle: {
    fontSize: 13,
    fontWeight: 500,
    marginBottom: 4,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  tabUrl: {
    fontFamily: tokens.font.mono,
    fontSize: 11,
    opacity: 0.6,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  muted: {
    fontSize: 13,
    opacity: 0.7,
    margin: "0 0 12px",
  },
  primaryButton: {
    width: "100%",
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
  disabledButton: {
    opacity: 0.5,
    cursor: "default",
  },
  error: {
    marginTop: 12,
    marginBottom: 12,
    fontSize: 13,
    fontWeight: 500,
    color: tokens.color.danger,
  },
};
