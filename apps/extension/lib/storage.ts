import { browser } from "wxt/browser";

export interface Settings {
  instanceUrl: string;
  token: string;
}

/**
 * The maintainer-run hosted instance. Offered as a one-click option on the
 * options page but never applied automatically: a store-installed extension
 * must not send a user's saves to a server they didn't choose.
 */
export const HOSTED_INSTANCE_URL = "https://openmind.gilla.fun";

// Ship with no instance configured. The popup detects the empty state and
// sends the user to the options page, where they either paste their own
// self-hosted URL or opt into HOSTED_INSTANCE_URL explicitly.
const DEFAULT_SETTINGS: Settings = {
  instanceUrl: "",
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
