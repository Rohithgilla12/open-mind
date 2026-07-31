import { defineConfig } from "wxt";

// See https://wxt.dev/api/config.html
export default defineConfig({
  modules: ["@wxt-dev/module-react"],
  manifest: ({ manifestVersion }) => ({
    // The qualifier is deliberate: the name sweep flagged a robotics "OpenMind"
    // with a pending USPTO application, so the store listing never ships the
    // bare word alone.
    name: "Openmind — the self-hosted commonplace book",
    description:
      "Save pages, quotes, and images to your own Openmind instance in one click, then tag them without leaving the page.",
    // Host access is optional and requested per instance origin at runtime
    // (see lib/permissions.ts), so an install asks for nothing beyond the one
    // server the user configures. MV2 has no optional_host_permissions — there
    // host patterns belong in optional_permissions alongside the API ones.
    ...(manifestVersion === 3
      ? {
          optional_host_permissions: ["https://*/*", "http://*/*"],
          permissions: [
            "storage",
            "activeTab",
            "contextMenus",
            "notifications",
          ],
        }
      : {
          optional_permissions: ["https://*/*", "http://*/*"],
          permissions: [
            "storage",
            "activeTab",
            "contextMenus",
            "notifications",
          ],
        }),
    commands: {
      "save-page": {
        suggested_key: {
          default: "Ctrl+Shift+S",
          mac: "Command+Shift+S",
        },
        description: "Save current page to Openmind",
      },
    },
  }),
});
