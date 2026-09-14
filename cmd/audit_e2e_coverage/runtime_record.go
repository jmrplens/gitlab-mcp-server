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

// recordSchemaVersion is the version of the committed document's shape.
//
// It is refused rather than read half-way when it is not this one, on the
// terms [e2ecalls.Record] holds a shard to: a document written by another
// version of this command may spell a field differently, and a check that
// read what it recognized and ignored the rest would report a coverage figure
// it had only half understood.
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
		RetrievedAt: retrievedAt,
		Runtime:     rep.Runtime,
		Edition:     rep.Edition,
		Tier:        rep.Tier,
		Runs:        rep.Runs,
		Sessions:    rep.Sessions,
		Summary:     rep.Summary,
		Levels:      rep.Levels,
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

// runIDStampLayout is the UTC timestamp a run ID opens with, spelled the way
// the harness formats it. It is duplicated here rather than exported from the
// harness because the harness is under the e2e build tag and this command is
// not; what holds the two together is that the committed shard fixtures carry
// real run IDs, so a layout change that left this constant behind leaves
// every entry dateless and the build test fails.
const runIDStampLayout = "20060102t150405z"

// errRecordDate is a run ID whose stamp says nothing about when the run
// happened.
var errRecordDate = errors.New("no run ID carries a timestamp this can read, so the entry would have no date and nothing could say how old it is")

// recordDate reads the day of the run off its run IDs.
//
// The earliest stamp wins, not the latest: a directory's packages start
// minutes apart and the window asks how long the measurement has been
// standing, so the honest answer is the age of its oldest part. A refused
// package writes a run line with no run ID at all and is skipped rather than
// refused, since it recorded no calls to be stale about.
func recordDate(rep *report) (string, error) {
	var earliest time.Time
	for _, run := range rep.Runs {
		stamp, _, found := strings.Cut(run.RunID, "-")
		if !found {
			continue
		}
		at, err := time.Parse(runIDStampLayout, stamp)
		if err != nil {
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
	for _, rep := range reps {
		key, _ := recordKeyFor(rep.Runtime)
		entry, buildErr := buildRecordEntry(rep)
		if buildErr != nil {
			return buildErr
		}
		doc.Runtimes[key] = entry
	}
	encoded, err := marshalRecord(doc)
	if err != nil {
		return err
	}
	if writeErr := docgen.WriteOrCheck(recordPath, encoded, false, recordRegenerate); writeErr != nil {
		return writeErr
	}
	return writeRecordPage(pagePath, doc, false)
}

// marshalRecord encodes the document the way it is committed.
//
// The one-space indent is what this command's own report already uses, and
// what the other committed records under docs/development are written with;
// every map encoding/json writes sorts its keys, and every list the record
// carries was sorted by the report, so two writes of one classification
// produce the same bytes.
func marshalRecord(doc *coverageRecord) ([]byte, error) {
	encoded, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		return nil, fmt.Errorf("encode the coverage record: %w", err)
	}
	return encoded, nil
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
func runRecordWrite(opts options, reports []*report, stdout, stderr io.Writer) int {
	if opts.results == "" || !opts.static {
		fmt.Fprintln(stderr, "audit_e2e_coverage: -record needs -results and -static: "+
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
		fmt.Fprintln(stderr, "audit_e2e_coverage: record:", err)
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
		fmt.Fprintln(stderr, "audit_e2e_coverage: record:", err)
		return exitUsage
	}
	if err = writeRecordPage(pagePath, doc, false); err != nil {
		fmt.Fprintln(stderr, "audit_e2e_coverage: record:", err)
		return exitUsage
	}
	fmt.Fprintf(stdout, "record: rendered %s from %s\n", pagePath, recordPath)
	return exitOK
}
