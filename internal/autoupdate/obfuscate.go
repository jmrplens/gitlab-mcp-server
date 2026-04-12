// obfuscate.go provides XOR-based token obfuscation to prevent trivial
// extraction of embedded credentials from compiled binaries via tools
// like `strings` or `hexdump`.

package autoupdate

import (
	"encoding/hex"
	"fmt"
)

// DeobfuscateHex decodes two hex-encoded strings (ciphertext and key),
// XORs them byte-by-byte, and returns the original plaintext token.
// Returns an empty string if either input is empty or decoding fails.
//
// Intermediate byte slices are zeroed after use to reduce the window during
// which sensitive material is present in memory. The returned Go string is
// immutable and will be garbage-collected normally.
func DeobfuscateHex(cipherHex, keyHex string) string {
	if cipherHex == "" || keyHex == "" {
		return ""
	}

	cipher, err := hex.DecodeString(cipherHex)
	if err != nil {
		return ""
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		zeroBytes(cipher)
		return ""
	}

	if len(cipher) != len(key) {
		zeroBytes(cipher)
		zeroBytes(key)
		return ""
	}

	plain := make([]byte, len(cipher))
	for i := range cipher {
		plain[i] = cipher[i] ^ key[i]
	}

	// Zero intermediate buffers before they become unreachable.
	zeroBytes(cipher)
	zeroBytes(key)

	result := string(plain)
	zeroBytes(plain)
	return result
}

// ObfuscateWithKey XOR-encrypts plaintext with the given key and returns
// hex-encoded ciphertext. The key must be at least as long as plaintext.
func ObfuscateWithKey(plaintext string, key []byte) (string, error) {
	if len(key) < len(plaintext) {
		return "", fmt.Errorf("key length %d < plaintext length %d", len(key), len(plaintext))
	}

	cipher := make([]byte, len(plaintext))
	for i := range plaintext {
		cipher[i] = plaintext[i] ^ key[i]
	}

	result := hex.EncodeToString(cipher)
	zeroBytes(cipher)
	return result, nil
}

// zeroBytes overwrites a byte slice with zeros to reduce the window during
// which sensitive material is present in process memory.
func zeroBytes(b []byte) {
	clear(b)
}
