import { describe, expect, it } from "vitest";
import { colorSearchHref, colourTerm } from "./colors";

describe("colorSearchHref", () => {
  it("encodes a hex term with its leading #", () => {
    expect(colorSearchHref("#1B3FD1")).toBe("/?color=%231B3FD1");
  });
  it("passes a named colour through", () => {
    expect(colorSearchHref("cobalt")).toBe("/?color=cobalt");
  });
  it("encodes spaces in a named colour", () => {
    expect(colorSearchHref("dark blue")).toBe("/?color=dark%20blue");
  });
});

describe("colourTerm", () => {
  const cases: [string, string, string | null][] = [
    ["a recognised name", "cobalt", "cobalt"],
    ["a name, case and space insensitive", "  Terracotta ", "terracotta"],
    ["hex with its hash", "#1B3FD1", "#1b3fd1"],
    ["shorthand hex with its hash", "#abc", "#abc"],
    // The whole point: these are ordinary words that happen to be spelled in
    // hex digits, and resolveColor would take every one of them for a colour.
    ["a hex-shaped word is not a colour", "facade", null],
    ["nor this one", "decade", null],
    ["nor a bare six-digit hex", "1b3fd1", null],
    ["an ordinary word", "kyoto", null],
    ["empty", "   ", null],
  ];
  for (const [name, input, want] of cases) {
    it(name, () => expect(colourTerm(input)).toBe(want));
  }
});
