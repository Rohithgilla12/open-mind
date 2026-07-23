import { Platform } from "react-native";
import * as SecureStore from "expo-secure-store";
import type { TokenCache } from "@clerk/clerk-expo";

// Default to the hosted Openmind instance's Clerk so a plain build signs in
// out of the box. A publishable key is public by design (it ships in every
// client bundle), so committing it is safe. Self-hosters override it — and the
// instance URL below — with EXPO_PUBLIC_* at build time.
export const clerkPublishableKey =
  process.env.EXPO_PUBLIC_CLERK_PUBLISHABLE_KEY ??
  "pk_live_Y2xlcmsub3Blbm1pbmQuZ2lsbGEuZnVuJA";

export const defaultInstanceUrl =
  process.env.EXPO_PUBLIC_INSTANCE_URL ?? "https://openmind.gilla.fun";

// SecureStore-backed token cache for the Clerk session. Web has no SecureStore,
// so it falls back to in-memory (the web/preview surface doesn't persist auth).
const memory = new Map<string, string>();

export const tokenCache: TokenCache = {
  getToken: async (key) => {
    if (Platform.OS === "web") return memory.get(key) ?? null;
    try {
      return await SecureStore.getItemAsync(key);
    } catch {
      return null;
    }
  },
  saveToken: async (key, value) => {
    if (Platform.OS === "web") {
      memory.set(key, value);
      return;
    }
    try {
      await SecureStore.setItemAsync(key, value);
    } catch {
      // best-effort; a failed cache write just forces re-auth next launch
    }
  },
};
