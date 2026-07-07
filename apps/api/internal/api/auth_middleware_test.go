package api_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/auth"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// mintAPIKey inserts an API key row for uid directly through the store
// (bypassing the HTTP layer, which is what's under test here) and returns the
// full secret.
func mintAPIKey(t *testing.T, s *store.Store, uid uuid.UUID, name string) string {
	t.Helper()
	full, hash, prefix, err := auth.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := s.Queries.CreateAPIKey(context.Background(), db.CreateAPIKeyParams{
		UserID: uid, Name: name, KeyHash: hash, Prefix: prefix,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return full
}

func getWithBearer(t *testing.T, url, bearer string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return resp
}

func TestAuthenticateLegacyTokenInTokenMode(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrvWithAuthConfig(t, s, rc, api.AuthConfig{Mode: api.AuthModeToken, LegacyToken: "sekret"}))
	t.Cleanup(srv.Close)

	if resp := getWithBearer(t, srv.URL+"/items", "sekret"); resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Errorf("correct legacy token = %d, want 200", resp.StatusCode)
	} else {
		resp.Body.Close()
	}

	if resp := getWithBearer(t, srv.URL+"/items", "wrong"); resp.StatusCode != http.StatusUnauthorized {
		resp.Body.Close()
		t.Errorf("wrong legacy token = %d, want 401", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
}

func TestAuthenticateAPIKeyWorksInTokenMode(t *testing.T) {
	s, rc, _ := testDeps(t)
	uid := uuid.New()
	if err := s.Queries.EnsureUser(context.Background(), uid); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	full := mintAPIKey(t, s, uid, "laptop")

	srv := httptest.NewServer(newSrvWithAuthConfig(t, s, rc, api.AuthConfig{Mode: api.AuthModeToken, LegacyToken: "sekret"}))
	t.Cleanup(srv.Close)

	resp := getWithBearer(t, srv.URL+"/items", full)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("omk_ key in token mode = %d, want 200", resp.StatusCode)
	}
}

func TestAuthenticateAPIKeyWorksInClerkMode(t *testing.T) {
	s, rc, _ := testDeps(t)
	uid := uuid.New()
	if err := s.Queries.EnsureUser(context.Background(), uid); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	full := mintAPIKey(t, s, uid, "phone")

	srv := httptest.NewServer(newSrvWithAuthConfig(t, s, rc, api.AuthConfig{Mode: api.AuthModeClerk, Verifier: auth.NewJWTVerifier("https://issuer.invalid")}))
	t.Cleanup(srv.Close)

	resp := getWithBearer(t, srv.URL+"/items", full)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("omk_ key in clerk mode = %d, want 200", resp.StatusCode)
	}
}

func TestAuthenticateRevokedAPIKeyIsUnauthorized(t *testing.T) {
	s, rc, _ := testDeps(t)
	uid := uuid.New()
	if err := s.Queries.EnsureUser(context.Background(), uid); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	full := mintAPIKey(t, s, uid, "old-laptop")

	rows, err := s.Queries.ListAPIKeys(context.Background(), uid)
	if err != nil || len(rows) != 1 {
		t.Fatalf("listing minted key: rows=%v err=%v", rows, err)
	}
	if n, err := s.Queries.RevokeAPIKey(context.Background(), db.RevokeAPIKeyParams{UserID: uid, ID: rows[0].ID}); err != nil || n != 1 {
		t.Fatalf("revoke: rows=%d err=%v", n, err)
	}

	srv := httptest.NewServer(newSrvWithAuthConfig(t, s, rc, api.AuthConfig{Mode: api.AuthModeToken}))
	t.Cleanup(srv.Close)

	resp := getWithBearer(t, srv.URL+"/items", full)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("revoked key = %d, want 401", resp.StatusCode)
	}
}

func TestAuthenticateGarbageAPIKeyIsUnauthorized(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrvWithAuthConfig(t, s, rc, api.AuthConfig{Mode: api.AuthModeToken}))
	t.Cleanup(srv.Close)

	resp := getWithBearer(t, srv.URL+"/items", "omk_thisisnotarealkey")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("garbage omk_ key = %d, want 401", resp.StatusCode)
	}
}

// --- Clerk JWT path, using the same fake-JWKS pattern as internal/auth/jwt_test.go. ---

func newFakeJWKSServer(t *testing.T, pub *rsa.PublicKey, kid string) *httptest.Server {
	t.Helper()
	nBytes := pub.N.Bytes()
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	body, err := json.Marshal(map[string]any{
		"keys": []map[string]any{
			{
				"kid": kid,
				"kty": "RSA",
				"n":   base64.RawURLEncoding.EncodeToString(nBytes),
				"e":   base64.RawURLEncoding.EncodeToString(eBytes),
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type fakeClerkClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email,omitempty"`
}

func signFakeClerkToken(t *testing.T, priv *rsa.PrivateKey, kid, issuer, subject, email string) string {
	t.Helper()
	claims := fakeClerkClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Email: email,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return signed
}

// TestAuthenticateClerkJWTJITProvisionsUser proves that a valid Clerk JWT for
// an unseen subject provisions a new user row on first sight, and that a
// second request with a token for the same subject reuses that row rather
// than creating a duplicate.
func TestAuthenticateClerkJWTJITProvisionsUser(t *testing.T) {
	s, rc, _ := testDeps(t)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	const kid = "test-kid"
	jwks := newFakeJWKSServer(t, &priv.PublicKey, kid)
	verifier := auth.NewJWTVerifier(jwks.URL)

	srv := httptest.NewServer(newSrvWithAuthConfig(t, s, rc, api.AuthConfig{Mode: api.AuthModeClerk, Verifier: verifier}))
	t.Cleanup(srv.Close)

	subject := "user_" + uuid.NewString()
	token := signFakeClerkToken(t, priv, kid, jwks.URL, subject, "person@example.com")

	resp := getWithBearer(t, srv.URL+"/items", token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first request = %d, want 200", resp.StatusCode)
	}

	clerkID := pgtype.Text{String: subject, Valid: true}
	user, err := s.Queries.GetUserByClerkID(context.Background(), clerkID)
	if err != nil {
		t.Fatalf("expected user row to be JIT-provisioned: %v", err)
	}
	if user.Email != "person@example.com" {
		t.Errorf("provisioned email = %q, want person@example.com", user.Email)
	}

	// A second call with a fresh token for the same subject must reuse the
	// same row, not create a second one.
	token2 := signFakeClerkToken(t, priv, kid, jwks.URL, subject, "person@example.com")
	resp2 := getWithBearer(t, srv.URL+"/items", token2)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second request = %d, want 200", resp2.StatusCode)
	}
	user2, err := s.Queries.GetUserByClerkID(context.Background(), clerkID)
	if err != nil {
		t.Fatalf("re-fetching provisioned user: %v", err)
	}
	if user2.ID != user.ID {
		t.Errorf("second call provisioned a new user (id %s != %s), want the same row reused", user2.ID, user.ID)
	}
}

func TestAuthenticateClerkModeRejectsGarbageBearer(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrvWithAuthConfig(t, s, rc, api.AuthConfig{Mode: api.AuthModeClerk, Verifier: auth.NewJWTVerifier("https://issuer.invalid")}))
	t.Cleanup(srv.Close)

	resp := getWithBearer(t, srv.URL+"/items", "not-a-jwt")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("garbage bearer in clerk mode = %d, want 401", resp.StatusCode)
	}
}
