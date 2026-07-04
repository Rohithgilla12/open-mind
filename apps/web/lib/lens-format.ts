import { cardKind, typeLabel } from "./cards";
import { resolveColor } from "./colors";
import type { LensRule } from "./types";

/** Colour for a Lens's sidebar dot: its rule colour, else the given fallback. */
export function lensDot(rule: LensRule | undefined, fallback: string): string {
  return resolveColor(rule?.color) ?? fallback;
}

/** A short human summary of a rule ("“raft” · cobalt · Article, Video"). */
export function lensSummary(rule: LensRule | undefined): string {
  if (!rule) return "";
  const parts: string[] = [];
  if (rule.q) parts.push(`“${rule.q}”`);
  if (rule.color) parts.push(rule.color);
  if (rule.types?.length) {
    parts.push(rule.types.map((t) => typeLabel[cardKind(t)]).join(", "));
  }
  return parts.join(" · ");
}
