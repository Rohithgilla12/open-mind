import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { PressScale } from "@/components/PressScale";
import { checkToken, claimDeviceCode } from "@/lib/api";
import { useSettingsContext } from "@/lib/settings-context";
import { colors, fonts, radius, spacing } from "@/lib/theme";

type Status =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "valid" }
  | { kind: "invalid" }
  | { kind: "saved_unconfirmed"; reason: "unreachable" | "server"; code?: number }
  | { kind: "save_failed" }
  | { kind: "incomplete" };

// Separate from `Status` above (which drives the manual token form and must
// stay untouched) — this only tracks the connect-with-code attempt itself.
// On success it hands off to the shared `status`/StatusMessage so a claimed
// code produces the exact same confirmation as a successful Validate & save.
type ClaimStatus = { kind: "idle" } | { kind: "claiming" } | { kind: "error"; message: string };

export default function SettingsScreen() {
  const { settings, save, signOut } = useSettingsContext();
  const [instanceUrl, setInstanceUrl] = useState("");
  const [token, setToken] = useState("");
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [code, setCode] = useState("");
  const [claimStatus, setClaimStatus] = useState<ClaimStatus>({ kind: "idle" });
  const [focusedField, setFocusedField] = useState<"instance" | "code" | "token" | null>(null);

  useEffect(() => {
    if (settings) {
      setInstanceUrl(settings.instanceUrl);
      setToken(settings.token);
    }
  }, [settings]);

  async function onValidateAndSave() {
    const url = instanceUrl.trim().replace(/\/+$/, "");
    const tok = token.trim();
    if (!url || !tok) {
      setStatus({ kind: "incomplete" });
      return;
    }
    setStatus({ kind: "checking" });
    const code = await checkToken({ instanceUrl: url, token: tok });
    // 401 is the only definitive "wrong token" — never persist it.
    if (code === 401) {
      setStatus({ kind: "invalid" });
      return;
    }
    // Every other result (200 confirmed; 0 unreachable; 429/5xx busy) still
    // persists the settings. An indeterminate result means the instance is
    // momentarily down or rate-limited, not that the token is bad — so we save
    // it anyway so it survives a relaunch and the user never has to re-enter it
    // once the instance recovers.
    try {
      await save({ instanceUrl: url, token: tok });
    } catch {
      setStatus({ kind: "save_failed" });
      return;
    }
    if (code === 200) {
      setStatus({ kind: "valid" });
    } else if (code === 0) {
      setStatus({ kind: "saved_unconfirmed", reason: "unreachable" });
    } else {
      setStatus({ kind: "saved_unconfirmed", reason: "server", code });
    }
  }

  async function onConnect() {
    const url = instanceUrl.trim().replace(/\/+$/, "");
    const codeInput = code.trim();
    if (!url || !codeInput) {
      setClaimStatus({ kind: "error", message: "Enter both an instance URL and a code." });
      return;
    }
    setClaimStatus({ kind: "claiming" });
    const res = await claimDeviceCode(url, codeInput, "Mobile");
    if (res.ok && res.key) {
      try {
        await save({ instanceUrl: url, token: res.key });
      } catch {
        setClaimStatus({ kind: "error", message: "Couldn't save to secure storage — try again." });
        return;
      }
      setCode("");
      setClaimStatus({ kind: "idle" });
      // Same confirmation a successful Validate & save would show.
      setStatus({ kind: "valid" });
      return;
    }
    if (res.status === 404) {
      setClaimStatus({ kind: "error", message: "Invalid, expired, or already-used code." });
    } else if (res.status === 429) {
      setClaimStatus({ kind: "error", message: "Too many attempts — wait a moment and try again." });
    } else if (res.status === 0) {
      setClaimStatus({ kind: "error", message: "Couldn't reach that instance — check the URL." });
    } else {
      setClaimStatus({ kind: "error", message: "Couldn't connect — try again." });
    }
  }

  async function onSignOut() {
    await signOut();
    setInstanceUrl("");
    setToken("");
    setStatus({ kind: "idle" });
  }

  const checking = status.kind === "checking";
  const claiming = claimStatus.kind === "claiming";

  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      <KeyboardAvoidingView
        style={styles.flex}
        behavior={Platform.OS === "ios" ? "padding" : undefined}
      >
        <ScrollView contentContainerStyle={styles.container} keyboardShouldPersistTaps="handled">
          <Text style={styles.title}>Settings</Text>
          <Text style={styles.subtitle}>Connect to your Openmind instance</Text>

          <View style={styles.field}>
            <Text style={styles.label}>INSTANCE URL</Text>
            <View style={styles.inputCard}>
              <TextInput
                style={[styles.input, focusedField === "instance" && styles.inputFocused]}
                value={instanceUrl}
                onChangeText={setInstanceUrl}
                onFocus={() => setFocusedField("instance")}
                onBlur={() => setFocusedField(null)}
                placeholder="https://openmind.example.com"
                placeholderTextColor={colors.inkFaint}
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="url"
                inputMode="url"
              />
            </View>
          </View>

          <Text style={styles.sectionHeading}>Connect with code</Text>
          <Text style={styles.sectionHint}>
            Scan the QR code on your Openmind web app, or type the code it shows.
          </Text>

          <View style={styles.field}>
            <Text style={styles.label}>DEVICE-CONNECT CODE</Text>
            <View style={styles.inputCard}>
              <TextInput
                style={[styles.input, focusedField === "code" && styles.inputFocused]}
                value={code}
                onChangeText={(next) => {
                  setCode(next);
                  if (claimStatus.kind === "error") setClaimStatus({ kind: "idle" });
                }}
                onFocus={() => setFocusedField("code")}
                onBlur={() => setFocusedField(null)}
                placeholder="ABCD-EFGH"
                placeholderTextColor={colors.inkFaint}
                autoCapitalize="characters"
                autoCorrect={false}
              />
            </View>
          </View>

          <ClaimStatusMessage status={claimStatus} />

          <PressScale onPress={onConnect} disabled={claiming}>
            <View style={styles.connectButton}>
              {claiming ? (
                <ActivityIndicator color={colors.cobalt} />
              ) : (
                <Text style={styles.connectButtonText}>Connect</Text>
              )}
            </View>
          </PressScale>

          <Text style={styles.divider}>OR CONNECT MANUALLY</Text>

          <View style={styles.field}>
            <Text style={styles.label}>API TOKEN</Text>
            <View style={styles.inputCard}>
              <TextInput
                style={[styles.input, focusedField === "token" && styles.inputFocused]}
                value={token}
                onChangeText={setToken}
                onFocus={() => setFocusedField("token")}
                onBlur={() => setFocusedField(null)}
                placeholder="Paste your API token"
                placeholderTextColor={colors.inkFaint}
                autoCapitalize="none"
                autoCorrect={false}
                secureTextEntry
              />
            </View>
          </View>

          <StatusMessage status={status} />

          <PressScale onPress={onValidateAndSave} disabled={checking}>
            <View style={styles.primaryButton}>
              {checking ? (
                <ActivityIndicator color={colors.paper} />
              ) : (
                <Text style={styles.primaryButtonText}>Validate & save</Text>
              )}
            </View>
          </PressScale>

          {settings ? (
            <PressScale onPress={onSignOut}>
              <View style={styles.secondaryButton}>
                <Text style={styles.secondaryButtonText}>Sign out</Text>
              </View>
            </PressScale>
          ) : null}
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

function StatusMessage({ status }: { status: Status }) {
  switch (status.kind) {
    case "valid":
      return <Text style={[styles.status, { color: colors.cobalt }]}>Saved ✓ Token valid.</Text>;
    case "invalid":
      return <Text style={[styles.status, { color: colors.danger }]}>Invalid token (401).</Text>;
    case "saved_unconfirmed":
      return (
        <Text style={[styles.status, { color: colors.gold }]}>
          {status.reason === "unreachable"
            ? "Saved — but couldn't reach the instance to confirm. Check the URL; your library will load once it's reachable."
            : `Saved — but the instance was busy${status.code ? ` (${status.code})` : ""}, so the token isn't confirmed yet. It'll work once the instance recovers.`}
        </Text>
      );
    case "save_failed":
      return (
        <Text style={[styles.status, { color: colors.danger }]}>
          Couldn't save to secure storage — try again.
        </Text>
      );
    case "incomplete":
      return (
        <Text style={[styles.status, { color: colors.danger }]}>
          Enter both an instance URL and a token.
        </Text>
      );
    default:
      return null;
  }
}

function ClaimStatusMessage({ status }: { status: ClaimStatus }) {
  if (status.kind !== "error") return null;
  return <Text style={[styles.status, { color: colors.danger }]}>{status.message}</Text>;
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.canvas },
  flex: { flex: 1 },
  container: { paddingHorizontal: spacing.xl, paddingTop: spacing.lg, paddingBottom: spacing.xxl },
  title: { fontFamily: fonts.serifBold, fontSize: 27, color: colors.ink },
  subtitle: {
    fontFamily: fonts.mono,
    fontSize: 12,
    color: colors.inkFaint,
    marginTop: spacing.xs,
    marginBottom: spacing.xl,
  },
  sectionHeading: { fontFamily: fonts.sansSemiBold, fontSize: 15, color: colors.ink, marginBottom: spacing.xs },
  sectionHint: {
    fontFamily: fonts.sans,
    fontSize: 13,
    color: colors.inkMuted,
    lineHeight: 18,
    marginBottom: spacing.md,
  },
  divider: {
    fontFamily: fonts.mono,
    fontSize: 10,
    letterSpacing: 0.5,
    color: colors.inkFaint,
    textAlign: "center",
    marginTop: spacing.xl,
    marginBottom: spacing.lg,
  },
  field: { marginBottom: spacing.lg },
  label: {
    fontFamily: fonts.monoMedium,
    fontSize: 10,
    letterSpacing: 0.8,
    textTransform: "uppercase",
    color: colors.inkMuted,
    marginBottom: spacing.sm,
  },
  inputCard: {
    backgroundColor: colors.paper,
    borderRadius: radius.card,
    padding: 3,
  },
  input: {
    borderWidth: 1,
    borderColor: colors.hairline,
    borderRadius: radius.button,
    backgroundColor: colors.cardSurface,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.md,
    fontFamily: fonts.sans,
    fontSize: 15,
    color: colors.ink,
  },
  inputFocused: { borderColor: colors.cobalt, borderWidth: 1.5 },
  status: { fontFamily: fonts.sans, fontSize: 13, marginBottom: spacing.md },
  primaryButton: {
    backgroundColor: colors.cobalt,
    borderRadius: radius.button,
    paddingVertical: spacing.md,
    alignItems: "center",
    marginTop: spacing.sm,
  },
  primaryButtonText: { color: colors.paper, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  secondaryButton: {
    borderRadius: radius.button,
    borderWidth: 1,
    borderColor: colors.hairline,
    paddingVertical: spacing.md,
    alignItems: "center",
    marginTop: spacing.md,
  },
  secondaryButtonText: { color: colors.danger, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  connectButton: {
    borderRadius: radius.button,
    borderWidth: 1,
    borderColor: colors.cobalt,
    paddingVertical: spacing.md,
    alignItems: "center",
  },
  connectButtonText: { color: colors.cobalt, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  buttonPressed: { opacity: 0.7 },
});
