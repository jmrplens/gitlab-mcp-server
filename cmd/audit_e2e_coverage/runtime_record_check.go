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
//
// The page beside the record is deliberately not its business: see
// [checkRecordPage] for why the two halves are asked for separately.
func checkRecord(opts options, doc *coverageRecord, now time.Time) (*recordVerdict, error) {
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
	return verdict, nil
}

// checkRecordPage reports whether the committed page is byte for byte what the
// committed record renders to, as the finding that says it is not.
//
// It is asked for separately from [checkRecord] because only this half moves
// with the source tree. The figures need a booted GitLab and nothing in this
// repository can regenerate them, so every judgment in [checkRecord] holds on
// any layer of any stack; the drawing around them is [renderRecordPage] and
// [docgen.RenderMarkdownTable], both of which a commit can change, and
// `make update-all` redraws the page for exactly that reason. That makes this
// half a freshness gate on the terms every other generated artifact here is
// held to -- deferred below a stack's tip and refreshed once at it -- which is
// the same arrangement bench-resources-render and check-bench-resources have.
func checkRecordPage(pagePath string, doc *coverageRecord) []string {
	if err := writeRecordPage(pagePath, doc, true); err != nil {
		return []string{err.Error()}
	}
	return nil
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
	dated, approaching := recordDateProblems(key, entry.RetrievedAt, now)
	for _, problem := range dated {
		verdict.failf("%s", problem)
	}
	for _, notice := range approaching {
		verdict.notef("%s", notice)
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

// recordLevel is one level's headline count and the list beside it.
type recordLevel struct {
	name  string
	count int
	ids   []string
}

// recordLevels returns the three levels in the order the record publishes
// them, l1 first.
//
// It is one table because every question asked of the levels is asked of all
// three, and answering it for l1 alone is how l2 and l3 came to be compared
// with nothing at all: a hand edit could put an id in either of them that no
// catalog has and no check would look.
func recordLevels(entry *recordEntry) []recordLevel {
	return []recordLevel{
		{name: "l1", count: entry.Summary.L1, ids: entry.Levels.L1},
		{name: "l2", count: entry.Summary.L2, ids: entry.Levels.L2},
		{name: "l3", count: entry.Summary.L3, ids: entry.Levels.L3},
	}
}

// checkRecordArithmetic holds the headline counts to the lists beside them,
// and the lists to each other.
//
// It is the one check that needs nothing outside the document, and it is the
// one that catches a hand edit: a figure raised in the summary without the
// action list that would justify it, or an id added to l2 or l3 that l1 does
// not carry. The subset is a property the writer guarantees -- an action
// asserted on the dynamic surface, or on all three, is asserted on at least
// one -- so a record where it does not hold was not written by this command.
func checkRecordArithmetic(verdict *recordVerdict, key string, entry *recordEntry) {
	for _, level := range recordLevels(entry) {
		if level.count != len(level.ids) {
			verdict.failf("%s says %s is %d and lists %d actions at that level: the summary was edited without the list",
				key, level.name, level.count, len(level.ids))
		}
	}
	asserted := make(map[string]bool, len(entry.Levels.L1))
	for _, id := range entry.Levels.L1 {
		asserted[id] = true
	}
	// l1 is the union the other two are drawn from, so it is the one list
	// there is nothing here to hold it against.
	for _, level := range recordLevels(entry)[1:] {
		var loose []string
		for _, id := range level.ids {
			if !asserted[id] {
				loose = append(loose, id)
			}
		}
		if len(loose) > 0 {
			verdict.failf("%s lists %d action(s) at %s that l1 does not carry (%s): l2 and l3 are drawn from l1, so the lists were edited by hand",
				key, len(loose), level.name, strings.Join(loose, ", "))
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
	// All three lists, not l1 alone: l2 and l3 are subsets of it in anything
	// this command wrote, so walking them costs a second pass over a few
	// hundred strings and catches the one case the subset argument does not
	// cover, which is a hand edit that put an id in l2 or l3 that no catalog
	// ever had.
	for _, level := range recordLevels(entry) {
		var gone []string
		for _, id := range level.ids {
			if _, held := catalog.actions[id]; !held {
				gone = append(gone, id)
			}
		}
		if len(gone) > 0 {
			verdict.notef("%s credits %d action(s) at %s that this tree's catalog no longer has (%s): check-e2e-static fails on the same rename from the scenario's side",
				key, len(gone), level.name, strings.Join(gone, ", "))
		}
	}
	return nil
}

// recordDateProblems reports the ways the entry's date stops it being one a
// gate can rest on, on the terms [provenance.Problems] states them and under
// this record's own window, split into what fails and what only informs.
//
// It calls [provenance.Age] and [provenance.Days] directly rather than
// joining the shared verdict, which is the arrangement cmd/audit_graphql_documents
// already has: the arithmetic is worth sharing and the window is not, because
// [recordMaxAge] is about this repository's release cadence and the shared one
// is about GitLab's. The approaching-expiry note is this record's own too, and
// for a reason the shared records do not have: refreshing one of those is a
// container and a generator, and refreshing this one is two Docker suites, one
// of which needs a license CI does not hold.
func recordDateProblems(key, retrievedAt string, now time.Time) (findings, notes []string) {
	switch age, err := provenance.Age(retrievedAt, now); {
	case err != nil:
		return []string{fmt.Sprintf(
			"%s says it was measured on %q, which is not a date: nothing can then say how old the figures are",
			key, retrievedAt,
		)}, nil
	case age < 0:
		return []string{fmt.Sprintf(
			"%s says it was measured on %s, which has not happened yet: no run writes a day in the future",
			key, retrievedAt,
		)}, nil
	case age > recordMaxAge:
		return []string{fmt.Sprintf(
			"%s is %d days old and the window is %d: it is a coverage claim about a catalog this tree no longer has; refresh it with %s",
			key, provenance.Days(age), provenance.Days(recordMaxAge), recordRegenerate,
		)}, nil
	case age > recordNoticeAge:
		return nil, []string{fmt.Sprintf(
			"%s is %d days old and the window is %d: %d days from now every push here fails on it, and clearing that needs a Docker run of both halves; refresh it with %s",
			key, provenance.Days(age), provenance.Days(recordMaxAge), provenance.Days(recordMaxAge-age), recordRegenerate,
		)}
	default:
		return nil, nil
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

// runCheckRecord is the offline gate: read the committed record, judge
// whichever halves were asked for, print what they found.
//
// One read of the document serves both halves, which is why they are one
// function and two flags rather than two commands: a caller that wants
// everything (`make analyze`, a developer before pushing) passes both and pays
// for one read, and CI passes them to two steps with different conditions.
func runCheckRecord(opts options, stdout, stderr io.Writer) int {
	recordPath, pagePath := recordPaths(opts)
	doc, err := readRecord(recordPath)
	if err != nil {
		fmt.Fprintf(stderr, "audit_e2e_coverage: record: %v: regenerate it with %s\n", err, recordRegenerate)
		return exitFindings
	}
	verdict := &recordVerdict{}
	if opts.checkRecord {
		if verdict, err = checkRecord(opts, doc, provenance.Clock(opts.now)); err != nil {
			fmt.Fprintln(stderr, "audit_e2e_coverage: record:", err)
			return exitUsage
		}
	}
	var pageFindings []string
	if opts.checkRecordPage {
		pageFindings = checkRecordPage(pagePath, doc)
		verdict.Findings = append(verdict.Findings, pageFindings...)
	}
	for _, note := range verdict.Notes {
		fmt.Fprintln(stdout, "record: note:", note)
	}
	for _, finding := range verdict.Findings {
		fmt.Fprintln(stderr, "record:", finding)
	}
	if opts.checkRecord {
		printRecordHeadlines(doc, stdout)
	}
	// Each half says what it looked at when it held, so a green step names the
	// question it answered rather than printing nothing at all.
	if opts.checkRecordPage && len(pageFindings) == 0 {
		fmt.Fprintf(stdout, "record: %s is what %s renders to\n", pagePath, recordPath)
	}
	if len(verdict.Findings) > 0 {
		return exitFindings
	}
	return exitOK
}

// printRecordHeadlines prints one line per runtime: what the committed record
// says that half of the suite covers.
func printRecordHeadlines(doc *coverageRecord, stdout io.Writer) {
	for _, key := range sortedKeys(doc.Runtimes) {
		if entry := doc.Runtimes[key]; entry != nil {
			fmt.Fprintf(stdout, "record: %s (%s, measured %s): L1 %d/%d (%.1f%%), L2 %d, L3 %d\n",
				key, entry.Runtime, entry.RetrievedAt, entry.Summary.L1, entry.Summary.CatalogActions,
				percent(entry.Summary.L1, entry.Summary.CatalogActions), entry.Summary.L2, entry.Summary.L3)
		}
	}
}
