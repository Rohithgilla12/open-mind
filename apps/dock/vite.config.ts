import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Tauri expects a fixed dev-server port; https://v2.tauri.app/start/frontend/
export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
  },
  test: {
    environment: "node",
    include: ["src/lib/**/*.test.ts"],
  },
});
