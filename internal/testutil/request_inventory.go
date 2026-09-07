package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

// InventoryDirEnv names the directory every request a test issues through
// [NewTestClient] is recorded into. Recording is off unless it is set, so an
// ordinary `go test` run pays nothing for it.
//
// The path must be absolute. A test binary runs with its own package
// directory as the working directory, so a relative path would scatter one
// shard per package under 178 different directories and the merge would find
// none of them. A relative value is refused rather than resolved, because
// resolving it would produce exactly that scattering silently.
//
// It carries the project's prefix but is not one of the settings
// internal/config resolves: it configures the test harness, cmd/server never
// reads it, and there is no legacy spelling of it to warn anybody about. That
// is the same standing the developer-only EVAL_SURFACE_* variables have.
const InventoryDirEnv = "GITLAB_MCP_TEST_INVENTORY_DIR"

const (
	// kindREST and kindGraphQL are the two shapes of request this server
	// makes, and they are recorded differently: a REST call is a method, a
	// path and query keys, a GraphQL call is one path and a document.
	kindREST    = "rest"
	kindGraphQL = "graphql"

	// modulePath is this module, trimmed off a recorded package so a row says
	// internal/tools/issues rather than repeating the module in every line.
	modulePath = "github.com/jmrplens/gitlab-mcp-server/v2"

	// testPackageSuffix is what the compiler appends to the import path of an
	// external test package. The requests a `package issues_test` file issues
	// belong to the issues package as plainly as the ones its internal tests
	// issue.
	testPackageSuffix = "_test"

	// unknownPackage is recorded when the walk runs out of frames without
	// leaving this package or reaching the harness, which a stack deeper than
	// maxStackDepth could do. It is written down rather than guessed at.
	unknownPackage = "unknown"

	// maxStackDepth bounds the walk. Four frames separate a test from
	// [NewTestClient] today and the margin is for a test helper or two in
	// between, not for an arbitrarily deep stack.
	maxStackDepth = 32

	// shardPattern names a shard. One process writes one shard, so package
	// binaries running in parallel never write to the same file and no
	// cross-process locking is needed.
	shardPattern = "requests-*.jsonl"

	// shardDirPerm is the mode the shard directory is created with. It holds
	// nothing secret, but nothing needs to read it either except the merge
	// that runs as the same user.
	shardDirPerm = 0o750
)

// requestRecord is one request a test issued, as one line of the shard file.
//
// The field names are the contract between the recorder and
// cmd/gen_request_inventory, which declares the same shape to read these lines
// back. It is not shared as a Go type on purpose: this package embeds the
// pinned GraphQL schema and net/http/httptest, and a command that imported it
// would carry both into a binary that has no use for either.
type requestRecord struct {
	Package string `json:"package"`
	Test    string `json:"test"`
	Kind    string `json:"kind"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	// Query holds the parameter names of a REST call, sorted and deduplicated.
	// The values are left out because a value is a fixture and a name is part
	// of the request we make.
	Query []string `json:"query,omitempty"`
	// Operation and Variables describe a GraphQL call the way Path and Query
	// describe a REST one. See [graphQLShape].
	Operation string   `json:"operation,omitempty"`
	Variables []string `json:"variables,omitempty"`
}

// requestOrigin is the attribution the transport can honestly make: the Go
// package that built the client, and the test that built it.
//
// It is not the action. Nothing on the wire names one, and nothing on the
// stack at the point a request is observed names one either, because the
// httptest server answers on its own goroutine while the test goroutine that
// called the handler is blocked somewhere this code cannot see. The catalog's
// route for an action is a closure over the handler function
// (toolutil.RouteAction), so even the catalog cannot hand back a function
// identity to match against.
//
// What that misses is worth stating plainly. A package owning thirty actions
// produces one bucket of rows, so the inventory answers "the issues package
// issues these requests" and not "issue.list issues this one". A test name is
// a strong hint at the action and no more, which is why it is recorded as
// provenance and never parsed into one: an attribution that is a guess would
// be worse than the honest coarse one, since the whole point of this dimension
// is to be believed when it says an action's request was never seen.
type requestOrigin struct {
	pkg  string
	test string
}

// requestReporter is the part of [testing.TB] the recorder uses. It is an
// interface so the reporting paths are reachable from a test with a recorder
// standing in, rather than only by failing the test that exercises them.
type requestReporter interface {
	Errorf(format string, args ...any)
}

// recorders holds one recorder per directory, so the thousands of clients a
// suite builds share one shard file and one seen set. Keying on the directory
// rather than caching the first lookup keeps [testing.T.Setenv] meaningful in
// this package's own tests.
var (
	recordersMu sync.Mutex
	recorders   = map[string]*recorder{}
)

// recorder writes one shard of the request inventory.
type recorder struct {
	dir string

	mu   sync.Mutex
	file *os.File
	// seen holds the lines already written, so a path exercised by two
	// hundred assertions costs two hundred map lookups and one write.
	seen map[string]bool
	// stopped is set once something has gone wrong and been reported, so a
	// broken directory produces one failure and not one per request.
	stopped bool
}

// inventoryRecorder returns the recorder for the configured directory, or nil
// when recording is off.
func inventoryRecorder() *recorder {
	dir := strings.TrimSpace(os.Getenv(InventoryDirEnv))
	if dir == "" {
		return nil
	}
	recordersMu.Lock()
	defer recordersMu.Unlock()
	if existing, ok := recorders[dir]; ok {
		return existing
	}
	created := &recorder{dir: dir, seen: map[string]bool{}}
	recorders[dir] = created
	return created
}

// recordingHandler wraps next so every request reaching the mock is recorded,
// and returns next unchanged when recording is off.
//
// The origin is resolved here rather than per request because this runs on the
// test's own goroutine, inside [NewTestClient], which is the last moment the
// calling package is on the stack.
func recordingHandler(tb testing.TB, next http.Handler) http.Handler {
	tb.Helper()
	rec := inventoryRecorder()
	if rec == nil {
		return next
	}
	origin := requestOrigin{pkg: callerPackage(), test: tb.Name()}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.observe(tb, origin, r)
		next.ServeHTTP(w, r)
	})
}

// observe records one request. It runs on the httptest server's goroutine, so
// it reports with Errorf and never aborts
// (.github/instructions/test-goroutines.instructions.md).
func (rec *recorder) observe(reporter requestReporter, origin requestOrigin, r *http.Request) {
	record, ok := describeRequest(origin, r)
	if !ok {
		return
	}
	// A struct of strings and string slices always marshals, so there is no
	// error here to report and no branch worth carrying for one.
	line, _ := json.Marshal(record) //nolint:errchkjson // see above
	rec.writeLine(reporter, string(line))
}

// describeRequest turns one request into the row it belongs in, reporting
// false for a request that has no shape worth recording.
func describeRequest(origin requestOrigin, r *http.Request) (requestRecord, bool) {
	record := requestRecord{
		Package: origin.pkg,
		Test:    origin.test,
		Kind:    kindREST,
		Method:  r.Method,
		Path:    templatePath(r.URL.EscapedPath()),
	}

	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, graphQLPath) {
		record.Query = queryKeys(r)
		return record, true
	}

	document, ok := graphQLDocument(r)
	if !ok {
		return requestRecord{}, false
	}
	shape, ok := summarizeGraphQL(document)
	if !ok {
		return requestRecord{}, false
	}
	record.Kind = kindGraphQL
	record.Operation = shape.Operation
	record.Variables = shape.Variables
	return record, true
}

// queryKeys lists the query parameter names of a request, sorted and
// deduplicated.
func queryKeys(r *http.Request) []string {
	values := r.URL.Query()
	if len(values) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(values))
}

// graphQLDocument reads the document out of a request body and puts the body
// back for the mock to read.
//
// It reports false for a body it cannot read or that is not the JSON envelope,
// which covers the multipart form a GraphQL upload arrives as. Neither is
// reported as a failure here: the schema gate one hop away already reports an
// unreadable body, and reporting it twice would name the same defect twice.
func graphQLDocument(r *http.Request) (string, bool) {
	body, err := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return "", false
	}
	var request graphqlRequest
	if json.Unmarshal(body, &request) != nil || strings.TrimSpace(request.Query) == "" {
		return "", false
	}
	return request.Query, true
}

// writeLine appends one line to the shard, skipping a line already written.
func (rec *recorder) writeLine(reporter requestReporter, line string) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.stopped || rec.seen[line] {
		return
	}
	if rec.file == nil {
		if !filepath.IsAbs(rec.dir) {
			rec.stopf(reporter, "%s must be an absolute path, got %q: a test binary runs in its own package directory, so a relative one writes a shard nothing will find", InventoryDirEnv, rec.dir)
			return
		}
		file, err := createShard(rec.dir)
		if err != nil {
			rec.stopf(reporter, "could not open a request inventory shard in %s: %v", rec.dir, err)
			return
		}
		rec.file = file
	}
	if _, err := rec.file.WriteString(line + "\n"); err != nil {
		rec.stopf(reporter, "could not write a request inventory shard: %v", err)
		return
	}
	rec.seen[line] = true
}

// stopf reports one failure and silences the recorder for the rest of the run.
// The caller holds rec.mu.
func (rec *recorder) stopf(reporter requestReporter, format string, args ...any) {
	rec.stopped = true
	reporter.Errorf(format, args...)
}

// createShard opens this process's shard file. It is a variable so the
// failure branch is reachable from a test: the suite runs as root often
// enough that a directory made read-only is not read-only.
var createShard = func(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, shardDirPerm); err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, shardPattern)
}

// callerPackage names the package that called into this one, repository
// relative.
//
// Reading the package off the runtime rather than off a file path keeps it
// correct whether or not the build trimmed paths, and reading it here rather
// than at the request keeps it correct at all: a request is observed on the
// httptest server's goroutine, whose stack knows nothing about the test that
// caused it.
func callerPackage() string {
	counters := make([]uintptr, maxStackDepth)
	depth := runtime.Callers(1, counters)
	frames := runtime.CallersFrames(counters[:depth])
	return packageOutside(func() (string, bool) {
		frame, more := frames.Next()
		return frame.Function, more
	})
}

// packageOutside walks function names outward from this package's own frame
// and names the package that called in.
//
// The walk takes its own package from the first frame rather than writing it
// down, so moving this file needs no edit here. Reaching the test harness
// without having left this package means this package called itself, which is
// what its own tests do, and the answer is this package.
func packageOutside(next func() (string, bool)) string {
	self := ""
	for {
		function, more := next()
		pkg := packageOf(function)
		switch {
		case pkg == "":
		case self == "":
			self = pkg
		case stackBoundaries[pkg]:
			return shortPackage(self)
		case pkg != self:
			return shortPackage(pkg)
		}
		if !more {
			return unknownPackage
		}
	}
}

// stackBoundaries are the packages that end the walk because no caller of
// [NewTestClient] can be one of them: testing runs the test and runtime
// finishes the goroutine.
var stackBoundaries = map[string]bool{"testing": true, "runtime": true}

// packageOf splits a runtime function name into the package that declares it.
// A function name is <import path>.<function>, and the import path is the only
// part that may contain a slash, so the first dot after the last slash is the
// separator.
func packageOf(function string) string {
	slash := strings.LastIndex(function, "/")
	dot := strings.Index(function[slash+1:], ".")
	if dot < 0 {
		return ""
	}
	return function[:slash+1+dot]
}

// shortPackage renders an import path the way a row prints it: repository
// relative, with an external test package folded into the package it tests.
func shortPackage(path string) string {
	return strings.TrimSuffix(strings.TrimPrefix(path, modulePath+"/"), testPackageSuffix)
}
