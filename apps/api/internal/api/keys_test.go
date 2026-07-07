package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/auth"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

type apiKeyCreatedResp struct {
	Key       string `json:"key"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	CreatedAt string `json:"createdAt"`
}

type deviceLinkCreatedResp struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
}

type deviceLinkClaimedResp struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

func TestCreateApiKeyReturnsKeyOnceAndListOmitsIt(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/api-keys", `{"name":"laptop"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created apiKeyCreatedResp
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(created.Key, "omk_") {
		t.Fatalf("created key = %q, want an omk_-prefixed secret", created.Key)
	}
	if created.Name != "laptop" {
		t.Errorf("name = %q, want laptop", created.Name)
	}

	listResp, err := http.Get(srv.URL + "/api-keys")
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	var raw []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("keys = %d, want 1", len(raw))
	}
	if raw[0]["prefix"] != created.Prefix {
		t.Errorf("listed prefix = %v, want %q", raw[0]["prefix"], created.Prefix)
	}
	if _, leaked := raw[0]["key"]; leaked {
		t.Error("list response must never include the full key")
	}
}

func TestCreateApiKeyRejectsEmptyName(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/api-keys", `{"name":"   "}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty name = %d, want 400", resp.StatusCode)
	}
}

func TestRevokeApiKeyStopsAuthenticating(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	create := postJSON(t, srv.URL+"/api-keys", `{"name":"laptop"}`)
	var created apiKeyCreatedResp
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	create.Body.Close()

	before := getWithBearer(t, srv.URL+"/items", created.Key)
	before.Body.Close()
	if before.StatusCode != http.StatusOK {
		t.Fatalf("key before revoke = %d, want 200", before.StatusCode)
	}

	del := doJSON(t, http.MethodDelete, srv.URL+"/api-keys/"+created.ID, "")
	defer del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want 204", del.StatusCode)
	}

	after := getWithBearer(t, srv.URL+"/items", created.Key)
	defer after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Errorf("key after revoke = %d, want 401", after.StatusCode)
	}
}

func TestRevokeApiKeyUnknownIDIs404(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	del := doJSON(t, http.MethodDelete, srv.URL+"/api-keys/"+uuid.NewString(), "")
	defer del.Body.Close()
	if del.StatusCode != http.StatusNotFound {
		t.Errorf("revoke unknown id = %d, want 404", del.StatusCode)
	}
}

func TestDeviceLinkMintClaimAndReuse(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	mintResp := postJSON(t, srv.URL+"/device-links", `{"deviceHint":"iPhone"}`)
	defer mintResp.Body.Close()
	if mintResp.StatusCode != http.StatusCreated {
		t.Fatalf("mint status = %d, want 201", mintResp.StatusCode)
	}
	var link deviceLinkCreatedResp
	if err := json.NewDecoder(mintResp.Body).Decode(&link); err != nil {
		t.Fatalf("decode mint: %v", err)
	}
	if link.Code == "" {
		t.Fatal("expected a non-empty code")
	}

	claimBody := fmt.Sprintf(`{"code":%q,"deviceName":"my iPhone"}`, link.Code)
	claimResp := postJSON(t, srv.URL+"/device-links/claim", claimBody)
	defer claimResp.Body.Close()
	if claimResp.StatusCode != http.StatusCreated {
		t.Fatalf("claim status = %d, want 201", claimResp.StatusCode)
	}
	var claimed deviceLinkClaimedResp
	if err := json.NewDecoder(claimResp.Body).Decode(&claimed); err != nil {
		t.Fatalf("decode claim: %v", err)
	}
	if claimed.Name != "my iPhone" {
		t.Errorf("claimed name = %q, want %q", claimed.Name, "my iPhone")
	}
	if !strings.HasPrefix(claimed.Key, "omk_") {
		t.Fatalf("claimed key = %q, want an omk_-prefixed secret", claimed.Key)
	}

	authed := getWithBearer(t, srv.URL+"/items", claimed.Key)
	defer authed.Body.Close()
	if authed.StatusCode != http.StatusOK {
		t.Errorf("claimed key auth = %d, want 200", authed.StatusCode)
	}

	// Reclaiming the same, already-claimed code must fail: single use.
	reclaim := postJSON(t, srv.URL+"/device-links/claim", claimBody)
	defer reclaim.Body.Close()
	if reclaim.StatusCode != http.StatusNotFound {
		t.Errorf("reclaim = %d, want 404", reclaim.StatusCode)
	}
}

func TestClaimDeviceLinkWrongCodeIs404(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/device-links/claim", `{"code":"ZZZZ-ZZZZ","deviceName":"phone"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("wrong code = %d, want 404", resp.StatusCode)
	}
}

func TestClaimDeviceLinkExpiredIs404(t *testing.T) {
	s, rc, _ := testDeps(t)
	uid := uuid.New()
	if err := s.Queries.EnsureUser(context.Background(), uid); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	code, hash, err := auth.GenerateCode()
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if _, err := s.Queries.CreateDeviceLink(context.Background(), db.CreateDeviceLinkParams{
		CodeHash:   hash,
		UserID:     uid,
		DeviceHint: "",
		ExpiresAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
	}); err != nil {
		t.Fatalf("create expired device link: %v", err)
	}

	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	body := fmt.Sprintf(`{"code":%q,"deviceName":"phone"}`, code)
	resp := postJSON(t, srv.URL+"/device-links/claim", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expired code = %d, want 404", resp.StatusCode)
	}
}

// TestClaimDeviceLinkRateLimited proves the claim endpoint sits behind its own
// strict per-IP bucket (5/min, burst 5) distinct from the general limiter:
// rapid repeated attempts against a wrong code must 429, not just 404 forever.
func TestClaimDeviceLinkRateLimited(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	var last int
	for i := 0; i < 8; i++ {
		resp := postJSON(t, srv.URL+"/device-links/claim", `{"code":"ZZZZ-ZZZZ","deviceName":"probe"}`)
		last = resp.StatusCode
		resp.Body.Close()
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("8th rapid claim request = %d, want 429", last)
	}
}
