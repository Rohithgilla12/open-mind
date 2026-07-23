package ai

import "testing"

func TestMergePlaces(t *testing.T) {
	loc := []Place{{Name: "Fabrica"}}                                      // default 0.98
	caption := []Place{{Name: "Fabrica"}, {Name: "Copenhagen Coffee Lab"}} // default 0.85
	vision := []Place{{Name: "Fabrica", Confidence: 0.9}}                  // explicit 0.9

	got := MergePlaces(
		PlaceGroup{Places: loc, Source: "location", DefaultConf: 0.98},
		PlaceGroup{Places: caption, Source: "caption", DefaultConf: 0.85},
		PlaceGroup{Places: vision, Source: "vision", DefaultConf: 0.70},
	)

	if len(got) != 2 {
		t.Fatalf("want 2 merged places, got %d: %+v", len(got), got)
	}
	// Fabrica: location 0.98 beats caption 0.85 and vision 0.9 → source "location".
	if got[0].Name != "Fabrica" || got[0].Source != "location" {
		t.Errorf("Fabrica winner = %+v, want location", got[0])
	}
	// First-seen order preserved (location added Fabrica first).
	if got[1].Name != "Copenhagen Coffee Lab" || got[1].Source != "caption" {
		t.Errorf("second = %+v, want Copenhagen Coffee Lab/caption", got[1])
	}
}

func TestMergePlaces_TieKeepsEarlierGroup(t *testing.T) {
	got := MergePlaces(
		PlaceGroup{Places: []Place{{Name: "Tie Cafe", Confidence: 0.8}}, Source: "caption", DefaultConf: 0.85},
		PlaceGroup{Places: []Place{{Name: "Tie Cafe", Confidence: 0.8}}, Source: "vision", DefaultConf: 0.70},
	)

	if len(got) != 1 {
		t.Fatalf("want 1 merged place, got %d: %+v", len(got), got)
	}
	// Equal confidence (0.8 == 0.8): earlier group (caption) wins the tie.
	if got[0].Name != "Tie Cafe" || got[0].Source != "caption" {
		t.Errorf("tie winner = %+v, want caption", got[0])
	}
}

func TestSanitisePlaces_ClampsConfidence(t *testing.T) {
	got := sanitisePlaces([]Place{
		{Name: "A", Confidence: 1.5},
		{Name: "B", Confidence: -0.2},
		{Name: ""},
		{Name: "A"}, // duplicate dropped
	})
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Confidence != 1 || got[1].Confidence != 0 {
		t.Errorf("clamped = %+v", got)
	}
}
