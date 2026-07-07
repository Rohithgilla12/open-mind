// Package auth provides API key, device-code, and JWT primitives shared by
// the auth handlers.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
)

const codeAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"

// GenerateKey creates a new API key. full is the secret to hand to the user
// exactly once; hash is what gets persisted; prefix is a non-secret display
// value safe to show in a key list.
func GenerateKey() (full string, hash []byte, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, "", fmt.Errorf("generating key entropy: %w", err)
	}
	full = "omk_" + base64.RawURLEncoding.EncodeToString(buf)
	prefix = full[4:12]
	return full, HashKey(full), prefix, nil
}

// HashKey returns the sha256 digest of a full API key, as stored in the
// database.
func HashKey(full string) []byte {
	sum := sha256.Sum256([]byte(full))
	return sum[:]
}

// GenerateCode creates a new 8-character device-link code in Crockford
// base32 (minus the ambiguous 0/1/O/I characters), along with the sha256
// hash of its undashed, uppercase form. The returned code is formatted for
// display as "ABCD-EFGH".
func GenerateCode() (code string, hash []byte, err error) {
	raw := make([]byte, 8)
	for i := range raw {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", nil, fmt.Errorf("generating code entropy: %w", err)
		}
		raw[i] = codeAlphabet[n.Int64()]
	}
	undashed := string(raw)
	code = undashed[:4] + "-" + undashed[4:]
	return code, HashCode(undashed), nil
}

// HashCode hashes a normalized (undashed, uppercase) device-link code. Mint
// and claim must share this one implementation: a drift (case, dashes) would
// not crash — every claim would just silently miss.
func HashCode(normalized string) []byte {
	sum := sha256.Sum256([]byte(normalized))
	return sum[:]
}

// NormalizeCode strips dashes and whitespace from a user-entered device-link
// code and uppercases it, so it can be hashed and compared against the
// stored hash.
func NormalizeCode(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}
