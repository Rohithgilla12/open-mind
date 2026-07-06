package feeds

import (
	"errors"
	"testing"
)

func entryURLs(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.URL
	}
	return out
}

func TestParseRSS2(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example Blog</title>
    <link>https://blog.example.com</link>
    <description>A blog</description>
    <item>
      <title>First &amp; foremost</title>
      <link>https://blog.example.com/first</link>
    </item>
    <item>
      <title>Second post</title>
      <link>https://blog.example.com/second</link>
    </item>
  </channel>
</rss>`)
	feed, err := Parse(data, "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if feed.Title != "Example Blog" {
		t.Errorf("title = %q, want %q", feed.Title, "Example Blog")
	}
	if feed.SiteURL != "https://blog.example.com" {
		t.Errorf("siteURL = %q", feed.SiteURL)
	}
	got := entryURLs(feed.Entries)
	if len(got) != 2 || got[0] != "https://blog.example.com/first" || got[1] != "https://blog.example.com/second" {
		t.Fatalf("urls = %v", got)
	}
	if feed.Entries[0].Title != "First & foremost" {
		t.Errorf("entry title = %q, want entity-decoded", feed.Entries[0].Title)
	}
}

func TestParseAtom(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Example</title>
  <link href="https://atom.example.com/" rel="alternate"/>
  <link href="https://atom.example.com/feed.xml" rel="self"/>
  <entry>
    <title>Entry One</title>
    <link href="https://atom.example.com/one" rel="alternate"/>
    <link href="https://atom.example.com/one/comments" rel="replies"/>
  </entry>
  <entry>
    <title>Entry Two</title>
    <link href="https://atom.example.com/two"/>
  </entry>
</feed>`)
	feed, err := Parse(data, "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if feed.Title != "Atom Example" {
		t.Errorf("title = %q", feed.Title)
	}
	if feed.SiteURL != "https://atom.example.com/" {
		t.Errorf("siteURL = %q, want alternate link", feed.SiteURL)
	}
	got := entryURLs(feed.Entries)
	if len(got) != 2 || got[0] != "https://atom.example.com/one" || got[1] != "https://atom.example.com/two" {
		t.Fatalf("urls = %v", got)
	}
	if feed.Entries[0].Title != "Entry One" {
		t.Errorf("entry title = %q", feed.Entries[0].Title)
	}
}

func TestParseRSSRelativeLink(t *testing.T) {
	data := []byte(`<rss version="2.0"><channel>
    <title>Rel Blog</title>
    <link>/home</link>
    <item><title>Post</title><link>/post/1</link></item>
  </channel></rss>`)
	feed, err := Parse(data, "https://rel.example.com/base/")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := entryURLs(feed.Entries)
	if len(got) != 1 || got[0] != "https://rel.example.com/post/1" {
		t.Fatalf("urls = %v, want resolved against baseURL", got)
	}
	if feed.SiteURL != "https://rel.example.com/home" {
		t.Errorf("siteURL = %q, want resolved against baseURL", feed.SiteURL)
	}
}

func TestParseAtomRelativeLink(t *testing.T) {
	// No baseURL param; relative entry link resolves against the feed's own link.
	data := []byte(`<feed xmlns="http://www.w3.org/2005/Atom">
    <title>Rel Atom</title>
    <link href="https://relatom.example.com/blog/" rel="alternate"/>
    <entry><title>P</title><link href="post/1"/></entry>
  </feed>`)
	feed, err := Parse(data, "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := entryURLs(feed.Entries)
	if len(got) != 1 || got[0] != "https://relatom.example.com/blog/post/1" {
		t.Fatalf("urls = %v, want resolved against feed link", got)
	}
}

func TestParseSkipsNoLinkEntry(t *testing.T) {
	data := []byte(`<rss version="2.0"><channel>
    <title>Blog</title>
    <link>https://blog.example.com</link>
    <item><title>Has link</title><link>https://blog.example.com/a</link></item>
    <item><title>No link at all</title></item>
    <item><title>Non-http link</title><link>mailto:someone@example.com</link></item>
  </channel></rss>`)
	feed, err := Parse(data, "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := entryURLs(feed.Entries)
	if len(got) != 1 || got[0] != "https://blog.example.com/a" {
		t.Fatalf("urls = %v, want only the http(s) entry", got)
	}
}

func TestParseMalformedXML(t *testing.T) {
	data := []byte(`<rss version="2.0"><channel><title>Broken</title><item><link>https://x.example.com`)
	if _, err := Parse(data, ""); err == nil {
		t.Fatal("expected error for malformed XML, got nil")
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := Parse(nil, ""); err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
	if _, err := Parse([]byte("   \n  "), ""); err == nil {
		t.Fatal("expected error for whitespace-only input, got nil")
	}
}

func TestParseUnknownRoot(t *testing.T) {
	data := []byte(`<html><body><p>not a feed</p></body></html>`)
	_, err := Parse(data, "")
	if err == nil {
		t.Fatal("expected error for non-feed XML, got nil")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
}
