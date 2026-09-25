//go:build unix

// download_output_unix_test.go covers the directory sync a download's rename
// is followed by on the platforms that can perform one.

package toolutil

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

// TestSyncDirectory_ExistingOrMissing_SyncsOrReportsNotExist verifies that a
// directory that exists is synced without error and that one that does not
// is reported as missing. The missing case is the one that tells an open
// whose error is checked from one whose error is not: carrying on with no
// directory would fail too, but with an invalid-file error rather than this
// one.
func TestSyncDirectory_ExistingOrMissing_SyncsOrReportsNotExist(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name        string
		dir         string
		wantMissing bool
	}{
		{name: "an existing directory is synced", dir: dir},
		{name: "a missing directory is reported as missing", dir: filepath.Join(dir, "gone"), wantMissing: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := syncDirectory(tt.dir)
			if tt.wantMissing {
				if !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("syncDirectory(%q) error = %v, want one wrapping fs.ErrNotExist", tt.dir, err)
				}
				return
			}
			if err != nil {
				t.Errorf("syncDirectory(%q) error = %v, want nil", tt.dir, err)
			}
		})
	}
}
