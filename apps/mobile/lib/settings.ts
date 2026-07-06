// Persisted connection settings ({ instanceUrl, token }) backed by expo-secure-store.
// The token is a secret and is never logged.
import * as SecureStore from "expo-secure-store";

export type Settings = {
  instanceUrl: string;
  token: string;
};

const INSTANCE_URL_KEY = "openmind.instanceUrl";
const TOKEN_KEY = "openmind.token";

/** Read the stored settings, or null if not fully configured. */
export async function getSettings(): Promise<Settings | null> {
  const [instanceUrl, token] = await Promise.all([
    SecureStore.getItemAsync(INSTANCE_URL_KEY),
    SecureStore.getItemAsync(TOKEN_KEY),
  ]);
  if (!instanceUrl || !token) return null;
  return { instanceUrl, token };
}

/** Persist settings. Trailing slashes on the instance URL are trimmed. */
export async function setSettings(settings: Settings): Promise<void> {
  const instanceUrl = settings.instanceUrl.trim().replace(/\/+$/, "");
  await Promise.all([
    SecureStore.setItemAsync(INSTANCE_URL_KEY, instanceUrl),
    SecureStore.setItemAsync(TOKEN_KEY, settings.token.trim()),
  ]);
}

/** Remove all stored settings (sign out). */
export async function clearSettings(): Promise<void> {
  await Promise.all([
    SecureStore.deleteItemAsync(INSTANCE_URL_KEY),
    SecureStore.deleteItemAsync(TOKEN_KEY),
  ]);
}
