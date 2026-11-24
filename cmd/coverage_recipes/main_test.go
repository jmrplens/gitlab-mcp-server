// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/conditions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// repository is the root of this repository from this package.
var repository = filepath.Join("..", "..")

// readRepositoryFile reads one file of this repository.
func readRepositoryFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestRun_EachSubcommand_IsReachedOrRefused verifies the dispatch: no
// subcommand and an unknown one print the usage, and each known one is reached
// with the arguments after its name, which its own usage error shows.
func TestRun_EachSubcommand_IsReachedOrRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no subcommand", nil, usage},
		{"unknown", []string{"report"}, "coverage_recipes: unknown subcommand \"report\"\n" + usage},
		{"conditions without a package", []string{"conditions"}, "coverage_recipes: conditions takes a package, and build tags after it\n"},
		{"conditions with an empty package", []string{"conditions", ""}, "coverage_recipes: conditions takes a package, and build tags after it\n"},
		{"conditions with a third argument", []string{"conditions", "./a", "e2e", "x"}, "coverage_recipes: conditions takes a package, and build tags after it\n"},
		{"install-gobco with an argument", []string{"install-gobco", "./a"}, "coverage_recipes: install-gobco takes no argument\n"},
		{"mutants without a package", []string{"mutants"}, "coverage_recipes: mutants takes a package, and a budget, a floor and a ceiling in seconds after it\n"},
		{"mutants with an empty package", []string{"mutants", ""}, "coverage_recipes: mutants takes a package, and a budget, a floor and a ceiling in seconds after it\n"},
		{"mutants with a fifth argument", []string{"mutants", "./a", "1", "2", "3", "4"}, "coverage_recipes: mutants takes a package, and a budget, a floor and a ceiling in seconds after it\n"},
		{"spec-conditions with two bases", []string{"spec-conditions", "a", "b"}, "coverage_recipes: spec-conditions takes one base at most\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			s := &session{stdout: &stdout, stderr: &stderr}
			if status := s.run(t.Context(), tc.args); status != 2 || stdout.Len() != 0 || stderr.String() != tc.want {
				t.Errorf("run(%q) = %d, stdout %q, stderr %q; want 2 and %q", tc.args, status, stdout.String(), stderr.String(), tc.want)
			}
		})
	}
}

// TestUsage_NamesEverySubcommand verifies the usage text names every
// subcommand the dispatch accepts.
func TestUsage_NamesEverySubcommand(t *testing.T) {
	t.Parallel()
	for _, subcommand := range []string{"conditions", "install-gobco", "mutants", "spec-conditions"} {
		t.Run(subcommand, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(usage, "\n  "+subcommand+" ") {
				t.Errorf("usage does not name %q:\n%s", subcommand, usage)
			}
		})
	}
}

// TestConditions_ThePackage_IsHandedToTheRecipeWithItsTags verifies that the
// conditions subcommand asks the recipe for the package where the session
// runs, with its environment and streams, and passes its status on.
func TestConditions_ThePackage_IsHandedToTheRecipeWithItsTags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		tags string
	}{{"no tags", []string{"conditions", "./a"}, ""}, {"tags", []string{"conditions", "./a", "e2e"}, "e2e"}, {"empty tags", []string{"conditions", "./a", ""}, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			var asked []conditions.Request
			s := &session{dir: "here", environ: []string{"A=1"}, stdout: &stdout, stderr: &stderr}
			s.measure = func(_ context.Context, _ conditions.Tools, req conditions.Request) int {
				asked = append(asked, req)
				return conditions.GateHeld
			}
			if status := s.run(t.Context(), tc.args); status != conditions.GateHeld {
				t.Errorf("run() = %d, want the recipe's 3", status)
			}
			want := []conditions.Request{{Dir: "here", Package: "./a", Tags: tc.tags, Environ: []string{"A=1"}, Stdout: &stdout, Stderr: &stderr}}
			if !reflect.DeepEqual(asked, want) {
				t.Errorf("asked for %+v, want %+v", asked, want)
			}
		})
	}
}

// TestInstallGobco_ThePinnedRelease_IsWhatIsLookedFor verifies that the
// install-gobco subcommand is the recipe's install: a gobco PATH already finds
// in the pinned release is left as it is.
func TestInstallGobco_ThePinnedRelease_IsWhatIsLookedFor(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	s := &session{stdout: &stdout, stderr: &stderr}
	s.LookPath = func(string) (string, error) { return "/planted/gobco", nil }
	s.Run = func(_ context.Context, call recipe.Call) (int, error) {
		_, err := call.Stdout.Write([]byte(strings.TrimPrefix(conditions.Release, "v") + "\n"))
		return 0, err
	}
	if status := s.run(t.Context(), []string{"install-gobco"}); status != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "PATH already finds "+conditions.Gobco+" at /planted/gobco") {
		t.Errorf("run() = %d, stdout %q, stderr %q", status, stdout.String(), stderr.String())
	}
}

// TestMain_NoSubcommand_ExitsWithTheUsageStatus verifies that main hands the
// process arguments after its own name to the session production runs, and
// passes the status on to the exit seam.
func TestMain_NoSubcommand_ExitsWithTheUsageStatus(t *testing.T) {
	previousArgs, previousExit, previousStderr := os.Args, osExit, os.Stderr
	t.Cleanup(func() { os.Args, osExit, os.Stderr = previousArgs, previousExit, previousStderr })
	silent, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = silent.Close() }()
	os.Stderr = silent
	got := -1
	os.Args = []string{"coverage_recipes"}
	osExit = func(code int) { got = code }
	main()
	if got != 2 {
		t.Errorf("main() exit = %d, want 2", got)
	}
}

// TestDefaultSession_IsThisProcess verifies the session production runs: this
// process's directory, environment and platform, and real tools.
func TestDefaultSession_IsThisProcess(t *testing.T) {
	t.Parallel()
	s := defaultSession()
	if s.dir != "." || s.goos != runtime.GOOS || s.Go != golist.Executable() || len(s.environ) == 0 || s.stdout != os.Stdout || s.stderr != os.Stderr {
		t.Errorf("defaultSession() = %+v", s)
	}
	if s.now == nil || s.limits == nil || s.measure == nil || s.Run == nil || s.LookPath == nil || s.Pause == nil {
		t.Errorf("defaultSession() leaves a tool unset: %+v", s)
	}
	if _, _, err := s.limits(); err != nil && runtime.GOOS == "linux" && !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("limits() = %v", err)
	}
}

// makeRecipe is the recipe of a Makefile target, its lines joined.
func makeRecipe(t *testing.T, target string) string {
	t.Helper()
	found := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `:\n((?:\t.*\n)+)`).FindStringSubmatch(readRepositoryFile(t, "Makefile"))
	if found == nil {
		t.Fatalf("the Makefile has no target %s", target)
	}
	return found[1]
}

// TestMakefile_EachTarget_RunsItsRecipe verifies that each make target runs
// this command with the variables it is documented with, that the mutation
// recipe is handed the same defaults it has of its own, since make always
// passes all three and the 300 s budget that measured first-mutant recompiles
// as kills rather than timeouts holds only as long as the Makefile says 300
// too, and that no target pins a tool the recipes pin.
func TestMakefile_EachTarget_RunsItsRecipe(t *testing.T) {
	t.Parallel()
	for target, want := range map[string]string{
		"coverage-conditions":   "\t@$(call RUN_BUILT,coverage_recipes) conditions $(PKG) $(TAGS)\n",
		"coverage-mutants":      "\t@$(call RUN_BUILT,coverage_recipes) mutants $(PKG) $(MUTANT_BUDGET) $(MUTANT_BUDGET_FLOOR) $(MUTANT_DEADLINE_MAX)\n",
		"check-spec-conditions": "\t$(call RUN_BUILT,coverage_recipes) spec-conditions $(BASE)\n",
	} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			recipeLines := makeRecipe(t, target)
			if !strings.HasSuffix(recipeLines, want) {
				t.Errorf("the recipe of %s is\n%s\nwant it to end\n%s", target, recipeLines, want)
			}
			if strings.Contains(recipeLines, "rillig/gobco") || strings.Contains(recipeLines, "go-gremlins") {
				t.Errorf("the recipe of %s pins a tool of its own:\n%s", target, recipeLines)
			}
		})
	}
	makefile := readRepositoryFile(t, "Makefile")
	for name, want := range map[string]string{"MUTANT_BUDGET": defaultBudget, "MUTANT_BUDGET_FLOOR": defaultFloor, "MUTANT_DEADLINE_MAX": defaultCeiling} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(makefile, "\n"+name+" ?= "+want+"\n") {
				t.Errorf("the Makefile does not default %s to %s, the recipe's own", name, want)
			}
		})
	}
}

// TestCIWorkflow_CrossPlatformLegs_RunTheGate verifies that the Windows and
// macOS legs run the recipe itself, not make, which windows-latest does not
// promise, on the two packages whose files split by platform, held to every
// condition as the local gate holds them on Linux (issue 1212), with the
// timeout cmd/server needs on Windows.
func TestCIWorkflow_CrossPlatformLegs_RunTheGate(t *testing.T) {
	t.Parallel()
	step := regexp.MustCompile(`(?m)^      - name: Condition coverage of cmd/server and internal/toolutil\n((?:        .*\n|\n)+)`).
		FindStringSubmatch(readRepositoryFile(t, ".github/workflows/ci.yml"))
	if step == nil {
		t.Fatal("the cross-platform gobco step is missing")
	}
	for _, want := range []string{
		"if: matrix.os != 'ubuntu-latest'", "shell: bash", "GOBCO_GATE: all", "GOBCO_TEST_FLAGS: -timeout=30m",
		"./cmd/server", "./internal/toolutil", `go run ./cmd/coverage_recipes conditions "$pkg"`,
	} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(step[1], want) {
				t.Errorf("the step does not carry %q:\n%s", want, step[1])
			}
		})
	}
	if strings.Contains(step[1], "make ") {
		t.Errorf("the step runs make:\n%s", step[1])
	}
}

// workflowJobs are the jobs of a workflow by id, with their steps in order,
// named or not. A step is its dash line and every line below it indented
// further, written as from its own level (`name:`, `run:` and its other keys
// at the start of a line) and without comment lines, so a comment above a
// step or inside its script runs nothing.
func workflowJobs(text string) map[string][]string {
	jobs := map[string][]string{}
	job, inJobs, inSteps := "", false, false
	header := regexp.MustCompile(`^  ([A-Za-z0-9_-]+):\s*$`)
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch found := header.FindStringSubmatch(line); {
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
		case !strings.HasPrefix(line, " "):
			inJobs, inSteps = trimmed == "jobs:", false
		case !inJobs:
		case found != nil:
			job, inSteps = found[1], false
			jobs[job] = nil
		case strings.HasPrefix(line, "    steps:"):
			inSteps = true
		case inSteps && strings.HasPrefix(line, "      - "):
			jobs[job] = append(jobs[job], line[len("      - "):]+"\n")
		case inSteps && strings.HasPrefix(line, "        ") && len(jobs[job]) > 0:
			jobs[job][len(jobs[job])-1] += line[len("        "):] + "\n"
		case !strings.HasPrefix(line, "      "):
			inSteps = false
		}
	}
	return jobs
}

// The steps of a workflow that measure conditions, whichever way, and the one
// that installs gobco for them.
var (
	measuringStep  = regexp.MustCompile(`coverage_recipes conditions\b|coverage_recipes spec-conditions\b|measure_conditions\s+measure\b|make\s+coverage-conditions\b|check-spec-conditions`)
	installingStep = regexp.MustCompile(`(?m)^run: go run \./cmd/coverage_recipes install-gobco$`)
	conditionLine  = regexp.MustCompile(`(?m)^if: .*$`)
)

// holdInstallBefore holds a job's steps to installing gobco once, in the step
// before the first that measures.
func holdInstallBefore(t *testing.T, steps []string, first int) {
	t.Helper()
	installs := slices.DeleteFunc(slices.Clone(steps), func(step string) bool { return !installingStep.MatchString(step) })
	if len(installs) != 1 || first == 0 || steps[first-1] != installs[0] {
		t.Fatalf("the job installs gobco in %q, want once, in the step before\n%s", installs, steps[first])
	}
	for _, want := range []string{`(?m)^name: Install gobco$`, `(?m)^shell: bash$`, `(?m)^timeout-minutes: [1-9]$`} {
		if !regexp.MustCompile(want).MatchString(installs[0]) {
			t.Errorf("the install step does not match %s:\n%s", want, installs[0])
		}
	}
	if got, want := conditionLine.FindAllString(installs[0], -1), conditionLine.FindAllString(steps[first], -1); !slices.Equal(got, want) {
		t.Errorf("the install runs under %q and the measurement under %q", got, want)
	}
}

// TestWorkflows_EveryJobThatMeasures_InstallsGobcoOnceFirst verifies that
// every job of every workflow that runs the condition recipe, itself, through
// measure_conditions or through make, installs gobco once, through this
// command, in the step just before the first that measures, so a measurement
// needs no network and the release stays pinned in one place (issue 1515).
// Every step counts, named or not, so a job that measures is found whatever
// its steps are called, and nothing runs between the install and the
// measurement it serves. The install is bounded by its three attempts and by
// a timeout of its own, and runs on the legs the measurement runs on. No
// workflow names a gobco release of its own.
func TestWorkflows_EveryJobThatMeasures_InstallsGobcoOnceFirst(t *testing.T) {
	t.Parallel()
	workflows, err := filepath.Glob(filepath.Join(repository, ".github", "workflows", "*.y*ml"))
	if err != nil || len(workflows) == 0 {
		t.Fatalf("found no workflow: %v", err)
	}
	var found []string
	for _, workflow := range workflows {
		name := filepath.Base(workflow)
		text := readRepositoryFile(t, ".github/workflows/"+name)
		if strings.Contains(text, "rillig/gobco@") {
			t.Errorf("%s pins a gobco release of its own", name)
		}
		for job, steps := range workflowJobs(text) {
			first := slices.IndexFunc(steps, measuringStep.MatchString)
			if first < 0 {
				continue
			}
			found = append(found, name+" "+job)
			t.Run(name+" "+job, func(t *testing.T) {
				t.Parallel()
				holdInstallBefore(t, steps, first)
			})
		}
	}
	// The two jobs known to measure, so a reader that found nothing fails
	// rather than holding nothing to anything.
	for _, known := range []string{"ci.yml cross_platform", "conditions.yml measure"} {
		t.Run(known, func(t *testing.T) {
			t.Parallel()
			if !slices.Contains(found, known) {
				t.Errorf("found the measuring jobs %q, want %q among them", found, known)
			}
		})
	}
}
