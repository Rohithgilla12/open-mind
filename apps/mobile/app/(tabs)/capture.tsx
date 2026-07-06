import { Ionicons } from "@expo/vector-icons";
import { Link, useLocalSearchParams } from "expo-router";
import { useEffect, useRef, useState } from "react";
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
import { listItems, saveItem } from "@/lib/api";
import { useSettingsContext } from "@/lib/settings-context";
import { colors, fonts, radius, spacing } from "@/lib/theme";

const URL_RE = /^https?:\/\//i;
const RECOVERY_WINDOW_MS = 2 * 60 * 1000;

type Status =
  | { kind: "idle" }
  | { kind: "saving" }
  | { kind: "saved"; recovered?: boolean }
  | { kind: "rejected" }
  | { kind: "unreachable" }
  | { kind: "unreachable-note" }
  | { kind: "error" };

// After a status-0 (network error/timeout) response, the POST may have
// actually landed. For URL saves we can check: list the newest items and
// look for this exact URL created just now.
async function wasRecentlySaved(url: string): Promise<boolean> {
  const res = await listItems(10);
  if (!res.ok) return false;
  const now = Date.now();
  return res.items.some((item) => {
    if (item.url !== url) return false;
    if (!item.createdAt) return true;
    const created = Date.parse(item.createdAt);
    return Number.isNaN(created) || now - created <= RECOVERY_WINDOW_MS;
  });
}

export default function CaptureScreen() {
  const { configured, loading } = useSettingsContext();
  const params = useLocalSearchParams<{ shared?: string }>();
  const [text, setText] = useState("");
  const [status, setStatus] = useState<Status>({ kind: "idle" });

  // Prefill from a share-sheet intent (routed here by ShareIntentGate). Apply
  // each distinct shared value once so revisiting the tab doesn't clobber edits.
  const appliedShareRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    const shared = typeof params.shared === "string" ? params.shared : undefined;
    if (shared && shared !== appliedShareRef.current) {
      appliedShareRef.current = shared;
      setText(shared);
      setStatus({ kind: "idle" });
    }
  }, [params.shared]);

  const trimmed = text.trim();
  const isUrl = URL_RE.test(trimmed);
  const saving = status.kind === "saving";
  const canSave = trimmed.length > 0 && !saving;

  async function onSave() {
    const value = text.trim();
    if (!value) return;
    const url = URL_RE.test(value);
    setStatus({ kind: "saving" });
    const res = await saveItem(url ? { url: value } : { note: value });
    if (res.ok) {
      setText("");
      setStatus({ kind: "saved" });
    } else if (res.status === 0) {
      if (url && (await wasRecentlySaved(value))) {
        setText("");
        setStatus({ kind: "saved", recovered: true });
        return;
      }
      setStatus({ kind: url ? "unreachable" : "unreachable-note" });
    } else if (res.status === 401) {
      setStatus({ kind: "rejected" });
    } else {
      setStatus({ kind: "error" });
    }
  }

  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      <KeyboardAvoidingView
        style={styles.flex}
        behavior={Platform.OS === "ios" ? "padding" : undefined}
      >
        <ScrollView contentContainerStyle={styles.container} keyboardShouldPersistTaps="handled">
          <Text style={styles.title}>Capture</Text>
          <Text style={styles.subtitle}>Save a link or a note — enriches in place</Text>

          {loading ? null : configured ? (
            <>
              <View style={styles.field}>
                <Text style={styles.label}>{isUrl ? "LINK" : "URL OR NOTE"}</Text>
                <TextInput
                  style={styles.input}
                  value={text}
                  onChangeText={(next) => {
                    setText(next);
                    if (status.kind !== "idle" && status.kind !== "saving") {
                      setStatus({ kind: "idle" });
                    }
                  }}
                  placeholder="Paste a URL or jot a note…"
                  placeholderTextColor={colors.inkFaint}
                  multiline
                  autoCapitalize="none"
                  autoCorrect={false}
                  textAlignVertical="top"
                />
              </View>

              <StatusMessage status={status} />

              <Pressable
                style={({ pressed }) => [
                  styles.primaryButton,
                  (pressed || saving) && styles.buttonPressed,
                  !canSave && styles.buttonDisabled,
                ]}
                onPress={onSave}
                disabled={!canSave}
              >
                {saving ? (
                  <ActivityIndicator color={colors.paper} />
                ) : (
                  <Text style={styles.primaryButtonText}>Save</Text>
                )}
              </Pressable>
            </>
          ) : (
            <View style={styles.placeholder}>
              <Text style={styles.placeholderText}>
                Connect to your Openmind instance before capturing.
              </Text>
              <Link href="/settings" style={styles.link}>
                Open Settings
              </Link>
            </View>
          )}
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

function StatusMessage({ status }: { status: Status }) {
  switch (status.kind) {
    case "saved":
      return (
        <View style={styles.savedRow}>
          <Ionicons name="checkmark-circle" size={16} color={colors.cobalt} />
          <Text style={[styles.status, { color: colors.cobalt }]}>
            {status.recovered ? "Saved — connection was slow." : "Saved — it'll appear in your Library."}
          </Text>
        </View>
      );
    case "rejected":
      return <Text style={[styles.status, { color: colors.danger }]}>Token rejected — check Settings.</Text>;
    case "unreachable":
      return (
        <Text style={[styles.status, { color: colors.danger }]}>
          Instance unreachable — check your connection.
        </Text>
      );
    case "unreachable-note":
      return (
        <Text style={[styles.status, { color: colors.danger }]}>
          Connection problem — the note may or may not have saved. Check your Library before retrying.
        </Text>
      );
    case "error":
      return <Text style={[styles.status, { color: colors.danger }]}>Couldn't save — try again.</Text>;
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
    minHeight: 120,
  },
  savedRow: { flexDirection: "row", alignItems: "center", gap: spacing.xs, marginBottom: spacing.md },
  status: { fontSize: 13, marginBottom: spacing.md },
  primaryButton: {
    backgroundColor: colors.cobalt,
    borderRadius: radius.button,
    paddingVertical: spacing.md,
    alignItems: "center",
    marginTop: spacing.sm,
  },
  primaryButtonText: { color: colors.paper, fontSize: 15, fontWeight: "600" },
  buttonPressed: { opacity: 0.7 },
  buttonDisabled: { opacity: 0.4 },
  placeholder: {
    marginTop: spacing.xl,
    padding: spacing.lg,
    borderRadius: radius.card,
    borderWidth: 1,
    borderColor: colors.hairline,
    backgroundColor: colors.cardSurface,
    gap: spacing.md,
  },
  placeholderText: { fontSize: 14, color: colors.inkMuted, lineHeight: 20 },
  link: { fontSize: 14, fontWeight: "600", color: colors.cobalt },
});
