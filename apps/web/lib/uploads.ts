/**
 * Which local files the drop zone will hand to POST /assets.
 *
 * The server sniffs the content and is the only authority on what is
 * accepted — this exists purely to avoid pointless round trips for files that
 * could never work. It is therefore deliberately permissive: a false accept
 * costs one 415, a false reject makes the app look broken.
 *
 * Extensions matter as much as MIME types here. Browsers routinely report an
 * empty `File.type` for .odt, .epub and .rtf, so filtering on MIME alone would
 * silently drop valid documents.
 */

const DOCUMENT_MIME_TYPES = new Set([
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  "application/vnd.oasis.opendocument.text",
  "application/rtf",
  "text/rtf",
  "application/epub+zip",
]);

const DOCUMENT_EXTENSIONS = [".docx", ".odt", ".rtf", ".epub"];

/** The `accept` attribute for the file input. */
export const UPLOAD_ACCEPT = [
  "image/*",
  "application/pdf",
  ...DOCUMENT_EXTENSIONS,
  ...DOCUMENT_MIME_TYPES,
].join(",");

/** Human-readable list for the drop zone's label and prompt. */
export const UPLOAD_PROMPT = "Drop images, PDFs, or documents here, or click to upload";
export const UPLOAD_LABEL =
  "Upload images, PDFs, or documents (.docx, .odt, .rtf, .epub) — drop files here or click to choose";

/** True when the filename ends in one of the document extensions. */
function hasDocumentExtension(name: string): boolean {
  const lower = name.toLowerCase();
  return DOCUMENT_EXTENSIONS.some((ext) => lower.endsWith(ext));
}

/** Whether the drop zone should attempt to upload this file. */
export function isUploadable(file: { name: string; type: string }): boolean {
  if (file.type.startsWith("image/")) return true;
  if (file.type === "application/pdf") return true;
  if (DOCUMENT_MIME_TYPES.has(file.type)) return true;
  // Fall back to the name when the browser gave us nothing useful.
  return hasDocumentExtension(file.name);
}
