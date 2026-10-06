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

// segmentPattern is one directory of a package path. It admits what this
// module's directories are named with and nothing a shell, a workflow
// command or an artifact name would read as syntax, and since a segment
// cannot start with a dot it refuses `.`, `..` and the `...` of a pattern.
var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// cell is one run the plan asks for: one package on one runner, and the name
// of the artifact its record and raw output are uploaded under.
type cell struct {
	System   string `json:"system"`
	Package  string `json:"package"`
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
	dir := flags.String("dir", ".", "module root the packages are looked for in")
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
	fmt.Fprintf(stderr, "%s: %d run(s) planned, %s on %s, with GOBCO_GATE=%s\n", toolName, len(planned.Include), strings.Join(packages, " "), strings.Join(systems, " "), *gate)
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
// names each cell's artifact after both. Two cells whose names would collide,
// which only directory names joined differently by a dash could do, are
// refused rather than left to overwrite each other's upload.
func buildMatrix(packages, systems []string) (matrix, error) {
	planned := matrix{Include: []cell{}}
	byArtifact := map[string]cell{}
	for _, pkg := range packages {
		for _, system := range systems {
			artifact := "gobco-" + system + "-" + strings.ReplaceAll(strings.TrimPrefix(pkg, "./"), "/", "-")
			if other, taken := byArtifact[artifact]; taken {
				return matrix{}, fmt.Errorf("%s and %s on %s would both upload as %s", other.Package, pkg, system, artifact)
			}
			entry := cell{System: system, Package: pkg, Artifact: artifact}
			byArtifact[artifact] = entry
			planned.Include = append(planned.Include, entry)
		}
	}
	return planned, nil
}
