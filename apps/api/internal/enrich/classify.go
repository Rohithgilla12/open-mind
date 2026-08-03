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

// codeHosts are the code forges whose repository URLs classify as repo cards.
// A host alone is not enough: the same hosts serve profiles and marketing
// pages, so the path must also be repository-shaped (see isRepoPath).
var codeHosts = map[string]bool{
	"github.com":    true,
	"gitlab.com":    true,
	"codeberg.org":  true,
	"bitbucket.org": true,
}

// reservedOwners are first path segments the code hosts reserve for their own
// routes, so none can ever be a repository owner. Without this, two-segment
// product pages like github.com/features/copilot would read as repositories.
// "-" covers GitLab's /-/ infix routes.
var reservedOwners = map[string]bool{
	"about": true, "apps": true, "collections": true, "contact": true,
	"enterprise": true, "explore": true, "features": true, "join": true,
	"login": true, "marketplace": true, "notifications": true, "orgs": true,
	"pricing": true, "pulls": true, "readme": true, "security": true,
	"settings": true, "sponsors": true, "topics": true, "trending": true,
	"-": true,
}

// isRepoPath reports whether p is repository-shaped: at least two non-empty
// segments whose first is not a reserved host route. Sub-pages keep the repo
// type — an issue, a pull request, or a blob is still that project.
func isRepoPath(p string) bool {
	segs := make([]string, 0, 4)
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) < 2 {
		return false
	}
	return !reservedOwners[strings.ToLower(segs[0])]
}

// Classify maps a saved URL (and its extracted content) to a card type using
// host, path, and extension heuristics. It defaults to "article".
func Classify(rawURL string, _ Extraction) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "article"
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")

	switch host {
	case "youtube.com", "m.youtube.com", "youtu.be":
		return "video"
	case "x.com", "twitter.com", "mobile.twitter.com":
		return "tweet"
	}
	if _, ok := socialVideoHosts[host]; ok {
		return "video"
	}

	// Before the extension check on purpose: a .png under /blob/ is an HTML
	// page on the forge, not an image file.
	if codeHosts[host] && isRepoPath(parsed.Path) {
		return "repo"
	}

	if imageExts[strings.ToLower(path.Ext(parsed.Path))] {
		return "image"
	}

	return "article"
}
