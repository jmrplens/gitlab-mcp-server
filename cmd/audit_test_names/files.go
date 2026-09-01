// files.go implements the -check-files mode: the test-file naming
// convention, as a gate.
//
// A _test.go file may only exist under the name of the module it tests
// (register.go -> register_test.go). Theme-named files hide tests from the
// reader and once hid them from CI itself: coverage_boost_test.go matched
// .gitignore's coverage_* rule and 622 lines of real tests sat untracked for
// months. Three shapes are exempt, each for a reason the rule cannot absorb:
//
//   - export_test.go: the standard Go idiom for exporting internals to an
//     external test package.
//   - <module>_<qualifier>_test.go carrying a //go:build constraint, when
//     <module>.go exists: a platform-gated test cannot live in the module's
//     unconstrained test file (fileutils.go -> fileutils_unix_test.go).
//   - <module>_<qualifier>_test.go in an external package (package x_test),
//     when <module>.go exists and an internal <module>_test.go holds the
//     plain name: Go allows one package per file name, so external-package
//     tests forced by an import cycle need a qualified sibling
//     (kind.go -> kind_test.go + kind_integration_test.go). Without that
//     internal sibling the external tests take the plain name themselves.
//
// test/e2e is exempt as a tree: its files have no source modules to be named
// after.
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// testFileSuffix is the suffix that makes a Go file a test file, and the
// part the naming convention leaves aside when matching a module.
const testFileSuffix = "_test.go"

// fileViolation is one test file whose name matches no module.
type fileViolation struct {
	path   string
	reason string
}

// runFileCheck audits test-file names under the given directories and
// reports whether the tree is clean.
func runFileCheck(dirs []string, stdout io.Writer) bool {
	var violations []fileViolation
	for _, dir := range dirs {
		violations = append(violations, checkFileNamesInDir(filepath.Clean(dir))...)
	}
	if len(violations) == 0 {
		fmt.Fprintln(stdout, "test-file naming: every test file is named after a module it tests")
		return true
	}
	for _, v := range violations {
		fmt.Fprintf(stdout, "%-70s %s\n", v.path, v.reason)
	}
	fmt.Fprintf(stdout, "test-file naming: %d file(s) violate the convention\n", len(violations))
	return false
}

// checkFileNamesInDir walks one directory tree collecting violations.
func checkFileNamesInDir(dir string) []fileViolation {
	if isE2ETree(dir) {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		// An unreadable directory is itself a violation: a gate that cannot
		// read what it was asked to certify must fail, not report clean.
		return []fileViolation{{path: filepath.ToSlash(dir), reason: "unreadable: " + err.Error()}}
	}

	var violations []fileViolation
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			violations = append(violations, checkFileNamesInDir(path)...)
			continue
		}
		if !strings.HasSuffix(e.Name(), testFileSuffix) {
			continue
		}
		if reason, ok := classifyTestFileName(dir, e.Name()); !ok {
			violations = append(violations, fileViolation{path: filepath.ToSlash(path), reason: reason})
		}
	}
	return violations
}

// isE2ETree reports whether the path lies in the exempt test/e2e tree.
func isE2ETree(path string) bool {
	slashed := filepath.ToSlash(path)
	return slashed == "test/e2e" || strings.HasPrefix(slashed, "test/e2e/") ||
		strings.Contains(slashed, "/test/e2e/") || strings.HasSuffix(slashed, "/test/e2e")
}

// classifyTestFileName decides whether one test file name is allowed in its
// directory, returning the violation reason when it is not.
func classifyTestFileName(dir, name string) (reason string, ok bool) {
	base := strings.TrimSuffix(name, testFileSuffix)
	if base == "export" {
		return "", true
	}
	if moduleExists(dir, base) {
		return "", true
	}

	// Qualifier form: the longest module prefix decides which module the
	// file claims to test; the qualifier is only earned by a build
	// constraint or an external test package.
	module := longestModulePrefix(dir, base)
	if module == "" {
		return "no module file matches this name", false
	}
	path := filepath.Join(dir, name)
	if hasBuildConstraint(path) {
		return "", true
	}
	// The external-package exemption is earned by a filename conflict, not by
	// the package clause alone: without an internal <module>_test.go the
	// external tests could simply take the plain name themselves.
	if isExternalTestPackage(path) && internalTestSiblingExists(dir, module) {
		return "", true
	}
	return fmt.Sprintf("qualified name over %s.go needs a //go:build constraint, or an external test package alongside an internal %s_test.go", module, module), false
}

// internalTestSiblingExists reports whether dir holds <module>_test.go
// declaring the module's own package, which is what makes a qualified
// external-package name a forced choice rather than a stylistic one.
func internalTestSiblingExists(dir, module string) bool {
	path := filepath.Join(dir, module+testFileSuffix)
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return !isExternalTestPackage(path)
}

// moduleExists reports whether dir holds a non-test source file named
// base.go.
func moduleExists(dir, base string) bool {
	if strings.HasSuffix(base, "_test") {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, base+".go"))
	return err == nil && !info.IsDir()
}

// longestModulePrefix returns the longest underscore-delimited prefix of
// base for which a module file exists in dir, or "".
func longestModulePrefix(dir, base string) string {
	parts := strings.Split(base, "_")
	for cut := len(parts) - 1; cut >= 1; cut-- {
		candidate := strings.Join(parts[:cut], "_")
		if moduleExists(dir, candidate) {
			return candidate
		}
	}
	return ""
}

// hasBuildConstraint reports whether the file starts with a //go:build line.
func hasBuildConstraint(path string) bool {
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//go:build ") {
			return true
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			// The constraint must precede the package clause.
			return false
		}
	}
	return false
}

// isExternalTestPackage reports whether the file declares a package ending
// in _test.
func isExternalTestPackage(path string) bool {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filepath.Clean(path), nil, parser.PackageClauseOnly)
	if err != nil {
		return false
	}
	return strings.HasSuffix(node.Name.Name, "_test")
}
