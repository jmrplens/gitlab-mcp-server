package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

// The exit statuses, spelled the way the other auditors in this tree spell
// them: nothing wrong, something published that should not be, and a command
// that could not be run at all.
const (
	exitOK       = 0
	exitFindings = 1
	exitUsage    = 2
)

// logPrefix names this command on every line it writes, so a maintainer
// reading a `make` log with several generators in it can tell whose sentence
// they are reading. logLead is the same prefix carrying the space itself, for
// the lines that build a format string rather than letting Fprintln put the
// space between its operands.
const (
	logPrefix = "gen_model_results:"
	logLead   = logPrefix + " "
)

// options are the four things this command can be asked to do.
type options struct {
	// shards names a run's record directory to fold into the committed record.
	// Empty leaves the record as it stands.
	shards string
	// refold merges those shards into the rows they publish again, case by
	// case, which is the whole of how a corrected scoring rule or a corrected
	// case reaches a row that is already published. It is a flag rather than
	// the default because the figures it replaces are a paid measurement:
	// replacing them without being asked to is what the duplicate-row rule
	// exists to refuse.
	refold bool
	// render redraws the managed blocks of both pages from the record.
	render bool
	// check verifies the record and the pages offline instead of writing
	// anything.
	check bool
	// dryRun folds and renders into a throwaway directory instead of the
	// repository, admitting the fake provider so a rehearsal can be read as a
	// published page would be. See [prepareDryRun].
	dryRun bool
}

// main folds a run in, redraws the pages, or gates both.
//
// Each flag does one thing and the two writing ones compose, because the two
// halves have different costs: a fold needs a directory of shards a paid run
// left behind, and a render needs only the committed record, which is why it is
// the half update-all runs and the half a stack refreshes at its tip.
func main() {
	shards := flag.String("shards", "", "fold a model evaluation run's shards from this directory into the committed record")
	refold := flag.Bool("refold", false, "with -shards: merge those shards into the rows they publish again, case by case, naming each case replaced")
	render := flag.Bool("render", false, "redraw the managed blocks of README.md and the results page from the record")
	check := flag.Bool("check", false, "verify the committed record and the pages drawn from it, writing nothing")
	dryRun := flag.Bool("dry-run", false, "with -shards: fold and render into "+dryRunRelDir+" instead of the repository, admitting the fake provider so a rehearsal can be read")
	flag.Parse()

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, logLead+"find repository root: %v\n", err)
		os.Exit(exitUsage)
	}
	os.Exit(run(root, options{shards: *shards, refold: *refold, render: *render, check: *check, dryRun: *dryRun}, os.Stdout, os.Stderr))
}

// run is main with its inputs handed to it, so a test can drive every path
// against a tree of its own.
func run(root string, opts options, stdout, stderr io.Writer) int {
	if opts.check && (opts.shards != "" || opts.render || opts.refold || opts.dryRun) {
		fmt.Fprintln(stderr, logLead+"-check writes nothing, so it cannot be given -shards, -refold, -render or -dry-run")
		return exitUsage
	}
	if opts.dryRun && opts.shards == "" {
		fmt.Fprintln(stderr, logLead+"-dry-run rehearses folding a run in, so it needs the -shards of the run to rehearse")
		return exitUsage
	}
	if opts.refold && opts.shards == "" {
		fmt.Fprintln(stderr, logLead+"-refold merges a run's shards into the rows they publish again, so it needs the -shards those rows would come from")
		return exitUsage
	}
	if opts.check {
		return runCheck(root, stdout, stderr)
	}
	if opts.shards == "" && !opts.render {
		fmt.Fprintln(stderr, logLead+"nothing to do: pass -shards to fold a run in, -render to redraw the pages, or -check to gate both")
		return exitUsage
	}
	return runWrite(root, opts, stdout, stderr)
}

// runWrite folds whatever it was given into the record and redraws the pages
// from it.
func runWrite(root string, opts options, stdout, stderr io.Writer) int {
	doc, committed, err := readRecord(root)
	if err != nil {
		fmt.Fprintln(stderr, logPrefix, err)
		return exitUsage
	}

	// A rehearsal reads the committed record and the real pages, and writes
	// neither: from here on, root is a throwaway tree under dist/.
	written := root
	if opts.dryRun {
		scratch, prepErr := prepareDryRun(root, stdout)
		if prepErr != nil {
			fmt.Fprintln(stderr, logPrefix, prepErr)
			return exitUsage
		}
		written = scratch
	}

	if opts.shards != "" {
		if foldErr := foldInto(&doc, opts.shards, opts.refold, opts.dryRun, stdout); foldErr != nil {
			fmt.Fprintln(stderr, logPrefix, foldErr)
			return exitUsage
		}
		if writeErr := writeRecord(written, doc); writeErr != nil {
			fmt.Fprintln(stderr, logPrefix, writeErr)
			return exitUsage
		}
		committed = true
		fmt.Fprintf(stdout, logLead+"wrote %s: %s\n", recordRelPath, comparisonSummary(doc.Rows))
	}

	if opts.render {
		changed, _, pageErr := applyPages(written, doc.Rows, false)
		if pageErr != nil {
			fmt.Fprintln(stderr, logPrefix, pageErr)
			return exitUsage
		}
		for _, path := range changed {
			fmt.Fprintf(stdout, logLead+"redrew the managed blocks of %s\n", path)
		}
		if !committed {
			fmt.Fprintf(stdout, logLead+"no %s yet, so every block says that nothing is published\n", recordRelPath)
		}
	}
	if opts.dryRun {
		if markErr := bannerDryRun(written, stdout); markErr != nil {
			fmt.Fprintln(stderr, logPrefix, markErr)
			return exitUsage
		}
	}
	for _, orphan := range unpublishedRows(doc.Rows) {
		fmt.Fprintf(stderr, logLead+"no managed block publishes the row %s\n", orphan)
	}
	return exitOK
}

// foldInto reads a run's shards, judges every row they would publish and adds
// the survivors to the document.
//
// Every refusal is printed, by rule and by row. A fold in which nothing
// survives is not an error: it is what a fake provider run is for, and saying
// so row by row is more use than a status.
//
// With refold set, the rows those same shards publish again are merged into
// **case by case**: the cases the shards measured replace their own entries and
// every other case keeps the figures it had. That is the whole of the
// re-scoring path this command offers, and it is deliberately narrow: a
// published row is the scoring of the runs that were folded into it, nothing
// re-computes it, and the only way a corrected rule or a corrected case reaches
// it is this, over the shards those runs left behind.
//
// The case is the unit and not the row, which it was until a re-run of one
// corrected case was found to replace a row measured over every case with a row
// measured over that one, report success, and leave nothing saying the rest of
// a paid run had been discarded.
func foldInto(doc *document, dir string, refold, dryRun bool, stdout io.Writer) error {
	shards, err := modelrecord.ReadShards(dir)
	if err != nil {
		return err
	}
	claimed := doc.claims()
	if refold {
		// A re-fold replaces what these shards measured, so the rows they name
		// stop being claimed by the record and are merged into below instead
		// of refused as duplicates.
		if dropErr := unclaimRefolded(shards, claimed); dropErr != nil {
			return dropErr
		}
	}
	candidates, err := fold(shards, claimed)
	if err != nil {
		return err
	}
	rows, refusals, err := judge(candidates, modelcorpus.Keys(), dryRun)
	if err != nil {
		return err
	}
	for _, one := range refusals {
		fmt.Fprintln(stdout, logPrefix, one)
	}
	fmt.Fprintf(stdout, logLead+"%s: %d candidate row(s), %d published, %d refused by [%s]\n",
		dir, len(candidates), len(rows), len(refusals), ruleNames())
	mergeRows(doc, rows, stdout)
	return nil
}

// unclaimRefolded releases the row keys the given shards publish again, so a
// re-fold is not refused by the rule that stops a run being folded twice.
//
// The keys come from a fold of the shards against nothing, which is the only
// honest way to say which rows they replace: a row is identified by its key and
// by nothing else.
func unclaimRefolded(shards []modelrecord.Shard, claimed map[string]string) error {
	replacing, err := fold(shards, map[string]string{})
	if err != nil {
		return err
	}
	for _, cand := range replacing {
		delete(claimed, cand.key.String())
	}
	return nil
}

// mergeRows folds new rows into the record, case by case where a row of the
// same key already stands.
//
// This is the whole of what a re-run of one case needs, and the reason it is a
// merge rather than a replacement is what replacement did: a row measured over
// every case, re-folded from the shards of a single corrected case, was
// replaced by a row measured over that one case, and the run that produced the
// other 257 was gone with nothing saying so. Here the incoming cases replace
// their own entries, every other case keeps the figures it had, and the three
// published blocks are re-derived from the union.
//
// Each replacement is named, because a maintainer who asked to re-score one
// case should be told exactly which cases changed hands and not have to diff
// the record to find out.
func mergeRows(doc *document, rows []row, stdout io.Writer) {
	for _, incoming := range rows {
		name := incoming.Key.String()
		held := -1
		for i, standing := range doc.Rows {
			if standing.Key.String() == name {
				held = i
				break
			}
		}
		if held < 0 {
			doc.Rows = append(doc.Rows, incoming)
			continue
		}

		standing := doc.Rows[held]
		if why := mergeRefusal(standing, incoming); why != "" {
			// Refused, and the standing row is left exactly as it stood. The
			// incoming figures are dropped rather than published under a
			// heading that would describe only one of the two runs behind
			// them, which is the fold this record exists to stop.
			fmt.Fprintf(stdout, logLead+"refused to merge into the published row %s: %s\n", name, why)
			continue
		}
		replaced := replacedCases(standing.Cases, incoming.Cases)
		kept := len(standing.Cases) - len(replaced)
		merged := mergeCases(standing.Cases, incoming.Cases)

		incoming.Cases = merged
		incoming.Counts, incoming.Columns, incoming.Tokens = sumCases(merged)
		doc.Rows[held] = incoming

		fmt.Fprintf(stdout, logLead+"merged into the published row %s: %d case(s) replaced (%s), %d kept as they stood\n",
			name, len(replaced), strings.Join(replaced, ", "), kept)
	}
}

// runCheck is the offline gate: the record against the rules, and the pages
// against the record.
//
// A tree with no committed record passes with a note. That is the state from
// this step until the first paid run is published, and failing on it would make
// every push red over a file nobody can write without spending money. The pages
// are still compared, because they are generated in that state too: what they
// are compared against is the rendering of a record with no rows, which is the
// sentence saying so.
func runCheck(root string, stdout, stderr io.Writer) int {
	doc, committed, err := readRecord(root)
	if err != nil {
		fmt.Fprintln(stderr, logPrefix, err)
		return exitFindings
	}
	var findings []string
	if !committed {
		fmt.Fprintf(stdout, logLead+"note: no %s, so nothing is published yet\n", recordRelPath)
	} else {
		for _, one := range reviewRows(doc) {
			findings = append(findings, one.String())
		}
		findings = append(findings, unpublishedFindings(doc.Rows)...)
		for _, note := range revisionNotes(root, doc) {
			fmt.Fprintln(stdout, logLead+"note:", note)
		}
	}

	_, stale, err := applyPages(root, doc.Rows, true)
	if err != nil {
		fmt.Fprintln(stderr, logPrefix, err)
		return exitFindings
	}
	findings = append(findings, stale...)

	for _, finding := range findings {
		fmt.Fprintln(stderr, logPrefix, finding)
	}
	if len(findings) > 0 {
		return exitFindings
	}
	fmt.Fprintf(stdout, logLead+"%s and the blocks drawn from it agree: %s\n",
		recordRelPath, comparisonSummary(doc.Rows))
	return exitOK
}

// unpublishedFindings names the rows no block publishes, as findings rather
// than notes: a measurement that is committed and shown to nobody is the same
// silence a dropped row would be.
func unpublishedFindings(rows []row) []string {
	var findings []string
	for _, orphan := range unpublishedRows(rows) {
		findings = append(findings, fmt.Sprintf("no managed block publishes the row %s; add one or widen a block", orphan))
	}
	return findings
}

// readRecord reads the committed document, and reports whether there was one.
//
// A missing file is not an error here, unlike everywhere else a record is read
// in this tree: this one is written by a paid run and does not exist yet, so
// the gate has to be able to say "nothing is published" rather than "the record
// is missing".
func readRecord(root string) (document, bool, error) {
	data, err := os.ReadFile(filepath.Join(root, recordRelPath)) //#nosec G304 -- the path is this command's own constant under the repository root
	if errors.Is(err, os.ErrNotExist) {
		return document{SchemaVersion: recordSchemaVersion, Note: recordNote}, false, nil
	}
	if err != nil {
		return document{}, false, fmt.Errorf("read %s: %w", recordRelPath, err)
	}
	doc, err := unmarshalDocument(data)
	if err != nil {
		return document{}, false, err
	}
	doc.Note = recordNote
	return doc, true, nil
}

// writeRecord writes the committed document.
func writeRecord(root string, doc document) error {
	doc.SchemaVersion = recordSchemaVersion
	doc.Note = recordNote
	body, err := doc.marshal()
	if err != nil {
		return err
	}
	if writeErr := docgen.WriteOrCheck(filepath.Join(root, recordRelPath), body, false, regenerate); writeErr != nil {
		return fmt.Errorf("model results record: %w", writeErr)
	}
	return nil
}

// applyPages rewrites, or compares, every managed block of both pages.
//
// One read and one write per file rather than per block, because two blocks of
// one file must be replaced in the text the first replacement produced; the
// four README blocks written one at a time over the file on disk would each
// overwrite the last.
func applyPages(root string, rows []row, check bool) (changed, stale []string, err error) {
	for _, path := range pagePaths() {
		full := filepath.Join(root, path)
		data, readErr := os.ReadFile(full) //#nosec G304 -- the path is one of this command's own constants under the repository root
		if readErr != nil {
			return nil, nil, fmt.Errorf("read %s: %w", path, readErr)
		}
		updated, blockErr := applyBlocks(string(data), path, rows)
		if blockErr != nil {
			return nil, nil, blockErr
		}
		if updated == string(data) {
			continue
		}
		if check {
			stale = append(stale, fmt.Sprintf("the managed blocks of %s do not match %s; run %s",
				path, recordRelPath, regenerate))
			continue
		}
		//#nosec G703,G306 -- the path is one of this command's own constants joined to the repository root, and the file was just read from it; the mode is the one every generated artifact here is written with
		if writeErr := os.WriteFile(full, []byte(updated), docgen.GeneratedFileMode); writeErr != nil {
			return nil, nil, fmt.Errorf("write %s: %w", path, writeErr)
		}
		changed = append(changed, path)
	}
	return changed, stale, nil
}

// applyBlocks replaces every managed block of one file in the text it was read
// from.
func applyBlocks(text, path string, rows []row) (string, error) {
	for _, b := range blocks {
		if b.Path != path {
			continue
		}
		updated, err := docgen.ComputeReplacedSection(text, b.Start, b.End, renderBlock(b, rows))
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		text = updated
	}
	return text, nil
}

// pagePaths lists the files holding a managed block, in the order the blocks
// declare them and without repeats.
func pagePaths() []string {
	var paths []string
	seen := map[string]bool{}
	for _, b := range blocks {
		if seen[b.Path] {
			continue
		}
		seen[b.Path] = true
		paths = append(paths, b.Path)
	}
	return paths
}

// fullSHA is what a recorded commit must look like before it is handed to git
// as an argument. A value read out of a committed file is not this command's
// own, and the probe below runs a process with it.
var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// gitProbe answers a yes-or-no question of the repository. It is a variable so
// that the three outcomes below -- git absent, the revision unknown, the
// revision known and not an ancestor -- are each reachable from a test, which
// none of them is from a checkout in one state.
var gitProbe = askGit

// gitProbeTimeout bounds one revision question. Both are answered out of the
// object database and take milliseconds; the bound is there so a repository on
// a filesystem that has stopped answering costs the gate a few seconds rather
// than the job's whole timeout.
const gitProbeTimeout = 10 * time.Second

// askGit runs one git command and reports whether it exited zero.
//
// The program is resolved to an absolute path first, which is how every other
// command here runs a tool it did not build (cmd/gen_stats resolves git this
// way, cmd/gen_icon_webp its two converters): the lookup is the same one exec
// would do, and what changes is that the program is decided here rather than
// by whatever the process's PATH happens to hold when the command is started.
// A machine with no git reports that as the error it is, which is the silence
// the notes above are built on.
func askGit(dir string, args ...string) (bool, error) {
	bin, lookErr := exec.LookPath("git")
	if lookErr != nil {
		return false, fmt.Errorf("find git: %w", lookErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitProbeTimeout)
	defer cancel()
	// #nosec G204 -- the program is the absolute path just resolved, and the
	// arguments are this file's own literals plus a commit the caller has
	// already held to fullSHA, which admits nothing a shell or git could read
	// as an option or a path.
	cmd := exec.CommandContext(ctx, bin, args...)
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

// revisionNotes say which rows were measured on a tree that is not in this
// history, and say nothing at all when they cannot tell.
//
// The silence is the point. CI checks this repository out at depth one, so
// every revision but HEAD is unknown there, and a note that fired whenever a
// revision could not be resolved would fire on every run of every pull request
// and inform nobody. A revision git does know and cannot reach from HEAD is a
// different matter, and it is what the withdrawn tables turned out to have been
// measured on: a reader comparing such a row with this tree should be told.
//
// It reports and never fails, for the reason the end-to-end coverage record
// learned: a shallow clone cannot resolve the question, and a gate that failed
// on an answer it could not get would fail on the clone rather than on the row.
func revisionNotes(root string, doc document) []string {
	var notes []string
	seen := map[string]bool{}
	for _, one := range doc.Rows {
		commit := one.Provenance.Commit
		if seen[commit] || !fullSHA.MatchString(commit) {
			continue
		}
		seen[commit] = true
		known, err := gitProbe(root, "cat-file", "-e", commit+"^{commit}")
		if err != nil || !known {
			continue
		}
		ancestor, err := gitProbe(root, "merge-base", "--is-ancestor", commit, "HEAD")
		if err != nil || ancestor {
			continue
		}
		notes = append(notes, fmt.Sprintf(
			"a row was measured on %s, which is not an ancestor of HEAD: the tree it ran against is not this one", commit,
		))
	}
	return notes
}
