import { listItems, readItemPage } from "../api";

describe("readItemPage", () => {
  it("reads the ItemPage envelope including the cursor", () => {
    const got = readItemPage({ items: [{ id: "a" }], nextCursor: "cur1" });
    expect(got).toEqual({ items: [{ id: "a" }], nextCursor: "cur1" });
  });

  it("reads a bare array from an instance predating the envelope, with no cursor", () => {
    // Graceful degradation: no cursor means pagination simply stops after
    // page 1 instead of the screen breaking against an older self-host.
    const got = readItemPage([{ id: "a" }]);
    expect(got).toEqual({ items: [{ id: "a" }], nextCursor: undefined });
  });

  it("returns null for an unrecognised body so callers can report a failure", () => {
    expect(readItemPage({ unexpected: true })).toBeNull();
    expect(readItemPage(null)).toBeNull();
    expect(readItemPage("nope")).toBeNull();
  });

  it("treats a missing nextCursor as the end of the list", () => {
    expect(readItemPage({ items: [] })).toEqual({ items: [], nextCursor: undefined });
  });
});

describe("listItems", () => {
  const settings = { instanceUrl: "https://openmind.test", token: "omk_test" } as const;

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("sends the cursor and returns it back out", async () => {
    const fetchMock = jest.spyOn(global, "fetch" as never).mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ items: [{ id: "a" }], nextCursor: "cur2" }),
    } as never);

    const res = await listItems(50, "cur1", settings as never);

    expect(res.ok).toBe(true);
    expect(res.nextCursor).toBe("cur2");
    const url = String((fetchMock.mock.calls[0] as unknown[])[0]);
    expect(url).toContain("limit=50");
    expect(url).toContain("cursor=cur1");
  });

  it("reports failure rather than an empty list when the body is unrecognised", async () => {
    jest.spyOn(global, "fetch" as never).mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ unexpected: true }),
    } as never);

    const res = await listItems(50, undefined, settings as never);
    expect(res.ok).toBe(false);
    expect(res.items).toEqual([]);
  });
});
