package jev

import (
	"sort"
	"time"
)

const (
	// RerankTimeout is the ideal added-latency budget for one Evaluate on the
	// Lens/search path. Callers put this on the context; DeadlineExceeded is
	// a Skip and leaves the hybrid order unchanged. Typeahead must never wait
	// on this call — only explicit Lens loads / submitted searches.
	RerankTimeout = 500 * time.Millisecond

	// RerankTopK is how many hybrid hits are sent to Jev (snippets only).
	// pgvector/FTS remain the recall layer; Jev only reorders this window.
	RerankTopK = 30
)

// RankedHit is one hybrid-search candidate before or after Jev reordering.
// VectorSim should already be in [0, 1] (normalised within the candidate set).
type RankedHit struct {
	ID        string
	Title     string
	Snippet   string
	VectorSim float64
	// OrigIndex is the position in the pre-rerank slice (stable tiebreak).
	OrigIndex int
}

// NormalizeVectorSims maps fused hybrid scores onto [0, 1] within the set by
// dividing by the max. When every score is ≤0, each candidate gets 0.5 so the
// blend still listens to Jev. Spec blend: 0.4·vectorSim + 0.6·jevRelevance.
func NormalizeVectorSims(scores []float64) []float64 {
	out := make([]float64, len(scores))
	if len(scores) == 0 {
		return out
	}
	max := scores[0]
	for _, s := range scores[1:] {
		if s > max {
			max = s
		}
	}
	if max <= 0 {
		for i := range out {
			out[i] = 0.5
		}
		return out
	}
	for i, s := range scores {
		v := s / max
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		out[i] = v
	}
	return out
}

// CandidateNoul returns the Noul probability for candidate_i, or 0 when the
// answer is missing or not a Noul.
func CandidateNoul(answers map[string]Answer, index int) float64 {
	if answers == nil {
		return 0
	}
	a, ok := answers[CandidateQuestionPrefix+itoa(index)]
	if !ok || a.Noul == nil {
		return 0
	}
	p := *a.Noul
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// ReorderByBlend sorts hits by BlendRerank(vectorSim, jev Noul) descending.
// Ties keep original order (OrigIndex ascending). answers keys are candidate_N
// matching BuildRerankState / RerankQuestions order (index in hits).
func ReorderByBlend(hits []RankedHit, answers map[string]Answer) []RankedHit {
	if len(hits) == 0 {
		return hits
	}
	out := make([]RankedHit, len(hits))
	copy(out, hits)
	type scored struct {
		hit     RankedHit
		blended float64
	}
	ranked := make([]scored, len(out))
	for i, h := range out {
		ranked[i] = scored{
			hit:     h,
			blended: BlendRerank(h.VectorSim, CandidateNoul(answers, i)),
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].blended != ranked[j].blended {
			return ranked[i].blended > ranked[j].blended
		}
		return ranked[i].hit.OrigIndex < ranked[j].hit.OrigIndex
	})
	for i, r := range ranked {
		out[i] = r.hit
	}
	return out
}

// itoa avoids strconv for the small fixed candidate ids (0..RerankTopK-1).
func itoa(n int) string {
	if n < 0 {
		return "0"
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
