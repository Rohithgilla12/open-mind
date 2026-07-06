// Persisted connection settings ({ instanceUrl, token }) backed by expo-secure-store
// on native. The token is a secret and is never logged.
//
// expo-secure-store has no web implementation, so on web (used only for the
// dev/preview surface) we fall back to localStorage. This keeps `expo export
// --platform web` and the web preview working without a native keychain.
import { Platform } from "react-native";
import * as SecureStore from "expo-secure-store";

export type Settings = {
  instanceUrl: string;
  token: string;
};

const INSTANCE_URL_KEY = "openmind.instanceUrl";
const TOKEN_KEY = "openmind.token";

const isWeb = Platform.OS === "web";

async function getItem(key: string): Promise<string | null> {
  if (isWeb) {
    try {
      return globalThis.localStorage?.getItem(key) ?? null;
    } catch {
      return null;
    }
  }
  return SecureStore.getItemAsync(key);
}

async function setItem(key: string, value: string): Promise<void> {
  if (isWeb) {
    try {
      globalThis.localStorage?.setItem(key, value);
    } catch {
      // ignore — preview surface only
    }
    return;
  }
  await SecureStore.setItemAsync(key, value);
}

async function deleteItem(key: string): Promise<void> {
  if (isWeb) {
    try {
      globalThis.localStorage?.removeItem(key);
    } catch {
      // ignore — preview surface only
    }
    return;
  }
  await SecureStore.deleteItemAsync(key);
}

/** Read the stored settings, or null if not fully configured. */
export async function getSettings(): Promise<Settings | null> {
  const [instanceUrl, token] = await Promise.all([
    getItem(INSTANCE_URL_KEY),
    getItem(TOKEN_KEY),
  ]);
  if (!instanceUrl || !token) return null;
  return { instanceUrl, token };
}

/** Persist settings. Trailing slashes on the instance URL are trimmed. */
export async function setSettings(settings: Settings): Promise<void> {
  const instanceUrl = settings.instanceUrl.trim().replace(/\/+$/, "");
  await Promise.all([
    setItem(INSTANCE_URL_KEY, instanceUrl),
    setItem(TOKEN_KEY, settings.token.trim()),
  ]);
}

/** Remove all stored settings (sign out). */
export async function clearSettings(): Promise<void> {
  await Promise.all([deleteItem(INSTANCE_URL_KEY), deleteItem(TOKEN_KEY)]);
}
