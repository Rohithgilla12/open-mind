import { chromium } from "playwright";
const b = await chromium.launch({ executablePath: process.env.CHROME_PATH || undefined });
const c = await b.newContext({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 });
const p = await c.newPage();
await p.goto("http://127.0.0.1:3999/welcome", { waitUntil: "networkidle" });
await p.waitForTimeout(1500);
await p.screenshot({ path: "out/landing-full.png", fullPage: true });
await p.screenshot({ path: "out/landing-hero.png" });
// narrow viewport, to prove the responsive collapse works
await p.setViewportSize({ width: 430, height: 900 });
await p.waitForTimeout(600);
await p.screenshot({ path: "out/landing-mobile.png", fullPage: true });
await b.close();
console.log("ok");
