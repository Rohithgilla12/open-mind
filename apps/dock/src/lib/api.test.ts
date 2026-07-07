import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { checkToken, claimDeviceCode, saveItem, searchItems } from "./api";
import type { Settings } from "./settings";

const settings: Settings = { instanceUrl: "https://openmind.example.com", token: "secret-tok" };

const getSettingsMock = vi.fn<() => Promise<Settings | null>>();
vi.mock("./settings", () => ({
  getSettings: () => getSettingsMock(),
}));

describe("api client", () => {
  beforeEach(() => {
    getSettingsMock.mockReset();
    getSettingsMock.mockResolvedValue(settings);
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe("checkToken", () => {
    it("sends the Bearer header and returns the status", async () => {
      vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 200 }));
      const status = await checkToken();
      expect(status).toBe(200);
      const [url, init] = vi.mocked(fetch).mock.calls[0];
      expect(url).toBe("https://openmind.example.com/api/auth/check");
      expect((init?.headers as Record<string, string>).Authorization).toBe("Bearer secret-tok");
    });

    it("passes through a 401", async () => {
      vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 401 }));
      expect(await checkToken()).toBe(401);
    });

    it("maps a network failure to status 0", async () => {
      vi.mocked(fetch).mockRejectedValueOnce(new Error("network down"));
      expect(await checkToken()).toBe(0);
    });

    it("uses the override settings instead of stored settings", async () => {
      vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 200 }));
      await checkToken({ instanceUrl: "https://other.example.com", token: "other-tok" });
      expect(getSettingsMock).not.toHaveBeenCalled();
      const [url, init] = vi.mocked(fetch).mock.calls[0];
      expect(url).toBe("https://other.example.com/api/auth/check");
      expect((init?.headers as Record<string, string>).Authorization).toBe("Bearer other-tok");
    });

    it("returns 0 when no settings are configured", async () => {
      getSettingsMock.mockResolvedValueOnce(null);
      expect(await checkToken()).toBe(0);
      expect(fetch).not.toHaveBeenCalled();
    });
  });

  describe("saveItem", () => {
    it("posts JSON and parses a 201 body", async () => {
      const item = { id: "1", url: "https://a.com", status: "pending" };
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(item), { status: 201 }));
      const result = await saveItem({ url: "https://a.com" });
      expect(result).toEqual({ ok: true, status: 201, item });
      const [url, init] = vi.mocked(fetch).mock.calls[0];
      expect(url).toBe("https://openmind.example.com/api/items");
      expect(init?.method).toBe("POST");
      expect(JSON.parse(init?.body as string)).toEqual({ url: "https://a.com" });
    });

    it("maps a network failure to status 0", async () => {
      vi.mocked(fetch).mockRejectedValueOnce(new Error("offline"));
      expect(await saveItem({ note: "hello" })).toEqual({ ok: false, status: 0 });
    });
  });

  describe("searchItems", () => {
    it("parses results and the understood echo", async () => {
      const body = { results: [{ item: { id: "1", url: "https://a.com", status: "done" }, score: 0.9 }], understood: "recipes" };
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(body), { status: 200 }));
      const result = await searchItems("recipe");
      expect(result.ok).toBe(true);
      expect(result.results).toHaveLength(1);
      expect(result.understood).toBe("recipes");
      const [url] = vi.mocked(fetch).mock.calls[0];
      expect(url).toBe("https://openmind.example.com/api/search?q=recipe&parse=true");
    });

    it("url-encodes the query", async () => {
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify({ results: [] }), { status: 200 }));
      await searchItems("a b&c");
      const [url] = vi.mocked(fetch).mock.calls[0];
      expect(url).toBe("https://openmind.example.com/api/search?q=a%20b%26c&parse=true");
    });

    it("maps a network failure to status 0 with empty results", async () => {
      vi.mocked(fetch).mockRejectedValueOnce(new Error("offline"));
      expect(await searchItems("x")).toEqual({ ok: false, status: 0, results: [] });
    });

    it("treats a non-ok response as empty results", async () => {
      vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 401 }));
      expect(await searchItems("x")).toEqual({ ok: false, status: 401, results: [] });
    });
  });

  describe("claimDeviceCode", () => {
    it("posts the normalised code and device name with no auth header", async () => {
      const body = { key: "omk_abc123", name: "Mac dock" };
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(body), { status: 201 }));
      const result = await claimDeviceCode("https://openmind.example.com", "abcd efgh", "Mac dock");
      expect(result).toEqual({ ok: true, status: 201, key: "omk_abc123", name: "Mac dock" });
      const [url, init] = vi.mocked(fetch).mock.calls[0];
      expect(url).toBe("https://openmind.example.com/api/device-links/claim");
      expect(init?.method).toBe("POST");
      expect(JSON.parse(init?.body as string)).toEqual({ code: "ABCD-EFGH", deviceName: "Mac dock" });
      expect((init?.headers as Record<string, string>).Authorization).toBeUndefined();
      expect(getSettingsMock).not.toHaveBeenCalled();
    });

    it("normalises a code that already has a dash and mixed case", async () => {
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify({ key: "k", name: "n" }), { status: 201 }));
      await claimDeviceCode("https://openmind.example.com", "aBcd-eFgH", "Mac dock");
      const [, init] = vi.mocked(fetch).mock.calls[0];
      expect(JSON.parse(init?.body as string).code).toBe("ABCD-EFGH");
    });

    it("returns ok:false on a 404 (unknown, expired, or used code)", async () => {
      vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 404 }));
      expect(await claimDeviceCode("https://openmind.example.com", "ABCD-EFGH", "Mac dock")).toEqual({
        ok: false,
        status: 404,
      });
    });

    it("maps a network failure to status 0", async () => {
      vi.mocked(fetch).mockRejectedValueOnce(new Error("offline"));
      expect(await claimDeviceCode("https://openmind.example.com", "ABCD-EFGH", "Mac dock")).toEqual({
        ok: false,
        status: 0,
      });
    });
  });
});
