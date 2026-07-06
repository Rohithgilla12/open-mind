package importer

import "testing"

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
  <DT><A HREF="https://example.com/a" ADD_DATE="1700000000" TAGS="go,work">First &amp; foremost</A>
  <DT><A HREF="https://example.com/b">Second</A>
  <DT><A HREF="">empty href skipped</A>
</DL>`)
	links, err := Parse("pocket_export.html", data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := urls(links); len(got) != 2 || got[0] != "https://example.com/a" || got[1] != "https://example.com/b" {
		t.Fatalf("urls = %v", got)
	}
	if links[0].Title != "First & foremost" {
		t.Errorf("title = %q, want entity-decoded", links[0].Title)
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
