import { describe, expect, it } from "vitest";
import {
  LAST_UPDATED,
  clients,
  pipelineStages,
  principles,
  stack,
} from "./architecture";

describe("architecture content", () => {
  it("has a valid ISO LAST_UPDATED date", () => {
    expect(LAST_UPDATED).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });

  it("keeps the six non-negotiable principles", () => {
    expect(principles).toHaveLength(6);
  });

  it("lists the four core pipeline stages in order", () => {
    expect(pipelineStages.map((s) => s.name)).toEqual([
      "extract",
      "classify",
      "summarise",
      "embed",
    ]);
  });

  it("documents every client app", () => {
    expect(clients.map((cl) => cl.name).sort()).toEqual([
      "Dock",
      "Extension",
      "Mobile",
      "Web",
    ]);
  });

  it("has no empty cells and no duplicate keys across sections", () => {
    for (const p of principles) {
      expect(p.title.trim()).not.toBe("");
      expect(p.body.trim()).not.toBe("");
    }
    for (const row of stack) {
      expect(row.layer.trim()).not.toBe("");
      expect(row.choice.trim()).not.toBe("");
      expect(row.why.trim()).not.toBe("");
    }

    const stackLayers = stack.map((r) => r.layer);
    expect(new Set(stackLayers).size).toBe(stackLayers.length);

    const principleTitles = principles.map((p) => p.title);
    expect(new Set(principleTitles).size).toBe(principleTitles.length);
  });
});
