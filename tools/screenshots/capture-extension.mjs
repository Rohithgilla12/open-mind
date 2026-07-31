import { chromium } from "playwright";
const EXT = `${process.cwd()}/../extshot/ext`;
const PROFILE = `${process.cwd()}/../extshot/profile2`;
const ctx = await chromium.launchPersistentContext(PROFILE, {
  headless: false,
  executablePath: process.env.CHROME_PATH || undefined,
  viewport: { width: 1000, height: 700 },
  deviceScaleFactor: 2,
  args: [`--disable-extensions-except=${EXT}`, `--load-extension=${EXT}`, "--no-first-run"],
});
let [sw] = ctx.serviceWorkers();
if (!sw) sw = await ctx.waitForEvent("serviceworker", { timeout: 20000 });
const id = new URL(sw.url()).host;
await sw.evaluate(async ([u, t]) => {
  await chrome.storage.local.set({ settings: { instanceUrl: u, token: t } });
}, ["http://127.0.0.1:3999", "screenshotstoken"]);

// A real article tab, which must stay the ACTIVE tab of this window so the
// popup's tabs.query({active:true,currentWindow:true}) resolves to it.
const article = ctx.pages()[0] ?? (await ctx.newPage());
await article.goto("http://127.0.0.1:3999/welcome", { waitUntil: "networkidle" });

const popup = await ctx.newPage();
await popup.setViewportSize({ width: 380, height: 640 });
await popup.goto(`chrome-extension://${id}/popup.html`, { waitUntil: "domcontentloaded" });
await article.bringToFront();          // article becomes the active tab again
await popup.waitForTimeout(2500);

const state = await popup.evaluate(() => document.body.innerText.slice(0, 120));
console.log("popup sees:", JSON.stringify(state));
await popup.screenshot({ path: "out/ext-popup2.png" });
await ctx.close();
