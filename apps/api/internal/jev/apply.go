package jev

import (
	"math"
	"sort"
	"strings"
)

// UserVerdict values backfilled on jev_decisions when the user reacts.
const (
	VerdictKept    = "kept"
	VerdictChanged = "changed"
	VerdictRemoved = "removed"
)

// CapturePlan is the code-side decision for one Evaluate result: which tags to
// auto-apply, which to surface as suggestions, and an optional card-type
// override. Action is the row-level jev_decisions.action for this capture.
type CapturePlan struct {
	AppliedTags   []string
	SuggestedTags []string
	CardType      string // empty = leave the item's card_type alone
	Action        TagDecision
}

// PlanCapture applies the provisional thresholds in questions.go to a full
// answers map. Tag keys use TagQuestionPrefix. Unknown or malformed answers
// are skipped. Results are sorted for stable tests and idempotent merges.
func PlanCapture(answers map[string]Answer) CapturePlan {
	plan := CapturePlan{Action: ActionSkipped}
	if len(answers) == 0 {
		return plan
	}

	for id, ans := range answers {
		if !strings.HasPrefix(id, TagQuestionPrefix) {
			continue
		}
		tag := strings.TrimSpace(strings.TrimPrefix(id, TagQuestionPrefix))
		if tag == "" || ans.Noul == nil || math.IsNaN(*ans.Noul) {
			continue
		}
		switch DecideTag(*ans.Noul) {
		case ActionApplied:
			plan.AppliedTags = append(plan.AppliedTags, tag)
		case ActionSuggested:
			plan.SuggestedTags = append(plan.SuggestedTags, tag)
		}
	}
	sort.Strings(plan.AppliedTags)
	sort.Strings(plan.SuggestedTags)

	if ct, ok := answers[QContentType]; ok {
		conf := 0.0
		if ct.Confidence != nil {
			conf = *ct.Confidence
		}
		if AcceptContentType(conf) {
			if mapped, ok := MapContentTypeToCardType(ct.Choice); ok && mapped != "article" {
				// article is Classify's default — only record overrides that
				// change the card (product/video). docs/paper stay untyped here.
				plan.CardType = mapped
			}
		}
	}

	switch {
	case len(plan.AppliedTags) > 0 || plan.CardType != "":
		plan.Action = ActionApplied
	case len(plan.SuggestedTags) > 0:
		plan.Action = ActionSuggested
	default:
		plan.Action = ActionSkipped
	}
	return plan
}

// MapContentTypeToCardType maps a Jev content_type Choice onto an existing
// Openmind card type. Choices with no clean mapping return ok=false — callers
// must not invent new card types.
func MapContentTypeToCardType(choice string) (cardType string, ok bool) {
	switch strings.TrimSpace(choice) {
	case ContentArticle, ContentDocs, ContentPaper:
		return "article", true
	case ContentTool:
		return "product", true
	case ContentVideo:
		return "video", true
	// discussion / other (and anything unknown): no clean card-type mapping.
	default:
		return "", false
	}
}

// MergeTags appends add into base without duplicates (case-sensitive; callers
// should pass already-canonical tags). Order: existing first, then new.
func MergeTags(base, add []string) []string {
	seen := make(map[string]struct{}, len(base)+len(add))
	out := make([]string, 0, len(base)+len(add))
	for _, t := range base {
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	for _, t := range add {
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// PendingSuggestions returns suggested tags that are not already on the item
// and not dismissed.
func PendingSuggestions(suggested, dismissed, userTags []string) []string {
	skip := make(map[string]struct{}, len(dismissed)+len(userTags))
	for _, t := range dismissed {
		skip[t] = struct{}{}
	}
	for _, t := range userTags {
		skip[t] = struct{}{}
	}
	out := make([]string, 0, len(suggested))
	for _, t := range suggested {
		if _, ok := skip[t]; ok {
			continue
		}
		out = append(out, t)
	}
	return out
}

// RemovableApplied returns auto-applied tags that are still present on the item
// (so the UI can offer one-tap undo).
func RemovableApplied(applied, userTags []string) []string {
	have := make(map[string]struct{}, len(userTags))
	for _, t := range userTags {
		have[t] = struct{}{}
	}
	out := make([]string, 0, len(applied))
	for _, t := range applied {
		if _, ok := have[t]; ok {
			out = append(out, t)
		}
	}
	return out
}
