import type { Item } from "./api";

export type HomeLists = {
  desk: Item[];
  recent: Item[];
};

/**
 * Cap Desk/Recent for the dock home view and drop Recent rows that are
 * already shown under Desk (same id).
 */
export function mergeHomeLists(
  desk: Item[],
  recent: Item[],
  opts: { deskCap?: number; recentCap?: number } = {},
): HomeLists {
  const deskCap = opts.deskCap ?? 5;
  const recentCap = opts.recentCap ?? 8;
  const cappedDesk = desk.slice(0, deskCap);
  const deskIds = new Set(cappedDesk.map((i) => i.id));
  const cappedRecent = recent.filter((i) => !deskIds.has(i.id)).slice(0, recentCap);
  return { desk: cappedDesk, recent: cappedRecent };
}
