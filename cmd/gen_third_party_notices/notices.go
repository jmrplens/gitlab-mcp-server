package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"unicode"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
)

// infoReader reads a binary's build information. Production passes
// [buildinfo.ReadFile]; the tests hand in build information no real binary
// records (two toolchains, a module at two versions, a target named twice)
// without building one.
type infoReader func(path string) (*debug.BuildInfo, error)

// runGoEnv asks the toolchain where GOROOT and the module cache are. It runs
// the go command of the toolchain running this one (see [golist.Executable]),
// which is the one that filled the module cache in every place this runs.
func runGoEnv() ([]byte, error) {
	return exec.CommandContext(context.Background(), golist.Executable(), "env", "-json", "GOROOT", "GOMODCACHE").Output() //#nosec G204 -- golist.Executable is the toolchain's own go binary, joined out of GOROOT
}

// licenseName matches the files at a module's root that carry a license,
// a notice or a patent grant: LICENSE in either spelling, UNLICENSE, COPYING,
// COPYRIGHT, NOTICE and PATENTS, in any case, alone, with a suffix after a
// dot, dash or underscore (LICENSE.md, LICENSE-APACHE, NOTICE.txt) or a
// prefix before one (MIT-LICENSE, THIRD-PARTY-NOTICES). A Go source file is
// never one, whatever its name.
var licenseName = regexp.MustCompile(`(?i)^(?:[a-z0-9]+[-_])*(?:un)?(?:licen[cs]e|copying|copyright|notices?|patents)(?:[-._][a-z0-9._-]*)?$`)

// linkedModule is one module the binaries link, as the build information
// records it, and the targets that link it.
type linkedModule struct {
	path, version string
	replace       *debug.Module
	targets       map[string]bool
}

// effective is the module whose files were compiled in: the replacement when
// there is one.
func (m *linkedModule) effective() (path, version string) {
	if m.replace != nil {
		return m.replace.Path, m.replace.Version
	}
	return m.path, m.version
}

// linkSet is what every binary read agrees on, and the modules they link.
type linkSet struct {
	mainPath, mainVersion, goVersion string
	targets                          map[string]string
	modules                          map[string]*linkedModule
}

// targetList is the targets the binaries were built for, sorted.
func (s *linkSet) targetList() []string {
	var list []string
	for target := range s.targets {
		list = append(list, target)
	}
	slices.Sort(list)
	return list
}

// requireTargets refuses a set of binaries that does not cover exactly the
// targets named, so a glob that matched one binary short of the release is
// a failure rather than notices that omit what that binary links. No names
// accept whatever the binaries are.
func (s *linkSet) requireTargets(want []string) error {
	if len(want) == 0 {
		return nil
	}
	named := slices.Clone(want)
	slices.Sort(named)
	named = slices.Compact(named)
	got := s.targetList()
	if !slices.Equal(got, named) {
		return fmt.Errorf("the binaries are built for %s, but -targets names %s",
			strings.Join(got, ", "), strings.Join(named, ", "))
	}
	return nil
}

// expand turns each argument into the files it names, a glob into every file
// it matches, and refuses an argument that names nothing: a pattern that
// matched no binary is a release missing one.
func expand(patterns []string) ([]string, error) {
	var paths []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pattern, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("%s matches no file", pattern)
		}
		paths = append(paths, matches...)
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

// targetOf reads the GOOS/GOARCH pair a binary was built for out of its
// build settings.
func targetOf(info *debug.BuildInfo) (string, error) {
	var goos, goarch string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "GOOS":
			goos = setting.Value
		case "GOARCH":
			goarch = setting.Value
		}
	}
	if goos == "" || goarch == "" {
		return "", errors.New("its build information records no GOOS or no GOARCH")
	}
	return goos + "/" + goarch, nil
}

// collect reads every binary's build information and merges the modules
// they link, refusing binaries that do not belong to one build: another main
// module, another toolchain, a target seen twice, or one module at two
// versions.
func collect(paths []string, read infoReader) (*linkSet, error) {
	set := &linkSet{targets: map[string]string{}, modules: map[string]*linkedModule{}}
	for _, path := range paths {
		info, err := read(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		target, err := targetOf(info)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if other, seen := set.targets[target]; seen {
			return nil, fmt.Errorf("%s and %s are both built for %s", other, path, target)
		}
		if len(set.targets) == 0 {
			set.mainPath, set.mainVersion, set.goVersion = info.Main.Path, info.Main.Version, info.GoVersion
		}
		if info.Main.Path != set.mainPath || info.Main.Version != set.mainVersion {
			return nil, fmt.Errorf("%s is built from %s %s, the others from %s %s",
				path, info.Main.Path, info.Main.Version, set.mainPath, set.mainVersion)
		}
		if info.GoVersion != set.goVersion {
			return nil, fmt.Errorf("%s is built with %s, the others with %s", path, info.GoVersion, set.goVersion)
		}
		set.targets[target] = path
		if addErr := set.add(path, target, info.Deps); addErr != nil {
			return nil, addErr
		}
	}
	return set, nil
}

// add records the modules one binary links.
func (s *linkSet) add(path, target string, deps []*debug.Module) error {
	for _, dep := range deps {
		known, seen := s.modules[dep.Path]
		if !seen {
			known = &linkedModule{path: dep.Path, version: dep.Version, replace: dep.Replace, targets: map[string]bool{}}
			s.modules[dep.Path] = known
		}
		if describe(dep.Version, dep.Replace) != describe(known.version, known.replace) {
			return fmt.Errorf("%s links %s at %s, another binary at %s",
				path, dep.Path, describe(dep.Version, dep.Replace), describe(known.version, known.replace))
		}
		known.targets[target] = true
	}
	return nil
}

// describe names a module version and, when there is one, its replacement.
func describe(version string, replace *debug.Module) string {
	if replace == nil {
		return version
	}
	return version + " => " + replace.Path + " " + replace.Version
}

// goEnv is where the texts are read from.
type goEnv struct {
	goroot, modcache string
}

// resolveGoEnv returns the GOROOT and module cache to read, asking `go env`
// (through ask) for whichever the caller did not name.
func resolveGoEnv(goroot, modcache string, ask func() ([]byte, error)) (goEnv, error) {
	env := goEnv{goroot: goroot, modcache: modcache}
	if env.goroot != "" && env.modcache != "" {
		return env, nil
	}
	out, err := ask()
	if err != nil {
		return env, fmt.Errorf("go env: %w", err)
	}
	var reported struct{ GOROOT, GOMODCACHE string }
	if decodeErr := json.Unmarshal(out, &reported); decodeErr != nil {
		return env, fmt.Errorf("go env: %w", decodeErr)
	}
	if env.goroot == "" {
		env.goroot = reported.GOROOT
	}
	if env.modcache == "" {
		env.modcache = reported.GOMODCACHE
	}
	if env.goroot == "" || env.modcache == "" {
		return env, errors.New("go env reports no GOROOT or no GOMODCACHE")
	}
	return env, nil
}

// licenseText is one file of license text and the name it has in its module.
type licenseText struct {
	name, text string
}

// readLicenses reads every license, notice and patent file at the root of
// fsys, which is the directory dir, in name order, refusing one that holds
// none: notices short of a module's license are notices this release cannot
// ship. A symbolic link or a directory is never a text, whatever its name.
func readLicenses(fsys fs.FS, dir, what string) ([]licenseText, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", what, dir, err)
	}
	var texts []licenseText
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !licenseName.MatchString(name) || strings.HasSuffix(strings.ToLower(name), ".go") {
			continue
		}
		data, readErr := fs.ReadFile(fsys, name)
		if readErr != nil {
			return nil, fmt.Errorf("%s: %s: %w", what, dir, readErr)
		}
		texts = append(texts, licenseText{name: name, text: normalize(string(data))})
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("%s publishes no license file in %s", what, dir)
	}
	return texts, nil
}

// normalize gives a text Unix line endings and exactly one trailing newline,
// so the file reads the same whichever platform each module was released
// from.
func normalize(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.TrimRight(text, " \t\n") + "\n"
}

// escapeModule escapes a module path or version the way the module cache
// spells it on disk: every upper-case letter becomes '!' and its lower case,
// since the cache must work on case-insensitive file systems.
func escapeModule(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsUpper(r) {
			b.WriteByte('!')
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// moduleDir is where the module cache holds a module's extracted files.
func moduleDir(modcache, path, version string) string {
	return filepath.Join(modcache, filepath.FromSlash(escapeModule(path)+"@"+escapeModule(version)))
}

// stdlibLicenses reads the standard library's texts out of GOROOT, after
// holding GOROOT's VERSION to the toolchain the binaries record, so the
// texts are the ones of the Go that built them.
func stdlibLicenses(goroot, goVersion string) ([]licenseText, error) {
	data, err := os.ReadFile(filepath.Join(goroot, "VERSION"))
	if err != nil {
		return nil, fmt.Errorf("GOROOT %s: %w", goroot, err)
	}
	have, _, _ := strings.Cut(string(data), "\n")
	want, _, _ := strings.Cut(goVersion, " ")
	if strings.TrimSpace(have) != want {
		return nil, fmt.Errorf("GOROOT %s is %s, but the binaries were built with %s", goroot, strings.TrimSpace(have), want)
	}
	return readLicenses(os.DirFS(goroot), goroot, "the Go standard library")
}

// section is one part of the notices: a component, its version, where it is
// linked, and its texts.
type section struct {
	heading string
	lines   []string
	texts   []licenseText
}

// render reads every text and writes the notices.
func render(set *linkSet, env goEnv) ([]byte, error) {
	stdlib, err := stdlibLicenses(env.goroot, set.goVersion)
	if err != nil {
		return nil, err
	}
	toolchain, _, _ := strings.Cut(set.goVersion, " ")
	sections := []section{{heading: "Go standard library " + toolchain, texts: stdlib}}

	var paths []string
	for path := range set.modules {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		module := set.modules[path]
		effPath, effVersion := module.effective()
		if effVersion == "" {
			return nil, fmt.Errorf("%s is replaced by the local directory %s, which no module cache holds", path, effPath)
		}
		dir := moduleDir(env.modcache, effPath, effVersion)
		texts, readErr := readLicenses(os.DirFS(dir), dir, path+" "+module.version)
		if readErr != nil {
			return nil, readErr
		}
		s := section{heading: path + " " + module.version, texts: texts}
		if module.replace != nil {
			s.lines = append(s.lines, "Replaced by: "+effPath+" "+effVersion)
		}
		if len(module.targets) != len(set.targets) {
			var linked []string
			for target := range module.targets {
				linked = append(linked, target)
			}
			slices.Sort(linked)
			s.lines = append(s.lines, "Linked into: "+strings.Join(linked, ", ")+" only")
		}
		sections = append(sections, s)
	}
	return write(set, sections), nil
}

// rule separates one component from the next.
var rule = strings.Repeat("=", 80)

// write lays the notices out: a header naming what they cover, a list of
// contents, then each component with its texts in full.
func write(set *linkSet, sections []section) []byte {
	var b bytes.Buffer
	b.WriteString("Third-party notices for gitlab-mcp-server\n\n")
	fmt.Fprintf(&b, "Module:    %s %s\n", set.mainPath, set.mainVersion)
	fmt.Fprintf(&b, "Toolchain: %s\n", set.goVersion)
	fmt.Fprintf(&b, "Builds:    %s\n\n", strings.Join(set.targetList(), ", "))
	b.WriteString(`gitlab-mcp-server is distributed under the MIT License, in the LICENSE
file that accompanies this one. Its binaries also contain the Go standard
library and the Go modules listed below, each distributed under its own
license. Every license, notice and patent file each of them publishes is
reproduced below in full, read from the module at the version the
binaries link. cmd/gen_third_party_notices writes this file from the
build information the binaries record.

Contents

`)
	for _, s := range sections {
		b.WriteString("  " + s.heading + "\n")
	}
	for _, s := range sections {
		b.WriteString("\n" + rule + "\n" + s.heading + "\n")
		for _, line := range s.lines {
			b.WriteString(line + "\n")
		}
		for _, t := range s.texts {
			b.WriteString("\n--- " + t.name + " ---\n\n" + t.text)
		}
	}
	return b.Bytes()
}
