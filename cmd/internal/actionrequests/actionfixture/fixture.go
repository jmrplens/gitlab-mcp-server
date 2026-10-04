package actionfixture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dir is the directory the fixture packages pretend to live in, relative to
// the module root. Nothing is written there: the packages exist only in the
// loader overlay, which keeps generated Go source out of the repository.
const Dir = "cmd/internal/actionrequests/fixture"

// Pattern matches every fixture package at once. It is spelled out rather
// than built from [Dir], and a test holds the two to each other.
const Pattern = "./cmd/internal/actionrequests/fixture/..."

// Backtick stands in for a backtick inside a fixture source, which is itself
// written as a raw string literal and so cannot contain one.
const Backtick = "@@"

// ErrNoModule is a start directory with no go.mod at or above it.
var ErrNoModule = errors.New("no go.mod above the directory")

// Root walks up from start, a test's working directory, to the module root,
// so an overlay can name absolute paths inside the module and the loader
// resolves the module's own import paths.
func Root(start string) (string, error) {
	dir := start
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w: %s", ErrNoModule, start)
		}
		dir = parent
	}
}

// Overlay turns a map of package name to source into a loader overlay rooted
// at root: each entry becomes one file in its own package directory under
// [Dir], with every [Backtick] made a backtick.
func Overlay(root string, sources map[string]string) map[string][]byte {
	overlay := make(map[string][]byte, len(sources))
	for name, source := range sources {
		path := filepath.Join(root, filepath.FromSlash(Dir), name, name+".go")
		overlay[path] = []byte(strings.ReplaceAll(source, Backtick, "`"))
	}
	return overlay
}

// Main is the fixture set the resolution and detection tests load: the
// construction shapes, the vulnerability-style handlers, the variable
// documents, and the second package that shares an action name with the
// first.
func Main() map[string]string {
	return map[string]string{
		"vuln":   Vuln,
		"shapes": Shapes,
		"vars":   Vars,
		"other":  Other,
	}
}
