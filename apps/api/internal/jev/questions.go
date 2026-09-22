package jev

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// QuestionsVersion labels this file for a future jev_decisions.questions_v
// column. Bump it when instructions, criteria, option lists, or thresholds change.
const QuestionsVersion = "v0"

// Provisional thresholds from the Openmind × Jev spec, section 7.
// They are starting points until shadow-capture data exists. Do not treat
// them as tuned.
const (
	// TagAutoApplyAbove is exclusive. A tag Noul must be greater than this
	// to auto-apply. The value itself is still a suggestion.
	TagAutoApplyAbove = 0.85

	// TagSuggestMin is inclusive. From here through TagAutoApplyAbove the tag
	// is a one-tap suggestion. Below this it is skipped.
	TagSuggestMin = 0.50

	// ContentTypeMinConfidence is inclusive. A content_type choice below this
	// is left unused.
	ContentTypeMinConfidence = 0.60

	// RerankVectorWeight and RerankJevWeight blend embedding similarity with
	// the rerank Noul. They sum to 1.
	RerankVectorWeight = 0.4
	RerankJevWeight    = 0.6

	// ColdStartTags is the minimum distinct tags before tag Nouls are asked.
	// Fewer than this and CaptureQuestions asks only the fixed questions.
	ColdStartTags = 5

	// ExcerptMaxChars caps a capture or drift excerpt. The model sees a
	// passage, never the full document.
	ExcerptMaxChars = 2000

	// SnippetMaxChars caps one rerank candidate snippet.
	SnippetMaxChars = 300
)

// Fixed question ids. Tag and candidate ids are prefixed; the model never
// sees the id, only instructions.
const (
	QContentType = "content_type"
	QDepth       = "depth"
	QEvergreen   = "is_evergreen"

	QWorthResurfacing = "worth_resurfacing"
	QStillActionable  = "still_actionable"

	TagQuestionPrefix       = "tag:"
	CandidateQuestionPrefix = "candidate_"
)

// Content-type choice ids. These are Jev's capture labels, not Openmind card
// types.
const (
	ContentArticle    = "article"
	ContentDocs       = "docs"
	ContentPaper      = "paper"
	ContentTool       = "tool"
	ContentVideo      = "video"
	ContentDiscussion = "discussion"
	ContentOther      = "other"
)

// TagDecision is what code does with one tag Noul. The strings match the
// planned jev_decisions.action values.
type TagDecision string

const (
	ActionApplied   TagDecision = "applied"
	ActionSuggested TagDecision = "suggested"
	ActionSkipped   TagDecision = "skipped"
)

// DecideTag maps a tag Noul to an action. NaN and values below TagSuggestMin
// are skipped.
func DecideTag(p float64) TagDecision {
	if p > TagAutoApplyAbove {
		return ActionApplied
	}
	if p >= TagSuggestMin {
		return ActionSuggested
	}
	return ActionSkipped
}

// AcceptContentType reports whether a content_type confidence is high enough
// to keep. Pass 0 when the response omitted confidence; that leaves the item
// untyped.
func AcceptContentType(confidence float64) bool {
	return confidence >= ContentTypeMinConfidence
}

// BlendRerank combines embedding similarity and a rerank Noul.
func BlendRerank(vectorSim, jevRelevance float64) float64 {
	return RerankVectorWeight*vectorSim + RerankJevWeight*jevRelevance
}

// CaptureState is the only capture text sent to Jev: URL, title, site, and a
// clipped excerpt. Never the full document, never another item.
type CaptureState struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Site    string `json:"site"`
	Excerpt string `json:"excerpt"`
}

// BuildCaptureState clips the excerpt to ExcerptMaxChars runes.
func BuildCaptureState(url, title, site, excerpt string) CaptureState {
	return CaptureState{
		URL:     url,
		Title:   title,
		Site:    site,
		Excerpt: clipRunes(excerpt, ExcerptMaxChars),
	}
}

// CaptureQuestions is the one capture call: content type, depth, evergreen,
// and one Noul per tag once the user has at least ColdStartTags distinct tags.
// Multi-tag is per-tag Nouls, not a Choice.
func CaptureQuestions(tags []string) map[string]Question {
	qs := map[string]Question{
		QContentType: Choice(
			"Which kind of item is this, based on `title`, `site`, `url`, and `excerpt`?",
			map[string]string{
				ContentArticle:    "A written piece: an essay, news story, blog post, or magazine article.",
				ContentDocs:       "Reference documentation for a product, library, or API.",
				ContentPaper:      "A scholarly or technical paper, preprint, or formal study.",
				ContentTool:       "A product, app, repository, or service someone could use.",
				ContentVideo:      "A video, or a page whose main content is a video.",
				ContentDiscussion: "A thread, forum, issue, or conversation among people.",
				ContentOther:      "None of the above.",
			},
		),
		QDepth: Score(
			"How much is here to come back to, based on `title` and `excerpt`?",
			[]string{
				"A glance. A headline, a link with a line of context, or a few sentences. There is nothing to work through.",
				"A working piece. A post, thread, or short document you would reopen to follow one argument or one task.",
				"A keeper. A reference, paper, or long treatment you would return to and work from.",
			},
		),
		QEvergreen: Noul(
			"Will the substance of this item (`title` and `excerpt`) still be worth opening a year from now?",
			"It does not depend on a news cycle, a launch, a price, or a version that will be stale within a year.",
			"It is timely: news, a changelog, a deal, a release note, or something tied to a specific moment.",
		),
	}
	vocab := distinctTags(tags)
	if len(vocab) < ColdStartTags {
		return qs
	}
	for _, tag := range vocab {
		qs[TagQuestionPrefix+tag] = Question{
			Type: TypeNoul,
			Instructions: map[string]string{
				"tag":      tag,
				"question": "Does this item substantively belong under `tag`, beyond a passing mention?",
			},
			Criteria: noulCriteria{
				True:  "Someone who saves things under this tag would expect this item there.",
				False: "The tag is absent, incidental, or only a shared word.",
			},
		}
	}
	return qs
}

// RerankCandidate is one embedding hit. Snippet is clipped in BuildRerankState.
type RerankCandidate struct {
	ID      string `json:"id"`
	Title   string `json:"title,omitempty"`
	Snippet string `json:"snippet"`
}

// RerankState is the query plus the candidate list the rerank Nouls point at.
type RerankState struct {
	Query      string            `json:"query"`
	Candidates []RerankCandidate `json:"candidates"`
}

// BuildRerankState clips each snippet to SnippetMaxChars runes and keeps order.
func BuildRerankState(query string, candidates []RerankCandidate) RerankState {
	out := make([]RerankCandidate, len(candidates))
	for i, cand := range candidates {
		cand.Snippet = clipRunes(cand.Snippet, SnippetMaxChars)
		out[i] = cand
	}
	return RerankState{Query: query, Candidates: out}
}

// RerankQuestions asks one Noul per candidate. The answer id candidate_N
// matches candidates[N] in the state from BuildRerankState. Build both from
// the same slice, in the same order.
func RerankQuestions(candidates []RerankCandidate) map[string]Question {
	qs := make(map[string]Question, len(candidates))
	for i := range candidates {
		qs[fmt.Sprintf("%s%d", CandidateQuestionPrefix, i)] = Noul(
			fmt.Sprintf("Is `candidates[%d]` substantively relevant to `query`, beyond sharing keywords?", i),
			"It would help someone who asked the query. The overlap is the subject, not just the words.",
			"It only shares vocabulary, or it is about something else.",
		)
	}
	return qs
}

// DriftItem is the saved thing Drift is considering, not the user's whole library.
type DriftItem struct {
	Title   string `json:"title"`
	URL     string `json:"url,omitempty"`
	Excerpt string `json:"excerpt"`
}

// DriftState pairs that item with a caller-built summary of the last 14 days
// of saves and opens. recent_activity must already be a summary.
type DriftState struct {
	Item           DriftItem `json:"item"`
	RecentActivity string    `json:"recent_activity"`
}

// BuildDriftState clips the item excerpt. It does not summarise activity;
// that summary is the caller's.
func BuildDriftState(item DriftItem, recentActivity string) DriftState {
	item.Excerpt = clipRunes(item.Excerpt, ExcerptMaxChars)
	return DriftState{Item: item, RecentActivity: recentActivity}
}

// DriftQuestions asks whether to resurface the item today and whether it is
// still actionable. Rank the blend in code; these two stay separate.
func DriftQuestions() map[string]Question {
	return map[string]Question{
		QWorthResurfacing: Noul(
			"Given `recent_activity` (saves and opens from the last 14 days), is `item` worth showing today?",
			"It connects to that recent activity, and it is not something the user just saw.",
			"It is unrelated to the last 14 days, or the user already opened it in that window.",
		),
		QStillActionable: Noul(
			"Is `item` still something the user could act on?",
			"A step, question, or use is still open.",
			"It is finished, expired, or only worth keeping as a record.",
		),
	}
}

func distinctTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func clipRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}
