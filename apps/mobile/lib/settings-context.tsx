// Shares the current connection settings across screens and exposes reload /
// sign-out helpers. Screens read `configured` to gate content and the
// unconfigured guard uses it to redirect to Settings.
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import {
  clearSettings,
  getSettings,
  setSettings as persistSettings,
  type Settings,
} from "./settings";

type SettingsContextValue = {
  settings: Settings | null;
  loading: boolean;
  configured: boolean;
  reload: () => Promise<void>;
  save: (settings: Settings) => Promise<void>;
  signOut: () => Promise<void>;
};

const SettingsContext = createContext<SettingsContextValue | null>(null);

export function SettingsProvider({ children }: { children: React.ReactNode }) {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [loading, setLoading] = useState(true);

  const reload = useCallback(async () => {
    try {
      const next = await getSettings();
      setSettings(next);
    } finally {
      // Always resolve loading so the routing guard can act — otherwise a read
      // failure would leave the app stuck on the loading state indefinitely.
      setLoading(false);
    }
  }, []);

  const save = useCallback(async (next: Settings) => {
    await persistSettings(next);
    await reload();
  }, [reload]);

  const signOut = useCallback(async () => {
    await clearSettings();
    setSettings(null);
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const value = useMemo<SettingsContextValue>(
    () => ({ settings, loading, configured: settings !== null, reload, save, signOut }),
    [settings, loading, reload, save, signOut],
  );

  return <SettingsContext.Provider value={value}>{children}</SettingsContext.Provider>;
}

export function useSettingsContext(): SettingsContextValue {
  const ctx = useContext(SettingsContext);
  if (!ctx) throw new Error("useSettingsContext must be used within a SettingsProvider");
  return ctx;
}
