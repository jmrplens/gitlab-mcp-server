// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package releaseassets

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// digestOfABC is the SHA-256 of "abc", from FIPS 180-2's own example.
const digestOfABC = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

// TestDigest_IsSHA256InLowercaseHex covers the digest every asset is listed
// with, in the spelling GoReleaser writes into checksums.txt.
func TestDigest_IsSHA256InLowercaseHex(t *testing.T) {
	t.Parallel()

	if got := Digest([]byte("abc")); got != digestOfABC {
		t.Errorf("Digest(abc) = %s, want %s", got, digestOfABC)
	}
}

// TestIsDigest_TakesOnlyTheSpellingGoReleaserWrites covers a SHA-256 in
// lowercase hex and every string that is not one: a character short or over,
// uppercase, a character outside hex, and nothing at all.
func TestIsDigest_TakesOnlyTheSpellingGoReleaserWrites(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		s    string
		want bool
	}{
		{name: "a lowercase SHA-256", s: digestOfABC, want: true},
		{name: "every hex digit", s: strings.Repeat("0123456789abcdef", 4), want: true},
		{name: "a character short", s: digestOfABC[1:], want: false},
		{name: "a character over", s: digestOfABC + "0", want: false},
		{name: "uppercase", s: strings.ToUpper(digestOfABC), want: false},
		{name: "a character outside hex", s: "g" + digestOfABC[1:], want: false},
		{name: "empty", s: "", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := IsDigest(tc.s); got != tc.want {
				t.Errorf("IsDigest(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

// TestParseChecksums_ReadsGoReleasersFile covers the file as GoReleaser
// writes it: a digest, two spaces and the asset name, one per line, ending in
// a newline, and the same lines without the last newline. A name may hold a
// space, and the file's name is its base name, which the errors of a reader
// of it name.
func TestParseChecksums_ReadsGoReleasersFile(t *testing.T) {
	t.Parallel()

	other := strings.Repeat("0", 64)
	lines := digestOfABC + "  fixture-linux-amd64\n" + other + "  fixture windows.exe"
	want := Checksums{Name: "checksums.txt", Digests: map[string]string{"fixture-linux-amd64": digestOfABC, "fixture windows.exe": other}}
	for _, tc := range []struct {
		name, text string
	}{
		{name: "ending in a newline", text: lines + "\n"},
		{name: "without the last newline", text: lines},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseChecksums(filepath.Join("dist", "checksums.txt"), []byte(tc.text))
			if err != nil {
				t.Fatalf("ParseChecksums: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("ParseChecksums = %v, want %v", got, want)
			}
		})
	}
}

// TestParseChecksums_RefusesALineItCannotRead covers every line that is not a
// digest and a name, and a name listed twice, either of which would leave a
// comparison with nothing certain to compare against. The error names the
// file as it was given and the line by its number.
func TestParseChecksums_RefusesALineItCannotRead(t *testing.T) {
	t.Parallel()

	path := filepath.Join("dist", "checksums.txt")
	notALine := func(line int, text string) string {
		return fmt.Sprintf("%s:%d: %q is not a SHA-256 and a file name separated by two spaces", path, line, text)
	}
	for _, tc := range []struct {
		name, text, want string
	}{
		{name: "an empty file", text: "", want: notALine(1, "")},
		{name: "no separator", text: digestOfABC + "name\n", want: notALine(1, digestOfABC+"name")},
		{name: "one space", text: digestOfABC + " name\n", want: notALine(1, digestOfABC+" name")},
		{name: "a short digest", text: digestOfABC[1:] + "  name\n", want: notALine(1, digestOfABC[1:]+"  name")},
		{name: "a digest that is not lowercase hex", text: strings.ToUpper(digestOfABC) + "  name\n", want: notALine(1, strings.ToUpper(digestOfABC)+"  name")},
		{name: "no name", text: digestOfABC + "  \n", want: notALine(1, digestOfABC+"  ")},
		{name: "a blank line after the first", text: digestOfABC + "  a\n\n", want: notALine(2, "")},
		{name: "a name listed twice", text: digestOfABC + "  a\n" + digestOfABC + "  a\n", want: path + ":2: a is listed twice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseChecksums(path, []byte(tc.text))
			if err == nil || err.Error() != tc.want {
				t.Errorf("ParseChecksums = %v, %v, want the error %q", got, err, tc.want)
			}
			if !reflect.DeepEqual(got, Checksums{}) {
				t.Errorf("ParseChecksums refused the file and returned %v, want nothing", got)
			}
		})
	}
}

// TestReadChecksums_ReadsTheFile covers the file read from disk: the lines
// it holds, and its base name.
func TestReadChecksums_ReadsTheFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fixture_checksums.txt")
	if err := os.WriteFile(path, []byte(digestOfABC+"  fixture-linux-amd64\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadChecksums(path)
	if err != nil {
		t.Fatalf("ReadChecksums: %v", err)
	}
	want := Checksums{Name: "fixture_checksums.txt", Digests: map[string]string{"fixture-linux-amd64": digestOfABC}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadChecksums = %v, want %v", got, want)
	}
}

// TestReadChecksums_RefusesWhatItCannotRead covers a directory that holds no
// checksum file, whose error is the one reading it returned, and a file that
// holds a line it cannot read, whose error names the path it was given.
func TestReadChecksums_RefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := ReadChecksums(filepath.Join(dir, "checksums.txt")); !os.IsNotExist(err) {
		t.Errorf("ReadChecksums of a missing file = %v, want a not-exist error", err)
	}
	path := filepath.Join(dir, "broken.txt")
	if err := os.WriteFile(path, []byte("not a line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadChecksums(path); err == nil || !strings.HasPrefix(err.Error(), path+":1: ") {
		t.Errorf("ReadChecksums of a broken file = %v, want an error naming %s:1", err, path)
	}
}
