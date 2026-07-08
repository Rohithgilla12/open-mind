// Shared press-scale micro-interaction for primary buttons (Move 6). Scales to
// 0.97 on press, respecting AccessibilityInfo.isReduceMotionEnabled — mirrors
// the reduce-motion pattern used by ItemCard's EnrichingLabel pulse.
import { useEffect, useRef, useState } from "react";
import { AccessibilityInfo, Animated, type GestureResponderEvent, Pressable } from "react-native";

type PressScaleProps = {
  onPress: () => void;
  disabled?: boolean;
  children: React.ReactNode;
  style?: object | object[];
};

export function PressScale({ onPress, disabled, children, style }: PressScaleProps) {
  const scale = useRef(new Animated.Value(1)).current;
  const [reduceMotion, setReduceMotion] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void AccessibilityInfo.isReduceMotionEnabled().then((reduced) => {
      if (!cancelled) setReduceMotion(reduced);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  function animateTo(value: number) {
    if (reduceMotion) return;
    Animated.timing(scale, { toValue: value, duration: 100, useNativeDriver: true }).start();
  }

  function handlePressIn(_e: GestureResponderEvent) {
    animateTo(0.97);
  }
  function handlePressOut(_e: GestureResponderEvent) {
    animateTo(1);
  }

  return (
    <Animated.View style={[{ transform: [{ scale }] }, style]}>
      <Pressable onPress={onPress} onPressIn={handlePressIn} onPressOut={handlePressOut} disabled={disabled}>
        {children}
      </Pressable>
    </Animated.View>
  );
}
