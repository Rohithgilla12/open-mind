import { describe, expect, it } from "vitest";
import { isValidElement } from "react";
import { renderInlineMarkdown, stripMarkdown } from "./text";

describe("stripMarkdown", () => {
  it("unwraps bold and italic markers", () => {
    expect(stripMarkdown("**Aside** is a browser")).toBe("Aside is a browser");
    expect(stripMarkdown("*soft* and __hard__")).toBe("soft and hard");
  });

  it("flattens links and headings", () => {
    expect(stripMarkdown("[CATE](https://example.com) editor")).toBe("CATE editor");
    expect(stripMarkdown("## Hello\nworld")).toBe("Hello\nworld");
  });

  it("returns empty for undefined", () => {
    expect(stripMarkdown(undefined)).toBe("");
  });
});

describe("renderInlineMarkdown", () => {
  it("returns null for empty input", () => {
    expect(renderInlineMarkdown(undefined)).toBeNull();
    expect(renderInlineMarkdown("")).toBeNull();
  });

  it("returns plain strings unchanged when there is no emphasis", () => {
    expect(renderInlineMarkdown("plain summary")).toBe("plain summary");
  });

  it("wraps **bold** in a strong element", () => {
    const node = renderInlineMarkdown("**Aside** built for work");
    expect(isValidElement(node)).toBe(true);
    const children = (node as { props: { children: unknown[] } }).props.children;
    expect(isValidElement(children[0])).toBe(true);
    expect((children[0] as { type: string; props: { children: string } }).type).toBe("strong");
    expect((children[0] as { props: { children: string } }).props.children).toBe("Aside");
    expect(children[1]).toBe(" built for work");
  });

  it("flattens links before rendering emphasis", () => {
    const node = renderInlineMarkdown("**CATE** — see [docs](https://x.test)");
    expect(isValidElement(node)).toBe(true);
    const children = (node as { props: { children: unknown[] } }).props.children;
    expect((children[0] as { props: { children: string } }).props.children).toBe("CATE");
    expect(children[1]).toBe(" — see docs");
  });
});
