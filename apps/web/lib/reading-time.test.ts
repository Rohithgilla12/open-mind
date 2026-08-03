import { describe, expect, it } from "vitest";
import { readingMinutes } from "./reading-time";

describe("readingMinutes", () => {
  it("returns null for empty or missing text", () => {
    expect(readingMinutes("")).toBeNull();
    expect(readingMinutes(undefined)).toBeNull();
    expect(readingMinutes(null)).toBeNull();
    expect(readingMinutes("   \n  ")).toBeNull();
  });

  it("returns null below the sub-minute threshold", () => {
    expect(readingMinutes("just a handful of words")).toBeNull();
    expect(readingMinutes(Array(59).fill("word").join(" "))).toBeNull();
  });

  it("rounds to at least one minute once past the threshold", () => {
    expect(readingMinutes(Array(80).fill("word").join(" "))).toBe(1);
  });

  it("scales with length", () => {
    expect(readingMinutes(Array(1100).fill("word").join(" "))).toBe(5);
  });

  it("ignores runs of whitespace when counting words", () => {
    const text = Array(220).fill("word").join("  \n\n  ");
    expect(readingMinutes(text)).toBe(1);
  });
});
