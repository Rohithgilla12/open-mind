package api

import (
	"encoding/json"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/search"
)

func strptr(s string) *string { return &s }

func typesptr(ts ...string) *[]LensRuleTypes {
	out := make([]LensRuleTypes, 0, len(ts))
	for _, t := range ts {
		out = append(out, LensRuleTypes(t))
	}
	return &out
}

func domainsptr(ds ...string) *[]string {
	out := append([]string(nil), ds...)
	return &out
}

func scopeptr(s LensRuleScope) *LensRuleScope { return &s }

func TestParseRule(t *testing.T) {
	tests := []struct {
		name     string
		rule     LensRule
		wantErr  bool
		wantQ    string
		wantCol  string
		wantTys  []string
		wantDoms []string
		wantSc   search.Scope
	}{
		{name: "empty rule rejected", rule: LensRule{}, wantErr: true},
		{name: "whitespace-only rejected", rule: LensRule{Q: strptr("   ")}, wantErr: true},
		{name: "text only", rule: LensRule{Q: strptr("  running shoes ")}, wantQ: "running shoes"},
		{name: "named colour", rule: LensRule{Color: strptr("cobalt")}, wantCol: "cobalt"},
		{name: "hex colour", rule: LensRule{Color: strptr("#1B3FD1")}, wantCol: "#1B3FD1"},
		{name: "bad colour rejected", rule: LensRule{Color: strptr("not-a-colour")}, wantErr: true},
		{name: "types deduped", rule: LensRule{Types: typesptr("recipe", "recipe", "book")}, wantTys: []string{"recipe", "book"}},
		{name: "unknown type rejected", rule: LensRule{Types: typesptr("gizmo")}, wantErr: true},
		{
			name:  "combined",
			rule:  LensRule{Q: strptr("warm"), Color: strptr("gold"), Types: typesptr("image")},
			wantQ: "warm", wantCol: "gold", wantTys: []string{"image"},
		},
		{name: "domains only", rule: LensRule{Domains: domainsptr("x.com")}, wantDoms: []string{"x.com"}},
		{
			name:     "domains+types",
			rule:     LensRule{Domains: domainsptr("https://www.x.com/a"), Types: typesptr("tweet")},
			wantDoms: []string{"x.com"},
			wantTys:  []string{"tweet"},
		},
		{name: "bad domain rejected", rule: LensRule{Domains: domainsptr("not a host")}, wantErr: true},
		{name: "whitespace-only domains rejected", rule: LensRule{Domains: domainsptr("  ", "")}, wantErr: true},
		{name: "scope library", rule: LensRule{Types: typesptr("article"), Scope: scopeptr(LensRuleScopeLibrary)}, wantTys: []string{"article"}, wantSc: search.ScopeLibrary},
		{name: "scope all", rule: LensRule{Types: typesptr("article"), Scope: scopeptr(LensRuleScopeAll)}, wantTys: []string{"article"}, wantSc: search.ScopeAll},
		{name: "invalid scope rejected", rule: LensRule{Types: typesptr("article"), Scope: scopeptr("feed")}, wantErr: true},
		{name: "scope alone rejected", rule: LensRule{Scope: scopeptr(LensRuleScopeLibrary)}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRule(tc.rule)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none (parsed %+v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.q != tc.wantQ {
				t.Errorf("q = %q, want %q", got.q, tc.wantQ)
			}
			if got.color != tc.wantCol {
				t.Errorf("color = %q, want %q", got.color, tc.wantCol)
			}
			if len(got.types) != len(tc.wantTys) {
				t.Fatalf("types = %v, want %v", got.types, tc.wantTys)
			}
			for i, ty := range tc.wantTys {
				if got.types[i] != ty {
					t.Errorf("types[%d] = %q, want %q", i, got.types[i], ty)
				}
			}
			if len(got.domains) != len(tc.wantDoms) {
				t.Fatalf("domains = %v, want %v", got.domains, tc.wantDoms)
			}
			for i, d := range tc.wantDoms {
				if got.domains[i] != d {
					t.Errorf("domains[%d] = %q, want %q", i, got.domains[i], d)
				}
			}
			if got.scope != tc.wantSc {
				t.Errorf("scope = %q, want %q", got.scope, tc.wantSc)
			}
		})
	}
}

func TestMarshalRuleRoundTrip(t *testing.T) {
	n := normalisedRule{
		q: "raft", color: "cobalt", types: []string{"article", "video"},
		domains: []string{"x.com"}, scope: search.ScopeAll,
	}
	raw, err := marshalRule(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var lr LensRule
	if err := json.Unmarshal(raw, &lr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := parseRule(lr)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if got.q != n.q || got.color != n.color || len(got.types) != 2 ||
		len(got.domains) != 1 || got.domains[0] != "x.com" || got.scope != search.ScopeAll {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	// Empty scope is omitted from JSON (library is the run-time default).
	raw, err = marshalRule(normalisedRule{q: "raft"})
	if err != nil {
		t.Fatalf("marshal text-only: %v", err)
	}
	var compact map[string]any
	if err := json.Unmarshal(raw, &compact); err != nil {
		t.Fatalf("unmarshal compact: %v", err)
	}
	if _, ok := compact["scope"]; ok {
		t.Errorf("empty scope should be omitted, got %v", compact)
	}
	if _, ok := compact["domains"]; ok {
		t.Errorf("empty domains should be omitted, got %v", compact)
	}

	// An empty rule must not marshal fields that would revive it as valid.
	raw, err = marshalRule(normalisedRule{})
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if string(raw) != "{}" {
		t.Errorf("empty rule marshalled to %q, want {}", raw)
	}
}
