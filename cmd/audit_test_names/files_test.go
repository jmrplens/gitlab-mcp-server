// files_test.go covers the -check-files mode: the test-file naming
// convention and each of its codified exemptions, exercised against
// directory trees written into t.TempDir().
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fileSpec is one file to write into a fixture directory.
type fileSpec struct {
	name    string
	content string
}

// TestClassifyTestFileName_ConventionAndExemptions verifies every shape the
// convention decides: a plain module match, export_test.go, the qualified
// forms earned by a build constraint or by an external package with an
// internal sibling, and the shapes that only look like exemptions.
func TestClassifyTestFileName_ConventionAndExemptions(t *testing.T) {
	const (
		internalPkg = "package kind\n"
		externalPkg = "package kind_test\n"
		constrained = "//go:build unix\n\npackage kind\n"
	)
	qualifiedReason := func(module string) string {
		return fmt.Sprintf("qualified name over %s.go needs a //go:build constraint, or an external test package alongside an internal %s_test.go", module, module)
	}

	testCases := []struct {
		name       string
		dirs       []string
		files      []fileSpec
		file       string
		wantOK     bool
		wantReason string
	}{
		{
			name:   "module file exists",
			files:  []fileSpec{{"files.go", internalPkg}},
			file:   "files_test.go",
			wantOK: true,
		},
		{
			name:   "export_test.go is the standard idiom",
			file:   "export_test.go",
			wantOK: true,
		},
		{
			name:       "no module matches",
			files:      []fileSpec{{"kind.go", internalPkg}},
			file:       "theme_test.go",
			wantReason: "no module file matches this name",
		},
		{
			name:       "a directory named like a module does not count",
			dirs:       []string{"kind.go"},
			file:       "kind_test.go",
			wantReason: "no module file matches this name",
		},
		{
			name:   "qualified name with a build constraint",
			files:  []fileSpec{{"kind.go", internalPkg}, {"kind_unix_test.go", constrained}},
			file:   "kind_unix_test.go",
			wantOK: true,
		},
		{
			name:       "a constraint after the package clause is not a constraint",
			files:      []fileSpec{{"kind.go", internalPkg}, {"kind_unix_test.go", "package kind\n\n//go:build unix\n"}},
			file:       "kind_unix_test.go",
			wantReason: qualifiedReason("kind"),
		},
		{
			name:   "external package with an internal sibling",
			files:  []fileSpec{{"kind.go", internalPkg}, {"kind_test.go", internalPkg}, {"kind_integration_test.go", externalPkg}},
			file:   "kind_integration_test.go",
			wantOK: true,
		},
		{
			name:       "external package without an internal sibling",
			files:      []fileSpec{{"kind.go", internalPkg}, {"kind_integration_test.go", externalPkg}},
			file:       "kind_integration_test.go",
			wantReason: qualifiedReason("kind"),
		},
		{
			name:       "external package whose sibling is external too",
			files:      []fileSpec{{"kind.go", internalPkg}, {"kind_test.go", externalPkg}, {"kind_integration_test.go", externalPkg}},
			file:       "kind_integration_test.go",
			wantReason: qualifiedReason("kind"),
		},
		{
			name:       "internal package qualifier without a constraint",
			files:      []fileSpec{{"kind.go", internalPkg}, {"kind_test.go", internalPkg}, {"kind_extra_test.go", internalPkg}},
			file:       "kind_extra_test.go",
			wantReason: qualifiedReason("kind"),
		},
		{
			name:       "an unparseable qualified file is not an external package",
			files:      []fileSpec{{"kind.go", internalPkg}, {"kind_test.go", internalPkg}, {"kind_broken_test.go", "package\n"}},
			file:       "kind_broken_test.go",
			wantReason: qualifiedReason("kind"),
		},
		{
			name:   "underscore module with a constrained qualifier",
			files:  []fileSpec{{"my_module.go", "package mine\n"}, {"my_module_unix_test.go", "//go:build unix\n\npackage mine\n"}},
			file:   "my_module_unix_test.go",
			wantOK: true,
		},
		{
			name:       "the longest module prefix names the module",
			files:      []fileSpec{{"kind.go", internalPkg}, {"kind_extra.go", internalPkg}, {"kind_extra_unix_test.go", internalPkg}},
			file:       "kind_extra_unix_test.go",
			wantReason: qualifiedReason("kind_extra"),
		},
		{
			name:       "a base ending in _test is never a module",
			files:      []fileSpec{{"foo.go", "package foo\n"}, {"foo_test_test.go", "package foo\n"}},
			file:       "foo_test_test.go",
			wantReason: qualifiedReason("foo"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFixtureDir(t, dir, tc.dirs, tc.files)

			reason, ok := classifyTestFileName(dir, tc.file)
			if ok != tc.wantOK {
				t.Errorf("classifyTestFileName(%q) ok = %t, want %t (reason %q)", tc.file, ok, tc.wantOK, reason)
			}
			if reason != tc.wantReason {
				t.Errorf("classifyTestFileName(%q) reason = %q, want %q", tc.file, reason, tc.wantReason)
			}
		})
	}
}

// TestIsE2ETree_PathShapes verifies the exemption matches the test/e2e tree
// whether the path is relative, absolute, the tree itself or a descendant,
// and nothing that merely resembles it.
func TestIsE2ETree_PathShapes(t *testing.T) {
	testCases := []struct {
		path string
		want bool
	}{
		{path: "test/e2e", want: true},
		{path: "test/e2e/suite", want: true},
		{path: "/repo/test/e2e", want: true},
		{path: "/repo/test/e2e/http", want: true},
		{path: "test/e2ex", want: false},
		{path: "internal/e2e", want: false},
		{path: "cmd", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			if got := isE2ETree(tc.path); got != tc.want {
				t.Errorf("isE2ETree(%q) = %t, want %t", tc.path, got, tc.want)
			}
		})
	}
}

// TestCheckFileNamesInDir_WalksTreeAndExemptsE2E verifies the walk descends
// into subdirectories, ignores non-test files, reports slash-separated paths
// in directory order, and skips the test/e2e tree entirely.
func TestCheckFileNamesInDir_WalksTreeAndExemptsE2E(t *testing.T) {
	root := t.TempDir()
	writeFixtureDir(t, root, []string{"pkg/nested", "test/e2e/suite"}, []fileSpec{
		{"pkg/kind.go", "package kind\n"},
		{"pkg/kind_test.go", "package kind\n"},
		{"pkg/theme_test.go", "package kind\n"},
		{"pkg/nested/other_test.go", "package nested\n"},
		{"pkg/nested/readme.txt", ""},
		{"test/e2e/suite/anything_test.go", "package suite\n"},
	})

	got := checkFileNamesInDir(root)
	want := []fileViolation{
		{path: filepath.ToSlash(filepath.Join(root, "pkg", "nested", "other_test.go")), reason: "no module file matches this name"},
		{path: filepath.ToSlash(filepath.Join(root, "pkg", "theme_test.go")), reason: "no module file matches this name"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %+v\nwant %+v", got, want)
	}
}

// TestCheckFileNamesInDir_UnreadableDirectoryIsAViolation verifies a
// directory the gate cannot read is reported as a violation rather than
// certified clean.
func TestCheckFileNamesInDir_UnreadableDirectoryIsAViolation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got := checkFileNamesInDir(file)
	if len(got) != 1 {
		t.Fatalf("violations = %+v, want exactly one", got)
	}
	if got[0].path != filepath.ToSlash(file) || !strings.HasPrefix(got[0].reason, "unreadable: ") {
		t.Fatalf("violation = %+v, want unreadable %s", got[0], file)
	}
}

// TestRunFileCheck_ReportsVerdict verifies the gate's stdout contract and
// boolean verdict for a clean tree, a violating tree, and a missing
// directory.
func TestRunFileCheck_ReportsVerdict(t *testing.T) {
	testCases := []struct {
		name       string
		files      []fileSpec
		dirs       func(root string) []string
		wantOK     bool
		wantStdout func(root string) string
	}{
		{
			name:   "clean tree",
			files:  []fileSpec{{"kind.go", "package kind\n"}, {"kind_test.go", "package kind\n"}},
			wantOK: true,
			wantStdout: func(string) string {
				return "test-file naming: every test file is named after a module it tests\n"
			},
		},
		{
			name:  "violating tree",
			files: []fileSpec{{"kind.go", "package kind\n"}, {"theme_test.go", "package kind\n"}},
			wantStdout: func(root string) string {
				path := filepath.ToSlash(filepath.Join(root, "theme_test.go"))
				return fmt.Sprintf("%-70s %s\n", path, "no module file matches this name") +
					"test-file naming: 1 file(s) violate the convention\n"
			},
		},
		{
			name: "missing directory",
			dirs: func(root string) []string { return []string{filepath.Join(root, "absent")} },
			wantStdout: func(root string) string {
				path := filepath.ToSlash(filepath.Join(root, "absent"))
				return fmt.Sprintf("%-70s %s\n", path, "unreadable: open "+filepath.Join(root, "absent")+": no such file or directory") +
					"test-file naming: 1 file(s) violate the convention\n"
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFixtureDir(t, root, nil, tc.files)
			dirs := []string{root}
			if tc.dirs != nil {
				dirs = tc.dirs(root)
			}

			var stdout bytes.Buffer
			ok := runFileCheck(dirs, &stdout)
			if ok != tc.wantOK {
				t.Errorf("runFileCheck() = %t, want %t", ok, tc.wantOK)
			}
			if got, want := stdout.String(), tc.wantStdout(root); got != want {
				t.Errorf("stdout = %q\nwant %q", got, want)
			}
		})
	}
}

// TestHasBuildConstraint_FileShapes verifies the constraint is only honored
// when it precedes the package clause, and that an unreadable or
// comment-only file earns no exemption.
func TestHasBuildConstraint_FileShapes(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		missing bool
		want    bool
	}{
		{name: "constraint before the package clause", content: "//go:build unix\n\npackage kind\n", want: true},
		{name: "constraint after the package clause", content: "package kind\n\n//go:build unix\n", want: false},
		{name: "comments only", content: "// a header comment\n\n// another\n", want: false},
		{name: "missing file", missing: true, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kind_unix_test.go")
			if !tc.missing {
				if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			}
			if got := hasBuildConstraint(path); got != tc.want {
				t.Errorf("hasBuildConstraint() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestIsExternalTestPackage_UnreadableFileIsFalse verifies a file that
// cannot be parsed is not treated as an external test package.
func TestIsExternalTestPackage_UnreadableFileIsFalse(t *testing.T) {
	if isExternalTestPackage(filepath.Join(t.TempDir(), "missing_test.go")) {
		t.Fatal("isExternalTestPackage(missing) = true, want false")
	}
}

// writeFixtureDir creates the named subdirectories under dir and writes each
// file (paths relative to dir) with its content.
func writeFixtureDir(t *testing.T, dir string, dirs []string, files []fileSpec) {
	t.Helper()
	for _, sub := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o750); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", sub, err)
		}
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", f.name, err)
		}
	}
}
