// Handles openmind://link?code=X&url=Y — the deep link opened when a phone's
// native camera scans the device-connect QR code shown on the web app.
// expo-router maps this to the route automatically because the `openmind`
// scheme is registered in app.json. The user confirms the destination host
// before anything is claimed: a deep link can be distributed by anyone, so
// connecting silently would let a crafted link repoint the app at an
// attacker's server. Claims the code, persists the resulting key exactly like
// a manual Settings save, then returns to the Library.
import { Link, useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { PressScale } from "@/components/PressScale";
import { claimDeviceCode } from "@/lib/api";
import { useSettingsContext } from "@/lib/settings-context";
import { colors, fonts, radius, spacing } from "@/lib/theme";

type State =
  | { kind: "missing" }
  | { kind: "confirm"; host: string }
  | { kind: "claiming" }
  | { kind: "success" }
  | { kind: "error"; message: string };

const SUCCESS_REDIRECT_DELAY_MS = 900;

// Only absolute http(s) URLs may become the stored instance URL.
function parseInstanceUrl(raw: string): { url: string; host: string } | null {
  const trimmed = raw.trim().replace(/\/+$/, "");
  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    return null;
  }
  if ((parsed.protocol !== "https:" && parsed.protocol !== "http:") || !parsed.host) {
    return null;
  }
  return { url: trimmed, host: parsed.host };
}

export default function LinkScreen() {
  const params = useLocalSearchParams<{ code?: string; url?: string }>();
  const router = useRouter();
  const { save } = useSettingsContext();

  const rawCode = typeof params.code === "string" ? params.code : undefined;
  const rawUrl = typeof params.url === "string" ? params.url : undefined;
  const instance = rawUrl ? parseInstanceUrl(rawUrl) : null;

  const [state, setState] = useState<State>(() =>
    rawCode && instance ? { kind: "confirm", host: instance.host } : { kind: "missing" },
  );

  async function onConfirm() {
    if (!rawCode || !instance) return;
    setState({ kind: "claiming" });
    const res = await claimDeviceCode(instance.url, rawCode, "Mobile");
    if (res.ok && res.key) {
      try {
        await save({ instanceUrl: instance.url, token: res.key });
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

  useEffect(() => {
    if (state.kind !== "success") return;
    const timer = setTimeout(() => router.replace("/"), SUCCESS_REDIRECT_DELAY_MS);
    return () => clearTimeout(timer);
  }, [state.kind, router]);

  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      <View style={styles.container}>
        <Text style={styles.title}>Connect device</Text>
        <Body state={state} onConfirm={onConfirm} />
      </View>
    </SafeAreaView>
  );
}

function Body({ state, onConfirm }: { state: State; onConfirm: () => void }) {
  switch (state.kind) {
    case "missing":
      return (
        <>
          <Text style={styles.message}>
            Open this link from the QR code shown on your Openmind web app, or enter a
            device-connect code manually in Settings. The link must point at an http(s)
            instance URL.
          </Text>
          <Link href="/settings" style={styles.link}>
            Open Settings
          </Link>
        </>
      );
    case "confirm":
      return (
        <>
          <Text style={styles.message}>
            This will connect the app to <Text style={styles.host}>{state.host}</Text> and
            replace any existing connection. Only continue if this is your Openmind
            instance.
          </Text>
          <PressScale onPress={onConfirm}>
            <View style={styles.primaryButton}>
              <Text style={styles.primaryButtonText}>Connect to {state.host}</Text>
            </View>
          </PressScale>
          <Link href="/settings" style={styles.link}>
            Cancel — open Settings instead
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
  safe: { flex: 1, backgroundColor: colors.canvas },
  container: { flex: 1, paddingHorizontal: spacing.xl, paddingTop: spacing.xl, gap: spacing.lg },
  title: { fontFamily: fonts.serifBold, fontSize: 27, color: colors.ink },
  message: { fontFamily: fonts.sans, fontSize: 14, color: colors.inkMuted, lineHeight: 20 },
  host: { fontFamily: fonts.mono, color: colors.ink },
  centre: { alignItems: "center", gap: spacing.md, marginTop: spacing.xl },
  centreText: { textAlign: "center" },
  link: { fontFamily: fonts.sansSemiBold, fontSize: 14, color: colors.cobalt },
  primaryButton: {
    backgroundColor: colors.cobalt,
    borderRadius: radius.button,
    paddingVertical: spacing.md,
    alignItems: "center",
  },
  primaryButtonText: { color: colors.paper, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  button: {
    borderRadius: radius.button,
    borderWidth: 1,
    borderColor: colors.cobalt,
    paddingVertical: spacing.md,
    alignItems: "center",
  },
  buttonPressed: { opacity: 0.7 },
  buttonText: { color: colors.cobalt, fontFamily: fonts.sansSemiBold, fontSize: 15 },
});
