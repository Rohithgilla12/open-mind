package api

import (
	"archive/zip"
	"bytes"
	"strings"
)

// Document uploads are sniffed here rather than trusted from the multipart
// part header, the same rule the image allowlist follows.
//
// .docx, .odt and .epub are all ZIP containers, so http.DetectContentType
// answers "application/zip" for every one of them and cannot tell them apart.
// Disambiguation reads entry *names* and the uncompressed mimetype member
// only — never decompressed content — so the sniff itself cannot be
// zip-bombed. MaxBytesReader still bounds the request as a whole.

const (
	docxContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	odtContentType  = "application/vnd.oasis.opendocument.text"
	rtfContentType  = "application/rtf"
	epubContentType = "application/epub+zip"
)

// rtfMagic opens every Rich Text Format file.
var rtfMagic = []byte(`{\rtf`)

// zipMagic opens a ZIP local file header. Empty and spanned archives use other
// signatures, neither of which can carry a document.
var zipMagic = []byte("PK\x03\x04")

// allowedDocTypes is the content-type allowlist for document uploads.
// Spreadsheets, presentations and CSV are deliberately absent: they make poor
// cards and noisy embeddings.
var allowedDocTypes = map[string]struct{}{
	docxContentType: {},
	odtContentType:  {},
	rtfContentType:  {},
	epubContentType: {},
}

// isDocument reports whether contentType is an accepted document type.
func isDocument(contentType string) bool {
	_, ok := allowedDocTypes[contentType]
	return ok
}

// detectDocType returns the content type of an accepted document format, or ""
// when data is not one. It never trusts the client-supplied part header.
func detectDocType(data []byte) string {
	if bytes.HasPrefix(data, rtfMagic) {
		return rtfContentType
	}
	if !bytes.HasPrefix(data, zipMagic) {
		return ""
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	// ODT and EPUB both declare themselves in an uncompressed "mimetype"
	// member, which their specifications require to come first.
	if mimetype := zipMimetype(zr); mimetype != "" {
		switch mimetype {
		case odtContentType:
			return odtContentType
		case epubContentType:
			return epubContentType
		}
		// A different OpenDocument or EPUB-adjacent type (.ods, .odp) declares
		// itself here too. Not accepted, and not worth probing further.
		return ""
	}
	// OOXML has no mimetype member; WordprocessingML is identified by its main
	// document part. .xlsx and .pptx carry xl/ and ppt/ parts instead, so they
	// fall through to "" and are rejected.
	if zipHasEntry(zr, "word/document.xml") {
		return docxContentType
	}
	return ""
}

// zipMimetype returns the contents of an uncompressed "mimetype" member, or ""
// when the archive has none. Only a STORED member is read, which is what both
// the ODF and EPUB specifications mandate — so this never decompresses.
func zipMimetype(zr *zip.Reader) string {
	for _, f := range zr.File {
		if f.Name != "mimetype" {
			continue
		}
		if f.Method != zip.Store {
			return ""
		}
		// The member is a short ASCII media type; anything longer is not one.
		if f.UncompressedSize64 > 128 {
			return ""
		}
		return readZipEntry(f)
	}
	return ""
}

// readZipEntry reads a small member in full, returning "" on any error.
func readZipEntry(f *zip.File) string {
	rc, err := f.Open()
	if err != nil {
		return ""
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rc); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

// zipHasEntry reports whether the archive's central directory lists name.
func zipHasEntry(zr *zip.Reader, name string) bool {
	for _, f := range zr.File {
		if f.Name == name {
			return true
		}
	}
	return false
}

// docCardTypeFor maps an accepted document content type onto a card type. An
// EPUB is a book; a word-processor file reads as an article, matching how
// uploaded PDFs are already classified.
func docCardTypeFor(contentType string) string {
	if contentType == epubContentType {
		return "book"
	}
	return "article"
}
