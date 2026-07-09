// The inline iOS Share Extension UI. This is the root component of a SEPARATE
// React Native bundle that iOS renders *inside* the share sheet (a small modal
// sheet over whatever app you're in) — the whole point is that saving never
// leaves the current app / never launches Openmind.
//
// Two constraints shape this file, both from expo-share-extension:
//   1. It runs in its own process, so it can't share React context/state with
//      the main app. It reads the instance URL + token from the shared keychain
//      (see lib/settings.ts `keychainAccessGroup`) and talks to /api directly.
//   2. Runtime-loaded custom fonts (our @expo-google-fonts brand fonts) don't
//      work in a share extension (NSURLSession limitation — see the package
//      README). So this UI uses the system font: palette only, no fontFamily.
import { close, openHostApp, type InitialProps } from "expo-share-extension";
import { useCallback, useEffect, useState } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from "react-native";
import { saveItem } from "@/lib/api";
import { getSettings, type Settings } from "@/lib/settings";
import { colors, radius, spacing } from "@/lib/theme";

const URL_RE = /^https?:\/\//i;

// Auto-dismiss shortly after a successful save so the sheet feels instant —
// long enough to register the checkmark, short enough to stay out of the way.
const CLOSE_DELAY_MS = 750;

type Phase =
  | { kind: "loading" }
  | { kind: "unconfigured" }
  | { kind: "ready" }
  | { kind: "saving" }
  | { kind: "saved" }
  | { kind: "rejected" }
  | { kind: "error" };

/**
 * Pick the shareable payload out of the initial props. Safari/link shares
 * arrive as `url`; selected text and message contents arrive as `text`. We
 * prefer the URL when both are present (it's the higher-signal capture).
 */
function pickShared({ url, text }: InitialProps): string {
  return (url ?? text ?? "").trim();
}

export default function ShareExtension(props: InitialProps) {
  const shared = pickShared(props);
  const isUrl = URL_RE.test(shared);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [phase, setPhase] = useState<Phase>({ kind: "loading" });

  // Read the connection from the shared keychain once on mount. No payload to
  // save (an empty share) is treated as an error the user can only dismiss.
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const stored = await getSettings();
      if (cancelled) return;
      if (!stored) {
        setPhase({ kind: "unconfigured" });
        return;
      }
      setSettings(stored);
      setPhase(shared ? { kind: "ready" } : { kind: "error" });
    })();
    return () => {
      cancelled = true;
    };
  }, [shared]);

  const onSave = useCallback(async () => {
    if (!settings || !shared) return;
    setPhase({ kind: "saving" });
    const res = await saveItem(isUrl ? { url: shared } : { note: shared }, settings);
    if (res.ok) {
      setPhase({ kind: "saved" });
      setTimeout(close, CLOSE_DELAY_MS);
    } else if (res.status === 401) {
      setPhase({ kind: "rejected" });
    } else {
      setPhase({ kind: "error" });
    }
  }, [settings, shared, isUrl]);

  return (
    <View style={styles.root}>
      <View style={styles.card}>
        <Text style={styles.kicker}>SAVE TO OPENMIND</Text>

        {phase.kind === "loading" ? (
          <View style={styles.centerRow}>
            <ActivityIndicator color={colors.cobalt} />
          </View>
        ) : phase.kind === "unconfigured" ? (
          <>
            <Text style={styles.body}>
              Connect this device to your Openmind instance first.
            </Text>
            <View style={styles.actions}>
              <SecondaryButton label="Cancel" onPress={close} />
              <PrimaryButton
                label="Open Openmind"
                onPress={() => openHostApp("settings")}
              />
            </View>
          </>
        ) : (
          <>
            <Text style={styles.preview} numberOfLines={3} ellipsizeMode="tail">
              {shared || "Nothing to save"}
            </Text>
            <StatusLine phase={phase} isUrl={isUrl} />
            <View style={styles.actions}>
              <SecondaryButton
                label={phase.kind === "saved" ? "Done" : "Cancel"}
                onPress={close}
                disabled={phase.kind === "saving"}
              />
              <PrimaryButton
                label={isUrl ? "Save link" : "Save note"}
                onPress={onSave}
                disabled={
                  phase.kind === "saving" ||
                  phase.kind === "saved" ||
                  phase.kind === "error" ||
                  !shared
                }
                busy={phase.kind === "saving"}
              />
            </View>
          </>
        )}
      </View>
    </View>
  );
}

function StatusLine({ phase, isUrl }: { phase: Phase; isUrl: boolean }) {
  switch (phase.kind) {
    case "saved":
      return (
        <Text style={[styles.status, { color: colors.green }]}>
          ✓ Saved — it'll enrich in your Library.
        </Text>
      );
    case "rejected":
      return (
        <Text style={[styles.status, { color: colors.danger }]}>
          Token rejected — reconnect in Openmind.
        </Text>
      );
    case "error":
      return (
        <Text style={[styles.status, { color: colors.danger }]}>
          {isUrl ? "Couldn't save — check your connection." : "Nothing to save."}
        </Text>
      );
    default:
      return <View style={styles.statusSpacer} />;
  }
}

function PrimaryButton({
  label,
  onPress,
  disabled,
  busy,
}: {
  label: string;
  onPress: () => void;
  disabled?: boolean;
  busy?: boolean;
}) {
  return (
    <Pressable
      onPress={onPress}
      disabled={disabled}
      style={({ pressed }) => [
        styles.primary,
        disabled && styles.buttonDisabled,
        pressed && !disabled && styles.pressed,
      ]}
    >
      {busy ? (
        <ActivityIndicator color={colors.paper} />
      ) : (
        <Text style={styles.primaryText}>{label}</Text>
      )}
    </Pressable>
  );
}

function SecondaryButton({
  label,
  onPress,
  disabled,
}: {
  label: string;
  onPress: () => void;
  disabled?: boolean;
}) {
  return (
    <Pressable
      onPress={onPress}
      disabled={disabled}
      style={({ pressed }) => [
        styles.secondary,
        disabled && styles.buttonDisabled,
        pressed && !disabled && styles.pressed,
      ]}
    >
      <Text style={styles.secondaryText}>{label}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, justifyContent: "center", padding: spacing.lg },
  card: {
    backgroundColor: colors.paper,
    borderRadius: radius.overlay,
    borderWidth: 1,
    borderColor: colors.hairline,
    padding: spacing.xl,
    gap: spacing.md,
  },
  kicker: {
    fontSize: 10,
    letterSpacing: 0.8,
    textTransform: "uppercase",
    color: colors.inkMuted,
    fontWeight: "600",
  },
  preview: {
    fontSize: 15,
    color: colors.ink,
    lineHeight: 21,
  },
  body: { fontSize: 14, color: colors.inkMuted, lineHeight: 20 },
  centerRow: { alignItems: "center", paddingVertical: spacing.lg },
  status: { fontSize: 13 },
  statusSpacer: { height: 13 },
  actions: {
    flexDirection: "row",
    justifyContent: "flex-end",
    gap: spacing.sm,
    marginTop: spacing.xs,
  },
  primary: {
    backgroundColor: colors.cobalt,
    borderRadius: radius.button,
    paddingVertical: spacing.md,
    paddingHorizontal: spacing.lg,
    minWidth: 96,
    alignItems: "center",
    justifyContent: "center",
  },
  primaryText: { color: colors.paper, fontSize: 15, fontWeight: "600" },
  secondary: {
    borderRadius: radius.button,
    paddingVertical: spacing.md,
    paddingHorizontal: spacing.lg,
    alignItems: "center",
    justifyContent: "center",
    borderWidth: 1,
    borderColor: colors.hairline,
  },
  secondaryText: { color: colors.inkMuted, fontSize: 15, fontWeight: "500" },
  buttonDisabled: { opacity: 0.4 },
  pressed: { opacity: 0.7 },
});
