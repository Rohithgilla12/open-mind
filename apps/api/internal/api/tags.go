package api

import (
	"strings"
	"unicode/utf8"
)

const (
	maxUserTags = 30
	maxTagRunes = 50
)

// canonicalTags normalises a user-supplied tag list into the stored form: each
// tag is trimmed, lowercased, and truncated to maxTagRunes runes; empties are
// dropped; duplicates are removed preserving first-seen order; and the result
// is capped at maxUserTags. It is deterministic and always returns a non-nil
// slice so callers can persist "no tags" as an empty array rather than null.
func canonicalTags(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > maxTagRunes {
			tag = string([]rune(tag)[:maxTagRunes])
		}
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
		if len(out) == maxUserTags {
			break
		}
	}
	return out
}
