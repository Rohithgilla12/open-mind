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
  | { kind: "unreachable" }
  | { kind: "server_error"; code: number }
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
    if (code === 200) {
      await save({ instanceUrl: url, token: tok });
      setStatus({ kind: "valid" });
    } else if (code === 0) {
      setStatus({ kind: "unreachable" });
    } else if (code === 401) {
      setStatus({ kind: "invalid" });
    } else {
      // 429 (rate limited), 502 (backend down), or any other non-200 — the
      // token may well be fine, so don't claim it's invalid.
      setStatus({ kind: "server_error", code });
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
    case "server_error":
      return (
        <Text style={[styles.status, { color: colors.danger }]}>
          {status.code === 429
            ? "Rate limited — try again shortly."
            : `Instance error (${status.code}) — try again shortly.`}
        </Text>
      );
    case "unreachable":
      return (
        <Text style={[styles.status, { color: colors.danger }]}>
          Instance unreachable — check the URL.
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
