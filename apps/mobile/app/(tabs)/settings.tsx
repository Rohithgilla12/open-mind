import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { checkToken } from "@/lib/api";
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

export default function SettingsScreen() {
  const { settings, save, signOut } = useSettingsContext();
  const [instanceUrl, setInstanceUrl] = useState("");
  const [token, setToken] = useState("");
  const [status, setStatus] = useState<Status>({ kind: "idle" });

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

  async function onSignOut() {
    await signOut();
    setInstanceUrl("");
    setToken("");
    setStatus({ kind: "idle" });
  }

  const checking = status.kind === "checking";

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
            <TextInput
              style={styles.input}
              value={instanceUrl}
              onChangeText={setInstanceUrl}
              placeholder="https://openmind.example.com"
              placeholderTextColor={colors.inkFaint}
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="url"
              inputMode="url"
            />
          </View>

          <View style={styles.field}>
            <Text style={styles.label}>API TOKEN</Text>
            <TextInput
              style={styles.input}
              value={token}
              onChangeText={setToken}
              placeholder="Paste your API token"
              placeholderTextColor={colors.inkFaint}
              autoCapitalize="none"
              autoCorrect={false}
              secureTextEntry
            />
          </View>

          <StatusMessage status={status} />

          <Pressable
            style={({ pressed }) => [
              styles.primaryButton,
              (pressed || checking) && styles.buttonPressed,
            ]}
            onPress={onValidateAndSave}
            disabled={checking}
          >
            {checking ? (
              <ActivityIndicator color={colors.paper} />
            ) : (
              <Text style={styles.primaryButtonText}>Validate & save</Text>
            )}
          </Pressable>

          {settings ? (
            <Pressable
              style={({ pressed }) => [styles.secondaryButton, pressed && styles.buttonPressed]}
              onPress={onSignOut}
            >
              <Text style={styles.secondaryButtonText}>Sign out</Text>
            </Pressable>
          ) : null}
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

function StatusMessage({ status }: { status: Status }) {
  switch (status.kind) {
    case "valid":
      return <Text style={[styles.status, { color: colors.cobalt }]}>Token valid — saved.</Text>;
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

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.paper },
  flex: { flex: 1 },
  container: { paddingHorizontal: spacing.xl, paddingTop: spacing.lg, paddingBottom: spacing.xxl },
  title: { fontFamily: fonts.serif, fontSize: 27, fontWeight: "600", color: colors.ink },
  subtitle: {
    fontFamily: fonts.mono,
    fontSize: 12,
    color: colors.inkFaint,
    marginTop: spacing.xs,
    marginBottom: spacing.xl,
  },
  field: { marginBottom: spacing.lg },
  label: {
    fontFamily: fonts.mono,
    fontSize: 10,
    letterSpacing: 0.5,
    color: colors.inkMuted,
    marginBottom: spacing.sm,
  },
  input: {
    borderWidth: 1,
    borderColor: colors.hairline,
    borderRadius: radius.button,
    backgroundColor: colors.cardSurface,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.md,
    fontSize: 15,
    color: colors.ink,
  },
  status: { fontSize: 13, marginBottom: spacing.md },
  primaryButton: {
    backgroundColor: colors.cobalt,
    borderRadius: radius.button,
    paddingVertical: spacing.md,
    alignItems: "center",
    marginTop: spacing.sm,
  },
  primaryButtonText: { color: colors.paper, fontSize: 15, fontWeight: "600" },
  secondaryButton: {
    borderRadius: radius.button,
    borderWidth: 1,
    borderColor: colors.hairline,
    paddingVertical: spacing.md,
    alignItems: "center",
    marginTop: spacing.md,
  },
  secondaryButtonText: { color: colors.danger, fontSize: 15, fontWeight: "600" },
  buttonPressed: { opacity: 0.7 },
});
