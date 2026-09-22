package jev

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCaptureQuestions(t *testing.T) {
	fixed := []struct {
		id   string
		kind string
	}{
		{id: QContentType, kind: TypeChoice},
		{id: QDepth, kind: TypeScore},
		{id: QEvergreen, kind: TypeNoul},
	}
	tests := []struct {
		name     string
		tags     []string
		wantTags []string
	}{
		{name: "no tags", tags: nil},
		{name: "four tags", tags: []string{"a", "b", "c", "d"}},
		{name: "blanks do not count", tags: []string{"a", " ", "b", "c", "d", ""}},
		{
			name:     "five tags",
			tags:     []string{"go", "postgres", "search", "notes", "type-theory"},
			wantTags: []string{"go", "postgres", "search", "notes", "type-theory"},
		},
		{
			name:     "duplicates collapse",
			tags:     []string{"go", "go", "postgres", "search", "notes", "type-theory"},
			wantTags: []string{"go", "postgres", "search", "notes", "type-theory"},
		},
		{
			name: "four distinct after trim",
			tags: []string{" go ", "go", "postgres", "search", "notes"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qs := CaptureQuestions(tt.tags)
			if len(qs) != len(fixed)+len(tt.wantTags) {
				t.Fatalf("len = %d, want %d", len(qs), len(fixed)+len(tt.wantTags))
			}
			for _, f := range fixed {
				q, ok := qs[f.id]
				if !ok {
					t.Fatalf("missing %s", f.id)
				}
				if q.Type != f.kind {
					t.Fatalf("%s type = %s, want %s", f.id, q.Type, f.kind)
				}
			}
			choice := qs[QContentType].Criteria.(map[string]string)
			for _, id := range []string{ContentArticle, ContentDocs, ContentPaper, ContentTool, ContentVideo, ContentDiscussion, ContentOther} {
				if choice[id] == "" {
					t.Errorf("content_type missing option %s", id)
				}
			}
			if len(choice) != 7 {
				t.Fatalf("content_type options = %d", len(choice))
			}
			levels := qs[QDepth].Criteria.([]string)
			if len(levels) != 3 {
				t.Fatalf("depth levels = %d", len(levels))
			}
			for i, level := range levels {
				if strings.TrimSpace(level) == "" {
					t.Errorf("depth level %d empty", i)
				}
			}
			ever := qs[QEvergreen].Criteria.(noulCriteria)
			if ever.True == "" || ever.False == "" {
				t.Fatalf("evergreen criteria = %+v", ever)
			}
			for _, tag := range tt.wantTags {
				q, ok := qs[TagQuestionPrefix+tag]
				if !ok {
					t.Fatalf("missing tag question %s", tag)
				}
				if q.Type != TypeNoul {
					t.Fatalf("tag %s type = %s", tag, q.Type)
				}
				ins := q.Instructions.(map[string]string)
				if ins["tag"] != tag {
					t.Fatalf("tag instructions = %+v", ins)
				}
				if !strings.Contains(ins["question"], "`tag`") {
					t.Fatalf("question does not point at the tag field: %s", ins["question"])
				}
			}
			if len(tt.wantTags) == 0 {
				for id := range qs {
					if strings.HasPrefix(id, TagQuestionPrefix) {
						t.Fatalf("cold start included %s", id)
					}
				}
			}
		})
	}
}

func TestRerankQuestions(t *testing.T) {
	cands := []RerankCandidate{
		{ID: "a", Title: "One", Snippet: "alpha"},
		{ID: "b", Title: "Two", Snippet: "beta"},
		{ID: "c", Snippet: "gamma"},
	}
	qs := RerankQuestions(cands)
	if len(qs) != len(cands) {
		t.Fatalf("len = %d", len(qs))
	}
	for i := range cands {
		id := CandidateQuestionPrefix + string(rune('0'+i))
		q, ok := qs[id]
		if !ok {
			t.Fatalf("missing %s", id)
		}
		if q.Type != TypeNoul {
			t.Fatalf("%s type = %s", id, q.Type)
		}
		text := q.Instructions.(string)
		if !strings.Contains(text, "substantively relevant") || !strings.Contains(text, "beyond sharing keywords") {
			t.Fatalf("instructions = %s", text)
		}
		wantPath := "candidates[" + string(rune('0'+i)) + "]"
		if !strings.Contains(text, wantPath) {
			t.Fatalf("instructions %q missing %s", text, wantPath)
		}
	}
	if len(RerankQuestions(nil)) != 0 {
		t.Fatal("empty candidate list should ask nothing")
	}
}

func TestDriftQuestions(t *testing.T) {
	qs := DriftQuestions()
	if len(qs) != 2 {
		t.Fatalf("len = %d", len(qs))
	}
	for _, id := range []string{QWorthResurfacing, QStillActionable} {
		q, ok := qs[id]
		if !ok {
			t.Fatalf("missing %s", id)
		}
		if q.Type != TypeNoul {
			t.Fatalf("%s type = %s", id, q.Type)
		}
		crit := q.Criteria.(noulCriteria)
		if crit.True == "" || crit.False == "" {
			t.Fatalf("%s criteria = %+v", id, crit)
		}
	}
	worth := qs[QWorthResurfacing].Instructions.(string)
	if !strings.Contains(worth, "recent_activity") || !strings.Contains(worth, "14 days") {
		t.Fatalf("worth_resurfacing = %s", worth)
	}
}

func TestThresholds(t *testing.T) {
	if TagAutoApplyAbove != 0.85 || TagSuggestMin != 0.50 || ContentTypeMinConfidence != 0.60 {
		t.Fatalf("thresholds drifted: apply %v suggest %v type %v", TagAutoApplyAbove, TagSuggestMin, ContentTypeMinConfidence)
	}
	if RerankVectorWeight != 0.4 || RerankJevWeight != 0.6 {
		t.Fatalf("weights %v / %v", RerankVectorWeight, RerankJevWeight)
	}
	if RerankVectorWeight+RerankJevWeight != 1 {
		t.Fatal("rerank weights must sum to 1")
	}
	if ColdStartTags != 5 {
		t.Fatalf("cold start = %d", ColdStartTags)
	}

	tests := []struct {
		name string
		p    float64
		want TagDecision
	}{
		{name: "zero", p: 0, want: ActionSkipped},
		{name: "just below suggest", p: 0.50 - 1e-9, want: ActionSkipped},
		{name: "suggest floor", p: 0.50, want: ActionSuggested},
		{name: "mid", p: 0.7, want: ActionSuggested},
		{name: "apply boundary stays a suggestion", p: 0.85, want: ActionSuggested},
		{name: "just above apply", p: 0.85 + 1e-9, want: ActionApplied},
		{name: "certain", p: 1, want: ActionApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecideTag(tt.p); got != tt.want {
				t.Fatalf("DecideTag(%v) = %s, want %s", tt.p, got, tt.want)
			}
		})
	}

	conf := []struct {
		c    float64
		want bool
	}{
		{c: 0, want: false},
		{c: 0.59, want: false},
		{c: 0.60, want: true},
		{c: 1, want: true},
	}
	for _, tt := range conf {
		if got := AcceptContentType(tt.c); got != tt.want {
			t.Errorf("AcceptContentType(%v) = %v, want %v", tt.c, got, tt.want)
		}
	}

	if got := BlendRerank(1, 0); got != RerankVectorWeight {
		t.Fatalf("BlendRerank(1, 0) = %v", got)
	}
	if got := BlendRerank(0, 1); got != RerankJevWeight {
		t.Fatalf("BlendRerank(0, 1) = %v", got)
	}
}

func TestBuildState_Clips(t *testing.T) {
	long := strings.Repeat("a", ExcerptMaxChars-1) + "éZ"
	got := BuildCaptureState("https://example.com", "Title", "example.com", long)
	if utf8.RuneCountInString(got.Excerpt) != ExcerptMaxChars {
		t.Fatalf("excerpt runes = %d", utf8.RuneCountInString(got.Excerpt))
	}
	if !strings.HasSuffix(got.Excerpt, "é") {
		t.Fatalf("excerpt clipped mid-rune: %q", got.Excerpt[len(got.Excerpt)-4:])
	}
	if got.URL != "https://example.com" || got.Title != "Title" || got.Site != "example.com" {
		t.Fatalf("state = %+v", got)
	}

	short := BuildCaptureState("u", "t", "s", "hello")
	if short.Excerpt != "hello" {
		t.Fatalf("short excerpt = %q", short.Excerpt)
	}

	snippet := strings.Repeat("b", SnippetMaxChars+10)
	rs := BuildRerankState("query", []RerankCandidate{{ID: "1", Snippet: snippet}})
	if utf8.RuneCountInString(rs.Candidates[0].Snippet) != SnippetMaxChars {
		t.Fatalf("snippet runes = %d", utf8.RuneCountInString(rs.Candidates[0].Snippet))
	}
	if rs.Query != "query" || rs.Candidates[0].ID != "1" {
		t.Fatalf("rerank state = %+v", rs)
	}

	drift := BuildDriftState(DriftItem{Title: "t", Excerpt: long}, "saved three notes about databases")
	if utf8.RuneCountInString(drift.Item.Excerpt) != ExcerptMaxChars {
		t.Fatalf("drift excerpt runes = %d", utf8.RuneCountInString(drift.Item.Excerpt))
	}
	if drift.RecentActivity != "saved three notes about databases" {
		t.Fatalf("activity = %q", drift.RecentActivity)
	}
}

func TestQuestionsVersion(t *testing.T) {
	if QuestionsVersion == "" {
		t.Fatal("empty questions version")
	}
}
