// Package shared tests the memoized typed-package loader the structs and
// actions analyzers share, against small throwaway modules so the tests
// state exactly which packages resolve and which errors abort a run.
package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeModule creates a Go module under a temp dir with the given files
// (paths relative to the module root) and returns the root.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["go.mod"] = "module example.com/fixture\n\ngo 1.27\n"
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

// TestLoadToolPackages_Module_ReturnsTypedToolPackagesOnce verifies the
// loader resolves every package under internal/tools with type information,
// leaves out both a package outside that tree and the root tools package
// itself (whose path lacks the /internal/tools/ infix, as the real
// internal/tools orchestration package does), and memoizes the result per
// root so a second call hands back the same slice instead of loading again.
func TestLoadToolPackages_Module_ReturnsTypedToolPackagesOnce(t *testing.T) {
	root := writeModule(t, map[string]string{
		"internal/tools/tools.go":       "package tools\n\n// Register is the root orchestration package, outside the audited set.\nfunc Register() {}\n",
		"internal/tools/alpha/alpha.go": "package alpha\n\n// A is a typed symbol.\nfunc A() int { return 1 }\n",
		"internal/tools/beta/beta.go":   "package beta\n\nimport \"strings\"\n\n// B uses an import so the loader resolves dependencies.\nfunc B() string { return strings.ToUpper(\"b\") }\n",
		"internal/other/other.go":       "package other\n\nfunc O() {}\n",
	})

	first, err := LoadToolPackages(root)
	if err != nil {
		t.Fatalf("LoadToolPackages: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("loaded %d packages, want the 2 sub-packages under internal/tools", len(first))
	}
	seen := map[string]bool{}
	for _, pkg := range first {
		seen[pkg.PkgPath] = true
		if !strings.Contains(pkg.PkgPath, ToolsPkgInfix) {
			t.Errorf("%s is outside %s", pkg.PkgPath, ToolsPkgInfix)
		}
		if pkg.Types == nil || pkg.TypesInfo == nil || len(pkg.Syntax) == 0 {
			t.Errorf("%s loaded without types or syntax", pkg.PkgPath)
		}
	}
	for _, want := range []string{"example.com/fixture/internal/tools/alpha", "example.com/fixture/internal/tools/beta"} {
		t.Run(want, func(t *testing.T) {
			if !seen[want] {
				t.Errorf("%s not loaded (got %v)", want, seen)
			}
		})
	}

	second, err := LoadToolPackages(root)
	if err != nil {
		t.Fatalf("second LoadToolPackages: %v", err)
	}
	if len(second) != len(first) || &second[0] != &first[0] {
		t.Error("second call did not return the memoized slice")
	}
}

// TestLoadToolPackages_Failures_AbortTheRun verifies both failure classes
// surface as errors rather than a partial package set: a root the go tool
// cannot enter, and a tool package that does not type-check.
func TestLoadToolPackages_Failures_AbortTheRun(t *testing.T) {
	cases := []struct {
		name    string
		root    func(t *testing.T) string
		wantErr string
	}{
		{
			name: "missing_root",
			root: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "absent")
			},
			wantErr: "load packages: ",
		},
		{
			name: "type_error_in_a_tool_package",
			root: func(t *testing.T) string {
				t.Helper()
				return writeModule(t, map[string]string{
					"internal/tools/broken/broken.go": "package broken\n\nfunc B() int { return \"not an int\" }\n",
				})
			},
			wantErr: "package load errors:",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := tc.root(t)
			pkgs, err := LoadToolPackages(root)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("LoadToolPackages error = %v, want it to contain %q", err, tc.wantErr)
			}
			if pkgs != nil {
				t.Errorf("packages = %d on failure, want none", len(pkgs))
			}
			if _, again := LoadToolPackages(root); again == nil || again.Error() != err.Error() {
				t.Errorf("memoized failure = %v, want the same error again", again)
			}
		})
	}
}
