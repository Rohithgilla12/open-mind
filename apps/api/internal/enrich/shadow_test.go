package enrich_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

func enableAIAssisted(t *testing.T, q *db.Queries, userID uuid.UUID) {
	t.Helper()
	if err := q.UpsertUserSetting(context.Background(), db.UpsertUserSettingParams{
		UserID: userID, Key: jev.SettingKeyAIAssisted, Value: "true",
	}); err != nil {
		t.Fatalf("enable setting: %v", err)
	}
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func TestLiveCapture_Table(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	type want struct {
		calls     int64
		action    string // empty = no row
		userTags  []string
		cardType  string // empty = don't assert
	}
	cases := []struct {
		name      string
		settingOn bool
		apiKey    string
		handler   http.HandlerFunc
		seedTags  []string // vocabulary on another item so tag Nouls fire
		want      want
	}{
		{
			name:      "setting off → no call",
			settingOn: false,
			apiKey:    "test-key",
			handler: func(w http.ResponseWriter, r *http.Request) {
				t.Error("Jev must not be called when setting is off")
				http.Error(w, "no", 500)
			},
			want: want{calls: 0, action: ""},
		},
		{
			name:      "disabled client → skipped row, no HTTP",
			settingOn: true,
			apiKey:    "",
			handler: func(w http.ResponseWriter, r *http.Request) {
				t.Error("Jev must not be called without API key")
				http.Error(w, "no", 500)
			},
			want: want{calls: 0, action: "skipped"},
		},
		{
			name:      "API error → skipped, item still enriched, tags unchanged",
			settingOn: true,
			apiKey:    "test-key",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			},
			want: want{calls: 1, action: "skipped", userTags: []string{}},
		},
		{
			name:      "high-confidence tag → applied",
			settingOn: true,
			apiKey:    "test-key",
			seedTags:  []string{"go", "rust", "search", "postgres", "ai"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"model": "jev-1.13.0",
					"answers": {
						"content_type": {"type": "choice", "choice": "article", "confidence": 0.9},
						"tag:go": {"type": "noul", "noul": 0.91},
						"tag:rust": {"type": "noul", "noul": 0.2}
					},
					"usage": {"input_tokens": 42, "output_tokens": 1}
				}`))
			},
			want: want{calls: 1, action: "applied", userTags: []string{"go"}},
		},
		{
			name:      "mid-confidence tag → suggested, no user tag write",
			settingOn: true,
			apiKey:    "test-key",
			seedTags:  []string{"go", "rust", "search", "postgres", "ai"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"model": "jev-1.13.0",
					"answers": {
						"tag:go": {"type": "noul", "noul": 0.7},
						"tag:rust": {"type": "noul", "noul": 0.1}
					},
					"usage": {"input_tokens": 10, "output_tokens": 0}
				}`))
			},
			want: want{calls: 1, action: "suggested", userTags: []string{}},
		},
		{
			name:      "tool content_type → product card",
			settingOn: true,
			apiKey:    "test-key",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"model": "jev-1.13.0",
					"answers": {
						"content_type": {"type": "choice", "choice": "tool", "confidence": 0.8},
						"is_evergreen": {"type": "noul", "noul": 0.5}
					},
					"usage": {"input_tokens": 8, "output_tokens": 0}
				}`))
			},
			want: want{calls: 1, action: "applied", userTags: []string{}, cardType: "product"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Pool.Exec(ctx, `TRUNCATE items, item_embeddings, jev_decisions, user_settings CASCADE`); err != nil {
				t.Fatalf("truncate: %v", err)
			}
			userID := uuid.New()
			if err := s.Queries.EnsureUser(ctx, userID); err != nil {
				t.Fatalf("ensure user: %v", err)
			}
			if tc.settingOn {
				enableAIAssisted(t, s.Queries, userID)
			}
			if len(tc.seedTags) > 0 {
				seed, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://vocab.example/seed", Body: ""})
				if err != nil {
					t.Fatalf("seed item: %v", err)
				}
				if _, err := s.Queries.SetUserTags(ctx, db.SetUserTagsParams{
					UserID: userID, ID: seed.ID, UserTags: tc.seedTags,
				}); err != nil {
					t.Fatalf("seed tags: %v", err)
				}
			}

			var calls atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.ReadAll(r.Body)
				tc.handler(w, r)
			}))
			t.Cleanup(srv.Close)

			htmlSrv := serveFixture(t, "testdata/article.html")
			item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: htmlSrv.URL, Body: ""})
			if err != nil {
				t.Fatalf("create: %v", err)
			}

			client := jev.New(tc.apiKey, jev.WithBaseURL(srv.URL), jev.WithHTTPClient(srv.Client()))
			p := &enrich.Pipeline{
				Store:     s,
				AI:        ai.NewFake(),
				Extractor: enrich.NewTrafilatura(htmlSrv.Client()),
				Jev:       client,
			}
			if err := p.Run(ctx, userID, item.ID); err != nil {
				t.Fatalf("pipeline: %v", err)
			}

			got, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
			if err != nil {
				t.Fatalf("get item: %v", err)
			}
			if got.Status != "enriched" {
				t.Fatalf("item status = %q, want enriched (save/enrich must succeed)", got.Status)
			}

			if gotCalls := calls.Load(); gotCalls != tc.want.calls {
				t.Fatalf("jev calls = %d, want %d", gotCalls, tc.want.calls)
			}

			if tc.want.userTags != nil {
				if got.UserTags == nil {
					got.UserTags = []string{}
				}
				if !slices.Equal(got.UserTags, tc.want.userTags) {
					t.Fatalf("user_tags = %v, want %v", got.UserTags, tc.want.userTags)
				}
			}
			if tc.want.cardType != "" && got.CardType != tc.want.cardType {
				t.Fatalf("card_type = %q, want %q", got.CardType, tc.want.cardType)
			}

			dec, err := s.Queries.GetJevCaptureDecision(ctx, db.GetJevCaptureDecisionParams{
				UserID: userID,
				ItemID: pgUUID(item.ID),
			})
			if tc.want.action == "" {
				if err == nil {
					t.Fatalf("unexpected decision row: %+v", dec)
				}
				return
			}
			if err != nil {
				t.Fatalf("get decision: %v", err)
			}
			if dec.Action != tc.want.action {
				t.Fatalf("action = %q, want %q", dec.Action, tc.want.action)
			}
			if dec.Surface != jev.SurfaceCapture {
				t.Fatalf("surface = %q", dec.Surface)
			}
			if tc.want.action == "applied" || tc.want.action == "suggested" {
				var answers map[string]json.RawMessage
				if err := json.Unmarshal(dec.Answers, &answers); err != nil || len(answers) == 0 {
					t.Fatalf("answers = %s, err=%v", dec.Answers, err)
				}
			}
			if tc.want.action == "suggested" {
				if !slices.Contains(dec.SuggestedTags, "go") {
					t.Fatalf("suggested_tags = %v, want go", dec.SuggestedTags)
				}
				if len(dec.AppliedTags) != 0 {
					t.Fatalf("applied_tags = %v, want empty", dec.AppliedTags)
				}
			}
			if tc.want.action == "applied" && slices.Equal(tc.want.userTags, []string{"go"}) {
				if !slices.Contains(dec.AppliedTags, "go") {
					t.Fatalf("applied_tags = %v, want go", dec.AppliedTags)
				}
			}
		})
	}
}

func TestLiveCapture_IdempotentOnReEnrich(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `TRUNCATE items, item_embeddings, jev_decisions, user_settings CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	enableAIAssisted(t, s.Queries, userID)

	// Seed vocabulary so tag Nouls are asked.
	seed, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "https://vocab.example/x", Body: ""})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := s.Queries.SetUserTags(ctx, db.SetUserTagsParams{
		UserID: userID, ID: seed.ID, UserTags: []string{"go", "rust", "search", "postgres", "ai"},
	}); err != nil {
		t.Fatalf("seed tags: %v", err)
	}

	var calls atomic.Int64
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "jev-1.13.0",
			"answers": {"tag:go": {"type": "noul", "noul": 0.95}},
			"usage": {"input_tokens": 10, "output_tokens": 0}
		}`))
	}))
	t.Cleanup(jevSrv.Close)

	htmlSrv := serveFixture(t, "testdata/article.html")
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: htmlSrv.URL})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	p := &enrich.Pipeline{
		Store:     s,
		AI:        ai.NewFake(),
		Extractor: enrich.NewTrafilatura(htmlSrv.Client()),
		Jev:       jev.New("key", jev.WithBaseURL(jevSrv.URL), jev.WithHTTPClient(jevSrv.Client())),
	}
	if err := p.Run(ctx, userID, item.ID); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !slices.Equal(first.UserTags, []string{"go"}) {
		t.Fatalf("first user_tags = %v, want [go]", first.UserTags)
	}
	if err := p.Run(ctx, userID, item.ID); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second, err := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if err != nil {
		t.Fatalf("get2: %v", err)
	}
	if first.Summary != second.Summary || first.Title != second.Title {
		t.Fatalf("re-enrich corrupted item")
	}
	if !slices.Equal(second.UserTags, []string{"go"}) {
		t.Fatalf("second user_tags = %v (must not duplicate)", second.UserTags)
	}
	if calls.Load() != 1 {
		t.Fatalf("jev calls = %d, want 1 (second enrich must not re-call)", calls.Load())
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM jev_decisions WHERE user_id=$1 AND item_id=$2`, userID, item.ID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("decision rows = %d, want 1", n)
	}
}
