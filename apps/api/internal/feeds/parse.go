// Package feeds parses RSS 2.0 and Atom feed documents into a normalised shape
// the API can turn into saved items. It uses only the standard library
// (encoding/xml, net/url) — no external dependency — matching the importer's
// dependency-free ethos.
//
// Parsing is pure: no network and no AI. RSS 1.0/RDF and podcast-specific tags
// are out of scope; the parser recognises them enough to sniff the root element
// but only extracts titles and links.
package feeds

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Feed is a parsed feed document. Title and SiteURL are best-effort and may be
// empty; Entries contains only entries with a usable http(s) link.
type Feed struct {
	Title   string
	SiteURL string
	Entries []Entry
}

// Entry is a single feed entry. URL is always a non-empty http(s) link for a
// returned Entry; Title is best-effort and may be empty.
type Entry struct {
	URL   string
	Title string
}

// ErrEmpty is returned when the input is empty or contains no XML.
var ErrEmpty = errors.New("empty feed document")

// ErrUnsupported is returned when the root element is neither an RSS/RDF feed
// nor an Atom feed.
var ErrUnsupported = errors.New("unsupported feed format")

// Parse decodes an RSS 2.0 or Atom document. Relative links (the site link and
// entry links) are resolved against baseURL when it is a valid absolute URL,
// otherwise against the feed's own site link. Entries without a usable http(s)
// link are skipped. Malformed or non-feed XML yields a wrapped error.
func Parse(data []byte, baseURL string) (Feed, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Feed{}, fmt.Errorf("parsing feed: %w", ErrEmpty)
	}

	root, err := sniffRoot(data)
	if err != nil {
		return Feed{}, fmt.Errorf("parsing feed: %w", err)
	}

	var base *url.URL
	if s := strings.TrimSpace(baseURL); s != "" {
		if u, err := url.Parse(s); err == nil && u.IsAbs() {
			base = u
		}
	}

	switch root {
	case "rss", "rdf":
		return parseRSS(data, base)
	case "feed":
		return parseAtom(data, base)
	default:
		return Feed{}, fmt.Errorf("parsing feed: %w", ErrUnsupported)
	}
}

// sniffRoot returns the lower-cased local name of the first start element, so
// the caller can pick the right shape. A document with no start element (not
// XML at all) is reported as empty.
func sniffRoot(data []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", ErrEmpty
		}
		if se, ok := tok.(xml.StartElement); ok {
			return strings.ToLower(se.Name.Local), nil
		}
	}
}

// rssItem is an <item> in an RSS/RDF document.
type rssItem struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

// rssDoc covers RSS 2.0 (items nested under <channel>) and RSS 1.0/RDF (items
// as siblings of <channel>).
type rssDoc struct {
	Channel struct {
		Title string    `xml:"title"`
		Link  string    `xml:"link"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	TopItems []rssItem `xml:"item"`
}

func parseRSS(data []byte, base *url.URL) (Feed, error) {
	var doc rssDoc
	if err := decode(data, &doc); err != nil {
		return Feed{}, fmt.Errorf("parsing rss: %w", err)
	}

	feed := Feed{Title: strings.TrimSpace(doc.Channel.Title)}
	feed.SiteURL = resolve(base, doc.Channel.Link)

	items := doc.Channel.Items
	if len(items) == 0 {
		items = doc.TopItems
	}

	entryBase := entryBase(base, feed.SiteURL)
	for _, it := range items {
		if link := resolve(entryBase, it.Link); link != "" {
			feed.Entries = append(feed.Entries, Entry{URL: link, Title: strings.TrimSpace(it.Title)})
		}
	}
	return feed, nil
}

// atomLink is an Atom <link> element.
type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomDoc struct {
	Title   string     `xml:"title"`
	Links   []atomLink `xml:"link"`
	Entries []struct {
		Title string     `xml:"title"`
		Links []atomLink `xml:"link"`
	} `xml:"entry"`
}

func parseAtom(data []byte, base *url.URL) (Feed, error) {
	var doc atomDoc
	if err := decode(data, &doc); err != nil {
		return Feed{}, fmt.Errorf("parsing atom: %w", err)
	}

	feed := Feed{Title: strings.TrimSpace(doc.Title)}
	feed.SiteURL = pickLink(doc.Links, base)

	entryBase := entryBase(base, feed.SiteURL)
	for _, e := range doc.Entries {
		if link := pickLink(e.Links, entryBase); link != "" {
			feed.Entries = append(feed.Entries, Entry{URL: link, Title: strings.TrimSpace(e.Title)})
		}
	}
	return feed, nil
}

// pickLink chooses an Atom link: it prefers rel="alternate" (or an empty rel)
// that resolves to an http(s) URL, then falls back to the first link that does.
func pickLink(links []atomLink, base *url.URL) string {
	for _, l := range links {
		switch strings.ToLower(strings.TrimSpace(l.Rel)) {
		case "alternate", "":
			if u := resolve(base, l.Href); u != "" {
				return u
			}
		}
	}
	for _, l := range links {
		if u := resolve(base, l.Href); u != "" {
			return u
		}
	}
	return ""
}

// entryBase picks the base for resolving entry links: the provided baseURL when
// present, otherwise the feed's own (already-resolved) site link.
func entryBase(base *url.URL, siteURL string) *url.URL {
	if base != nil {
		return base
	}
	if siteURL != "" {
		if u, err := url.Parse(siteURL); err == nil && u.IsAbs() {
			return u
		}
	}
	return nil
}

// resolve trims ref, resolves it against base when base is set, and returns the
// result only if it is an http(s) URL; otherwise it returns "".
func resolve(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.String()
}

// decode unmarshals an XML document permissively so minor feed quirks don't
// abort parsing; truncated or genuinely broken XML still errors.
func decode(data []byte, v any) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	return dec.Decode(v)
}
