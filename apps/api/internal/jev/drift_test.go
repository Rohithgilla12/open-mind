package jev

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestRecencyDecay(t *testing.T) {
	if got := RecencyDecay(0); math.Abs(got-1) > 1e-12 {
		t.Fatalf("age0 = %v, want 1", got)
	}
	half := RecencyDecay(DriftRecencyHalfLifeDays)
	if math.Abs(half-0.5) > 1e-9 {
		t.Fatalf("half-life = %v, want ~0.5", half)
	}
	if RecencyDecay(-5) != 1 {
		t.Fatalf("negative age should clamp to 1")
	}
	if RecencyDecay(1000) >= RecencyDecay(10) {
		t.Fatalf("older should decay more")
	}
}

func TestBlendDriftWeights(t *testing.T) {
	sum := DriftWorthWeight + DriftActionWeight + DriftRecencyWeight
	if math.Abs(sum-1) > 1e-12 {
		t.Fatalf("weights sum = %v, want 1", sum)
	}
	got := BlendDrift(1, 1, 1)
	if math.Abs(got-1) > 1e-12 {
		t.Fatalf("all-one = %v", got)
	}
	got = BlendDrift(1, 0, 0)
	if math.Abs(got-DriftWorthWeight) > 1e-12 {
		t.Fatalf("worth only = %v", got)
	}
	got = BlendDrift(2, -1, 0.5) // clamp
	want := DriftWorthWeight*1 + DriftActionWeight*0 + DriftRecencyWeight*0.5
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("clamped = %v, want %v", got, want)
	}
}

func TestScoreDriftCandidate(t *testing.T) {
	worth, action := 0.9, 0.1
	answers := map[string]Answer{
		QWorthResurfacing: {Type: TypeNoul, Noul: &worth},
		QStillActionable:  {Type: TypeNoul, Noul: &action},
	}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	created := now.Add(-90 * 24 * time.Hour)
	got := ScoreDriftCandidate(answers, created, now)
	want := BlendDrift(0.9, 0.1, 0.5)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
	if ScoreDriftCandidate(nil, created, now) != BlendDrift(0, 0, 0.5) {
		t.Fatalf("nil answers")
	}
}

func TestReorderDriftByScore(t *testing.T) {
	now := time.Now().UTC()
	cands := []DriftScored{
		{ID: "a", Score: 0.2, HasScore: true, ScoredAt: now, OrigIndex: 0},
		{ID: "b", Score: 0.9, HasScore: true, ScoredAt: now, OrigIndex: 1},
		{ID: "c", Score: 0.5, HasScore: true, ScoredAt: now, OrigIndex: 2},
	}
	got := ReorderDriftByScore(cands, now)
	if got[0].ID != "b" || got[1].ID != "c" || got[2].ID != "a" {
		t.Fatalf("order = %s,%s,%s", got[0].ID, got[1].ID, got[2].ID)
	}

	// No fresh scores → preserve heuristic order.
	stale := []DriftScored{
		{ID: "x", OrigIndex: 0},
		{ID: "y", Score: 0.99, HasScore: true, ScoredAt: now.Add(-48 * time.Hour), OrigIndex: 1},
	}
	same := ReorderDriftByScore(stale, now)
	if same[0].ID != "x" || same[1].ID != "y" {
		t.Fatalf("stale should keep heuristic: %+v", same)
	}

	// Fresh scored before unscored; unscored keep relative order.
	mixed := []DriftScored{
		{ID: "u1", OrigIndex: 0},
		{ID: "s", Score: 0.3, HasScore: true, ScoredAt: now, OrigIndex: 1},
		{ID: "u2", OrigIndex: 2},
	}
	m := ReorderDriftByScore(mixed, now)
	if m[0].ID != "s" || m[1].ID != "u1" || m[2].ID != "u2" {
		t.Fatalf("mixed = %s,%s,%s", m[0].ID, m[1].ID, m[2].ID)
	}
}

func TestFormatRecentActivity(t *testing.T) {
	now := time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)
	if got := FormatRecentActivity(nil, now); got != "No saves in the last 14 days." {
		t.Fatalf("empty = %q", got)
	}
	saves := []RecentSave{
		{Title: "Postgres tips", Site: "example.com", CardType: "article", Created: now.Add(-time.Hour)},
		{Title: "Note", Site: "example.com", CardType: "note", Created: now.Add(-2 * time.Hour)},
		{Title: "Other", Site: "news.test", CardType: "article", Created: now.Add(-3 * time.Hour)},
	}
	got := FormatRecentActivity(saves, now)
	for _, want := range []string{
		"Saved 3 items",
		"2026-09-23",
		"article=2",
		"note=1",
		"example.com=2",
		"Postgres tips",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "body") {
		t.Fatalf("must not mention bodies: %q", got)
	}
}

func TestDriftConstants(t *testing.T) {
	if DriftScorePool < 5 {
		t.Fatalf("pool too small: %d", DriftScorePool)
	}
	if DriftTimeout < time.Second {
		t.Fatalf("timeout = %v", DriftTimeout)
	}
}
