// Durable storage for queued image captures. Picker/share URIs are ephemeral
// cache files the OS can evict, so a queued image is copied into the app's
// document directory and referenced by that path until it uploads.
import { Directory, File, Paths } from "expo-file-system";

const QUEUE_DIR_NAME = "capture-queue";

export function extForMime(mime: string): string {
  switch (mime) {
    case "image/png":
      return "png";
    case "image/webp":
      return "webp";
    case "image/gif":
      return "gif";
    default:
      return "jpg";
  }
}

function queueDir(): Directory {
  return new Directory(Paths.document, QUEUE_DIR_NAME);
}

function ensureQueueDir(): Directory {
  const dir = queueDir();
  if (!dir.exists) dir.create({ intermediates: true, idempotent: true });
  return dir;
}

export async function copyIntoQueue(
  sourceUri: string,
  id: string,
  mime: string,
): Promise<string> {
  const dir = ensureQueueDir();
  const dest = new File(dir, `${id}.${extForMime(mime)}`);
  await new File(sourceUri).copy(dest, { overwrite: true });
  return dest.uri;
}

export function deleteQueueFile(uri: string): void {
  try {
    const file = new File(uri);
    if (file.exists) file.delete();
  } catch {
    // Best-effort — a missing file is already the desired state.
  }
}

export function queueFileExists(uri: string): boolean {
  try {
    return new File(uri).exists;
  } catch {
    return false;
  }
}
