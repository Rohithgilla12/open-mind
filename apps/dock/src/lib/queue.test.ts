import { beforeEach, describe, expect, it, vi } from "vitest";

const invokeMock = vi.fn();
const listenMock = vi.fn();

vi.mock("@tauri-apps/api/core", () => ({ invoke: (...a: unknown[]) => invokeMock(...a) }));
vi.mock("@tauri-apps/api/event", () => ({ listen: (...a: unknown[]) => listenMock(...a) }));

import { enqueueCapture, flushQueue, listQueue, removeQueued, subscribeQueue } from "./queue";

describe("queue client", () => {
  beforeEach(() => {
    invokeMock.mockReset();
    listenMock.mockReset();
  });

  it("lists via queue_list", async () => {
    invokeMock.mockResolvedValueOnce([]);
    expect(await listQueue()).toEqual([]);
    expect(invokeMock).toHaveBeenCalledWith("queue_list");
  });

  it("returns an empty list when the command fails", async () => {
    invokeMock.mockRejectedValueOnce(new Error("no state"));
    expect(await listQueue()).toEqual([]);
  });

  it("passes url and note through to queue_enqueue", async () => {
    invokeMock.mockResolvedValueOnce({ id: "x", deduped: false, dropped: 0 });
    await enqueueCapture({ url: "https://example.com" });
    expect(invokeMock).toHaveBeenCalledWith("queue_enqueue", {
      url: "https://example.com",
      note: undefined,
    });
  });

  it("flushes via queue_flush and swallows a failure", async () => {
    invokeMock.mockRejectedValueOnce(new Error("offline"));
    await expect(flushQueue()).resolves.toBeUndefined();
    expect(invokeMock).toHaveBeenCalledWith("queue_flush");
  });

  it("removes by id", async () => {
    invokeMock.mockResolvedValueOnce(undefined);
    await removeQueued("abc");
    expect(invokeMock).toHaveBeenCalledWith("queue_remove", { id: "abc" });
  });

  it("subscribes to queue-changed and hands the payload to the callback", async () => {
    let fire: ((e: { payload: unknown }) => void) | undefined;
    listenMock.mockImplementation((_name: string, cb: (e: { payload: unknown }) => void) => {
      fire = cb;
      return Promise.resolve(() => {});
    });
    const seen: unknown[] = [];
    await subscribeQueue((items) => seen.push(items));
    expect(listenMock.mock.calls[0][0]).toBe("queue-changed");
    fire?.({ payload: [{ id: "a", createdAt: 1, attempts: 0 }] });
    expect(seen).toEqual([[{ id: "a", createdAt: 1, attempts: 0 }]]);
  });
});
