package jev

import (
	"math"
	"testing"
	"time"
)

func TestNormalizeVectorSims(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want []float64
	}{
		{name: "empty", in: nil, want: []float64{}},
		{name: "single", in: []float64{0.42}, want: []float64{1}},
		{name: "scale by max", in: []float64{0.2, 0.4, 0.1}, want: []float64{0.5, 1, 0.25}},
		{name: "all zero → mid", in: []float64{0, 0, 0}, want: []float64{0.5, 0.5, 0.5}},
		{name: "negative clamped via max", in: []float64{-1, 2}, want: []float64{0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeVectorSims(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if math.Abs(got[i]-tt.want[i]) > 1e-9 {
					t.Fatalf("[%d] = %v, want %v (full %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestBlendRerankTable(t *testing.T) {
	tests := []struct {
		name        string
		vector, jev float64
		want        float64
	}{
		{name: "vector only", vector: 1, jev: 0, want: RerankVectorWeight},
		{name: "jev only", vector: 0, jev: 1, want: RerankJevWeight},
		{name: "both mid", vector: 0.5, jev: 0.5, want: 0.5},
		{name: "provisional weights", vector: 1, jev: 1, want: 1},
		{name: "weighted mix", vector: 0.8, jev: 0.2, want: 0.4*0.8 + 0.6*0.2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BlendRerank(tt.vector, tt.jev)
			if math.Abs(got-tt.want) > 1e-12 {
				t.Fatalf("BlendRerank(%v,%v) = %v, want %v", tt.vector, tt.jev, got, tt.want)
			}
		})
	}
}

func TestReorderByBlend(t *testing.T) {
	hits := []RankedHit{
		{ID: "a", VectorSim: 1.0, OrigIndex: 0},
		{ID: "b", VectorSim: 0.5, OrigIndex: 1},
		{ID: "c", VectorSim: 0.25, OrigIndex: 2},
	}
	// High Jev on the weakest vector hit should pull it to the front.
	noul := func(p float64) *float64 { return &p }
	answers := map[string]Answer{
		"candidate_0": {Type: TypeNoul, Noul: noul(0.1)},
		"candidate_1": {Type: TypeNoul, Noul: noul(0.2)},
		"candidate_2": {Type: TypeNoul, Noul: noul(1.0)},
	}
	got := ReorderByBlend(hits, answers)
	if got[0].ID != "c" {
		t.Fatalf("order = %v,%v,%v; want c first", got[0].ID, got[1].ID, got[2].ID)
	}
	// Missing answers → vector order preserved (by blend of vector + 0).
	same := ReorderByBlend(hits, nil)
	if same[0].ID != "a" || same[1].ID != "b" || same[2].ID != "c" {
		t.Fatalf("nil answers order = %+v", same)
	}
}

func TestReorderByBlend_StableTies(t *testing.T) {
	hits := []RankedHit{
		{ID: "a", VectorSim: 0.5, OrigIndex: 0},
		{ID: "b", VectorSim: 0.5, OrigIndex: 1},
	}
	noul := 0.5
	answers := map[string]Answer{
		"candidate_0": {Type: TypeNoul, Noul: &noul},
		"candidate_1": {Type: TypeNoul, Noul: &noul},
	}
	got := ReorderByBlend(hits, answers)
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("tie order = %s,%s; want original a,b", got[0].ID, got[1].ID)
	}
}

func TestCandidateNoul(t *testing.T) {
	p := 0.77
	answers := map[string]Answer{"candidate_3": {Type: TypeNoul, Noul: &p}}
	if got := CandidateNoul(answers, 3); got != 0.77 {
		t.Fatalf("got %v", got)
	}
	if got := CandidateNoul(answers, 0); got != 0 {
		t.Fatalf("missing = %v", got)
	}
	if got := CandidateNoul(nil, 0); got != 0 {
		t.Fatalf("nil map = %v", got)
	}
}

func TestRerankConstants(t *testing.T) {
	if RerankTopK != 30 {
		t.Fatalf("RerankTopK = %d", RerankTopK)
	}
	if RerankTimeout != 500*time.Millisecond {
		t.Fatalf("RerankTimeout = %v", RerankTimeout)
	}
}
