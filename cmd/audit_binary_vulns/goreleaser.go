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
// ignore and targets are decoded only to be refused: each one changes which
// targets an entry builds, and reading goos and goarch while one of them is
// set would scan a set of binaries the release does not publish.
type goreleaserFile struct {
	Builds []struct {
		ID      string    `yaml:"id"`
		Main    string    `yaml:"main"`
		Env     []string  `yaml:"env"`
		Flags   []string  `yaml:"flags"`
		Goos    []string  `yaml:"goos"`
		Goarch  []string  `yaml:"goarch"`
		Ignore  yaml.Node `yaml:"ignore"`
		Targets yaml.Node `yaml:"targets"`
	} `yaml:"builds"`
}

// readBuilds reads the builds a GoReleaser configuration declares.
//
// Every refusal names the entry and what is missing, because the alternative to
// refusing is guessing GoReleaser's defaults, and a guess that differs from
// what GoReleaser does is a scan of binaries nobody ships.
func readBuilds(path string) ([]build, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file goreleaserFile
	if decodeErr := yaml.Unmarshal(data, &file); decodeErr != nil {
		return nil, fmt.Errorf("%s: %w", path, decodeErr)
	}
	if len(file.Builds) == 0 {
		return nil, fmt.Errorf("%s declares no builds", path)
	}

	builds := make([]build, 0, len(file.Builds))
	for i, entry := range file.Builds {
		name := entry.ID
		if name == "" {
			name = fmt.Sprintf("builds[%d]", i)
		}
		if entry.Main == "" {
			return nil, fmt.Errorf("%s: %s names no main package", path, name)
		}
		if len(entry.Goos) == 0 || len(entry.Goarch) == 0 {
			return nil, fmt.Errorf("%s: %s lists no goos or no goarch; this command does not assume GoReleaser's defaults", path, name)
		}
		if !entry.Ignore.IsZero() || !entry.Targets.IsZero() {
			return nil, fmt.Errorf("%s: %s selects its targets with ignore or targets, which this command does not read", path, name)
		}
		b := build{id: name, main: entry.Main, env: entry.Env, flags: entry.Flags}
		for _, goos := range entry.Goos {
			for _, goarch := range entry.Goarch {
				b.targets = append(b.targets, target{goos: goos, goarch: goarch})
			}
		}
		builds = append(builds, b)
	}
	return builds, nil
}
