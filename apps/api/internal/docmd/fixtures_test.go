package docmd

import (
	"archive/zip"
	"bytes"
	"testing"
)

// Fixtures are built here rather than committed as binaries: the formats are
// container specs, so constructing them in code keeps them readable and
// editable. These are minimal-but-valid documents — they exercise the wasm
// boundary and our flattening, not anydoc's parser fidelity, which is
// anydoc's own concern.

const (
	fixtureDocxHeading = "Openmind Document Capture"
	fixtureDocxProse   = "The quick brown fox jumps over the lazy dog."
	fixtureODTHeading  = "Openmind ODT Fixture"
	fixtureODTProse    = "Sphinx of black quartz, judge my vow."
	fixtureEPUBTitle   = "Openmind EPUB Fixture"
	fixtureEPUBProse   = "Pack my box with five dozen liquor jugs."
	fixtureRTFProse    = "Openmind RTF fixture. How vexingly quick daft zebras jump!"
)

// fixtureRTF is plain text with an RTF wrapper — no container involved.
const fixtureRTF = `{\rtf1\ansi\deff0 {\fonttbl{\f0 Times New Roman;}}\f0\fs24 ` +
	fixtureRTFProse + `\par}`

// zipEntry is one member of a container fixture. stored forces STORE rather
// than DEFLATE, which ODT and EPUB require for their leading mimetype member.
type zipEntry struct {
	name   string
	body   string
	stored bool
}

// buildZip assembles entries into a ZIP archive.
func buildZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		method := zip.Deflate
		if e.stored {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: method})
		if err != nil {
			t.Fatalf("creating zip entry %s: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatalf("writing zip entry %s: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buf.Bytes()
}

// docxFixture builds a minimal WordprocessingML document.
func docxFixture(t *testing.T) []byte {
	t.Helper()
	const contentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`
	const rels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`
	const document = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t xml:space="preserve">` + fixtureDocxHeading + `</w:t></w:r></w:p>
<w:p><w:r><w:t xml:space="preserve">` + fixtureDocxProse + `</w:t></w:r></w:p>
</w:body></w:document>`

	return buildZip(t, []zipEntry{
		{name: "[Content_Types].xml", body: contentTypes},
		{name: "_rels/.rels", body: rels},
		{name: "word/document.xml", body: document},
	})
}

// odtFixture builds a minimal OpenDocument Text file.
func odtFixture(t *testing.T) []byte {
	t.Helper()
	const content = `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" office:version="1.2">
<office:body><office:text>
<text:h text:outline-level="1">` + fixtureODTHeading + `</text:h>
<text:p>` + fixtureODTProse + `</text:p>
</office:text></office:body></office:document-content>`

	return buildZip(t, []zipEntry{
		{name: "mimetype", body: "application/vnd.oasis.opendocument.text", stored: true},
		{name: "content.xml", body: content},
	})
}

// epubFixture builds a minimal EPUB 3 file with one chapter.
func epubFixture(t *testing.T) []byte {
	t.Helper()
	const container = `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
<rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	const opf = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:title>` + fixtureEPUBTitle + `</dc:title><dc:identifier id="bookid">urn:uuid:openmind-test</dc:identifier><dc:language>en</dc:language>
</metadata>
<manifest><item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/></manifest>
<spine><itemref idref="c1"/></spine></package>`
	const chapter = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Chapter One</title></head>
<body><h1>Chapter One</h1><p>` + fixtureEPUBProse + `</p></body></html>`

	return buildZip(t, []zipEntry{
		{name: "mimetype", body: "application/epub+zip", stored: true},
		{name: "META-INF/container.xml", body: container},
		{name: "OEBPS/content.opf", body: opf},
		{name: "OEBPS/ch1.xhtml", body: chapter},
	})
}
