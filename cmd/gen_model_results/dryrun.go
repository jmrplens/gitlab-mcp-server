// The rehearsal: the whole publishing path run against a throwaway tree, so a
// maintainer can read the pages a run would produce before spending anything.
//
// `fake:perfect` already walks the corpus key through the binary, GitLab, the
// recording and the scoring, but folding it publishes nothing: the
// fake-provider rule refuses every row it would make, on the grounds that the
// fake answers from this repository's own key and its figures are a reading of
// that rather than of a model. That refusal is right and stays. What it cost is
// that nobody could see how a result is stored or how a page is drawn from it
// without first spending money on a real run.
//
// So a dry run sets aside that one rule and writes somewhere the committed
// record cannot be reached from. Two properties make it safe to look at and
// impossible to mistake:
//
//   - It never writes into the repository. The record and both pages are
//     assembled under dist/, which Git ignores, and the real ones are read for
//     their markers and never modified.
//   - Every page it writes opens with a banner saying the figures are a
//     rehearsal and describe no model's ability.
//
// Every other refusal still runs. A dry run of shards that are stale, filtered,
// unobserved or short of provenance is refused exactly as a real fold would
// refuse them, because the point is to rehearse the real thing.

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// dryRunRelDir is where a rehearsal is assembled, under the ignored dist tree.
const dryRunRelDir = "dist/modeleval/dry-run"

// dryRunBanner opens every page a rehearsal writes.
//
// It is prepended to the file rather than rendered into a block because the
// blocks are the thing being rehearsed: a banner inside one would be a
// difference between what a dry run draws and what a real fold draws, which is
// the one thing a rehearsal must not have.
const dryRunBanner = "> **This is a rehearsal, not a measurement.**\n" +
	"> These pages were drawn by `gen_model_results -dry-run` from a run that\n" +
	"> answered from the corpus's own answer key rather than from a model. The\n" +
	"> figures say that the recording, the scoring and the rendering work. They\n" +
	"> say nothing whatever about any model, and nothing here is published.\n\n"

// prepareDryRun builds the throwaway tree a rehearsal is written into and
// returns its root.
//
// The pages are copied from the real ones so that the blocks a rehearsal draws
// are the blocks the repository actually carries: a fixture written here would
// drift from them, and a rehearsal that draws a page nobody publishes rehearses
// nothing. Anything left by an earlier rehearsal is removed first, so what is
// on disk afterwards is one run's output and not two runs' mixed.
func prepareDryRun(root string, stdout io.Writer) (string, error) {
	scratch := filepath.Join(root, dryRunRelDir)
	if err := os.RemoveAll(scratch); err != nil {
		return "", fmt.Errorf("clear the dry-run directory: %w", err)
	}
	for _, path := range pagePaths() {
		body, err := os.ReadFile(filepath.Join(root, path)) //#nosec G304 -- one of this command's own page constants under the repository root
		if err != nil {
			return "", fmt.Errorf("read %s to rehearse it: %w", path, err)
		}
		full := filepath.Join(scratch, path)
		if mkErr := os.MkdirAll(filepath.Dir(full), 0o750); mkErr != nil {
			return "", fmt.Errorf("make the dry-run directory: %w", mkErr)
		}
		//#nosec G703 -- path is one of this command's own page constants, joined under the repository's own dist directory
		if writeErr := os.WriteFile(full, body, 0o600); writeErr != nil {
			return "", fmt.Errorf("copy %s into the dry run: %w", path, writeErr)
		}
	}
	fmt.Fprintf(stdout, logLead+"rehearsing into %s, which is ignored by git; nothing published is touched\n", dryRunRelDir)
	return scratch, nil
}

// bannerDryRun puts the rehearsal banner at the top of every page the dry run
// drew, after the blocks are in place.
func bannerDryRun(scratch string, stdout io.Writer) error {
	for _, path := range pagePaths() {
		full := filepath.Join(scratch, path)
		body, err := os.ReadFile(full) //#nosec G304 -- a path this command just wrote under dist/
		if err != nil {
			return fmt.Errorf("read the rehearsed %s: %w", path, err)
		}
		//#nosec G703 -- the same page constants under the same dist directory, re-read from where this command just wrote them
		if writeErr := os.WriteFile(full, append([]byte(dryRunBanner), body...), 0o600); writeErr != nil {
			return fmt.Errorf("mark the rehearsed %s: %w", path, writeErr)
		}
		fmt.Fprintf(stdout, logLead+"rehearsed %s\n", filepath.Join(dryRunRelDir, path))
	}
	return nil
}
