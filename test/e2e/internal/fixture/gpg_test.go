//go:build e2e

// gpg_test.go pins the GPG key generation, in two layers.
//
// The first reads the block back the way the code wrote it: the armor, the
// checksum, and the three packets in the order gpg expects them. The second
// hands the block to the real gpg, which is the only oracle that matters here
// because GitLab runs the key through GPGME, and GnuPG refuses a key whose
// self-signature does not verify or whose user id is not certified. The
// packets are written by hand out of the standard library, so a layout error
// would otherwise surface as a GitLab refusal in an end-to-end run against a
// booted instance, which costs minutes to reach and says little about why.

package fixture

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGPGPublicKey_Generated_CarriesTheThreePacketsGPGExpects checks the
// armor, the checksum line and the packet sequence, and that two calls give
// two different keys, since an instance refuses a fingerprint it already
// holds.
func TestGPGPublicKey_Generated_CarriesTheThreePacketsGPGExpects(t *testing.T) {
	first, err := GPGPublicKey("owner-one")
	if err != nil {
		t.Fatalf("GPGPublicKey() error = %v", err)
	}
	if !strings.HasPrefix(first, "-----BEGIN PGP PUBLIC KEY BLOCK-----\n\n") {
		t.Errorf("the block does not open with the public key armor header and a blank line: %q", firstLineOf(first))
	}
	if !strings.HasSuffix(first, "-----END PGP PUBLIC KEY BLOCK-----\n") {
		t.Errorf("the block does not close with the armor footer and a newline: %q", lastLineOf(first))
	}

	packets, checksum := decodeArmor(t, first)
	if !bytes.Equal(checksum, crc24(packets)) {
		t.Errorf("the checksum line carries %x, and the packets checksum to %x", checksum, crc24(packets))
	}

	tags := packetTags(t, packets)
	want := []byte{packetTagPublicKey, packetTagUserID, packetTagSignature}
	if !bytes.Equal(tags, want) {
		t.Errorf("the block holds packets %v, want the public key, the user id and the certification %v", tags, want)
	}

	second, err := GPGPublicKey("owner-two")
	if err != nil {
		t.Fatalf("second GPGPublicKey() error = %v", err)
	}
	if first == second {
		t.Error("two generated keys are identical")
	}
}

// TestGPGPublicKey_Generated_ImportsIntoGPGNamingItsOwner hands the block to
// gpg and checks it imports as one key whose user id names the owner. An
// import that reports no user id is a self-signature gpg did not accept, and
// is the failure this test exists to catch. It is skipped where gpg is not
// installed, since nothing else in the suite needs it.
func TestGPGPublicKey_Generated_ImportsIntoGPGNamingItsOwner(t *testing.T) {
	gpg, err := exec.LookPath("gpg")
	if err != nil {
		t.Skipf("gpg is not installed here, so the key cannot be held to the parser GitLab uses: %v", err)
	}

	block, err := GPGPublicKey("import-owner")
	if err != nil {
		t.Fatalf("GPGPublicKey() error = %v", err)
	}
	home := t.TempDir()
	path := filepath.Join(home, "key.asc")
	if writeErr := os.WriteFile(path, []byte(block), 0o600); writeErr != nil {
		t.Fatalf("writing the key out: %v", writeErr)
	}

	imported, importErr := exec.CommandContext(t.Context(), gpg, "--homedir", home, "--batch", "--import", path).CombinedOutput()
	if importErr != nil {
		t.Fatalf("gpg refused the generated key: %v\n%s", importErr, imported)
	}
	if !strings.Contains(string(imported), "imported: 1") {
		t.Errorf("gpg did not import one key:\n%s", imported)
	}

	// A user id gpg lists is one whose certification it accepted: an import
	// drops the ones it could not verify, and a key left with none is not
	// imported at all.
	listed, listErr := exec.CommandContext(t.Context(), gpg, "--homedir", home, "--batch", "--list-keys", "--with-colons").CombinedOutput()
	if listErr != nil {
		t.Fatalf("listing the imported key: %v\n%s", listErr, listed)
	}
	if !strings.Contains(string(listed), "import-owner"+gpgUserEmailDomain) {
		t.Errorf("the imported key carries no user id naming its owner:\n%s", listed)
	}

	// And said in gpg's own words: a checked signature whose status field is
	// "!" is one it verified against the key it is on.
	checked, checkErr := exec.CommandContext(t.Context(), gpg, "--homedir", home, "--batch", "--check-sigs", "--with-colons").CombinedOutput()
	if checkErr != nil {
		t.Fatalf("checking the signatures of the imported key: %v\n%s", checkErr, checked)
	}
	if !strings.Contains(string(checked), "sig:!:") {
		t.Errorf("gpg verified no signature on the imported key, so its self-certification did not hold:\n%s", checked)
	}
}

// decodeArmor splits an armored block into its packets and the three octets
// of its checksum line.
func decodeArmor(t *testing.T, block string) (packets, checksum []byte) {
	t.Helper()

	var body strings.Builder
	var checksumLine string
	for line := range strings.SplitSeq(block, "\n") {
		switch {
		case line == "" || strings.HasPrefix(line, "-----"):
		case strings.HasPrefix(line, "="):
			checksumLine = line[1:]
		default:
			body.WriteString(line)
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(body.String())
	if err != nil {
		t.Fatalf("the armored body is not base64: %v", err)
	}
	sum, err := base64.StdEncoding.DecodeString(checksumLine)
	if err != nil {
		t.Fatalf("the checksum line is not base64: %v", err)
	}
	return decoded, sum
}

// packetTags walks the packet stream and returns the tag of each packet, which
// is what says the block is laid out the way gpg reads it.
func packetTags(t *testing.T, packets []byte) []byte {
	t.Helper()

	var tags []byte
	for offset := 0; offset < len(packets); {
		header := packets[offset]
		if header&0xC0 != 0xC0 {
			t.Fatalf("the packet at offset %d is not in the new format: %#x", offset, header)
		}
		tags = append(tags, header&0x3F)
		offset++

		length, read := packetLength(t, packets, offset)
		offset += read + length
		if offset > len(packets) {
			t.Fatalf("the packet at offset %d claims %d octets, and the stream holds %d", offset, length, len(packets))
		}
	}
	return tags
}

// packetLength reads a new-format body length, returning it and how many
// octets it took.
func packetLength(t *testing.T, packets []byte, offset int) (length, read int) {
	t.Helper()

	if offset >= len(packets) {
		t.Fatalf("the stream ends where a length was expected, at offset %d", offset)
	}
	switch first := int(packets[offset]); {
	case first < 192:
		return first, 1
	case first < 224:
		if offset+1 >= len(packets) {
			t.Fatalf("a two-octet length is cut short at offset %d", offset)
		}
		return (first-192)<<8 + int(packets[offset+1]) + 192, 2
	case first == 0xFF:
		if offset+4 >= len(packets) {
			t.Fatalf("a five-octet length is cut short at offset %d", offset)
		}
		return int(packets[offset+1])<<24 | int(packets[offset+2])<<16 | int(packets[offset+3])<<8 | int(packets[offset+4]), 5
	default:
		t.Fatalf("a partial body length at offset %d, which nothing here writes", offset)
		return 0, 0
	}
}

// firstLineOf returns the first line of a block, for a message.
func firstLineOf(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

// lastLineOf returns the last non-empty line of a block, for a message.
func lastLineOf(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	return lines[len(lines)-1]
}
