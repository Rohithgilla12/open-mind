import { copyIntoQueue, deleteQueueFile, extForMime } from "../asset-store";

const mockCopy = jest.fn();
const mockDelete = jest.fn();

jest.mock("expo-file-system", () => {
  class Directory {
    uri: string;
    exists = false;
    constructor(...parts: unknown[]) {
      this.uri = parts.map(String).join("/");
    }
    create() {
      this.exists = true;
    }
  }
  class File {
    uri: string;
    exists = true;
    constructor(...parts: unknown[]) {
      this.uri = parts.map((p) => (p && (p as { uri?: string }).uri) || String(p)).join("/");
    }
    copy(dest: { uri: string }) {
      mockCopy(this.uri, dest.uri);
      return Promise.resolve();
    }
    delete() {
      mockDelete(this.uri);
    }
  }
  return { Paths: { document: { uri: "DOC" } }, Directory, File };
});

test("extForMime maps known types and defaults to jpg", () => {
  expect(extForMime("image/png")).toBe("png");
  expect(extForMime("image/webp")).toBe("webp");
  expect(extForMime("image/gif")).toBe("gif");
  expect(extForMime("image/jpeg")).toBe("jpg");
  expect(extForMime("application/octet-stream")).toBe("jpg");
});

test("copyIntoQueue copies source into the queue dir and returns the dest uri", async () => {
  const dest = await copyIntoQueue("file:///tmp/pick.jpg", "abc", "image/png");
  expect(dest).toContain("capture-queue");
  expect(dest).toContain("abc.png");
  expect(mockCopy).toHaveBeenCalledWith("file:///tmp/pick.jpg", dest);
});

test("deleteQueueFile deletes when the file exists", () => {
  deleteQueueFile("file:///DOC/capture-queue/abc.png");
  expect(mockDelete).toHaveBeenCalled();
});
