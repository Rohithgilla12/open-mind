import { Ionicons } from "@expo/vector-icons";
import { Tabs } from "expo-router";
import { fonts, colors } from "@/lib/theme";

export default function TabsLayout() {
  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarActiveTintColor: colors.cobalt,
        tabBarInactiveTintColor: colors.inkFaint,
        tabBarStyle: {
          backgroundColor: colors.paper,
          borderTopWidth: 1,
          borderTopColor: colors.hairline,
        },
        tabBarLabelStyle: {
          fontFamily: fonts.sans,
          fontSize: 11,
        },
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: "Library",
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="albums-outline" color={color} size={size} />
          ),
        }}
      />
      <Tabs.Screen
        name="capture"
        options={{
          title: "Capture",
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="add-circle-outline" color={color} size={size} />
          ),
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          title: "Settings",
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="settings-outline" color={color} size={size} />
          ),
        }}
      />
    </Tabs>
  );
}
