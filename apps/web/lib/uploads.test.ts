import { describe, expect, it } from "vitest";
import { isUploadable, UPLOAD_ACCEPT } from "./uploads";

describe("isUploadable", () => {
  it("accepts images by MIME type", () => {
    expect(isUploadable({ name: "photo.png", type: "image/png" })).toBe(true);
    expect(isUploadable({ name: "photo.jpg", type: "image/jpeg" })).toBe(true);
    expect(isUploadable({ name: "photo.avif", type: "image/avif" })).toBe(true);
  });

  it("accepts PDFs", () => {
    expect(isUploadable({ name: "paper.pdf", type: "application/pdf" })).toBe(true);
  });

  it("accepts documents by MIME type", () => {
    expect(
      isUploadable({
        name: "report.docx",
        type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      }),
    ).toBe(true);
    expect(isUploadable({ name: "notes.odt", type: "application/vnd.oasis.opendocument.text" })).toBe(
      true,
    );
    expect(isUploadable({ name: "novel.epub", type: "application/epub+zip" })).toBe(true);
  });

  // The case that motivates the extension fallback: browsers often report no
  // type at all for these, and filtering on MIME alone would drop them.
  it("accepts documents by extension when the browser reports no type", () => {
    expect(isUploadable({ name: "report.docx", type: "" })).toBe(true);
    expect(isUploadable({ name: "notes.odt", type: "" })).toBe(true);
    expect(isUploadable({ name: "memo.rtf", type: "" })).toBe(true);
    expect(isUploadable({ name: "novel.epub", type: "" })).toBe(true);
  });

  it("matches extensions case-insensitively", () => {
    expect(isUploadable({ name: "REPORT.DOCX", type: "" })).toBe(true);
    expect(isUploadable({ name: "Novel.ePub", type: "" })).toBe(true);
  });

  it("accepts both RTF MIME spellings", () => {
    expect(isUploadable({ name: "a", type: "application/rtf" })).toBe(true);
    expect(isUploadable({ name: "a", type: "text/rtf" })).toBe(true);
  });

  it("rejects unsupported files", () => {
    expect(isUploadable({ name: "sheet.xlsx", type: "application/vnd.ms-excel" })).toBe(false);
    expect(isUploadable({ name: "deck.pptx", type: "" })).toBe(false);
    expect(isUploadable({ name: "archive.zip", type: "application/zip" })).toBe(false);
    expect(isUploadable({ name: "notes.txt", type: "text/plain" })).toBe(false);
    expect(isUploadable({ name: "no-extension", type: "" })).toBe(false);
  });

  // ".doc" is not accepted, and must not sneak in as a suffix match of ".docx".
  it("rejects legacy .doc", () => {
    expect(isUploadable({ name: "old.doc", type: "application/msword" })).toBe(false);
  });
});

describe("UPLOAD_ACCEPT", () => {
  it("covers images, PDFs, and every document extension", () => {
    expect(UPLOAD_ACCEPT).toContain("image/*");
    expect(UPLOAD_ACCEPT).toContain("application/pdf");
    for (const ext of [".docx", ".odt", ".rtf", ".epub"]) {
      expect(UPLOAD_ACCEPT).toContain(ext);
    }
  });
});
