// variant.go builds the arm a bound with no switch is compared against.
//
// Every other bound here is put in force and taken out by the switches a
// process is started with, and the two arms run one binary. A constant no
// operator can move has no such switch, and the server must not grow one: a
// flag that disables a security bound is a flag somebody sets in production.
// So the arm without the bound runs a build of its own, this checkout with the
// one declaration that sizes the bound replaced, made through the go command's
// overlay so the tree on disk is never touched and the two builds differ in
// that line alone.

package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// buildVariant is a build of this checkout that differs from the one under
// test in one declaration.
type buildVariant struct {
	// File is the source file the declaration is in, slash-separated and
	// relative to the module root.
	File string
	// Declaration is how the line to replace begins once its indentation is
	// set aside, and exactly one line of File may begin that way.
	Declaration string
	// Replacement is the line written in its place.
	Replacement string
}

// describe names what the variant replaced, for the progress line and the
// record, so a reader sees what the arm without the bound actually ran.
func (v buildVariant) describe() string { return v.File + ": " + v.Replacement }

// rewrite returns src with the declaration's line replaced.
//
// It refuses a file that no longer holds the declaration, and one that holds
// it twice, rather than building whatever it finds: a variant that replaced
// nothing is the binary under test in both arms, which reads as a bound that
// changes nothing, and one that replaced the wrong line is a comparison
// against something else. Either is a code change the bound has to follow,
// and the error names the line it looked for.
func (v buildVariant) rewrite(src []byte) ([]byte, error) {
	lines := strings.SplitAfter(string(src), "\n")
	found := -1
	for index, line := range lines {
		if !strings.HasPrefix(strings.TrimLeft(line, " \t"), v.Declaration) {
			continue
		}
		if found >= 0 {
			return nil, fmt.Errorf("%s declares %q on more than one line, so the variant cannot tell which to replace",
				v.File, v.Declaration)
		}
		found = index
	}
	if found < 0 {
		return nil, fmt.Errorf("%s no longer declares %q: the arm without the bound is built by replacing that line, "+
			"so the bound's variant has to follow the code", v.File, v.Declaration)
	}
	ending := strings.TrimPrefix(lines[found], strings.TrimSuffix(lines[found], "\n"))
	lines[found] = v.Replacement + ending
	return []byte(strings.Join(lines, "")), nil
}

// buildOverlay is the document the go command reads its -overlay from: which
// files of the tree to build from another path instead.
type buildOverlay struct {
	Replace map[string]string `json:"Replace"`
}

// overlay writes the rewritten file and the overlay pointing the build at it
// into dir, and returns the overlay's path.
func (v buildVariant) overlay(root, dir string) (string, error) {
	source := filepath.Join(root, filepath.FromSlash(v.File))
	original, err := os.ReadFile(source) // #nosec G304 -- a path the bound table names inside this checkout
	if err != nil {
		return "", fmt.Errorf("read the file the variant replaces a line of: %w", err)
	}
	rewritten, err := v.rewrite(original)
	if err != nil {
		return "", err
	}
	replaced := filepath.Join(dir, path.Base(v.File))
	//#nosec G703 -- a file of this run's own build directory, named after a path the bound table names
	if writeErr := os.WriteFile(replaced, rewritten, 0o600); writeErr != nil {
		return "", fmt.Errorf("write the variant's source: %w", writeErr)
	}
	document := filepath.Join(dir, "overlay.json")
	if writeErr := writeJSON(document, &buildOverlay{Replace: map[string]string{source: replaced}}, "the build overlay"); writeErr != nil {
		return "", writeErr
	}
	return document, nil
}

// buildVariantServer compiles the variant into a directory of its own and
// returns the binary's path, whose directory the caller removes.
func buildVariantServer(root string, v buildVariant) (string, error) {
	dir, err := os.MkdirTemp("", "bench-resources-variant")
	if err != nil {
		return "", fmt.Errorf("create a build directory: %w", err)
	}
	overlay, err := v.overlay(root, dir)
	if err == nil {
		out := filepath.Join(dir, serverExecutable())
		fmt.Printf("building ./cmd/server with %s\n", v.describe())
		if err = goBuild(root, out, "-overlay", overlay); err == nil {
			return out, nil
		}
	}
	_ = os.RemoveAll(dir)
	return "", err
}

// buildVariantBinary builds the arm without a bound that has no switch. A
// variable for the reason buildServerBinary is one: a test drives the mode
// without a minute of linking.
var buildVariantBinary = buildVariantServer
