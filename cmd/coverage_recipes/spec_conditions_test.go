// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/conditions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// specPackage is one package a recorded run changed: the action_specs.go the
// change leaves, nil when it removes the file, and how the condition recipe
// answers for it.
type specPackage struct {
	Specs  *string `json:"specs"`
	Output string  `json:"output"`
	Status int     `json:"status"`
}

// specRun is one run of scripts/check-spec-conditions.sh: the packages its
// branch changed, the base it was given when it is not the commit before, and
// what it did.
type specRun struct {
	Name     string                 `json:"name"`
	Packages map[string]specPackage `json:"packages"`
	Base     string                 `json:"base"`
	Status   int                    `json:"status"`
	Stdout   string                 `json:"stdout"`
	Stderr   string                 `json:"stderr"`
}

// specRecord is testdata/spec_conditions_runs.json.
type specRecord struct {
	Runs []specRun `json:"runs"`
}

// specBench is a throwaway git repository with a session over it whose
// condition recipe answers as a recorded run says.
type specBench struct {
	repo     string
	session  *session
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	answers  map[string]specPackage
	requests []conditions.Request
}

// newSpecBench makes the repository. It reads no configuration of the person
// running the tests, so a signing key or a hook of theirs changes nothing.
func newSpecBench(t *testing.T) *specBench {
	t.Helper()
	b := &specBench{repo: t.TempDir(), answers: map[string]specPackage{}}
	environ := []string{
		"GIT_CONFIG_GLOBAL=" + filepath.Join(b.repo, "absent"), "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.com",
	}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			environ = append(environ, entry)
		}
	}
	b.session = &session{
		Run: recipe.Exec, LookPath: exec.LookPath, measure: b.measure,
		dir: b.repo, environ: environ, stdout: &b.stdout, stderr: &b.stderr,
	}
	b.git(t, "init", "-q", "-b", "main")
	return b
}

// measure answers for the condition recipe.
func (b *specBench) measure(_ context.Context, _ conditions.Tools, req conditions.Request) int {
	b.requests = append(b.requests, req)
	answer := b.answers[strings.TrimPrefix(req.Package, "./internal/tools/")]
	_, _ = io.WriteString(req.Stdout, answer.Output)
	return answer.Status
}

// git runs git in the repository and returns what it wrote.
func (b *specBench) git(t *testing.T, args ...string) string {
	t.Helper()
	bin, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	out, err := recipe.Output(t.Context(), recipe.Exec, recipe.Call{Program: bin, Args: args, Dir: b.repo, Env: b.session.environ, Stderr: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

// commit commits everything the repository holds and returns the commit.
func (b *specBench) commit(t *testing.T, message string) string {
	t.Helper()
	b.git(t, "add", "-A")
	b.git(t, "commit", "-q", "--allow-empty", "-m", message)
	return b.git(t, "rev-parse", "HEAD")
}

// specsPath is the action_specs.go of a package.
func specsPath(pkg string) string {
	return "internal/tools/" + pkg + "/action_specs.go"
}

// change commits a base holding every package of the answers and a change of
// each, and returns the base.
func (b *specBench) change(t *testing.T, packages map[string]specPackage) string {
	t.Helper()
	b.answers = packages
	for pkg := range packages {
		writeFile(t, b.repo, specsPath(pkg), "package "+pkg+"\n")
	}
	base := b.commit(t, "base")
	for pkg, change := range packages {
		if change.Specs == nil {
			b.git(t, "rm", "-q", specsPath(pkg))
			continue
		}
		writeFile(t, b.repo, specsPath(pkg), *change.Specs)
	}
	b.commit(t, "change")
	return base
}

// TestRecordedRuns_SpecConditions_DoesWhatTheScriptDid replays every recorded
// run of scripts/check-spec-conditions.sh: the packages a branch changed, and
// the condition recipe's answer for each, give the status and both streams
// the script gave, and the recipe is asked for each package that is left.
func TestRecordedRuns_SpecConditions_DoesWhatTheScriptDid(t *testing.T) {
	t.Parallel()
	var rec specRecord
	readRecord(t, "spec_conditions_runs.json", &rec)
	if len(rec.Runs) == 0 {
		t.Fatal("the record holds no run")
	}
	for _, run := range rec.Runs {
		t.Run(run.Name, func(t *testing.T) {
			t.Parallel()
			b := newSpecBench(t)
			base := b.change(t, run.Packages)
			if run.Base != "" {
				base = run.Base
			}
			if status := b.session.run(t.Context(), []string{"spec-conditions", base}); status != run.Status {
				t.Errorf("status = %d, want %d", status, run.Status)
			}
			wantStdout := run.Stdout
			if len(run.Packages) == 0 && run.Base == "" {
				wantStdout = "check-spec-conditions: no action_specs.go changed against " + base + "\n"
			}
			if got := b.stdout.String(); got != wantStdout {
				t.Errorf("stdout =\n%s\nwant\n%s", got, wantStdout)
			}
			if got := b.stderr.String(); got != run.Stderr {
				t.Errorf("stderr =\n%s\nwant\n%s", got, run.Stderr)
			}
		})
	}
}

// oneSided is the packages of a branch that changed one, whose recipe measured
// it and named a condition no test decides.
func oneSided() map[string]specPackage {
	specs := "package alpha\n\nfunc f(c int) bool {\n\treturn c == 1\n}\n"
	return map[string]specPackage{"alpha": {Specs: &specs, Output: "\nCondition coverage: 3/4\naction_specs.go:4:9: condition \"c == 1\" was never evaluated\n"}}
}

// TestSpecConditions_TheRecipe_IsRunWithNoGateOfItsOwn verifies that each
// package is measured from the repository's root with the environment the gate
// was given, but for a gate and a mode of the recipe's own: the gate reads the
// report itself, so an exported GOBCO_GATE must not turn a measured package
// into one that was not, nor GOBCO_MODE leave it without the figure it reads.
func TestSpecConditions_TheRecipe_IsRunWithNoGateOfItsOwn(t *testing.T) {
	t.Parallel()
	b := newSpecBench(t)
	base := b.change(t, oneSided())
	b.session.environ = append(b.session.environ, "GOBCO_GATE=all", "GOBCO_MODE=branch", "GOBCO_TEST_FLAGS=-timeout=30m")
	if status := b.session.run(t.Context(), []string{"spec-conditions", base}); status != 1 {
		t.Errorf("status = %d, want 1 for the finding", status)
	}
	if len(b.requests) != 1 {
		t.Fatalf("measured %d package(s), want one", len(b.requests))
	}
	req := b.requests[0]
	root, err := filepath.EvalSymlinks(b.repo)
	if err != nil {
		t.Fatal(err)
	}
	if got, evalErr := filepath.EvalSymlinks(req.Dir); evalErr != nil || got != root || req.Package != "./internal/tools/alpha" || req.Tags != "" {
		t.Errorf("asked for %s with tags %q in %s (%v), want the package in %s", req.Package, req.Tags, req.Dir, evalErr, root)
	}
	for name, want := range map[string]string{"GOBCO_GATE": "", "GOBCO_MODE": "condition", "GOBCO_TEST_FLAGS": "-timeout=30m"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, set := recipe.Getenv(req.Environ, name); got != want || !set {
				t.Errorf("%s = %q (set %t), want %q", name, got, set, want)
			}
		})
	}
}

// TestSpecConditions_ARootThatEndsInABlank_IsReadWhole verifies that a checkout
// whose name ends in a space is the directory the changes are read in and the
// package measured from.
func TestSpecConditions_ARootThatEndsInABlank_IsReadWhole(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("a file name does not end in a space there")
	}
	b := newSpecBench(t)
	base := b.change(t, oneSided())
	root, err := filepath.EvalSymlinks(b.repo)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "checkout ")
	if err = os.Mkdir(moved, 0o750); err != nil {
		t.Fatal(err)
	}
	err = errors.Join(
		os.Rename(filepath.Join(root, ".git"), filepath.Join(moved, ".git")),
		os.Rename(filepath.Join(root, "internal"), filepath.Join(moved, "internal")),
	)
	if err != nil {
		t.Fatal(err)
	}
	b.session.dir = moved
	if status := b.session.run(t.Context(), []string{"spec-conditions", base}); status != 1 || len(b.requests) != 1 {
		t.Fatalf("status = %d after measuring %d package(s), want 1 for the finding; stderr %q", status, len(b.requests), b.stderr.String())
	}
	if got := b.requests[0].Dir; got != moved {
		t.Errorf("measured in %q, want %q", got, moved)
	}
}

// TestSpecConditions_EveryFigure_IsShown verifies that each figure of a report
// is printed, and not the other lines of it.
func TestSpecConditions_EveryFigure_IsShown(t *testing.T) {
	t.Parallel()
	b := newSpecBench(t)
	packages := oneSided()
	packages["alpha"] = specPackage{Specs: packages["alpha"].Specs, Output: "noise\nCondition coverage: 1/2\r\nnoise\nCondition coverage: 4/4\n"}
	base := b.change(t, packages)
	if status := b.session.run(t.Context(), []string{"spec-conditions", base}); status != 0 {
		t.Errorf("status = %d, want 0; stderr %q", status, b.stderr.String())
	}
	want := "check-spec-conditions: measuring internal/tools/alpha\nCondition coverage: 1/2\nCondition coverage: 4/4\ncheck-spec-conditions: every measured condition is evaluated both ways\n"
	if b.stdout.String() != want {
		t.Errorf("stdout = %q, want %q", b.stdout.String(), want)
	}
}

// TestSpecConditions_NoBase_IsTheMainBranchOfOrigin verifies the base a branch
// is read against when none is given, or an empty one as an unset make
// variable is.
func TestSpecConditions_NoBase_IsTheMainBranchOfOrigin(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"spec-conditions"}, {"spec-conditions", ""}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			b := newSpecBench(t)
			base := b.change(t, oneSided())
			if status := b.session.run(t.Context(), args); status != 2 || b.stderr.String() != "check-spec-conditions: no such ref: origin/main\n" {
				t.Errorf("status = %d, stderr = %q; want 2 and the ref named", status, b.stderr.String())
			}
			b.stderr.Reset()
			b.git(t, "update-ref", "refs/remotes/origin/main", base)
			if status := b.session.run(t.Context(), args); status != 1 || len(b.requests) != 1 {
				t.Errorf("status = %d after measuring %d package(s), want 1 after one", status, len(b.requests))
			}
		})
	}
}

// TestSpecConditions_AnotherDirectory_ReadsTheSameRepository verifies that the
// gate finds the repository's root from a directory below it.
func TestSpecConditions_AnotherDirectory_ReadsTheSameRepository(t *testing.T) {
	t.Parallel()
	b := newSpecBench(t)
	base := b.change(t, oneSided())
	b.session.dir = filepath.Join(b.repo, "internal", "tools")
	if status := b.session.run(t.Context(), []string{"spec-conditions", base}); status != 1 || !strings.Contains(b.stderr.String(), "internal/tools/alpha/action_specs.go:4:9") {
		t.Errorf("status = %d, stderr = %q; want the finding", status, b.stderr.String())
	}
}

// TestSpecConditions_ChangesThatCannotBeRead_AreRefused verifies that the gate
// passes nothing it could not read: no git, a directory that is no repository,
// a base with no history in common with the branch, for which git diff fails
// and the script's substitution read no change, and a changed file that cannot
// be read.
func TestSpecConditions_ChangesThatCannotBeRead_AreRefused(t *testing.T) {
	t.Parallel()
	t.Run("no git", func(t *testing.T) {
		t.Parallel()
		b := newSpecBench(t)
		b.session.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
		if status := b.session.run(t.Context(), []string{"spec-conditions"}); status != 2 || !strings.HasPrefix(b.stderr.String(), "check-spec-conditions: find git: ") {
			t.Errorf("status = %d, stderr = %q", status, b.stderr.String())
		}
	})
	t.Run("no repository", func(t *testing.T) {
		t.Parallel()
		b := newSpecBench(t)
		b.session.dir = t.TempDir()
		b.session.environ = append(b.session.environ, "GIT_CEILING_DIRECTORIES="+filepath.Dir(b.session.dir))
		if status := b.session.run(t.Context(), []string{"spec-conditions"}); status != 2 || !strings.Contains(b.stderr.String(), "check-spec-conditions: the repository's root could not be found: ") {
			t.Errorf("status = %d, stderr = %q", status, b.stderr.String())
		}
	})
	t.Run("no history in common", func(t *testing.T) {
		t.Parallel()
		b := newSpecBench(t)
		base := b.change(t, oneSided())
		b.git(t, "checkout", "-q", "--orphan", "other")
		b.commit(t, "unrelated")
		if status := b.session.run(t.Context(), []string{"spec-conditions", base}); status != 2 || !strings.Contains(b.stderr.String(), "check-spec-conditions: the changes against "+base+" could not be read: ") {
			t.Errorf("status = %d, stderr = %q", status, b.stderr.String())
		}
	})
	t.Run("a file that cannot be read", func(t *testing.T) {
		t.Parallel()
		b := newSpecBench(t)
		base := b.change(t, oneSided())
		file := filepath.Join(b.repo, filepath.FromSlash(specsPath("alpha")))
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(file, 0o750); err != nil {
			t.Fatal(err)
		}
		if status := b.session.run(t.Context(), []string{"spec-conditions", base}); status != 2 || !strings.HasPrefix(b.stderr.String(), "check-spec-conditions: ") || len(b.requests) != 0 {
			t.Errorf("status = %d, stderr = %q", status, b.stderr.String())
		}
	})
}

// TestSpecConditions_AnInterrupt_EndsTheGate verifies that a signal while a
// package is measured ends the gate with the status a shell gives it, measures
// no other package and says nothing of one it did not measure.
func TestSpecConditions_AnInterrupt_EndsTheGate(t *testing.T) {
	t.Parallel()
	b := newSpecBench(t)
	packages := oneSided()
	packages["beta"] = packages["alpha"]
	base := b.change(t, packages)
	ctx, interrupt := interruptible(t)
	b.session.measure = func(_ context.Context, _ conditions.Tools, req conditions.Request) int {
		b.requests = append(b.requests, req)
		interrupt(os.Interrupt)
		return 130
	}
	if status := b.session.run(ctx, []string{"spec-conditions", base}); status != 130 || len(b.requests) != 1 {
		t.Errorf("status = %d after measuring %d package(s), want 130 after one", status, len(b.requests))
	}
	if b.stdout.String() != "check-spec-conditions: measuring internal/tools/alpha\n" || b.stderr.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q", b.stdout.String(), b.stderr.String())
	}
}

// TestDeclared_EachPosition_IsAnsweredByItsLineOrTheOneAbove verifies which
// declarations answer a condition, at the first line and at the last, and that
// a position outside the file, or one that is no line, is answered by none.
func TestDeclared_EachPosition_IsAnsweredByItsLineOrTheOneAbove(t *testing.T) {
	t.Parallel()
	const answer = "x // gobco: always true"
	cases := []struct {
		name     string
		lines    []string
		position string
		want     bool
	}{
		{"its line", []string{"a", answer, "b"}, "2:9: c", true},
		{"the line above", []string{"a", answer, "b"}, "3:9: c", true},
		{"two above", []string{answer, "a", "b"}, "3:9: c", false},
		{"the line below", []string{"a", answer}, "1:9: c", false},
		{"the first line", []string{answer}, "1:9: c", true},
		{"the last line", []string{"a", "b", answer}, "3:9: c", true},
		{"past the file", []string{answer}, "2:9: c", false},
		{"line zero", []string{answer}, "0:9: c", false},
		{"no line", []string{answer}, "x:9: c", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := declared(tc.lines, tc.position); got != tc.want {
				t.Errorf("declared(%q, %q) = %t, want %t", tc.lines, tc.position, got, tc.want)
			}
		})
	}
}
