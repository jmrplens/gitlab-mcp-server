// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
)

// tagHint is said after a refusal a build tag may answer.
const tagHint = "a package behind a build tag is measured with TAGS=<tag>, which reaches go list and gobco's go test alike"

// excludedByConstraints is the go command's answer for a platform that builds
// no file of a package.
const excludedByConstraints = "build constraints exclude all Go files"

// testDeclaration is a top-level declaration that makes a file one of tests.
// A test file without one is there for what it declares.
var testDeclaration = regexp.MustCompile(`(?m)^func (Test|Benchmark|Fuzz|Example)`)

// constraintName is a name a build constraint uses.
var constraintName = regexp.MustCompile(`[A-Za-z0-9_.]+`)

// platform is the build context every go command of the run shares with the
// `go test` gobco runs: the platform, the module cache, and the names a
// constraint may use and still be read by gobco's context exactly as the go
// command reads it, which are every operating system and architecture the
// toolchain knows, and `unix`, which go/build derives from GOOS the same way.
type platform struct {
	target   string
	modCache string
	names    map[string]bool
}

// staging is why a package is measured in a copy of the module, and is empty
// for one gobco reads where it is.
type staging struct {
	// leftOut are the .go files gobco would parse and the go command does
	// not build here.
	leftOut []string
	// constrained are the files the go command builds under a constraint
	// gobco's own context would misjudge.
	constrained []string
	// support are the package's own test files that declare no test, where
	// an external test package may use their names.
	support []string
	// unimported is set for an external test package that imports nothing
	// of its package.
	unimported bool
}

// needed reports whether the package has to be staged.
func (s staging) needed() bool {
	return len(s.leftOut) > 0 || len(s.constrained) > 0 || len(s.support) > 0 || s.unimported
}

// tagArgs are the build tags as a go command takes them.
func (m *measurement) tagArgs() []string {
	if m.Tags == "" {
		return nil
	}
	return []string{"-tags", m.Tags}
}

// list asks go list for the package, with env added to its environment. -e
// keeps a package go cannot load (a path that is not there, a directory every
// file of which a constraint leaves out) from ending the listing with go's
// own message, which does not say what the run needed.
func (m *measurement) list(ctx context.Context, env []string) (golist.PackageFiles, error) {
	args := append([]string{"list", "-e"}, m.tagArgs()...)
	out, err := m.goOutput(ctx, env, append(args, "-f", golist.FilesFormat, m.Package)...)
	if err != nil {
		return golist.PackageFiles{}, err
	}
	rows, err := golist.ParseFileRows([]byte(out))
	if err != nil {
		return golist.PackageFiles{}, err
	}
	if len(rows) != 1 {
		return golist.PackageFiles{}, fmt.Errorf("it names %d packages, and one is measured at a time", len(rows))
	}
	return rows[0], nil
}

// load reads the package under the build context every go command here shares
// with the `go test` gobco runs, and reports whether it can be measured.
func (m *measurement) load(ctx context.Context) bool {
	pkg, err := m.list(ctx, nil)
	if err != nil {
		m.refusef(ctx, "go list could not list %s under build tags %s: %v; refusing to measure", m.Package, m.tagsShown(), err)
		return false
	}
	if pkg.Error != "" {
		m.refusef(ctx, "go cannot load %s under build tags %s: %s; refusing to measure", m.Package, m.tagsShown(), pkg.Error)
		m.refusef(ctx, tagHint)
		return false
	}
	m.pkg = pkg
	return true
}

// plan reads the platform, the files a beyond: gate holds, and what has to be
// staged of the package and why. An error is the reason nothing can be
// measured, in the words of a refusal.
func (m *measurement) plan(ctx context.Context) error {
	if err := m.readPlatform(ctx); err != nil {
		return err
	}
	if err := m.readReference(ctx); err != nil {
		return err
	}
	if err := m.readStaging(); err != nil {
		return err
	}
	if len(m.pkg.TestGoFiles) == 0 && len(m.pkg.XTestGoFiles) == 0 && slices.ContainsFunc(m.staged.leftOut, isTestFile) {
		return fmt.Errorf("%s keeps no test file on %s: its tests are among the files the go command leaves out here (%s), so no test would run and gobco would measure nothing; refusing to measure\ngobco: %s",
			m.Package, m.context(), joined(m.staged.leftOut), tagHint)
	}
	return nil
}

// isTestFile reports whether a file is a test by its name.
func isTestFile(name string) bool {
	return strings.HasSuffix(name, "_test.go")
}

// readPlatform asks the go command which platform it builds for, where its
// module cache is, and which platforms there are.
func (m *measurement) readPlatform(ctx context.Context) error {
	env, err := m.goOutput(ctx, nil, "env", "GOOS", "GOARCH", "GOMODCACHE")
	if err != nil {
		return fmt.Errorf("go env failed: %w; refusing to measure", err)
	}
	lines := strings.Split(strings.ReplaceAll(env, "\r", ""), "\n")
	if len(lines) < 3 {
		return fmt.Errorf("go env answered %q for three variables; refusing to measure", env)
	}
	dist, err := m.goOutput(ctx, nil, "tool", "dist", "list")
	if err != nil {
		return fmt.Errorf("go tool dist list failed: %w; refusing to measure", err)
	}
	names := map[string]bool{"unix": true}
	for _, name := range strings.FieldsFunc(dist, func(r rune) bool { return strings.ContainsRune("/\r\n", r) }) {
		names[name] = true
	}
	m.platform = platform{target: lines[0] + "/" + lines[1], modCache: lines[2], names: names}
	return nil
}

// readReference finds the files a beyond: gate holds: the sources this build
// compiles that the reference platform does not, asked of the go command under
// the same tags. CGO_ENABLED is set because the go command turns cgo off for a
// platform other than the host's, which would count a cgo file the reference
// does build, on a machine with a C toolchain, as one it leaves out. A package
// the reference builds no file of at all is one whose every source is beyond
// it.
func (m *measurement) readReference(ctx context.Context) error {
	if m.settings.reference == "" {
		return nil
	}
	goos, goarch, _ := strings.Cut(m.settings.reference, "/")
	reference, err := m.list(ctx, []string{"CGO_ENABLED=1", "GOOS=" + goos, "GOARCH=" + goarch})
	if err != nil {
		return fmt.Errorf("go list could not list %s for %s: %w; refusing to measure", m.Package, m.settings.reference, err)
	}
	built := reference.Sources()
	if strings.Contains(reference.Error, excludedByConstraints) {
		built = nil
	} else if reference.Error != "" {
		return fmt.Errorf("go cannot load %s for %s under build tags %s, which GOBCO_GATE=%s compares this build with: %s; refusing to measure",
			m.Package, m.settings.reference, m.tagsShown(), m.settings.gate, reference.Error)
	}
	for _, name := range m.pkg.Sources() {
		if !slices.Contains(built, name) {
			m.gated = append(m.gated, name)
		}
	}
	return nil
}

// readStaging decides what of the package gobco cannot read where it is.
func (m *measurement) readStaging() error {
	entries, err := os.ReadDir(m.pkg.Dir)
	if err != nil {
		return fmt.Errorf("the files of %s could not be listed: %w; refusing to measure", m.Package, err)
	}
	built := m.pkg.Built()
	for _, entry := range entries {
		// Every file gobco would parse: each regular file of the directory
		// whose name ends in .go, a leading dot or underscore included,
		// since ParseDir skips none of them and the go command skips all of
		// them.
		if strings.HasSuffix(entry.Name(), ".go") && !slices.Contains(built, entry.Name()) && isRegular(filepath.Join(m.pkg.Dir, entry.Name())) {
			m.staged.leftOut = append(m.staged.leftOut, entry.Name())
		}
	}
	var unread []error
	for _, name := range built {
		header, readErr := os.ReadFile(filepath.Join(m.pkg.Dir, name))
		unread = append(unread, readErr)
		if m.platform.misjudges(string(header)) {
			m.staged.constrained = append(m.staged.constrained, name)
		}
	}
	if len(m.pkg.XTestGoFiles) > 0 {
		for _, name := range m.pkg.TestGoFiles {
			text, readErr := os.ReadFile(filepath.Join(m.pkg.Dir, name))
			unread = append(unread, readErr)
			if !testDeclaration.Match(text) {
				m.staged.support = append(m.staged.support, name)
			}
		}
		m.staged.unimported = m.pkg.Name != "main" && !slices.Contains(m.pkg.XTestImports, m.pkg.ImportPath)
	}
	if err = errors.Join(unread...); err != nil {
		return fmt.Errorf("a file of %s could not be read: %w; refusing to measure", m.Package, err)
	}
	return nil
}

// isRegular reports whether path is a regular file, or a link to one.
func isRegular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// misjudges reports whether a file's header carries a build constraint in
// either spelling that names anything gobco's own context is not given, which
// is what that context would misjudge.
func (p platform) misjudges(text string) bool {
	for line := range headerLines(text) {
		constraint, ok := constraintOf(line)
		if !ok {
			continue
		}
		for _, name := range constraintName.FindAllString(constraint, -1) {
			if !p.names[name] {
				return true
			}
		}
	}
	return false
}

// headerLines are the lines of a file above its package clause.
func headerLines(text string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for line := range strings.SplitSeq(text, "\n") {
			if isPackageClause(line) || !yield(line) {
				return
			}
		}
	}
}

// isPackageClause reports whether a line opens the package clause.
func isPackageClause(line string) bool {
	rest, ok := strings.CutPrefix(line, "package")
	return ok && startsBlank(rest)
}

// constraintOf returns what a line constrains the build to, when it is a
// build constraint in either spelling.
func constraintOf(line string) (constraint string, ok bool) {
	for _, spelling := range []string{"//go:build", "// +build"} {
		if rest, cut := strings.CutPrefix(line, spelling); cut && startsBlank(rest) {
			return rest, true
		}
	}
	return "", false
}

// startsBlank reports whether text opens with a space or a tab.
func startsBlank(text string) bool {
	return strings.HasPrefix(text, " ") || strings.HasPrefix(text, "\t")
}
