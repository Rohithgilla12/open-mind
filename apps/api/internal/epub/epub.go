// Package epub builds minimal, valid EPUB 3 files from plain-text chapters
// using only the standard library.
package epub

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"strings"
)

// Chapter is a single chapter of an EPUB document. Body is plain text;
// paragraphs are split on blank lines and HTML-escaped on render.
type Chapter struct {
	Title string
	Body  string
}

// Document describes the book to build.
type Document struct {
	Title    string
	Author   string
	Chapters []Chapter
}

// xmlProlog is written as raw bytes directly to each zip entry, ahead of the
// html/template output. html/template's HTML5 tokenizer treats a leading
// "<?xml ...?>" as a bogus comment and HTML-escapes it (producing
// "&lt;?xml ...") if it's part of the template source, so the prolog must
// never appear inside a template — it's prepended to the rendered bytes
// instead.
const xmlProlog = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

const containerXML = xmlProlog + `<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`

var chapterTemplate = template.Must(template.New("chapter").Parse(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="en">
<head>
  <title>{{.Title}}</title>
  <meta charset="UTF-8"/>
</head>
<body>
  <h1>{{.Title}}</h1>
{{range .Paragraphs}}  <p>{{.}}</p>
{{end}}</body>
</html>
`))

var navTemplate = template.Must(template.New("nav").Parse(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="en">
<head>
  <title>Table of Contents</title>
  <meta charset="UTF-8"/>
</head>
<body>
  <nav epub:type="toc" id="toc">
    <ol>
{{range .Chapters}}      <li><a href="{{.File}}">{{.Title}}</a></li>
{{end}}    </ol>
  </nav>
</body>
</html>
`))

var opfTemplate = template.Must(template.New("opf").Parse(`<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="book-id" xml:lang="en">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="book-id">urn:openmind:{{.ID}}</dc:identifier>
    <dc:title>{{.Title}}</dc:title>
    <dc:creator>{{.Author}}</dc:creator>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
{{range .Chapters}}    <item id="{{.ID}}" href="{{.File}}" media-type="application/xhtml+xml"/>
{{end}}  </manifest>
  <spine>
{{range .Chapters}}    <itemref idref="{{.ID}}"/>
{{end}}  </spine>
</package>
`))

type chapterView struct {
	ID    string
	File  string
	Title string
}

type opfView struct {
	ID       string
	Title    string
	Author   string
	Chapters []chapterView
}

func chapterFileName(index int) string {
	return fmt.Sprintf("chapter-%d.xhtml", index+1)
}

func chapterID(index int) string {
	return fmt.Sprintf("chapter-%d", index+1)
}

// paragraphs splits plain text into paragraphs on blank lines, trimming
// whitespace and skipping empties.
func paragraphs(body string) []string {
	parts := strings.Split(body, "\n\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// documentID derives a deterministic, opaque identifier from the document's
// title, author, and chapters so repeated builds of the same content produce
// byte-identical output. It is deliberately not shaped like a UUID (the
// SHA-256 digest doesn't carry valid RFC 4122 version/variant bits) — it's
// used under the "urn:openmind:" scheme instead of "urn:uuid:".
func documentID(doc Document) string {
	h := sha256.New()
	h.Write([]byte(doc.Title))
	h.Write([]byte{0})
	h.Write([]byte(doc.Author))
	for _, c := range doc.Chapters {
		h.Write([]byte{0})
		h.Write([]byte(c.Title))
		h.Write([]byte{0})
		h.Write([]byte(c.Body))
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

// Build writes an EPUB 3 archive for doc to w.
func Build(w io.Writer, doc Document) error {
	zw := zip.NewWriter(w)

	if err := writeMimetype(zw); err != nil {
		return fmt.Errorf("writing mimetype: %w", err)
	}

	if err := writeDeflated(zw, "META-INF/container.xml", []byte(containerXML)); err != nil {
		return fmt.Errorf("writing container.xml: %w", err)
	}

	chapterViews := make([]chapterView, len(doc.Chapters))
	for i, c := range doc.Chapters {
		chapterViews[i] = chapterView{
			ID:    chapterID(i),
			File:  chapterFileName(i),
			Title: c.Title,
		}
	}

	opf := opfView{
		ID:       documentID(doc),
		Title:    doc.Title,
		Author:   doc.Author,
		Chapters: chapterViews,
	}
	var opfBuf strings.Builder
	opfBuf.WriteString(xmlProlog)
	if err := opfTemplate.Execute(&opfBuf, opf); err != nil {
		return fmt.Errorf("rendering content.opf: %w", err)
	}
	if err := writeDeflated(zw, "OEBPS/content.opf", []byte(opfBuf.String())); err != nil {
		return fmt.Errorf("writing content.opf: %w", err)
	}

	var navBuf strings.Builder
	navBuf.WriteString(xmlProlog)
	if err := navTemplate.Execute(&navBuf, opf); err != nil {
		return fmt.Errorf("rendering nav.xhtml: %w", err)
	}
	if err := writeDeflated(zw, "OEBPS/nav.xhtml", []byte(navBuf.String())); err != nil {
		return fmt.Errorf("writing nav.xhtml: %w", err)
	}

	for i, c := range doc.Chapters {
		var chBuf strings.Builder
		chBuf.WriteString(xmlProlog)
		data := struct {
			Title      string
			Paragraphs []string
		}{
			Title:      c.Title,
			Paragraphs: paragraphs(c.Body),
		}
		if err := chapterTemplate.Execute(&chBuf, data); err != nil {
			return fmt.Errorf("rendering %s: %w", chapterFileName(i), err)
		}
		if err := writeDeflated(zw, "OEBPS/"+chapterFileName(i), []byte(chBuf.String())); err != nil {
			return fmt.Errorf("writing %s: %w", chapterFileName(i), err)
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("closing epub archive: %w", err)
	}
	return nil
}

// writeMimetype writes the mandatory first entry of an EPUB archive,
// uncompressed, as required by the EPUB OCF specification.
func writeMimetype(zw *zip.Writer) error {
	hdr := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	fw, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = fw.Write([]byte("application/epub+zip"))
	return err
}

func writeDeflated(zw *zip.Writer, name string, data []byte) error {
	hdr := &zip.FileHeader{
		Name:   name,
		Method: zip.Deflate,
	}
	fw, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = fw.Write(data)
	return err
}
