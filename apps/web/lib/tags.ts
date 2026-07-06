/**
 * Deduped union of AI tags and user tags for display. Case-insensitive dedupe
 * (first-seen casing wins), preserving order with AI tags first then user tags.
 */
export function unionTags(ai?: string[], user?: string[]): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const tag of [...(ai ?? []), ...(user ?? [])]) {
    const key = tag.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(tag);
  }
  return out;
}
