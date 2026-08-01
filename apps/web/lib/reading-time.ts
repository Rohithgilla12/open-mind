const WORDS_PER_MINUTE = 220;

/**
 * Estimated minutes to read `text`, or null when the text is too short for a
 * reading-time label to mean anything (sub-minute reads just add noise).
 */
export function readingMinutes(text: string | undefined | null): number | null {
  const words = (text ?? "").trim().split(/\s+/).filter(Boolean).length;
  if (words < 60) return null;
  return Math.max(1, Math.round(words / WORDS_PER_MINUTE));
}
