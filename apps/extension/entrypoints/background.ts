import { browser } from "wxt/browser";
import { saveItem } from "../lib/save";
import { clampNote } from "../lib/format";

/** Show text in the action badge, then clear it after 2 seconds. */
async function flashBadge(text: string): Promise<void> {
  await browser.action.setBadgeText({ text });
  setTimeout(() => {
    void browser.action.setBadgeText({ text: "" });
  }, 2000);
}

export default defineBackground(() => {
  browser.runtime.onInstalled.addListener(async () => {
    await browser.contextMenus.removeAll();
    browser.contextMenus.create({
      id: "om-selection",
      title: "Save selection to Openmind",
      contexts: ["selection"],
    });
    browser.contextMenus.create({
      id: "om-image",
      title: "Save image to Openmind",
      contexts: ["image"],
    });
  });

  browser.commands.onCommand.addListener(async (command) => {
    if (command !== "save-page") return;
    const [active] = await browser.tabs.query({
      active: true,
      currentWindow: true,
    });
    if (!active?.url) {
      await flashBadge("!");
      return;
    }
    const res = await saveItem({ url: active.url });
    await flashBadge(res.ok ? "✓" : "!");
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
        message:
          res.status === 401
            ? "Token invalid — open extension settings."
            : res.status === 0
              ? "Instance unreachable — check the extension settings."
              : `Error ${res.status}`,
      });
    }
  });
});
