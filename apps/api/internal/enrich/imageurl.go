package enrich

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// imageExtensions are the file extensions that identify an image URL offline,
// without a network round-trip.
var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".avif": true,
}

// isImageURL reports whether rawURL points at an image. It first checks the
// path extension (no network); if that is inconclusive it issues a HEAD request
// via client and inspects the Content-Type. A HEAD failure or non-2xx response
// returns (false, nil) so the caller falls through to normal extraction — the
// sniff must never fail the enrichment job.
func isImageURL(ctx context.Context, client *http.Client, rawURL string) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, nil
	}
	if ext := strings.ToLower(path.Ext(u.Path)); imageExtensions[ext] {
		return true, nil
	}
	if client == nil {
		return false, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return false, nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, nil
	}
	return strings.HasPrefix(resp.Header.Get("Content-Type"), "image/"), nil
}

// imageTitle derives a card title from an image URL: the filename stem (last
// path segment without its extension), URL-decoded.
func imageTitle(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	base := path.Base(u.Path)
	stem := strings.TrimSuffix(base, path.Ext(base))
	if decoded, err := url.PathUnescape(stem); err == nil {
		return decoded
	}
	return stem
}
