// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
