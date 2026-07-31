package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

type accountResp struct {
	Email      *string `json:"email"`
	ItemCount  int64   `json:"itemCount"`
	AssetBytes int64   `json:"assetBytes"`
}

func getAccount(t *testing.T, url string) accountResp {
	t.Helper()
	resp, err := http.Get(url + "/account")
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get account status = %d, want 200", resp.StatusCode)
	}
	var out accountResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode account: %v", err)
	}
	return out
}

// TestAccountEmptyOmitsEmail covers the single-user token-mode default: the
// auto-provisioned user has no e-mail, and the endpoint must omit the field
// rather than inventing a display name — the whole reason it exists.
func TestAccountEmptyOmitsEmail(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	got := getAccount(t, srv.URL)
	if got.Email != nil {
		t.Errorf("email = %q, want absent", *got.Email)
	}
	if got.ItemCount != 0 {
		t.Errorf("itemCount = %d, want 0", got.ItemCount)
	}
	if got.AssetBytes != 0 {
		t.Errorf("assetBytes = %d, want 0", got.AssetBytes)
	}
}

// TestAccountCounts checks the totals reflect real rows, and that assetBytes
// sums rather than counts.
func TestAccountCounts(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)
	ctx := context.Background()

	var first uuid.UUID
	for i, u := range []string{
		"https://a.example.com/1",
		"https://a.example.com/2",
		"https://a.example.com/3",
	} {
		item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
			UserID: api.DevUserID, Url: u, Body: "",
		})
		if err != nil {
			t.Fatalf("create item: %v", err)
		}
		if i == 0 {
			first = item.ID
		}
	}
	if _, err := s.Queries.CreateAsset(ctx, db.CreateAssetParams{
		UserID:      api.DevUserID,
		ItemID:      pgtype.UUID{Bytes: first, Valid: true},
		ContentType: "image/png",
		ByteSize:    1500,
	}); err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if _, err := s.Queries.CreateAsset(ctx, db.CreateAssetParams{
		UserID:      api.DevUserID,
		ItemID:      pgtype.UUID{Bytes: first, Valid: true},
		ContentType: "image/png",
		ByteSize:    2500,
	}); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	got := getAccount(t, srv.URL)
	if got.ItemCount != 3 {
		t.Errorf("itemCount = %d, want 3", got.ItemCount)
	}
	if got.AssetBytes != 4000 {
		t.Errorf("assetBytes = %d, want 4000 (sum, not count)", got.AssetBytes)
	}
}

// TestAccountIsolatesTenants is the important one: totals must be scoped to the
// caller. A second user's items and assets must not leak into the response.
func TestAccountIsolatesTenants(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)
	ctx := context.Background()

	mine, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
		UserID: api.DevUserID, Url: "https://mine.example.com/1", Body: "",
	})
	if err != nil {
		t.Fatalf("create own item: %v", err)
	}
	if _, err := s.Queries.CreateAsset(ctx, db.CreateAssetParams{
		UserID:      api.DevUserID,
		ItemID:      pgtype.UUID{Bytes: mine.ID, Valid: true},
		ContentType: "image/png",
		ByteSize:    100,
	}); err != nil {
		t.Fatalf("create own asset: %v", err)
	}

	other := uuid.MustParse("00000000-0000-0000-0000-0000000000fe")
	if err := s.Queries.EnsureUser(ctx, other); err != nil {
		t.Fatalf("ensure other user: %v", err)
	}
	for _, u := range []string{"https://other.example.com/1", "https://other.example.com/2"} {
		otherItem, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
			UserID: other, Url: u, Body: "",
		})
		if err != nil {
			t.Fatalf("create other item: %v", err)
		}
		if _, err := s.Queries.CreateAsset(ctx, db.CreateAssetParams{
			UserID:      other,
			ItemID:      pgtype.UUID{Bytes: otherItem.ID, Valid: true},
			ContentType: "image/png",
			ByteSize:    9_000_000,
		}); err != nil {
			t.Fatalf("create other asset: %v", err)
		}
	}

	got := getAccount(t, srv.URL)
	if got.ItemCount != 1 {
		t.Errorf("itemCount = %d, want 1 (other tenant's items leaked)", got.ItemCount)
	}
	if got.AssetBytes != 100 {
		t.Errorf("assetBytes = %d, want 100 (other tenant's bytes leaked)", got.AssetBytes)
	}
}
