import { useEffect, useState } from "react";
import type { CSSProperties, KeyboardEvent } from "react";
import { browser } from "wxt/browser";
import { tokens } from "@openmind/ui";
import { getSettings } from "../../lib/storage";
import { patchUserTags, recentItems, saveItem } from "../../lib/save";
import type { Item } from "../../lib/save";

type SaveState = "idle" | "saving" | "saved" | "error";

interface ActiveTab {
  title: string;
  url: string;
}

/** Best-effort hostname for display; falls back to the raw string. */
function host(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

export function Popup() {
  const [ready, setReady] = useState(false);
  const [configured, setConfigured] = useState(false);
  const [instanceUrl, setInstanceUrl] = useState("");
  const [tab, setTab] = useState<ActiveTab | null>(null);

  const [state, setState] = useState<SaveState>("idle");
  const [errorText, setErrorText] = useState("");
  const [savedItem, setSavedItem] = useState<Item | null>(null);

  const [tags, setTags] = useState<string[]>([]);
  const [tagInput, setTagInput] = useState("");
  const [tagError, setTagError] = useState("");

  const [recent, setRecent] = useState<Item[]>([]);
  const [recentLoading, setRecentLoading] = useState(true);
  const [recentStatus, setRecentStatus] = useState<number | null>(null);

  async function loadRecent() {
    setRecentLoading(true);
    const res = await recentItems(5);
    setRecent(res.items);
    setRecentStatus(res.status);
    setRecentLoading(false);
  }

  useEffect(() => {
    void (async () => {
      const settings = await getSettings();
      const ok =
        settings.token.trim().length > 0 &&
        settings.instanceUrl.trim().length > 0;
      setConfigured(ok);
      setInstanceUrl(settings.instanceUrl.replace(/\/+$/, ""));
      setReady(true);

      if (!ok) {
        setRecentLoading(false);
        return;
      }

      const [active] = await browser.tabs.query({
        active: true,
        currentWindow: true,
      });
      if (active?.url) {
        setTab({ title: active.title ?? active.url, url: active.url });
      }

      await loadRecent();
    })();
  }, []);

  function openSettings() {
    void browser.runtime.openOptionsPage();
  }

  async function handleSave() {
    if (!tab) return;
    setState("saving");
    setErrorText("");
    const res = await saveItem({ url: tab.url });
    if (res.ok && res.item) {
      setSavedItem(res.item);
      setTags(res.item.userTags ?? []);
      setState("saved");
      void loadRecent();
    } else if (res.ok) {
      setState("saved");
      void loadRecent();
    } else if (res.status === 401) {
      setState("error");
      setErrorText("Token rejected — open options.");
    } else if (res.status === 0) {
      setState("error");
      setErrorText("Instance unreachable.");
    } else {
      setState("error");
      setErrorText(`Save failed (error ${res.status}).`);
    }
  }

  async function applyTags(next: string[]) {
    if (!savedItem) return;
    const previous = tags;
    setTags(next);
    setTagError("");
    const res = await patchUserTags(savedItem.id, next);
    if (!res.ok) {
      setTags(previous);
      setTagError("Couldn't update tags.");
    }
  }

  function addTag() {
    const value = tagInput.trim();
    if (!value || tags.includes(value)) {
      setTagInput("");
      return;
    }
    setTagInput("");
    void applyTags([...tags, value]);
  }

  function removeTag(tag: string) {
    void applyTags(tags.filter((t) => t !== tag));
  }

  function onTagKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      e.preventDefault();
      addTag();
    }
  }

  function openItem(id: string) {
    void browser.tabs.create({ url: `${instanceUrl}/item/${id}` });
  }

  if (!ready) {
    return (
      <div style={styles.page}>
        <p style={styles.muted}>Loading…</p>
      </div>
    );
  }

  if (!configured) {
    return (
      <div style={styles.page}>
        <h1 style={styles.heading}>Set up Openmind</h1>
        <p style={styles.muted}>
          Add your instance URL and access token to start saving.
        </p>
        <button type="button" style={styles.primaryButton} onClick={openSettings}>
          Open options
        </button>
      </div>
    );
  }

  return (
    <div style={styles.page}>
      <h1 style={styles.heading}>Save to Openmind</h1>

      {state === "saved" ? (
        <div style={styles.savedCard}>
          <div style={styles.savedLabel}>Saved</div>
          <div style={styles.tabTitle}>
            {savedItem?.title ?? tab?.title ?? "Untitled"}
          </div>

          {savedItem && (
            <>
              <div style={styles.chipRow}>
                {tags.map((tag) => (
                  <span key={tag} style={styles.chip}>
                    {tag}
                    <button
                      type="button"
                      style={styles.chipRemove}
                      onClick={() => removeTag(tag)}
                      aria-label={`Remove ${tag}`}
                    >
                      ×
                    </button>
                  </span>
                ))}
              </div>
              <input
                type="text"
                style={styles.tagInput}
                placeholder="Add a tag…"
                value={tagInput}
                onChange={(e) => setTagInput(e.target.value)}
                onKeyDown={onTagKeyDown}
                onBlur={addTag}
              />
              {tagError && <p style={styles.error}>{tagError}</p>}
            </>
          )}
        </div>
      ) : (
        <>
          {tab ? (
            <div style={styles.tabCard}>
              <div style={styles.tabTitle}>{tab.title}</div>
              <div style={styles.tabUrl}>{tab.url}</div>
            </div>
          ) : (
            <p style={styles.muted}>No active tab to save.</p>
          )}

          <button
            type="button"
            style={{
              ...styles.primaryButton,
              ...(tab && state !== "saving" ? {} : styles.disabledButton),
            }}
            onClick={handleSave}
            disabled={!tab || state === "saving"}
          >
            {state === "saving" ? "Saving…" : "Save page"}
          </button>
          {state === "error" && (
            <p style={styles.error}>
              {errorText}
              {errorText.includes("options") && (
                <>
                  {" "}
                  <button
                    type="button"
                    style={styles.linkButton}
                    onClick={openSettings}
                  >
                    Open options
                  </button>
                </>
              )}
            </p>
          )}
        </>
      )}

      <div style={styles.section}>
        <div style={styles.sectionTitle}>Recently saved</div>
        {recentLoading ? (
          <p style={styles.muted}>Loading…</p>
        ) : recent.length === 0 && recentStatus === 401 ? (
          <p style={styles.muted}>Token rejected — check options</p>
        ) : recent.length === 0 ? (
          <p style={styles.muted}>Nothing saved yet</p>
        ) : (
          <ul style={styles.list}>
            {recent.map((item) => (
              <li key={item.id}>
                <button
                  type="button"
                  style={styles.rowButton}
                  onClick={() => openItem(item.id)}
                >
                  <span style={styles.rowTitle}>
                    {item.title || host(item.url)}
                  </span>
                  <span
                    style={{
                      ...styles.rowCaption,
                      ...(item.status === "pending"
                        ? styles.rowCaptionPending
                        : {}),
                    }}
                  >
                    {item.status === "pending" ? "enriching…" : host(item.url)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

const styles: Record<string, CSSProperties> = {
  page: {
    width: 360,
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
  savedCard: {
    background: tokens.color.surface,
    border: `1px solid ${tokens.color.line}`,
    borderRadius: 8,
    padding: 12,
    marginBottom: 12,
  },
  savedLabel: {
    fontFamily: tokens.font.mono,
    fontSize: 11,
    textTransform: "uppercase",
    letterSpacing: "0.05em",
    color: tokens.color.cobalt,
    marginBottom: 6,
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
    color: tokens.color.inkFaint,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  chipRow: {
    display: "flex",
    flexWrap: "wrap",
    gap: 6,
    margin: "8px 0",
  },
  chip: {
    display: "inline-flex",
    alignItems: "center",
    gap: 4,
    fontSize: 12,
    fontWeight: 500,
    padding: "3px 8px",
    borderRadius: 999,
    background: `color-mix(in srgb, ${tokens.color.cobalt} 9%, transparent)`,
    color: tokens.color.cobalt,
  },
  chipRemove: {
    border: "none",
    background: "none",
    padding: 0,
    margin: 0,
    lineHeight: 1,
    fontSize: 14,
    cursor: "pointer",
    color: tokens.color.cobalt,
  },
  tagInput: {
    width: "100%",
    boxSizing: "border-box",
    fontFamily: tokens.font.sans,
    fontSize: 13,
    padding: "6px 8px",
    border: `1px solid ${tokens.color.line}`,
    borderRadius: 6,
    background: tokens.color.paper,
    color: tokens.color.ink,
  },
  muted: {
    fontSize: 13,
    color: tokens.color.inkMuted,
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
  linkButton: {
    border: "none",
    background: "none",
    padding: 0,
    fontSize: 13,
    fontWeight: 500,
    color: tokens.color.cobalt,
    cursor: "pointer",
    textDecoration: "underline",
  },
  error: {
    marginTop: 10,
    marginBottom: 0,
    fontSize: 13,
    fontWeight: 500,
    color: tokens.color.danger,
  },
  section: {
    marginTop: 16,
    paddingTop: 12,
    borderTop: `1px solid ${tokens.color.line}`,
  },
  sectionTitle: {
    fontFamily: tokens.font.mono,
    fontSize: 11,
    textTransform: "uppercase",
    letterSpacing: "0.05em",
    color: tokens.color.inkMuted,
    marginBottom: 8,
  },
  list: {
    listStyle: "none",
    margin: 0,
    padding: 0,
    display: "flex",
    flexDirection: "column",
    gap: 2,
  },
  rowButton: {
    display: "flex",
    flexDirection: "column",
    alignItems: "flex-start",
    gap: 2,
    width: "100%",
    textAlign: "left",
    border: "none",
    background: "none",
    padding: "6px 4px",
    borderRadius: 6,
    cursor: "pointer",
  },
  rowTitle: {
    fontSize: 13,
    color: tokens.color.ink,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    maxWidth: "100%",
  },
  rowCaption: {
    fontFamily: tokens.font.mono,
    fontSize: 11,
    color: tokens.color.inkFaint,
  },
  rowCaptionPending: {
    color: tokens.color.cobalt,
  },
};
