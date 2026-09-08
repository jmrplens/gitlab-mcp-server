package testsource

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FileSuffix is what makes a Go file a test file.
const FileSuffix = "_test.go"

// goFileSuffix is the extension every Go source file carries; a test file is
// the subset of those whose name ends in FileSuffix.
const goFileSuffix = ".go"

// testPrefix is the prefix Go requires of a test entry point.
const testPrefix = "Test"

// mainTestName is the framework entry point for a test binary. It is spelled
// exactly, never as a prefix: TestMain_Something is an ordinary test.
const mainTestName = "TestMain"

// The naming buckets ClassifyTestName sorts a test name into. The convention
// this repository holds tests to is TestThing_Scenario_Outcome, so a 3-part
// name is compliant, a 2-part one is tolerated, and the other two are the
// legacy shapes the naming auditor offers to rewrite.
const (
	Pattern3Part        = "3-part"
	Pattern2Part        = "2-part"
	PatternNoUnderscore = "no-underscore"
	PatternTestCov      = "TestCov"
)

// covPattern matches the TestCovThing coverage-helper shape, which predates the
// naming convention and is classified apart so it can be counted and rewritten.
var covPattern = regexp.MustCompile(`^TestCov[A-Z]`)

// IsTestFunction reports whether name is a Go test entry point, by the rule the
// testing package itself applies: the prefix "Test" followed by a rune that is
// not lower case, or by nothing at all. "TestMain" is excluded because it is
// the framework's entry point rather than a test; a longer name that merely
// starts with those letters, such as TestMain_Flags_Parse, is a test.
func IsTestFunction(name string) bool {
	if name == mainTestName || !strings.HasPrefix(name, testPrefix) {
		return false
	}
	rest := name[len(testPrefix):]
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}

// ClassifyTestName returns the naming bucket name falls in. It classifies the
// name it is given and asks nothing about whether that name is a test, so a
// caller filters with IsTestFunction first.
func ClassifyTestName(name string) string {
	if covPattern.MatchString(name) {
		return PatternTestCov
	}
	switch parts := strings.Split(name, "_"); {
	case len(parts) >= 3:
		return Pattern3Part
	case len(parts) == 2:
		return Pattern2Part
	default:
		return PatternNoUnderscore
	}
}

// Policy names the files a walk hands to its visitor.
type Policy int

const (
	// TestFiles selects Go test files.
	TestFiles Policy = iota
	// NonTestGoFiles selects the Go source files that are not test files.
	NonTestGoFiles
)

// selects reports whether a regular file of this base name is in the corpus.
func (p Policy) selects(name string) bool {
	if !strings.HasSuffix(name, goFileSuffix) {
		return false
	}
	if p == NonTestGoFiles {
		return !strings.HasSuffix(name, FileSuffix)
	}
	return strings.HasSuffix(name, FileSuffix)
}

// SkipDir reports whether a directory of this base name is left out of a walk
// that descends into it: a generated or vendored tree, a tool's own fixtures,
// or a dot directory. The relative names "." and ".." are exempt because they
// name a tree the caller is already in rather than one to descend into.
//
// A walk root is exempt too, but that is WalkFiles' decision rather than this
// one: a scan pointed at a fixtures directory scans it, and only what lies
// below a root is judged by name.
func SkipDir(name string) bool {
	switch name {
	case "node_modules", "dist", "testdata":
		return true
	case ".", "..":
		return false
	default:
		return strings.HasPrefix(name, ".")
	}
}

// WalkFiles calls visit once for every file under each root that policy
// selects, in lexical order, entering no directory below a root that SkipDir
// names. A root is always entered, whatever it is called and whether it is a
// directory or a symlink to one, so a scan asked for one of those directories
// by name, or through a link, still runs. It stops at the first error,
// whether the walk raised it or visit returned it, so a caller that cannot
// parse a file reports that rather than a partial corpus.
func WalkFiles(roots []string, policy Policy, visit func(path string) error) error {
	for _, root := range roots {
		if err := walkRoot(root, policy, visit); err != nil {
			return err
		}
	}
	return nil
}

// walkRoot walks one root of WalkFiles.
//
// filepath.WalkDir lstats the root it is given, so a root that is a symlink to
// a directory arrives at the callback as a plain file and the tree below it is
// never read — a scan pointed at such a path would report an empty corpus and
// a gate would certify it clean. A root is named by the caller rather than
// found by the walk, so it is resolved first and every visited path is
// reported back under the name the caller gave, which keeps the paths in a
// report the ones the caller can act on. Below the root nothing is resolved:
// WalkDir does not follow symlinks it finds, and neither does this.
func walkRoot(root string, policy Policy, visit func(path string) error) error {
	target, err := resolveRoot(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		atRoot := path == target
		if target != root && !atRoot {
			// WalkDir builds every path below the root by joining it onto the
			// root it was given, so trimming that prefix is exact.
			path = filepath.Join(root, strings.TrimPrefix(path, target))
		}
		if d.IsDir() {
			if !atRoot && SkipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !policy.selects(d.Name()) {
			return nil
		}
		return visit(path)
	})
}

// resolveRoot returns the path to walk for root: the directory root points at
// when it is a symlink to one, and root itself otherwise. A symlink that
// cannot be resolved is an error rather than an empty walk, because a caller
// asked for that tree and must hear that it was not read.
func resolveRoot(root string) (string, error) {
	info, err := os.Lstat(root)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return root, nil //nolint:nilerr // an absent root is WalkDir's error to report, with its own path in it
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	if target, statErr := os.Stat(resolved); statErr != nil || !target.IsDir() {
		return root, nil //nolint:nilerr // a link to a non-directory is walked as the file it is
	}
	return resolved, nil
}
