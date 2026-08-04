import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { checkToken, claimDeviceCode, listDesk, listRecent, saveItem, searchItems } from "./api";
import type { Settings } from "./settings";

const settings: Settings = { instanceUrl: "https://openmind.example.com", token: "secret-tok" };

const getSettingsMock = vi.fn<() => Promise<Settings | null>>();
vi.mock("@tauri-apps/plugin-http", () => ({
  // api.ts imports the plugin's fetch (CORS-free via Rust); tests keep using
  // the same global mock so every existing assertion stays valid.
  fetch: (...args: Parameters<typeof fetch>) => globalThis.fetch(...args),
}));

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
    it("parses results and the UnderstoodQuery text echo", async () => {
      const body = {
        results: [{ item: { id: "1", url: "https://a.com", status: "done" }, score: 0.9 }],
        understood: { text: "recipes", color: "green", types: ["recipe"] },
      };
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(body), { status: 200 }));
      const result = await searchItems("recipe");
      expect(result.ok).toBe(true);
      expect(result.results).toHaveLength(1);
      expect(result.understood).toBe("recipes");
      const [url] = vi.mocked(fetch).mock.calls[0];
      expect(url).toBe("https://openmind.example.com/api/search?q=recipe&parse=true");
    });

    it("accepts a legacy string understood echo", async () => {
      const body = {
        results: [],
        understood: "cabins",
      };
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(body), { status: 200 }));
      expect((await searchItems("cozy cabins")).understood).toBe("cabins");
    });

    it("never returns an object for understood (avoids React child crash)", async () => {
      const body = {
        results: [{ item: { id: "1", url: "https://a.com", status: "done" }, score: 1 }],
        understood: { text: "x", color: "#00ff00" },
      };
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(body), { status: 200 }));
      const result = await searchItems("x");
      expect(typeof result.understood === "string" || result.understood === undefined).toBe(true);
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

    it("maps a timed-out / aborted request without an external signal to status 0", async () => {
      vi.mocked(fetch).mockRejectedValueOnce(new DOMException("Aborted", "AbortError"));
      expect(await searchItems("x")).toEqual({ ok: false, status: 0, results: [] });
    });

    it("rethrows when the caller's AbortSignal is aborted", async () => {
      const controller = new AbortController();
      controller.abort();
      await expect(searchItems("x", undefined, controller.signal)).rejects.toBeTruthy();
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

  describe("listDesk", () => {
    it("GETs /api/desk with Bearer and returns items", async () => {
      const items = [{ id: "1", url: "https://a.com", status: "enriched", title: "A" }];
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(items), { status: 200 }));
      const result = await listDesk();
      expect(result).toEqual({ ok: true, status: 200, items });
      const [url, init] = vi.mocked(fetch).mock.calls[0];
      expect(url).toBe("https://openmind.example.com/api/desk");
      expect((init?.headers as Record<string, string>).Authorization).toBe("Bearer secret-tok");
    });

    it("maps a network failure to status 0", async () => {
      vi.mocked(fetch).mockRejectedValueOnce(new Error("offline"));
      expect(await listDesk()).toEqual({ ok: false, status: 0, items: [] });
    });
  });

  describe("listRecent", () => {
    it("GETs /api/items?limit= with Bearer", async () => {
      const items = [{ id: "2", url: "https://b.com", status: "enriched" }];
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(items), { status: 200 }));
      const result = await listRecent(8);
      expect(result).toEqual({ ok: true, status: 200, items });
      expect(vi.mocked(fetch).mock.calls[0][0]).toBe("https://openmind.example.com/api/items?limit=8");
    });

    it("treats a 401 as empty items", async () => {
      vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 401 }));
      expect(await listRecent(8)).toEqual({ ok: false, status: 401, items: [] });
    });

    it("reads items out of the ItemPage envelope", async () => {
      vi.mocked(fetch).mockResolvedValue(
        new Response(JSON.stringify({ items: [{ id: "1", url: "https://a.test" }], nextCursor: "abc" }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
      const res = await listRecent(8, settings);
      expect(res.ok).toBe(true);
      expect(res.items).toHaveLength(1);
    });

    it("still reads a bare array from an instance predating the envelope", async () => {
      vi.mocked(fetch).mockResolvedValue(
        new Response(JSON.stringify([{ id: "1", url: "https://a.test" }]), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
      const res = await listRecent(8, settings);
      expect(res.ok).toBe(true);
      expect(res.items).toHaveLength(1);
    });

    it("reports failure rather than an empty list when the body is unrecognised", async () => {
      vi.mocked(fetch).mockResolvedValue(
        new Response(JSON.stringify({ unexpected: true }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
      const res = await listRecent(8, settings);
      // An empty library and "the server said something we do not understand"
      // must not look identical to the caller.
      expect(res.ok).toBe(false);
      expect(res.items).toEqual([]);
    });
  });
});
