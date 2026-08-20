import { describe, expect, it } from "vitest";
import { serverSearchParams } from "./search-request";

describe("serverSearchParams", () => {
  const cases: [string, string, string][] = [
    ["a plain word", "kyoto", "q=kyoto&parse=true"],
    ["trims", "  kyoto  ", "q=kyoto&parse=true"],
    ["a named colour also goes as colour", "cobalt", "q=cobalt&parse=true&color=cobalt"],
    ["a hex term too", "#1B3FD1", "q=%231B3FD1&parse=true&color=%231B3FD1"],
    ["a colour word inside a phrase is left to the parser", "cobalt print", "q=cobalt+print&parse=true"],
    ["a non-colour word", "postgres", "q=postgres&parse=true"],
    ["a hex-shaped word is not sent as a colour", "facade", "q=facade&parse=true"],
    ["nor is a bare hex string", "1b3fd1", "q=1b3fd1&parse=true"],
  ];
  for (const [name, input, want] of cases) {
    it(name, () => expect(serverSearchParams(input).toString()).toBe(want));
  }
});
