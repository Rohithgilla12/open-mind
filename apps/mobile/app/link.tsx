// Handles openmind://link?code=X&url=Y — the deep link opened when a phone's
// native camera scans the device-connect QR code shown on the web app.
// expo-router maps this to the route automatically because the `openmind`
// scheme is registered in app.json. Claims the code, persists the resulting
// key exactly like a manual Settings save, then returns to the Library.
import { Link, useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useRef, useState } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { claimDeviceCode } from "@/lib/api";
import { useSettingsContext } from "@/lib/settings-context";
import { colors, fonts, radius, spacing } from "@/lib/theme";

type State =
  | { kind: "missing" }
  | { kind: "claiming" }
  | { kind: "success" }
  | { kind: "error"; message: string };

const SUCCESS_REDIRECT_DELAY_MS = 900;

export default function LinkScreen() {
  const params = useLocalSearchParams<{ code?: string; url?: string }>();
  const router = useRouter();
  const { save } = useSettingsContext();
  const [state, setState] = useState<State>({ kind: "missing" });
  const attemptedRef = useRef(false);

  const rawCode = typeof params.code === "string" ? params.code : undefined;
  const rawUrl = typeof params.url === "string" ? params.url : undefined;

  // Claim exactly once per (code, url) pair the screen is opened with — a
  // re-render (e.g. from the success-state redirect timer) must not re-fire it.
  useEffect(() => {
    if (attemptedRef.current) return;
    if (!rawCode || !rawUrl) {
      setState({ kind: "missing" });
      return;
    }
    attemptedRef.current = true;

    async function claim(code: string, instanceUrl: string) {
      setState({ kind: "claiming" });
      const url = instanceUrl.trim().replace(/\/+$/, "");
      const res = await claimDeviceCode(url, code, "Mobile");
      if (res.ok && res.key) {
        try {
          await save({ instanceUrl: url, token: res.key });
        } catch {
          setState({ kind: "error", message: "Couldn't save to secure storage — try again." });
          return;
        }
        setState({ kind: "success" });
        return;
      }
      if (res.status === 404) {
        setState({ kind: "error", message: "This code is invalid, expired, or already used." });
      } else if (res.status === 429) {
        setState({ kind: "error", message: "Too many attempts — wait a moment and try again." });
      } else if (res.status === 0) {
        setState({ kind: "error", message: "Couldn't reach that instance — check the URL." });
      } else {
        setState({ kind: "error", message: "Couldn't connect — try again." });
      }
    }

    void claim(rawCode, rawUrl);
  }, [rawCode, rawUrl, save]);

  useEffect(() => {
    if (state.kind !== "success") return;
    const timer = setTimeout(() => router.replace("/"), SUCCESS_REDIRECT_DELAY_MS);
    return () => clearTimeout(timer);
  }, [state.kind, router]);

  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      <View style={styles.container}>
        <Text style={styles.title}>Connect device</Text>
        <Body state={state} />
      </View>
    </SafeAreaView>
  );
}

function Body({ state }: { state: State }) {
  switch (state.kind) {
    case "missing":
      return (
        <>
          <Text style={styles.message}>
            Open this link from the QR code shown on your Openmind web app, or enter a
            device-connect code manually in Settings.
          </Text>
          <Link href="/settings" style={styles.link}>
            Open Settings
          </Link>
        </>
      );
    case "claiming":
      return (
        <View style={styles.centre}>
          <ActivityIndicator color={colors.cobalt} />
          <Text style={[styles.message, styles.centreText]}>Connecting…</Text>
        </View>
      );
    case "success":
      return (
        <Text style={[styles.message, { color: colors.cobalt }]}>
          Connected — taking you to your library…
        </Text>
      );
    case "error":
      return (
        <>
          <Text style={[styles.message, { color: colors.danger }]}>{state.message}</Text>
          <Link href="/settings" asChild>
            <Pressable style={({ pressed }) => [styles.button, pressed && styles.buttonPressed]}>
              <Text style={styles.buttonText}>Go to Settings</Text>
            </Pressable>
          </Link>
        </>
      );
  }
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.paper },
  container: { flex: 1, paddingHorizontal: spacing.xl, paddingTop: spacing.xl, gap: spacing.lg },
  title: { fontFamily: fonts.serif, fontSize: 27, fontWeight: "600", color: colors.ink },
  message: { fontSize: 14, color: colors.inkMuted, lineHeight: 20 },
  centre: { alignItems: "center", gap: spacing.md, marginTop: spacing.xl },
  centreText: { textAlign: "center" },
  link: { fontSize: 14, fontWeight: "600", color: colors.cobalt },
  button: {
    borderRadius: radius.button,
    borderWidth: 1,
    borderColor: colors.cobalt,
    paddingVertical: spacing.md,
    alignItems: "center",
  },
  buttonPressed: { opacity: 0.7 },
  buttonText: { color: colors.cobalt, fontSize: 15, fontWeight: "600" },
});
