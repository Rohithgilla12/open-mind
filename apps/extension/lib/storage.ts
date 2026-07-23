import { browser } from "wxt/browser";

export interface Settings {
  instanceUrl: string;
  token: string;
}

// Pre-fill the hosted Openmind instance so the extension points at it out of
// the box — the user only needs to add a token. Self-hosters change the URL in
// the extension's options page.
const DEFAULT_SETTINGS: Settings = {
  instanceUrl: "https://openmind.gilla.fun",
  token: "",
};

const STORAGE_KEY = "settings";

export async function getSettings(): Promise<Settings> {
  const stored = await browser.storage.local.get(STORAGE_KEY);
  const value = stored[STORAGE_KEY] as Partial<Settings> | undefined;
  return {
    instanceUrl: value?.instanceUrl?.trim() || DEFAULT_SETTINGS.instanceUrl,
    token: value?.token ?? DEFAULT_SETTINGS.token,
  };
}

export async function setSettings(settings: Settings): Promise<void> {
  await browser.storage.local.set({ [STORAGE_KEY]: settings });
}
