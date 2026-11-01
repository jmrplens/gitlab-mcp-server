// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

// creatorSystems are the names zipinfo gives the host systems a zip entry
// can be recorded under, by the number the archive stores.
var creatorSystems = map[uint16]string{0: "fat", 3: "unx", 7: "mac", 10: "ntf", 19: "osx"}

// checkBundle reads back the archive of one bundle, since the archive is what
// ships: exactly the expected entries, in order; each executable recorded as
// a Unix file with mode 0755, since a zip written without Unix attributes
// extracts every file 0600 in Claude Desktop; and a manifest that can start
// on every platform it lists.
func (b *mcpbBuild) checkBundle(t bundleTarget) {
	output := b.output(t)
	archive, err := zip.OpenReader(b.local(output))
	if err != nil {
		b.failf("%s: the archive could not be read: %v", output, err)
		return
	}
	defer archive.Close()

	carried := make([]string, 0, len(archive.File))
	byName := map[string]*zip.File{}
	for _, file := range archive.File {
		carried = append(carried, file.Name)
		byName[file.Name] = file
	}
	if !slices.Equal(carried, t.entries) {
		b.failf("%s: the archive does not carry exactly the expected entries", output)
		writeEntryDiff(b.stderr, t.entries, carried)
	}

	for _, entry := range t.entries {
		file, ok := byName[entry]
		if !ok || !executable(entry) {
			continue
		}
		if recorded := recordedMode(file); recorded != "-rwxr-xr-x unx" {
			b.failf("%s: %s is recorded as '%s', expected '-rwxr-xr-x unx'", output, entry, recorded)
		}
	}

	file, ok := byName["manifest.json"]
	if !ok {
		return
	}
	manifest, err := readManifest(file)
	var problems []string
	if err == nil {
		problems, err = manifestProblems(manifest, b.version, t)
	}
	if err != nil {
		b.failf("%s: the packed manifest.json could not be read and checked", output)
		return
	}
	for _, problem := range problems {
		b.failf("%s: %s", output, problem)
	}
}

// recordedMode is an entry's mode and host system as zipinfo prints them.
func recordedMode(file *zip.File) string {
	system, ok := creatorSystems[file.CreatorVersion>>8]
	if !ok {
		system = fmt.Sprintf("system %d", file.CreatorVersion>>8)
	}
	return file.Mode().String() + " " + system
}

// writeEntryDiff prints the entries a bundle should carry and does not, and
// those it carries and should not, one per line in the shape diff gives them.
func writeEntryDiff(out io.Writer, expected, carried []string) {
	for _, entry := range expected {
		if !slices.Contains(carried, entry) {
			fmt.Fprintf(out, "< %s\n", entry)
		}
	}
	for _, entry := range carried {
		if !slices.Contains(expected, entry) {
			fmt.Fprintf(out, "> %s\n", entry)
		}
	}
	if slices.Equal(sorted(expected), sorted(carried)) {
		fmt.Fprintf(out, "< %s\n> %s\n", strings.Join(expected, " "), strings.Join(carried, " "))
	}
}

// sorted returns a sorted copy.
func sorted(values []string) []string {
	copied := slices.Clone(values)
	sort.Strings(copied)
	return copied
}

// readManifest reads and parses an archive's manifest.json.
func readManifest(file *zip.File) (*jsonValue, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return parseJSON(data)
}

// manifestProblems checks what a packed manifest says against what its
// archive carries. Claude Desktop resolves platform_overrides[process.platform]
// and substitutes ${__dirname} with the extension directory, so a path the
// manifest names and the archive lacks is a server that cannot start on that
// platform. Two rules the schema cannot express come from the same resolver:
// an override's env replaces the base env rather than merging with it, which
// would drop GITLAB_TOKEN and every other setting, and an override's args
// replace the base args.
//
// The platform rules run both ways. Every override has to be listed in
// compatibility.platforms, and every listed platform other than the one the
// base command serves has to have an override, since a platform without one
// would be handed the base command: the macOS binary in the universal
// bundle. The list has to name exactly the platforms the archive carries a
// server for: Desktop marks the bundle incompatible on a platform the list
// leaves out, and on one it lists that the archive has no server for it would
// start a command that is not there.
//
// The problems come in the order and the words the jq program of
// scripts/build-mcpb.sh gave them, which this replaces. A manifest of a shape
// that program could not read at all, an override that is not an object for
// one, is an error rather than a list.
func manifestProblems(manifest *jsonValue, version string, t bundleTarget) ([]string, error) {
	shape, err := readLaunch(manifest)
	if err != nil {
		return nil, err
	}
	problems := pathProblems(manifest, shape, version, t)
	problems = append(problems, platformProblems(shape, t)...)
	return append(problems, overrideProblems(shape)...), nil
}

// launch is the part of a manifest that says how a host starts the server:
// the base mcp_config, its overrides (an object of objects), the platforms
// it lists, and every word of the command lines that names a path under
// ${__dirname}.
type launch struct {
	config, overrides *jsonValue
	platforms         []*jsonValue
	named             []string
}

// readLaunch reads a manifest's launch, or refuses a shape the jq program
// could not read: a server, a config or a compatibility that is not an
// object, overrides or an override that is not one, a platform list that is
// not a list, or args that cannot be iterated.
func readLaunch(manifest *jsonValue) (launch, error) {
	config, err := index(manifest, "server", "mcp_config")
	if err != nil {
		return launch{}, err
	}
	overrides, err := index(config, "platform_overrides")
	if err != nil {
		return launch{}, err
	}
	if !overrides.truthy() {
		overrides = &jsonValue{kind: jsonObject}
	}
	listed, err := index(manifest, "compatibility", "platforms")
	if err != nil {
		return launch{}, err
	}
	if !listed.truthy() {
		listed = jsonList()
	}
	if overrides.kind != jsonObject || listed.kind != jsonArray {
		return launch{}, fmt.Errorf("platform_overrides is %s and compatibility.platforms %s", overrides.typeName(), listed.typeName())
	}
	for _, member := range overrides.members {
		if member.value.kind != jsonObject {
			return launch{}, fmt.Errorf("platform_overrides.%s is %s", member.key, member.value.typeName())
		}
	}
	named, err := namedPaths(config, overrides)
	if err != nil {
		return launch{}, err
	}
	return launch{config: config, overrides: overrides, platforms: listed.items, named: named}, nil
}

// overrideKeys are the platforms a launch overrides, sorted as jq's keys
// sorts them.
func (l launch) overrideKeys() []string {
	keys := make([]string, 0, len(l.overrides.members))
	for _, member := range l.overrides.members {
		keys = append(keys, member.key)
	}
	return sorted(keys)
}

// lists reports whether the launch lists a platform.
func (l launch) lists(platform string) bool {
	return slices.ContainsFunc(l.platforms, func(p *jsonValue) bool { return p.equal(jsonText(platform)) })
}

// pathProblems holds the version and every path the manifest names to the
// archive: the entry point, the icon when there is one, and each path a
// command line names under ${__dirname}.
func pathProblems(manifest *jsonValue, l launch, version string, t bundleTarget) []string {
	var problems []string
	carries := func(v *jsonValue) bool { return v.kind == jsonString && slices.Contains(t.entries, v.text) }
	if got := manifest.field("version"); !got.equal(jsonText(version)) {
		problems = append(problems, fmt.Sprintf("manifest version is %s, expected %s", got.interpolate(), version))
	}
	if entryPoint := manifest.field("server").field("entry_point"); !carries(entryPoint) {
		problems = append(problems, fmt.Sprintf("server.entry_point %s is not in the archive", entryPoint.interpolate()))
	}
	if icon := manifest.field("icon"); icon.kind != jsonNull && !carries(icon) {
		problems = append(problems, fmt.Sprintf("icon %s is not in the archive", icon.interpolate()))
	}
	for _, path := range l.named {
		rel, leading := strings.CutPrefix(path, dirname)
		switch {
		case !leading:
			problems = append(problems, path+" uses ${__dirname} other than as a leading path component, so it cannot be checked")
		case !slices.Contains(t.entries, rel):
			problems = append(problems, fmt.Sprintf("%s names %s, which is not in the archive", path, rel))
		}
	}
	return problems
}

// overridePrefix is how a problem names one entry of the manifest's
// platform_overrides, by its key.
const overridePrefix = "platform_overrides."

// platformProblems holds the overrides and the platform list to each other
// and to the platforms the archive carries a server for.
func platformProblems(l launch, t bundleTarget) []string {
	var problems []string
	keys := l.overrideKeys()
	for _, key := range keys {
		if !slices.Contains(desktopPlatforms, key) {
			problems = append(problems, overridePrefix+key+" is not a platform Claude Desktop reports (darwin, win32 or linux)")
		}
	}
	for _, key := range keys {
		if !l.lists(key) {
			problems = append(problems, overridePrefix+key+" is not listed in compatibility.platforms")
		}
	}
	base := jsonText(t.basePlatform())
	for _, platform := range l.platforms {
		overridden := slices.ContainsFunc(keys, func(key string) bool { return platform.equal(jsonText(key)) })
		if !platform.equal(base) && !overridden {
			problems = append(problems, fmt.Sprintf("compatibility.platforms lists %s with no platform_overrides entry, so it would start the base command, which serves %s", platform.interpolate(), t.basePlatform()))
		}
	}
	for _, platform := range t.platforms {
		if !l.lists(platform) {
			problems = append(problems, "compatibility.platforms does not list "+platform+", although the archive carries its server")
		}
	}
	for _, platform := range l.platforms {
		if platform.kind != jsonString || !slices.Contains(t.platforms, platform.text) {
			problems = append(problems, "compatibility.platforms lists "+platform.interpolate()+", although the archive carries no server for it")
		}
	}
	return problems
}

// overrideProblems holds each override to what Claude Desktop's resolver does
// with it: its env replaces the base env, and its args replace base args
// that are not empty.
func overrideProblems(l launch) []string {
	var problems []string
	for _, member := range l.overrides.members {
		if member.value.has("env") {
			problems = append(problems, overridePrefix+member.key+" declares env, which would replace the base env")
		}
	}
	// The base args were iterated by readLaunch, so they are an array or an
	// object, and jq's length of either is how many it holds.
	if baseArgs := l.config.field("args"); len(baseArgs.items)+len(baseArgs.members) > 0 {
		for _, member := range l.overrides.members {
			if member.value.has("args") {
				problems = append(problems, overridePrefix+member.key+" declares args, which would replace the non-empty base args")
			}
		}
	}
	return problems
}

// namedPaths are the words of the base command line and of each override's,
// in that order, that are strings carrying ${__dirname}.
func namedPaths(config, overrides *jsonValue) ([]string, error) {
	lines := []*jsonValue{config}
	for _, member := range overrides.members {
		lines = append(lines, member.value)
	}
	var named []string
	for _, line := range lines {
		words, err := commandLine(line)
		if err != nil {
			return nil, err
		}
		for _, word := range words {
			if word.kind == jsonString && strings.Contains(word.text, "${__dirname}") {
				named = append(named, word.text)
			}
		}
	}
	return named, nil
}
