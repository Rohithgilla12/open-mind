import { describe, expect, it } from "vitest";
import { cardKind, isTextForward, typeGradient, typeLabel } from "./cards";

describe("repo card type", () => {
  it("normalises repo to itself", () => {
    expect(cardKind("repo")).toBe("repo");
  });
  it("labels it Repo", () => {
    expect(typeLabel.repo).toBe("Repo");
  });
  it("has a gradient", () => {
    expect(typeGradient.repo).toContain("linear-gradient");
  });
  it("still falls back to article for unknown types", () => {
    expect(cardKind("gizmo")).toBe("article");
  });
});

describe("isTextForward", () => {
  it("treats repo as text-forward (README-style body reads like an article)", () => {
    expect(isTextForward("repo")).toBe(true);
  });
  it("excludes non-text-forward kinds, e.g. image", () => {
    expect(isTextForward("image")).toBe(false);
  });
});
