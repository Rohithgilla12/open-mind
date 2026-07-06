package api

import (
	"fmt"
	"strings"
	"testing"
)

func TestCanonicalTags(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil in, empty out", nil, []string{}},
		{"empty in, empty out", []string{}, []string{}},
		{"lowercase", []string{"Go", "RUST"}, []string{"go", "rust"}},
		{"trim whitespace", []string{"  go ", "\trust\n"}, []string{"go", "rust"}},
		{"drop empties", []string{"go", "", "   ", "rust"}, []string{"go", "rust"}},
		{"dedupe preserving first-seen order", []string{"go", "rust", "go", "GO", " go "}, []string{"go", "rust"}},
		{"dedupe after canonicalisation", []string{"A", "a", " b "}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalTags(tt.in)
			if !equalStrings(got, tt.want) {
				t.Errorf("canonicalTags(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}

	t.Run("caps at 30 tags", func(t *testing.T) {
		in := make([]string, 40)
		for i := range in {
			in[i] = fmt.Sprintf("tag%d", i)
		}
		got := canonicalTags(in)
		if len(got) != maxUserTags {
			t.Fatalf("len = %d, want %d", len(got), maxUserTags)
		}
		if got[0] != "tag0" || got[maxUserTags-1] != fmt.Sprintf("tag%d", maxUserTags-1) {
			t.Errorf("truncation dropped the wrong end: %q", got)
		}
	})

	t.Run("cap counts unique tags only", func(t *testing.T) {
		// 35 duplicates of the same tag plus one more → 2 unique, under the cap.
		in := make([]string, 0, 36)
		for range 35 {
			in = append(in, "dup")
		}
		in = append(in, "other")
		got := canonicalTags(in)
		if !equalStrings(got, []string{"dup", "other"}) {
			t.Errorf("got %q, want [dup other]", got)
		}
	})

	t.Run("truncates tags longer than 50 runes", func(t *testing.T) {
		long := strings.Repeat("é", 60) // multi-byte runes
		got := canonicalTags([]string{long})
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if runes := []rune(got[0]); len(runes) != maxTagRunes {
			t.Errorf("tag rune length = %d, want %d", len(runes), maxTagRunes)
		}
	})

	t.Run("truncation then dedupe", func(t *testing.T) {
		a := strings.Repeat("a", 55)
		b := strings.Repeat("a", 60) // same first 50 runes as a
		got := canonicalTags([]string{a, b})
		if len(got) != 1 {
			t.Errorf("got %q, want a single deduped tag", got)
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
