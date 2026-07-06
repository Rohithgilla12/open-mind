import { useShareIntent } from "expo-share-intent";
import { Stack, useRouter } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { useEffect } from "react";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { SettingsProvider } from "@/lib/settings-context";
import { colors } from "@/lib/theme";

// Watches for a shared URL/text (iOS share extension / Android SEND intent) and,
// when one arrives, routes to the Capture tab pre-filled with it. On web the
// native module is absent, so `useShareIntent` no-ops (disabled by default) —
// keeping the web export buildable.
function ShareIntentGate() {
  const router = useRouter();
  const { hasShareIntent, shareIntent, resetShareIntent } = useShareIntent({
    resetOnBackground: true,
  });

  useEffect(() => {
    if (!hasShareIntent) return;
    const shared = shareIntent.webUrl ?? shareIntent.text ?? "";
    if (shared) {
      router.navigate({ pathname: "/capture", params: { shared } });
    }
    resetShareIntent();
    // Only react to a newly-received intent; resetShareIntent clears it after.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasShareIntent]);

  return null;
}

export default function RootLayout() {
  return (
    <SafeAreaProvider>
      <SettingsProvider>
        <StatusBar style="dark" />
        <ShareIntentGate />
        <Stack
          screenOptions={{
            headerShown: false,
            contentStyle: { backgroundColor: colors.canvas },
          }}
        >
          <Stack.Screen name="(tabs)" />
        </Stack>
      </SettingsProvider>
    </SafeAreaProvider>
  );
}
