package enrich

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/net/html"

	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// socialVideoHosts maps the hosts of caption-driven social-video platforms
// (reels, TikToks) to a human label used for the degraded card title. These
// pages are JS-rendered and often login-walled, so the generic article
// extractor is useless on them — but their server-rendered HTML carries the
// caption in OpenGraph meta tags, which is where place extraction reads from.
var socialVideoHosts = map[string]string{
	"instagram.com": "Instagram reel",
	"instagr.am":    "Instagram reel",
	"tiktok.com":    "TikTok video",
	"vm.tiktok.com": "TikTok video",
	"vt.tiktok.com": "TikTok video",
}

// IsSocialVideoURL reports whether the URL belongs to a social-video platform
// whose items go through the OG extractor and qualify for place extraction.
func IsSocialVideoURL(rawURL string) bool {
	_, ok := socialVideoLabel(rawURL)
	return ok
}

// socialVideoLabel returns the degraded-card label for a social-video URL.
func socialVideoLabel(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
	label, ok := socialVideoHosts[host]
	return label, ok
}

// extractOpenGraph fetches a page and returns its og:title, og:description,
// and og:image. Social-video pages serve these to anonymous agents even when
// the content itself sits behind a login wall; og:description carries the
// caption, which becomes the item body (searchable, summarisable, and the
// input to place extraction).
func extractOpenGraph(ctx context.Context, client *http.Client, rawURL string) (Extraction, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Extraction{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "openmind/0.1 (+https://github.com/rohithgilla12/open-mind)")
	resp, err := client.Do(req)
	if err != nil {
		return Extraction{}, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return Extraction{}, fmt.Errorf("fetching %s: status %d", rawURL, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Extraction{}, fmt.Errorf("reading %s: %w", rawURL, err)
	}
	og, err := parseOpenGraph(bytes.NewReader(raw))
	if err != nil {
		return Extraction{}, fmt.Errorf("parsing %s: %w", rawURL, err)
	}
	og.TaggedLocation = parseTaggedLocation(raw)
	return og, nil
}

// locationNameRe matches an inline-JSON tagged location, e.g. Instagram's
// GraphQL blob `"location":{"id":"…","name":"Blue Bottle Coffee"}`. Best-effort
// and opportunistic: most login-walled pages omit it, which is fine.
var locationNameRe = regexp.MustCompile(`"location":\{[^{}]*?"name":"([^"]{1,200})"`)

// parseTaggedLocation returns the first inline-JSON tagged-location name in the
// page, JSON-unescaped, or "" when none is present.
func parseTaggedLocation(raw []byte) string {
	m := locationNameRe.FindSubmatch(raw)
	if m == nil {
		return ""
	}
	name := string(m[1])
	// The capture is a raw JSON string body; unescape \uXXXX and friends.
	if unq, err := strconv.Unquote(`"` + name + `"`); err == nil {
		name = unq
	}
	return strings.TrimSpace(name)
}

// parseOpenGraph tokenises HTML and collects the first og:title,
// og:description, and og:image meta tags. Tokenising (rather than building a
// full tree) keeps memory flat on large pages, and meta tags live in <head>,
// so the scan usually ends early.
func parseOpenGraph(r io.Reader) (Extraction, error) {
	var ex Extraction
	tok := html.NewTokenizer(r)
	for {
		switch tok.Next() {
		case html.ErrorToken:
			if tok.Err() == io.EOF {
				return ex, nil
			}
			return ex, tok.Err()
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := tok.TagName()
			switch string(name) {
			case "meta":
				var property, content string
				for hasAttr {
					var key, val []byte
					key, val, hasAttr = tok.TagAttr()
					switch string(key) {
					case "property", "name":
						property = string(val)
					case "content":
						content = string(val)
					}
				}
				switch property {
				case "og:title":
					if ex.Title == "" {
						ex.Title = content
					}
				case "og:description":
					if ex.Body == "" {
						ex.Body = content
					}
				case "og:image":
					if ex.LeadImageURL == "" {
						ex.LeadImageURL = content
					}
				}
			case "body":
				// Meta tags live in <head>; once the body starts there is
				// nothing left to find.
				return ex, nil
			}
		}
		if ex.Title != "" && ex.Body != "" && ex.LeadImageURL != "" {
			return ex, nil
		}
	}
}

// runSocialVideo enriches a social-video item (Instagram reel, TikTok): OG
// meta tags instead of article extraction, always classified as a video card.
// A fetch/parse failure or a login-wall page with no tags degrades to a bare
// card named after the platform — never a failed item, so a re-run (or a
// later phase with better signals) can only improve it.
func (p *Pipeline) runSocialVideo(ctx context.Context, userID uuid.UUID, item db.Item) error {
	q := p.Store.Queries
	label, _ := socialVideoLabel(item.Url)

	ex, err := extractOpenGraph(ctx, p.httpClient(), item.Url)
	if err != nil {
		slog.Warn("social video: og extraction failed, degrading to bare card", "item_id", item.ID, "err", err)
		ex = Extraction{}
	}
	ex.Title, ex.Body = normalizeSocialVideo(ex.Title, ex.Body, label)

	if err := q.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: item.ID,
		Title: ex.Title, Body: ex.Body, LeadImageUrl: ex.LeadImageURL, CardType: "video",
		TaggedLocation: ex.TaggedLocation,
	}); err != nil {
		return fmt.Errorf("saving social video extraction: %w", err)
	}
	return p.enrichText(ctx, userID, item.ID, ex.Title, ex.Body)
}

// socialVideoTitleMax is the display cap for reel/TikTok card titles. Matches
// the spirit of noteTitle (80) with a little room for an "Author: …" prefix.
const socialVideoTitleMax = 100

// normalizeSocialVideo turns noisy OpenGraph metadata into a short card title
// and a caption body. Instagram often stuffs the full caption into og:title as
// `Name on Instagram: 'caption…'` while og:description may or may not repeat
// it — without this, the detail screen renders a multi-paragraph serif "title".
func normalizeSocialVideo(ogTitle, ogBody, label string) (title, body string) {
	body = strings.TrimSpace(ogBody)
	raw := strings.TrimSpace(ogTitle)

	author, caption := splitSocialVideoTitle(raw)
	if body == "" {
		body = caption
	}
	title = socialVideoTitle(author, caption, label)
	return title, body
}

// splitSocialVideoTitle peels `Author on Instagram: 'caption'` (and the
// TikTok analogue) into author + caption. When the pattern doesn't match,
// author is empty and caption is the whole string (still useful as a body
// fallback).
func splitSocialVideoTitle(title string) (author, caption string) {
	for _, marker := range []string{" on Instagram:", " on TikTok:"} {
		if i := strings.Index(title, marker); i >= 0 {
			author = strings.TrimSpace(title[:i])
			caption = strings.TrimSpace(title[i+len(marker):])
			caption = strings.Trim(caption, `'"`)
			return author, caption
		}
	}
	for _, suffix := range []string{" on Instagram", " on TikTok"} {
		if strings.HasSuffix(title, suffix) {
			return strings.TrimSpace(strings.TrimSuffix(title, suffix)), ""
		}
	}
	return "", title
}

// socialVideoTitle builds a short display title: prefer "Author: hook" from a
// peeled caption, else a capped first line of the raw OG title, else the
// platform label. Platform fallbacks use `label` (from the URL host), never a
// substring search on the OG title — handles like "TikTokFan on Instagram"
// must not flip to TikTok.
func socialVideoTitle(author, caption, label string) string {
	hook := firstTitleHook(caption)
	if author != "" && hook != "" {
		return truncateRunes(author+": "+hook, socialVideoTitleMax)
	}
	if author != "" {
		platform := "Instagram"
		if strings.HasPrefix(label, "TikTok") {
			platform = "TikTok"
		}
		return truncateRunes(author+" on "+platform, socialVideoTitleMax)
	}
	if hook = firstTitleHook(caption); hook != "" {
		return truncateRunes(hook, socialVideoTitleMax)
	}
	return label
}

// firstTitleHook takes the first sentence or line of s, suitable as a card
// title fragment. Empty input yields "".
func firstTitleHook(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	runes := []rune(s)
	for i := 0; i < len(runes)-1; i++ {
		c := runes[i]
		if (c == '.' || c == '!' || c == '?') && runes[i+1] == ' ' {
			return string(runes[:i+1])
		}
	}
	return s
}

// truncateRunes caps s at n runes, appending an ellipsis when truncated.
// Prefers a word boundary in the final third so we don't cut mid-word when
// a nearby space exists.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	cut := r[:n]
	for i := len(cut) - 1; i >= (n*2)/3; i-- {
		if cut[i] == ' ' || cut[i] == '\t' {
			return string(cut[:i]) + "…"
		}
	}
	return string(cut) + "…"
}
