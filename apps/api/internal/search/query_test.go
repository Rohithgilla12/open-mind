package search

import (
	"reflect"
	"testing"
)

func TestNormalizeDomain(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"x.com", "x.com", true},
		{"https://www.X.com/foo", "x.com", true},
		{"HTTP://Twitter.com", "twitter.com", true},
		{"  www.example.co.uk ", "example.co.uk", true},
		{"", "", false},
		{"not a host", "", false}, // spaces / garbage
		{"http://", "", false},
	}
	for _, tc := range cases {
		got, ok := NormalizeDomain(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("NormalizeDomain(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNormalizeDomainsDedupe(t *testing.T) {
	got := NormalizeDomains([]string{"x.com", "https://www.x.com/a", "twitter.com", ""})
	want := []string{"x.com", "twitter.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeDomains = %v, want %v", got, want)
	}
}

func TestQueryHasMatchSignal(t *testing.T) {
	cases := []struct {
		name string
		q    Query
		want bool
	}{
		{"empty", Query{}, false},
		{"text", Query{Text: "foo"}, true},
		{"color", Query{Color: "#fff"}, true},
		{"types", Query{Types: []string{"article"}}, true},
		{"domains", Query{Domains: []string{"x.com"}}, true},
		{"scope alone", Query{Scope: ScopeLibrary}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.q.HasMatchSignal(); got != tc.want {
				t.Errorf("HasMatchSignal() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQueryLibraryOnly(t *testing.T) {
	if !((Query{Scope: ScopeLibrary}).LibraryOnly()) {
		t.Error("ScopeLibrary should be library-only")
	}
	if !((Query{}).LibraryOnly()) {
		t.Error("empty Scope should be library-only")
	}
	if (Query{Scope: ScopeAll}).LibraryOnly() {
		t.Error("ScopeAll should not be library-only")
	}
}
