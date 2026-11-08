// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// description_test.go holds the description half (make
// check-pr-description) to the cases scripts/check_em_dash_sh_test.py held
// the script to, and to the reading of a description through gh, which runs
// against a stand-in gh: this test binary started again.
//
// This repository squash merges, so a pull request's title and body become
// the commit message on main. The gate refuses three things in them: an em
// dash, a block a review bot injected, and a command that makes GitHub skip
// workflows. GitHub skips every workflow a push triggers when the commit it
// lands on carries one of those commands anywhere in its message, so a
// description that merely explained a fixture committing with one turned off
// CI on main for its merge, and the release for the tag put on that commit
// (7baa4a40b, the merge of pull request 1094, and v3.1.0).
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/committext"
)

// skipFailure is how the skip-command finding opens.
const skipFailure = "carries a command that makes GitHub skip"

// skipCommands are the five commands GitHub documents for skipping workflows
// on push and pull_request events, built from their parts so that this file
// does not carry them as a description would.
var skipCommands = []string{"[" + "skip ci]", "[" + "ci skip]", "[" + "no ci]", "[" + "skip actions]", "[" + "actions skip]"}

// skipCI is the first of them.
var skipCI = skipCommands[0]

// environment answers a lookup from vars, and nothing else from the process.
func environment(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

// judgeFiles runs the description gate over title and body from disk, the
// rehearsal path, and returns its exit code and what it wrote to stdout and
// stderr, in that order.
func judgeFiles(t *testing.T, title, body string) (int, string) {
	t.Helper()
	dir := t.TempDir()
	titleFile, bodyFile := filepath.Join(dir, "title"), filepath.Join(dir, "body")
	writeFile(t, titleFile, []byte(title))
	writeFile(t, bodyFile, []byte(body))
	return judge(t, map[string]string{"PR_TITLE_FILE": titleFile, "PR_BODY_FILE": bodyFile})
}

// judge runs the description gate with vars as its environment.
func judge(t *testing.T, vars map[string]string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), []string{"description"}, t.TempDir(), environment(vars), &stdout, &stderr)
	return code, stdout.String() + stderr.String()
}

// writeFile writes a scratch file of a test.
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil { //#nosec G703 -- a path below the test's own t.TempDir
		t.Fatal(err)
	}
}

// onlyOnPath leaves PATH holding one directory, which holds a program named
// name that is never started, and returns the path a lookup of name finds
// there. It changes the process's PATH, so a test calling it does not run in
// parallel.
func onlyOnPath(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		// A lookup on Windows finds a program by an extension PATHEXT
		// lists, never by its bare name.
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o700); err != nil { //#nosec G306 G703 -- a program the lookup must find executable, below the test's own t.TempDir
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

// TestDescription_ACleanDescription_Passes verifies the passing answer, which
// names what it judged.
func TestDescription_ACleanDescription_Passes(t *testing.T) {
	t.Parallel()

	code, output := judgeFiles(t, "Refuse a skip command in a pull request description", "Plain prose, no em dash, no generated block.\n")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, output)
	}
	for _, want := range []string{
		"Judging the pull request title and body (files on disk).",
		"OK: the title and body carry no em dash, no injected block and no skip command.",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output lacks %q:\n%s", want, output)
		}
	}
}

// TestDescription_EachSkipCommandInTheBody_FailsAndNamesItsLine verifies
// each of the five commands, reported with the line it is on.
func TestDescription_EachSkipCommandInTheBody_FailsAndNamesItsLine(t *testing.T) {
	t.Parallel()

	for _, command := range skipCommands {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			code, output := judgeFiles(t, "A title", "first line\nthe fixture commits with "+command+"\n")
			if code != 1 || !strings.Contains(output, "body:2:the fixture commits with "+command) || !strings.Contains(output, skipFailure) {
				t.Errorf("exit %d, want 1, the line and the finding\n%s", code, output)
			}
		})
	}
}

// TestDescription_ASkipCommandInTheTitle_FailsInTheEmDashFormat verifies
// that the title, one line, is reported whole.
func TestDescription_ASkipCommandInTheTitle_FailsInTheEmDashFormat(t *testing.T) {
	t.Parallel()

	code, output := judgeFiles(t, "Commit fixtures with "+skipCI, "body\n")
	if code != 1 || !strings.Contains(output, "title: Commit fixtures with "+skipCI) || !strings.Contains(output, skipFailure) {
		t.Errorf("exit %d, want 1, the title and the finding\n%s", code, output)
	}
}

// TestDescription_ASkipCommand_IsMatchedInAnyCaseSpacingAndInsideBackticks
// verifies the spellings GitHub may read besides the documented one.
func TestDescription_ASkipCommand_IsMatchedInAnyCaseSpacingAndInsideBackticks(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"commits with `" + skipCI + "`", "commits with [" + "Skip CI]", "commits with [" + "NO CI]",
		"commits with [" + "skip  ci]", "commits with [" + "actions\tskip]", "commits with [" + " skip ci ]",
	} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()

			code, output := judgeFiles(t, "A title", text+"\n")
			if code != 1 || !strings.Contains(output, "body:1:"+text) || !strings.Contains(output, skipFailure) {
				t.Errorf("exit %d, want 1, the line and the finding\n%s", code, output)
			}
		})
	}
}

// TestDescription_ACommandNamedWithoutItsBrackets_Passes verifies that prose
// naming a command is not refused.
func TestDescription_ACommandNamedWithoutItsBrackets_Passes(t *testing.T) {
	t.Parallel()

	if code, output := judgeFiles(t, "A title", "the fixture commits with skip ci in square brackets\n"); code != 0 {
		t.Errorf("exit %d, want 0\n%s", code, output)
	}
}

// TestDescription_ASkipChecksTrailerAtTheStartOfALine_Fails verifies the
// trailer in each spelling, reported with its line.
func TestDescription_ASkipChecksTrailerAtTheStartOfALine_Fails(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, body, location string }{
		{name: "no space", body: "body\n\n\nskip-checks:true\n", location: "body:4:"},
		{name: "one space", body: "body\n\n\nskip-checks: true\n", location: "body:4:"},
		{name: "any case", body: "body\n\n\nSkip-Checks: TRUE\n", location: "body:4:"},
		{name: "in backticks", body: "body\n`skip-checks: true`\n", location: "body:2:"},
		{name: "before another trailer", body: "body\n\nskip-checks: true\nReviewed-by: A <a@example.com>\n", location: "body:3:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, output := judgeFiles(t, "A title", tc.body)
			if code != 1 || !strings.Contains(output, tc.location) || !strings.Contains(output, skipFailure) {
				t.Errorf("exit %d, want 1 at %s\n%s", code, tc.location, output)
			}
		})
	}
}

// TestDescription_ASkipChecksValueThatIsNotTheWordTrue_Passes verifies the
// whole-word rule issue 1114 asks for: a value that only starts with true is
// not the trailer, and prose naming the trailer mid-sentence is not one
// either.
func TestDescription_ASkipChecksValueThatIsNotTheWordTrue_Passes(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"trueish":                   "body\n\nskip-checks: trueish\n",
		"named inside a sentence":   "GitHub also reads a skip-checks: true trailer at the end.\n",
		"true followed by a period": "body\n\nskip-checks: true.\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if code, output := judgeFiles(t, "A title", body); code != 0 {
				t.Errorf("exit %d, want 0\n%s", code, output)
			}
		})
	}
}

// TestDescription_ASkipCommandBesideAByteInvalidInUTF8_IsStillJudged
// verifies that a body that is not valid UTF-8 is read as text all the same,
// where grep would have called it binary and printed nothing.
func TestDescription_ASkipCommandBesideAByteInvalidInUTF8_IsStillJudged(t *testing.T) {
	t.Parallel()

	bodyFile := filepath.Join(t.TempDir(), "body")
	writeFile(t, bodyFile, []byte("x\ncaf\xe9 commits with "+skipCI+"\n"))
	code, output := judge(t, map[string]string{"PR_BODY_FILE": bodyFile})
	if code != 1 || !strings.Contains(output, "body:2:") {
		t.Errorf("exit %d, want 1 at body:2\n%s", code, output)
	}
}

// TestDescription_ALineCarryingBothForms_IsReportedOnce verifies that a line
// is listed once however many skip spellings it carries.
func TestDescription_ALineCarryingBothForms_IsReportedOnce(t *testing.T) {
	t.Parallel()

	code, output := judgeFiles(t, "A title", "skip-checks: true "+skipCI+"\n")
	if code != 1 || strings.Count(output, "body:1:") != 1 {
		t.Errorf("exit %d, want 1 and body:1 once\n%s", code, output)
	}
}

// TestDescription_AnEmDash_Fails verifies the em dash in the body and in the
// title, each reported where it is.
func TestDescription_AnEmDash_Fails(t *testing.T) {
	t.Parallel()

	code, output := judgeFiles(t, "A title "+committext.EmDash+" here", "first\na sentence "+committext.EmDash+" with a dash\n")
	for _, want := range []string{"carries an em dash", "title: A title " + committext.EmDash + " here", "body:2:a sentence "} {
		if !strings.Contains(output, want) {
			t.Errorf("output lacks %q:\n%s", want, output)
		}
	}
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
}

// TestDescription_AnInjectedBlock_FailsAndIsListedInTheBodysOrder verifies
// each marker a review bot writes, and that the markers found are listed by
// line, each line once, whatever the order of the patterns that found them.
func TestDescription_AnInjectedBlock_FailsAndIsListedInTheBodysOrder(t *testing.T) {
	t.Parallel()

	body := "intro\n" +
		"<!-- walkthrough_start -->\n" +
		"## Summary by CodeRabbit\n" +
		"<!-- This is an auto-generated comment: summary by coderabbit.ai -->\n" +
		"<!-- end of auto-generated comment: summary by coderabbit.ai -->\n" +
		"<!--  description_end  -->\n" +
		"## A heading of our own\n"
	code, output := judgeFiles(t, "A title", body)
	want := "body:2:<!-- walkthrough_start -->\n" +
		"body:3:## Summary by CodeRabbit\n" +
		"body:4:<!-- This is an auto-generated comment: summary by coderabbit.ai -->\n" +
		"body:5:<!-- end of auto-generated comment: summary by coderabbit.ai -->\n" +
		"body:6:<!--  description_end  -->\n"
	if code != 1 || !strings.Contains(output, want) || !strings.Contains(output, "carries a block a review bot generated") {
		t.Errorf("exit %d, want 1 and\n%s\nin\n%s", code, want, output)
	}
	if strings.Contains(output, "body:7:") {
		t.Errorf("a heading of our own was reported:\n%s", output)
	}
}

// TestDescription_EveryFinding_IsReportedInOneRun verifies that one finding
// does not hide another.
func TestDescription_EveryFinding_IsReportedInOneRun(t *testing.T) {
	t.Parallel()

	code, output := judgeFiles(t, "A title "+committext.EmDash, "commits with "+skipCI+"\n## Summary by CodeRabbit\n")
	for _, want := range []string{"carries an em dash", "carries a block a review bot generated", skipFailure, "Re-check with: make check-pr-description"} {
		if !strings.Contains(output, want) {
			t.Errorf("output lacks %q:\n%s", want, output)
		}
	}
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
}

// TestDescription_OnlyOneFile_IsJudgedWithTheOtherEmpty verifies that either
// file alone is enough to judge from disk.
func TestDescription_OnlyOneFile_IsJudgedWithTheOtherEmpty(t *testing.T) {
	t.Parallel()

	titleFile := filepath.Join(t.TempDir(), "title")
	writeFile(t, titleFile, []byte("A title "+committext.EmDash+"\n"))
	code, output := judge(t, map[string]string{"PR_TITLE_FILE": titleFile})
	if code != 1 || !strings.Contains(output, "title: A title "+committext.EmDash+"\n") {
		t.Errorf("exit %d, want 1 and the title without its newline\n%s", code, output)
	}
}

// TestDescription_AFileThatCannotBeRead_Fails verifies that a title or body
// file that is not there stops the run, where the script judged it empty.
func TestDescription_AFileThatCannotBeRead_Fails(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"PR_TITLE_FILE", "PR_BODY_FILE"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			absent := filepath.Join(t.TempDir(), "absent")
			code, output := judge(t, map[string]string{name: absent})
			if code != 1 || !strings.Contains(output, "ERROR: read "+absent+": ") {
				t.Errorf("exit %d, want 1 and the read error\n%s", code, output)
			}
		})
	}
}

// ghStandinEnv names the directory the stand-in gh reads its answers from.
// Its presence is what makes this binary the stand-in rather than the tests.
const ghStandinEnv = "AUDIT_PR_TEXT_GH_STANDIN"

// TestMain runs the tests, or, in a process the tests started as gh, the
// stand-in.
func TestMain(m *testing.M) {
	if dir, standin := os.LookupEnv(ghStandinEnv); standin {
		os.Exit(ghStandIn(dir, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// ghStandIn answers the three calls the gate makes as gh would, from files
// under dir, and records each call it was asked: "api ..." prints payload,
// "repo view ..." prints repo and "pr view ..." prints number, and a call
// whose file is missing fails as gh does.
func ghStandIn(dir string, args []string) int {
	calls, err := os.OpenFile(filepath.Join(dir, "calls"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 125
	}
	_, _ = calls.WriteString(strings.Join(args, " ") + "\n")
	_ = calls.Close()

	answer := map[string]string{"api": "payload", "repo": "repo", "pr": "number"}[args[0]]
	data, err := os.ReadFile(filepath.Join(dir, answer))
	if answer == "" || err != nil {
		_, _ = os.Stderr.WriteString("stand-in gh: no answer for " + strings.Join(args, " ") + "\n")
		return 1
	}
	_, _ = os.Stdout.Write(data)
	return 0
}

// useGh points the gh seam at the stand-in answering from dir, for the rest
// of the test. The tests that call it do not run in parallel, since the seam
// is one variable.
func useGh(t *testing.T, dir string) {
	t.Helper()
	previous := ghCommand
	t.Cleanup(func() { ghCommand = previous })
	coverage := t.TempDir()
	ghCommand = func(ctx context.Context, args ...string) *exec.Cmd {
		// #nosec G204 G702 -- the program is this test binary, started again as the stand-in
		cmd := exec.CommandContext(ctx, os.Args[0], args...)
		cmd.Env = append(os.Environ(), ghStandinEnv+"="+dir, "GOCOVERDIR="+coverage)
		return cmd
	}
}

// calls reads what the stand-in was asked, one call per line.
func calls(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		return ""
	}
	return string(data)
}

// TestDescription_ReadThroughTheAPIWithCRLFLineEndings_IsJudged verifies the
// path CI takes, PR_NUMBER and GITHUB_REPOSITORY through one gh api call,
// over a body carrying Windows line endings, as the API returns a
// description edited in the browser.
func TestDescription_ReadThroughTheAPIWithCRLFLineEndings_IsJudged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "payload"), []byte(`{"title": "A title", "body": "first line\r\nthe fixture commits with `+skipCI+`\r\n\r\nskip-checks: true\r\n"}`))
	useGh(t, dir)

	code, output := judge(t, map[string]string{"PR_NUMBER": "7", "GITHUB_REPOSITORY": "owner/repo"})
	if got := calls(t, dir); got != "api repos/owner/repo/pulls/7\n" {
		t.Errorf("gh was asked %q, want the one api call", got)
	}
	for _, want := range []string{
		"owner/repo#7, read from the API just now",
		"body:2:the fixture commits with " + skipCI + "\n",
		// The trailer ends in a carriage return in the payload, which an
		// anchor on the end of the value would miss.
		"body:4:skip-checks: true\n",
		skipFailure,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output lacks %q:\n%s", want, output)
		}
	}
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
}

// TestDescription_TheRepositoryAndPullRequestGhKnows_AreJudged verifies the
// local path: no variable names either, so gh is asked for the repository
// and for the pull request open for the branch, and a null body is empty.
func TestDescription_TheRepositoryAndPullRequestGhKnows_AreJudged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "repo"), []byte("owner/repo\n"))
	writeFile(t, filepath.Join(dir, "number"), []byte("12\n"))
	writeFile(t, filepath.Join(dir, "payload"), []byte(`{"title": "A title", "body": null}`))
	useGh(t, dir)

	code, output := judge(t, nil)
	want := "repo view --json nameWithOwner --jq .nameWithOwner\npr view --json number --jq .number\napi repos/owner/repo/pulls/12\n"
	if got := calls(t, dir); got != want {
		t.Errorf("gh was asked\n%s\nwant\n%s", got, want)
	}
	if code != 0 || !strings.Contains(output, "owner/repo#12, read from the API just now") {
		t.Errorf("exit %d, want 0 and the source named\n%s", code, output)
	}
}

// TestDescription_TheTitleTheAPIAnswers_IsJudged verifies that the title is
// read from the payload, and that a payload with no title is judged on its
// body alone.
func TestDescription_TheTitleTheAPIAnswers_IsJudged(t *testing.T) {
	cases := []struct {
		name, payload, want string
		code                int
	}{
		{name: "a title with an em dash", payload: `{"title": "A title ` + committext.EmDash + `", "body": "clean"}`, code: 1, want: "title: A title " + committext.EmDash + "\n"},
		{name: "no title", payload: `{"title": null, "body": "clean"}`, code: 0, want: "OK: the title and body carry"},
	}
	// sequential: each case points the one gh seam at its own stand-in
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "payload"), []byte(tc.payload))
			useGh(t, dir)

			code, output := judge(t, map[string]string{"PR_NUMBER": "7", "GITHUB_REPOSITORY": "owner/repo"})
			if code != tc.code || !strings.Contains(output, tc.want) {
				t.Errorf("exit %d, want %d and %q\n%s", code, tc.code, tc.want, output)
			}
		})
	}
}

// TestDescription_NoPullRequestOpen_Passes verifies that a branch with no
// pull request has no description to land, and that the gate says so.
func TestDescription_NoPullRequestOpen_Passes(t *testing.T) {
	cases := map[string]string{
		"gh pr view fails":          "",
		"gh pr view prints nothing": "\n",
	}
	// sequential: each case points the one gh seam at its own stand-in
	for name, number := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if number != "" {
				writeFile(t, filepath.Join(dir, "number"), []byte(number))
			}
			useGh(t, dir)

			code, output := judge(t, map[string]string{"GITHUB_REPOSITORY": "owner/repo"})
			want := "No pull request is open for this branch, so there is no description to judge.\n" +
				"Name one with PR_NUMBER=<n>, or rehearse with PR_BODY_FILE=<path>.\n"
			if code != 0 || output != want {
				t.Errorf("exit %d, output %q, want 0 and %q", code, output, want)
			}
		})
	}
}

// TestDescription_AGhCallThatFails_FailsTheRun verifies that a description
// the gate could not read is a failure, never an empty description that
// passes: the repository gh cannot name, an API call that fails, and a
// payload that is not JSON.
func TestDescription_AGhCallThatFails_FailsTheRun(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		vars  map[string]string
		want  string
	}{
		{name: "no repository", vars: map[string]string{"PR_NUMBER": "7"}, want: "ERROR: gh repo view --json nameWithOwner --jq .nameWithOwner: "},
		{name: "the API refuses", vars: map[string]string{"PR_NUMBER": "7", "GITHUB_REPOSITORY": "owner/repo"}, want: "ERROR: gh api repos/owner/repo/pulls/7: "},
		{
			name:  "a payload that is not JSON",
			files: map[string]string{"payload": "<html>"},
			vars:  map[string]string{"PR_NUMBER": "7", "GITHUB_REPOSITORY": "owner/repo"},
			want:  "ERROR: read the answer of gh api repos/owner/repo/pulls/7: ",
		},
	}
	// sequential: each case points the one gh seam at its own stand-in
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tc.files {
				writeFile(t, filepath.Join(dir, name), []byte(content))
			}
			useGh(t, dir)

			code, output := judge(t, tc.vars)
			if code != 1 || !strings.Contains(output, tc.want) {
				t.Errorf("exit %d, want 1 and %q\n%s", code, tc.want, output)
			}
		})
	}
}

// TestDescription_WithoutGh_FailsAndSaysWhatElseWorks verifies that a gate
// that cannot read the description says so, and names the rehearsal path,
// whichever of its three gh calls is the first to find gh missing: a missing
// gh is never read as there being no pull request. gh is missing the way it
// is on a machine without it, the real seam looking it up on a PATH that
// holds none, so the test changes PATH and does not run in parallel.
func TestDescription_WithoutGh_FailsAndSaysWhatElseWorks(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	want := "ERROR: gh is needed to read the pull request description.\n" +
		"Install it, or set PR_TITLE_FILE and PR_BODY_FILE to judge text from disk.\n"
	for name, vars := range map[string]map[string]string{
		"gh api":       {"PR_NUMBER": "7", "GITHUB_REPOSITORY": "owner/repo"},
		"gh pr view":   {"GITHUB_REPOSITORY": "owner/repo"},
		"gh repo view": nil,
	} {
		t.Run(name, func(t *testing.T) {
			if code, output := judge(t, vars); code != 1 || output != want {
				t.Errorf("exit %d, output %q, want 1 and %q", code, output, want)
			}
		})
	}
}

// TestGhCommand_RunsGhFromThePath verifies the seam's own answer without
// starting it: the gh a lookup finds on PATH, run by its absolute path with
// the arguments as given. It changes PATH, so it does not run in parallel.
func TestGhCommand_RunsGhFromThePath(t *testing.T) {
	gh := onlyOnPath(t, "gh")

	cmd := ghCommand(t.Context(), "api", "repos/o/r/pulls/1")
	if cmd.Err != nil || cmd.Path != gh || !slices.Equal(cmd.Args, []string{gh, "api", "repos/o/r/pulls/1"}) {
		t.Errorf("command %q at %q (error %v), want %s with the arguments as given", cmd.Args, cmd.Path, cmd.Err, gh)
	}
}

// TestGhCommand_NoGhOnPath_IsTheLookupsFailure verifies that gh is found on
// PATH before it is run, and that a PATH holding none is the error running
// the command returns: the lookup's failure, still read as a gh that is not
// installed. It changes PATH, so it does not run in parallel.
func TestGhCommand_NoGhOnPath_IsTheLookupsFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := ghCommand(t.Context(), "--version").Run()
	if !errors.Is(err, exec.ErrNotFound) || !strings.HasPrefix(err.Error(), "find gh: ") {
		t.Errorf("running ghCommand() with no gh on PATH = %v, want the lookup's failure", err)
	}
}
