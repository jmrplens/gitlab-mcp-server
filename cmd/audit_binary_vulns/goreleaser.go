package main

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

// target is one operating system and architecture a release build produces.
type target struct {
	goos, goarch string
}

// String names the target the way the go command and GoReleaser both do.
func (t target) String() string {
	return t.goos + "/" + t.goarch
}

// build is one entry of the builds list, reduced to what decides which modules
// a binary links: the main package, the environment, the flags and the targets.
type build struct {
	id      string
	main    string
	env     []string
	flags   []string
	targets []target
}

// goreleaserFile is the part of .goreleaser.yml this command reads.
//
// The builds are kept as nodes so each entry's keys can be held to buildKeys
// before it is decoded: a decode alone drops every key it has no field for,
// and a key it drops may change what GoReleaser links. The global env is
// decoded only to be refused, since GoReleaser applies it to every build.
type goreleaserFile struct {
	Env    yaml.Node   `yaml:"env"`
	Builds []yaml.Node `yaml:"builds"`
}

// buildEntry is one builds entry once its keys have been checked.
type buildEntry struct {
	ID        string      `yaml:"id"`
	Main      string      `yaml:"main"`
	Env       []string    `yaml:"env"`
	Flags     []string    `yaml:"flags"`
	Goos      []string    `yaml:"goos"`
	Goarch    []string    `yaml:"goarch"`
	Overrides []yaml.Node `yaml:"overrides"`
}

// buildKeys are the keys a builds entry may carry. Every other key is refused,
// because each of the others can change what is built or how: tags, dir,
// gobinary or tool, buildmode, gcflags, hooks, ignore and targets among them,
// and the next one GoReleaser adds. Reading the entry without the key would
// scan binaries the release does not build, and say nothing.
var buildKeys = map[string]bool{
	// Read: the package, the environment, the flags and the targets.
	"id": true, "main": true, "env": true, "flags": true, "goos": true, "goarch": true,
	// Not read, and unable to change which modules a binary links: the file
	// name, the linker flags and the file's timestamp.
	"binary": true, "ldflags": true, "mod_timestamp": true,
	// Read entry by entry against overrideKeys.
	"overrides": true,
}

// overrideKeys are the keys an overrides entry may carry: what selects the
// targets it applies to, and the linker flags, which is what the repository's
// overrides change (the ELF loader of each Linux target). An override setting
// env, flags or tags would change a target's build in a way this command does
// not apply, so it is refused like an unknown key of the entry itself.
var overrideKeys = map[string]bool{
	"goos": true, "goarch": true, "goamd64": true, "goarm": true, "goarm64": true,
	"ldflags": true,
}

// readBuilds reads the builds a GoReleaser configuration declares.
//
// Every refusal names the entry and what it cannot read, because the
// alternative to refusing is guessing what GoReleaser does with it, and a guess
// that differs from what GoReleaser does is a scan of binaries nobody ships.
func readBuilds(path string) ([]build, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file goreleaserFile
	if decodeErr := yaml.Unmarshal(data, &file); decodeErr != nil {
		return nil, fmt.Errorf("%s: %w", path, decodeErr)
	}
	if !file.Env.IsZero() {
		return nil, fmt.Errorf("%s sets a global env, which every build inherits and this command does not read", path)
	}
	if len(file.Builds) == 0 {
		return nil, fmt.Errorf("%s declares no builds", path)
	}

	builds := make([]build, 0, len(file.Builds))
	for i := range file.Builds {
		b, buildErr := readBuild(path, i, &file.Builds[i])
		if buildErr != nil {
			return nil, buildErr
		}
		builds = append(builds, b)
	}
	return builds, nil
}

// readBuild checks and decodes the builds entry at index i.
func readBuild(path string, i int, node *yaml.Node) (build, error) {
	var entry buildEntry
	if err := node.Decode(&entry); err != nil {
		return build{}, fmt.Errorf("%s: builds[%d]: %w", path, i, err)
	}
	name := entry.ID
	if name == "" {
		name = fmt.Sprintf("builds[%d]", i)
	}
	if key := unknownKey(node, buildKeys); key != "" {
		return build{}, fmt.Errorf("%s: %s sets %s, which this command does not read", path, name, key)
	}
	for j := range entry.Overrides {
		if key := unknownKey(&entry.Overrides[j], overrideKeys); key != "" {
			return build{}, fmt.Errorf("%s: %s overrides[%d] sets %s; an override may change ldflags and nothing else", path, name, j, key)
		}
	}
	if entry.Main == "" {
		return build{}, fmt.Errorf("%s: %s names no main package", path, name)
	}
	if len(entry.Goos) == 0 || len(entry.Goarch) == 0 {
		return build{}, fmt.Errorf("%s: %s lists no goos or no goarch; this command does not assume GoReleaser's defaults", path, name)
	}
	b := build{id: name, main: entry.Main, env: entry.Env, flags: entry.Flags}
	for _, goos := range entry.Goos {
		for _, goarch := range entry.Goarch {
			b.targets = append(b.targets, target{goos: goos, goarch: goarch})
		}
	}
	return b, nil
}

// unknownKey returns the first key of a mapping that allowed does not name, or
// "" when it names every one. An alias is followed to the mapping it names,
// since that is what GoReleaser reads. A YAML merge key is returned as "<<"
// rather than expanded: the keys it brings are not written here to be judged.
func unknownKey(node *yaml.Node, allowed map[string]bool) string {
	for node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	for i := 0; i < len(node.Content); i += 2 {
		if key := node.Content[i].Value; !allowed[key] {
			return key
		}
	}
	return ""
}
