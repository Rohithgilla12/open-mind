// Shared-element card→detail morph. A card reports its hero rect on press; this
// overlay springs the hero (lead image if present, else the palette gradient)
// from that rect into the detail-hero position while the destination screen
// fades in underneath, then cross-fades out onto the real hero.
//
// Reduce-motion is a best-effort gate: the OS setting is read async on mount,
// so a tap in the first frames may still animate (navigation is unaffected —
// begin() only drives the cosmetic overlay; callers push regardless).
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { AccessibilityInfo, Dimensions, Image, StyleSheet } from "react-native";
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

/** What the overlay paints: the palette gradient, plus the lead image when the
 * card shows one (so the flying panel matches what the user actually tapped). */
export type MorphHero = {
  colors: [string, string];
  image?: { uri: string; headers?: Record<string, string> };
};

type MorphState = { from: MorphRect; hero: MorphHero } | null;

type MorphContextValue = {
  /** Kick off the morph from a measured card-hero rect. No-op under reduce-motion. */
  begin: (from: MorphRect, hero: MorphHero) => void;
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
// Detail-screen topbar (item/[id].tsx styles.topbar): paddingVertical spacing.md
// around a ~20pt row.
const TOPBAR_H = spacing.md * 2 + 20;
const HERO_W = SCREEN_W - spacing.xl * 2;
// Two detail hero shapes (item/[id].tsx): styles.hero is a fixed-height 120 wash
// (no lead image); styles.heroImageWrap is a full-width 4:3 image (has one).
const GRADIENT_HERO_H = 120;
const IMAGE_HERO_H = (HERO_W * 3) / 4;

export function MorphProvider({ children }: { children: React.ReactNode }) {
  const insets = useSafeAreaInsets();
  const [state, setState] = useState<MorphState>(null);
  const reduceMotion = useRef(false);
  // progress 0 = at the card hero rect, 1 = at the detail hero.
  const progress = useSharedValue(0);
  const opacity = useSharedValue(1);

  useEffect(() => {
    AccessibilityInfo.isReduceMotionEnabled()
      .then((r) => {
        reduceMotion.current = r;
      })
      .catch((err) => {
        // Fail safe: if the setting can't be read, suppress motion rather than
        // assume it's fine to animate.
        console.warn("[morph] reduce-motion check failed", err);
        reduceMotion.current = true;
      });
  }, []);

  const clear = useCallback(() => setState(null), []);

  const begin = useCallback(
    (from: MorphRect, hero: MorphHero) => {
      if (reduceMotion.current) return;
      if (from.width <= 0 || from.height <= 0) return; // pre-layout / detached
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
    const th = state?.hero.image ? IMAGE_HERO_H : GRADIENT_HERO_H;
    const p = progress.value;
    return {
      opacity: opacity.value,
      left: from.x + (tx - from.x) * p,
      top: from.y + (ty - from.y) * p,
      width: from.width + (HERO_W - from.width) * p,
      height: from.height + (th - from.height) * p,
    };
  });

  const value = useMemo<MorphContextValue>(() => ({ begin }), [begin]);

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
          {state.hero.image ? (
            <Image source={state.hero.image} style={StyleSheet.absoluteFill} resizeMode="cover" />
          ) : null}
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
