# Mobile Design Pass — Design

Date: 2026-07-07 · Status: Approved (user OK with a new native build; no OTA constraint) · Authority: `docs/design/README.md` + the shipped web app.

## Goal

Bring `apps/mobile` to the web's warm-editorial standard: real brand typography, the signature palette dots, type-aware cards, and layered canvas/paper surfaces — a "wow" first screen instead of flat rows.

## Moves

1. **Typography (expo-font + @expo-google-fonts):** Newsreader italic (titles/quotes), Instrument Sans (UI/body), JetBrains Mono (metadata/labels). Loaded in root layout with a paper splash-hold until ready. `theme.ts` fonts map to the real family names; a `type` scale is added (title 27, cardTitle 17.5, body 15.5, meta 10.5 mono, kicker 10 mono +0.8 tracking).
2. **Item model:** mobile `Item` gains `palette?: string[]`, `leadImageUrl?: string`, `tags?: string[]`, `userTags?: string[]` (API already sends them).
3. **Cards (`components/ItemCard.tsx`, replaces ItemRow):** paper surface, 11px radius, hairline border, slight shadow. Per type: **article/product/book/recipe** — 84px gradient hero (`expo-linear-gradient`, from item palette[0]→[1], fallback per-type gradients from the design doc) or the lead image when present (bearer headers for instance `/assets/` URLs) + Newsreader title + 2-line summary; **quote** — ink `#1C1A16` card, gold 34px `"` glyph, italic serif text, no hero; **note** — `#FBF4D8` surface, serif body excerpt; **image** — tall lead image with palette strip footer; **video/tweet** — dark thumb wash / plain text treatment. Every card ends with: **palette dots** (9px circles, inset hairline ring, max 5) + mono meta (`ARTICLE · danluu.com`); pending → cobalt "ENRICHING…" with a soft opacity pulse (Animated loop; respect `AccessibilityInfo.isReduceMotionEnabled`).
4. **Library:** 2px terracotta top hairline; header "The Mind" (Newsreader 27) + mono subline `«n» gatherings · organised by the machine`; canvas background; card list spacing 14; pull-to-refresh tint cobalt.
5. **Detail:** palette dot row under the kicker; quote items get the ink+gold treatment; hero wash for imageful types; Newsreader/mono/serif applied; "Open original ↗" as a cobalt pill.
6. **Capture & Settings:** canvas bg, paper input cards, mono uppercase labels, cobalt focus border, primary buttons with press-scale (0.97) micro-interaction; success states styled like web ("Saved ✓" cobalt).
7. **Tab bar:** paper bg (exists), active cobalt (exists), add hairline top border + Instrument Sans labels.

## New dependencies (all standard Expo)

`expo-font`, `expo-linear-gradient`, `@expo-google-fonts/newsreader`, `@expo-google-fonts/instrument-sans`, `@expo-google-fonts/jetbrains-mono`. Native module (`expo-linear-gradient`) ⇒ requires a new binary (build 3) — accepted by user.

## Verification

`tsc --noEmit` + `expo export --platform web`; visual pass on the iOS simulator (screenshots of Library/Detail/Capture/Settings vs the web) before building; then local EAS build → user submits build 3; subsequent tweaks via OTA (runtime version unchanged only if... note: adding native modules bumps the fingerprint — OTA updates after build 3 target build 3+).

## Out of scope

Masonry/two-column layout, blur effects, haptics, dark mode, Android-specific polish.
