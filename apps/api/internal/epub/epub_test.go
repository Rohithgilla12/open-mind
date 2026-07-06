package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func buildTestDoc() Document {
	return Document{
		Title:  "Test Book",
		Author: "Jane Doe",
		Chapters: []Chapter{
			{
				Title: "Chapter One",
				Body:  "First paragraph.\n\nSecond paragraph.",
			},
			{
				Title: "Chapter Two",
				Body:  "<script>alert(1)</script>",
			},
		},
	}
}

func mustBuild(t *testing.T, doc Document) *zip.Reader {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := Build(buf, doc); err != nil {
		t.Fatalf("Build: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	return r
}

func readZipFile(t *testing.T, r *zip.Reader, name string) string {
	t.Helper()
	for _, f := range r.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			return string(data)
		}
	}
	t.Fatalf("file %s not found in zip", name)
	return ""
}

func TestBuild_MimetypeIsFirstStoredEntry(t *testing.T) {
	r := mustBuild(t, buildTestDoc())
	if len(r.File) == 0 {
		t.Fatal("empty zip")
	}
	first := r.File[0]
	if first.Name != "mimetype" {
		t.Fatalf("first entry = %q, want %q", first.Name, "mimetype")
	}
	if first.Method != zip.Store {
		t.Fatalf("mimetype method = %v, want zip.Store", first.Method)
	}
	content := readZipFile(t, r, "mimetype")
	if content != "application/epub+zip" {
		t.Fatalf("mimetype content = %q, want %q", content, "application/epub+zip")
	}
}

func TestBuild_ContainerXMLReferencesOPF(t *testing.T) {
	r := mustBuild(t, buildTestDoc())
	container := readZipFile(t, r, "META-INF/container.xml")
	if !strings.Contains(container, "OEBPS/content.opf") {
		t.Fatalf("container.xml does not reference OEBPS/content.opf: %s", container)
	}
}

func TestBuild_OPFContainsTitleManifestAndSpine(t *testing.T) {
	doc := buildTestDoc()
	r := mustBuild(t, doc)
	opf := readZipFile(t, r, "OEBPS/content.opf")

	if !strings.Contains(opf, "<dc:title>Test Book</dc:title>") {
		t.Fatalf("opf missing dc:title: %s", opf)
	}

	for i := range doc.Chapters {
		chapterFile := chapterFileName(i)
		if !strings.Contains(opf, chapterFile) {
			t.Fatalf("opf missing manifest/spine reference to %s: %s", chapterFile, opf)
		}
	}
	if !strings.Contains(opf, "nav.xhtml") {
		t.Fatalf("opf missing nav reference: %s", opf)
	}
}

func TestBuild_ChapterAndNavFilesAreWellFormedXML(t *testing.T) {
	doc := buildTestDoc()
	r := mustBuild(t, doc)

	names := []string{"OEBPS/nav.xhtml"}
	for i := range doc.Chapters {
		names = append(names, "OEBPS/"+chapterFileName(i))
	}

	for _, name := range names {
		content := readZipFile(t, r, name)
		dec := xml.NewDecoder(strings.NewReader(content))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s is not well-formed XML: %v", name, err)
			}
		}
	}
}

// TestBuild_XMLPrologIsRawNotEscaped guards against html/template mangling
// the "<?xml ...?>" prolog: if the prolog were rendered through the
// template (rather than written as raw bytes ahead of it), html/template's
// HTML5 tokenizer would treat "<?" as a bogus comment and HTML-escape it to
// "&lt;?xml ...", which is not well-formed XML even though Go's lenient
// encoding/xml decoder wouldn't complain.
func TestBuild_XMLPrologIsRawNotEscaped(t *testing.T) {
	doc := buildTestDoc()
	r := mustBuild(t, doc)

	const wantPrefix = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

	names := []string{"OEBPS/content.opf", "OEBPS/nav.xhtml"}
	for i := range doc.Chapters {
		names = append(names, "OEBPS/"+chapterFileName(i))
	}

	for _, name := range names {
		content := readZipFile(t, r, name)
		if !strings.HasPrefix(content, wantPrefix) {
			t.Fatalf("%s does not start with raw XML prolog %q: %s", name, wantPrefix, content)
		}
		if strings.Contains(content, "&lt;?") {
			t.Fatalf("%s contains an HTML-escaped XML prolog (&lt;?): %s", name, content)
		}
	}
}

func TestBuild_HTMLEscapesChapterBody(t *testing.T) {
	doc := buildTestDoc()
	r := mustBuild(t, doc)
	content := readZipFile(t, r, "OEBPS/"+chapterFileName(1))
	if strings.Contains(content, "<script>alert(1)</script>") {
		t.Fatalf("chapter body was not escaped: %s", content)
	}
	if !strings.Contains(content, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("chapter body missing expected escaped content: %s", content)
	}
}

func TestBuild_ParagraphsSplitOnBlankLines(t *testing.T) {
	doc := buildTestDoc()
	r := mustBuild(t, doc)
	content := readZipFile(t, r, "OEBPS/"+chapterFileName(0))
	if strings.Count(content, "<p>") != 2 {
		t.Fatalf("expected 2 <p> elements, got content: %s", content)
	}
	if !strings.Contains(content, "First paragraph.") || !strings.Contains(content, "Second paragraph.") {
		t.Fatalf("chapter missing expected paragraph text: %s", content)
	}
}

func TestBuild_EmptyChaptersProducesValidEPUB(t *testing.T) {
	doc := Document{Title: "Empty", Author: "Nobody"}
	r := mustBuild(t, doc)
	opf := readZipFile(t, r, "OEBPS/content.opf")
	if !strings.Contains(opf, "<dc:title>Empty</dc:title>") {
		t.Fatalf("opf missing title: %s", opf)
	}
}

func TestBuild_DeterministicID(t *testing.T) {
	doc := buildTestDoc()
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}
	if err := Build(buf1, doc); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := Build(buf2, doc); err != nil {
		t.Fatalf("Build: %v", err)
	}
	r1, _ := zip.NewReader(bytes.NewReader(buf1.Bytes()), int64(buf1.Len()))
	r2, _ := zip.NewReader(bytes.NewReader(buf2.Bytes()), int64(buf2.Len()))
	opf1 := readZipFile(t, r1, "OEBPS/content.opf")
	opf2 := readZipFile(t, r2, "OEBPS/content.opf")
	if opf1 != opf2 {
		t.Fatalf("Build is not deterministic for identical input")
	}
	if !strings.Contains(opf1, "urn:openmind:") {
		t.Fatalf("opf missing urn:openmind: identifier scheme: %s", opf1)
	}
}
