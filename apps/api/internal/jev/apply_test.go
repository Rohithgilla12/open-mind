package jev_test

import (
	"math"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/jev"
)

func TestPlanCapture_Table(t *testing.T) {
	noul := func(p float64) *float64 { return &p }
	conf := func(c float64) *float64 { return &c }

	cases := []struct {
		name      string
		answers   map[string]jev.Answer
		wantAct   jev.TagDecision
		wantApply []string
		wantSug   []string
		wantCard  string
	}{
		{
			name:    "empty → skipped",
			answers: nil,
			wantAct: jev.ActionSkipped,
		},
		{
			name: "tag above auto-apply",
			answers: map[string]jev.Answer{
				"tag:go": {Type: "noul", Noul: noul(0.86)},
			},
			wantAct:   jev.ActionApplied,
			wantApply: []string{"go"},
		},
		{
			name: "tag exactly 0.85 → suggest",
			answers: map[string]jev.Answer{
				"tag:go": {Type: "noul", Noul: noul(0.85)},
			},
			wantAct: jev.ActionSuggested,
			wantSug: []string{"go"},
		},
		{
			name: "tag at suggest min",
			answers: map[string]jev.Answer{
				"tag:rust": {Type: "noul", Noul: noul(0.50)},
			},
			wantAct: jev.ActionSuggested,
			wantSug: []string{"rust"},
		},
		{
			name: "tag below suggest min",
			answers: map[string]jev.Answer{
				"tag:rust": {Type: "noul", Noul: noul(0.49)},
			},
			wantAct: jev.ActionSkipped,
		},
		{
			name: "mixed applied + suggested",
			answers: map[string]jev.Answer{
				"tag:a": {Type: "noul", Noul: noul(0.9)},
				"tag:b": {Type: "noul", Noul: noul(0.7)},
				"tag:c": {Type: "noul", Noul: noul(0.1)},
			},
			wantAct:   jev.ActionApplied,
			wantApply: []string{"a"},
			wantSug:   []string{"b"},
		},
		{
			name: "content_type high conf maps to product",
			answers: map[string]jev.Answer{
				jev.QContentType: {Type: "choice", Choice: jev.ContentTool, Confidence: conf(0.60)},
			},
			wantAct:  jev.ActionApplied,
			wantCard: "product",
		},
		{
			name: "content_type article mapping is a no-op for action",
			answers: map[string]jev.Answer{
				jev.QContentType: {Type: "choice", Choice: jev.ContentArticle, Confidence: conf(0.99)},
			},
			wantAct: jev.ActionSkipped,
		},
		{
			name: "content_type below conf → skipped",
			answers: map[string]jev.Answer{
				jev.QContentType: {Type: "choice", Choice: jev.ContentTool, Confidence: conf(0.59)},
			},
			wantAct: jev.ActionSkipped,
		},
		{
			name: "content_type discussion has no mapping",
			answers: map[string]jev.Answer{
				jev.QContentType: {Type: "choice", Choice: jev.ContentDiscussion, Confidence: conf(0.99)},
			},
			wantAct: jev.ActionSkipped,
		},
		{
			name: "NaN noul skipped",
			answers: map[string]jev.Answer{
				"tag:x": {Type: "noul", Noul: noul(math.NaN())},
			},
			wantAct: jev.ActionSkipped,
		},
		{
			name: "suggest-only → suggested action",
			answers: map[string]jev.Answer{
				"tag:z": {Type: "noul", Noul: noul(0.6)},
			},
			wantAct: jev.ActionSuggested,
			wantSug: []string{"z"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jev.PlanCapture(tc.answers)
			if got.Action != tc.wantAct {
				t.Fatalf("action = %s, want %s", got.Action, tc.wantAct)
			}
			if !eqStrings(got.AppliedTags, tc.wantApply) {
				t.Fatalf("applied = %v, want %v", got.AppliedTags, tc.wantApply)
			}
			if !eqStrings(got.SuggestedTags, tc.wantSug) {
				t.Fatalf("suggested = %v, want %v", got.SuggestedTags, tc.wantSug)
			}
			if got.CardType != tc.wantCard {
				t.Fatalf("cardType = %q, want %q", got.CardType, tc.wantCard)
			}
		})
	}
}

func TestMapContentTypeToCardType(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantOK  bool
	}{
		{jev.ContentArticle, "article", true},
		{jev.ContentDocs, "article", true},
		{jev.ContentPaper, "article", true},
		{jev.ContentTool, "product", true},
		{jev.ContentVideo, "video", true},
		{jev.ContentDiscussion, "", false},
		{jev.ContentOther, "", false},
		{"", "", false},
		{"bogus", "", false},
	}
	for _, tc := range cases {
		got, ok := jev.MapContentTypeToCardType(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("MapContentTypeToCardType(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestMergeTagsAndPending(t *testing.T) {
	merged := jev.MergeTags([]string{"a", "b"}, []string{"b", "c"})
	if !eqStrings(merged, []string{"a", "b", "c"}) {
		t.Fatalf("MergeTags = %v", merged)
	}
	pending := jev.PendingSuggestions([]string{"x", "y", "z"}, []string{"y"}, []string{"z"})
	if !eqStrings(pending, []string{"x"}) {
		t.Fatalf("PendingSuggestions = %v", pending)
	}
	rem := jev.RemovableApplied([]string{"a", "b"}, []string{"b", "c"})
	if !eqStrings(rem, []string{"b"}) {
		t.Fatalf("RemovableApplied = %v", rem)
	}
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil && b == nil {
		return true
	}
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
