// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/committext"
)

// injectedBlockMarkers are the markers a review bot writes around the block
// it injects into a pull request description. The HTML comments are the
// vendor-neutral half and the ones that matter: a bot re-finds its own block
// by them, which is exactly why the block comes back after an author deletes
// it. The heading names CodeRabbit, which is the bot this repository runs,
// because "Summary by <something>" is a heading a human might reasonably
// write.
var injectedBlockMarkers = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<!--\s*(?:this is an\s+)?auto-generated comment`),
	regexp.MustCompile(`(?i)<!--\s*end of auto-generated comment`),
	regexp.MustCompile(`(?i)<!--\s*(?:walkthrough|description|summary|changed_files)_(?:start|end)\s*-->`),
	regexp.MustCompile(`(?i)^#{1,6}\s*summary by coderabbit`),
}

// ghCommand builds a gh invocation. It is a seam so the tests can answer it
// with a stand-in; the program itself is the gh on PATH, which the runner
// image carries and a developer has installed to open the pull request.
//
// The program is resolved to an absolute path first, which is how every other
// command here runs a tool it did not build (cmd/gen_orbit_record): the
// lookup is the one exec would do, decided here rather than by whatever PATH
// holds when the command starts. A gh the lookup does not find is the
// command's Err, which Start returns wrapping the lookup's own error, so gh
// still reads it as a gh that is not installed.
var ghCommand = func(ctx context.Context, args ...string) *exec.Cmd {
	bin, err := exec.LookPath("gh")
	if err != nil {
		return &exec.Cmd{Err: fmt.Errorf("find gh: %w", err)}
	}
	// #nosec G204 G702 -- the program is the absolute path just resolved, with this command's own gh arguments, the pull request among them
	return exec.CommandContext(ctx, bin, args...)
}

// errNoPullRequest is why there is no description to judge: no pull request
// is open for the branch, and none was named.
var errNoPullRequest = errors.New("no pull request")

// errNoGh is why a description could not be read through the API: gh is not
// installed.
var errNoGh = errors.New("gh is not installed")

// errNoBranch is why the pull request open for the branch could not be
// looked up: HEAD is on no branch, as it is in the middle of a rebase or at a
// commit checked out by its hash. That says nothing about whether a pull
// request is open, so it is not read as there being none.
var errNoBranch = errors.New("HEAD is on no branch, so no pull request can be looked up for it: name one with PR_NUMBER=<n>")

// description is the text a squash merge makes the commit message, and
// where it was read from, which every run prints: a gate that silently
// judged the wrong text is worse than one that did not run at all.
type description struct {
	source string
	title  string
	body   string
}

// findings are what the description carries that it may not, each list in
// the order the text carries it.
type findings struct {
	dashes   []string
	injected []string
	skips    []string
	// upstream holds the locations of the upstream references, and
	// references the references themselves, each once.
	upstream   []string
	references []string
}

// addUpstream records that the text at location carries the upstream
// references refs, when it carries any.
func (f *findings) addUpstream(location string, refs []string) {
	if len(refs) == 0 {
		return
	}
	f.upstream = append(f.upstream, location)
	for _, ref := range refs {
		if !slices.Contains(f.references, ref) {
			f.references = append(f.references, ref)
		}
	}
}

// runDescription judges the pull request title and body of the branch
// checked out at dir and returns the exit code.
func runDescription(ctx context.Context, dir string, env func(string) string, stdout, stderr io.Writer) int {
	d, err := readDescription(ctx, dir, env)
	if errors.Is(err, errNoPullRequest) {
		// Nothing to judge is not a failure: the diff half still gates, and
		// a branch with no pull request yet has no description to land in
		// main.
		fmt.Fprintln(stderr, "No pull request is open for this branch, so there is no description to judge.")
		fmt.Fprintln(stderr, "Name one with PR_NUMBER=<n>, or rehearse with PR_BODY_FILE=<path>.")
		return 0
	}
	if errors.Is(err, errNoGh) {
		fmt.Fprintln(stderr, "ERROR: gh is needed to read the pull request description.")
		fmt.Fprintln(stderr, "Install it, or set PR_TITLE_FILE and PR_BODY_FILE to judge text from disk.")
		return 1
	}
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Judging the pull request title and body (%s).\n", d.source)
	f := judgeDescription(d)
	report(stdout, stderr, f.dashes, dashFailure)
	report(stdout, stderr, f.injected, injectedFailure)
	report(stdout, stderr, f.skips, skipFailureText)
	report(stdout, stderr, f.upstream, fmt.Sprintf(upstreamFailure, "    "+strings.Join(f.references, "\n    ")))
	if len(f.dashes)+len(f.injected)+len(f.skips)+len(f.upstream) > 0 {
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Re-check with: make check-pr-description")
		return 1
	}
	fmt.Fprintln(stdout, "OK: the title and body carry no em dash, no injected block, no skip command and no upstream reference.")
	return 0
}

// report prints the locations of one kind of finding, then what to do about
// it, when there are any.
func report(stdout, stderr io.Writer, locations []string, failure string) {
	if len(locations) == 0 {
		return
	}
	for _, location := range locations {
		fmt.Fprintln(stdout, location)
	}
	fmt.Fprint(stderr, failure)
}

// readDescription reads the title and body from the files env names, or
// else from the API through gh, of the pull request env names or the one
// open for the branch checked out at dir.
func readDescription(ctx context.Context, dir string, env func(string) string) (description, error) {
	titleFile, bodyFile := env("PR_TITLE_FILE"), env("PR_BODY_FILE")
	if titleFile != "" || bodyFile != "" {
		d := description{source: "files on disk"}
		var err error
		if d.title, err = readTextFile(titleFile); err != nil {
			return description{}, err
		}
		if d.body, err = readTextFile(bodyFile); err != nil {
			return description{}, err
		}
		return d, nil
	}

	repo := env("GITHUB_REPOSITORY")
	if repo == "" {
		out, err := gh(ctx, "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
		if err != nil {
			return description{}, err
		}
		repo = strings.TrimSpace(out)
	}
	number := env("PR_NUMBER")
	if number == "" {
		var err error
		if number, err = openPullRequest(ctx, dir, repo); err != nil {
			return description{}, err
		}
	}
	if number == "" {
		return description{}, errNoPullRequest
	}

	// One request, answered now rather than read out of the event payload:
	// see the command's documentation for why the difference matters.
	call := "repos/" + repo + "/pulls/" + number
	out, err := gh(ctx, "api", call)
	if err != nil {
		return description{}, err
	}
	var payload struct {
		Title *string `json:"title"`
		Body  *string `json:"body"`
	}
	if decodeErr := json.Unmarshal([]byte(out), &payload); decodeErr != nil {
		return description{}, fmt.Errorf("read the answer of gh api %s: %w", call, decodeErr)
	}
	d := description{source: repo + "#" + number + ", read from the API just now"}
	if payload.Title != nil {
		d.title = trimNewlines(*payload.Title)
	}
	if payload.Body != nil {
		d.body = trimNewlines(*payload.Body)
	}
	return d, nil
}

// readTextFile reads one of the rehearsal files, the empty string when none
// is named.
func readTextFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path) //#nosec G304,G703 -- the title or body file the person rehearsing the gate names
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return trimNewlines(string(data)), nil
}

// trimNewlines drops the newlines a text ends with, as the shell did with
// the output of a command substitution.
func trimNewlines(text string) string {
	return strings.TrimRight(text, "\n")
}

// openPullRequest returns the number of the pull request open in repo from
// the branch checked out at dir, or the empty string when there is none.
//
// gh is asked for a listing, and not for gh pr view, because a listing
// answers a branch with no pull request with an empty list, where gh pr view
// answers it with a failure nothing tells apart from a token GitHub refused
// or a network that is down: all three exit 1, and only gh's words on stderr
// differ, which no gh release promises to keep. Reading every failure of gh
// pr view as there being none passed a description the run never read. Here
// none is a list that holds none, and every failure is the run's, an answer
// that is not a list included. So is a list with an entry that does not say
// whether it is from a fork, which would be taken for the repository's own,
// or that names no pull request, which would be asked for as one, and the
// whole listing is read before any of it is taken.
//
// The branch is the one git has checked out, which is the head gh pr view
// looks a pull request up by too under git's default push.default; a branch
// pushed under another name is named with PR_NUMBER. The listing is of repo,
// the repository the description is then read from, and only a pull request
// from a branch of repo itself is taken. The listing matches the head by its
// name alone, so it also holds a fork's pull request from a branch of the
// same name, which gh pr view skipped for a branch pushed to repo, since it
// names a fork's branch owner:branch: taking it would judge a local main or
// patch-1 by a stranger's description. A pull request opened from a fork is
// named with PR_NUMBER too, which CI always sets. gh lists the newest first,
// so a branch with pull requests open into two bases is judged by the newest.
//
// gh lists no more than it is asked for, 30 unless told, and a listing cut
// at its limit says nothing about the pull requests past it. The lookup asks
// for listLimit, and a listing that holds as many with none from a branch of
// repo fails rather than reading as none, since the one to judge may be past
// them.
func openPullRequest(ctx context.Context, dir, repo string) (string, error) {
	cmd := gitCommand(ctx, dir, "branch", "--show-current")
	var gitErr bytes.Buffer
	cmd.Stderr = &gitErr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git branch --show-current: %w: %s", err, strings.TrimSpace(gitErr.String()))
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", errNoBranch
	}
	args := []string{"pr", "list", "--repo", repo, "--head", branch, "--state", "open", "--limit", strconv.Itoa(listLimit), "--json", "number,isCrossRepository"}
	call := strings.Join(args, " ")
	listing, err := gh(ctx, args...)
	if err != nil {
		return "", err
	}
	// Pointers, because null decodes into a slice as no error and no
	// element, which would read as a list that holds none, and an entry
	// with no isCrossRepository decodes as false, which would read as the
	// repository's own pull request.
	var open *[]struct {
		Number            int   `json:"number"`
		IsCrossRepository *bool `json:"isCrossRepository"`
	}
	if decodeErr := json.Unmarshal([]byte(listing), &open); decodeErr != nil {
		return "", fmt.Errorf("read the answer of gh %s: %w", call, decodeErr)
	}
	if open == nil {
		return "", fmt.Errorf("read the answer of gh %s: null is not a list", call)
	}
	number := ""
	for i, pr := range *open {
		if pr.IsCrossRepository == nil {
			return "", fmt.Errorf("read the answer of gh %s: entry %d does not say whether it is from a fork", call, i+1)
		}
		if pr.Number <= 0 {
			return "", fmt.Errorf("read the answer of gh %s: entry %d names no pull request, number %d", call, i+1, pr.Number)
		}
		if number == "" && !*pr.IsCrossRepository {
			number = strconv.Itoa(pr.Number)
		}
	}
	if number != "" {
		return number, nil
	}
	if len(*open) >= listLimit {
		return "", fmt.Errorf("gh %s listed %d pull requests, as many as it was asked for and every one from a fork, so one from a branch of %s itself may be past them: name it with PR_NUMBER=<n>", call, len(*open), repo)
	}
	return "", nil
}

// listLimit is how many pull requests the lookup asks gh to list, far more
// than a branch of the repository has open: a listing that reaches it is of
// a branch name many forks share, such as main or patch-1.
const listLimit = 100

// gh runs gh with args and returns what it printed. A gh that is not
// installed is errNoGh; one that fails is an error carrying the call and
// what gh said.
func gh(ctx context.Context, args ...string) (string, error) {
	cmd := ghCommand(ctx, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return "", errNoGh
	}
	if err != nil {
		return "", fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// judgeDescription finds what the description carries that it may not. The
// title is one line, reported whole; the body is reported by line, each line
// once per kind of finding, as "body:N:text".
func judgeDescription(d description) findings {
	var f findings
	titleLocation := "title: " + d.title
	if strings.Contains(d.title, committext.EmDash) {
		f.dashes = append(f.dashes, titleLocation)
	}
	if anyLine(d.title, committext.SkipCommand.MatchString) {
		f.skips = append(f.skips, titleLocation)
	}
	f.addUpstream(titleLocation, upstreamReferences(d.title))
	for i, line := range strings.Split(d.body, "\n") {
		// A description edited in the browser comes back from the API with
		// Windows line endings.
		line = strings.TrimSuffix(line, "\r")
		location := fmt.Sprintf("body:%d:%s", i+1, line)
		if strings.Contains(line, committext.EmDash) {
			f.dashes = append(f.dashes, location)
		}
		if anyMatch(injectedBlockMarkers, line) {
			f.injected = append(f.injected, location)
		}
		if committext.SkipCommand.MatchString(line) || committext.SkipChecksTrailer.MatchString(line) {
			f.skips = append(f.skips, location)
		}
		f.addUpstream(location, upstreamReferences(line))
	}
	return f
}

// anyLine reports whether match holds for any line of text.
func anyLine(text string, match func(string) bool) bool {
	return slices.ContainsFunc(strings.Split(text, "\n"), match)
}

// anyMatch reports whether any of patterns matches line.
func anyMatch(patterns []*regexp.Regexp, line string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}

// dashFailure is what an em dash in the description costs, and how to fix it.
const dashFailure = `
FAIL: the pull request title or body carries an em dash (U+2014).

Edit the pull request description itself, not a commit. This repository squash
merges, so the description becomes the commit message on main and no later
commit can take it back out:

    gh pr edit <number>
`

// injectedFailure is what a review bot's block costs, and why fixing the
// configuration rather than the description is the fix.
const injectedFailure = `
FAIL: the pull request description carries a block a review bot generated.

Delete the block named above and everything down to its closing marker, then
edit the description:

    gh pr edit <number>

This should not be reachable. The .coderabbit.yaml file sets
high_level_summary_in_walkthrough, which puts the summary in the walkthrough
comment and leaves the description alone, so a block appearing here means that
setting stopped working, the bot changed where it writes, or another bot is
writing now. Fix the configuration rather than the description: stripping a
block by hand loses a race that runs on every review, since the bot writes it
again on the next one.

It matters because a squash merge copies the description into main's history,
where editing the pull request afterwards does not reach the commit. That is
how pull request 832 carried one in permanently.
`

// upstreamFailure is what an upstream reference in the description costs,
// its %s standing for the references found, one per line (issue 1477), and
// how a Dependabot pull request, whose description copies what names them, is
// cut before its merge.
const upstreamFailure = `
FAIL: the pull request title or body names an item of another project or group the way GitLab resolves it:

%s

This repository squash merges, so the description becomes the commit message
on main, and the GitLab mirror of this repository reads that message like any
commit pushed to it: GitLab adds a "mentioned in commit" note to every item a
message names that way, on somebody else's project, and main cannot be
rewritten to take it back. Name the row of docs/development/upstream-bugs.md
that records the item instead.

An item of another forge written the same way, such as a GitHub issue as
owner/repo#N or repo#N, is refused too, since GitLab reads the path as its own:
write it as its github.com URL, which passes, or name it in words.
Then edit the description:

    gh pr edit <number>

A Dependabot pull request fails here when its description copies a
dependency's release notes, changelog or commit list, which name the
dependency's own items. Keep what Dependabot writes outside its <details>
blocks, delete the blocks, write the rest back and re-run the failed jobs,
right before the merge: editing a description starts no run, and Dependabot
can write its description again when it updates the pull request.

    gh pr edit <number> --body-file <file>
    gh run rerun <run id> --failed
`

// skipFailureText is what a skip command in the description costs.
const skipFailureText = `
FAIL: the pull request title or body carries a command that makes GitHub skip
workflows.

GitHub skips every workflow a push triggers when the commit it lands on carries
one of the bracketed commands in its message, wherever it appears and in
backticks too, or ends with a skip-checks trailer. This repository squash
merges, so the description becomes that message: the merge would run no CI,
CodeQL, mirror or Pages on main, and a release tag put on the commit would run
no release. Name the command without its square brackets, or the trailer
inside a sentence rather than on a line of its own, then edit the description:

    gh pr edit <number>
`
