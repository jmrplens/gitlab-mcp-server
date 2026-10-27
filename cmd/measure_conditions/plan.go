// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// runners are the systems a dispatch may name, in the order the plan lists
// them. They are the three hosted images CI's cross-platform job already runs
// on; a label outside them is refused, since runs-on takes whatever the plan
// prints.
var runners = []string{"windows-latest", "macos-latest", "ubuntu-latest"}

// gatePattern is every GOBCO_GATE value scripts/coverage-conditions.sh reads,
// spelled as it checks them, so a value it would refuse after a runner has
// started is refused before any has.
var gatePattern = regexp.MustCompile(`^(all|beyond:[a-z0-9]+/[a-z0-9]+)$`)

// defaultMode is the mode a subcommand measures or reads in when it is not
// given one: gobco's own default, which every run measured in before the
// mode could be chosen.
const defaultMode = "condition"

// refuseMode says on stderr that a -mode is not a GOBCO_MODE the script
// reads, and reports whether it did, so a value the script would refuse
// after a runner has started is refused before any has, as the gate is.
func refuseMode(stderr io.Writer, mode string) bool {
	if _, known := coverageModes[mode]; known {
		return false
	}
	fmt.Fprintf(stderr, "%s: -mode %q is neither condition nor branch, which is what scripts/coverage-conditions.sh reads\n", toolName, mode)
	return true
}

// segmentPattern is one directory of a package path. It admits what this
// module's directories are named with and nothing a shell, a workflow
// command or an artifact name would read as syntax, and since a segment
// cannot start with a dot it refuses `.`, `..` and the `...` of a pattern.
var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// treeShards is how many jobs a -tree plan splits the packages below cmd/ and
// internal/ into on each runner. gobco caches nothing and takes from a few
// seconds to several minutes a package; docs/development/testing/README.md
// records how long the whole tree took on one machine and with how many
// packages at a time. Twenty shards make a job of fifteen or so packages one
// after another, which is expected to finish well inside the two hours a
// shard's job is given, and twenty shards on all three runners stay far below
// the 256 jobs a matrix may hold.
const treeShards = 20

// treePatterns are the packages a -tree plan measures: every package of the
// server and of the repository's tooling. test/ is left out, since every
// file of the e2e suite sits behind a build tag and gobco cannot measure it
// (docs/development/testing/README.md).
var treePatterns = []string{"./cmd/...", "./internal/..."}

// cell is one job the plan asks for: the packages it measures on one runner,
// one after another, separated by spaces, what the job is called, and the
// name of the artifact the records and raw outputs of its packages are
// uploaded under. A plan of named packages gives each package a job of its
// own, named after it; a -tree plan gives each job a shard of the tree.
type cell struct {
	System   string `json:"system"`
	Name     string `json:"name"`
	Packages string `json:"packages"`
	Artifact string `json:"artifact"`
}

// matrix is the strategy matrix the workflow fans out over, in the shape
// `strategy.matrix` reads: every cell is an include entry.
type matrix struct {
	Include []cell `json:"include"`
}

// runPlan validates a dispatch's inputs and prints the matrix as one
// `matrix=<json>` line for $GITHUB_OUTPUT, and what it planned on stderr. It
// returns 2 for an input it cannot read and 1 when the packages could not be
// found or none was.
func runPlan(args []string, stdout, stderr io.Writer, list lister) int {
	flags := flag.NewFlagSet("plan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	packagesFlag := flags.String("packages", "", "packages to measure, ./cmd/server style, separated by spaces or commas; empty means every package carrying a GOOS- or GOARCH-constrained file")
	systemsFlag := flags.String("systems", "", "runners to measure on, separated by spaces or commas: "+strings.Join(runners, ", "))
	gate := flags.String("gate", "", "the GOBCO_GATE the script runs with: all or beyond:GOOS/GOARCH")
	mode := flags.String("mode", defaultMode, "the GOBCO_MODE the script runs with: condition or branch")
	dir := flags.String("dir", ".", "module root the packages are looked for in")
	tree := flags.Bool("tree", false, "measure every package below cmd/ and internal/, in shards, instead of the packages -packages names")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "%s: plan takes no arguments, and was given %q\n", toolName, flags.Args())
		return 2
	}
	if !gatePattern.MatchString(*gate) {
		fmt.Fprintf(stderr, "%s: -gate %q is neither all nor beyond:GOOS/GOARCH, which is what scripts/coverage-conditions.sh reads\n", toolName, *gate)
		return 2
	}
	if refuseMode(stderr, *mode) {
		return 2
	}
	measuredWith := fmt.Sprintf("GOBCO_GATE=%s and GOBCO_MODE=%s", *gate, *mode)
	systems, err := parseSystems(*systemsFlag)
	if err != nil {
		fmt.Fprintf(stderr, "%s: -systems: %v\n", toolName, err)
		return 2
	}
	packages, err := parsePackages(*packagesFlag)
	if err != nil {
		fmt.Fprintf(stderr, "%s: -packages: %v\n", toolName, err)
		return 2
	}
	if *tree {
		return planTree(stdout, stderr, list, *dir, packages, systems, measuredWith)
	}
	if len(packages) == 0 {
		packages, err = constrainedPackages(context.Background(), *dir, list)
		if err != nil {
			fmt.Fprintf(stderr, "%s: no package named, and the packages carrying a GOOS- or GOARCH-constrained file could not be found: %v\n", toolName, err)
			return 1
		}
		if len(packages) == 0 {
			fmt.Fprintf(stderr, "%s: no package named, and no package builds different files on two of the release targets (%s), so there is nothing to measure\n", toolName, strings.Join(releaseTargets, ", "))
			return 1
		}
		fmt.Fprintf(stderr, "%s: no package named; measuring the %d whose built files differ between release targets: %s\n", toolName, len(packages), strings.Join(packages, " "))
	}
	planned, err := buildMatrix(packages, systems)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 2
	}
	fmt.Fprintf(stdout, "matrix=%s\n", cmdutil.Must(json.Marshal(planned)))
	fmt.Fprintf(stderr, "%s: %d run(s) planned, %s on %s, with %s\n", toolName, len(planned.Include), strings.Join(packages, " "), strings.Join(systems, " "), measuredWith)
	return 0
}

// splitList splits a dispatch input on spaces and commas, the two separators
// a person typing a list into the dispatch form reaches for.
func splitList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
}

// parseSystems returns the runners a dispatch named, each once and in the
// order of runners, refusing an empty list and a label outside it.
func parseSystems(value string) ([]string, error) {
	named := splitList(value)
	for _, system := range named {
		if !slices.Contains(runners, system) {
			return nil, fmt.Errorf("%q is not one of %s", system, strings.Join(runners, ", "))
		}
	}
	var systems []string
	for _, system := range runners {
		if slices.Contains(named, system) {
			systems = append(systems, system)
		}
	}
	if len(systems) == 0 {
		return nil, fmt.Errorf("name at least one of %s", strings.Join(runners, ", "))
	}
	return systems, nil
}

// parsePackages returns the packages a dispatch named, each written ./ and
// its directory and each once, in the order first named. A pattern, a path
// leaving the module and a character outside segmentPattern are refused,
// since the script measures one package and every name ends up in a shell
// variable, an artifact name and a summary.
func parsePackages(value string) ([]string, error) {
	var packages []string
	for _, named := range splitList(value) {
		segments := strings.Split(strings.TrimPrefix(named, "./"), "/")
		for _, segment := range segments {
			if !segmentPattern.MatchString(segment) {
				return nil, fmt.Errorf("%q is not a package directory below the module root, such as ./cmd/server", named)
			}
		}
		pkg := "./" + strings.Join(segments, "/")
		if !slices.Contains(packages, pkg) {
			packages = append(packages, pkg)
		}
	}
	return packages, nil
}

// buildMatrix crosses the packages with the systems, package by package, and
// gives each package a job of its own on each system, named after it, with an
// artifact named after both. Two cells whose names would collide, which only
// directory names joined differently by a dash could do, are refused rather
// than left to overwrite each other's upload.
func buildMatrix(packages, systems []string) (matrix, error) {
	planned := matrix{Include: []cell{}}
	byArtifact := map[string]cell{}
	for _, pkg := range packages {
		for _, system := range systems {
			artifact := "gobco-" + system + "-" + packageSlug(pkg)
			if other, taken := byArtifact[artifact]; taken {
				return matrix{}, fmt.Errorf("%s and %s on %s would both upload as %s", other.Packages, pkg, system, artifact)
			}
			entry := cell{System: system, Name: pkg, Packages: pkg, Artifact: artifact}
			byArtifact[artifact] = entry
			planned.Include = append(planned.Include, entry)
		}
	}
	return planned, nil
}

// planTree prints the plan of a -tree dispatch: every package below cmd/ and
// internal/, in shards, and says on stderr what it planned and the gate and
// mode the runs measure with. A package list beside -tree is refused, since
// the dispatch would be asking for two things at once.
func planTree(stdout, stderr io.Writer, list lister, dir string, named, systems []string, measuredWith string) int {
	if len(named) > 0 {
		fmt.Fprintf(stderr, "%s: -tree measures every package below cmd/ and internal/, and -packages names %s besides; leave -packages empty\n", toolName, strings.Join(named, " "))
		return 2
	}
	packages, err := treePackages(context.Background(), dir, list)
	if err != nil {
		fmt.Fprintf(stderr, "%s: -tree: the packages below cmd/ and internal/ could not be listed: %v\n", toolName, err)
		return 1
	}
	if len(packages) == 0 {
		fmt.Fprintf(stderr, "%s: -tree: there is no package below cmd/ and internal/, so there is nothing to measure\n", toolName)
		return 1
	}
	planned := buildShards(packages, systems, treeShards)
	fmt.Fprintf(stdout, "matrix=%s\n", cmdutil.Must(json.Marshal(planned)))
	fmt.Fprintf(stderr, "%s: %d run(s) planned, the %d packages below cmd/ and internal/ in %d shard(s) on %s, with %s\n",
		toolName, len(planned.Include), len(packages), min(treeShards, len(packages)), strings.Join(systems, " "), measuredWith)
	return 0
}

// buildShards deals the packages, in the order given, into at most shards
// jobs on each system, the first package to the first shard, the second to
// the second and so on round again, so the slow packages of one directory
// do not all land in one job. Each shard is named by its number and its
// artifact by its system and number, which no two shards share.
func buildShards(packages, systems []string, shards int) matrix {
	count := min(shards, len(packages))
	planned := matrix{Include: []cell{}}
	for shard := range count {
		var members []string
		for i := shard; i < len(packages); i += count {
			members = append(members, packages[i])
		}
		for _, system := range systems {
			planned.Include = append(planned.Include, cell{
				System:   system,
				Name:     fmt.Sprintf("tree %02d of %02d", shard+1, count),
				Packages: strings.Join(members, " "),
				Artifact: fmt.Sprintf("gobco-%s-tree-%02d", system, shard+1),
			})
		}
	}
	return planned
}

// packageSlug is a package written as one directory name: its path below
// the module root with each slash a dash. It names a package's artifact in a
// plan of named packages and its record's directory everywhere.
func packageSlug(pkg string) string {
	return strings.ReplaceAll(strings.TrimPrefix(pkg, "./"), "/", "-")
}
