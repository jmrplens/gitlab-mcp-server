// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// extensions are the operating systems this command rebuilds for, each with
// the extension GoReleaser gives its binaries. Another system's binary may
// carry a name this command would get wrong, so it is refused.
var extensions = map[string]string{"linux": "", "darwin": "", "windows": ".exe"}

// levels are the architectures this command rebuilds for, each with the
// level GoReleaser builds it for when a configuration names none (GOAMD64
// and GOARM64). Another architecture's default is not written down here, so
// it is refused rather than guessed.
var levels = map[string]string{"amd64": "v1", "arm64": "v8.0"}

// target is one operating system and architecture a release builds.
type target struct {
	goos, goarch string
}

// String names the target the way the go command and GoReleaser both do.
func (t target) String() string {
	return t.goos + "/" + t.goarch
}

// level is the microarchitecture level the target is built for.
func (t target) level() string {
	return levels[t.goarch]
}

// ext is the extension of the target's binary.
func (t target) ext() string {
	return extensions[t.goos]
}

// distDir is the directory of dist/ GoReleaser writes the target's binary of
// build id into: the id, the platform and the level, joined by underscores.
func (t target) distDir(id string) string {
	return id + "_" + t.goos + "_" + t.goarch + "_" + t.level()
}

// env is the environment GoReleaser sets for the target, after the build's
// own (internal/builders/golang, Target.env): the platform, and every level
// variable it knows, empty except the target's own.
func (t target) env() []string {
	level := map[string]string{t.goarch: t.level()}
	return []string{
		"GOOS=" + t.goos, "GOARCH=" + t.goarch, "GOAMD64=" + level["amd64"], "GO386=", "GOARM=",
		"GOARM64=" + level["arm64"], "GOMIPS=", "GOMIPS64=", "GOPPC64=", "GORISCV64=",
	}
}

// build is one entry of the builds list: what it builds, with what, and for
// which targets. The env, flags, ldflags and binary are GoReleaser templates,
// rendered per target.
type build struct {
	id, main, binary    string
	env, flags, ldflags []string
	overrides           []override
	targets             []target
}

// override replaces a build's ldflags for the targets it selects.
type override struct {
	goos, goarch, goamd64, goarm64 string
	ldflags                        []string
}

// matches reports whether the override applies to t, as GoReleaser decides
// it: the platform is the target's, and the level the override names for the
// target's architecture, when it names one, is the target's.
func (o override) matches(t target) bool {
	if o.goos != t.goos || o.goarch != t.goarch {
		return false
	}
	named := map[string]string{"amd64": o.goamd64, "arm64": o.goarm64}[t.goarch]
	return named == "" || named == t.level()
}

// universal is one entry of universal_binaries: a darwin file joining the
// darwin binaries of the builds ids names.
type universal struct {
	id, name string
	ids      []string
}

// release is what a GoReleaser configuration says about the binaries a
// release publishes and the file that lists them.
type release struct {
	builds     []build
	universals []universal
	// archive is the name template of the one archive, which publishes each
	// binary as it is under the name it renders.
	archive string
	// checksums is the name template of the checksum file.
	checksums string
}

// usesShortCommit reports whether a template a build renders names
// {{.ShortCommit}}, whose value depends on the clone that renders it.
func (r release) usesShortCommit() bool {
	return slices.ContainsFunc(r.builds, func(b build) bool {
		texts := slices.Concat([]string{b.binary}, b.env, b.flags, b.ldflags)
		for _, o := range b.overrides {
			texts = append(texts, o.ldflags...)
		}
		return slices.ContainsFunc(texts, func(text string) bool { return strings.Contains(text, "ShortCommit") })
	})
}

// configFile is the part of .goreleaser.yml this command reads.
//
// Each section is kept as nodes so its keys can be held to the list this
// command reads before it is decoded: a decode drops every key it has no
// field for, and a key it drops may change what GoReleaser builds. The global
// env, the gomod section and the before hooks are read only to be refused,
// since each reaches every build. Every other top-level key is collected in
// Other, so it is held to passedKeys rather than dropped.
type configFile struct {
	Env               yaml.Node            `yaml:"env"`
	Gomod             yaml.Node            `yaml:"gomod"`
	Before            yaml.Node            `yaml:"before"`
	Builds            []yaml.Node          `yaml:"builds"`
	UniversalBinaries []yaml.Node          `yaml:"universal_binaries"`
	Archives          []yaml.Node          `yaml:"archives"`
	Checksum          yaml.Node            `yaml:"checksum"`
	Other             map[string]yaml.Node `yaml:",inline"`
}

// passedKeys are the top-level sections this command reads past: the schema
// version, the naming of a snapshot run, which no release is, the changelog,
// the signatures, the SBOMs and notices, and the GitHub release. Each writes
// files beside the binaries or describes the release rather than building or
// changing a binary. Every other top-level key is refused: GoReleaser accepts
// no key it does not define, and one it defines may rewrite a binary after it
// is built (upx compresses it, notarize signs the darwin ones in place) or
// change what every build is given (dist, git, includes), so a key not judged
// here is not guessed at.
var passedKeys = map[string]bool{
	"version": true, "snapshot": true, "changelog": true, "signs": true, "sboms": true, "release": true,
}

// beforeHook is the one before hook a configuration may run. It fetches the
// modules go.mod already names and changes nothing a build compiles.
const beforeHook = "go mod download"

// buildKeys are the keys a builds entry may carry: what this command reads,
// and mod_timestamp, which sets the file's time and not its bytes. Every
// other key can change what GoReleaser builds (tags, gcflags, buildmode, dir,
// tool, goamd64, ignore, targets, hooks and the next one GoReleaser adds), so
// it is refused.
var buildKeys = map[string]bool{
	"id": true, "main": true, "binary": true, "env": true, "flags": true, "goos": true, "goarch": true,
	"ldflags": true, "overrides": true, "mod_timestamp": true,
}

// overrideKeys are the keys an override may carry: what selects its targets,
// and the ldflags it replaces. An override setting env, flags or tags would
// change a build in a way this command does not apply.
var overrideKeys = map[string]bool{"goos": true, "goarch": true, "goamd64": true, "goarm64": true, "ldflags": true}

// universalKeys are the keys a universal_binaries entry may carry. Its hooks
// would run commands this command does not.
var universalKeys = map[string]bool{"id": true, "ids": true, "name_template": true, "replace": true, "mod_timestamp": true}

// archiveKeys are the keys the archive may carry. Any other (files, ids,
// wrap_in_directory) belongs to an archive that is not the binary itself.
var archiveKeys = map[string]bool{"id": true, "formats": true, "name_template": true}

// checksumKeys are the keys the checksum section may carry. split, ids and
// extra_files change what the file lists.
var checksumKeys = map[string]bool{"name_template": true, "algorithm": true}

// buildEntry is one builds entry once its keys have been checked.
type buildEntry struct {
	ID        string      `yaml:"id"`
	Main      string      `yaml:"main"`
	Binary    string      `yaml:"binary"`
	Env       []string    `yaml:"env"`
	Flags     []string    `yaml:"flags"`
	Goos      []string    `yaml:"goos"`
	Goarch    []string    `yaml:"goarch"`
	Ldflags   []string    `yaml:"ldflags"`
	Overrides []yaml.Node `yaml:"overrides"`
}

// overrideEntry is one overrides entry once its keys have been checked.
type overrideEntry struct {
	Goos    string   `yaml:"goos"`
	Goarch  string   `yaml:"goarch"`
	Goamd64 string   `yaml:"goamd64"`
	Goarm64 string   `yaml:"goarm64"`
	Ldflags []string `yaml:"ldflags"`
}

// universalEntry is one universal_binaries entry once its keys have been
// checked.
type universalEntry struct {
	ID           string   `yaml:"id"`
	IDs          []string `yaml:"ids"`
	NameTemplate string   `yaml:"name_template"`
	Replace      bool     `yaml:"replace"`
}

// archiveEntry is the archive once its keys have been checked.
type archiveEntry struct {
	Formats      []string `yaml:"formats"`
	NameTemplate string   `yaml:"name_template"`
}

// checksumEntry is the checksum section once its keys have been checked.
type checksumEntry struct {
	NameTemplate string `yaml:"name_template"`
	Algorithm    string `yaml:"algorithm"`
}

// readRelease reads what a GoReleaser configuration says about the binaries
// a release publishes.
//
// Every refusal names the section and what it cannot read, because the
// alternative to refusing is guessing what GoReleaser does with it, and a
// rebuild made on a guess compares the release with something it never
// built.
func readRelease(path string) (release, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- the operator's own -config, inside the tree being rebuilt
	if err != nil {
		return release{}, err
	}
	var file configFile
	if decodeErr := yaml.Unmarshal(data, &file); decodeErr != nil {
		return release{}, fmt.Errorf("%s: %w", path, decodeErr)
	}
	if topErr := checkTopLevel(path, &file); topErr != nil {
		return release{}, topErr
	}
	builds, err := readBuilds(path, file.Builds)
	if err != nil {
		return release{}, err
	}
	universals, err := readUniversals(path, file.UniversalBinaries, builds)
	if err != nil {
		return release{}, err
	}
	archive, err := readArchive(path, file.Archives)
	if err != nil {
		return release{}, err
	}
	checksums, err := readChecksum(path, &file.Checksum)
	if err != nil {
		return release{}, err
	}
	return release{builds: builds, universals: universals, archive: archive, checksums: checksums}, nil
}

// checkTopLevel refuses the top-level settings this command does not apply: a
// section it neither reads nor reads past, a global env, a gomod section that
// sets anything, a before key other than hooks, and a hook other than
// beforeHook. The other sections are refused in sorted order, so a
// configuration setting two is refused for the same one every run.
func checkTopLevel(path string, file *configFile) error {
	for _, key := range slices.Sorted(maps.Keys(file.Other)) {
		if !passedKeys[key] {
			return fmt.Errorf("%s sets %s, a top-level section this command does not read and has not judged to leave the binaries alone", path, key)
		}
	}
	if !file.Env.IsZero() {
		return fmt.Errorf("%s sets a global env, which every build inherits and this command does not read", path)
	}
	if key := unknownKey(&file.Gomod, nil); key != "" {
		return fmt.Errorf("%s sets gomod.%s, which changes how every build fetches or builds the module and this command does not read", path, key)
	}
	if file.Before.IsZero() {
		return nil
	}
	if key := unknownKey(&file.Before, map[string]bool{"hooks": true}); key != "" {
		return fmt.Errorf("%s sets before.%s, which this command does not read", path, key)
	}
	var before struct {
		Hooks []yaml.Node `yaml:"hooks"`
	}
	if err := file.Before.Decode(&before); err != nil {
		return fmt.Errorf("%s: before: %w", path, err)
	}
	// A hook written with options (cmd, dir, env) is a mapping, whose Value is
	// empty, so it is refused with every other command.
	for i := range before.Hooks {
		if resolve(&before.Hooks[i]).Value != beforeHook {
			return fmt.Errorf("%s: before.hooks[%d] is not %q; a hook may change the source a build compiles, and this command does not run it", path, i, beforeHook)
		}
	}
	return nil
}

// readBuilds reads every builds entry.
func readBuilds(path string, nodes []yaml.Node) ([]build, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("%s declares no builds", path)
	}
	builds := make([]build, 0, len(nodes))
	for i := range nodes {
		b, err := readBuild(fmt.Sprintf("%s: builds[%d]", path, i), &nodes[i])
		if err != nil {
			return nil, err
		}
		builds = append(builds, b)
	}
	return builds, nil
}

// readBuild checks and decodes one builds entry, which where names.
func readBuild(where string, node *yaml.Node) (build, error) {
	if key := unknownKey(node, buildKeys); key != "" {
		return build{}, fmt.Errorf("%s sets %s, which this command does not read", where, key)
	}
	var entry buildEntry
	if err := node.Decode(&entry); err != nil {
		return build{}, fmt.Errorf("%s: %w", where, err)
	}
	if entry.ID == "" || entry.Main == "" || entry.Binary == "" {
		return build{}, fmt.Errorf("%s needs an id, a main package and a binary name; this command does not assume GoReleaser's defaults", where)
	}
	targets, err := readTargets(where, entry.Goos, entry.Goarch)
	if err != nil {
		return build{}, err
	}
	overrides, err := readOverrides(where, entry.Overrides)
	if err != nil {
		return build{}, err
	}
	return build{
		id: entry.ID, main: entry.Main, binary: entry.Binary,
		env: entry.Env, flags: entry.Flags, ldflags: entry.Ldflags,
		overrides: overrides, targets: targets,
	}, nil
}

// readTargets crosses each goos with each goarch, in GoReleaser's order.
func readTargets(where string, goos, goarch []string) ([]target, error) {
	if len(goos) == 0 || len(goarch) == 0 {
		return nil, fmt.Errorf("%s lists no goos or no goarch; this command does not assume GoReleaser's defaults", where)
	}
	var targets []target
	for _, system := range goos {
		if _, known := extensions[system]; !known {
			return nil, fmt.Errorf("%s builds for %s, and this command knows the file names of linux, darwin and windows builds only", where, system)
		}
		for _, arch := range goarch {
			if _, known := levels[arch]; !known {
				return nil, fmt.Errorf("%s builds for %s, and this command knows GoReleaser's levels for amd64 and arm64 only", where, arch)
			}
			targets = append(targets, target{goos: system, goarch: arch})
		}
	}
	return targets, nil
}

// readOverrides checks and decodes the overrides of the build where names.
func readOverrides(where string, nodes []yaml.Node) ([]override, error) {
	overrides := make([]override, 0, len(nodes))
	for i := range nodes {
		at := fmt.Sprintf("%s overrides[%d]", where, i)
		if key := unknownKey(&nodes[i], overrideKeys); key != "" {
			return nil, fmt.Errorf("%s sets %s; an override may select its targets and set ldflags, and nothing else", at, key)
		}
		var entry overrideEntry
		if err := nodes[i].Decode(&entry); err != nil {
			return nil, fmt.Errorf("%s: %w", at, err)
		}
		if entry.Goos == "" || entry.Goarch == "" {
			return nil, fmt.Errorf("%s names no goos or no goarch, so it selects no target", at)
		}
		overrides = append(overrides, override{
			goos: entry.Goos, goarch: entry.Goarch, goamd64: entry.Goamd64, goarm64: entry.Goarm64, ldflags: entry.Ldflags,
		})
	}
	return overrides, nil
}

// readUniversals checks and decodes every universal_binaries entry against
// the builds it may join.
func readUniversals(path string, nodes []yaml.Node, builds []build) ([]universal, error) {
	universals := make([]universal, 0, len(nodes))
	for i := range nodes {
		where := fmt.Sprintf("%s: universal_binaries[%d]", path, i)
		if key := unknownKey(&nodes[i], universalKeys); key != "" {
			return nil, fmt.Errorf("%s sets %s, which this command does not read", where, key)
		}
		var entry universalEntry
		if err := nodes[i].Decode(&entry); err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		if entry.ID == "" || entry.NameTemplate == "" {
			return nil, fmt.Errorf("%s needs an id and a name_template; this command does not assume GoReleaser's defaults", where)
		}
		if entry.Replace {
			return nil, fmt.Errorf("%s sets replace, which publishes it in place of the darwin binaries this command rebuilds it from", where)
		}
		ids := entry.IDs
		if len(ids) == 0 {
			ids = []string{entry.ID}
		}
		for _, id := range ids {
			if !slices.ContainsFunc(builds, func(b build) bool { return b.id == id }) {
				return nil, fmt.Errorf("%s joins build %s, which the configuration does not declare", where, id)
			}
		}
		universals = append(universals, universal{id: entry.ID, name: entry.NameTemplate, ids: ids})
	}
	return universals, nil
}

// readArchive checks and decodes the archive, of which there is exactly one,
// publishing each binary as it is, so the checksum file lists the binaries
// themselves.
func readArchive(path string, nodes []yaml.Node) (string, error) {
	if len(nodes) != 1 {
		return "", fmt.Errorf("%s declares %d archives; this command reads one, which publishes each binary as it is", path, len(nodes))
	}
	where := path + ": archives[0]"
	if key := unknownKey(&nodes[0], archiveKeys); key != "" {
		return "", fmt.Errorf("%s sets %s, which this command does not read", where, key)
	}
	var entry archiveEntry
	if err := nodes[0].Decode(&entry); err != nil {
		return "", fmt.Errorf("%s: %w", where, err)
	}
	if !slices.Equal(entry.Formats, []string{"binary"}) {
		return "", fmt.Errorf("%s publishes %q, and this command compares binaries published as they are (formats: [binary])", where, entry.Formats)
	}
	if entry.NameTemplate == "" {
		return "", fmt.Errorf("%s names no name_template; this command does not assume GoReleaser's default", where)
	}
	return entry.NameTemplate, nil
}

// readChecksum checks and decodes the checksum section, which has to name
// its file and may name no algorithm but SHA-256.
func readChecksum(path string, node *yaml.Node) (string, error) {
	where := path + ": checksum"
	if node.IsZero() {
		return "", fmt.Errorf("%s declares no checksum section; GoReleaser's default file name carries the project name, which this command does not evaluate", path)
	}
	if key := unknownKey(node, checksumKeys); key != "" {
		return "", fmt.Errorf("%s sets %s, which changes what the file lists and this command does not read", where, key)
	}
	var entry checksumEntry
	if err := node.Decode(&entry); err != nil {
		return "", fmt.Errorf("%s: %w", where, err)
	}
	if entry.NameTemplate == "" {
		return "", fmt.Errorf("%s names no name_template", where)
	}
	if entry.Algorithm != "" && entry.Algorithm != "sha256" {
		return "", fmt.Errorf("%s uses %s, and this command compares SHA-256 digests", where, entry.Algorithm)
	}
	return entry.NameTemplate, nil
}

// unknownKey returns the first key of a mapping that allowed does not name, or
// "" when it names every one. An alias is followed to the mapping it names,
// since that is what GoReleaser reads. A YAML merge key is returned as "<<"
// rather than expanded: the keys it brings are not written here to be judged.
func unknownKey(node *yaml.Node, allowed map[string]bool) string {
	node = resolve(node)
	for i := 0; i < len(node.Content); i += 2 {
		if key := node.Content[i].Value; !allowed[key] {
			return key
		}
	}
	return ""
}

// resolve follows an alias to the node it names, which is what GoReleaser
// reads in its place.
func resolve(node *yaml.Node) *yaml.Node {
	for node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}
