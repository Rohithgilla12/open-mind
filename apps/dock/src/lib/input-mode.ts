export type InputMode = "search" | "save-url";

const URL_PATTERN = /^https?:\/\//i;

/** Plain-text input searches; a bare http(s) URL switches the panel to save mode. */
export function detectMode(input: string): InputMode {
  return URL_PATTERN.test(input.trim()) ? "save-url" : "search";
}
