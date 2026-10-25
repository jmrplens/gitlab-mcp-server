// checksums_test.go covers the checksum file a release publishes and the
// published binaries read against it.
package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// digestOfABC is the SHA-256 of "abc", from FIPS 180-2's own example.
const digestOfABC = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

// TestDigest_IsSHA256InLowercaseHex covers the one digest this command
// computes, in the spelling GoReleaser writes into checksums.txt.
func TestDigest_IsSHA256InLowercaseHex(t *testing.T) {
	t.Parallel()

	if got := digest([]byte("abc")); got != digestOfABC {
		t.Errorf("digest(abc) = %s, want %s", got, digestOfABC)
	}
}

// writeChecksums writes text as a checksum file and returns its path.
func writeChecksums(t *testing.T, text string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "checksums.txt")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestReadChecksums_ReadsGoReleasersFile covers the file as GoReleaser
// writes it: a digest, two spaces and the asset name, one per line, ending in
// a newline.
func TestReadChecksums_ReadsGoReleasersFile(t *testing.T) {
	t.Parallel()

	other := strings.Repeat("0", 64)
	got, err := readChecksums(writeChecksums(t, digestOfABC+"  fixture-linux-amd64\n"+other+"  fixture windows.exe\n"))
	if err != nil {
		t.Fatalf("readChecksums: %v", err)
	}
	want := checksums{name: "checksums.txt", digests: map[string]string{"fixture-linux-amd64": digestOfABC, "fixture windows.exe": other}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("readChecksums = %v, want %v", got, want)
	}
}

// TestReadChecksums_RefusesALineItCannotRead covers every line that is not a
// digest and a name, and a name listed twice, either of which would leave the
// comparison with nothing certain to compare against. The line is named.
func TestReadChecksums_RefusesALineItCannotRead(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, text, want string
	}{
		{name: "an empty file", text: "", want: "checksums.txt:1"},
		{name: "no separator", text: digestOfABC + " name\n", want: "checksums.txt:1"},
		{name: "a short digest", text: digestOfABC[1:] + "  name\n", want: "checksums.txt:1"},
		{name: "a digest that is not lowercase hex", text: strings.ToUpper(digestOfABC) + "  name\n", want: "checksums.txt:1"},
		{name: "no name", text: digestOfABC + "  \n", want: "checksums.txt:1"},
		{name: "a name listed twice", text: digestOfABC + "  a\n" + digestOfABC + "  a\n", want: "checksums.txt:2: a is listed twice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := readChecksums(writeChecksums(t, tc.text)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("readChecksums = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// TestReadChecksums_AFileThatCannotBeRead covers a published directory that
// holds no checksum file.
func TestReadChecksums_AFileThatCannotBeRead(t *testing.T) {
	t.Parallel()

	if _, err := readChecksums(filepath.Join(t.TempDir(), "checksums.txt")); !os.IsNotExist(err) {
		t.Errorf("readChecksums = %v, want a not-exist error", err)
	}
}

// TestReadPublished_HoldsTheFileToItsListedDigest covers a published asset:
// read only when the checksum file lists it, and only when it is the file the
// list says, since what the comparison measures is the published release and
// not whatever sits in the directory.
func TestReadPublished_HoldsTheFileToItsListedDigest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"good": "abc", "tampered": "abd"})
	sums := checksums{name: "checksums.txt", digests: map[string]string{"good": digestOfABC, "tampered": digestOfABC, "missing": digestOfABC}}

	data, err := readPublished(sums, dir, "good")
	if err != nil || string(data) != "abc" {
		t.Errorf("readPublished(good) = %q, %v", data, err)
	}
	for _, tc := range []struct {
		asset, want string
	}{
		{asset: "unlisted", want: "checksums.txt lists no unlisted"},
		{asset: "missing", want: "missing"},
		{asset: "tampered", want: "the published tampered is not the file checksums.txt lists"},
	} {
		t.Run(tc.asset, func(t *testing.T) {
			t.Parallel()

			if _, refused := readPublished(sums, dir, tc.asset); refused == nil || !strings.Contains(refused.Error(), tc.want) {
				t.Errorf("readPublished(%s) = %v, want an error containing %q", tc.asset, refused, tc.want)
			}
		})
	}
}
