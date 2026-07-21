import AsyncStorage from "@react-native-async-storage/async-storage";

const mockCopyIntoQueue = jest.fn(
  async (_src: string, id: string, _mime: string) => `file:///q/${id}.jpg`,
);
const mockDeleteQueueFile = jest.fn();
const mockQueueFileExists = jest.fn((_u: string) => true);
jest.mock("../asset-store", () => ({
  copyIntoQueue: (s: string, id: string, m: string) => mockCopyIntoQueue(s, id, m),
  deleteQueueFile: (u: string) => mockDeleteQueueFile(u),
  queueFileExists: (u: string) => mockQueueFileExists(u),
  extForMime: () => "jpg",
}));

const mockUploadAsset = jest.fn();
const mockSaveItem = jest.fn();
jest.mock("../api", () => ({
  uploadAsset: (f: unknown) => mockUploadAsset(f),
  saveItem: (p: unknown) => mockSaveItem(p),
}));

import { enqueueAsset, flushQueue, listQueued, removeQueued, enqueue } from "../capture-queue";

beforeEach(async () => {
  await AsyncStorage.clear();
  mockCopyIntoQueue.mockClear();
  mockDeleteQueueFile.mockClear();
  mockQueueFileExists.mockClear();
  mockQueueFileExists.mockImplementation(() => true);
  mockUploadAsset.mockClear();
  mockSaveItem.mockClear();
});

test("enqueueAsset copies each file and stores an asset entry", async () => {
  const { ids } = await enqueueAsset([
    { uri: "file:///tmp/a.jpg", name: "a.jpg", type: "image/jpeg" },
  ]);
  expect(ids).toHaveLength(1);
  expect(mockCopyIntoQueue).toHaveBeenCalledTimes(1);
  const pending = await listQueued();
  expect(pending[0].asset).toEqual({
    filePath: `file:///q/${ids[0]}.jpg`,
    name: "a.jpg",
    type: "image/jpeg",
  });
});

test("flush uploads an asset entry, then removes it and deletes its file", async () => {
  mockUploadAsset.mockResolvedValue({ ok: true, status: 201 });
  await enqueueAsset([{ uri: "file:///tmp/a.jpg", name: "a.jpg", type: "image/jpeg" }]);
  const res = await flushQueue();
  expect(res).toEqual({ sent: 1, remaining: 0 });
  expect(mockUploadAsset).toHaveBeenCalledWith(
    expect.objectContaining({ name: "a.jpg", type: "image/jpeg" }),
  );
  expect(mockDeleteQueueFile).toHaveBeenCalledWith(expect.stringContaining("file:///q/"));
});

test("permanent 4xx on an asset drops the entry and deletes its file", async () => {
  mockUploadAsset.mockResolvedValue({ ok: false, status: 415 });
  await enqueueAsset([{ uri: "file:///tmp/a.jpg", name: "a.jpg", type: "image/jpeg" }]);
  const res = await flushQueue();
  expect(res.remaining).toBe(0);
  expect(mockDeleteQueueFile).toHaveBeenCalled();
});

test("network error on an asset keeps the entry and file, bumps attempts", async () => {
  mockUploadAsset.mockResolvedValue({ ok: false, status: 0 });
  await enqueueAsset([{ uri: "file:///tmp/a.jpg", name: "a.jpg", type: "image/jpeg" }]);
  const res = await flushQueue();
  expect(res.remaining).toBe(1);
  expect(mockDeleteQueueFile).not.toHaveBeenCalled();
  const pending = await listQueued();
  expect(pending[0].attempts).toBe(1);
});

test("enqueueAsset skips a file whose copy fails but still enqueues the rest", async () => {
  mockCopyIntoQueue
    .mockImplementationOnce(async () => {
      throw new Error("disk full");
    })
    .mockImplementationOnce(async (_src: string, id: string) => `file:///q/${id}.jpg`);
  const { ids } = await enqueueAsset([
    { uri: "file:///tmp/bad.jpg", name: "bad.jpg", type: "image/jpeg" },
    { uri: "file:///tmp/good.jpg", name: "good.jpg", type: "image/jpeg" },
  ]);
  expect(ids).toHaveLength(1);
  const pending = await listQueued();
  expect(pending).toHaveLength(1);
  expect(pending[0].asset?.name).toBe("good.jpg");
});

test("cap eviction deletes the evicted oldest asset files", async () => {
  for (let i = 0; i < 101; i += 1) {
    await enqueueAsset([{ uri: `file:///tmp/${i}.jpg`, name: `${i}.jpg`, type: "image/jpeg" }]);
  }
  const pending = await listQueued();
  expect(pending).toHaveLength(100);
  expect(mockDeleteQueueFile).toHaveBeenCalled();
});

test("removeQueued deletes an asset's file but not for url/note entries", async () => {
  const { ids } = await enqueueAsset([
    { uri: "file:///tmp/a.jpg", name: "a.jpg", type: "image/jpeg" },
  ]);
  const { id: urlId } = await enqueue({ url: "https://example.com" });

  await removeQueued(ids[0]);
  expect(mockDeleteQueueFile).toHaveBeenCalledWith(expect.stringContaining("file:///q/"));

  mockDeleteQueueFile.mockClear();
  await removeQueued(urlId);
  expect(mockDeleteQueueFile).not.toHaveBeenCalled();
});

test("flush drops an asset entry whose backing file vanished, without blocking a following entry", async () => {
  const { ids } = await enqueueAsset([
    { uri: "file:///tmp/gone.jpg", name: "gone.jpg", type: "image/jpeg" },
  ]);
  const goneFilePath = `file:///q/${ids[0]}.jpg`;
  mockQueueFileExists.mockImplementation((u: string) => u !== goneFilePath);
  await enqueue({ url: "https://still-flushes.com" });
  mockSaveItem.mockResolvedValue({ ok: true, status: 201 });

  const res = await flushQueue();
  expect(res.sent).toBe(1);
  expect(res.remaining).toBe(0);
  expect(mockUploadAsset).not.toHaveBeenCalled();
  expect(mockDeleteQueueFile).toHaveBeenCalledWith(goneFilePath);
  const pending = await listQueued();
  expect(pending).toHaveLength(0);
});

test("readQueue drops a row whose asset lacks a string filePath, keeps valid rows", async () => {
  await AsyncStorage.setItem(
    "openmind.captureQueue",
    JSON.stringify([
      { id: "corrupt", createdAt: 1, attempts: 0, asset: { name: "x.jpg", type: "image/jpeg" } },
      { id: "url-ok", createdAt: 2, attempts: 0, url: "https://ok.com" },
      {
        id: "asset-ok",
        createdAt: 3,
        attempts: 0,
        asset: { filePath: "file:///q/asset-ok.jpg", name: "x.jpg", type: "image/jpeg" },
      },
    ]),
  );
  const pending = await listQueued();
  expect(pending.map((p) => p.id).sort()).toEqual(["asset-ok", "url-ok"]);
});
