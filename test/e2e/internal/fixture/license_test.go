//go:build e2e

// license_test.go covers the license cache reader's pure halves: where the
// file is looked for, and what an absent or empty one is reported as.

package fixture

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestLicenseCachePath_Configured_ResolvesAgainstTheRoot checks the three
// spellings a cache path can have: none, which is the script's default under
// the root; a relative one, which the scripts also read against the root;
// and an absolute one, which is taken as it is.
func TestLicenseCachePath_Configured_ResolvesAgainstTheRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo")
	absolute := filepath.Join(string(filepath.Separator), "elsewhere", "key")
	cases := []struct {
		name       string
		configured string
		want       string
	}{
		{name: "default", configured: "", want: filepath.Join(root, "test", "e2e", ".enterprise-license")},
		{name: "relative", configured: filepath.Join("var", "key"), want: filepath.Join(root, "var", "key")},
		{name: "absolute", configured: absolute, want: absolute},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := licenseCachePath(testCase.configured, root); got != testCase.want {
				t.Errorf("licenseCachePath(%q) = %q, want %q", testCase.configured, got, testCase.want)
			}
		})
	}
}

// TestReadCachedLicense_Files_AreReadTrimmedOrRefused checks that a cached
// key comes back without the newline the script may have left, and that a
// missing or empty file is the same named refusal, since both mean there is
// nothing to install.
func TestReadCachedLicense_Files_AreReadTrimmedOrRefused(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present")
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(present, []byte("  dGVzdA==\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		path    string
		want    string
		refused bool
		broken  bool
	}{
		{name: "present", path: present, want: "dGVzdA=="},
		{name: "empty", path: empty, refused: true},
		{name: "missing", path: filepath.Join(dir, "missing"), refused: true},
		// A directory where the file should be opens and cannot be read: a
		// broken cache, which is reported as itself and never as absent, so
		// that the caller fails rather than skips.
		{name: "directory", path: dir, broken: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := readCachedLicense(testCase.path)
			switch {
			case testCase.refused:
				if !errors.Is(err, errNoLicenseCached) {
					t.Errorf("readCachedLicense(%s) error = %v, want %v", testCase.name, err, errNoLicenseCached)
				}
			case testCase.broken:
				if err == nil || errors.Is(err, errNoLicenseCached) {
					t.Errorf("readCachedLicense(%s) error = %v, want a read error that is not %v", testCase.name, err, errNoLicenseCached)
				}
			default:
				if err != nil || got != testCase.want {
					t.Errorf("readCachedLicense(%s) = (%q, %v), want (%q, nil)", testCase.name, got, err, testCase.want)
				}
			}
		})
	}
}
