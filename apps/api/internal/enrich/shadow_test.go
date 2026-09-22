package enrich_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestShadowCapture_Table(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	type want struct {
		calls  int64
		action string // empty = no row
	}
	cases := []struct {
		name      string
		settingOn bool
		apiKey    string
		handler   http.HandlerFunc
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
			// Evaluate returns ErrDisabled before any request; we still log skipped.
			want: want{calls: 0, action: "skipped"},
		},
		{
			name:      "API error → skipped, item still enriched",
			settingOn: true,
			apiKey:    "test-key",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			},
			want: want{calls: 1, action: "skipped"},
		},
		{
			name:      "success → shadow",
			settingOn: true,
			apiKey:    "test-key",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"model": "jev-1.13.0",
					"answers": {"content_type": {"type": "choice", "choice": "article", "confidence": 0.9},
						"depth": {"type": "score", "score": 1.0},
						"is_evergreen": {"type": "noul", "noul": 0.8}},
					"usage": {"input_tokens": 42, "output_tokens": 1}
				}`))
			},
			want: want{calls: 1, action: "shadow"},
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
			if dec.QuestionsV != jev.QuestionsVersion {
				t.Fatalf("questions_v = %q", dec.QuestionsV)
			}
			if tc.want.action == "shadow" {
				var answers map[string]json.RawMessage
				if err := json.Unmarshal(dec.Answers, &answers); err != nil || len(answers) == 0 {
					t.Fatalf("answers = %s, err=%v", dec.Answers, err)
				}
				if !dec.InputTokens.Valid || dec.InputTokens.Int32 != 42 {
					t.Fatalf("input_tokens = %+v, want 42", dec.InputTokens)
				}
			}
		})
	}
}

func TestShadowCapture_IdempotentOnReEnrich(t *testing.T) {
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

	var calls atomic.Int64
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "jev-1.13.0",
			"answers": {"is_evergreen": {"type": "noul", "noul": 0.5}},
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
