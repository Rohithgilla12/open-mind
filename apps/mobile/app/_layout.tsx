import {
  InstrumentSans_400Regular,
  InstrumentSans_500Medium,
  InstrumentSans_600SemiBold,
} from "@expo-google-fonts/instrument-sans";
import { JetBrainsMono_400Regular, JetBrainsMono_500Medium } from "@expo-google-fonts/jetbrains-mono";
import { Newsreader_500Medium_Italic, Newsreader_600SemiBold_Italic } from "@expo-google-fonts/newsreader";
import { ClerkProvider } from "@clerk/clerk-expo";
import { useFonts } from "expo-font";
import * as Notifications from "expo-notifications";
import { useShareIntent } from "expo-share-intent";
import { fallbackFilename, isUploadableMimeType, uploadMimeType } from "../lib/uploads";
import { Stack, useRouter, type Href } from "expo-router";
import * as SplashScreen from "expo-splash-screen";
import { StatusBar } from "expo-status-bar";
import { useEffect } from "react";
import { View } from "react-native";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { CaptureQueueProvider } from "@/lib/capture-queue-context";
import { MorphProvider } from "@/lib/morph";
import { clerkPublishableKey, tokenCache } from "@/lib/clerk";
import { routeForNotificationData } from "@/lib/notifications";
import { QueryProvider } from "@/lib/query";
import { SettingsProvider } from "@/lib/settings-context";
import { colors } from "@/lib/theme";

// Routes a tapped push notification (cold-start or foreground/background tap)
// to the screen its data payload identifies. Registered once at the root so
// it applies regardless of which tab is active when the tap arrives.
function NotificationRouterGate() {
  const router = useRouter();

  useEffect(() => {
    // The app may have been launched by a notification tap rather than a
    // normal open — getLastNotificationResponseAsync surfaces that response
    // exactly once so a cold start still routes correctly.
    void Notifications.getLastNotificationResponseAsync().then((response) => {
      if (!response) return;
      const path = routeForNotificationData(response.notification.request.content.data);
      // Cast: routeForNotificationData resolves a runtime data payload to one
      // of a small fixed set of real routes (verified in notifications.ts),
      // but that mapping happens outside expo-router's typed-route analysis,
      // so the literal-union type can't be inferred here.
      if (path) router.push(path as Href);
    });

    const subscription = Notifications.addNotificationResponseReceivedListener((response) => {
      const path = routeForNotificationData(response.notification.request.content.data);
      if (path) router.push(path as Href);
    });
    return () => subscription.remove();
  }, [router]);

  return null;
}

// Held open until the brand fonts finish loading below.
void SplashScreen.preventAutoHideAsync();

// Watches for a shared URL/text/image (Android SEND intent) and, when one
// arrives, routes to the Capture tab. Text/URL is pre-filled; images are
// handed off as JSON and uploaded immediately. iOS shares are handled by the
// native share extension in targets/share (inline save, never opens the app),
// so expo-share-intent is Android-only; on iOS/web the native module is absent
// and `useShareIntent` no-ops — keeping both builds working.
function ShareIntentGate() {
  const router = useRouter();
  const { hasShareIntent, shareIntent, resetShareIntent } = useShareIntent({
    resetOnBackground: true,
  });

  useEffect(() => {
    if (!hasShareIntent) return;
    const uploadFiles = (shareIntent.files ?? []).filter(
      (f) => isUploadableMimeType(f.mimeType) && !!f.path,
    );
    if (uploadFiles.length > 0) {
      const sharedImages = JSON.stringify(
        uploadFiles.map((f) => ({
          uri: f.path,
          name: f.fileName || fallbackFilename(f.mimeType),
          type: uploadMimeType(f.mimeType),
        })),
      );
      router.navigate({ pathname: "/capture", params: { sharedImages } });
    } else {
      const shared = shareIntent.webUrl ?? shareIntent.text ?? "";
      if (shared) {
        router.navigate({ pathname: "/capture", params: { shared } });
      }
    }
    resetShareIntent();
    // Only react to a newly-received intent; resetShareIntent clears it after.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasShareIntent]);

  return null;
}

export default function RootLayout() {
  const [fontsLoaded] = useFonts({
    InstrumentSans_400Regular,
    InstrumentSans_500Medium,
    InstrumentSans_600SemiBold,
    Newsreader_500Medium_Italic,
    Newsreader_600SemiBold_Italic,
    JetBrainsMono_400Regular,
    JetBrainsMono_500Medium,
  });

  useEffect(() => {
    if (fontsLoaded) void SplashScreen.hideAsync();
  }, [fontsLoaded]);

  if (!fontsLoaded) {
    return <View style={{ flex: 1, backgroundColor: colors.paper }} />;
  }

  const tree = (
    <QueryProvider>
      <SettingsProvider>
        <CaptureQueueProvider>
          <StatusBar style="dark" />
          <ShareIntentGate />
          <NotificationRouterGate />
          <Stack
            screenOptions={{
              headerShown: false,
              contentStyle: { backgroundColor: colors.canvas },
            }}
          >
            <Stack.Screen name="(tabs)" />
          </Stack>
        </CaptureQueueProvider>
      </SettingsProvider>
    </QueryProvider>
  );

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <MorphProvider>
          {clerkPublishableKey ? (
            <ClerkProvider publishableKey={clerkPublishableKey} tokenCache={tokenCache}>
              {tree}
            </ClerkProvider>
          ) : (
            tree
          )}
        </MorphProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
