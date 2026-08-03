import { cardKind, KNOWN_KINDS, typeLabel, typeLabelPlural } from "../cards";
import { colors, typeGradients } from "../theme";

describe("repo card type", () => {
  it("normalises repo to itself", () => {
    expect(cardKind("repo")).toBe("repo");
  });

  it("falls back unknown/absent types to article", () => {
    expect(cardKind("something-unknown")).toBe("article");
    expect(cardKind(undefined)).toBe("article");
  });

  it("is included in KNOWN_KINDS", () => {
    expect(KNOWN_KINDS).toContain("repo");
  });

  it("labels it Repo / Repos", () => {
    expect(typeLabel.repo).toBe("Repo");
    expect(typeLabelPlural.repo).toBe("Repos");
  });

  it("has a gold-to-green gradient", () => {
    expect(typeGradients.repo).toEqual([colors.gold, colors.green]);
  });
});
