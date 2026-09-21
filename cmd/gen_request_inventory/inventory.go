package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

const (
	// shardExt is what the recorder names its files. One shard per test
	// process, so a package binary never shares a file with another.
	shardExt = ".jsonl"

	// maxShardLine bounds one recorded line. The longest today is a GraphQL
	// row with a dozen variables at a few hundred bytes, and the default
	// scanner limit of 64 KiB would be enough, but a line silently dropped
	// for being long would look exactly like a request nobody makes.
	maxShardLine = 1 << 20
)

// shardRecord is one line of a recorder shard.
//
// It repeats the shape of requestRecord in internal/testutil rather than
// sharing the type: that package embeds the pinned GraphQL schema and
// net/http/httptest, and a command importing it would carry both into a binary
// with no use for either. The JSON field names are the contract, and both
// declarations say so.
type shardRecord struct {
	Package string `json:"package"`
	// Test names the test that built the client. It is read and deliberately
	// not written: the committed inventory is about the requests this server
	// makes, so adding a test that reaches an endpoint already listed must not
	// change it. The shard keeps it so a row can still be traced back.
	Test      string   `json:"test"`
	Kind      string   `json:"kind"`
	Method    string   `json:"method"`
	Path      string   `json:"path"`
	Query     []string `json:"query,omitempty"`
	Body      []string `json:"body,omitempty"`
	Operation string   `json:"operation,omitempty"`
	Variables []string `json:"variables,omitempty"`
	// Identifiers is the running count of distinct raw values the recorder had
	// seen behind each placeholder when it wrote this line. It rises across a
	// shard, so the row's answer is the highest line's; see [mergeIdentifiers].
	Identifiers map[string]int `json:"identifiers,omitempty"`
}

// row is one endpoint one package was seen to call.
//
// The shape of the committed artifact lives in cmd/internal/requestinventory,
// because the audit that reads it has to agree with the generator that writes
// it about what a row is and which packages recorded nothing.
type row = requestinventory.Row

// rowKey identifies the row a record belongs to.
type rowKey struct {
	pkg       string
	kind      string
	method    string
	path      string
	operation string
}

// recording is one shard directory: the records a suite run left there, and
// when it left them.
//
// The time travels with the records because this command merges a recording it
// did not make. `make check-request-inventory` compares the committed artifact
// with whatever run last wrote shards, which is an answer about that run and
// only about the tree as it stands if the recording was made from it, and
// nothing in the merged rows says when they were observed.
type recording struct {
	records []shardRecord
	written time.Time
}

// shardInfo reads one directory entry's metadata. It is a variable so the
// branch that cannot read it stays reachable from a test, the way
// catalogActions is: the only way an entry ReadDir just listed refuses to
// describe itself is a race with its own removal.
var shardInfo = func(entry os.DirEntry) (os.FileInfo, error) { return entry.Info() }

// readDirEntries lists the shard directory. It is a variable for the reason
// shardInfo is one: the third refusal below, where the path is there, is a
// directory, and still cannot be read, is a permission or a resource limit,
// and a test cannot arrange either — the suite runs as root both in CI and on
// the builder, and root is exempt from the permission half. Leaving that
// branch unreached would leave the three refusals, which are fixed by three
// different things, told apart by nothing.
var readDirEntries = os.ReadDir

// readShards reads every shard in dir.
//
// An empty directory is an error rather than an empty inventory: the shards
// are written by a suite run, so nothing there means the suite did not run
// with recording on, and writing the empty result would erase the artifact and
// call it a change.
func readShards(dir string) (recording, error) {
	entries, err := readDirEntries(dir)
	if err != nil {
		// Which of the two refusals this is cannot be read off the error
		// class, because the two platforms disagree about it. Opening a
		// regular file as a directory is ENOTDIR on Unix, which is not
		// fs.ErrNotExist, and on Windows it reports ERROR_PATH_NOT_FOUND,
		// which Go maps to ENOENT, which is. Asking the filesystem what the
		// path actually is settles it the same way everywhere, and the
		// distinction is worth keeping: a path that is not there means nobody
		// has recorded a run, and a path that is a file means whoever set
		// GITLAB_MCP_TEST_INVENTORY_DIR pointed it at the wrong thing.
		if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
			return recording{}, fmt.Errorf("read shard directory: %s is not a directory", dir)
		}
		if errors.Is(err, fs.ErrNotExist) {
			return recording{}, notRecorded(dir)
		}
		return recording{}, fmt.Errorf("read shard directory: %w", err)
	}

	var merged recording
	shards := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != shardExt {
			continue
		}
		read, readErr := readShard(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return recording{}, readErr
		}
		// A shard holding nothing is a shard whose process died between
		// creating the file and writing its first line. The recorder opens
		// one lazily, inside the write, so a test binary that issues no
		// request leaves no file at all and an empty one cannot mean "this
		// package was silent". The window is microseconds wide and only a
		// SIGKILL lands in it, which is exactly why it must be refused
		// rather than merged: the run would otherwise publish an inventory
		// missing one package's requests, with nothing anywhere saying a
		// package was lost. That is the failure this command exists to make
		// impossible, one shard down instead of all of them.
		if len(read) == 0 {
			return recording{}, fmt.Errorf("shard %s in %s holds no request: a recording was interrupted before it wrote one, so the merge would silently drop whatever that test process saw. Record again with `make record-request-inventory`", entry.Name(), dir)
		}
		shards++
		merged.records = append(merged.records, read...)
		merged.written = newest(merged.written, entry)
	}
	if shards == 0 {
		return recording{}, notRecorded(dir)
	}
	return merged, nil
}

// notRecorded is the refusal when there is nothing to merge, and it names the
// target that would record something.
//
// A directory that is absent and a directory holding no shard are the same
// answer to the reader, and the absent one is the ordinary case: nothing but a
// recorded run creates it, so a fresh checkout running the gate gets this. It
// used to get `open dist/request-inventory: no such file or directory`, which
// is the truth and says nothing about what to do with it.
func notRecorded(dir string) error {
	return fmt.Errorf("no %s shard in %s: nothing has recorded a run here. `make record-request-inventory` records one, and `make gen-request-inventory` records and then rewrites the artifact", shardExt, dir)
}

// newest returns the later of what is known so far and this shard's own write
// time, and leaves the answer alone for a shard that cannot be described.
func newest(known time.Time, entry os.DirEntry) time.Time {
	info, err := shardInfo(entry)
	if err != nil {
		return known
	}
	if written := info.ModTime(); written.After(known) {
		return written
	}
	return known
}

// readShard reads one shard file.
func readShard(path string) ([]shardRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open shard: %w", err)
	}
	defer func() { _ = file.Close() }()

	var records []shardRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxShardLine)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var record shardRecord
		if unmarshalErr := json.Unmarshal([]byte(text), &record); unmarshalErr != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", path, line, unmarshalErr)
		}
		records = append(records, record)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("read %s: %w", path, scanErr)
	}
	return records, nil
}

// merge folds the records into one row per package, method and endpoint.
func merge(records []shardRecord) []row {
	queries := map[rowKey]map[string]struct{}{}
	bodies := map[rowKey]map[string]struct{}{}
	variables := map[rowKey]map[string]struct{}{}
	identifiers := map[rowKey]map[string]int{}
	for _, record := range records {
		key := rowKey{
			pkg:       record.Package,
			kind:      record.Kind,
			method:    record.Method,
			path:      record.Path,
			operation: record.Operation,
		}
		if _, ok := queries[key]; !ok {
			queries[key] = map[string]struct{}{}
			bodies[key] = map[string]struct{}{}
			variables[key] = map[string]struct{}{}
		}
		addAll(queries[key], record.Query)
		addAll(bodies[key], record.Body)
		addAll(variables[key], record.Variables)
		identifiers[key] = mergeIdentifiers(identifiers[key], record.Identifiers)
	}

	rows := make([]row, 0, len(queries))
	for key, query := range queries {
		rows = append(rows, row{
			Package:     key.pkg,
			Kind:        key.kind,
			Method:      key.method,
			Path:        key.path,
			Query:       sortedNames(query),
			Body:        sortedNames(bodies[key]),
			Operation:   key.operation,
			Variables:   sortedNames(variables[key]),
			Identifiers: identifiers[key],
		})
	}
	sort.Slice(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
	return rows
}

// mergeIdentifiers folds one line's running counts into the row's, keeping the
// highest seen for each placeholder.
//
// The highest is the answer rather than the sum, for two different reasons that
// both land here. Within one shard the counts are a running total of the same
// set, so adding them would count every value again on every later line. Across
// shards they are two processes' totals, and a row belongs to one package,
// which the compiler builds into one test binary, so two shards carrying one
// row means the same package ran twice over the same fixtures. Where that is
// not true the highest undercounts, which can only leave a row looking narrower
// than the suite's reach and so can only produce a lead that is not one, never
// hide one.
func mergeIdentifiers(into, counts map[string]int) map[string]int {
	if len(counts) == 0 {
		return into
	}
	if into == nil {
		into = make(map[string]int, len(counts))
	}
	for placeholder, count := range counts {
		into[placeholder] = max(into[placeholder], count)
	}
	return into
}

// addAll adds every name to set.
func addAll(set map[string]struct{}, names []string) {
	for _, name := range names {
		set[name] = struct{}{}
	}
}

// sortedNames renders a name set as a sorted slice, or nil when it is empty so
// the field is omitted rather than written as an empty array.
func sortedNames(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(set))
}

// less orders two rows. Every field of the key takes part, so the order is
// total and two runs over the same tree write the same bytes.
func less(a, b row) bool {
	left := []string{a.Package, a.Path, a.Method, a.Operation, a.Kind}
	right := []string{b.Package, b.Path, b.Method, b.Operation, b.Kind}
	for i := range left {
		if left[i] != right[i] {
			return left[i] < right[i]
		}
	}
	return false
}

// maxReportedDifferences bounds how many changed requests a stale-artifact
// failure lists. A handler that moved changes one or two rows, and a
// regenerated catalog can change hundreds, which nobody reads off a CI log.
const maxReportedDifferences = 10

// differences names the requests the committed artifact and the recording
// disagree about, as lines under the failure that reports the drift.
//
// A committed file that is not this format at all yields nothing to say, and
// the failure stands on its own: the artifact is stale either way, and the
// only difference is whether this can be precise about how.
func differences(committed []byte, rows []row) string {
	var previous requestinventory.Inventory
	if json.Unmarshal(committed, &previous) != nil {
		return ""
	}
	var report strings.Builder
	writeDifferences(&report, "now issued and not in the committed inventory", onlyIn(rows, previous.Requests))
	writeDifferences(&report, "in the committed inventory and no longer issued", onlyIn(previous.Requests, rows))
	return report.String()
}

// onlyIn lists the rows of these that are not in those, as the lines a report
// prints. Both are already sorted, so the result is too.
func onlyIn(these, those []row) []string {
	other := make(map[string]struct{}, len(those))
	for _, r := range those {
		other[rowLine(r)] = struct{}{}
	}
	only := make([]string, 0)
	for _, r := range these {
		if line := rowLine(r); !mapHas(other, line) {
			only = append(only, line)
		}
	}
	return only
}

// mapHas reports membership, named so the loop above reads as prose.
func mapHas(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

// rowLine renders one row as the single line a difference is reported on.
//
// The identifier counts are deliberately left off it. They are a property of
// how far the fixtures reach rather than of the request, so a test that adds a
// second project would otherwise print the row under both headings as though
// the server had started calling something new and stopped calling something
// old. The artifact is still reported stale, since the comparison is on bytes;
// what this list loses is only the ability to say which row moved, in the one
// case where nothing about the request did.
func rowLine(r row) string {
	line := r.Package + " " + r.Method + " " + r.Path
	if r.Operation != "" {
		line += " " + r.Operation
	}
	if len(r.Query) > 0 {
		line += " ?" + strings.Join(r.Query, ",")
	}
	if len(r.Body) > 0 {
		line += " {" + strings.Join(r.Body, ",") + "}"
	}
	if len(r.Variables) > 0 {
		line += " $" + strings.Join(r.Variables, ",")
	}
	return line
}

// writeDifferences appends one titled group of lines, capped, and nothing at
// all when the group is empty.
func writeDifferences(report *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(report, "\n  %s (%d):", title, len(lines))
	for _, line := range lines[:min(len(lines), maxReportedDifferences)] {
		fmt.Fprintf(report, "\n    %s", line)
	}
	if len(lines) > maxReportedDifferences {
		fmt.Fprintf(report, "\n    and %d more", len(lines)-maxReportedDifferences)
	}
}

// render marshals the inventory, with the trailing newline a text file in this
// repository ends with.
func render(rows []row) []byte {
	return requestinventory.Render(rows)
}
