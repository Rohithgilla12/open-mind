# Dock v1.1 — Desk/Recents + Launch at login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Empty dock panel shows Desk pins + recent items; Settings gains a Launch at login toggle.

**Architecture:** Thin web `GET /api/desk` proxy; dock client `listDesk`/`listRecent` + `mergeHomeLists` dedupe; Panel home view when query is blank; `tauri-plugin-autostart` (LaunchAgent) wired into Settings.

**Tech Stack:** Tauri v2, `@tauri-apps/plugin-autostart`, Vite/React/TS, existing `@tauri-apps/plugin-http` client, Next.js API route proxy.

## Global Constraints

- No OpenAPI/Go changes — reuse `GET /desk` and `GET /items`.
- Token never logged; all dock HTTP via timed Bearer fetch.
- Autostart default **off**; macOS uses `MacosLauncher::LaunchAgent`.
- Product vocabulary: **Desk** (not “Smart Space” / pinboard slang in UI).
- Out of scope: tray pin submenu, Drift, colour swatches, hotkey rebinding.

## File map

| File | Role |
|------|------|
| `apps/web/app/api/desk/route.ts` | Bearer/cookie pass-through to `/desk` |
| `apps/dock/src/lib/api.ts` | `listDesk`, `listRecent` |
| `apps/dock/src/lib/home-lists.ts` | `mergeHomeLists(desk, recent, caps)` |
| `apps/dock/src/lib/home-lists.test.ts` | Dedupe + caps tests |
| `apps/dock/src/lib/api.test.ts` | Client tests for desk/recent |
| `apps/dock/src/panel/Panel.tsx` | Home view + keyboard over flat list |
| `apps/dock/src/panel/SettingsView.tsx` | Launch at login toggle |
| `apps/dock/src-tauri/Cargo.toml` | `tauri-plugin-autostart` |
| `apps/dock/src-tauri/src/lib.rs` | Register autostart plugin |
| `apps/dock/src-tauri/capabilities/default.json` | Autostart permissions |
| `apps/dock/package.json` | `@tauri-apps/plugin-autostart` |
| `TODO.md` | Move slice to Done |

---

### Task 1: Web `/api/desk` proxy

**Files:**
- Create: `apps/web/app/api/desk/route.ts`
- Mirror: `apps/web/app/api/feeds/route.ts` GET pattern

**Interfaces:**
- Produces: `GET /api/desk` → upstream `GET /desk` status + JSON body

- [ ] **Step 1: Add route**

```ts
import { NextResponse } from "next/server";
import { apiFetch } from "../../../lib/api";

export async function GET(req: Request) {
  const res = await apiFetch("/desk", undefined, req);
  return new NextResponse(await res.text(), {
    status: res.status,
    headers: { "content-type": "application/json" },
  });
}
```

- [ ] **Step 2: Smoke** — `pnpm --filter web exec tsc --noEmit` (or turbo lint for web) green.

---

### Task 2: Dock API + home-list merge (TDD)

**Files:**
- Create: `apps/dock/src/lib/home-lists.ts`
- Create: `apps/dock/src/lib/home-lists.test.ts`
- Modify: `apps/dock/src/lib/api.ts`
- Modify: `apps/dock/src/lib/api.test.ts`

**Interfaces:**
- Produces:
  - `listDesk(override?: Settings, signal?: AbortSignal): Promise<{ ok: boolean; status: number; items: Item[] }>`
  - `listRecent(limit: number, override?: Settings, signal?: AbortSignal): Promise<{ ok: boolean; status: number; items: Item[] }>`
  - `mergeHomeLists(desk: Item[], recent: Item[], opts?: { deskCap?: number; recentCap?: number }): { desk: Item[]; recent: Item[] }` — default caps 5 / 8; recent excludes desk ids

- [ ] **Step 1: Failing tests** for merge (desk cap, recent cap, dedupe) and for `listDesk`/`listRecent` Bearer + status-0.

- [ ] **Step 2: Implement** `mergeHomeLists`, `listDesk` (`GET /api/desk`), `listRecent` (`GET /api/items?limit=`).

- [ ] **Step 3: `pnpm --filter dock test`** — all pass.

---

### Task 3: Panel home view

**Files:**
- Modify: `apps/dock/src/panel/Panel.tsx`

**Interfaces:**
- Consumes: `listDesk`, `listRecent`, `mergeHomeLists`
- Behaviour: when `mode === "search" && !query.trim()` and settings present, load home lists on focus / view→main; flat ↑/↓ across desk then recent; Enter/click → `openResult`.

- [ ] **Step 1: State** — `homeDesk`, `homeRecent`, `homeLoading`, `homeError`; abort on teardown; parallel fetch + merge.

- [ ] **Step 2: Render** — section headers “Desk” / “Recent”; reuse row button styles; empty both → existing calm hint.

- [ ] **Step 3: Keyboard** — when query empty, selection walks `homeDesk.concat(homeRecent)` instead of search `results`.

- [ ] **Step 4: `pnpm --filter dock lint`** green.

---

### Task 4: Launch at login

**Files:**
- Modify: `apps/dock/package.json` — add `@tauri-apps/plugin-autostart`
- Modify: `apps/dock/src-tauri/Cargo.toml` — `tauri-plugin-autostart = "2"`
- Modify: `apps/dock/src-tauri/src/lib.rs` — plugin init `MacosLauncher::LaunchAgent`
- Modify: `apps/dock/src-tauri/capabilities/default.json` — autostart permissions
- Modify: `apps/dock/src/panel/SettingsView.tsx` — toggle UI

**Interfaces:**
- Produces: Settings toggle calling `isEnabled` / `enable` / `disable` from `@tauri-apps/plugin-autostart`

- [ ] **Step 1: Add deps** (`pnpm add` in `apps/dock`, cargo dep).

- [ ] **Step 2: Register plugin + capabilities** (`autostart:allow-enable`, `allow-disable`, `allow-is-enabled`).

- [ ] **Step 3: Settings toggle** — load on mount; flip with error line on failure; default reflects OS state (usually off).

- [ ] **Step 4: `pnpm --filter dock lint`** + `cargo check` in `apps/dock/src-tauri`.

---

### Task 5: Tracker + mark done

**Files:**
- Modify: `TODO.md`
- Modify: `docs/superpowers/specs/20260715-dock-desk-autostart-design.md` — Status: Approved

- [ ] **Step 1: TODO Done entry** dated, with evidence (tests green).
- [ ] **Step 2: Spec status → Approved.**

---

## Spec coverage check

| Spec requirement | Task |
|------------------|------|
| Empty panel Desk ≤5 + Recent ≤8 deduped | 2, 3 |
| Open item / hide panel | 3 |
| Partial failure OK | 3 |
| Refresh on focus / leave Settings | 3 |
| `GET /api/desk` proxy | 1 |
| Launch at login LaunchAgent, default off | 4 |
| No tray pins / no contract change | (constraints) |
