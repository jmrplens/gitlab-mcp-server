// main_test.go covers the command end to end over real repositories: what it
// judges, what it prints and the status it exits with, the arguments it
// refuses, and the CI job and the make target that run it.
package main

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"
)

// runCase is one repository and command line of the table below, and what the
// run must do with them. setup builds the repository and returns it and the
// arguments after -dir.
type runCase struct {
	name       string
	setup      func(t *testing.T) (dir string, args []string)
	wantCode   int
	wantStdout []string
	wantStderr string
}

// check runs the case and reports every way the run departed from it: the
// exit code, each fragment stdout must hold, and stderr, which must be empty
// or hold wantStderr.
func (tc runCase) check(t *testing.T) {
	t.Helper()

	dir, args := tc.setup(t)
	args = append([]string{"-dir", dir}, args...)
	var stdout, stderr bytes.Buffer
	code := runMain(context.Background(), args, &stdout, &stderr)
	if code != tc.wantCode {
		t.Errorf("runMain(%q) = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, code, tc.wantCode, stdout.String(), stderr.String())
	}
	for _, want := range tc.wantStdout {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout does not hold %q:\n%s", want, stdout.String())
		}
	}
	if tc.wantStderr == "" && stderr.Len() != 0 || !strings.Contains(stderr.String(), tc.wantStderr) {
		t.Errorf("stderr = %q, want it to hold %q", stderr.String(), tc.wantStderr)
	}
}

// authored is a commit message and the identity it is committed as.
type authored struct {
	author  identity
	message string
}

// by pairs a commit message with its author for branch.
func by(author identity, message string) authored { return authored{author: author, message: message} }

// branch is a repository whose root is followed by the given commits, and
// the root and the last commit.
func branch(t *testing.T, commits ...authored) (dir, base, head string) {
	t.Helper()

	dir = newRepository(t)
	base = commitAs(t, dir, jane, "root\n")
	head = base
	for _, each := range commits {
		head = commitAs(t, dir, each.author, each.message)
	}
	return dir, base, head
}

// sideAndTrunk is a repository whose root is followed by a signed commit on a
// branch named side and another on the trunk, which is checked out, and the
// root. The two commits add different files, so they merge without a
// conflict.
func sideAndTrunk(t *testing.T) (dir, base string) {
	t.Helper()

	dir, base, _ = branch(t)
	trunk := gitFixture(t, dir, jane, "symbolic-ref", "--short", "HEAD")
	gitFixture(t, dir, jane, "checkout", "--quiet", "-b", "side")
	commitFile(t, dir, jane, "side.txt", "side\n", signedMessage("feat: side", jane))
	gitFixture(t, dir, jane, "checkout", "--quiet", trunk)
	commitFile(t, dir, jane, "trunk.txt", "trunk\n", signedMessage("feat: trunk", jane))
	return dir, base
}

// dependabotSignOff is the sign-off Dependabot writes into its commits, which
// names an address other than the one its commits are authored with.
var dependabotSignOff = identity{name: "dependabot[bot]", email: "support@github.com"}

// TestRunMain_Repositories_JudgeTheCommitsOfTheRange verifies the runs over
// real repositories: a branch whose every commit is signed off by its author
// passes, one with a commit that is not fails with the remedy naming the
// base, an empty range passes with nothing judged, an unsigned merge is not
// judged when it brings no change of its own and fails when it does, a
// sign-off git reads with the separator byte at its end names nobody, a
// Dependabot commit is exempt only in a pull request Dependabot opened,
// -annotate adds the workflow commands, and the defaults read
// origin/main..HEAD.
func TestRunMain_Repositories_JudgeTheCommitsOfTheRange(t *testing.T) {
	tests := []runCase{
		{
			name: "every commit signed off by its author",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, head := branch(t, by(jane, signedMessage("feat: one", jane)), by(john, signedMessage("fix: two", john)))
				return dir, []string{"-base", base, "-head", head}
			},
			wantStdout: []string{"audit_dco: 2 commit(s) in ", "  ok      ", "feat: one", "fix: two", "audit_dco: 2 signed off by the author, 0 not, 0 exempt, 0 merge(s) not judged\n"},
		},
		{
			name: "a commit without a sign-off",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, head := branch(t, by(jane, signedMessage("feat: one", jane)), by(jane, "fix: two\n"))
				return dir, []string{"-base", base, "-head", head}
			},
			wantCode:   1,
			wantStdout: []string{"  FAIL    ", "fix: two\n          carries no Signed-off-by trailer", "1 signed off by the author, 1 not", "git rebase --keep-base --signoff "},
		},
		{
			name: "a commit signed off by somebody else",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, head := branch(t, by(john, signedMessage("docs: three", jane)))
				return dir, []string{"-base", base, "-head", head}
			},
			wantCode:   1,
			wantStdout: []string{"is signed off by Jane Doe <jane@example.org>, and not by its author"},
		},
		{
			name: "an empty range",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, _ := branch(t)
				return dir, []string{"-base", base, "-head", base}
			},
			wantStdout: []string{"audit_dco: no commits in ", "so nothing to judge"},
		},
		{
			name: "an unsigned merge that brings no change of its own",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base := sideAndTrunk(t)
				gitFixture(t, dir, jane, "merge", "--quiet", "--no-ff", "--no-edit", "--no-gpg-sign", "side")
				return dir, []string{"-base", base}
			},
			wantStdout: []string{"  merge   ", "a merge commit that brings no change of its own, which is not judged", "2 signed off by the author, 0 not, 0 exempt, 1 merge(s) not judged"},
		},
		{
			name: "an unsigned merge with a change of its own",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base := sideAndTrunk(t)
				mergeWithChange(t, dir, jane, "side", "Merge branch 'side'\n")
				return dir, []string{"-base", base}
			},
			wantCode:   1,
			wantStdout: []string{"  FAIL    ", "Merge branch 'side'\n          carries no Signed-off-by trailer", "2 signed off by the author, 1 not, 0 exempt, 0 merge(s) not judged"},
		},
		{
			name: "a sign-off the separator byte ends",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, head := branch(t, by(jane, "feat: one\n\nSigned-off-by: Jane Doe <jane@example.org>\x1e\n"))
				return dir, []string{"-base", base, "-head", head}
			},
			wantCode:   1,
			wantStdout: []string{"is signed off by Jane Doe <jane@example.org>�, and not by its author"},
		},
		{
			name: "Dependabot's commit in a pull request Dependabot opened",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, head := branch(t, by(dependabot, signedMessage("chore(deps): bump", dependabotSignOff)))
				return dir, []string{"-base", base, "-head", head, "-opened-by", "dependabot[bot]"}
			},
			wantStdout: []string{"  exempt  ", "0 signed off by the author, 0 not, 1 exempt"},
		},
		{
			name: "Dependabot's commit in a pull request a person opened",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, head := branch(t, by(dependabot, signedMessage("chore(deps): bump", dependabotSignOff)))
				return dir, []string{"-base", base, "-head", head, "-opened-by", "jmrplens"}
			},
			wantCode:   1,
			wantStdout: []string{"carries the identity of dependabot[bot], whose commits are exempt only in a pull request dependabot[bot] opened, and jmrplens opened this one"},
		},
		{
			name: "annotated",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, head := branch(t, by(jane, "fix: two\n"))
				return dir, []string{"-base", base, "-head", head, "-annotate"}
			},
			wantCode:   1,
			wantStdout: []string{"\n::error title=DCO sign-off::", " by Jane Doe <jane@example.org> carries no Signed-off-by trailer\n"},
		},
		{
			name: "the defaults, origin/main..HEAD",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, _ := branch(t, by(jane, signedMessage("feat: one", jane)))
				gitFixture(t, dir, jane, "update-ref", "refs/remotes/origin/main", base)
				return dir, nil
			},
			wantStdout: []string{"audit_dco: 1 commit(s) in origin/main..HEAD (", "  ok      "},
		},
		{
			name: "revisions named by name",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, head := branch(t, by(jane, "fix: two\n"))
				gitFixture(t, dir, jane, "branch", "layer-below", base)
				return dir, []string{"-base", "layer-below", "-head", head[:12]}
			},
			wantCode:   1,
			wantStdout: []string{"in layer-below..", "git rebase --keep-base --signoff layer-below\n"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.check)
	}
}

// TestRunMain_UnreadableRange_FailsWithTheReasonOnStderr verifies the runs
// git cannot answer: a base or a head naming no commit, which also says what
// a shallow clone lacks, a directory that is not a repository, a range git
// log refuses to read, and a merge git cannot remerge.
func TestRunMain_UnreadableRange_FailsWithTheReasonOnStderr(t *testing.T) {
	tests := []runCase{
		{
			name: "base names no commit",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, _, _ := branch(t)
				return dir, []string{"-base", "no-such-branch"}
			},
			wantCode:   1,
			wantStderr: "audit_dco: -base no-such-branch names no commit in ",
		},
		{
			name: "base missing from a shallow clone",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, _, _ := branch(t)
				return dir, []string{"-base", strings.Repeat("0", 40)}
			},
			wantCode:   1,
			wantStderr: "fetch-depth: 0",
		},
		{
			name: "head names no commit",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, _ := branch(t)
				return dir, []string{"-base", base, "-head", "no-such-branch"}
			},
			wantCode:   1,
			wantStderr: "audit_dco: -head no-such-branch names no commit in ",
		},
		{
			name: "not a repository",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				return t.TempDir(), []string{"-base", "HEAD"}
			},
			wantCode:   1,
			wantStderr: "names no commit",
		},
		{
			name: "a range git log cannot read",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, head := branch(t, by(jane, signedMessage("feat: one", jane)))
				gitFixture(t, dir, jane, "config", "log.showSignature", "maybe")
				return dir, []string{"-base", base, "-head", head}
			},
			wantCode:   1,
			wantStderr: "audit_dco: could not read the commits of ",
		},
		{
			name: "a merge git cannot remerge",
			setup: func(t *testing.T) (string, []string) {
				t.Helper()
				dir, base, head, _ := mergeMissingATree(t)
				return dir, []string{"-base", base, "-head", head}
			},
			wantCode:   1,
			wantStderr: "audit_dco: could not read the commits of ",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.check)
	}
}

// TestRunMain_Arguments_RefuseWhatTheyCannotUse verifies the usage errors,
// which exit 2 before git is asked anything: an unknown flag, a positional
// argument, and a revision that is empty or that git would read as an option.
func TestRunMain_Arguments_RefuseWhatTheyCannotUse(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "unknown flag", args: []string{"-no-such-flag"}, wantStderr: "flag provided but not defined"},
		{name: "positional argument", args: []string{"main"}, wantStderr: `audit_dco: takes no arguments, and was given ["main"]; name the range with -base and -head`},
		{name: "empty base", args: []string{"-base", ""}, wantStderr: `audit_dco: -base "" is not a revision`},
		{name: "empty head", args: []string{"-head", ""}, wantStderr: `audit_dco: -head "" is not a revision`},
		{name: "base read as an option", args: []string{"-base", "--output=x"}, wantStderr: `audit_dco: -base "--output=x" is not a revision`},
		{name: "head read as an option", args: []string{"-head", "-n1"}, wantStderr: `audit_dco: -head "-n1" is not a revision`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"-dir", t.TempDir()}, tc.args...)
			if code := runMain(context.Background(), args, &stdout, &stderr); code != 2 {
				t.Errorf("runMain(%q) = %d, want 2", args, code)
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) || stdout.Len() != 0 {
				t.Errorf("stdout = %q, stderr = %q; want nothing on stdout and %q on stderr", stdout.String(), stderr.String(), tc.wantStderr)
			}
		})
	}
}

// TestMain_UnknownFlag_ExitsWithTheUsageStatus verifies that main hands the
// process arguments after its own name to runMain and passes the status on to
// the exit seam.
func TestMain_UnknownFlag_ExitsWithTheUsageStatus(t *testing.T) {
	previousArgs, previousExit := os.Args, osExit
	t.Cleanup(func() { os.Args, osExit = previousArgs, previousExit })
	got := -1
	os.Args = []string{"audit_dco", "-no-such-flag"}
	osExit = func(code int) { got = code }
	main()
	if got != 2 {
		t.Errorf("main() exit = %d, want 2", got)
	}
}

// ciJob is one job of .github/workflows/ci.yml as the contracts below read it.
type ciJob struct {
	If          string            `yaml:"if"`
	Needs       []string          `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	Steps       []struct {
		Name string            `yaml:"name"`
		Uses string            `yaml:"uses"`
		With map[string]any    `yaml:"with"`
		Run  string            `yaml:"run"`
		Env  map[string]string `yaml:"env"`
	} `yaml:"steps"`
}

// loadCIJob reads one job of ci.yml, failing the test when the workflow does
// not have it.
func loadCIJob(t *testing.T, name string) ciJob {
	t.Helper()

	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]yaml.Node `yaml:"jobs"`
	}
	if err = yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	node, ok := workflow.Jobs[name]
	if !ok {
		t.Fatalf("ci.yml has no %s job", name)
	}
	var job ciJob
	if err = node.Decode(&job); err != nil {
		t.Fatalf("ci.yml's %s job: %v", name, err)
	}
	return job
}

// TestCIWorkflow_DCOJob_RunsTheCheckOnEveryPullRequest holds ci.yml's dco job
// to this command: it runs on every pull request and on nothing else, with
// the whole history checked out and no permission beyond the workflow's read
// token, and hands the make target the pull request's base and head commits
// and the login that opened it, with annotations.
func TestCIWorkflow_DCOJob_RunsTheCheckOnEveryPullRequest(t *testing.T) {
	job := loadCIJob(t, "dco")
	if job.If != "github.event_name == 'pull_request'" {
		t.Errorf("dco job if = %q, want it to run on pull requests alone", job.If)
	}
	if len(job.Permissions) != 0 {
		t.Errorf("dco job permissions = %v, want none beyond the workflow's", job.Permissions)
	}
	checkedOut, ran := false, false
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/checkout@") {
			checkedOut = step.With["fetch-depth"] == 0 && step.With["persist-credentials"] == false
		}
		if step.Run == "make check-dco DCO_FLAGS=-annotate" {
			ran = step.Env["DCO_BASE"] == "${{ github.event.pull_request.base.sha }}" &&
				step.Env["DCO_HEAD"] == "${{ github.event.pull_request.head.sha }}" &&
				step.Env["DCO_OPENED_BY"] == "${{ github.event.pull_request.user.login }}"
		}
	}
	if !checkedOut {
		t.Error("dco job does not check out the whole history without persisting the token")
	}
	if !ran {
		t.Error("dco job does not run make check-dco DCO_FLAGS=-annotate with the pull request's base, head and opener")
	}
}

// TestCIWorkflow_ChecksJob_WaitsOnTheDCOJobAndPassesItsSkip holds the fan-in
// job to the dco job: it waits on it, and counts the skip a push to main
// makes as a pass.
func TestCIWorkflow_ChecksJob_WaitsOnTheDCOJobAndPassesItsSkip(t *testing.T) {
	job := loadCIJob(t, "checks")
	if !slices.Contains(job.Needs, "dco") {
		t.Errorf("checks job needs %v, want dco among them", job.Needs)
	}
	maySkip := false
	for _, step := range job.Steps {
		maySkip = maySkip || (step.Name == "Decide" && slices.Contains(strings.Fields(step.Env["MAY_SKIP"]), "dco"))
	}
	if !maySkip {
		t.Error("checks job's Decide step does not count a skipped dco job as a pass, and a push to main skips it")
	}
}

// TestMakefile_CheckDCO_HandsTheRangeAndTheOpenerToTheCommand holds the make
// target to the command line the CI job relies on, and to defaults that judge
// the branch against origin/main.
func TestMakefile_CheckDCO_HandsTheRangeAndTheOpenerToTheCommand(t *testing.T) {
	data, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(data)
	tests := []struct{ name, want string }{
		{name: "base default", want: "\nDCO_BASE ?= origin/main\n"},
		{name: "head default", want: "\nDCO_HEAD ?= HEAD\n"},
		{name: "recipe", want: "\ncheck-dco:\n\tgo run ./cmd/audit_dco/ -base \"$(DCO_BASE)\" -head \"$(DCO_HEAD)\" -opened-by \"$(DCO_OPENED_BY)\" $(DCO_FLAGS)\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(makefile, tc.want) {
				t.Errorf("Makefile does not hold %q", tc.want)
			}
		})
	}
}
