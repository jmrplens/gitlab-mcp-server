package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// recordVerdict is what the offline check found.
//
// Findings and notes are kept apart because only one of them is a gate. A
// finding says the committed record contradicts the tree it sits in, which
// nothing but an edit can fix. A note says the record and the tree have
// drifted in a way another gate already covers, or in a way the check cannot
// judge from a checkout alone, and a reader who is told it is better informed
// while CI stays green.
type recordVerdict struct {
	Findings []string
	Notes    []string
}

// failf records one finding.
func (v *recordVerdict) failf(format string, args ...any) {
	v.Findings = append(v.Findings, fmt.Sprintf(format, args...))
}

// notef records one note.
func (v *recordVerdict) notef(format string, args ...any) {
	v.Notes = append(v.Notes, fmt.Sprintf(format, args...))
}

// checkRecord judges the committed document against the tree it sits in,
// with no GitLab and no network.
//
// The one oracle it needs is the catalog each runtime serves, and that is
// built offline through [buildServedCatalog], which drives the server's own
// assembly against an in-process stub client. Everything else is arithmetic
// over the document.
func checkRecord(opts options, doc *coverageRecord, pagePath string, now time.Time) (*recordVerdict, error) {
	verdict := &recordVerdict{}
	if doc.SchemaVersion != recordSchemaVersion {
		// Refused rather than read half-way: a document written by another
		// version of this command may spell a field differently, and every
		// figure below would then be computed over a partial read.
		verdict.failf("the record is at schema %d and this command writes %d: regenerate it with %s",
			doc.SchemaVersion, recordSchemaVersion, recordRegenerate)
		return verdict, nil
	}
	for _, key := range recordRuntimes {
		if _, present := doc.Runtimes[key]; !present {
			verdict.failf("the record holds no %s runtime: run make e2e-coverage-record-%s after a Docker run of that half", key, key)
		}
	}
	for _, key := range sortedKeys(doc.Runtimes) {
		if !isRecordKey(key) {
			verdict.failf("the record holds an entry keyed %q: %v", key, errRecordRuntime)
			continue
		}
		if err := checkRecordEntry(opts, verdict, key, doc.Runtimes[key], now); err != nil {
			return nil, err
		}
	}
	if err := writeRecordPage(pagePath, doc, true); err != nil {
		verdict.failf("%v", err)
	}
	return verdict, nil
}

// isRecordKey reports whether a key is one of the two the record is written
// under, which is what makes an entry reproducible from a Makefile target.
func isRecordKey(key string) bool {
	return slices.Contains(recordRuntimes, key)
}

// checkRecordEntry judges one runtime's entry.
func checkRecordEntry(opts options, verdict *recordVerdict, key string, entry *recordEntry, now time.Time) error {
	if entry == nil {
		verdict.failf("the %s entry is empty", key)
		return nil
	}
	if !matchesRuntime(entry.Runtime, key) {
		// The key is what a reader looks the entry up by and the runtime is
		// what it was measured on; an entry filed under the other key would
		// have its ce figures read as the licensed run's.
		verdict.failf("the %s entry was measured on %s, which is not a %s runtime", key, entry.Runtime, key)
	}
	for _, problem := range recordDateProblems(key, entry.RetrievedAt, now) {
		verdict.failf("%s", problem)
	}
	checkRecordArithmetic(verdict, key, entry)
	// The same floors -check applies to a live run, re-applied to the
	// committed evidence: a record whose own rows say a package refused, or
	// ran under a -run filter, is a coverage claim about a run that did not
	// happen, and committing it does not make it one.
	if floors := checkRuntime(&report{
		Runtime: entry.Runtime, Summary: entry.Summary, Runs: entry.Runs,
	}, []string{key}); !floors.Passed {
		for _, finding := range floors.Findings {
			verdict.failf("%s: %s", key, finding)
		}
	}
	if note := revisionNote(opts.dir, key, entry); note != "" {
		verdict.notef("%s", note)
	}
	return checkRecordCatalog(opts, verdict, key, entry)
}

// checkRecordArithmetic holds the headline counts to the lists beside them.
//
// It is the one check that needs nothing outside the document, and it is the
// one that catches a hand edit: a figure raised in the summary without the
// action list that would justify it.
func checkRecordArithmetic(verdict *recordVerdict, key string, entry *recordEntry) {
	for _, level := range []struct {
		name  string
		count int
		ids   []string
	}{
		{name: "l1", count: entry.Summary.L1, ids: entry.Levels.L1},
		{name: "l2", count: entry.Summary.L2, ids: entry.Levels.L2},
		{name: "l3", count: entry.Summary.L3, ids: entry.Levels.L3},
	} {
		if level.count != len(level.ids) {
			verdict.failf("%s says %s is %d and lists %d actions at that level: the summary was edited without the list",
				key, level.name, level.count, len(level.ids))
		}
	}
	if entry.Summary.L1 > entry.Summary.CatalogActions {
		verdict.failf("%s says %d of %d catalog actions are asserted, which is more actions than the catalog has",
			key, entry.Summary.L1, entry.Summary.CatalogActions)
	}
}

// checkRecordCatalog compares the record with the catalog this tree builds,
// and reports every difference as a note.
//
// Nothing here fails, and the reason is that another gate already does. An
// action the record names and the catalog no longer has is a rename, and
// `make check-e2e-static` fails on it from the other side, where the typed
// ActionID constant the scenario carries no longer resolves; failing here too
// would mean a rename could not be committed without a booted GitLab to
// refresh this file first. An action the catalog has and the record does not
// is a new action, which the same static gate already refuses without a
// scenario or an exemption. What the notes add is the figure a reader needs
// to know how much of the record is still describing this tree.
func checkRecordCatalog(opts options, verdict *recordVerdict, key string, entry *recordEntry) error {
	tier, known := edition.ParseTier(entry.Tier)
	if !known {
		verdict.failf("%s names the tier %q, which is not one this server knows", key, entry.Tier)
		return nil
	}
	catalog, err := opts.catalogs(tier)
	if err != nil {
		return err
	}
	if entry.Summary.CatalogActions != len(catalog.ids) {
		verdict.notef("%s was measured against %d catalog actions and this tree builds %d at %s: the shares below are of the older catalog",
			key, entry.Summary.CatalogActions, len(catalog.ids), tier)
	}
	var gone []string
	for _, id := range entry.Levels.L1 {
		if _, held := catalog.actions[id]; !held {
			gone = append(gone, id)
		}
	}
	if len(gone) > 0 {
		verdict.notef("%s credits %d action(s) this tree's catalog no longer has (%s): check-e2e-static fails on the same rename from the scenario's side",
			key, len(gone), strings.Join(gone, ", "))
	}
	return nil
}

// recordDateProblems reports the ways the entry's date stops it being one a
// gate can rest on, on the terms [provenance.Problems] states them and under
// this record's own window.
//
// It calls [provenance.Age] and [provenance.Days] directly rather than
// joining the shared verdict, which is the arrangement cmd/audit_graphql_documents
// already has: the arithmetic is worth sharing and the window is not, because
// [recordMaxAge] is about this repository's release cadence and the shared one
// is about GitLab's.
func recordDateProblems(key, retrievedAt string, now time.Time) []string {
	switch age, err := provenance.Age(retrievedAt, now); {
	case err != nil:
		return []string{fmt.Sprintf(
			"%s says it was measured on %q, which is not a date: nothing can then say how old the figures are",
			key, retrievedAt,
		)}
	case age < 0:
		return []string{fmt.Sprintf(
			"%s says it was measured on %s, which has not happened yet: no run writes a day in the future",
			key, retrievedAt,
		)}
	case age > recordMaxAge:
		return []string{fmt.Sprintf(
			"%s is %d days old and the window is %d: it is a coverage claim about a catalog this tree no longer has; refresh it with %s",
			key, provenance.Days(age), provenance.Days(recordMaxAge), recordRegenerate,
		)}
	default:
		return nil
	}
}

// fullSHA is what a recorded commit must look like before it is handed to
// git as an argument. A value read out of a committed file is not this
// command's own, and the probe below runs a process with it.
var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// gitProbe answers a yes-or-no question of the repository. It is a variable
// so the three branches below -- git absent, the revision unknown, the
// revision known and not an ancestor -- are each reachable from a test,
// which none of them is from a checkout in one state.
var gitProbe = askGit

// gitProbeTimeout bounds one revision question. Both are answered out of the
// object database and take milliseconds; the bound is there so a repository
// on a filesystem that has stopped answering costs the gate a few seconds
// rather than the job's whole timeout, and a probe that times out reads as
// "git could not be run", which is silence.
const gitProbeTimeout = 10 * time.Second

// askGit runs one git command and reports whether it exited zero.
//
// A non-zero exit is the answer "no" for both questions asked here, so it is
// reported as a false rather than as an error; anything that is not an exit
// status means git could not be run at all, which is a different silence.
func askGit(dir string, args ...string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitProbeTimeout)
	defer cancel()
	// #nosec G204 -- the arguments are this file's own literals plus a commit
	// the caller has already held to fullSHA, which admits nothing a shell or
	// git could read as an option or a path.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if _, isExit := errors.AsType[*exec.ExitError](err); isExit {
		return false, nil
	}
	return false, err
}

// revisionNote says when an entry was measured on a tree that is not in this
// history, and says nothing at all when it cannot tell.
//
// The silence is the point. CI checks this repository out at depth one, so
// every revision but HEAD is unknown there, and a note that fired whenever a
// revision could not be resolved would fire on every run of every pull
// request and inform nobody. A revision git does know and cannot reach from
// HEAD is a different matter: the ee half of the committed record is in
// exactly that state, measured on a branch that was rebased away, and a
// reader comparing its figures with this tree's catalog should be told.
func revisionNote(dir, key string, entry *recordEntry) string {
	commit := ""
	for _, run := range entry.Runs {
		if run.Commit != "" {
			commit = run.Commit
			break
		}
	}
	if !fullSHA.MatchString(commit) {
		return ""
	}
	known, err := gitProbe(dir, "cat-file", "-e", commit+"^{commit}")
	if err != nil || !known {
		return ""
	}
	ancestor, err := gitProbe(dir, "merge-base", "--is-ancestor", commit, "HEAD")
	if err != nil || ancestor {
		return ""
	}
	return fmt.Sprintf("%s was measured on %s, which is not an ancestor of HEAD: the tree it ran against is not this one", key, commit)
}

// runCheckRecord is the offline gate: read the committed record, judge it,
// print what it found.
func runCheckRecord(opts options, stdout, stderr io.Writer) int {
	recordPath, pagePath := recordPaths(opts)
	doc, err := readRecord(recordPath)
	if err != nil {
		fmt.Fprintf(stderr, "audit_e2e_coverage: record: %v: regenerate it with %s\n", err, recordRegenerate)
		return exitFindings
	}
	verdict, err := checkRecord(opts, doc, pagePath, provenance.Clock(opts.now))
	if err != nil {
		fmt.Fprintln(stderr, "audit_e2e_coverage: record:", err)
		return exitUsage
	}
	for _, note := range verdict.Notes {
		fmt.Fprintln(stdout, "record: note:", note)
	}
	for _, finding := range verdict.Findings {
		fmt.Fprintln(stderr, "record:", finding)
	}
	for _, key := range sortedKeys(doc.Runtimes) {
		if entry := doc.Runtimes[key]; entry != nil {
			fmt.Fprintf(stdout, "record: %s (%s, measured %s): L1 %d/%d (%.1f%%), L2 %d, L3 %d\n",
				key, entry.Runtime, entry.RetrievedAt, entry.Summary.L1, entry.Summary.CatalogActions,
				percent(entry.Summary.L1, entry.Summary.CatalogActions), entry.Summary.L2, entry.Summary.L3)
		}
	}
	if len(verdict.Findings) > 0 {
		return exitFindings
	}
	return exitOK
}
