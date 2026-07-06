import { Pressable, StyleSheet, Text, View } from "react-native";
import type { Item } from "@/lib/api";
import { colors, fonts, radius, spacing } from "@/lib/theme";

/** Extract a bare host (no scheme, no www., no path) from a URL, or null. */
function hostOf(url?: string): string | null {
  if (!url) return null;
  const match = /^[a-z]+:\/\/([^/?#]+)/i.exec(url.trim());
  if (!match) return null;
  return match[1].replace(/^www\./i, "");
}

type ItemRowProps = {
  item: Item;
  onPress: (item: Item) => void;
};

export function ItemRow({ item, onPress }: ItemRowProps) {
  const host = hostOf(item.url);
  const primary = item.title?.trim() || host || item.url || "Untitled";
  const pending = item.status === "pending";

  // Caption: enrichment state takes priority; otherwise show card type and
  // host together (both are useful hints), falling back to status.
  let caption: string;
  if (pending) {
    caption = "enriching…";
  } else {
    caption = [item.cardType, host].filter(Boolean).join(" · ") || item.status;
  }

  return (
    <Pressable
      style={({ pressed }) => [styles.row, pressed && styles.rowPressed]}
      onPress={() => onPress(item)}
    >
      <Text style={styles.title} numberOfLines={2}>
        {primary}
      </Text>
      <View style={styles.metaRow}>
        <Text style={[styles.caption, pending && styles.captionPending]} numberOfLines={1}>
          {caption}
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  row: {
    padding: spacing.lg,
    borderRadius: radius.card,
    borderWidth: 1,
    borderColor: colors.hairline,
    backgroundColor: colors.cardSurface,
    gap: spacing.sm,
  },
  rowPressed: { opacity: 0.7 },
  title: { fontSize: 15, fontWeight: "600", color: colors.ink, lineHeight: 21 },
  metaRow: { flexDirection: "row", alignItems: "center" },
  caption: { fontFamily: fonts.mono, fontSize: 12, color: colors.inkFaint },
  captionPending: { color: colors.cobalt },
});
