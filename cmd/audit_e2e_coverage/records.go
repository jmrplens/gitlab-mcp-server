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
// runtime, only the run line does, so the directory is the unit a runtime can
// be recovered at: a Docker target writes into one, and every run line in it
// agrees. One shard is one package: a process writes one file and one run
// line, so the run line beside a call is the package that made it, which is
// the only place that attribution exists and why the shards are read apart.
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
	// toolSessions are the session lines whose shard carries a tools/call
	// under their label. Only those can say anything about dispatch: the
	// server's span is a tool dispatch, so a session that read resources,
	// asked for completions or only listed its surface has no span to wait
	// for, and its unobserved flag is not evidence that one went missing.
	//
	// The join is by label within one shard, since a label repeats across
	// packages and a shard is one package's process.
	toolSessions map[*e2ecalls.Session]bool
	// calls are the call lines, each with Dispatched filled from its dispatch
	// line when the call was flushed before the span arrived.
	calls []*e2ecalls.Call
	// packages names the package each call was recorded by, read off the run
	// line of the shard the call sits in. A call from a shard with no run
	// line, or with more than one, has no entry: its package is unknown and
	// is never guessed.
	packages map[*e2ecalls.Call]string
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
// e2ecalls.ReadShards gives for a directory without shards.
//
// The directory is listed once and both questions are asked of that listing.
// Listing it a second time to find its children could only differ from the
// first by the directory changing in between, and an error branch no input
// but a race reaches is one no test can hold to anything.
func readRuntimes(dir string) ([]*runtimeRecords, error) {
	entries, err := listDirectory(dir)
	if err != nil {
		return nil, err
	}
	if holdsShard(entries) {
		one, readErr := readRuntime(dir)
		if readErr != nil {
			return nil, readErr
		}
		return []*runtimeRecords{one}, nil
	}
	var runtimes []*runtimeRecords
	for _, child := range childDirectories(dir, entries) {
		childEntries, childErr := listDirectory(child)
		if childErr != nil {
			return nil, childErr
		}
		if !holdsShard(childEntries) {
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

// readDir is os.ReadDir behind a variable, for the one failure of
// [readRuntimes] no input reaches: a child directory the parent's listing
// named that cannot then be listed itself. A test process that can write the
// tree can also read it, and the only other ways to get there are a race and
// a permission that root ignores, so a test hands in a listing that fails
// instead. What it holds to is that such a child refuses the whole read rather
// than being passed over as a directory that holds nothing.
var readDir = os.ReadDir

// listDirectory lists dir, naming it in the error a listing that failed gives.
func listDirectory(dir string) ([]os.DirEntry, error) {
	entries, err := readDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	return entries, nil
}

// holdsShard reports whether a directory's listing holds at least one shard
// file directly.
func holdsShard(entries []os.DirEntry) bool {
	for _, entry := range entries {
		// The predicate is the record package's own, because this is the same
		// question its reader asks a moment later: a second spelling of the
		// name here is how the two would come to disagree about what a shard
		// is called.
		if !entry.IsDir() && e2ecalls.IsShard(entry.Name()) {
			return true
		}
	}
	return false
}

// childDirectories names the subdirectories a listing of dir holds, sorted by
// name so a report is stable from one run to the next.
func childDirectories(dir string, entries []os.DirEntry) []string {
	var children []string
	for _, entry := range entries {
		if entry.IsDir() {
			children = append(children, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(children)
	return children
}

// errNoRunLine is a shard set with calls and no run line, which is a process
// that never reached its exit flush: what it recorded cannot be placed on a
// runtime and is refused rather than guessed at.
var errNoRunLine = errors.New("no run line: the package never wrote its exit record, so its runtime is unknown")

// readRuntime reads one directory as one runtime.
func readRuntime(dir string) (*runtimeRecords, error) {
	shards, err := e2ecalls.ReadShards(dir)
	if err != nil {
		return nil, err
	}
	rt, err := foldShards(shards)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	rt.dir = dir
	return rt, nil
}

// foldRecords is [foldShards] over lines with no file boundary, for a
// caller that holds records rather than shards: every call is placed in no
// package, as a shard without a run line would place it.
func foldRecords(records []e2ecalls.Record) (*runtimeRecords, error) {
	return foldShards([]e2ecalls.Shard{{Records: records}})
}

// foldShards sorts records by line type, places every call in the package
// of its shard, joins dispatches to calls and settles the runtime the run
// lines name.
func foldShards(shards []e2ecalls.Shard) (*runtimeRecords, error) {
	rt := &runtimeRecords{packages: map[*e2ecalls.Call]string{}}
	dispatches := map[string]*e2ecalls.Dispatch{}
	for _, shard := range shards {
		var calls []*e2ecalls.Call
		var runs []*e2ecalls.Run
		var sessions []*e2ecalls.Session
		for _, record := range shard.Records {
			switch record.Type {
			case e2ecalls.TypeRun:
				runs = append(runs, record.Run)
			case e2ecalls.TypeSession:
				sessions = append(sessions, record.Session)
			case e2ecalls.TypeCall:
				calls = append(calls, record.Call)
			case e2ecalls.TypeDispatch:
				rt.dispatches++
				dispatches[record.Dispatch.TraceID] = record.Dispatch
			case e2ecalls.TypeSkip:
				rt.skips = append(rt.skips, record.Skip)
			}
		}
		rt.runs = append(rt.runs, runs...)
		rt.sessions = append(rt.sessions, sessions...)
		rt.calls = append(rt.calls, calls...)
		rt.markToolSessions(sessions, calls)
		if len(runs) == 1 && runs[0].Package != "" {
			for _, call := range calls {
				rt.packages[call] = runs[0].Package
			}
		}
	}
	rt.joinDispatches(dispatches)
	if err := rt.settleRuntime(); err != nil {
		return nil, err
	}
	return rt, nil
}

// markToolSessions records which of one shard's session lines called a tool.
func (rt *runtimeRecords) markToolSessions(sessions []*e2ecalls.Session, calls []*e2ecalls.Call) {
	labels := map[string]bool{}
	for _, call := range calls {
		if call.Method == methodCallTool {
			labels[call.Session] = true
		}
	}
	for _, session := range sessions {
		if !labels[session.Label] {
			continue
		}
		if rt.toolSessions == nil {
			rt.toolSessions = map[*e2ecalls.Session]bool{}
		}
		rt.toolSessions[session] = true
	}
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
