// Thin wrappers over the Rust `settings_*` Tauri commands, which persist
// { instance_url, token } in the macOS Keychain via the `keyring` crate. The
// token is a secret and is never logged.
import { invoke } from "@tauri-apps/api/core";

export type Settings = {
  instanceUrl: string;
  token: string;
};

/** Hosted Openmind instance — pre-filled so the dock connects out of the box.
 *  Self-hosters just replace it with their own URL in Settings. */
export const DEFAULT_INSTANCE_URL = "https://openmind.gilla.fun";

type RustSettings = {
  instance_url: string;
  token: string;
};

/** Read the stored settings, or null if not fully configured. Throws on keychain errors. */
export async function getSettings(): Promise<Settings | null> {
  const raw = await invoke<RustSettings | null>("settings_get");
  if (!raw) return null;
  return { instanceUrl: raw.instance_url, token: raw.token };
}

/** Persist settings. Trailing slashes on the instance URL are trimmed by the Rust side. */
export async function setSettings(settings: Settings): Promise<void> {
  await invoke("settings_set", {
    instanceUrl: settings.instanceUrl,
    token: settings.token,
  });
}

/** Remove all stored settings (sign out). */
export async function clearSettings(): Promise<void> {
  await invoke("settings_clear");
}
