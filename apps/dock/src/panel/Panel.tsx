import { Component, useEffect, useRef, useState } from "react";
import type { CSSProperties, ErrorInfo, KeyboardEvent, ReactNode } from "react";
import { listen } from "@tauri-apps/api/event";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { openUrl } from "@tauri-apps/plugin-opener";
import { tokens } from "@openmind/ui";
import { PanelDragStrip } from "../components/DragRegion";
import { IconButton, SettingsIcon } from "../components/SettingsIcon";
import { saveItem, searchItems, listDesk, listRecent, type Item, type SearchResult } from "../lib/api";
import { mergeHomeLists } from "../lib/home-lists";
import { detectMode } from "../lib/input-mode";
import { getSettings, type Settings } from "../lib/settings";
import { SettingsView } from "./SettingsView";

type ViewMode = "settings" | "main";

type Toast = { kind: "saved" } | { kind: "error"; message: string } | null;

const SEARCH_DEBOUNCE_MS = 250;
const HOME_RECENT_FETCH = 16; // fetch extra so merge can still fill 8 after desk dedupe

/** Best-effort hostname for display; falls back to the raw string. */
function host(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

function statusMessage(status: number): string {
  if (status === 401) return "Token rejected — open Settings";
  if (status === 0) return "Instance unreachable";
  return `Request failed (${status})`;
}

function ItemRow({
  item,
  selected,
  onSelect,
  onOpen,
}: {
  item: Item;
  selected: boolean;
  onSelect: () => void;
  onOpen: () => void;
}) {
  return (
    <button
      type="button"
      style={{
        ...styles.rowButton,
        ...(selected ? styles.rowButtonSelected : {}),
      }}
      onMouseEnter={onSelect}
      onClick={onOpen}
    >
      <span style={styles.rowTitle}>{item.title || host(item.url)}</span>
      <span style={styles.rowCaption}>
        {[host(item.url), item.cardType, (item.tags ?? item.userTags)?.join(", ")]
          .filter(Boolean)
          .join(" · ")}
      </span>
    </button>
  );
}

/** Keeps a render crash from blanking the whole panel until quit. */
export class PanelErrorBoundary extends Component<
  { children: ReactNode },
  { error: Error | null }
> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("dock panel crashed", error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <div style={styles.shell}>
          <PanelDragStrip />
          <div style={styles.errorBlock}>
            <div style={styles.errorRow}>Something went wrong in the panel.</div>
            <button
              type="button"
              style={styles.errorAction}
              onClick={() => this.setState({ error: null })}
            >
              Try again
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
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
  const [bootError, setBootError] = useState<string | null>(null);
  const [homeDesk, setHomeDesk] = useState<Item[]>([]);
  const [homeRecent, setHomeRecent] = useState<Item[]>([]);
  const [homeLoading, setHomeLoading] = useState(false);
  const [homeError, setHomeError] = useState<string | null>(null);
  const [homeEpoch, setHomeEpoch] = useState(0);

  const inputRef = useRef<HTMLInputElement>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const searchAbortRef = useRef<AbortController | null>(null);
  const homeAbortRef = useRef<AbortController | null>(null);

  const mode = detectMode(query);
  const queryEmpty = query.trim().length === 0;
  const showHome = mode === "search" && queryEmpty && !!settings;
  const homeItems = [...homeDesk, ...homeRecent];
  const navigable =
    showHome && !toast
      ? homeItems.map((item) => ({ kind: "item" as const, item }))
      : results.map((r) => ({ kind: "result" as const, item: r.item, result: r }));

  function bumpHome() {
    setHomeEpoch((n) => n + 1);
  }

  async function loadSettings() {
    setBootError(null);
    try {
      const s = await getSettings();
      setSettingsState(s);
      setView(s ? "main" : "settings");
    } catch {
      setSettingsState(null);
      setView("settings");
      setBootError("Couldn't read settings from the keychain — try again.");
    }
  }

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const s = await getSettings();
        if (cancelled) return;
        setSettingsState(s);
        setView(s ? "main" : "settings");
      } catch {
        if (cancelled) return;
        setSettingsState(null);
        setView("settings");
        setBootError("Couldn't read settings from the keychain — try again.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let unlisten: (() => void) | undefined;
    void getCurrentWindow()
      .onFocusChanged(({ payload: focused }) => {
        // The panel floats: losing focus no longer hides it (Esc / ⌘⇧O / tray
        // do), so it can sit beside whatever you're reading.
        if (focused) {
          inputRef.current?.focus();
          bumpHome();
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
      bumpHome();
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

  // Home: Desk + Recent when the query is empty.
  useEffect(() => {
    if (!showHome || view !== "main") return;

    homeAbortRef.current?.abort();
    const controller = new AbortController();
    homeAbortRef.current = controller;
    setHomeLoading(true);
    setHomeError(null);
    setSelectedIndex(0);

    void (async () => {
      try {
        const [deskRes, recentRes] = await Promise.all([
          listDesk(settings ?? undefined, controller.signal),
          listRecent(HOME_RECENT_FETCH, settings ?? undefined, controller.signal),
        ]);
        if (controller.signal.aborted) return;

        const merged = mergeHomeLists(
          deskRes.ok ? deskRes.items : [],
          recentRes.ok ? recentRes.items : [],
        );
        setHomeDesk(merged.desk);
        setHomeRecent(merged.recent);
        setHomeLoading(false);

        if (!deskRes.ok && !recentRes.ok) {
          setHomeError(statusMessage(deskRes.status || recentRes.status));
        } else {
          setHomeError(null);
        }
      } catch {
        if (controller.signal.aborted) return;
        setHomeLoading(false);
        setHomeDesk([]);
        setHomeRecent([]);
        setHomeError("Instance unreachable");
      }
    })();

    return () => {
      controller.abort();
    };
  }, [showHome, view, settings, homeEpoch]);

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    searchAbortRef.current?.abort();
    searchAbortRef.current = null;
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
      const controller = new AbortController();
      searchAbortRef.current = controller;
      void (async () => {
        try {
          const res = await searchItems(trimmed, settings, controller.signal);
          if (controller.signal.aborted) return;
          setSearching(false);
          if (res.ok) {
            setResults(res.results);
            setUnderstood(res.understood);
          } else if (res.status === 401) {
            setResults([]);
            setUnderstood(undefined);
            setSearchError("Token rejected — open Settings");
          } else if (res.status === 0) {
            setResults([]);
            setUnderstood(undefined);
            setSearchError("Instance unreachable");
          } else {
            setResults([]);
            setUnderstood(undefined);
            setSearchError(`Search failed (${res.status})`);
          }
        } catch {
          if (controller.signal.aborted) return;
          setSearching(false);
          setResults([]);
          setUnderstood(undefined);
          setSearchError("Instance unreachable");
        }
      })();
    }, SEARCH_DEBOUNCE_MS);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      searchAbortRef.current?.abort();
      searchAbortRef.current = null;
    };
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
    try {
      const res = await saveItem(body, settings);
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
    } finally {
      setSaving(false);
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

  async function openItem(item: Item) {
    if (!settings) return;
    const url = `${settings.instanceUrl}/item/${item.id}`;
    try {
      await openUrl(url);
    } catch {
      showErrorToast("Couldn't open in browser");
      return;
    }
    try {
      await getCurrentWindow().hide();
    } catch {
      // Panel may already be hidden; ignore.
    }
  }

  function moveSelection(delta: number) {
    setSelectedIndex((i) => {
      if (navigable.length === 0) return 0;
      const next = i + delta;
      if (next < 0) return navigable.length - 1;
      if (next >= navigable.length) return 0;
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
        const entry = navigable[selectedIndex];
        if (entry) void openItem(entry.item);
      }
    } else if (mode === "save-url" && e.key === "Enter") {
      e.preventDefault();
      void saveRawInput();
    }
  }

  if (settings === undefined) {
    return (
      <div style={styles.shell}>
        <PanelDragStrip />
        <div style={styles.emptyRow}>Loading…</div>
      </div>
    );
  }

  if (view === "settings") {
    return (
      <div style={styles.shell}>
        <PanelDragStrip />
        {bootError ? (
          <div style={styles.bootBanner}>
            <span>{bootError}</span>
            <button type="button" style={styles.errorAction} onClick={() => void loadSettings()}>
              Retry
            </button>
          </div>
        ) : null}
        <SettingsView
          initial={settings}
          onCancel={settings ? () => setView("main") : undefined}
          onSignedOut={() => {
            setSettingsState(null);
            setView("settings");
          }}
          onSaved={(s) => {
            setSettingsState(s);
            setBootError(null);
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
        ) : showHome ? (
          homeError && homeItems.length === 0 ? (
            <div style={styles.errorBlock}>
              <div style={styles.errorRow}>{homeError}</div>
              {homeError.includes("Settings") || homeError === "Instance unreachable" ? (
                <button type="button" style={styles.errorAction} onClick={() => setView("settings")}>
                  Open Settings
                </button>
              ) : null}
            </div>
          ) : homeLoading && homeItems.length === 0 ? (
            <div style={styles.emptyRow}>Loading…</div>
          ) : homeItems.length === 0 ? (
            <div style={styles.emptyRow}>Type to search · ⌘⇧S saves the front tab</div>
          ) : (
            <div style={styles.homeStack}>
              {homeDesk.length > 0 ? (
                <section>
                  <h2 style={styles.sectionHeading}>Desk</h2>
                  <ul style={styles.list}>
                    {homeDesk.map((item, i) => (
                      <li key={item.id}>
                        <ItemRow
                          item={item}
                          selected={selectedIndex === i}
                          onSelect={() => setSelectedIndex(i)}
                          onOpen={() => void openItem(item)}
                        />
                      </li>
                    ))}
                  </ul>
                </section>
              ) : null}
              {homeRecent.length > 0 ? (
                <section>
                  <h2 style={styles.sectionHeading}>Recent</h2>
                  <ul style={styles.list}>
                    {homeRecent.map((item, i) => {
                      const index = homeDesk.length + i;
                      return (
                        <li key={item.id}>
                          <ItemRow
                            item={item}
                            selected={selectedIndex === index}
                            onSelect={() => setSelectedIndex(index)}
                            onOpen={() => void openItem(item)}
                          />
                        </li>
                      );
                    })}
                  </ul>
                </section>
              ) : null}
            </div>
          )
        ) : searchError ? (
          <div style={styles.errorBlock}>
            <div style={styles.errorRow}>{searchError}</div>
            {searchError.includes("Settings") || searchError === "Instance unreachable" ? (
              <button type="button" style={styles.errorAction} onClick={() => setView("settings")}>
                Open Settings
              </button>
            ) : null}
          </div>
        ) : searching ? (
          <div style={styles.emptyRow}>Searching…</div>
        ) : results.length === 0 ? (
          <div style={styles.emptyRow}>Nothing found · ⌘Enter saves as a note</div>
        ) : (
          <ul style={styles.list}>
            {results.map((r, i) => (
              <li key={r.item.id}>
                <ItemRow
                  item={r.item}
                  selected={i === selectedIndex}
                  onSelect={() => setSelectedIndex(i)}
                  onOpen={() => void openItem(r.item)}
                />
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
  homeStack: {
    display: "flex",
    flexDirection: "column",
    gap: 14,
  },
  sectionHeading: {
    fontFamily: tokens.font.mono,
    fontSize: 10,
    letterSpacing: "0.08em",
    textTransform: "uppercase",
    color: tokens.color.inkFaint,
    margin: "4px 10px 6px",
    fontWeight: 500,
  },
  emptyRow: {
    padding: "28px 12px",
    textAlign: "center",
    fontSize: 14,
    color: tokens.color.inkFaint,
    fontFamily: tokens.font.mono,
  },
  bootBanner: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
    padding: "10px 16px",
    background: tokens.color.noteSurface,
    borderBottom: `1px solid ${tokens.color.hairline}`,
    fontSize: 13,
    color: tokens.color.inkMuted,
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
