import { defineConfig } from "wxt";

// See https://wxt.dev/api/config.html
export default defineConfig({
  modules: ["@wxt-dev/module-react"],
  manifest: {
    name: "Openmind",
    permissions: ["storage", "activeTab", "contextMenus", "notifications"],
    // Broad host permissions for now; narrow-origin optional permissions are
    // deferred (see README).
    host_permissions: ["https://*/*", "http://localhost/*"],
  },
});
