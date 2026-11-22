// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// stampServerJSON is the manifest the stamp writes first and most of.
const stampServerJSON = "server.json"

// stampManifests are the manifests whose version the stamp sets after
// server.json, each with the name its note gives it when the file is absent.
var stampManifests = [][2]string{
	{".plugin/plugin.json", "Open Plugins manifest"},
	{"plugin.json", "Agent Plugins manifest"},
	{"mcpb/manifest.json", "MCPB manifest"},
	{"lhm.plugin.json", "LobeHub manifest"},
}

// The rewrites of step 3, as jq's sub applied them: the first match of each
// is replaced.
var (
	latestDownload = regexp.MustCompile(`releases/latest/download`)
	pinnedDownload = regexp.MustCompile(`releases/download/v\d+\.\d+\.\d+(-[A-Za-z0-9.]+)?`)
	lastSegment    = regexp.MustCompile(`/[^/]*$`)
	ociDigest      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// claudePlatforms are the systems Claude Desktop reports, in the order the
// stamp asks for a bundle of each.
var claudePlatforms = []string{"darwin", "win32", "linux"}

// stampBundle is one .mcpb bundle the stamp declares.
type stampBundle struct {
	name, sha string
	platforms []*jsonValue
}

// stamp is one run of the stamp subcommand.
type stamp struct {
	root, version, digest string
	stdout, stderr        io.Writer
}

// sayf writes one line of progress.
func (s *stamp) sayf(format string, args ...any) { fmt.Fprintf(s.stdout, format+"\n", args...) }

// errorf writes one ERROR line and returns 1, the exit code of a refusal.
func (s *stamp) errorf(format string, args ...any) int {
	fmt.Fprintf(s.stderr, "ERROR: "+format+"\n", args...)
	return 1
}

// errorLines writes a refusal that runs over several lines, the first
// prefixed with ERROR and the rest indented under it, and returns 1.
func (s *stamp) errorLines(first string, rest ...string) int {
	fmt.Fprintf(s.stderr, "ERROR: %s\n", first)
	for _, line := range rest {
		fmt.Fprintf(s.stderr, "       %s\n", line)
	}
	return 1
}

// runStamp is the stamp subcommand, scripts/update-server-json-sha.sh moved
// into Go: it writes the release's version into server.json and the five
// manifests beside it, pins the release URLs and the OCI image digest, sets
// the hashes checksums.txt names, declares one mcpb entry per bundle given,
// and syncs the npm launcher, each file written as jq wrote it.
func runStamp(root string, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || len(args) > 4 {
		fmt.Fprintf(stderr, "%s: stamp takes <checksums-file> <version> [mcpb-files] [oci-digest]\n%s", toolName, usage)
		return 2
	}
	s := &stamp{root: root, version: args[1], digest: optional(args, 3, ""), stdout: stdout, stderr: stderr}
	return s.run(args[0], optional(args, 2, ""))
}

// run stamps the manifests in the script's order, writing server.json after
// each step as the script did, so a refusal part way leaves the file where
// the script left it.
func (s *stamp) run(checksums, mcpbFiles string) int {
	if !isRegularFile(localPath(s.root, checksums)) {
		return s.errorf("checksums file not found: %s", checksums)
	}
	serverFile := localPath(s.root, stampServerJSON)
	if !isRegularFile(serverFile) {
		return s.errorf("%s not found in current directory", stampServerJSON)
	}
	server, err := readJSONFile(serverFile)
	if err != nil {
		return s.errorf("%v", err)
	}
	var bundles []stampBundle
	if mcpbFiles != "" {
		if bundles, err = s.readBundles(mcpbFiles); err != nil {
			return 1
		}
		if code := s.checkBundles(server, bundles); code != 0 {
			return code
		}
	}
	steps := []func(*jsonValue) int{s.setVersions, s.pinIdentifiers, s.pinImages}
	for _, step := range steps {
		if code := step(server); code != 0 {
			return code
		}
		if err = writeFileMode(serverFile, server.pretty(), 0o644); err != nil {
			return s.errorf("%v", err)
		}
	}
	updated, err := s.setHashes(server, localPath(s.root, checksums))
	if err != nil {
		return s.errorf("%v", err)
	}
	updated += s.declareBundles(server, bundles, mcpbFiles != "")
	if err = writeFileMode(serverFile, server.pretty(), 0o644); err != nil {
		return s.errorf("%v", err)
	}
	s.sayf("Updated %d of %d package entries", updated, len(server.field("packages").items))
	if updated == 0 {
		return s.errorLines("no package got a hash. Binary checksums no longer match any",
			"identifier, and no .mcpb bundle was passed as the third argument.")
	}
	return s.stampOthers()
}

// isRegularFile reports whether name is a regular file, following links, as
// the shell's -f does.
func isRegularFile(name string) bool {
	info, err := os.Stat(name) //#nosec G703 -- a path the release workflow passes, under the checkout
	return err == nil && info.Mode().IsRegular()
}

// readJSONFile reads a manifest.
func readJSONFile(file string) (*jsonValue, error) {
	data, err := os.ReadFile(file) //#nosec G304 -- a manifest of the repository
	if err != nil {
		return nil, err
	}
	doc, err := parseJSON(data)
	if err != nil {
		return nil, fmt.Errorf("%s could not be read as JSON: %w", filepath.Base(file), err)
	}
	return doc, nil
}

// readBundles reads each bundle named, comma-separated as the shell's read
// split them, with the platforms its manifest.json declares and its SHA-256.
func (s *stamp) readBundles(list string) ([]stampBundle, error) {
	files := strings.Split(list, ",")
	if files[len(files)-1] == "" {
		files = files[:len(files)-1]
	}
	var bundles []stampBundle
	for _, file := range files {
		if !isRegularFile(localPath(s.root, file)) {
			s.errorf("mcpb bundle not found: %s", file)
			return nil, errRefused
		}
		name := filepath.Base(file)
		data, err := os.ReadFile(localPath(s.root, file)) //#nosec G304,G703 -- a bundle the release built
		if err != nil {
			return nil, err
		}
		platforms, ok := bundlePlatforms(data)
		if !ok {
			s.errorf("%s carries no manifest.json with a compatibility.platforms list this script can read", name)
			return nil, errRefused
		}
		bundles = append(bundles, stampBundle{name: name, sha: releaseassets.Digest(data), platforms: platforms})
	}
	return bundles, nil
}

// bundlePlatforms reads compatibility.platforms from a bundle's
// manifest.json, an absent or null list read as empty, as the script's
// `.compatibility.platforms // []` did.
func bundlePlatforms(data []byte) ([]*jsonValue, bool) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, false
	}
	for _, f := range archive.File {
		if f.Name != "manifest.json" {
			continue
		}
		raw, readErr := readZipEntry(f)
		if readErr != nil {
			return nil, false
		}
		manifest, parseErr := parseJSON(raw)
		if parseErr != nil {
			return nil, false
		}
		platforms := manifest.field("compatibility").field("platforms")
		switch {
		case !platforms.truthy():
			return []*jsonValue{}, true
		case platforms.kind == jsonArray:
			return platforms.items, true
		}
		return nil, false
	}
	return nil, false
}

// checkBundles refuses a set of bundles that is not one per operating
// system, and a server.json with no mcpb entry to copy, before anything is
// written.
func (s *stamp) checkBundles(server *jsonValue, bundles []stampBundle) int {
	var problems, names []string
	var served []*jsonValue
	for _, b := range bundles {
		names = append(names, b.name)
		served = append(served, b.platforms...)
	}
	problems = append(problems, givenTwice(names)...)
	for _, b := range bundles {
		if len(b.platforms) != 1 {
			list := jsonList(b.platforms...)
			problems = append(problems, fmt.Sprintf("%s serves %d platforms (%s), and a declared bundle serves exactly one",
				b.name, len(b.platforms), list.joinStrings(", ")))
		}
	}
	var unique []string
	for _, p := range served {
		unique = append(unique, p.interpolate())
	}
	sort.Strings(unique)
	for _, p := range slices.Compact(unique) {
		if !slices.Contains(claudePlatforms, p) {
			problems = append(problems, p+" is not a platform Claude Desktop reports")
		}
	}
	for _, platform := range claudePlatforms {
		count := 0
		for _, p := range served {
			if p.kind == jsonString && p.text == platform {
				count++
			}
		}
		switch {
		case count == 0:
			problems = append(problems, "no bundle given serves "+platform)
		case count > 1:
			problems = append(problems, fmt.Sprintf("%s is served by %d bundles", platform, count))
		}
	}
	if len(problems) > 0 {
		for _, problem := range problems {
			s.errorf("%s", problem)
		}
		return s.errorf("the bundles given are not one per operating system; %s was left as it was", stampServerJSON)
	}
	if !slices.ContainsFunc(server.field("packages").items, isMcpbEntry) {
		return s.errorf("%s declares no mcpb entry to take the bundles' transport, environment and release URL from", stampServerJSON)
	}
	return 0
}

// givenTwice names each bundle given more than once, once, in sorted order,
// as the script's group_by did.
func givenTwice(names []string) []string {
	sorted := slices.Sorted(slices.Values(names))
	var problems []string
	for i, name := range sorted {
		if i > 0 && sorted[i-1] == name && (i < 2 || sorted[i-2] != name) {
			problems = append(problems, name+" is given more than once")
		}
	}
	return problems
}

// isMcpbEntry reports whether a package entry is a .mcpb bundle.
func isMcpbEntry(pkg *jsonValue) bool { return text(pkg, "registryType") == "mcpb" }

// isOCIEntry reports whether a package entry is an image.
func isOCIEntry(pkg *jsonValue) bool { return text(pkg, "registryType") == "oci" }

// setVersions is steps 1 and 2: the top-level version, and the version of
// every package entry that declares one.
func (s *stamp) setVersions(server *jsonValue) int {
	server.set("version", jsonText(s.version))
	s.sayf("Top-level version set to %s", s.version)
	for _, pkg := range server.field("packages").items {
		if pkg.has("version") {
			pkg.set("version", jsonText(s.version))
		}
	}
	s.sayf("Per-package version fields set to %s", s.version)
	return 0
}

// replaceFirst replaces the first match of pattern in s, as jq's sub does.
func replaceFirst(pattern *regexp.Regexp, s, replacement string) string {
	match := pattern.FindStringIndex(s)
	if match == nil {
		return s
	}
	return s[:match[0]] + replacement + s[match[1]:]
}

// pinIdentifiers is step 3: every release URL pinned to this version,
// whether it named latest or an earlier version.
func (s *stamp) pinIdentifiers(server *jsonValue) int {
	release := "releases/download/v" + s.version
	for _, pkg := range server.field("packages").items {
		if pkg.field("identifier").kind != jsonString {
			continue
		}
		identifier := replaceFirst(latestDownload, text(pkg, "identifier"), release)
		pkg.set("identifier", jsonText(replaceFirst(pinnedDownload, identifier, release)))
	}
	s.sayf("Identifiers pinned to v%s", s.version)
	return 0
}

// pinImages is step 3b: every OCI reference becomes <repo>:<version>, with
// the digest given appended, and a digest-pinned reference refuses a run
// that gives none rather than leave the previous release's image under the
// new tag.
func (s *stamp) pinImages(server *jsonValue) int {
	packages := server.field("packages").items
	if !slices.ContainsFunc(packages, isOCIEntry) {
		return 0
	}
	pinned := slices.ContainsFunc(packages, func(pkg *jsonValue) bool {
		return isOCIEntry(pkg) && strings.Contains(text(pkg, "identifier"), "@")
	})
	if pinned && s.digest == "" {
		return s.errorLines("an OCI identifier is digest-pinned but no digest was passed as the",
			"fourth argument. Stamping the tag alone would leave the previous",
			"release's image pinned under the new version.")
	}
	if s.digest != "" && !ociDigest.MatchString(s.digest) {
		return s.errorf("oci-digest must look like sha256:<64 hex chars> (got: %s)", s.digest)
	}
	suffix := ""
	if s.digest != "" {
		suffix = "@" + s.digest
	}
	for _, pkg := range packages {
		if !isOCIEntry(pkg) {
			continue
		}
		reference, _, _ := strings.Cut(text(pkg, "identifier"), "@")
		if colon := strings.LastIndex(reference, ":"); colon >= 0 {
			reference = reference[:colon]
		}
		pkg.set("identifier", jsonText(reference+":"+s.version+suffix))
	}
	for _, pkg := range packages {
		if isOCIEntry(pkg) {
			s.sayf("OCI image reference pinned to %s", text(pkg, "identifier"))
		}
	}
	return 0
}

// setHashes is step 4: every package whose identifier ends with a name
// checksums.txt lists takes its hash. Lines are read as the shell's read
// split them, and a last line with no newline is not read at all.
func (s *stamp) setHashes(server *jsonValue, checksums string) (int, error) {
	data, err := os.ReadFile(checksums) //#nosec G304,G703 -- the release's checksums.txt
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if !strings.HasSuffix(line, "\n") {
			break
		}
		hash, filename := readFields(line)
		if hash == "" || filename == "" {
			continue
		}
		matched := false
		for _, pkg := range server.field("packages").items {
			if strings.HasSuffix(text(pkg, "identifier"), filename) {
				pkg.set("fileSha256", jsonText(hash))
				matched = true
			}
		}
		if matched {
			s.sayf("SHA256 for %s: %s...", filename, firstRunes(hash, 16))
			updated++
		}
	}
	return updated, nil
}

// readFields splits a line as the shell's `read -r first rest` does with
// the default IFS: the first field, and the rest with the blanks around it
// removed.
func readFields(line string) (first, rest string) {
	trimmed := strings.Trim(line, " \t\n")
	end := strings.IndexAny(trimmed, " \t")
	if end < 0 {
		return trimmed, ""
	}
	return trimmed[:end], strings.TrimLeft(trimmed[end:], " \t")
}

// firstRunes is the first n characters of s, as the shell's ${s:0:n}.
func firstRunes(s string, n int) string {
	if runes := []rune(s); len(runes) > n {
		return string(runes[:n])
	}
	return s
}

// declareBundles is step 4b: server.json ends up with one mcpb entry per
// bundle given, each a copy of the first mcpb entry it held with the
// bundle's identifier and hash, in that entry's place.
func (s *stamp) declareBundles(server *jsonValue, bundles []stampBundle, given bool) int {
	if !given {
		s.sayf("NOTE: no mcpb bundle given, leaving the declared bundles untouched")
		return 0
	}
	packages := server.field("packages")
	first := slices.IndexFunc(packages.items, isMcpbEntry)
	template := packages.items[first]
	release := replaceFirst(lastSegment, text(template, "identifier"), "")
	declared := slices.Clone(packages.items[:first])
	for _, b := range bundles {
		entry := template.clone()
		entry.set("identifier", jsonText(release+"/"+b.name))
		entry.set("fileSha256", jsonText(b.sha))
		declared = append(declared, entry)
	}
	for _, pkg := range packages.items[first:] {
		if !isMcpbEntry(pkg) {
			declared = append(declared, pkg)
		}
	}
	packages.items = declared
	for _, b := range bundles {
		s.sayf("SHA256 for %s: %s...", b.name, firstRunes(b.sha, 16))
	}
	return len(bundles)
}

// stampOthers is steps 5 to 8: the version of each manifest beside
// server.json that the tree holds, and the npm launcher's version and pins.
func (s *stamp) stampOthers() int {
	for _, manifest := range stampManifests {
		file := localPath(s.root, manifest[0])
		if !isRegularFile(file) {
			s.sayf("NOTE: %s not found, skipping %s update", manifest[0], manifest[1])
			continue
		}
		doc, err := readJSONFile(file)
		if err != nil {
			return s.errorf("%v", err)
		}
		doc.set("version", jsonText(s.version))
		if err = writeFileMode(file, doc.pretty(), 0o644); err != nil {
			return s.errorf("%v", err)
		}
		s.sayf("%s version set to %s", manifest[0], s.version)
	}
	npmMain := npmMainDir + "/package.json"
	if !isRegularFile(localPath(s.root, npmMain)) {
		s.sayf("NOTE: %s not found, skipping npm manifest update", npmMain)
		return 0
	}
	if err := syncNpmMain(s.root, s.version); err != nil {
		return s.errorf("%v", err)
	}
	s.sayf("npm main package synced to v%s (%s)", s.version, npmMainName)
	return 0
}
