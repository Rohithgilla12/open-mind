package search

import (
	"net/url"
	"strings"
)

type Scope string

const (
	ScopeLibrary Scope = "library"
	ScopeAll     Scope = "all"
)

// Query is the structured search/Lens request: soft rank signals + hard filters.
type Query struct {
	Text    string
	Color   string
	Types   []string
	Domains []string // already normalised hosts
	Scope   Scope    // empty means caller must set default before RunQuery
}

// NormalizeDomain extracts a lowercase host from a URL-ish string.
// Leading www. is stripped. Returns ok=false for empty or invalid input.
func NormalizeDomain(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, " ") {
		return "", false
	}
	toParse := raw
	if !strings.Contains(raw, "://") {
		toParse = "https://" + raw
	}
	u, err := url.Parse(toParse)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.Contains(host, " ") {
		return "", false
	}
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return "", false
	}
	return host, true
}

// NormalizeDomains normalises each entry, skips invalids, and dedupes
// order-preserving (first-seen wins).
func NormalizeDomains(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, r := range raw {
		host, ok := NormalizeDomain(r)
		if !ok {
			continue
		}
		if _, dup := seen[host]; dup {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// HasMatchSignal reports whether the query has anything to match on
// (text, colour, types, or domains). Scope alone is not a match signal.
func (q Query) HasMatchSignal() bool {
	return q.Text != "" || q.Color != "" || len(q.Types) > 0 || len(q.Domains) > 0
}

// LibraryOnly reports whether results should be restricted to Mind
// (saved/kept). True unless Scope is explicitly ScopeAll.
func (q Query) LibraryOnly() bool {
	return q.Scope != ScopeAll
}
