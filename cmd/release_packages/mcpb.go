// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The repository files every bundle is assembled from, relative to the
// repository root the command runs from.
const (
	manifestPath = "mcpb/manifest.json"
	iconPath     = "mcpb/icon.png"
	launcherPath = "mcpb/linux/launch.sh"
	// licensePath travels with the binaries it covers: a bundle is a
	// redistribution of the server, and MIT asks for its notice to accompany
	// every copy.
	licensePath = "LICENSE"
)

// noticesName is the notices of the modules the binaries link, which
// GoReleaser writes beside the release binaries and `make mcpb` beside its
// own (cmd/gen_third_party_notices), and noticesHeader the line that file
// opens with.
const (
	noticesName   = "THIRD_PARTY_NOTICES"
	noticesHeader = "Third-party notices for gitlab-mcp-server"
)

// The entries a bundle can carry, in archive order. manifest.json comes
// first, so a reader that streams the archive finds the manifest before the
// multi-megabyte binaries.
var (
	commonEntries  = []string{"manifest.json", "icon.png", "LICENSE", noticesName}
	darwinEntries  = []string{"server/gitlab-mcp-server"}
	windowsEntries = []string{"server/gitlab-mcp-server.exe"}
	linuxEntries   = []string{
		"server/linux/launch.sh",
		"server/linux/gitlab-mcp-server-linux-amd64",
		"server/linux/gitlab-mcp-server-linux-arm64",
	}
)

// bundleTarget is one bundle a run builds: its file name, the platform its
// per-OS manifest is derived for (empty for the universal bundle, which packs
// the committed manifest), the platforms its manifest has to list, exactly,
// and the entries it carries.
type bundleTarget struct {
	name      string
	platform  string
	platforms []string
	entries   []string
	// declared is whether server.json declares the bundle; the universal one
	// is kept for the links to it that exist outside this repository.
	declared bool
}

// basePlatform is the platform the manifest's base command serves, the one
// listed platform that needs no override.
func (t bundleTarget) basePlatform() string {
	if t.platform == "" {
		return "darwin"
	}
	return t.platform
}

// bundleTargets are the four bundles one run builds, the per-OS ones first
// so their sizes lead the report. The three per-OS ones are what server.json
// declares, one registry entry each; the universal one keeps the name it has
// always had.
func bundleTargets() []bundleTarget {
	join := func(parts ...[]string) []string { return slices.Concat(append([][]string{commonEntries}, parts...)...) }
	return []bundleTarget{
		{name: "gitlab-mcp-server-darwin.mcpb", platform: "darwin", platforms: []string{"darwin"}, entries: join(darwinEntries), declared: true},
		{name: "gitlab-mcp-server-windows.mcpb", platform: "win32", platforms: []string{"win32"}, entries: join(windowsEntries), declared: true},
		{name: "gitlab-mcp-server-linux.mcpb", platform: "linux", platforms: []string{"linux"}, entries: join(linuxEntries), declared: true},
		{name: "gitlab-mcp-server.mcpb", platforms: desktopPlatforms, entries: join(darwinEntries, windowsEntries, linuxEntries)},
	}
}

// executable reports whether an entry is stored with mode 0755. Claude
// Desktop extracts every file 0600 and restores the execute bit only for an
// entry whose recorded mode has owner-execute, so every server entry carries
// it, the Windows executable too, which Windows ignores.
func executable(entry string) bool { return strings.HasPrefix(entry, "server/") }

// binarySearch is one release binary the bundles carry: the entry it is
// packed as, a part of the path that names its build directory, and its
// file name. GoReleaser's directories are <id>_<goos>_<goarch>[_<variant>]
// (gitlab-mcp-server_linux_amd64_v1), `make mcpb`'s are local_<goos>_<goarch>,
// and the darwin universal binary comes from the universal_binaries step,
// whose goarch is all.
type binarySearch struct {
	entry, part, name string
}

// binarySearches are the four release binaries, in the order they are
// looked for.
var binarySearches = []binarySearch{
	{entry: "server/gitlab-mcp-server", part: "darwin_all", name: "gitlab-mcp-server"},
	{entry: "server/gitlab-mcp-server.exe", part: "windows_amd64", name: "gitlab-mcp-server.exe"},
	{entry: "server/linux/gitlab-mcp-server-linux-amd64", part: "linux_amd64", name: "gitlab-mcp-server"},
	{entry: "server/linux/gitlab-mcp-server-linux-arm64", part: "linux_arm64", name: "gitlab-mcp-server"},
}

// mcpbBuild is one run of the mcpb subcommand.
type mcpbBuild struct {
	// root is the directory the repository paths are read from, and dist the
	// output directory as given, relative to root unless absolute, and as the
	// messages name it.
	root, dist string
	version    string
	stdout     io.Writer
	stderr     io.Writer
	getenv     func(string) string
	// writeArchive and measure are the archive writer and the size reader,
	// behind seams so a test can drop an entry or report sizes no stand-in
	// could reach.
	writeArchive func(path string, files []bundleFile) error
	measure      func(path string) (bundleSizes, error)

	failures int
	rows     []string
}

// newMcpbBuild returns a run over the repository at root with the real
// archive writer and size reader.
func newMcpbBuild(root, dist, version string, stdout, stderr io.Writer, getenv func(string) string) *mcpbBuild {
	return &mcpbBuild{
		root: root, dist: dist, version: version, stdout: stdout, stderr: stderr, getenv: getenv,
		writeArchive: writeBundle, measure: measureBundle,
	}
}

// local is a path named relative to the repository root, on disk.
func (b *mcpbBuild) local(rel string) string { return localPath(b.root, rel) }

// output is where a bundle is written, as the messages name it.
func (b *mcpbBuild) output(t bundleTarget) string { return b.dist + "/" + t.name }

// failf reports one check a bundle failed, on stderr, and counts it.
func (b *mcpbBuild) failf(format string, args ...any) {
	fmt.Fprintf(b.stderr, "ERROR: "+format+"\n", args...)
	b.failures++
}

// warnf reports a size that is not a refusal: a workflow command on stdout
// in GitHub Actions, which makes it an annotation on the run's summary, and
// a line on stderr anywhere else.
func (b *mcpbBuild) warnf(format string, args ...any) {
	if b.getenv("GITHUB_ACTIONS") == "true" {
		fmt.Fprintf(b.stdout, "::warning::"+format+"\n", args...)
		return
	}
	fmt.Fprintf(b.stderr, "WARNING: "+format+"\n", args...)
}

// run builds, checks and measures the four bundles and returns the exit
// code. The previous run's bundles are removed before anything can refuse
// this run, so a refusal on any path does not leave them in dist/ under the
// old version for a later step or a developer to take for this one, and
// every bundle is removed again when any check fails: a bundle that did not
// pass its own checks must not be left behind, and they go as a set, since
// the release declares three of them and a later step handed two would
// publish a registry entry short of a system.
func (b *mcpbBuild) run() int {
	targets := bundleTargets()
	b.removeOutputs(targets)
	if !b.buildAll(targets) {
		b.removeOutputs(targets)
		return 1
	}
	fmt.Fprintf(b.stdout, "Built %d bundles (version %s)\n", len(targets), b.version)
	for _, t := range targets {
		b.list(t)
	}
	if err := b.writeSummary(); err != nil {
		fmt.Fprintf(b.stderr, "ERROR: the job summary could not be written: %v\n", err)
		return 1
	}
	return 0
}

// removeOutputs removes every bundle a run writes.
func (b *mcpbBuild) removeOutputs(targets []bundleTarget) {
	for _, t := range targets {
		_ = os.Remove(b.local(b.output(t))) //#nosec G703 -- a bundle name of this command's own under the dist directory the operator named
	}
}

// buildAll reads the inputs and packs, checks and measures each bundle in
// turn, and reports whether every one passed.
func (b *mcpbBuild) buildAll(targets []bundleTarget) bool {
	sources, manifest, ok := b.inputs()
	if !ok {
		return false
	}
	for _, t := range targets {
		if err := b.pack(t, sources, manifest); err != nil {
			fmt.Fprintf(b.stderr, "ERROR: %s: %v\n", b.output(t), err)
			return false
		}
		before := b.failures
		b.checkBundle(t)
		b.checkSize(t)
		if b.failures > before {
			fmt.Fprintf(b.stderr, "ERROR: %s failed %d check(s)\n", b.output(t), b.failures-before)
		}
	}
	if b.failures > 0 {
		fmt.Fprintf(b.stderr, "ERROR: the bundles failed %d check(s) in all, and the whole set was removed\n", b.failures)
		return false
	}
	return true
}

// inputs checks that every repository file, the notices and the four
// binaries are there, and returns where each entry is packed from and the
// committed manifest, or reports the first one missing.
func (b *mcpbBuild) inputs() (map[string]string, *jsonValue, bool) {
	for _, rel := range []string{manifestPath, iconPath, launcherPath, licensePath} {
		if !regularFile(b.local(rel)) {
			fmt.Fprintf(b.stderr, "ERROR: %s not found (run from the repository root)\n", rel)
			return nil, nil, false
		}
	}
	notices := b.dist + "/" + noticesName
	if !regularFile(b.local(notices)) {
		fmt.Fprintf(b.stderr, "ERROR: %s not found: GoReleaser writes it beside the release binaries and make mcpb beside its own (cmd/gen_third_party_notices)\n", notices)
		return nil, nil, false
	}
	// The first line without its line break, as head -n 1 prints it.
	text, err := os.ReadFile(b.local(notices)) //#nosec G304,G703 -- the notices under the dist directory the operator named, read and never written
	if first, _, _ := bytes.Cut(text, []byte("\n")); err != nil || string(first) != noticesHeader {
		fmt.Fprintf(b.stderr, "ERROR: %s does not open with '%s', so it is not what cmd/gen_third_party_notices writes\n", notices, noticesHeader)
		return nil, nil, false
	}
	sources := map[string]string{
		"icon.png":               b.local(iconPath),
		"LICENSE":                b.local(licensePath),
		noticesName:              b.local(notices),
		"server/linux/launch.sh": b.local(launcherPath),
	}
	for _, search := range binarySearches {
		found, ok := b.findBinary(search)
		if !ok {
			return nil, nil, false
		}
		sources[search.entry] = found
	}
	data, err := os.ReadFile(b.local(manifestPath)) //#nosec G304,G703 -- the committed manifest of the tree the command runs in
	var manifest *jsonValue
	if err == nil {
		manifest, err = parseJSON(data)
	}
	if err == nil && manifest.kind != jsonObject {
		err = fmt.Errorf("it holds %s rather than an object", manifest.typeName())
	}
	if err != nil {
		fmt.Fprintf(b.stderr, "ERROR: %s could not be read as JSON: %v\n", manifestPath, err)
		return nil, nil, false
	}
	return sources, manifest, true
}

// findBinary returns the one regular file under dist named as the search
// names it whose path holds its part, and reports none or more than one.
// Exactly one is accepted: with two, which one a walk finds first is up to
// the filesystem, and the bundle would carry whichever it was.
func (b *mcpbBuild) findBinary(search binarySearch) (string, bool) {
	var found []string
	_ = filepath.WalkDir(b.local(b.dist), func(path string, entry fs.DirEntry, err error) error { //#nosec G703 -- the dist directory the operator named, only listed
		if err == nil && entry.Type().IsRegular() && entry.Name() == search.name &&
			strings.Contains(b.shown(path), search.part) {
			found = append(found, path)
		}
		return nil
	})
	pattern := "*" + search.part + "*"
	switch len(found) {
	case 1:
		return found[0], true
	case 0:
		fmt.Fprintf(b.stderr, "ERROR: no %s matching %s under %s; run GoReleaser or make mcpb first\n", search.name, pattern, b.dist)
	default:
		fmt.Fprintf(b.stderr, "ERROR: %d files named %s match %s under %s; remove the stale ones:\n", len(found), search.name, pattern, b.dist)
		for _, path := range found {
			fmt.Fprintln(b.stderr, b.shown(path))
		}
	}
	return "", false
}

// shown is a path the walk of dist found as the messages name it: below
// dist as given.
func (b *mcpbBuild) shown(path string) string {
	rel, _ := filepath.Rel(b.local(b.dist), path)
	return b.dist + "/" + filepath.ToSlash(rel)
}

// pack writes one bundle: its manifest, stamped with the version and derived
// for its platform when it has one, and every other entry copied from where
// sources says, each with its mode set rather than inherited.
func (b *mcpbBuild) pack(t bundleTarget, sources map[string]string, committed *jsonValue) error {
	manifest := committed
	if t.platform != "" {
		derived, err := derivePlatformManifest(committed, t.platform)
		if err != nil {
			return err
		}
		manifest = derived
	}
	stamped := stampVersion(manifest, b.version)
	files := make([]bundleFile, 0, len(t.entries))
	for _, entry := range t.entries {
		file := bundleFile{name: entry, mode: 0o644, source: sources[entry]}
		if entry == "manifest.json" {
			file.data = stamped.pretty()
		}
		if executable(entry) {
			file.mode = 0o755
		}
		files = append(files, file)
	}
	return b.writeArchive(b.local(b.output(t)), files)
}

// regularFile reports whether path is a regular file, following a symbolic
// link as the shell's -f test does.
func regularFile(path string) bool {
	info, err := os.Stat(path) //#nosec G703 -- an input of the tree or the dist directory the operator named, only examined
	return err == nil && info.Mode().IsRegular()
}
