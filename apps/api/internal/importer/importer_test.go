package importer

import (
	"reflect"
	"testing"
)

func urls(links []Link) []string {
	out := make([]string, len(links))
	for i, l := range links {
		out[i] = l.URL
	}
	return out
}

func TestParseNetscapeHTML(t *testing.T) {
	data := []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
  <DT><A HREF="https://example.com/a" ADD_DATE="1700000000" TAGS="go, rust">First &amp; foremost</A>
  <DT><A TAGS="reading" HREF="https://example.com/b">Second</A>
  <DT><A HREF="https://example.com/c">No tags</A>
  <DT><A HREF="">empty href skipped</A>
</DL>`)
	links, err := Parse("pocket_export.html", data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := urls(links); len(got) != 3 || got[0] != "https://example.com/a" || got[1] != "https://example.com/b" || got[2] != "https://example.com/c" {
		t.Fatalf("urls = %v", got)
	}
	if links[0].Title != "First & foremost" {
		t.Errorf("title = %q, want entity-decoded", links[0].Title)
	}
	if got := links[0].Tags; !reflect.DeepEqual(got, []string{"go", "rust"}) {
		t.Errorf("tags[0] = %v, want [go rust]", got)
	}
	// TAGS before HREF must still parse (attribute order is irrelevant).
	if got := links[1].Tags; !reflect.DeepEqual(got, []string{"reading"}) {
		t.Errorf("tags[1] = %v, want [reading]", got)
	}
	// No TAGS attribute → nil.
	if links[2].Tags != nil {
		t.Errorf("tags[2] = %v, want nil", links[2].Tags)
	}
}

func TestParseCSV(t *testing.T) {
	data := []byte("title,url,time_added,tags\n" +
		"Hello World,https://example.com/x,1700000000,go|rust\n" +
		"No URL row,,123,\n" +
		"Another,https://example.com/y,124,\n")
	links, err := Parse("pocket.csv", data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := urls(links)
	if len(got) != 2 || got[0] != "https://example.com/x" || got[1] != "https://example.com/y" {
		t.Fatalf("urls = %v", got)
	}
	if links[0].Title != "Hello World" {
		t.Errorf("title = %q", links[0].Title)
	}
	if got := links[0].Tags; !reflect.DeepEqual(got, []string{"go", "rust"}) {
		t.Errorf("tags[0] = %v, want [go rust]", got)
	}
	// Row with an empty tags cell → nil.
	if links[1].Tags != nil {
		t.Errorf("tags[1] = %v, want nil", links[1].Tags)
	}
}

func TestParseCSVRaindropSpaceSeparatedTags(t *testing.T) {
	// Raindrop separates tags with spaces (no comma in the cell).
	data := []byte("url,title,tags\nhttps://example.com/r,Read this,go rust web\n")
	links, err := Parse("raindrop.csv", data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("links = %d, want 1", len(links))
	}
	if got := links[0].Tags; !reflect.DeepEqual(got, []string{"go", "rust", "web"}) {
		t.Errorf("tags = %v, want [go rust web]", got)
	}
}

func TestParseCSVDetectedByContent(t *testing.T) {
	// No .csv extension, but a header naming a URL column.
	data := []byte("id,URL,folder\n1,https://example.com/z,inbox\n")
	links, err := Parse("export.txt", data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := urls(links); len(got) != 1 || got[0] != "https://example.com/z" {
		t.Fatalf("urls = %v", got)
	}
	// No tags column → nil.
	if links[0].Tags != nil {
		t.Errorf("tags = %v, want nil", links[0].Tags)
	}
}

func TestParsePlainTextURLs(t *testing.T) {
	data := []byte("https://example.com/1\n\n  https://example.com/2  \nnot a url\nftp://skip.me\n")
	links, err := Parse("urls.txt", data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := urls(links)
	if len(got) != 2 || got[0] != "https://example.com/1" || got[1] != "https://example.com/2" {
		t.Fatalf("urls = %v", got)
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := Parse("empty.txt", []byte("nothing here\njust prose\n")); err != ErrEmpty {
		t.Errorf("err = %v, want ErrEmpty", err)
	}
}
