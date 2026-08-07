/**
 * Which shared files the Android SEND intent path will upload to POST /assets.
 *
 * The server sniffs content and is the authority on what is accepted; this
 * only decides what the app forwards rather than dropping on the floor.
 *
 * iOS does not use this: its share extension (targets/share) saves inline and
 * narrows types natively before the app is ever involved.
 */

/** Document MIME types the enrichment pipeline converts. */
const DOCUMENT_MIME_TYPES: Record<string, string> = {
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
  "application/vnd.oasis.opendocument.text": "odt",
  "application/rtf": "rtf",
  "text/rtf": "rtf",
  "application/epub+zip": "epub",
  "application/pdf": "pdf",
};

/** Whether a shared file should be uploaded as an asset. */
export function isUploadableMimeType(mimeType: string | undefined | null): boolean {
  if (typeof mimeType !== "string" || mimeType === "") return false;
  if (mimeType.startsWith("image/")) return true;
  return mimeType in DOCUMENT_MIME_TYPES;
}

/**
 * The MIME type to send for a shared file. Unknown types fall back to JPEG
 * only when nothing better is known, preserving the previous image behaviour;
 * a recognised document type is always passed through untouched, because the
 * server needs it to pick the right parser.
 */
export function uploadMimeType(mimeType: string | undefined | null): string {
  if (typeof mimeType === "string") {
    if (mimeType.startsWith("image/")) return mimeType;
    if (mimeType in DOCUMENT_MIME_TYPES) return mimeType;
  }
  return "image/jpeg";
}

/** A sensible filename extension for a MIME type, used when the share gave none. */
export function extensionForMimeType(mimeType: string | undefined | null): string {
  if (typeof mimeType === "string") {
    const documentExt = DOCUMENT_MIME_TYPES[mimeType];
    if (documentExt) return documentExt;
    if (mimeType === "image/png") return "png";
    if (mimeType === "image/webp") return "webp";
    if (mimeType === "image/gif") return "gif";
    if (mimeType === "image/avif") return "avif";
  }
  return "jpg";
}

/** A default filename for a shared file of this type. */
export function fallbackFilename(mimeType: string | undefined | null): string {
  const ext = extensionForMimeType(mimeType);
  const stem = ext === "jpg" || ext === "png" || ext === "webp" || ext === "gif" || ext === "avif"
    ? "photo"
    : "document";
  return `${stem}.${ext}`;
}
