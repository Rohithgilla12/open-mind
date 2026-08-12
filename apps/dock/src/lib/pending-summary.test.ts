import { describe, expect, it } from "vitest";
import { entryLabel, pendingSummary, relativeAge, STUCK_ATTEMPTS } from "./pending-summary";
import type { QueuedCapture } from "./queue";

function entry(over: Partial<QueuedCapture> = {}): QueuedCapture {
  return { id: "a", createdAt: 0, attempts: 0, ...over };
}

describe("pendingSummary", () => {
  it("singularises one save", () => {
    expect(pendingSummary([entry()]).label).toBe("1 save waiting to sync");
  });

  it("pluralises more than one", () => {
    expect(pendingSummary([entry({ id: "a" }), entry({ id: "b" })]).label).toBe(
      "2 saves waiting to sync",
    );
  });

  it("is not stuck while attempts stay under the threshold", () => {
    expect(pendingSummary([entry({ attempts: STUCK_ATTEMPTS - 1 })]).stuck).toBe(false);
  });

  it("is stuck once any entry reaches the threshold", () => {
    expect(pendingSummary([entry({ attempts: STUCK_ATTEMPTS })]).stuck).toBe(true);
  });
});

describe("entryLabel", () => {
  it("shows a bare hostname for a URL", () => {
    expect(entryLabel(entry({ url: "https://www.example.com/deep/path" }))).toBe("example.com");
  });

  it("excerpts a note", () => {
    const note = "a".repeat(80);
    const label = entryLabel(entry({ note }));
    expect(label.length).toBeLessThanOrEqual(49);
    expect(label.endsWith("…")).toBe(true);
  });

  it("leaves a short note intact", () => {
    expect(entryLabel(entry({ note: "short note" }))).toBe("short note");
  });

  it("falls back for an entry with neither", () => {
    expect(entryLabel(entry())).toBe("Untitled save");
  });
});

describe("relativeAge", () => {
  const now = 1_000_000_000_000;

  it("reads just now under a minute", () => {
    expect(relativeAge(now - 30_000, now)).toBe("just now");
  });

  it("reads minutes", () => {
    expect(relativeAge(now - 5 * 60_000, now)).toBe("5m ago");
  });

  it("reads hours", () => {
    expect(relativeAge(now - 3 * 3_600_000, now)).toBe("3h ago");
  });

  it("reads days", () => {
    expect(relativeAge(now - 2 * 86_400_000, now)).toBe("2d ago");
  });

  it("never reads negative for a clock skewed into the future", () => {
    expect(relativeAge(now + 60_000, now)).toBe("just now");
  });
});
