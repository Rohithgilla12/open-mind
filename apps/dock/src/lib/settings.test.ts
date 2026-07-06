import { beforeEach, describe, expect, it, vi } from "vitest";

const invoke = vi.fn();
vi.mock("@tauri-apps/api/core", () => ({ invoke: (...args: unknown[]) => invoke(...args) }));

import { clearSettings, getSettings, setSettings } from "./settings";

describe("settings", () => {
  beforeEach(() => {
    invoke.mockReset();
  });

  it("maps the Rust snake_case response to camelCase", async () => {
    invoke.mockResolvedValueOnce({ instance_url: "https://openmind.example.com", token: "tok" });
    const settings = await getSettings();
    expect(settings).toEqual({ instanceUrl: "https://openmind.example.com", token: "tok" });
    expect(invoke).toHaveBeenCalledWith("settings_get");
  });

  it("returns null when nothing is stored", async () => {
    invoke.mockResolvedValueOnce(null);
    expect(await getSettings()).toBeNull();
  });

  it("sends camelCase keys to settings_set", async () => {
    invoke.mockResolvedValueOnce(undefined);
    await setSettings({ instanceUrl: "https://openmind.example.com", token: "tok" });
    expect(invoke).toHaveBeenCalledWith("settings_set", {
      instanceUrl: "https://openmind.example.com",
      token: "tok",
    });
  });

  it("invokes settings_clear with no arguments", async () => {
    invoke.mockResolvedValueOnce(undefined);
    await clearSettings();
    expect(invoke).toHaveBeenCalledWith("settings_clear");
  });
});
