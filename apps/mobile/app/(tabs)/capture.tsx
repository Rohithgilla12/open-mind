import { Ionicons } from "@expo/vector-icons";
import * as ImagePicker from "expo-image-picker";
import { Link, useFocusEffect, useLocalSearchParams } from "expo-router";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  ActivityIndicator,
  Alert,
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
import { listItems, saveItem, uploadAsset, type AssetUpload } from "@/lib/api";
import { useCaptureQueue } from "@/lib/capture-queue-context";
import { useInvalidateLists } from "@/lib/mutations";
import { useSettingsContext } from "@/lib/settings-context";
import { colors, fonts, radius, spacing } from "@/lib/theme";
import { fallbackFilename, uploadMimeType } from "@/lib/uploads";

const URL_RE = /^https?:\/\//i;
const CLOCK_SKEW_MS = 30 * 1000;

type Status =
  | { kind: "idle" }
  | { kind: "saving" }
  | { kind: "saved"; recovered?: boolean; count?: number }
  | { kind: "queued" }
  | { kind: "rejected" }
  | { kind: "error"; message?: string };

// After a status-0 (network error/timeout) response, the POST may have
// actually landed. For URL saves we can check: list the newest items and look
// for this exact URL created since THIS attempt started. Anchoring to the
// attempt's start (not a rolling window) matters: the same URL saved
// deliberately twice in a row must not have its second, timed-out save
// "recovered" by finding the first copy. The skew buffer absorbs client/server
// clock drift.
async function wasRecentlySaved(url: string, attemptStartedAt: number): Promise<boolean> {
  const res = await listItems(10);
  if (!res.ok) return false;
  return res.items.some((item) => {
    if (item.url !== url || !item.createdAt) return false;
    const created = Date.parse(item.createdAt);
    return !Number.isNaN(created) && created >= attemptStartedAt - CLOCK_SKEW_MS;
  });
}

function guessName(uri: string, fallback: string): string {
  const cleaned = uri.split("?")[0] ?? uri;
  const base = cleaned.split("/").pop();
  if (base && /\.[a-z0-9]+$/i.test(base)) return base;
  return fallback;
}

function assetFromPicker(asset: ImagePicker.ImagePickerAsset): AssetUpload {
  const mime = asset.mimeType ?? "image/jpeg";
  const ext = mime === "image/png" ? "png" : mime === "image/webp" ? "webp" : "jpg";
  return {
    uri: asset.uri,
    name: asset.fileName ?? guessName(asset.uri, `photo.${ext}`),
    type: mime,
  };
}

function parseSharedImages(raw: string | undefined): AssetUpload[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((row): row is AssetUpload => {
      if (!row || typeof row !== "object") return false;
      const r = row as AssetUpload;
      return typeof r.uri === "string" && r.uri.length > 0;
    }).map((row) => ({
      uri: row.uri,
      name:
        typeof row.name === "string" && row.name
          ? row.name
          : guessName(row.uri, fallbackFilename(row.type)),
      type: uploadMimeType(row.type),
    }));
  } catch {
    return [];
  }
}

export default function CaptureScreen() {
  const { configured, loading } = useSettingsContext();
  const { pendingCount, flushing, enqueue, enqueueAsset, flush } = useCaptureQueue();
  const invalidateLists = useInvalidateLists();
  const params = useLocalSearchParams<{ shared?: string; sharedImages?: string }>();
  const [text, setText] = useState("");
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [focused, setFocused] = useState(false);

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

  const uploadFiles = useCallback(
    async (files: AssetUpload[]) => {
      if (files.length === 0) return;
      setStatus({ kind: "saving" });
      let saved = 0;
      let lastStatus = 0;
      for (let i = 0; i < files.length; i += 1) {
        const res = await uploadAsset(files[i]);
        lastStatus = res.status;
        if (res.ok) {
          saved += 1;
        } else if (res.status === 401) {
          setStatus({ kind: "rejected" });
          if (saved > 0) invalidateLists();
          return;
        } else if (res.status === 0) {
          // Network error: queue this file and every one not yet attempted so
          // nothing is lost, then let the durable queue sync later.
          const { ids } = await enqueueAsset(files.slice(i));
          if (saved > 0) invalidateLists();
          if (ids.length === 0) {
            setStatus({
              kind: "error",
              message: "Couldn't save photo — check your connection and try again.",
            });
          } else {
            setStatus({ kind: "queued" });
          }
          return;
        } else if (res.status === 415) {
          setStatus({
            kind: "error",
            message: "That format isn't supported — try JPEG, PNG, WebP, or GIF.",
          });
          if (saved > 0) invalidateLists();
          return;
        } else {
          setStatus({ kind: "error", message: "Couldn't save photo — try again." });
          if (saved > 0) invalidateLists();
          return;
        }
      }
      if (saved > 0) {
        setStatus({ kind: "saved", count: saved });
        invalidateLists();
      } else {
        setStatus({
          kind: "error",
          message: lastStatus
            ? `Couldn't save photo (HTTP ${lastStatus}).`
            : "Couldn't save photo — try again.",
        });
      }
    },
    [invalidateLists, enqueueAsset],
  );

  // Shared images from Android SEND intents land here as JSON and upload
  // immediately once settings are ready. Do not consume the param while
  // unconfigured — otherwise a share-before-connect fails with no feedback.
  const appliedImagesRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    const raw = typeof params.sharedImages === "string" ? params.sharedImages : undefined;
    if (!raw || raw === appliedImagesRef.current) return;
    if (loading) return;
    if (!configured) return;
    appliedImagesRef.current = raw;
    const files = parseSharedImages(raw);
    if (files.length === 0) return;
    void uploadFiles(files);
  }, [params.sharedImages, uploadFiles, configured, loading]);

  useFocusEffect(
    useCallback(() => {
      void flush();
    }, [flush]),
  );

  const trimmed = text.trim();
  const isUrl = URL_RE.test(trimmed);
  const saving = status.kind === "saving";
  const canSave = trimmed.length > 0 && !saving;

  async function onSave() {
    const value = text.trim();
    if (!value) return;
    const url = URL_RE.test(value);
    setStatus({ kind: "saving" });
    const attemptStartedAt = Date.now();
    const res = await saveItem(url ? { url: value } : { note: value });
    if (res.ok) {
      setText("");
      setStatus({ kind: "saved" });
      invalidateLists();
    } else if (res.status === 0) {
      if (url && (await wasRecentlySaved(value, attemptStartedAt))) {
        setText("");
        setStatus({ kind: "saved", recovered: true });
        invalidateLists();
        return;
      }
      // Durable offline queue — clear the field so the user can keep capturing.
      await enqueue(url ? { url: value } : { note: value });
      setText("");
      setStatus({ kind: "queued" });
    } else if (res.status === 401) {
      setStatus({ kind: "rejected" });
    } else {
      setStatus({ kind: "error" });
    }
  }

  async function pickFromLibrary() {
    const perm = await ImagePicker.requestMediaLibraryPermissionsAsync();
    if (!perm.granted) {
      Alert.alert("Photos permission needed", "Allow photo access in Settings to save images.");
      return;
    }
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ["images"],
      quality: 0.92,
      allowsMultipleSelection: true,
      selectionLimit: 10,
    });
    if (result.canceled || result.assets.length === 0) return;
    await uploadFiles(result.assets.map(assetFromPicker));
  }

  async function takePhoto() {
    const perm = await ImagePicker.requestCameraPermissionsAsync();
    if (!perm.granted) {
      Alert.alert("Camera permission needed", "Allow camera access in Settings to take a photo.");
      return;
    }
    const result = await ImagePicker.launchCameraAsync({
      mediaTypes: ["images"],
      quality: 0.92,
    });
    if (result.canceled || result.assets.length === 0) return;
    await uploadFiles(result.assets.map(assetFromPicker));
  }

  async function onSyncNow() {
    const result = await flush();
    if (result.sent > 0) {
      invalidateLists();
    }
    if (result.sent > 0 && result.remaining === 0) {
      setStatus({ kind: "saved" });
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
          <Text style={styles.subtitle}>Save a link, note, or photo — enriches in place</Text>

          {loading ? null : configured ? (
            <>
              {pendingCount > 0 ? (
                <View style={styles.pendingStrip}>
                  <Text style={styles.pendingText}>
                    {pendingCount} waiting to sync
                  </Text>
                  <PressScale onPress={onSyncNow} disabled={flushing}>
                    <View style={[styles.syncButton, flushing && styles.buttonDisabled]}>
                      {flushing ? (
                        <ActivityIndicator color={colors.cobalt} size="small" />
                      ) : (
                        <Text style={styles.syncButtonText}>Sync now</Text>
                      )}
                    </View>
                  </PressScale>
                </View>
              ) : null}

              <View style={styles.field}>
                <Text style={styles.label}>{isUrl ? "LINK" : "URL OR NOTE"}</Text>
                <View style={styles.inputCard}>
                  <TextInput
                    style={[styles.input, focused && styles.inputFocused]}
                    value={text}
                    onChangeText={(next) => {
                      setText(next);
                      if (status.kind !== "idle" && status.kind !== "saving") {
                        setStatus({ kind: "idle" });
                      }
                    }}
                    onFocus={() => setFocused(true)}
                    onBlur={() => setFocused(false)}
                    placeholder="Paste a URL or jot a note…"
                    placeholderTextColor={colors.inkFaint}
                    multiline
                    autoCapitalize="none"
                    autoCorrect={false}
                    textAlignVertical="top"
                  />
                </View>
              </View>

              <View style={styles.field}>
                <Text style={styles.label}>PHOTO</Text>
                <View style={styles.photoRow}>
                  <PressScale onPress={pickFromLibrary} disabled={saving} style={styles.photoPress}>
                    <View style={[styles.photoButton, saving && styles.buttonDisabled]}>
                      <Ionicons name="images-outline" size={18} color={colors.cobalt} />
                      <Text style={styles.photoButtonText}>Choose photo</Text>
                    </View>
                  </PressScale>
                  {Platform.OS !== "web" ? (
                    <PressScale onPress={takePhoto} disabled={saving} style={styles.photoPress}>
                      <View style={[styles.photoButton, saving && styles.buttonDisabled]}>
                        <Ionicons name="camera-outline" size={18} color={colors.cobalt} />
                        <Text style={styles.photoButtonText}>Take photo</Text>
                      </View>
                    </PressScale>
                  ) : null}
                </View>
              </View>

              <StatusMessage status={status} />

              <PressScale onPress={onSave} disabled={!canSave}>
                <View style={[styles.primaryButton, !canSave && styles.buttonDisabled]}>
                  <Text style={styles.primaryButtonText}>Save</Text>
                </View>
              </PressScale>
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
    case "saving":
      return (
        <View style={styles.savedRow}>
          <ActivityIndicator color={colors.cobalt} size="small" />
          <Text style={[styles.status, { color: colors.inkMuted }]}>Saving…</Text>
        </View>
      );
    case "saved":
      return (
        <View style={styles.savedRow}>
          <Ionicons name="checkmark-circle" size={16} color={colors.cobalt} />
          <Text style={[styles.status, { color: colors.cobalt }]}>
            {status.recovered
              ? "Saved — connection was slow."
              : status.count && status.count > 1
                ? `Saved ${status.count} photos — they'll appear in your Library.`
                : "Saved — it'll appear in your Library."}
          </Text>
        </View>
      );
    case "queued":
      return (
        <View style={styles.savedRow}>
          <Ionicons name="cloud-upload-outline" size={16} color={colors.inkMuted} />
          <Text style={[styles.status, { color: colors.inkMuted }]}>
            Queued — will sync when you’re back online.
          </Text>
        </View>
      );
    case "rejected":
      return <Text style={[styles.status, { color: colors.danger }]}>Token rejected — check Settings.</Text>;
    case "error":
      return (
        <Text style={[styles.status, { color: colors.danger }]}>
          {status.message ?? "Couldn't save — try again."}
        </Text>
      );
    default:
      return null;
  }
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
  pendingStrip: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: spacing.md,
    backgroundColor: colors.paper,
    borderWidth: 1,
    borderColor: colors.hairline,
    borderRadius: radius.card,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    marginBottom: spacing.lg,
  },
  pendingText: { fontFamily: fonts.sansMedium, fontSize: 13, color: colors.inkMuted, flex: 1 },
  syncButton: {
    borderWidth: 1,
    borderColor: colors.cobalt,
    borderRadius: radius.button,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.xs + 2,
    minWidth: 84,
    alignItems: "center",
  },
  syncButtonText: { fontFamily: fonts.sansSemiBold, fontSize: 13, color: colors.cobalt },
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
    minHeight: 120,
  },
  inputFocused: { borderColor: colors.cobalt, borderWidth: 1.5 },
  photoRow: { flexDirection: "row", gap: spacing.sm },
  photoPress: { flex: 1 },
  photoButton: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: spacing.xs,
    backgroundColor: colors.paper,
    borderWidth: 1,
    borderColor: colors.hairline,
    borderRadius: radius.button,
    paddingVertical: spacing.md,
    paddingHorizontal: spacing.sm,
  },
  photoButtonText: { fontFamily: fonts.sansSemiBold, fontSize: 13, color: colors.cobalt },
  savedRow: { flexDirection: "row", alignItems: "center", gap: spacing.xs, marginBottom: spacing.md },
  status: { fontFamily: fonts.sans, fontSize: 13, marginBottom: spacing.md, flexShrink: 1 },
  primaryButton: {
    backgroundColor: colors.cobalt,
    borderRadius: radius.button,
    paddingVertical: spacing.md,
    alignItems: "center",
    marginTop: spacing.sm,
  },
  primaryButtonText: { color: colors.paper, fontFamily: fonts.sansSemiBold, fontSize: 15 },
  buttonDisabled: { opacity: 0.4 },
  placeholder: {
    marginTop: spacing.xl,
    padding: spacing.lg,
    borderRadius: radius.card,
    borderWidth: 1,
    borderColor: colors.hairline,
    backgroundColor: colors.paper,
    gap: spacing.md,
  },
  placeholderText: { fontFamily: fonts.sans, fontSize: 14, color: colors.inkMuted, lineHeight: 20 },
  link: { fontFamily: fonts.sansSemiBold, fontSize: 14, color: colors.cobalt },
});
