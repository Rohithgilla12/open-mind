import { Link } from "expo-router";
import { StyleSheet, Text, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useSettingsContext } from "@/lib/settings-context";
import { colors, fonts, radius, spacing } from "@/lib/theme";

export default function CaptureScreen() {
  const { configured, loading } = useSettingsContext();

  return (
    <SafeAreaView style={styles.safe} edges={["top", "left", "right"]}>
      <View style={styles.container}>
        <Text style={styles.title}>Capture</Text>
        <Text style={styles.subtitle}>Save a link or a note — enriches in place</Text>

        {loading ? null : configured ? (
          <View style={styles.placeholder}>
            <Text style={styles.placeholderText}>
              Quick capture (URL or note) will live here. (Coming in Task 2.)
            </Text>
          </View>
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
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.paper },
  container: { flex: 1, paddingHorizontal: spacing.xl, paddingTop: spacing.lg },
  title: { fontFamily: fonts.serif, fontSize: 27, fontWeight: "600", color: colors.ink },
  subtitle: {
    fontFamily: fonts.mono,
    fontSize: 12,
    color: colors.inkFaint,
    marginTop: spacing.xs,
  },
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
