# WXT Capture Extension Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Browser extension (WXT, MV3) that saves the current page, a text selection, or an image to Openmind via the web app's `/api/*` routes with a Bearer token; plus the two enabling backend/web changes.

**Architecture:** Web routes gain Bearer-header support + `GET /api/auth/check`; pipeline gains an image-URL branch; `apps/extension` becomes a real WXT workspace member with options page, popup, and context menus. Spec: `docs/superpowers/specs/20260703-wxt-extension-design.md`.

**Tech Stack:** WXT (latest), React 19, strict TS, `browser.storage.local`; no other new deps.

## Global Constraints

- Extension is a thin client: POSTs only, no enrichment logic. Data flows through `{instanceUrl}/api/items` and `{instanceUrl}/api/auth/check` with `Authorization: Bearer <token>`.
- Token stored only in `browser.storage.local`; never logged.
- Design tokens from `@openmind/ui` — no hardcoded colours (surface/danger/paper/ink/cobalt/line exist).
- Image-URL detection: extensions png/jpg/jpeg/gif/webp/avif (query-tolerant) or `Content-Type: image/*`; fetches use the existing SSRF-safe client. cardType `image`, `leadImageUrl` = url, title = filename stem, idempotent.
- `/api/` paths never redirect to /login — JSON 401 instead.
- Selection saves format: `<selection text>\n\n— <page URL>` as a note (10000-rune cap applies — truncate selection to fit with the source line intact).
- Strict TS; `pnpm turbo run build/lint` green including the new workspace; go suite `go test -p 1 ./...` green.
- Commit after every task; never amend/force-push. No banner comments; Go errors `%w`.

---

### Task 1: Web — Bearer support + /api/auth/check + middleware fix

**Files:**
- Modify: `apps/web/lib/api.ts`, `apps/web/middleware.ts`, `apps/web/app/api/items/route.ts`
- Create: `apps/web/app/api/auth/check/route.ts`

**Interfaces:**
- Produces: `apiFetch(path, init?, req?: Request)` — optional third param; when provided and it carries `Authorization: Bearer`, that token wins over the cookie. `GET /api/auth/check` → 200 `{ok:true}` / 401 / 429 / 502 JSON. All `/api/*` paths excluded from login redirects.

- [ ] **Step 1: lib/api.ts** — add optional `req` param:

```ts
export async function apiFetch(path: string, init?: RequestInit, req?: Request): Promise<Response> {
  let token = (await cookies()).get("om_token")?.value;
  const header = req?.headers.get("authorization");
  if (header?.startsWith("Bearer ")) token = header.slice(7);
  const headers = new Headers(init?.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (init?.body) headers.set("content-type", "application/json");
  return fetch(`${API_URL}${path}`, { ...init, headers, cache: "no-store" });
}
```

- [ ] **Step 2: items route passes the request** — `apiFetch("/items", { method: "POST", body }, req)`.

- [ ] **Step 3: check route** `apps/web/app/api/auth/check/route.ts`:

```ts
import { NextResponse } from "next/server";
import { apiFetch } from "../../../../lib/api";

export async function GET(req: Request) {
  try {
    const res = await apiFetch("/items?limit=1", undefined, req);
    if (res.ok) return NextResponse.json({ ok: true });
    if (res.status === 429) return NextResponse.json({ error: "rate limited" }, { status: 429 });
    return NextResponse.json({ error: "invalid token" }, { status: 401 });
  } catch {
    return NextResponse.json({ error: "api unreachable" }, { status: 502 });
  }
}
```

- [ ] **Step 4: middleware** — change the redirect guard: if `pathname.startsWith("/api/")` return `NextResponse.next()` before the cookie checks (route handlers do their own auth). Keep matcher as-is otherwise.

- [ ] **Step 5: Verify + commit**

Run: `pnpm turbo run build --filter=web && pnpm turbo run lint --filter=web` → PASS.
```bash
git add apps/web && git commit -m "feat(web): bearer support on api routes + auth check endpoint"
```

---

### Task 2: Backend — image-URL pipeline branch

**Files:**
- Modify: `apps/api/internal/enrich/pipeline.go`
- Create: `apps/api/internal/enrich/imageurl.go`
- Test: `apps/api/internal/enrich/imageurl_test.go`, extend `pipeline_test.go`

**Interfaces:**
- Produces: `isImageURL(ctx, client, url) (bool, error)` (unexported): extension check first (no network), else HEAD via the pipeline's safe client checking `Content-Type: image/*` (HEAD failure → false, nil — fall through to normal extraction). `imageTitle(url) string` = filename stem, decoded. Pipeline branch: image URL → skip extractor, `UpdateItemExtraction{Title: imageTitle, Body: "", LeadImageUrl: url, CardType: "image"}` → `enrichText(title, title)`.

- [ ] **Step 1: Failing tests** — table-driven `TestIsImageURLByExtension` (png/jpg/webp with query strings → true offline; .html → needs sniff), `TestImageTitle` (`https://cdn.x.com/pics/sunset-beach.jpg?w=200` → `sunset-beach`), and pipeline test `TestPipelineImageURLSkipsExtraction`: httptest server serving `Content-Type: image/png` on a `/photo` path (no extension → exercises the sniff), failing extractor injected, assert cardType image, leadImageUrl set, status enriched, idempotent second run.

- [ ] **Step 2: RED → implement → GREEN.** HEAD request must use the Pipeline's extractor-independent safe client: give Pipeline an optional `HTTPClient *http.Client` field defaulting to `SafeHTTPClient(10 * time.Second)`; tests inject the httptest client.

- [ ] **Step 3: Full suite + commit**

Run: `cd apps/api && go test -p 1 ./... && go vet ./...` → PASS.
```bash
git add apps/api && git commit -m "feat(enrich): image-url saves skip extraction, render as image cards"
```

---

### Task 3: Extension scaffold + options page

**Files:**
- Create: `apps/extension/package.json`, `wxt.config.ts`, `tsconfig.json`, `entrypoints/options/` (index.html, main.tsx, Options.tsx), `lib/storage.ts`, `lib/save.ts`
- Modify: `pnpm-workspace.yaml` already covers apps/*? CHECK — it lists `apps/web` + `packages/*` explicitly; ADD `apps/extension`.

**Interfaces:**
- Produces: `lib/storage.ts`: `getSettings(): Promise<{instanceUrl: string; token: string}>`, `setSettings(s)` over `browser.storage.local` (default instanceUrl `https://openmind.gilla.fun`). `lib/save.ts`: `saveItem(body: {url?: string; note?: string}): Promise<{ok: boolean; status: number}>` — POSTs `{instanceUrl}/api/items` with Bearer; `checkToken(): Promise<number>` — GET `/api/auth/check` status. Task 4 consumes both.
- WXT scaffolding: `pnpm dlx wxt@latest init` is interactive — instead author files by hand: package.json with `wxt` dev dep + `react`/`react-dom` + scripts `dev`, `build` (wxt build), `build:firefox` (wxt build -b firefox), `lint` (tsc --noEmit), `test` (echo no tests); wxt.config.ts with manifest permissions `storage`, `activeTab`, `contextMenus`, `notifications`, `host_permissions: ["https://*/*", "http://localhost/*"]` (narrow-origin optional permissions deferred — note in README).
- Options page: two inputs + Validate button (states: idle/checking/valid/invalid/unreachable via checkToken status 200/401|429/502), tokens-styled.

- [ ] **Step 1: workspace + scaffold files; `pnpm install`**
- [ ] **Step 2: storage + save libs + options page per interfaces**
- [ ] **Step 3: Verify + commit**

Run: `pnpm --filter extension build && pnpm --filter extension lint && pnpm turbo run build` → PASS (wxt outputs `.output/chrome-mv3`).
```bash
git add -A && git commit -m "feat(extension): wxt scaffold, settings storage, options page"
```

---

### Task 4: Popup + context menus + background

**Files:**
- Create: `apps/extension/entrypoints/popup/` (index.html, main.tsx, Popup.tsx), `apps/extension/entrypoints/background.ts`
- Create: `apps/extension/README.md` (load-unpacked instructions Chrome + Firefox, token setup)

**Interfaces:**
- Consumes: `saveItem`, `getSettings` (Task 3).
- Produces: popup saves active tab URL; background registers two context menus and handles clicks.

- [ ] **Step 1: Popup.tsx** — `browser.tabs.query({active: true, currentWindow: true})` for title/URL; Save button → `saveItem({url})`; states saving/saved/error; 401 or missing token → "Open settings" link (`browser.runtime.openOptionsPage()`). Tokens styling.
- [ ] **Step 2: background.ts** —

```ts
export default defineBackground(() => {
  browser.runtime.onInstalled.addListener(() => {
    browser.contextMenus.create({ id: "om-selection", title: "Save selection to Openmind", contexts: ["selection"] });
    browser.contextMenus.create({ id: "om-image", title: "Save image to Openmind", contexts: ["image"] });
  });
  browser.contextMenus.onClicked.addListener(async (info, tab) => {
    const body =
      info.menuItemId === "om-selection" && info.selectionText
        ? { note: clampNote(info.selectionText, tab?.url) }
        : info.menuItemId === "om-image" && info.srcUrl
          ? { url: info.srcUrl }
          : null;
    if (!body) return;
    const res = await saveItem(body);
    if (res.ok) {
      await flashBadge("✓");
    } else {
      await flashBadge("!");
      browser.notifications.create({
        type: "basic",
        iconUrl: browser.runtime.getURL("/icon/128.png"),
        title: "Openmind save failed",
        message: res.status === 401 ? "Token invalid — open extension settings." : `Error ${res.status}`,
      });
    }
  });
});
```
`clampNote(text, source)`: trim, truncate text so `text + "\n\n— " + source` ≤ 10000 chars, append source line when present. `flashBadge`: set badge text 2s then clear. Provide a simple generated icon set (solid cobalt square PNGs 16/48/128 — generate with a tiny node script or a base64-embedded asset; do not copy any mymind asset).
- [ ] **Step 3: Verify + commit**

Run: `pnpm --filter extension build && pnpm --filter extension build:firefox && pnpm --filter extension lint` → PASS; confirm `.output/chrome-mv3/manifest.json` lists both menus' permissions.
```bash
git add -A && git commit -m "feat(extension): popup save-page, selection/image context menus"
```

---

### Task 5: E2E against deployment + wrap-up

**Files:**
- Modify: `TODO.md`, `docs/self-hosting.md` (extension section)

- [ ] **Step 1: API-level e2e (controller-runnable, no browser)** — with the deployed instance: `GET https://<instance>/api/auth/check` with Bearer (200 + 401 wrong-token), `POST /api/items {url}` and `{note}` with Bearer (201), confirming the extension's exact call shapes work against production. If the public hostname isn't mapped yet, run the same against `localhost:3000` on the server over ssh.
- [ ] **Step 2: Local extension smoke** — `pnpm --filter extension build`; verify manifest + load-unpacked instructions accurate (a human browser session is not scriptable here; document what the user should click to verify — options → validate → save page).
- [ ] **Step 3: Docs** — self-hosting.md gains "Browser extension" section (build, load unpacked, settings). TODO.md: extension slice → Done; prune the stale Next items (River queue, hybrid search, web quick-add, grid) that earlier milestones delivered.
- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat(extension): e2e evidence, docs, todo cleanup"
```

- [ ] **Step 5 (controller): merge to main, push, redeploy web** (rsync + `docker compose up -d --build web` on the server — api unchanged unless Task 2 shipped, then rebuild api too), re-run the server smoke.
