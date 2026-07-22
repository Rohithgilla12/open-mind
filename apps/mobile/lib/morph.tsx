// Shared-element card→detail morph. A card reports its hero rect on press; this
// overlay springs a gradient hero panel from that rect into the detail-hero
// position while the destination screen fades in underneath, then cross-fades
// out onto the real hero. Reduce-motion callers skip begin() and just navigate.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { AccessibilityInfo, Dimensions, StyleSheet } from "react-native";
import Animated, {
  Easing,
  runOnJS,
  useAnimatedStyle,
  useSharedValue,
  withSpring,
  withTiming,
} from "react-native-reanimated";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { LinearGradient } from "expo-linear-gradient";
import { colors, radius, spacing } from "./theme";

export type MorphRect = { x: number; y: number; width: number; height: number };

/** What the overlay needs to paint the flying hero. */
export type MorphHero = { colors: [string, string] };

type MorphState = { from: MorphRect; hero: MorphHero } | null;

type MorphContextValue = {
  /** Kick off the morph from a measured card-hero rect. No-op under reduce-motion. */
  begin: (from: MorphRect, hero: MorphHero) => void;
  enabled: boolean;
};

const MorphContext = createContext<MorphContextValue | null>(null);

export function useMorph(): MorphContextValue {
  const ctx = useContext(MorphContext);
  if (!ctx) throw new Error("useMorph must be used within a MorphProvider");
  return ctx;
}

// Legible, cinematic arc (~500ms) with a hair of overshoot (ζ≈0.87) — snappier
// springs read as an instant cut rather than a morph.
const SPRING = { mass: 1, damping: 17, stiffness: 95 };
const SCREEN_W = Dimensions.get("window").width;
// Detail-screen topbar: paddingVertical spacing.md around a ~20pt row.
const TOPBAR_H = spacing.md * 2 + 20;
// Detail hero (item/[id].tsx styles.hero): inset by spacing.xl, height 120.
const HERO_H = 120;

export function MorphProvider({ children }: { children: React.ReactNode }) {
  const insets = useSafeAreaInsets();
  const [state, setState] = useState<MorphState>(null);
  const reduceMotion = useRef(false);
  // progress 0 = at the card hero rect, 1 = at the detail hero.
  const progress = useSharedValue(0);
  const opacity = useSharedValue(1);

  useEffect(() => {
    void AccessibilityInfo.isReduceMotionEnabled().then((r) => {
      reduceMotion.current = r;
    });
  }, []);

  const clear = useCallback(() => setState(null), []);

  const begin = useCallback(
    (from: MorphRect, hero: MorphHero) => {
      if (reduceMotion.current) return;
      progress.value = 0;
      opacity.value = 1;
      setState({ from, hero });
      progress.value = withSpring(1, SPRING, (finished) => {
        if (!finished) return;
        opacity.value = withTiming(0, { duration: 150, easing: Easing.out(Easing.quad) }, (done) => {
          if (done) runOnJS(clear)();
        });
      });
    },
    [progress, opacity, clear],
  );

  const overlayStyle = useAnimatedStyle(() => {
    const from = state?.from;
    if (!from) return { opacity: 0 };
    const tx = spacing.xl;
    const ty = insets.top + TOPBAR_H;
    const tw = SCREEN_W - spacing.xl * 2;
    const p = progress.value;
    return {
      opacity: opacity.value,
      left: from.x + (tx - from.x) * p,
      top: from.y + (ty - from.y) * p,
      width: from.width + (tw - from.width) * p,
      height: from.height + (HERO_H - from.height) * p,
    };
  });

  const value = useMemo<MorphContextValue>(() => ({ begin, enabled: true }), [begin]);

  return (
    <MorphContext.Provider value={value}>
      {children}
      {state ? (
        <Animated.View pointerEvents="none" style={[styles.overlay, overlayStyle]}>
          <LinearGradient
            colors={state.hero.colors}
            start={{ x: 0, y: 0 }}
            end={{ x: 0, y: 1 }}
            style={StyleSheet.absoluteFill}
          />
        </Animated.View>
      ) : null}
    </MorphContext.Provider>
  );
}

const styles = StyleSheet.create({
  overlay: {
    position: "absolute",
    overflow: "hidden",
    borderRadius: radius.card,
    backgroundColor: colors.cardSurface,
    zIndex: 1000,
    elevation: 1000,
  },
});
