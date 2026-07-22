// Native iOS tab bar (real UITabBar via react-native-screens) so it adopts the
// system Liquid Glass material automatically on iOS 26, minimizing on scroll.
// No backgroundColor — that lets the glass show through; tintColor keeps the
// cobalt brand accent. Each icon carries an SF Symbol (iOS, true native glyph)
// AND an Ionicon via `src` (Android + any non-SF platform) so icons render
// everywhere. On iOS < 26 / Android the bar falls back to the platform-standard
// opaque tab bar with no code change.
import { Ionicons } from "@expo/vector-icons";
import { NativeTabs } from "expo-router/unstable-native-tabs";
import { colors } from "@/lib/theme";

export default function TabsLayout() {
  return (
    <NativeTabs tintColor={colors.cobalt} minimizeBehavior="onScrollDown">
      <NativeTabs.Trigger name="index">
        <NativeTabs.Trigger.Icon
          sf="books.vertical"
          src={<NativeTabs.Trigger.VectorIcon family={Ionicons} name="albums-outline" />}
        />
        <NativeTabs.Trigger.Label>Library</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="desk">
        <NativeTabs.Trigger.Icon
          sf="bookmark"
          src={<NativeTabs.Trigger.VectorIcon family={Ionicons} name="bookmark-outline" />}
        />
        <NativeTabs.Trigger.Label>Desk</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="feed">
        <NativeTabs.Trigger.Icon
          sf="newspaper"
          src={<NativeTabs.Trigger.VectorIcon family={Ionicons} name="newspaper-outline" />}
        />
        <NativeTabs.Trigger.Label>Feed</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="capture">
        <NativeTabs.Trigger.Icon
          sf="plus.circle"
          src={<NativeTabs.Trigger.VectorIcon family={Ionicons} name="add-circle-outline" />}
        />
        <NativeTabs.Trigger.Label>Capture</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="settings">
        <NativeTabs.Trigger.Icon
          sf="gearshape"
          src={<NativeTabs.Trigger.VectorIcon family={Ionicons} name="settings-outline" />}
        />
        <NativeTabs.Trigger.Label>Settings</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
    </NativeTabs>
  );
}
