/**
 * Capture Openmind screenshots from the throwaway seeded stack.
 *
 * Shoots at 1280x800 with deviceScaleFactor 2, so the PNGs are 2560x1600 —
 * sharp enough for the README on a retina display. The store variants are
 * downscaled to an exact 1280x800 afterwards (see make_store_shots.sh).
 */

import { chromium } from "playwright";
import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const HERE = dirname(fileURLToPath(import.meta.url));
const OUT = join(HERE, "out");
const BASE = process.env.BASE_URL ?? "http://127.0.0.1:3999";
const TOKEN = process.env.OM_TOKEN ?? "screenshotstoken";

mkdirSync(OUT, { recursive: true });

// No DOM fixups needed: the sidebar used to hardcode the maintainer's name and
// a fake storage meter, which had to be rewritten before publishing. Both now
// come from GET /account, and token mode renders a neutral "Self-hosted".

const TARGETS = [
  { name: "the-mind", path: "/", desc: "The Mind — masonry library" },
  {
    name: "search-colour",
    path: "/?color=%231B3FD1",
    desc: "Colour search — cobalt",
  },
  { name: "search-text", path: "/?q=craft", desc: "Text/semantic search" },
  {
    name: "reader",
    path: "/item/a1000000-0000-0000-0000-000000000001",
    desc: "Reader / item detail",
  },
  { name: "desk", path: "/desk", desc: "Desk — pinboard" },
  { name: "drift", path: "/drift", desc: "Drift — resurfacing" },
  { name: "feed", path: "/feed", desc: "Feed river" },
  { name: "places", path: "/places", desc: "Places map", settle: 5000 },
  {
    name: "lens",
    path: "/lens/e0000000-0000-0000-0000-000000000001",
    desc: "Lens — saved query",
  },
  { name: "import", path: "/import", desc: "Import" },
  { name: "devices", path: "/settings/devices", desc: "Device keys" },
  { name: "feeds", path: "/feeds", desc: "Feed subscriptions" },
];

// Uses Playwright's own Chromium by default (`pnpm exec playwright install
// chromium`). Set CHROME_PATH to reuse a browser you already have on disk
// instead of downloading the exact build this Playwright version pins.
const browser = await chromium.launch({
  executablePath: process.env.CHROME_PATH || undefined,
});
const ctx = await browser.newContext({
  viewport: { width: 1280, height: 800 },
  deviceScaleFactor: 2,
  colorScheme: "light",
  reducedMotion: "reduce",
});
await ctx.addCookies([
  {
    name: "om_token",
    value: TOKEN,
    domain: "127.0.0.1",
    path: "/",
    httpOnly: true,
    secure: false,
    sameSite: "Lax",
  },
]);

const page = await ctx.newPage();
const results = [];

for (const t of TARGETS) {
  try {
    const res = await page.goto(BASE + t.path, {
      waitUntil: "networkidle",
      timeout: 30000,
    });
    await page.waitForTimeout(t.settle ?? 1200);
    await page.screenshot({ path: join(OUT, `${t.name}.png`) });
    results.push({ name: t.name, status: res?.status() ?? 0, ok: true });
  } catch (err) {
    results.push({ name: t.name, ok: false, error: String(err).slice(0, 120) });
  }
}

await browser.close();

for (const r of results) {
  console.log(
    `${r.ok ? "ok  " : "FAIL"} ${r.name.padEnd(16)} ${r.status ?? ""} ${r.error ?? ""}`,
  );
}
