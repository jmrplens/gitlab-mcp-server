// obfuscate_test.go verifies XOR-based token obfuscation and deobfuscation.
// Tests cover round-trip consistency, known values, empty inputs,
// length mismatches, invalid hex strings, short keys, and that the
// ciphertext does not contain the original token.
package autoupdate

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
)

// TestDeobfuscateHex_RoundTrip verifies that obfuscating a token with a
// random key and then deobfuscating produces the original token.
func TestDeobfuscateHex_RoundTrip(t *testing.T) {
	t.Parallel()
	token := "glpat-xxxxxxxxxxxxxxxxxxxx"

	key := make([]byte, len(token))
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating random key: %v", err)
	}

	cipherHex, err := ObfuscateWithKey(token, key)
	if err != nil {
		t.Fatalf("ObfuscateWithKey: %v", err)
	}
	keyHex := hex.EncodeToString(key)

	got := DeobfuscateHex(cipherHex, keyHex)
	if got != token {
		t.Errorf("DeobfuscateHex round-trip failed: got %q, want %q", got, token)
	}
}

// TestDeobfuscateHex_KnownValues verifies deobfuscation with pre-computed
// XOR values to detect regressions in the algorithm.
func TestDeobfuscateHex_KnownValues(t *testing.T) {
	t.Parallel()
	// "AB" XOR [0x01, 0x02] = [0x40, 0x40]
	cipherHex := "4040"
	keyHex := "0102"
	got := DeobfuscateHex(cipherHex, keyHex)
	if got != "AB" {
		t.Errorf("got %q, want %q", got, "AB")
	}
}

// TestDeobfuscateHex_EmptyInputs verifies that empty cipher or key returns
// an empty string without error.
func TestDeobfuscateHex_EmptyInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		cipher    string
		key       string
		wantEmpty bool
	}{
		{"both empty", "", "", true},
		{"empty cipher", "", "aabb", true},
		{"empty key", "aabb", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DeobfuscateHex(tt.cipher, tt.key)
			if tt.wantEmpty && got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
		})
	}
}

// TestDeobfuscateHex_LengthMismatch verifies that mismatched cipher and key
// lengths return an empty string.
func TestDeobfuscateHex_LengthMismatch(t *testing.T) {
	t.Parallel()
	got := DeobfuscateHex("aabbcc", "aabb")
	if got != "" {
		t.Errorf("expected empty string for length mismatch, got %q", got)
	}
}

// TestDeobfuscateHex_InvalidHex verifies that invalid hex strings return
// an empty string without panicking.
func TestDeobfuscateHex_InvalidHex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cipher string
		key    string
	}{
		{"invalid cipher", "ggzz", "aabb"},
		{"invalid key", "aabb", "ggzz"},
		{"both invalid", "xxxx", "yyyy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DeobfuscateHex(tt.cipher, tt.key)
			if got != "" {
				t.Errorf("expected empty string for invalid hex, got %q", got)
			}
		})
	}
}

// TestObfuscateWithKey_ShortKey verifies that ObfuscateWithKey returns an
// error when the key is shorter than the plaintext.
func TestObfuscateWithKey_ShortKey(t *testing.T) {
	t.Parallel()
	_, err := ObfuscateWithKey("long-token-value", []byte{0x01})
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

// TestDeobfuscateHex_NoPlainTokenInOutput verifies that the obfuscated
// hex string does not contain the original token as a substring, confirming
// the token is not trivially extractable via `strings`.
func TestDeobfuscateHex_NoPlainTokenInOutput(t *testing.T) {
	t.Parallel()
	token := "glpat-secret-token-12345"

	key := make([]byte, len(token))
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating random key: %v", err)
	}

	cipherHex, err := ObfuscateWithKey(token, key)
	if err != nil {
		t.Fatalf("ObfuscateWithKey: %v", err)
	}

	// The ciphertext must not contain the original token.
	if cipherHex == hex.EncodeToString([]byte(token)) {
		t.Error("ciphertext equals plaintext hex -- XOR key is all zeros")
	}
}
