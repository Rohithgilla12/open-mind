package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rohithgilla12/openmind/api/internal/auth"
)

const testKid = "test1"

type testClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email,omitempty"`
}

func newTestJWKSServer(t *testing.T, pub *rsa.PublicKey) (*httptest.Server, *int64) {
	t.Helper()
	var fetchCount int64

	nBytes := pub.N.Bytes()
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	jwksBody := map[string]any{
		"keys": []map[string]any{
			{
				"kid": testKid,
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(nBytes),
				"e":   base64.RawURLEncoding.EncodeToString(eBytes),
			},
		},
	}
	body, err := json.Marshal(jwksBody)
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&fetchCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &fetchCount
}

func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims testClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return signed
}

func TestJWTVerifierValidToken(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	srv, _ := newTestJWKSServer(t, &priv.PublicKey)

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    srv.URL,
			Subject:   "user_abc123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Email: "person@example.com",
	}
	signed := signToken(t, priv, testKid, claims)

	v := auth.NewJWTVerifier(srv.URL)
	got, err := v.Verify(context.Background(), signed)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Subject != "user_abc123" {
		t.Errorf("Subject = %q, want user_abc123", got.Subject)
	}
	if got.Email != "person@example.com" {
		t.Errorf("Email = %q, want person@example.com", got.Email)
	}
}

func TestJWTVerifierMissingEmailClaim(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	srv, _ := newTestJWKSServer(t, &priv.PublicKey)

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    srv.URL,
			Subject:   "user_no_email",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed := signToken(t, priv, testKid, claims)

	v := auth.NewJWTVerifier(srv.URL)
	got, err := v.Verify(context.Background(), signed)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Email != "" {
		t.Errorf("Email = %q, want empty", got.Email)
	}
}

func TestJWTVerifierMissingSub(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	srv, _ := newTestJWKSServer(t, &priv.PublicKey)

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    srv.URL,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed := signToken(t, priv, testKid, claims)

	v := auth.NewJWTVerifier(srv.URL)
	if _, err := v.Verify(context.Background(), signed); err == nil {
		t.Fatal("Verify with missing sub succeeded, want error")
	}
}

func TestJWTVerifierExpiredToken(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	srv, _ := newTestJWKSServer(t, &priv.PublicKey)

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    srv.URL,
			Subject:   "user_abc123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	signed := signToken(t, priv, testKid, claims)

	v := auth.NewJWTVerifier(srv.URL)
	if _, err := v.Verify(context.Background(), signed); err == nil {
		t.Fatal("Verify with expired token succeeded, want error")
	}
}

func TestJWTVerifierWrongIssuer(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	srv, _ := newTestJWKSServer(t, &priv.PublicKey)

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://not-the-right-issuer.example",
			Subject:   "user_abc123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed := signToken(t, priv, testKid, claims)

	v := auth.NewJWTVerifier(srv.URL)
	if _, err := v.Verify(context.Background(), signed); err == nil {
		t.Fatal("Verify with wrong issuer succeeded, want error")
	}
}

func TestJWTVerifierRejectsHS256(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	srv, _ := newTestJWKSServer(t, &priv.PublicKey)

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    srv.URL,
			Subject:   "user_abc123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = testKid
	signed, err := tok.SignedString([]byte("some-shared-secret"))
	if err != nil {
		t.Fatalf("signing HS256 token: %v", err)
	}

	v := auth.NewJWTVerifier(srv.URL)
	if _, err := v.Verify(context.Background(), signed); err == nil {
		t.Fatal("Verify accepted an HS256 token, want error")
	}
}

func TestJWTVerifierTamperedSignature(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	srv, _ := newTestJWKSServer(t, &priv.PublicKey)

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    srv.URL,
			Subject:   "user_abc123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed := signToken(t, priv, testKid, claims)

	parts := strings.Split(signed, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token shape: %d parts", len(parts))
	}
	tampered := parts[0] + "." + parts[1] + "." + flipMiddleChar(parts[2])

	v := auth.NewJWTVerifier(srv.URL)
	if _, err := v.Verify(context.Background(), tampered); err == nil {
		t.Fatal("Verify accepted a tampered signature, want error")
	}
}

func TestJWTVerifierUnknownKidCooldown(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	srv, fetchCount := newTestJWKSServer(t, &priv.PublicKey)

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    srv.URL,
			Subject:   "user_abc123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed := signToken(t, priv, "not-in-jwks", claims)

	v := auth.NewJWTVerifier(srv.URL)
	if _, err := v.Verify(context.Background(), signed); err == nil {
		t.Fatal("Verify with unknown kid succeeded, want error")
	}
	if got := atomic.LoadInt64(fetchCount); got != 1 {
		t.Fatalf("fetch count after first unknown kid = %d, want 1", got)
	}

	// A second unknown-kid token, verified immediately after, must not
	// trigger another JWKS fetch: refetches are throttled to one per
	// cooldown window so a stream of bad kids can't be used to hammer the
	// JWKS endpoint.
	signed2 := signToken(t, priv, "also-not-in-jwks", claims)
	if _, err := v.Verify(context.Background(), signed2); err == nil {
		t.Fatal("Verify with second unknown kid succeeded, want error")
	}
	if got := atomic.LoadInt64(fetchCount); got != 1 {
		t.Fatalf("fetch count after second unknown kid = %d, want 1 (cooldown should block refetch)", got)
	}
}

// flipMiddleChar mutates one base64url character well inside the string
// (not the final character, whose low-order bits are unused decode padding
// and could tamper the encoding without changing the decoded bytes).
func flipMiddleChar(s string) string {
	if len(s) < 2 {
		return s
	}
	i := len(s) / 2
	replacement := byte('A')
	if s[i] == 'A' {
		replacement = 'B'
	}
	return s[:i] + string(replacement) + s[i+1:]
}

func TestVerify_FailedFetchDoesNotPoisonRefetchCooldown(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var failing int64 = 1
	srv, fetches := newTestJWKSServer(t, &key.PublicKey)
	// Wrap the server: while failing==1, return 500s.
	wrapped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt64(&failing) == 1 {
			atomic.AddInt64(fetches, 1)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.URL.Path = "/.well-known/jwks.json"
		srv.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(wrapped.Close)

	v := auth.NewJWTVerifier(wrapped.URL)
	tok := signToken(t, key, testKid, testClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: wrapped.URL, Subject: "user_1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}})

	// First attempt: fetch fails.
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error while jwks is failing")
	}
	// Immediately after a FAILURE we must be on the short retry cooldown, not
	// the long refetch cooldown (which only a success may start).
	_, err = v.Verify(context.Background(), tok)
	if err == nil || !strings.Contains(err.Error(), "retry cooldown") {
		t.Fatalf("want short retry-cooldown error after failed fetch, got: %v", err)
	}
	if strings.Contains(err.Error(), "refetch on cooldown") {
		t.Fatalf("failed fetch poisoned the long refetch cooldown: %v", err)
	}
	atomic.StoreInt64(&failing, 0)
}

func TestVerify_ConcurrentKidMissesCoalesceIntoOneFetch(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	srv, fetches := newTestJWKSServer(t, &key.PublicKey)
	// Slow the fetch down so the goroutines genuinely overlap.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		r.URL.Path = "/.well-known/jwks.json"
		srv.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(slow.Close)

	v := auth.NewJWTVerifier(slow.URL)
	tok := signToken(t, key, testKid, testClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: slow.URL, Subject: "user_1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}})

	const n = 8
	errs := make(chan error, n)
	for range n {
		go func() {
			_, err := v.Verify(context.Background(), tok)
			errs <- err
		}()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent verify failed (should have coalesced onto the in-flight fetch): %v", err)
		}
	}
	if got := atomic.LoadInt64(fetches); got != 1 {
		t.Fatalf("want exactly 1 jwks fetch, got %d", got)
	}
}
