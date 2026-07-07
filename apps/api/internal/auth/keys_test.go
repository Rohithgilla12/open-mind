package auth_test

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/auth"
)

func TestGenerateKeyShape(t *testing.T) {
	full, hash, prefix, err := auth.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if !strings.HasPrefix(full, "omk_") {
		t.Errorf("full = %q, want prefix omk_", full)
	}
	if len(full) != 47 {
		t.Errorf("len(full) = %d, want 47", len(full))
	}
	for _, r := range full {
		if r == '+' || r == '/' || r == '=' {
			t.Errorf("full contains non-url-safe char %q", r)
		}
	}
	wantHash := sha256.Sum256([]byte(full))
	if string(hash) != string(wantHash[:]) {
		t.Errorf("hash does not match sha256(full)")
	}
	wantPrefix := full[4:12]
	if prefix != wantPrefix {
		t.Errorf("prefix = %q, want %q", prefix, wantPrefix)
	}
}

func TestGenerateKeyUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		full, _, _, err := auth.GenerateKey()
		if err != nil {
			t.Fatalf("GenerateKey: %v", err)
		}
		if seen[full] {
			t.Fatalf("duplicate key generated: %q", full)
		}
		seen[full] = true
	}
}

func TestHashKeyDeterministic(t *testing.T) {
	full := "omk_someFixedValueForTesting123"
	h1 := auth.HashKey(full)
	h2 := auth.HashKey(full)
	if string(h1) != string(h2) {
		t.Errorf("HashKey not deterministic: %x != %x", h1, h2)
	}
	want := sha256.Sum256([]byte(full))
	if string(h1) != string(want[:]) {
		t.Errorf("HashKey mismatch with sha256")
	}
}

func TestGenerateCodeFormat(t *testing.T) {
	const alphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"
	for i := 0; i < 50; i++ {
		code, hash, err := auth.GenerateCode()
		if err != nil {
			t.Fatalf("GenerateCode: %v", err)
		}
		if len(code) != 9 { // 8 chars + 1 dash
			t.Fatalf("len(code) = %d, want 9 (ABCD-EFGH), code=%q", len(code), code)
		}
		if code[4] != '-' {
			t.Fatalf("code[4] = %q, want '-': %q", code[4], code)
		}
		undashed := strings.ReplaceAll(code, "-", "")
		if len(undashed) != 8 {
			t.Fatalf("undashed len = %d, want 8", len(undashed))
		}
		for _, r := range undashed {
			if !strings.ContainsRune(alphabet, r) {
				t.Fatalf("code %q contains disallowed rune %q", code, r)
			}
			if r == '0' || r == '1' || r == 'O' || r == 'I' {
				t.Fatalf("code %q contains excluded rune %q", code, r)
			}
		}
		wantHash := sha256.Sum256([]byte(strings.ToUpper(undashed)))
		if string(hash) != string(wantHash[:]) {
			t.Fatalf("hash does not match sha256(undashed uppercase)")
		}
	}
}

func TestNormalizeCode(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"abcd-efgh", "ABCDEFGH"},
		{"ABCD-EFGH", "ABCDEFGH"},
		{"abcd efgh", "ABCDEFGH"},
		{"  abcd-efgh  ", "ABCDEFGH"},
		{"abcdefgh", "ABCDEFGH"},
	}
	for _, tt := range tests {
		got := auth.NormalizeCode(tt.in)
		if got != tt.want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
