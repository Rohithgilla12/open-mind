package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwksFetchTimeout    = 10 * time.Second
	jwksRefetchCooldown = 5 * time.Minute
	jwtLeeway           = 30 * time.Second
)

// ClerkClaims are the claims we care about out of a Clerk session JWT.
type ClerkClaims struct {
	Subject string
	Email   string
}

type clerkTokenClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email,omitempty"`
}

// JWTVerifier verifies Clerk-issued RS256 session JWTs against a cached JWKS.
type JWTVerifier struct {
	issuer  string
	jwksURL string
	client  *http.Client

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	lastFetch time.Time
}

// NewJWTVerifier builds a verifier for tokens issued by issuer. The JWKS URL
// is derived as issuer + "/.well-known/jwks.json".
func NewJWTVerifier(issuer string) *JWTVerifier {
	return &JWTVerifier{
		issuer:  issuer,
		jwksURL: issuer + "/.well-known/jwks.json",
		client:  &http.Client{Timeout: jwksFetchTimeout},
		keys:    make(map[string]*rsa.PublicKey),
	}
}

// Verify checks the signature, issuer, and expiry of token and returns the
// Clerk claims it carries. Only RS256-signed tokens are accepted.
func (v *JWTVerifier) Verify(ctx context.Context, token string) (ClerkClaims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithLeeway(jwtLeeway),
		jwt.WithExpirationRequired(),
	)

	var claims clerkTokenClaims
	_, err := parser.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, fmt.Errorf("token header missing kid")
		}
		return v.keyForKid(ctx, kid)
	})
	if err != nil {
		return ClerkClaims{}, fmt.Errorf("verifying token: %w", err)
	}
	if claims.Subject == "" {
		return ClerkClaims{}, fmt.Errorf("token missing sub claim")
	}

	return ClerkClaims{Subject: claims.Subject, Email: claims.Email}, nil
}

// keyForKid returns the cached RSA public key for kid, refetching the JWKS
// if it isn't known yet. Refetches are throttled to at most one per
// jwksRefetchCooldown so a stream of tokens with bogus kids can't be used to
// hammer the JWKS endpoint.
func (v *JWTVerifier) keyForKid(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	if key, ok := v.keys[kid]; ok {
		v.mu.Unlock()
		return key, nil
	}
	if time.Since(v.lastFetch) < jwksRefetchCooldown {
		v.mu.Unlock()
		return nil, fmt.Errorf("unknown key id %q (jwks refetch on cooldown)", kid)
	}
	v.lastFetch = time.Now()
	v.mu.Unlock()

	if err := v.fetchJWKS(ctx); err != nil {
		return nil, fmt.Errorf("fetching jwks: %w", err)
	}

	v.mu.Lock()
	key, ok := v.keys[kid]
	v.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown key id %q", kid)
	}
	return key, nil
}

type jwks struct {
	Keys []jwksKey `json:"keys"`
}

type jwksKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (v *JWTVerifier) fetchJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("building jwks request: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("requesting jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks endpoint returned status %d", resp.StatusCode)
	}

	var parsed jwks
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("decoding jwks response: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(parsed.Keys))
	for _, k := range parsed.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()
	return nil
}

func rsaPublicKeyFromJWK(k jwksKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decoding n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decoding e: %w", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}
