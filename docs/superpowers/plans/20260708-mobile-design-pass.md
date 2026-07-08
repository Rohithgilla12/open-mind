# Mobile Design Pass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring `apps/mobile` to the web's warm-editorial standard — brand fonts, palette dots, type-aware cards, layered surfaces — verified visually on the iOS simulator, ending in a new native build (build 3).

**Architecture:** Fonts via `expo-font` + `@expo-google-fonts/*` loaded in the root layout (splash held on paper until ready); `theme.ts` gains real family names + a type scale. `ItemCard` replaces `ItemRow` with per-card-type treatments (gradient/lead-image heroes via `expo-linear-gradient`, quote/note/image specials) and the signature palette-dots + mono-meta footer. Library/Detail/Capture/Settings restyled per the spec. One new native module (`expo-linear-gradient`) ⇒ binary bump accepted.

**Tech Stack:** Expo SDK 57, expo-font, expo-linear-gradient, @expo-google-fonts/{newsreader,instrument-sans,jetbrains-mono}, existing app structure.

Spec (authoritative, follow its Moves exactly): `docs/superpowers/specs/20260707-mobile-design-pass.md`. Design authority: `docs/design/README.md`.

## Global Constraints

- Standalone app: work only in `apps/mobile`; npm-style installs via `./node_modules/.bin/expo install <pkg>` (bare `npx` misresolves in this shell).
- Colours/typography from `lib/theme.ts` only — extend it (type scale, per-type gradient fallbacks) rather than inlining values in components. Newsreader **italic** for titles/quotes; Instrument Sans UI/body; JetBrains Mono metadata (uppercase, tracked).
- Palette dots: 9px circles, inset hairline ring, max 5 — the signature; on every card footer and the detail screen.
- Respect `AccessibilityInfo.isReduceMotionEnabled` for the enriching pulse and press-scale.
- Token never logged; lead-image fetches to the instance's `/assets/` need the Bearer header (RN `Image` accepts `source={{uri, headers}}`).
- Gates per task: `./node_modules/.bin/tsc --noEmit` clean; final task adds `expo export --platform web` + simulator screenshots.
- No banner comments. Keep `ItemRow` deleted (not deprecated) once `ItemCard` lands — single card component.

---

### Task 1: Fonts + theme foundation

**Files:** `apps/mobile/package.json` (deps), `apps/mobile/app/_layout.tsx` (font loading + splash hold), `apps/mobile/lib/theme.ts` (families, type scale, per-type gradients)

- [ ] `./node_modules/.bin/expo install expo-font expo-linear-gradient expo-splash-screen @expo-google-fonts/newsreader @expo-google-fonts/instrument-sans @expo-google-fonts/jetbrains-mono`
- [ ] Root layout: `useFonts` with `Newsreader_500Medium_Italic`, `Newsreader_600SemiBold_Italic`, `InstrumentSans_400Regular`, `InstrumentSans_500Medium`, `InstrumentSans_600SemiBold`, `JetBrainsMono_400Regular`, `JetBrainsMono_500Medium` (verify exact export names in each package's index.d.ts); `SplashScreen.preventAutoHideAsync()` at module scope, hide when loaded; render nothing (or paper view) until ready.
- [ ] `theme.ts`: `fonts` → real family names (`serif: "Newsreader_500Medium_Italic"`, `serifBold: "Newsreader_600SemiBold_Italic"`, `sans`/`sansMedium`/`sansSemiBold`, `mono`/`monoMedium`); add `type` scale per spec Move 1 (title 27, cardTitle 17.5, body 15.5, meta 10.5, kicker 10 with letterSpacing 0.8); add `typeGradients: Record<cardType, [string, string]>` fallbacks derived from `docs/design/README.md` (read it — it defines per-type gradient hues; pick tasteful two-stop warm gradients per type, e.g. article cobalt-tinted, recipe green-tinted, product terracotta, book gold; document each choice inline).
- [ ] Existing screens still compile with renamed font keys (update usages: `fonts.serif` stays valid, `fonts.mono` stays valid — keep key names stable so only values change; verify by grep).
- [ ] Gate: tsc clean. Commit `feat(mobile): brand fonts + type scale + gradient tokens`.

### Task 2: Item model + ItemCard (replaces ItemRow)

**Files:** `apps/mobile/lib/api.ts` (Item fields), `apps/mobile/components/ItemCard.tsx` (new), delete `apps/mobile/components/ItemRow.tsx`, `apps/mobile/app/(tabs)/index.tsx` (use ItemCard, Library restyle per Move 4)

- [ ] `Item` gains `palette?: string[]`, `leadImageUrl?: string`, `tags?: string[]`, `userTags?: string[]`.
- [ ] `ItemCard` per spec Move 3 — all treatments: gradient/lead-image hero types (article/product/book/recipe: 84px hero, `LinearGradient` from `palette[0]→[1]` else `typeGradients[type]`, lead image with bearer headers when present and it's an instance asset — absolute instance URL needs `settings.instanceUrl` prefix when `leadImageUrl` starts with `/assets/`); quote (ink card, gold 34px `"`, italic serif, no hero); note (`#FBF4D8` — add as `colors.note` token); image (tall image + palette strip footer); video/tweet dark-wash/plain. Footer: palette dots (max 5) + mono meta `ARTICLE · danluu.com`; pending → cobalt `ENRICHING…` with opacity pulse (Animated.loop, skipped under reduce-motion).
- [ ] Library screen: terracotta 2px top hairline, "The Mind" Newsreader header + mono subline with the count, canvas bg, 14 spacing, cobalt refresh tint.
- [ ] Gate: tsc clean. Commit `feat(mobile): type-aware ItemCard with palette dots + The Mind library`.

### Task 3: Detail, Capture, Settings, tab bar polish

**Files:** `apps/mobile/app/item/[id].tsx`, `apps/mobile/app/(tabs)/capture.tsx`, `apps/mobile/app/(tabs)/settings.tsx`, `apps/mobile/app/link.tsx` (same input/button styling), `apps/mobile/app/(tabs)/_layout.tsx`

- [ ] Detail per Move 5: palette dot row under kicker; quote treatment; hero wash for imageful types; fonts applied; "Open original ↗" cobalt pill.
- [ ] Capture/Settings/link per Move 6: canvas bg, paper input cards, mono uppercase labels, cobalt focus border (onFocus/onBlur state), press-scale 0.97 on primary buttons (reduce-motion aware), web-style success states.
- [ ] Tab bar per Move 7: hairline top border + Instrument Sans labels.
- [ ] Gate: tsc clean. Commit `feat(mobile): detail/capture/settings warm-editorial polish`.

### Task 4: Verify on simulator + build 3 (controller-run)

- [ ] `./node_modules/.bin/tsc --noEmit` + `./node_modules/.bin/expo export --platform web` green.
- [ ] Run on the booted iPhone sim (`./node_modules/.bin/expo run:ios --device <udid>`); screenshot Library / a detail / Capture / Settings; compare against the web + design doc; fix visual misses.
- [ ] Local EAS build: `command npx --yes eas-cli build --platform ios --profile production --local --non-interactive` → build 3 ipa (native module added ⇒ new fingerprint; note in TODO that post-build-3 OTAs target it).
- [ ] TODO.md: design pass → Done (dated, evidence); user submits build 3.
