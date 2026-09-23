package jev

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	// DriftTimeout is the per-candidate Evaluate budget on the daily scoring
	// job. The request path never waits on this call.
	DriftTimeout = 2 * time.Second

	// DriftScorePool is how many Drift candidates the daily job scores per
	// user. GET /drift still returns at most a small batch; scoring a larger
	// pool lets the blend pick better suggestions than the oldest-five heuristic.
	DriftScorePool = 40

	// DriftScoreFresh is how long a stored drift_score stays eligible to
	// reorder GET /drift. Older scores fall back to the recency heuristic so
	// a stale job run cannot permanently pin a bad order.
	DriftScoreFresh = 36 * time.Hour
)

// DriftScored is one Drift candidate with an optional precomputed blend score.
// OrigIndex is the position under the recency heuristic (stable tie-break /
// unscored fallthrough).
type DriftScored struct {
	ID        string
	Score     float64
	ScoredAt  time.Time
	HasScore  bool
	OrigIndex int
}

// RecencyDecay maps item age in days onto (0, 1] with half-life
 // DriftRecencyHalfLifeDays. At age == half-life the value is 0.5
 // (decay = 2^(-age/halfLife)). Negative ages clamp to 0 days (decay = 1).
func RecencyDecay(ageDays float64) float64 {
	if ageDays < 0 {
		ageDays = 0
	}
	if DriftRecencyHalfLifeDays <= 0 {
		return 1
	}
	return math.Pow(0.5, ageDays/DriftRecencyHalfLifeDays)
}

// RecencyDecaySince is RecencyDecay for the duration between created and now.
func RecencyDecaySince(created, now time.Time) float64 {
	if created.IsZero() || now.IsZero() {
		return 0.5
	}
	days := now.Sub(created).Hours() / 24
	return RecencyDecay(days)
}

// DriftNoul returns the Noul probability for a Drift question id, or 0 when
// missing / not a Noul.
func DriftNoul(answers map[string]Answer, id string) float64 {
	if answers == nil {
		return 0
	}
	a, ok := answers[id]
	if !ok || a.Noul == nil {
		return 0
	}
	return clamp01(*a.Noul)
}

// ScoreDriftCandidate builds the blend from an Evaluate answers map and the
// item's created_at (for recency). now is injectable for tests.
func ScoreDriftCandidate(answers map[string]Answer, created, now time.Time) float64 {
	return BlendDrift(
		DriftNoul(answers, QWorthResurfacing),
		DriftNoul(answers, QStillActionable),
		RecencyDecaySince(created, now),
	)
}

// ReorderDriftByScore sorts candidates by fresh blend score descending.
// When no candidate has a fresh score, the input order is returned unchanged
// (recency heuristic). Unscored / stale items keep their relative order after
// all fresh-scored ones. Ties break by OrigIndex ascending.
func ReorderDriftByScore(cands []DriftScored, now time.Time) []DriftScored {
	if len(cands) == 0 {
		return cands
	}
	fresh := 0
	for _, c := range cands {
		if driftScoreFresh(c, now) {
			fresh++
		}
	}
	if fresh == 0 {
		out := make([]DriftScored, len(cands))
		copy(out, cands)
		return out
	}
	out := make([]DriftScored, len(cands))
	copy(out, cands)
	sort.SliceStable(out, func(i, j int) bool {
		fi := driftScoreFresh(out[i], now)
		fj := driftScoreFresh(out[j], now)
		if fi != fj {
			return fi // fresh before stale/missing
		}
		if fi && out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].OrigIndex < out[j].OrigIndex
	})
	return out
}

func driftScoreFresh(c DriftScored, now time.Time) bool {
	if !c.HasScore || c.ScoredAt.IsZero() {
		return false
	}
	return !c.ScoredAt.Before(now.Add(-DriftScoreFresh))
}

// RecentSave is one privacy-filtered save used to build recent_activity.
type RecentSave struct {
	Title    string
	Site     string
	CardType string
	Created  time.Time
}

// FormatRecentActivity builds a deterministic summary of the last 14 days of
// saves for DriftQuestions. Openmind does not yet record item opens, so the
// summary is saves only: counts, card types, sites, and clipped titles —
// never bodies or other users' items.
func FormatRecentActivity(saves []RecentSave, now time.Time) string {
	if len(saves) == 0 {
		return "No saves in the last 14 days."
	}
	if len(saves) > ActivitySummaryMaxSaves {
		saves = saves[:ActivitySummaryMaxSaves]
	}

	type hostCount struct {
		host  string
		count int
	}
	hosts := map[string]int{}
	types := map[string]int{}
	for _, s := range saves {
		h := strings.TrimSpace(s.Site)
		if h == "" {
			h = "(no site)"
		}
		hosts[h]++
		ct := strings.TrimSpace(s.CardType)
		if ct == "" {
			ct = "other"
		}
		types[ct]++
	}

	hostList := make([]hostCount, 0, len(hosts))
	for h, n := range hosts {
		hostList = append(hostList, hostCount{h, n})
	}
	sort.Slice(hostList, func(i, j int) bool {
		if hostList[i].count != hostList[j].count {
			return hostList[i].count > hostList[j].count
		}
		return hostList[i].host < hostList[j].host
	})
	typeList := make([]string, 0, len(types))
	for ct := range types {
		typeList = append(typeList, ct)
	}
	sort.Strings(typeList)

	var b strings.Builder
	fmt.Fprintf(&b, "Saved %d items in the last 14 days (as of %s UTC).",
		len(saves), now.UTC().Format("2006-01-02"))
	b.WriteString(" Card types:")
	for i, ct := range typeList {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, " %s=%d", ct, types[ct])
	}
	b.WriteByte('.')
	if len(hostList) > 0 {
		b.WriteString(" Top sites:")
		limit := 8
		if len(hostList) < limit {
			limit = len(hostList)
		}
		for i := 0; i < limit; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, " %s=%d", hostList[i].host, hostList[i].count)
		}
		b.WriteByte('.')
	}
	b.WriteString(" Recent titles:")
	titleLimit := 12
	if len(saves) < titleLimit {
		titleLimit = len(saves)
	}
	for i := 0; i < titleLimit; i++ {
		title := strings.TrimSpace(saves[i].Title)
		if title == "" {
			title = "(untitled)"
		}
		title = clipRunes(title, 80)
		if i > 0 {
			b.WriteByte(';')
		}
		fmt.Fprintf(&b, " %s", title)
	}
	b.WriteByte('.')
	return b.String()
}
