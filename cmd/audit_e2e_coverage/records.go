package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// runtimeRecords is everything the shards of one directory said, folded by
// line type with every dispatch joined to its call.
//
// One directory is one runtime. No call, session or skip line carries the
// runtime, only the run line does, and e2ecalls.Read merges the files of a
// directory without saying which line came from which, so the directory is
// the unit a runtime can be recovered at: a Docker target writes into one, and
// every run line in it agrees.
type runtimeRecords struct {
	// dir is the directory the records were read from.
	dir string
	// key names the runtime as edition/tier.
	key string
	// edition is the run lines' edition token.
	edition string
	// tier is the tier the catalog was built at.
	tier edition.Tier
	// runs are the run lines, one per package that started or refused.
	runs []*e2ecalls.Run
	// sessions are the session lines, as recorded. A label repeats across
	// packages, and the fold reads surface and mode off each call line rather
	// than joining on it.
	sessions []*e2ecalls.Session
	// calls are the call lines, each with Dispatched filled from its dispatch
	// line when the call was flushed before the span arrived.
	calls []*e2ecalls.Call
	// skips are the skip lines.
	skips []*e2ecalls.Skip
	// dispatches is how many dispatch lines the shards carried, kept for the
	// diagnostics: a runtime with calls and no dispatch lines never saw a
	// span, which is a harness failure and not a coverage figure.
	dispatches int
	// lateJoins counts the calls whose dispatched action came from a separate
	// dispatch line rather than the call line itself.
	lateJoins int
}

// readRuntimes reads every runtime under dir.
//
// A directory holding a shard is one runtime, its subdirectories included; a
// directory holding none is read as one runtime per child directory, which is
// the layout dist/e2e-calls/<target> produces. Anything else is the error
// e2ecalls.Read gives for a directory without shards.
func readRuntimes(dir string) ([]*runtimeRecords, error) {
	holds, err := holdsShard(dir)
	if err != nil {
		return nil, err
	}
	if holds {
		one, readErr := readRuntime(dir)
		if readErr != nil {
			return nil, readErr
		}
		return []*runtimeRecords{one}, nil
	}
	children, err := childDirectories(dir)
	if err != nil {
		return nil, err
	}
	var runtimes []*runtimeRecords
	for _, child := range children {
		childHolds, childErr := holdsShard(child)
		if childErr != nil {
			return nil, childErr
		}
		if !childHolds {
			continue
		}
		one, readErr := readRuntime(child)
		if readErr != nil {
			return nil, readErr
		}
		runtimes = append(runtimes, one)
	}
	if len(runtimes) == 0 {
		// Shards deeper than one level, or none at all: the recursive read
		// answers both, and its refusal names the variable and the pattern,
		// which is the message a reader pointed at an empty directory needs.
		one, readErr := readRuntime(dir)
		if readErr != nil {
			return nil, readErr
		}
		return []*runtimeRecords{one}, nil
	}
	return runtimes, nil
}

// holdsShard reports whether dir directly holds at least one shard file.
func holdsShard(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasPrefix(name, "calls-") && strings.HasSuffix(name, ".jsonl") {
			return true, nil
		}
	}
	return false, nil
}

// childDirectories lists the subdirectories of dir, sorted by name so a report
// is stable from one run to the next.
func childDirectories(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var children []string
	for _, entry := range entries {
		if entry.IsDir() {
			children = append(children, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(children)
	return children, nil
}

// errNoRunLine is a shard set with calls and no run line, which is a process
// that never reached its exit flush: what it recorded cannot be placed on a
// runtime and is refused rather than guessed at.
var errNoRunLine = errors.New("no run line: the package never wrote its exit record, so its runtime is unknown")

// readRuntime reads one directory as one runtime.
func readRuntime(dir string) (*runtimeRecords, error) {
	records, err := e2ecalls.Read(dir)
	if err != nil {
		return nil, err
	}
	rt, err := foldRecords(records)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	rt.dir = dir
	return rt, nil
}

// foldRecords sorts records by line type, joins dispatches to calls and
// settles the runtime the run lines name.
func foldRecords(records []e2ecalls.Record) (*runtimeRecords, error) {
	rt := &runtimeRecords{}
	dispatches := map[string]*e2ecalls.Dispatch{}
	for _, record := range records {
		switch record.Type {
		case e2ecalls.TypeRun:
			rt.runs = append(rt.runs, record.Run)
		case e2ecalls.TypeSession:
			rt.sessions = append(rt.sessions, record.Session)
		case e2ecalls.TypeCall:
			rt.calls = append(rt.calls, record.Call)
		case e2ecalls.TypeDispatch:
			rt.dispatches++
			dispatches[record.Dispatch.TraceID] = record.Dispatch
		case e2ecalls.TypeSkip:
			rt.skips = append(rt.skips, record.Skip)
		}
	}
	rt.joinDispatches(dispatches)
	if err := rt.settleRuntime(); err != nil {
		return nil, err
	}
	return rt, nil
}

// joinDispatches fills the dispatched action of every call that was flushed
// before its span arrived.
//
// A call line already carrying a dispatched action keeps it: the harness made
// that join with the same span, and a dispatch line that disagreed with it
// would be a second record of one event, which cannot happen from one
// receiver.
func (rt *runtimeRecords) joinDispatches(dispatches map[string]*e2ecalls.Dispatch) {
	for _, call := range rt.calls {
		if call.Dispatched != "" || call.TraceID == "" {
			continue
		}
		dispatch, arrived := dispatches[call.TraceID]
		if !arrived || dispatch.Action == "" {
			continue
		}
		call.Dispatched = dispatch.Action
		rt.lateJoins++
	}
}

// settleRuntime reads the runtime off the run lines and refuses a set whose
// run lines disagree.
//
// Two editions or two tiers under one directory would fold two catalogs into
// one report and credit each against the other's denominator. The reader is
// told to point at one directory per runtime instead, which is how the
// Makefile writes them.
func (rt *runtimeRecords) settleRuntime() error {
	if len(rt.runs) == 0 {
		return errNoRunLine
	}
	keys := map[string]*e2ecalls.Run{}
	for _, run := range rt.runs {
		if run.Edition == "" || run.Tier == "" {
			// A refused run that never probed carries neither, and says
			// nothing about which runtime it refused.
			continue
		}
		keys[runtimeKey(run.Edition, run.Tier)] = run
	}
	if len(keys) == 0 {
		return errors.New("no run line names an edition and a tier: every package refused before probing the instance")
	}
	if len(keys) > 1 {
		names := make([]string, 0, len(keys))
		for key := range keys {
			names = append(names, key)
		}
		sort.Strings(names)
		return fmt.Errorf("the run lines name %d runtimes (%s): read one directory per runtime",
			len(names), strings.Join(names, ", "))
	}
	for key, run := range keys {
		tier, known := edition.ParseTier(run.Tier)
		if !known {
			return fmt.Errorf("run line for %s names the tier %q, which is not one this server knows", run.Package, run.Tier)
		}
		rt.key = key
		rt.edition = run.Edition
		rt.tier = tier
	}
	return nil
}

// runtimeKey spells a runtime as its edition and tier.
func runtimeKey(editionToken, tier string) string {
	return editionToken + "/" + tier
}

// matchesRuntime reports whether a runtime key satisfies one -runtime
// selector.
//
// Two shorthands stand for the two Docker targets: ce is any community
// runtime, and ee is an enterprise runtime holding a license. An enterprise
// image with no license is neither, because neither target produces it, and
// a reader expecting ee should be told that its shards came from something
// else. A full edition/tier key matches itself.
func matchesRuntime(key, selector string) bool {
	editionToken, tier, found := strings.Cut(key, "/")
	if !found {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(selector)) {
	case "ce":
		return editionToken == "community"
	case "ee":
		parsed, known := edition.ParseTier(tier)
		return editionToken == "enterprise" && known && parsed.IsEnterprise()
	default:
		return key == strings.TrimSpace(selector)
	}
}

// selectRuntimes keeps the runtimes matching any of the selectors, and every
// runtime when there is no selector.
func selectRuntimes(runtimes []*runtimeRecords, selectors []string) []*runtimeRecords {
	if len(selectors) == 0 {
		return runtimes
	}
	var kept []*runtimeRecords
	for _, rt := range runtimes {
		for _, selector := range selectors {
			if matchesRuntime(rt.key, selector) {
				kept = append(kept, rt)
				break
			}
		}
	}
	return kept
}

// splitSelectors turns the comma-separated -runtime value into its entries,
// with blanks dropped.
func splitSelectors(value string) []string {
	var selectors []string
	for part := range strings.SplitSeq(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			selectors = append(selectors, trimmed)
		}
	}
	return selectors
}
