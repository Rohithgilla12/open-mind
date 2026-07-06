package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rohithgilla12/openmind/api/internal/api"
)

// setDriftFields sets status, created_at, pinned_at, and last_drifted_at on an
// item directly, giving Drift tests deterministic candidacy and ordering
// independent of wall-clock timing. Nil times leave the column NULL.
func setDriftFields(t *testing.T, pool *pgxpool.Pool, id, status string, createdAt time.Time, pinnedAt, lastDriftedAt *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE items SET status = $2, created_at = $3, pinned_at = $4, last_drifted_at = $5 WHERE id = $1`,
		uuid.MustParse(id), status, createdAt, pinnedAt, lastDriftedAt)
	if err != nil {
		t.Fatalf("set drift fields: %v", err)
	}
}

func getDrift(t *testing.T, baseURL string) api.DriftResponse {
	t.Helper()
	resp, err := http.Get(baseURL + "/drift")
	if err != nil {
		t.Fatalf("get drift: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("drift status = %d, want 200", resp.StatusCode)
	}
	var out api.DriftResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode drift: %v", err)
	}
	return out
}

func postDrift(t *testing.T, baseURL, id string, keep bool) *http.Response {
	t.Helper()
	body := `{"keep":false}`
	if keep {
		body = `{"keep":true}`
	}
	resp, err := http.Post(baseURL+"/drift/"+id, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post drift: %v", err)
	}
	return resp
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestGetDriftReturnsOrderedCandidatesOnly(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	now := time.Now()
	// Candidates: enriched, unpinned, not recently drifted.
	oldest := createNoteItem(t, srv.URL, "candidate oldest never drifted")
	newer := createNoteItem(t, srv.URL, "candidate newer never drifted")
	longAgo := createNoteItem(t, srv.URL, "candidate drifted 40 days ago")
	setDriftFields(t, pool, oldest, "enriched", now.Add(-10*24*time.Hour), nil, nil)
	setDriftFields(t, pool, newer, "enriched", now.Add(-5*24*time.Hour), nil, nil)
	setDriftFields(t, pool, longAgo, "enriched", now.Add(-8*24*time.Hour), nil, ptrTime(now.Add(-40*24*time.Hour)))

	// Excluded rows.
	pinned := createNoteItem(t, srv.URL, "pinned excluded")
	pending := createNoteItem(t, srv.URL, "pending excluded")
	recent := createNoteItem(t, srv.URL, "recently drifted excluded")
	setDriftFields(t, pool, pinned, "enriched", now.Add(-20*24*time.Hour), ptrTime(now), nil)
	setDriftFields(t, pool, pending, "pending", now.Add(-20*24*time.Hour), nil, nil)
	setDriftFields(t, pool, recent, "enriched", now.Add(-20*24*time.Hour), nil, ptrTime(now.Add(-5*24*time.Hour)))

	out := getDrift(t, srv.URL)
	if out.Total != 3 {
		t.Errorf("total = %d, want 3 candidates", out.Total)
	}
	if len(out.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(out.Items))
	}
	// Ordering: never-drifted first (NULLS FIRST) by created_at ASC, then drifted.
	wantOrder := []string{oldest, newer, longAgo}
	for i, want := range wantOrder {
		if out.Items[i].Id.String() != want {
			t.Errorf("item[%d] = %s, want %s", i, out.Items[i].Id, want)
		}
	}
}

func TestGetDriftCapsAtFiveButTotalCountsAll(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	now := time.Now()
	for i := 0; i < 7; i++ {
		id := createNoteItem(t, srv.URL, "candidate")
		setDriftFields(t, pool, id, "enriched", now.Add(-time.Duration(i)*time.Hour), nil, nil)
	}

	out := getDrift(t, srv.URL)
	if len(out.Items) != 5 {
		t.Errorf("items = %d, want batch capped at 5", len(out.Items))
	}
	if out.Total != 7 {
		t.Errorf("total = %d, want 7 (all candidates)", out.Total)
	}
}

func TestGetDriftEmptyReturnsArray(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	out := getDrift(t, srv.URL)
	if out.Items == nil {
		t.Errorf("items = nil, want non-null empty array")
	}
	if len(out.Items) != 0 || out.Total != 0 {
		t.Errorf("empty drift = %+v, want 0 items / 0 total", out)
	}
}

func TestDriftKeepPinsAndDropsFromNextBatch(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	id := createNoteItem(t, srv.URL, "keep me")
	setDriftFields(t, pool, id, "enriched", time.Now().Add(-time.Hour), nil, nil)

	resp := postDrift(t, srv.URL, id, true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post drift keep status = %d, want 200", resp.StatusCode)
	}

	// Kept item is pinned → appears on the Desk.
	desk := getDesk(t, srv.URL)
	if len(desk) != 1 || desk[0]["id"] != id {
		t.Errorf("desk = %v, want the kept item %s", desk, id)
	}
	// And it no longer resurfaces (pinned + freshly drifted).
	out := getDrift(t, srv.URL)
	if out.Total != 0 || len(out.Items) != 0 {
		t.Errorf("drift after keep = %+v, want empty", out)
	}
}

func TestDriftLetGoMarksButDoesNotPin(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	id := createNoteItem(t, srv.URL, "let me go")
	setDriftFields(t, pool, id, "enriched", time.Now().Add(-time.Hour), nil, nil)

	resp := postDrift(t, srv.URL, id, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post drift letgo status = %d, want 200", resp.StatusCode)
	}

	// Not pinned: Desk stays empty.
	if desk := getDesk(t, srv.URL); len(desk) != 0 {
		t.Errorf("desk = %v, want empty (letgo does not pin)", desk)
	}
	// last_drifted_at set → excluded from the next batch.
	if out := getDrift(t, srv.URL); out.Total != 0 || len(out.Items) != 0 {
		t.Errorf("drift after letgo = %+v, want empty", out)
	}
}

func TestDriftMissingOrCrossTenantReturns404(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := newHTTPTest(t, s, rc)

	// Unknown id.
	resp := postDrift(t, srv.URL, uuid.NewString(), true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown drift status = %d, want 404", resp.StatusCode)
	}

	// Cross-tenant: another user's item.
	other := seedOtherUserItem(t, s, "theirs")
	resp = postDrift(t, srv.URL, other, true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant drift status = %d, want 404", resp.StatusCode)
	}
}
