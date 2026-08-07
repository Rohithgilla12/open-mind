import {
  extensionForMimeType,
  fallbackFilename,
  isUploadableMimeType,
  uploadMimeType,
} from "./uploads";

const DOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
const ODT = "application/vnd.oasis.opendocument.text";
const EPUB = "application/epub+zip";

describe("isUploadableMimeType", () => {
  it("accepts images", () => {
    expect(isUploadableMimeType("image/jpeg")).toBe(true);
    expect(isUploadableMimeType("image/png")).toBe(true);
  });

  it("accepts documents and PDFs", () => {
    expect(isUploadableMimeType(DOCX)).toBe(true);
    expect(isUploadableMimeType(ODT)).toBe(true);
    expect(isUploadableMimeType(EPUB)).toBe(true);
    expect(isUploadableMimeType("application/rtf")).toBe(true);
    expect(isUploadableMimeType("text/rtf")).toBe(true);
    expect(isUploadableMimeType("application/pdf")).toBe(true);
  });

  it("rejects everything else", () => {
    expect(isUploadableMimeType("application/vnd.ms-excel")).toBe(false);
    expect(isUploadableMimeType("application/zip")).toBe(false);
    expect(isUploadableMimeType("text/plain")).toBe(false);
    expect(isUploadableMimeType("")).toBe(false);
    expect(isUploadableMimeType(undefined)).toBe(false);
    expect(isUploadableMimeType(null)).toBe(false);
  });
});

describe("uploadMimeType", () => {
  // The bug this guards: forcing every non-image to image/jpeg would make the
  // server sniff a .docx as an unsupported image and reject the upload.
  it("passes document types through untouched", () => {
    expect(uploadMimeType(DOCX)).toBe(DOCX);
    expect(uploadMimeType(EPUB)).toBe(EPUB);
    expect(uploadMimeType("application/pdf")).toBe("application/pdf");
  });

  it("passes image types through untouched", () => {
    expect(uploadMimeType("image/png")).toBe("image/png");
  });

  it("falls back to jpeg when nothing better is known", () => {
    expect(uploadMimeType(undefined)).toBe("image/jpeg");
    expect(uploadMimeType("")).toBe("image/jpeg");
    expect(uploadMimeType("application/octet-stream")).toBe("image/jpeg");
  });
});

describe("extensionForMimeType", () => {
  it("maps documents", () => {
    expect(extensionForMimeType(DOCX)).toBe("docx");
    expect(extensionForMimeType(ODT)).toBe("odt");
    expect(extensionForMimeType(EPUB)).toBe("epub");
    expect(extensionForMimeType("application/rtf")).toBe("rtf");
    expect(extensionForMimeType("application/pdf")).toBe("pdf");
  });

  it("maps images", () => {
    expect(extensionForMimeType("image/png")).toBe("png");
    expect(extensionForMimeType("image/webp")).toBe("webp");
    expect(extensionForMimeType("image/jpeg")).toBe("jpg");
  });

  it("defaults to jpg", () => {
    expect(extensionForMimeType(undefined)).toBe("jpg");
  });
});

describe("fallbackFilename", () => {
  it("names documents as documents and images as photos", () => {
    expect(fallbackFilename(DOCX)).toBe("document.docx");
    expect(fallbackFilename(EPUB)).toBe("document.epub");
    expect(fallbackFilename("application/pdf")).toBe("document.pdf");
    expect(fallbackFilename("image/png")).toBe("photo.png");
    expect(fallbackFilename(undefined)).toBe("photo.jpg");
  });
});
