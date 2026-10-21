package goprogram

import (
	"path/filepath"
	"testing"
)

// TestTrimModulePath_NamesAPackageAsTheRepositoryDoes verifies a package of
// this module loses the module path and the slash after it, the module's root
// package is named by nothing, and a package of another module keeps its path
// whole, a module whose path only begins with this one's included.
func TestTrimModulePath_NamesAPackageAsTheRepositoryDoes(t *testing.T) {
	for _, tt := range []struct{ name, path, want string }{
		{"a package of this module", ModulePath + "/internal/oauth", "internal/oauth"},
		{"the module's root package", ModulePath, ""},
		{"a package of another module", "example.com/p", "example.com/p"},
		{"a module whose path begins with this one's", ModulePath + "x/p", ModulePath + "x/p"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := TrimModulePath(tt.path); got != tt.want {
				t.Errorf("TrimModulePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestRelativePath_NamesAFileBelowTheRootAndNoOtherRelatively verifies a file
// below the root is named relative to it, in slashes, a directory whose name
// begins with two dots included, and the root itself as "."; and that a file
// outside it, the directory above it, or one that cannot be made relative to
// it because one of the two paths is relative and the other is not, is kept as
// it was given.
func TestRelativePath_NamesAFileBelowTheRootAndNoOtherRelatively(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	outside := filepath.Join(filepath.Dir(root), "elsewhere", "file.go")
	below := filepath.Join(root, "internal", "x.go")
	for _, tt := range []struct{ name, path, root, want string }{
		{"a file below the root", below, root, "internal/x.go"},
		{"a file below a directory whose name begins with two dots", filepath.Join(root, "..cache", "a.go"), root, "..cache/a.go"},
		{"the root itself", root, root, "."},
		{"a file outside the root", outside, root, filepath.ToSlash(outside)},
		{"the directory above the root", filepath.Dir(root), root, filepath.ToSlash(filepath.Dir(root))},
		{"a relative file against an absolute root", filepath.Join("already", "relative.go"), root, "already/relative.go"},
		{"an absolute file against no root", below, "", filepath.ToSlash(below)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := RelativePath(tt.path, tt.root); got != tt.want {
				t.Errorf("RelativePath(%q, %q) = %q, want %q", tt.path, tt.root, got, tt.want)
			}
		})
	}
}
