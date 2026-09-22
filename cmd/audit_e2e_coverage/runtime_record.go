package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// The committed artifacts, relative to the repository root.
//
// The JSON is the record and the Markdown is a rendering of it, which is why
// the page is regenerated from the committed document rather than from a run:
// a reader on any checkout can redraw it, and a gate can compare the bytes.
const (
	recordRelPath     = "docs/development/e2e-coverage.json"
	recordPageRelPath = "docs/development/testing/e2e-coverage.md"
)

// recordErrPrefix opens every line this file writes to stderr.
//
// One spelling rather than three, because the three places that print it are
// the three ways a record operation can fail and a reader grepping their logs
// for one of them should find all of them.
const recordErrPrefix = "audit_e2e_coverage: record:"

// recordSchemaVersion is the version of the committed document's shape.
//
// It is refused rather than read half-way when it is not this one, on the
// terms [e2ecalls.Record] holds a shard to: a document written by another
// version of this command may spell a field differently, and a check that
// read what it recognized and ignored the rest would report a coverage figure
// it had only half understood.
//
// Adding an optional field is not a new version, and the capability grain is
// the case that settled it. Its rows ([recordEntry.CapabilitySurfaces]) are
// omitted when empty, so a document either version wrote is read by both, and
// the grain an entry was measured at is said by the entry rather than by the
// document: [writeRecord] refreshes one runtime at a time and stamps this
// version on every write, so a document-level version would put the new number
// over a licensed entry still measured at the old grain.
const recordSchemaVersion = 1

// recordRuntimes are the two runtimes the record is written under, in the
// order the page lists them.
//
// They are the two Docker targets and nothing else: `make test-e2e-gitlab`
// writes a third directory, `self-hosted`, whose instance is the developer's
// own and whose catalog nobody else can reproduce, so a record holding it
// would be a claim no checkout could check.
var recordRuntimes = []string{"ce", "ee"}

// recordMaxAge is how long the committed coverage record may stand before
// the check that reads it refuses it.
//
// It is deliberately not [provenance.MaxAge], and the difference is the
// reason: that window is one window for the records that pin an external
// truth, chosen for GitLab's release cadence, and widening or narrowing it is
// a decision about GitLab. This record pins nothing external. It is a
// measurement of this repository's own suite against this repository's own
// catalog, and the catalog moves whenever an action is added here, which is
// far more often than GitLab ships. A quarter is about as long as these
// figures can describe a tree that is still being worked on; past it the
// record is a coverage claim about a catalog that no longer exists, and the
// only honest way to say so is to fail.
const recordMaxAge = 90 * 24 * time.Hour

// recordNoticeAge is when the window starts being announced: from here to
// [recordMaxAge] the age is a note that prints and exits zero, and only the
// expiry itself is a finding.
//
// A deadline should be visible before it stops the repository. Clearing this
// one is `make test-e2e-ce` plus `make test-e2e-ee` plus a record write, and
// the licensed half needs an activation code that is deliberately not in CI,
// so on the day the window closes every open pull request goes red at once
// over something no contributor can fix. A fortnight is about the notice a
// maintainer needs to schedule two hour-long Docker runs, and a note on every
// push in that fortnight is how they hear about it.
const recordNoticeAge = recordMaxAge - 14*24*time.Hour

// coverageRecord is the committed per-runtime summary.
//
// It carries an allowlist of the report rather than the whole of it. Two
// halves of the report are deliberately absent: the per-action cells, which
// are one row per catalog action per surface and would make every refresh a
// thousand-line diff, and [report.Directory], which is a path on the machine
// that ran the suite and means nothing anywhere else. What the levels do
// carry is the answer to "which actions": [levels] is three sorted id lists.
type coverageRecord struct {
	SchemaVersion int `json:"schema_version"`
	// Note says what the document is to a reader who opened it first.
	Note string `json:"note"`
	// Runtimes is one entry per runtime, keyed by the -runtime selector that
	// produces it: ce or ee.
	Runtimes map[string]*recordEntry `json:"runtimes"`
}

// recordEntry is one runtime's committed summary.
type recordEntry struct {
	// RetrievedAt is the day the run that produced this entry started, read
	// off the run IDs rather than taken from the writing clock: re-running
	// the command over shards from months ago must not reset the window,
	// since the window is about how old the measurement is and not about how
	// recently somebody rebuilt the file from it.
	RetrievedAt string `json:"retrieved_at"`
	// Runtime, Edition and Tier name what the suite ran against.
	Runtime string `json:"runtime"`
	Edition string `json:"edition"`
	Tier    string `json:"tier"`
	// Runs is one row per package, carrying the commit, the GitLab version
	// and the fixture profile that package had.
	Runs []runRow `json:"runs"`
	// Sessions is what each surface and mode served.
	Sessions []sessionRow `json:"sessions"`
	// CapabilitySurfaces is what each capability surface served: the
	// denominator of the resources, prompts, completions and subscriptions
	// histograms, which are counted per capability surface; tool_manifest,
	// elicitation and modes are counted per shape and have no figure here.
	// An entry without them was recorded before the capability grain, when
	// every capability kind was counted once per surface x mode; the check
	// says so and the page states the older grain, and nothing else reads
	// the difference.
	CapabilitySurfaces []capabilitySurfaceRow `json:"capability_surfaces,omitempty"`
	// Summary is the headline counts.
	Summary summary `json:"summary"`
	// Levels lists the actions at each level, which is what makes the record
	// an answer to "which actions" rather than only to "how many".
	Levels levels `json:"levels"`
}

// recordNote is the sentence the document opens with.
const recordNote = "What the end-to-end suite asserts, per runtime, from the calls it recorded rather than from " +
	"mentions in its source. Written by `make e2e-coverage-record` after a Docker run of both halves; the page at " +
	"docs/development/testing/e2e-coverage.md is rendered from this file and `make check-e2e-coverage-record` " +
	"compares both without a GitLab and without a network. One entry per Docker target: ce is the unlicensed " +
	"instance, ee the licensed one."

// errRecordRuntime is a runtime the record has no key for.
var errRecordRuntime = errors.New("no Makefile target produces it, so no checkout could reproduce the entry")

// recordKeyFor names the record key a runtime is written under, and reports
// false for one that belongs in no committed record.
func recordKeyFor(runtime string) (string, bool) {
	for _, key := range recordRuntimes {
		if matchesRuntime(runtime, key) {
			return key, true
		}
	}
	return "", false
}

// buildRecordEntry turns one runtime's report into the entry the record
// commits, or refuses the report as evidence not worth committing.
//
// The refusals are the same floors -check applies, plus two the committed
// artifact needs and a live report does not: run lines that disagree on the
// commit came from two trees and there is no one revision to record, and a
// run ID whose stamp will not parse leaves the entry with no date, which is
// the one field the staleness window rests on.
func buildRecordEntry(rep *report) (*recordEntry, error) {
	key, known := recordKeyFor(rep.Runtime)
	if !known {
		return nil, fmt.Errorf("%s is not a runtime the record holds: %w", rep.Runtime, errRecordRuntime)
	}
	if verdict := checkRuntime(rep, []string{key}); !verdict.Passed {
		return nil, fmt.Errorf("%s is not a run worth committing:\n  %s", rep.Runtime, strings.Join(verdict.Findings, "\n  "))
	}
	if err := oneCommit(rep); err != nil {
		return nil, fmt.Errorf("%s: %w", rep.Runtime, err)
	}
	retrievedAt, err := recordDate(rep)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rep.Runtime, err)
	}
	return &recordEntry{
		RetrievedAt:        retrievedAt,
		Runtime:            rep.Runtime,
		Edition:            rep.Edition,
		Tier:               rep.Tier,
		Runs:               rep.Runs,
		Sessions:           rep.Sessions,
		CapabilitySurfaces: rep.CapabilitySurfaces,
		Summary:            rep.Summary,
		Levels:             rep.Levels,
	}, nil
}

// oneCommit refuses a runtime whose packages name different revisions.
//
// A directory is one Docker run, so its packages were built from one tree;
// two commits under it means the shards of two runs were folded together, and
// the figures then describe neither tree. An empty commit is left alone: it is
// a run of a tree git could not see, which the run line already says.
func oneCommit(rep *report) error {
	seen := map[string][]string{}
	for _, run := range rep.Runs {
		if run.Commit != "" {
			seen[run.Commit] = append(seen[run.Commit], run.Package)
		}
	}
	if len(seen) < 2 {
		return nil
	}
	var named []string
	for commit, packages := range seen {
		named = append(named, fmt.Sprintf("%s (%s)", commit, strings.Join(packages, ", ")))
	}
	sort.Strings(named)
	return fmt.Errorf("the run lines name %d revisions (%s): the shards of two runs were folded together",
		len(seen), strings.Join(named, "; "))
}

// errRecordDate is a run ID whose stamp says nothing about when the run
// happened.
var errRecordDate = errors.New("no run ID carries a timestamp this can read, so the entry would have no date and nothing could say how old it is")

// recordDate reads the day of the run off its run IDs.
//
// The stamp is read through [e2ecalls.RunIDDate], which sits beside the layout
// the harness mints the identifier with: this command is outside the e2e build
// tag and the harness is inside it, so the shared untagged package both already
// import is the only place a single spelling can live.
//
// The earliest stamp wins, not the latest: a directory's packages start
// minutes apart and the window asks how long the measurement has been
// standing, so the honest answer is the age of its oldest part. A refused
// package writes a run line with no run ID at all and is skipped rather than
// refused, since it recorded no calls to be stale about.
func recordDate(rep *report) (string, error) {
	var earliest time.Time
	for _, run := range rep.Runs {
		at, read := e2ecalls.RunIDDate(run.RunID)
		if !read {
			continue
		}
		if earliest.IsZero() || at.Before(earliest) {
			earliest = at
		}
	}
	if earliest.IsZero() {
		return "", errRecordDate
	}
	return earliest.UTC().Format(time.DateOnly), nil
}

// readRecord reads the committed document.
func readRecord(path string) (*coverageRecord, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read the coverage record: %w", err)
	}
	var doc coverageRecord
	if unmarshalErr := json.Unmarshal(data, &doc); unmarshalErr != nil {
		return nil, fmt.Errorf("read %s: %w", path, unmarshalErr)
	}
	if doc.Runtimes == nil {
		doc.Runtimes = map[string]*recordEntry{}
	}
	return &doc, nil
}

// writeRecord folds this run's runtimes into the committed document and
// writes both artifacts.
//
// The fold is what lets a maintainer refresh one half: `make test-e2e-ce`
// boots a community instance and knows nothing about the licensed one, so a
// write that replaced the whole document would silently drop the ee entry and
// the page would then report a suite that covers half of what it does. An
// unreadable document is refused rather than replaced, for the same reason.
//
// Two reports of one invocation settling to one key is refused rather than
// folded, and that is the fold's own blind spot: a key is an edition and a
// tier, so `make test-e2e-gitlab`'s self-hosted directory -- a developer's own
// instance, unlicensed, which the record must never hold -- is community/free
// exactly as the Docker ce instance is. Pointed at the parent directory both
// sit under, the write would put the stranger's figures under ce, in sorted
// order so self-hosted wins, and exit zero.
func writeRecord(recordPath, pagePath string, reps []*report) error {
	doc, err := readRecord(recordPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if doc == nil {
		doc = &coverageRecord{Runtimes: map[string]*recordEntry{}}
	}
	doc.SchemaVersion = recordSchemaVersion
	doc.Note = recordNote
	settled := map[string]*report{}
	for _, rep := range reps {
		entry, buildErr := buildRecordEntry(rep)
		if buildErr != nil {
			return buildErr
		}
		key, _ := recordKeyFor(rep.Runtime)
		if first, taken := settled[key]; taken {
			return fmt.Errorf("two runs of this invocation settle to the %s entry, %s and %s: "+
				"the key is the edition and the tier, so a Docker run and a developer's own instance of the same "+
				"edition are indistinguishable here and the second would replace the first with nothing said; "+
				"record one directory at a time", key, recordSource(first), recordSource(rep))
		}
		settled[key] = rep
		doc.Runtimes[key] = entry
	}
	if writeErr := docgen.WriteOrCheck(recordPath, marshalRecord(doc), false, recordRegenerate); writeErr != nil {
		return writeErr
	}
	return writeRecordPage(pagePath, doc, false)
}

// recordSource names a report the way a refusal about two of them has to: the
// runtime it measured and the directory its shards were read from, which is
// the only thing that tells two runs of one edition apart.
//
// It is the one place [report.Directory] is allowed out, and the ban it is an
// exception to is intact: the string goes to the operator who is being told to
// choose, never into the document. A hand-built report carries no directory
// and is named by its runtime alone.
func recordSource(rep *report) string {
	if rep.Directory == "" {
		return rep.Runtime
	}
	return rep.Runtime + " (" + rep.Directory + ")"
}

// marshalRecord encodes the document the way it is committed.
//
// The one-space indent is what this command's own report already uses, and
// what the other committed records under docs/development are written with;
// every map encoding/json writes sorts its keys, and every list the record
// carries was sorted by the report, so two writes of one classification
// produce the same bytes.
//
// The encoding cannot fail for this type, so it goes through [cmdutil.Must]
// rather than handing its caller an error branch no input reaches: every field
// of the document is a string, an int, a bool, or a slice or map of those, and
// none declares a MarshalJSON, so there is no channel, function, cycle or
// non-finite float for encoding/json to refuse.
func marshalRecord(doc *coverageRecord) []byte {
	return cmdutil.Must(json.MarshalIndent(doc, "", " "))
}

// recordRegenerate names the target that refreshes the record, and
// pageRegenerate the one that redraws the page from it. They are different
// commands on purpose: the first needs a booted GitLab and the second needs
// only a checkout, which is what makes the page a member of update-all and
// the record not.
const (
	recordRegenerate = "make e2e-coverage-record"
	pageRegenerate   = "make e2e-coverage-record-render"
)

// writeRecordPage renders the page from the document and writes it, or --
// with check -- reports whether the committed page is exactly that rendering.
func writeRecordPage(path string, doc *coverageRecord, check bool) error {
	return docgen.WriteOrCheck(path, []byte(renderRecordPage(doc)), check, pageRegenerate)
}

// recordPaths resolves the two artifact paths from the options.
func recordPaths(opts options) (recordPath, pagePath string) {
	recordPath = opts.recordPath
	if recordPath == "" {
		recordPath = filepath.Join(opts.dir, filepath.FromSlash(recordRelPath))
	}
	pagePath = opts.recordPage
	if pagePath == "" {
		pagePath = filepath.Join(opts.dir, filepath.FromSlash(recordPageRelPath))
	}
	return recordPath, pagePath
}

// runRecordWrite writes the record from this run's reports.
//
// The static scan is judged by what it produced and not by the flag that asked
// for it, through the same [staticApplied] the classification itself is gated
// on. The flag is the weaker question by a distance: a scan that could not load
// the packages -- which any type error under the e2e build tag does -- leaves a
// nil result, and a tree with no test/e2e/gitlab in it leaves a skipped one,
// and in both cases the classification this would commit is exactly the
// over-generous one the guard exists to keep out of the document, written
// with exit status 0 while the run's own status says the gate failed.
func runRecordWrite(opts options, static *staticResult, reports []*report, stdout, stderr io.Writer) int {
	if opts.results == "" || !opts.static || !staticApplied(static) {
		fmt.Fprintln(stderr, "audit_e2e_coverage: -record needs -results and a -static scan that ran: "+
			"without the results stream a failed test still counts as coverage, and without the static scan a call "+
			"site that throws its answer away does too, so the record would freeze the most generous classification "+
			"this command can produce rather than the one the gates judge by")
		return exitUsage
	}
	if len(reports) == 0 {
		// Without this the write would rewrite the document with nothing in
		// it, leaving the committed figures intact and the page redrawn from
		// them, which reads as a successful refresh of a run that selected
		// no runtime at all.
		fmt.Fprintln(stderr, "audit_e2e_coverage: -record selected no runtime: there is nothing to record")
		return exitUsage
	}
	recordPath, pagePath := recordPaths(opts)
	if err := writeRecord(recordPath, pagePath, reports); err != nil {
		fmt.Fprintln(stderr, recordErrPrefix, err)
		return exitUsage
	}
	for _, rep := range reports {
		key, _ := recordKeyFor(rep.Runtime)
		fmt.Fprintf(stdout, "record: %s: L1 %d/%d, L2 %d, L3 %d, written to %s\n",
			key, rep.Summary.L1, rep.Summary.CatalogActions, rep.Summary.L2, rep.Summary.L3, recordPath)
	}
	return exitOK
}

// runRecordRender redraws the page from the committed record.
func runRecordRender(opts options, stdout, stderr io.Writer) int {
	recordPath, pagePath := recordPaths(opts)
	doc, err := readRecord(recordPath)
	if err != nil {
		fmt.Fprintln(stderr, recordErrPrefix, err)
		return exitUsage
	}
	if err = writeRecordPage(pagePath, doc, false); err != nil {
		fmt.Fprintln(stderr, recordErrPrefix, err)
		return exitUsage
	}
	fmt.Fprintf(stdout, "record: rendered %s from %s\n", pagePath, recordPath)
	return exitOK
}
