// Package importer parses bookmark/read-later export files into a flat list of
// links the API can turn into saved items. It recognises the Netscape bookmark
// HTML format (browsers, Pocket, Raindrop, Pinboard, Instapaper), CSV exports
// (Pocket's current export, Raindrop — a folder/collection column becomes a
// tag), a plain newline-delimited URL list, and Omnivore export zips
// (metadata_*.json pages; labels become tags).
//
// It only extracts candidate links; URL validation, de-duplication, and item
// creation are the caller's job. No network, no AI — parsing is pure.
package importer

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"html"
	"io"
	"path"
	"regexp"
	"strings"
)

// Link is a single parsed entry. Title is best-effort and may be empty; the
// enrichment pipeline is the source of truth for titles, so callers may ignore
// it. URL is always non-empty for a returned Link. Tags carries any tags the
// source format preserved (bookmark TAGS attribute, CSV tags column) and is nil
// when the entry had none; the caller canonicalises and stores them as user tags.
type Link struct {
	URL   string
	Title string
	Tags  []string
}

// ErrEmpty is returned when a file parses successfully but yields no links.
var ErrEmpty = errors.New("no links found in file")

// anchorRe matches a Netscape-bookmark <A …>title</A>: group 1 is the tag's
// attribute text, group 2 the inner label. Case-insensitive, dot-matches-newline.
var anchorRe = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)

// hrefRe pulls the href value out of an anchor's attribute text.
var hrefRe = regexp.MustCompile(`(?is)\bhref\s*=\s*"([^"]*)"`)

// tagsRe pulls the TAGS attribute value out of an anchor's attribute text.
// Pocket, Raindrop, and Firefox emit TAGS="a,b,c". Because it reads from the
// attribute group, the attribute may appear before or after HREF.
var tagsRe = regexp.MustCompile(`(?is)\btags\s*=\s*"([^"]*)"`)

// tagStripRe removes any nested markup from an anchor label.
var tagStripRe = regexp.MustCompile(`(?is)<[^>]+>`)

// urlLineRe matches a bare http(s) URL used by the plain-text fallback.
var urlLineRe = regexp.MustCompile(`(?i)^\s*(https?://\S+)\s*$`)

// zipMagic is the zip local-file-header signature; Omnivore exports are zips.
var zipMagic = []byte("PK\x03\x04")

const (
	// omnivoreMaxEntryBytes skips any single zip entry larger than this when
	// decompressed — metadata pages are well under 1 MB, so anything bigger is
	// not a metadata file we want in memory.
	omnivoreMaxEntryBytes = 8 << 20
	// omnivoreMaxLinks stops parsing once this many links are collected,
	// mirroring the API handler's per-import cap.
	omnivoreMaxLinks = 10000
)

// Parse detects the format from the filename and content and returns the links
// it finds. The result preserves file order; callers de-duplicate.
func Parse(filename string, data []byte) ([]Link, error) {
	name := strings.ToLower(strings.TrimSpace(filename))
	looksHTML := strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".htm") ||
		bytes.Contains(bytes.ToLower(data), []byte("<a ")) ||
		bytes.Contains(bytes.ToLower(data), []byte("netscape-bookmark"))

	var links []Link
	switch {
	case strings.HasSuffix(name, ".zip") || bytes.HasPrefix(data, zipMagic):
		links = parseOmnivoreZip(data)
	case looksHTML:
		links = parseHTML(data)
	case strings.HasSuffix(name, ".csv") || looksCSV(data):
		links = parseCSV(data)
	default:
		links = parseText(data)
	}
	if len(links) == 0 {
		return nil, ErrEmpty
	}
	return links, nil
}

// parseHTML extracts links from a Netscape bookmark file's <A HREF> anchors.
func parseHTML(data []byte) []Link {
	var out []Link
	for _, m := range anchorRe.FindAllSubmatch(data, -1) {
		hm := hrefRe.FindSubmatch(m[1])
		if hm == nil {
			continue
		}
		url := html.UnescapeString(string(hm[1]))
		if url == "" {
			continue
		}
		title := strings.TrimSpace(html.UnescapeString(string(tagStripRe.ReplaceAll(m[2], nil))))
		var tags []string
		if tm := tagsRe.FindSubmatch(m[1]); tm != nil {
			tags = splitTags(html.UnescapeString(string(tm[1])))
		}
		out = append(out, Link{URL: url, Title: title, Tags: tags})
	}
	return out
}

// splitTags parses a delimited tag string into trimmed, non-empty tags,
// returning nil when there are none. Netscape TAGS and Pocket/Raindrop CSV
// cells are comma-separated; Pocket's CSV uses "|" and Raindrop may use spaces,
// so when no comma is present we fall back to splitting on "|" then whitespace.
func splitTags(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var parts []string
	switch {
	case strings.Contains(s, ","):
		parts = strings.Split(s, ",")
	case strings.Contains(s, "|"):
		parts = strings.Split(s, "|")
	default:
		parts = strings.Fields(s)
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// looksCSV reports whether the first line looks like a CSV header naming a URL
// column — used when the filename gives no hint.
func looksCSV(data []byte) bool {
	line := data
	if i := bytes.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if !bytes.ContainsRune(line, ',') {
		return false
	}
	return bytes.Contains(bytes.ToLower(line), []byte("url"))
}

// parseCSV reads a CSV export, locating the URL column (and optional title,
// tags, and folder columns) by header name. Rows without a URL cell are
// skipped. A folder cell (Raindrop's collection) is appended to the row's tags
// so the user's organisation survives the import — except "Unsorted",
// Raindrop's default bucket, which would just be noise on most rows.
func parseCSV(data []byte) []Link {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1 // exports are ragged; don't enforce column counts.
	header, err := r.Read()
	if err != nil {
		return nil
	}
	urlCol, titleCol, tagsCol, folderCol := -1, -1, -1, -1
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "url", "uri", "link":
			if urlCol == -1 {
				urlCol = i
			}
		case "title", "name":
			if titleCol == -1 {
				titleCol = i
			}
		case "tags":
			if tagsCol == -1 {
				tagsCol = i
			}
		case "folder":
			if folderCol == -1 {
				folderCol = i
			}
		}
	}
	if urlCol == -1 {
		return nil
	}
	var out []Link
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if urlCol >= len(rec) {
			continue
		}
		url := strings.TrimSpace(rec[urlCol])
		if url == "" {
			continue
		}
		var title string
		if titleCol >= 0 && titleCol < len(rec) {
			title = strings.TrimSpace(rec[titleCol])
		}
		var tags []string
		if tagsCol >= 0 && tagsCol < len(rec) {
			tags = splitTags(rec[tagsCol])
		}
		if folderCol >= 0 && folderCol < len(rec) {
			// The whole cell is one tag: folder names may contain spaces, and a
			// nested path ("Dev/Go") is more useful intact than split.
			if folder := strings.TrimSpace(rec[folderCol]); folder != "" && !strings.EqualFold(folder, "unsorted") {
				tags = append(tags, folder)
			}
		}
		out = append(out, Link{URL: url, Title: title, Tags: tags})
	}
	return out
}

// parseText treats the file as a newline-delimited list of URLs (one per line),
// ignoring blank lines and anything that isn't a bare http(s) URL.
func parseText(data []byte) []Link {
	var out []Link
	for _, line := range strings.Split(string(data), "\n") {
		if m := urlLineRe.FindStringSubmatch(line); m != nil {
			out = append(out, Link{URL: strings.TrimSpace(m[1])})
		}
	}
	return out
}

// omnivoreLabel is one entry of an Omnivore metadata "labels" array. The
// export writes plain strings, but Omnivore's API shape used {"name": ...}
// objects, so both are accepted.
type omnivoreLabel struct {
	Name string
}

func (l *omnivoreLabel) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		l.Name = s
		return nil
	}
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	l.Name = obj.Name
	return nil
}

// omnivoreEntry is one saved page in an Omnivore metadata_*.json array.
type omnivoreEntry struct {
	URL    string          `json:"url"`
	Title  string          `json:"title"`
	State  string          `json:"state"`
	Labels []omnivoreLabel `json:"labels"`
}

// parseOmnivoreZip reads an Omnivore export zip, collecting links from every
// metadata_*.json entry. content/ and highlights/ entries are ignored (the
// archived bodies are a future slice). Malformed or oversized entries are
// skipped rather than failing the whole import.
func parseOmnivoreZip(data []byte) []Link {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}
	var out []Link
	for _, f := range zr.File {
		base := strings.ToLower(path.Base(f.Name))
		if !strings.HasPrefix(base, "metadata_") || !strings.HasSuffix(base, ".json") {
			continue
		}
		if f.UncompressedSize64 > omnivoreMaxEntryBytes {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(rc, omnivoreMaxEntryBytes+1))
		_ = rc.Close()
		if err != nil || len(raw) > omnivoreMaxEntryBytes {
			continue
		}
		var entries []omnivoreEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			continue
		}
		for _, e := range entries {
			if e.URL == "" || e.State == "DELETED" {
				continue
			}
			var tags []string
			for _, l := range e.Labels {
				if name := strings.TrimSpace(l.Name); name != "" {
					tags = append(tags, name)
				}
			}
			out = append(out, Link{URL: e.URL, Title: strings.TrimSpace(e.Title), Tags: tags})
			if len(out) >= omnivoreMaxLinks {
				return out
			}
		}
	}
	return out
}
