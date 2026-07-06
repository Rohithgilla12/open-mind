package api

import (
	"encoding/json"
	"testing"
)

func strptr(s string) *string { return &s }

func typesptr(ts ...string) *[]LensRuleTypes {
	out := make([]LensRuleTypes, 0, len(ts))
	for _, t := range ts {
		out = append(out, LensRuleTypes(t))
	}
	return &out
}

func TestParseRule(t *testing.T) {
	tests := []struct {
		name    string
		rule    LensRule
		wantErr bool
		wantQ   string
		wantCol string
		wantTys []string
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
		})
	}
}

func TestMarshalRuleRoundTrip(t *testing.T) {
	n := normalisedRule{q: "raft", color: "cobalt", types: []string{"article", "video"}}
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
	if got.q != n.q || got.color != n.color || len(got.types) != 2 {
		t.Errorf("round-trip mismatch: %+v", got)
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
