package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
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

// listedPackage is one package a build links, as `go list -deps` reports it:
// its import path, the module, at the version required, that provides it, and
// the replacement the go.mod being listed names for that module, nil when it
// names none.
type listedPackage struct {
	importPath, modulePath, moduleVersion string
	replace                               *debug.Module
}

// packageLister names the packages one binary links. Production asks
// `go list -deps` ([goListPackages]); the tests hand in the packages of
// fixture binaries no real build produced.
type packageLister func(info *debug.BuildInfo) ([]listedPackage, error)

// listFormat prints one line per package that belongs to a module, with the
// module's replacement after it when there is one. The standard library's
// packages are left out, since its texts are read from GOROOT's root (see the
// package comment for why no directory below it is). It is one literal rather
// than a concatenation: a constant has no statement to cover, so the mutation
// gate would report the operator of a concatenation as not covered.
const listFormat = "{{if .Module}}{{.ImportPath}}\t{{.Module.Path}}\t{{.Module.Version}}{{with .Module.Replace}}\t{{.Path}}\t{{.Version}}{{end}}{{end}}"

// goListPackages asks the toolchain which packages the build that produced
// info links. Build information records modules, not packages, and a module
// can carry a license of its own in a package directory below its root (code
// copied in from another project keeps that project's license where it was
// put), so the package graph is listed again, for the binary's main package
// under the target and build settings the binary records. It runs in the
// working directory, which has to be inside the module the binaries were
// built from, and the go command of the toolchain running this one
// ([golist.Executable]).
func goListPackages(info *debug.BuildInfo) ([]listedPackage, error) {
	args, env := listInvocation(info, os.Environ())
	cmd := exec.CommandContext(context.Background(), golist.Executable(), args...) //#nosec G204 -- golist.Executable is the toolchain's own go binary, joined out of GOROOT
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -deps %s: %w: %s", info.Path, err, strings.TrimSpace(stderr.String()))
	}
	return parseListed(out)
}

// listInvocation is the `go list` command line and environment that list the
// packages of the build info records. The build tags are a flag of that
// command, and so are the instrumentation flags -race, -msan and -asan, each
// recorded as true when it was given, since each adds a build tag of its own;
// the target and toolchain settings (GOOS, GOARCH, the GOAMD64 family,
// GOEXPERIMENT, CGO_ENABLED) are its environment, appended after environ so
// they win over whatever the caller's environment says. Every other setting
// (-buildmode, -ldflags, -trimpath, the vcs.* stamps) changes how a package is
// built and never which packages are linked, so it is left out.
func listInvocation(info *debug.BuildInfo, environ []string) (args, env []string) {
	args = []string{"list", "-deps", "-f", listFormat}
	env = environ
	for _, setting := range info.Settings {
		// Separate ifs rather than a tagless switch, whose case expressions
		// carry no statement counter for the mutation tool to measure. No key
		// satisfies two of them.
		if setting.Key == "-tags" {
			args = append(args, "-tags", setting.Value)
		}
		if slices.Contains(instrumentFlags, setting.Key) && setting.Value == "true" {
			args = append(args, setting.Key)
		}
		if setting.Key == "CGO_ENABLED" || strings.HasPrefix(setting.Key, "GO") {
			env = append(env, setting.Key+"="+setting.Value)
		}
	}
	return append(args, info.Path), env
}

// instrumentFlags are the build flags that instrument a binary, which build
// information records as settings of the same name.
var instrumentFlags = []string{"-race", "-msan", "-asan"}

// parseListed reads the lines [listFormat] prints, refusing one it cannot.
//
// A replacement by a local directory has no version in the listing, while the
// build information records it as (devel), so it is read as (devel) here and
// the two compare as one.
func parseListed(out []byte) ([]listedPackage, error) {
	var packages []listedPackage
	for line := range strings.SplitSeq(string(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 && len(fields) != 5 {
			return nil, fmt.Errorf("go list printed %q, not a package, a module and a version, and a replacement's path and version", line)
		}
		listed := listedPackage{importPath: fields[0], modulePath: fields[1], moduleVersion: fields[2]}
		if len(fields) == 5 {
			listed.replace = &debug.Module{Path: fields[3], Version: cmp.Or(fields[4], develVersion)}
		}
		packages = append(packages, listed)
	}
	return packages, nil
}

// develVersion is how build information records a module it has no version
// for: the main module of a build outside version control, and a replacement
// by a local directory.
const develVersion = "(devel)"

// licenseName matches the files at a module's root that carry a license,
// a notice or a patent grant: LICENSE in either spelling, UNLICENSE, COPYING,
// COPYRIGHT, NOTICE and PATENTS, in any case, alone, with a suffix after a
// dot, dash or underscore (LICENSE.md, LICENSE-APACHE, NOTICE.txt) or a
// prefix before one (MIT-LICENSE, THIRD-PARTY-NOTICES). A Go source file is
// never one, whatever its name.
var licenseName = regexp.MustCompile(`(?i)^(?:[a-z0-9]+[-_])*(?:un)?(?:licen[cs]e|copying|copyright|notices?|patents)(?:[-._][a-z0-9._-]*)?$`)

// linkedModule is one module the binaries link, as the build information
// records it, the targets that link it, and the directories below its root
// that hold a linked package or a parent of one.
type linkedModule struct {
	path, version string
	replace       *debug.Module
	targets       map[string]bool
	dirs          map[string]bool
}

// effective is the module whose files were compiled in: the replacement when
// there is one.
func (m *linkedModule) effective() (path, version string) {
	if m.replace != nil {
		return m.replace.Path, m.replace.Version
	}
	return m.path, m.version
}

// linkSet is what every binary read agrees on, the modules they link, and
// each target's build information.
type linkSet struct {
	mainPath, mainVersion, goVersion string
	targets                          map[string]string
	modules                          map[string]*linkedModule
	infos                            map[string]*debug.BuildInfo
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
	set := &linkSet{targets: map[string]string{}, modules: map[string]*linkedModule{}, infos: map[string]*debug.BuildInfo{}}
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
		set.infos[target] = info
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
			known = &linkedModule{path: dep.Path, version: dep.Version, replace: dep.Replace, targets: map[string]bool{}, dirs: map[string]bool{}}
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

// addPackages lists the packages each target links and records, for every
// module, the directories below its root that hold one of them, and every
// parent of such a directory: a license placed there governs the package
// below it. A directory holding only packages no binary links is never
// recorded, so a license the binaries carry no code under is not reproduced.
// The main module's packages are this project's own, under LICENSE.
//
// The listing is held to each target's own build information in both
// directions, since it is the evidence the nested texts rest on: a package
// from a module that target's build information does not name, or names at
// another version, and a module it names that the listing gives no package
// for, each mean the tree being listed is not the one the binaries were built
// from, and are refused.
func (s *linkSet) addPackages(list packageLister) error {
	for _, target := range s.targetList() {
		packages, err := list(s.infos[target])
		if err != nil {
			return fmt.Errorf("%s: %w", s.targets[target], err)
		}
		listed := map[string]bool{}
		for _, p := range packages {
			if p.modulePath == s.mainPath {
				continue
			}
			module, known := s.modules[p.modulePath]
			if !known || !module.targets[target] {
				return fmt.Errorf("go list reports %s from %s, which the build information of %s does not name", p.importPath, p.modulePath, s.targets[target])
			}
			if listed, linked := describe(p.moduleVersion, p.replace), describe(module.version, module.replace); listed != linked {
				return fmt.Errorf("go list reports %s at %s, but the binaries link %s", p.modulePath, listed, linked)
			}
			listed[p.modulePath] = true
			rel := strings.TrimPrefix(strings.TrimPrefix(p.importPath, p.modulePath), "/")
			for _, dir := range withParents(rel) {
				module.dirs[dir] = true
			}
		}
		for _, path := range slices.Sorted(maps.Keys(s.modules)) {
			if s.modules[path].targets[target] && !listed[path] {
				return fmt.Errorf("the build information of %s names %s, which go list gives no package of", s.targets[target], path)
			}
		}
	}
	return nil
}

// withParents is a slash-separated directory below a module root and each of
// its parents short of the root: a/b/c gives a/b/c, a/b and a, and the root
// itself, "", gives nothing.
func withParents(dir string) []string {
	var dirs []string
	for dir != "" {
		dirs = append(dirs, dir)
		i := strings.LastIndexByte(dir, '/')
		dir = dir[:max(i, 0)]
	}
	return dirs
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
// ship.
func readLicenses(fsys fs.FS, dir, what string) ([]licenseText, error) {
	texts, err := licenseTexts(fsys, dir, what)
	if err != nil {
		return nil, err
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("%s publishes no license file in %s", what, dir)
	}
	return texts, nil
}

// nestedLicenses reads the license, notice and patent files in each
// directory below a module's root that holds a linked package or a parent of
// one, in path order, each named for the file and the import path of the
// directory it sits in. Most directories hold none, and holding none is no
// failure there, unlike at a module's root; the ones that do carry code under
// a license of its own, like a project vendored into another under its
// original terms.
func nestedLicenses(root string, module *linkedModule) ([]licenseText, error) {
	var texts []licenseText
	for _, rel := range slices.Sorted(maps.Keys(module.dirs)) {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		found, err := licenseTexts(os.DirFS(dir), dir, module.path+" "+module.version)
		if err != nil {
			return nil, err
		}
		for _, t := range found {
			texts = append(texts, licenseText{name: t.name + " in " + module.path + "/" + rel, text: t.text})
		}
	}
	return texts, nil
}

// licenseTexts reads every license, notice and patent file at the root of
// fsys, which is the directory dir, in name order. A symbolic link or a
// directory is never a text, whatever its name.
func licenseTexts(fsys fs.FS, dir, what string) ([]licenseText, error) {
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
		// A replacement by a local directory is recorded with the version
		// (devel), which no module cache holds; one with no version at all is
		// refused alike, though a build does not record that.
		effPath, effVersion := module.effective()
		if effVersion == "" || effVersion == develVersion {
			return nil, fmt.Errorf("%s is replaced by the local directory %s, which no module cache holds", path, effPath)
		}
		dir := moduleDir(env.modcache, effPath, effVersion)
		texts, readErr := readLicenses(os.DirFS(dir), dir, path+" "+module.version)
		if readErr != nil {
			return nil, readErr
		}
		nested, nestedErr := nestedLicenses(dir, module)
		if nestedErr != nil {
			return nil, nestedErr
		}
		s := section{heading: path + " " + module.version, texts: append(texts, nested...)}
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
	// A build from a tree with no version control metadata, which is how the
	// image's builder sees its context, records its main module as (devel),
	// which names no version, so it is left out.
	if set.mainVersion == "" || set.mainVersion == develVersion {
		fmt.Fprintf(&b, "Module:    %s\n", set.mainPath)
	} else {
		fmt.Fprintf(&b, "Module:    %s %s\n", set.mainPath, set.mainVersion)
	}
	fmt.Fprintf(&b, "Toolchain: %s\n", set.goVersion)
	fmt.Fprintf(&b, "Builds:    %s\n\n", strings.Join(set.targetList(), ", "))
	b.WriteString(`gitlab-mcp-server is distributed under the MIT License, in the LICENSE
file that accompanies this one. Its binaries also contain the Go standard
library and the Go modules listed below, each distributed under its own
license. The standard library's license and patent grant, at the root of
the Go release, are reproduced below in full, and so is every license,
notice and patent file each module publishes, at its root and in the
directory of each package the binaries link or of a parent of one, read
from the module at the version the binaries link.
cmd/gen_third_party_notices writes this file from the build information
the binaries record and the packages the toolchain lists for each of them.

The source code of each module is available at the version listed, or
at the replacement's where one is named, from the Go module proxy: the
command go mod download -json <module>@<version> fetches it, from
https://proxy.golang.org unless GOPROXY names another. The source code
of the Go standard library is part of the Go release named under
Toolchain, published at https://go.dev/dl/.

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
