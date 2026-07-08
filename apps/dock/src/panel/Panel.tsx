import { useEffect, useRef, useState } from "react";
import type { CSSProperties, KeyboardEvent } from "react";
import { listen } from "@tauri-apps/api/event";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { openUrl } from "@tauri-apps/plugin-opener";
import { tokens } from "@openmind/ui";
import { PanelDragStrip } from "../components/DragRegion";
import { IconButton, SettingsIcon } from "../components/SettingsIcon";
import { saveItem, searchItems, type SearchResult } from "../lib/api";
import { detectMode } from "../lib/input-mode";
import { getSettings, type Settings } from "../lib/settings";
import { SettingsView } from "./SettingsView";

type ViewMode = "settings" | "main";

type Toast = { kind: "saved" } | { kind: "error"; message: string } | null;

const SEARCH_DEBOUNCE_MS = 250;

/** Best-effort hostname for display; falls back to the raw string. */
function host(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

export function Panel() {
  const [settings, setSettingsState] = useState<Settings | null | undefined>(undefined);
  const [view, setView] = useState<ViewMode>("main");
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult[]>([]);
  const [understood, setUnderstood] = useState<string | undefined>(undefined);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState<Toast>(null);

  const inputRef = useRef<HTMLInputElement>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const mode = detectMode(query);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const s = await getSettings();
      if (cancelled) return;
      setSettingsState(s);
      setView(s ? "main" : "settings");
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let unlisten: (() => void) | undefined;
    void getCurrentWindow()
      .onFocusChanged(({ payload: focused }) => {
        if (focused) {
          inputRef.current?.focus();
        } else {
          void getCurrentWindow().hide();
        }
      })
      .then((fn) => {
        unlisten = fn;
      });
    return () => unlisten?.();
  }, []);

  useEffect(() => {
    if (view === "main") {
      inputRef.current?.focus();
    }
  }, [view]);

  // Tray menu's "Settings" item shows the panel and emits this to switch view.
  useEffect(() => {
    let unlisten: (() => void) | undefined;
    void listen("open-settings", () => setView("settings")).then((fn) => {
      unlisten = fn;
    });
    return () => unlisten?.();
  }, []);

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    setSelectedIndex(0);
    setSearchError(null);

    if (mode !== "search" || !settings) {
      setResults([]);
      setUnderstood(undefined);
      setSearching(false);
      return;
    }
    const trimmed = query.trim();
    if (!trimmed) {
      setResults([]);
      setUnderstood(undefined);
      setSearching(false);
      return;
    }

    setSearching(true);
    debounceRef.current = setTimeout(() => {
      void (async () => {
        const res = await searchItems(trimmed, settings);
        setSearching(false);
        if (res.ok) {
          setResults(res.results);
          setUnderstood(res.understood);
        } else if (res.status === 401) {
          setResults([]);
          setSearchError("Token rejected — open Settings");
        } else if (res.status === 0) {
          setResults([]);
          setSearchError("Instance unreachable");
        } else {
          setResults([]);
          setSearchError(`Search failed (${res.status})`);
        }
      })();
    }, SEARCH_DEBOUNCE_MS);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, mode, settings]);

  useEffect(() => {
    return () => {
      if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
    };
  }, []);

  function showSavedToast() {
    setToast({ kind: "saved" });
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
    toastTimerRef.current = setTimeout(() => {
      setToast(null);
      setQuery("");
    }, 1000);
  }

  function showErrorToast(message: string) {
    setToast({ kind: "error", message });
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
    toastTimerRef.current = setTimeout(() => setToast(null), 2000);
  }

  async function performSave(body: { url?: string; note?: string }) {
    if (!settings || saving) return;
    setSaving(true);
    const res = await saveItem(body, settings);
    setSaving(false);
    if (res.ok) {
      showSavedToast();
      return;
    }
    if (res.status === 401) {
      showErrorToast("Token rejected — open Settings");
    } else if (res.status === 0) {
      showErrorToast("Instance unreachable");
    } else {
      showErrorToast(`Save failed (${res.status})`);
    }
  }

  async function saveRawInput() {
    const trimmed = query.trim();
    if (!trimmed) return;
    if (detectMode(trimmed) === "save-url") {
      await performSave({ url: trimmed });
    } else {
      await performSave({ note: trimmed });
    }
  }

  async function openSelected() {
    const result = results[selectedIndex];
    if (!result || !settings) return;
    await openUrl(`${settings.instanceUrl}/item/${result.item.id}`);
    await getCurrentWindow().hide();
  }

  function moveSelection(delta: number) {
    setSelectedIndex((i) => {
      if (results.length === 0) return 0;
      const next = i + delta;
      if (next < 0) return results.length - 1;
      if (next >= results.length) return 0;
      return next;
    });
  }

  function onInputKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Escape") {
      e.preventDefault();
      void getCurrentWindow().hide();
      return;
    }
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      void saveRawInput();
      return;
    }
    if (mode === "search") {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        moveSelection(1);
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        moveSelection(-1);
      } else if (e.key === "Enter") {
        e.preventDefault();
        void openSelected();
      }
    } else if (mode === "save-url" && e.key === "Enter") {
      e.preventDefault();
      void saveRawInput();
    }
  }

  if (settings === undefined) {
    return <div style={styles.shell} />;
  }

  if (view === "settings") {
    return (
      <div style={styles.shell}>
        <PanelDragStrip />
        <SettingsView
          initial={settings}
          onCancel={settings ? () => setView("main") : undefined}
          onSignedOut={() => {
            setSettingsState(null);
            setView("settings");
          }}
          onSaved={(s) => {
            setSettingsState(s);
            setView("main");
          }}
        />
      </div>
    );
  }

  return (
    <div style={styles.shell}>
      <PanelDragStrip />
      <div style={styles.inputRow}>
        <input
          ref={inputRef}
          style={styles.input}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onInputKeyDown}
          placeholder="Search your mind, or paste a link…"
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
        />
        <IconButton label="Open settings" onClick={() => setView("settings")}>
          <SettingsIcon />
        </IconButton>
      </div>

      {understood && understood !== query.trim() && mode === "search" ? (
        <div style={styles.understood}>Understood as “{understood}”</div>
      ) : null}

      <div style={styles.body}>
        {toast?.kind === "saved" ? (
          <div style={styles.toastRow}>Saved ✓</div>
        ) : toast?.kind === "error" ? (
          <div style={styles.errorRow}>{toast.message}</div>
        ) : mode === "save-url" ? (
          <button
            type="button"
            style={styles.actionRow}
            onClick={() => void saveRawInput()}
            disabled={saving}
          >
            {saving ? "Saving…" : `Save ${host(query.trim())}`}
          </button>
        ) : searchError ? (
          <div style={styles.errorBlock}>
            <div style={styles.errorRow}>{searchError}</div>
            {searchError.includes("Settings") || searchError === "Instance unreachable" ? (
              <button type="button" style={styles.errorAction} onClick={() => setView("settings")}>
                Open Settings
              </button>
            ) : null}
          </div>
        ) : query.trim().length === 0 ? (
          <div style={styles.emptyRow}>Type to search your mind</div>
        ) : searching ? (
          <div style={styles.emptyRow}>Searching…</div>
        ) : results.length === 0 ? (
          <div style={styles.emptyRow}>Nothing found</div>
        ) : (
          <ul style={styles.list}>
            {results.map((r, i) => (
              <li key={r.item.id}>
                <button
                  type="button"
                  style={{
                    ...styles.rowButton,
                    ...(i === selectedIndex ? styles.rowButtonSelected : {}),
                  }}
                  onMouseEnter={() => setSelectedIndex(i)}
                  onClick={() => void openSelected()}
                >
                  <span style={styles.rowTitle}>{r.item.title || host(r.item.url)}</span>
                  <span style={styles.rowCaption}>
                    {[host(r.item.url), r.item.cardType, (r.item.tags ?? r.item.userTags)?.join(", ")]
                      .filter(Boolean)
                      .join(" · ")}
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
  shell: {
    width: 640,
    height: 420,
    boxSizing: "border-box",
    display: "flex",
    flexDirection: "column",
    background: tokens.color.paper,
    border: `1px solid ${tokens.color.hairline}`,
    borderRadius: 16,
    overflow: "hidden",
    fontFamily: tokens.font.sans,
    color: tokens.color.ink,
    boxShadow: "0 18px 48px rgba(28, 26, 22, 0.18), 0 2px 8px rgba(28, 26, 22, 0.08)",
  },
  inputRow: {
    display: "flex",
    alignItems: "center",
    gap: 10,
    padding: "12px 16px",
    borderBottom: `1px solid ${tokens.color.hairline}`,
    background: tokens.color.paper,
  },
  input: {
    flex: 1,
    border: "none",
    outline: "none",
    background: "transparent",
    fontSize: 18,
    fontFamily: tokens.font.sans,
    color: tokens.color.ink,
  },
  understood: {
    fontFamily: tokens.font.mono,
    fontSize: 11,
    color: tokens.color.inkFaint,
    padding: "6px 18px 0",
  },
  body: {
    flex: 1,
    overflowY: "auto",
    padding: "8px 10px",
  },
  emptyRow: {
    padding: "28px 12px",
    textAlign: "center",
    fontSize: 14,
    color: tokens.color.inkFaint,
    fontFamily: tokens.font.mono,
  },
  errorBlock: {
    padding: "24px 12px",
    textAlign: "center",
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    gap: 12,
  },
  errorRow: {
    fontSize: 14,
    color: tokens.color.danger,
    fontFamily: tokens.font.quote,
    fontStyle: "italic",
  },
  errorAction: {
    border: `1px solid ${tokens.color.hairline}`,
    borderRadius: 999,
    background: tokens.color.cardSurface,
    color: tokens.color.cobalt,
    fontSize: 13,
    fontWeight: 600,
    padding: "8px 14px",
    cursor: "pointer",
  },
  toastRow: {
    padding: "24px 8px",
    textAlign: "center",
    fontSize: 14,
    fontWeight: 600,
    color: tokens.color.green,
  },
  actionRow: {
    width: "100%",
    textAlign: "left",
    border: "none",
    borderRadius: 8,
    background: tokens.color.cardSurface,
    color: tokens.color.cobalt,
    fontSize: 14,
    fontWeight: 600,
    padding: "12px 14px",
    cursor: "pointer",
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
    width: "100%",
    display: "flex",
    flexDirection: "column",
    alignItems: "flex-start",
    gap: 2,
    textAlign: "left",
    border: "none",
    background: "none",
    borderRadius: 8,
    padding: "8px 10px",
    cursor: "pointer",
  },
  rowButtonSelected: {
    background: tokens.color.panel,
  },
  rowTitle: {
    fontFamily: tokens.font.quote,
    fontSize: 15.5,
    fontWeight: 500,
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
};
