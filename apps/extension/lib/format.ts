const MAX_NOTE_CHARS = 10000;

/**
 * Build a note body from selected text plus an optional source URL.
 *
 * Trims the text, then truncates it (rune-safe) so that the final string —
 * `text` followed by a `"\n\n— " + source` attribution line when a source is
 * present — never exceeds MAX_NOTE_CHARS characters. Pure and idempotent.
 */
export function clampNote(text: string, source?: string): string {
  const trimmed = text.trim();
  const src = source?.trim();
  const suffix = src ? `\n\n— ${src}` : "";
  const suffixLen = Array.from(suffix).length;

  const budget = Math.max(0, MAX_NOTE_CHARS - suffixLen);
  const chars = Array.from(trimmed);
  const body =
    chars.length > budget ? chars.slice(0, budget).join("") : trimmed;

  return body + suffix;
}
