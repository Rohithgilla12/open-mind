import { describe, expect, it } from "vitest";
import { detectMode } from "./input-mode";

describe("detectMode", () => {
  it("treats plain text as search", () => {
    expect(detectMode("recipe for sourdough")).toBe("search");
  });

  it("treats an empty string as search", () => {
    expect(detectMode("")).toBe("search");
  });

  it("treats an http url as save-url", () => {
    expect(detectMode("http://example.com/article")).toBe("save-url");
  });

  it("treats an https url as save-url", () => {
    expect(detectMode("https://example.com/article")).toBe("save-url");
  });

  it("is case-insensitive on the scheme", () => {
    expect(detectMode("HTTPS://example.com")).toBe("save-url");
  });

  it("ignores leading whitespace", () => {
    expect(detectMode("   https://example.com")).toBe("save-url");
  });

  it("does not treat a bare domain without a scheme as a url", () => {
    expect(detectMode("example.com")).toBe("search");
  });

  it("does not treat other schemes as save-url", () => {
    expect(detectMode("ftp://example.com/file")).toBe("search");
  });
});
