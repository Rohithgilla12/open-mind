jest.mock("react-native", () => ({ Platform: { OS: "ios" } }));

const mockEnqueue = jest.fn(async (_i?: unknown) => ({ id: "x", deduped: false }));
const mockEnqueueAsset = jest.fn(async (_f?: unknown) => ({ ids: ["y"] }));
jest.mock("../capture-queue", () => ({
  enqueue: (i: unknown) => mockEnqueue(i),
  enqueueAsset: (f: unknown) => mockEnqueueAsset(f),
}));

const mockFileDelete = jest.fn();
jest.mock("expo-file-system", () => {
  class File {
    uri: string;
    exists = true;
    constructor(...parts: unknown[]) {
      this.uri = parts.map((p) => (p && (p as { uri?: string }).uri) || String(p)).join("/");
    }
    delete() {
      mockFileDelete(this.uri);
    }
  }
  return {
    File,
    Directory: class {},
    Paths: { appleSharedContainers: { "group.fun.gilla.openmind": { uri: "GROUP" } } },
  };
});

let mockStore: Record<string, unknown> = {};
const mockSet = jest.fn((k: string, v: unknown) => {
  if (v == null) delete mockStore[k];
  else mockStore[k] = v;
});
const mockGet = jest.fn((k: string) => (mockStore[k] == null ? null : JSON.stringify(mockStore[k])));
jest.mock("@bacons/apple-targets", () => ({
  ExtensionStorage: class {
    constructor(_group: string) {}
    get(k: string) {
      return mockGet(k);
    }
    set(k: string, v: unknown) {
      mockSet(k, v);
    }
  },
}));

import { drainSharedPending } from "../shared-pending";

beforeEach(() => {
  mockStore = {};
  mockEnqueue.mockClear();
  mockEnqueueAsset.mockClear();
  mockFileDelete.mockClear();
  mockSet.mockClear();
});

test("drains an asset record: enqueues it, deletes the container file, clears the manifest", async () => {
  mockStore.pendingShares = [
    { kind: "asset", filename: "u.jpg", name: "u.jpg", mimeType: "image/jpeg", createdAt: 1 },
  ];
  const n = await drainSharedPending();
  expect(n).toBe(1);
  expect(mockEnqueueAsset).toHaveBeenCalledWith([
    expect.objectContaining({ name: "u.jpg", type: "image/jpeg" }),
  ]);
  expect(mockFileDelete).toHaveBeenCalled();
  expect(mockStore.pendingShares).toBeUndefined();
});

test("drains url and note records", async () => {
  mockStore.pendingShares = [
    { kind: "url", value: "https://e.com", createdAt: 1 },
    { kind: "note", value: "hi", createdAt: 2 },
  ];
  const n = await drainSharedPending();
  expect(n).toBe(2);
  expect(mockEnqueue).toHaveBeenCalledWith({ url: "https://e.com" });
  expect(mockEnqueue).toHaveBeenCalledWith({ note: "hi" });
});

test("malformed manifest is a no-op", async () => {
  mockStore.pendingShares = "not-an-array" as unknown;
  const n = await drainSharedPending();
  expect(n).toBe(0);
});

test("a throw mid-drain leaves the remaining records intact", async () => {
  mockStore.pendingShares = [
    { kind: "url", value: "https://a.com", createdAt: 1 },
    { kind: "url", value: "https://b.com", createdAt: 2 },
  ];
  mockEnqueue.mockImplementationOnce(async () => ({ id: "1", deduped: false }));
  mockEnqueue.mockImplementationOnce(async () => {
    throw new Error("boom");
  });
  const n = await drainSharedPending();
  expect(n).toBe(1);
  expect((mockStore.pendingShares as unknown[]).length).toBe(1);
});

test("non-iOS is a no-op and never calls enqueue", async () => {
  await jest.isolateModulesAsync(async () => {
    jest.doMock("react-native", () => ({ Platform: { OS: "android" } }));
    const { drainSharedPending: drain } = require("../shared-pending");
    const n = await drain();
    expect(n).toBe(0);
    expect(mockEnqueue).not.toHaveBeenCalled();
    expect(mockEnqueueAsset).not.toHaveBeenCalled();
  });
});

test("module absent (@bacons/apple-targets throws) is a no-op", async () => {
  await jest.isolateModulesAsync(async () => {
    jest.doMock("@bacons/apple-targets", () => ({
      ExtensionStorage: class {
        constructor() {
          throw new Error("module not linked");
        }
      },
    }));
    const { drainSharedPending: drain } = require("../shared-pending");
    const n = await drain();
    expect(n).toBe(0);
    expect(mockEnqueue).not.toHaveBeenCalled();
    expect(mockEnqueueAsset).not.toHaveBeenCalled();
  });
});
