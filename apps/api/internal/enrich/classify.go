package enrich

import (
	"net/url"
	"path"
	"strings"
)

// imageExts are file extensions that classify a URL as an image card.
var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".svg":  true,
	".avif": true,
}

// Classify maps a saved URL (and its extracted content) to a card type using
// host, path, and extension heuristics. It defaults to "article".
func Classify(rawURL string, _ Extraction) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "article"
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))

	switch host {
	case "youtube.com", "m.youtube.com", "youtu.be":
		return "video"
	case "x.com", "twitter.com", "mobile.twitter.com":
		return "tweet"
	}

	if imageExts[strings.ToLower(path.Ext(parsed.Path))] {
		return "image"
	}

	return "article"
}
