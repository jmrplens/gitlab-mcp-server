package testutil

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// newRecorderIn builds a recorder writing into a fresh directory, which is the
// shape every recorder test needs and none of them should repeat.
func newRecorderIn(t *testing.T) *recorder {
	t.Helper()
	rec := &recorder{dir: t.TempDir(), seen: map[string]bool{}}
	// The shard has to be closed before the directory is removed. On Linux it
	// makes no difference; on Windows a directory holding an open file cannot
	// be removed, and t.TempDir reports that as a failure of the test whose
	// assertions have all already passed.
	t.Cleanup(rec.release)
	return rec
}

// shardLines reads back every line a recorder wrote.
func shardLines(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", dir, err)
	}
	var lines []string
	for _, entry := range entries {
		content, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", entry.Name(), readErr)
		}
		for line := range strings.SplitSeq(strings.TrimSpace(string(content)), "\n") {
			if line != "" {
				lines = append(lines, line)
			}
		}
	}
	slices.Sort(lines)
	return lines
}

// restRequest builds a GET the recorder can describe.
func restRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://gitlab.example.com"+target, nil)
}

// TestDescribeRequest_REST_RecordsMethodPathAndQueryNames verifies the row a
// REST call produces: the endpoint it reached and the parameter names it sent,
// never the values, because a value is a fixture and a name is part of the
// request this server makes.
func TestDescribeRequest_REST_RecordsMethodPathAndQueryNames(t *testing.T) {
	request := restRequest(t, "/api/v4/projects/42/issues?state=opened&per_page=20&state=closed")

	record, ok := describeRequest(requestOrigin{pkg: "internal/tools/issues", test: "TestIssueList"}, request)

	if !ok {
		t.Fatal("describeRequest reported no record for a REST call")
	}
	if record.Kind != kindREST || record.Method != http.MethodGet || record.Path != "/projects/:project_id/issues" {
		t.Errorf("record = %+v, want a rest GET of /projects/:project_id/issues", record)
	}
	if !slices.Equal(record.Query, []string{"per_page", "state"}) {
		t.Errorf("query = %v, want [per_page state]", record.Query)
	}
	if record.Package != "internal/tools/issues" || record.Test != "TestIssueList" {
		t.Errorf("attribution = %s/%s, want internal/tools/issues/TestIssueList", record.Package, record.Test)
	}
}

// TestDescribeRequest_NoQuery_OmitsTheField verifies that a call with no query
// string records none, so the artifact carries no empty arrays.
func TestDescribeRequest_NoQuery_OmitsTheField(t *testing.T) {
	record, ok := describeRequest(requestOrigin{}, restRequest(t, "/api/v4/projects/42"))

	if !ok {
		t.Fatal("describeRequest reported no record")
	}
	if record.Query != nil {
		t.Errorf("query = %v, want nil", record.Query)
	}
}

// jsonRequest builds a request with a JSON body of the given content type,
// which is what decides whether the recorder reads it at all.
func jsonRequest(t *testing.T, method, contentType, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method, "http://gitlab.example.com/api/v4/projects/1/issues", strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	return request
}

// TestDescribeRequest_JSONBody_RecordsItsFieldNames verifies the half of a
// mutating request the path cannot carry. Without it a create or an update is
// a method and an endpoint and nothing else, which is most of what this server
// sends and where a wrong field name would live.
//
// The values are absent for the reason the query values are: a value is a
// fixture, a name is part of the request. A body this cannot read field names
// out of is recorded as a row with no body rather than skipped, because the
// endpoint was still reached.
func TestDescribeRequest_JSONBody_RecordsItsFieldNames(t *testing.T) {
	tests := []struct {
		name    string
		request func() *http.Request
		want    []string
	}{
		{"a JSON object", func() *http.Request {
			return jsonRequest(t, http.MethodPost, "application/json", `{"title":"t","labels":"a,b","confidential":true}`)
		}, []string{"confidential", "labels", "title"}},
		{"a content type with a charset after it", func() *http.Request {
			return jsonRequest(t, http.MethodPut, "application/json; charset=utf-8", `{"title":"t"}`)
		}, []string{"title"}},
		{"a multipart upload, which is never read", func() *http.Request {
			return jsonRequest(t, http.MethodPost, "multipart/form-data; boundary=x", `{"title":"t"}`)
		}, nil},
		{"a request declaring no content type", func() *http.Request {
			return jsonRequest(t, http.MethodPost, "", `{"title":"t"}`)
		}, nil},
		{"a JSON array, which has no field names", func() *http.Request {
			return jsonRequest(t, http.MethodPost, "application/json", `[{"title":"t"}]`)
		}, nil},
		{"an empty JSON object", func() *http.Request {
			return jsonRequest(t, http.MethodPost, "application/json", `{}`)
		}, nil},
		{"a body that cannot be read", func() *http.Request {
			request := jsonRequest(t, http.MethodPost, "application/json", `{"title":"t"}`)
			request.Body = failingBody{}
			return request
		}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := tt.request()

			record, ok := describeRequest(requestOrigin{}, request)

			if !ok {
				t.Fatal("describeRequest reported no record for a REST call")
			}
			if !slices.Equal(record.Body, tt.want) {
				t.Errorf("body = %v, want %v", record.Body, tt.want)
			}
		})
	}
}

// TestDescribeRequest_JSONBody_StaysReadable verifies that reading the field
// names leaves the body where the handler under test can still read it, which
// every mock that decodes what it was sent depends on.
func TestDescribeRequest_JSONBody_StaysReadable(t *testing.T) {
	request := jsonRequest(t, http.MethodPost, "application/json", `{"title":"t"}`)

	if _, ok := describeRequest(requestOrigin{}, request); !ok {
		t.Fatal("describeRequest reported no record")
	}

	var round map[string]any
	if err := json.NewDecoder(request.Body).Decode(&round); err != nil {
		t.Fatalf("the body was consumed: %v", err)
	}
	if round["title"] != "t" {
		t.Errorf("body round-tripped as %v, want the title it was sent", round)
	}
}

// TestDescribeRequest_GraphQL_RecordsOperationAndVariables verifies that a
// GraphQL call is recorded as its operation and declared variables rather than
// as one more POST to one path, which is all the path itself would say.
func TestDescribeRequest_GraphQL_RecordsOperationAndVariables(t *testing.T) {
	request := graphQLRequest(t, `{"query":"query($fullPath: ID!) { project(fullPath: $fullPath) { id } }","variables":{"fullPath":"g/p"}}`)

	record, ok := describeRequest(requestOrigin{pkg: "internal/tools/projects"}, request)

	if !ok {
		t.Fatal("describeRequest reported no record for a GraphQL call")
	}
	if record.Kind != kindGraphQL || record.Path != "/graphql" {
		t.Errorf("record = %+v, want a graphql POST of /graphql", record)
	}
	if record.Operation != "query project" || !slices.Equal(record.Variables, []string{"fullPath"}) {
		t.Errorf("operation = %q, variables = %v, want %q and [fullPath]", record.Operation, record.Variables, "query project")
	}
}

// TestDescribeRequest_GraphQL_BodyStaysReadable verifies the half a mock
// depends on: reading the document must leave the body where the handler under
// test can still read it.
func TestDescribeRequest_GraphQL_BodyStaysReadable(t *testing.T) {
	const body = `{"query":"{ project { id } }"}`
	request := graphQLRequest(t, body)

	if _, ok := describeRequest(requestOrigin{}, request); !ok {
		t.Fatal("describeRequest reported no record")
	}

	var round graphqlRequest
	if err := json.NewDecoder(request.Body).Decode(&round); err != nil {
		t.Fatalf("the body was consumed: %v", err)
	}
	if round.Query == "" {
		t.Error("the body no longer carries the document")
	}
}

// TestDescribeRequest_UnrecordableGraphQL_IsSkipped verifies that a request
// this cannot describe produces no row at all.
//
// None of these is reported as a failure here. An unreadable body and a
// refused document are the GraphQL gate's business one hop away, and reporting
// either twice would name one defect twice; a multipart upload is a request
// shape this describes nothing about.
func TestDescribeRequest_UnrecordableGraphQL_IsSkipped(t *testing.T) {
	tests := []struct {
		name    string
		request func() *http.Request
	}{
		{"a body that cannot be read", func() *http.Request {
			request := graphQLRequest(t, "{}")
			request.Body = failingBody{}
			return request
		}},
		{"a body that is not the JSON envelope", func() *http.Request {
			return graphQLRequest(t, "--boundary\r\nContent-Disposition: form-data\r\n")
		}},
		{"an envelope carrying no document", func() *http.Request {
			return graphQLRequest(t, `{"variables":{"id":1}}`)
		}},
		{"a document that does not parse", func() *http.Request {
			return graphQLRequest(t, `{"query":"query { unclosed"}`)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if record, ok := describeRequest(requestOrigin{}, tt.request()); ok {
				t.Errorf("describeRequest recorded %+v, want nothing", record)
			}
		})
	}
}

// TestReleaseRecorders_AnOpenShard_IsClosedAndForgotten verifies the release
// the tests depend on, and that a real run does not need.
//
// A shard belongs to one test process and the operating system closes it at
// exit, so nothing in production calls this. Windows is why it exists: a
// directory holding an open file cannot be removed there, so a test recording
// into t.TempDir() fails in cleanup after every one of its own assertions has
// passed, which is a failure nobody reading the test can explain. Two CI runs
// went red for exactly that.
func TestReleaseRecorders_AnOpenShard_IsClosedAndForgotten(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(InventoryDirEnv, dir)
	t.Cleanup(releaseRecorders)

	rec := inventoryRecorder()
	if rec == nil {
		t.Fatal("inventoryRecorder() = nil with a directory set")
	}
	rec.observe(&recordingReporter{name: t.Name()}, requestOrigin{pkg: "internal/tools/issues", test: t.Name()},
		restRequest(t, "/api/v4/projects/1/issues"))
	if rec.file == nil {
		t.Fatal("the recorder wrote a line without opening a shard")
	}

	releaseRecorders()

	if rec.file != nil {
		t.Error("releaseRecorders() left the shard open, so the directory holding it cannot be removed on Windows")
	}
	if inventoryRecorder() == rec {
		t.Error("releaseRecorders() left the released recorder in the registry, so a later run would write through a closed file")
	}
}

// TestRecorderObserve_RepeatedRequest_IsWrittenOnce verifies the dedup that
// makes recording affordable: a path two hundred assertions reach is one line.
func TestRecorderObserve_RepeatedRequest_IsWrittenOnce(t *testing.T) {
	rec := newRecorderIn(t)
	reporter := &recordingReporter{name: t.Name()}
	origin := requestOrigin{pkg: "internal/tools/issues", test: t.Name()}

	rec.observe(reporter, origin, restRequest(t, "/api/v4/projects/1/issues"))
	rec.observe(reporter, origin, restRequest(t, "/api/v4/projects/2/issues"))
	rec.observe(reporter, origin, restRequest(t, "/api/v4/projects/3/issues?state=opened"))

	lines := shardLines(t, rec.dir)
	if len(lines) != 2 {
		t.Fatalf("wrote %d line(s), want 2:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if reporter.joined() != "" {
		t.Errorf("reported %q, want nothing", reporter.joined())
	}
}

// TestRecorderObserve_UnrecordableRequest_WritesNothing verifies that a
// request with no row to write does not open a shard for it.
func TestRecorderObserve_UnrecordableRequest_WritesNothing(t *testing.T) {
	rec := newRecorderIn(t)

	rec.observe(&recordingReporter{name: t.Name()}, requestOrigin{}, graphQLRequest(t, `{"query":"query { unclosed"}`))

	if lines := shardLines(t, rec.dir); len(lines) != 0 {
		t.Errorf("wrote %v, want nothing", lines)
	}
}

// TestRecorderWriteLine_RelativeDirectory_IsRefused verifies the one
// configuration mistake that would look like success: a test binary runs in
// its own package directory, so a relative directory writes 178 shards nobody
// will find. It is refused rather than resolved, and refused once.
func TestRecorderWriteLine_RelativeDirectory_IsRefused(t *testing.T) {
	rec := &recorder{dir: filepath.Join("dist", "request-inventory"), seen: map[string]bool{}}
	reporter := &recordingReporter{name: t.Name()}

	rec.writeLine(reporter, `{"path":"/projects"}`)
	rec.writeLine(reporter, `{"path":"/groups"}`)

	if len(reporter.messages) != 1 {
		t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	if !strings.Contains(reporter.joined(), "must be an absolute path") {
		t.Errorf("report = %q, want it to name the absolute path requirement", reporter.joined())
	}
}

// TestRecorderWriteLine_UnopenableShard_IsReportedOnce verifies that a
// directory the recorder cannot write into fails loudly and exactly once.
//
// The constructor is stubbed rather than the filesystem made hostile because
// this suite is often run as root, where a directory stripped of write
// permission is still writable.
func TestRecorderWriteLine_UnopenableShard_IsReportedOnce(t *testing.T) {
	original := createShard
	createShard = func(string) (*os.File, error) { return nil, errors.New("no room") }
	t.Cleanup(func() { createShard = original })

	rec := newRecorderIn(t)
	reporter := &recordingReporter{name: t.Name()}

	rec.writeLine(reporter, `{"path":"/projects"}`)
	rec.writeLine(reporter, `{"path":"/groups"}`)

	if len(reporter.messages) != 1 {
		t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	if !strings.Contains(reporter.joined(), "no room") {
		t.Errorf("report = %q, want it to carry the cause", reporter.joined())
	}
}

// TestCreateShard_UncreatableDirectory_Fails verifies that the real
// constructor reports a directory it cannot make, which is the failure the
// stub above stands in for.
func TestCreateShard_UncreatableDirectory_Fails(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	if file, err := createShard(filepath.Join(blocked, "shards")); err == nil {
		_ = file.Close()
		t.Error("createShard succeeded under a regular file, want an error")
	}
}

// TestRecorderWriteLine_UnwritableShard_IsReported verifies that a shard that
// stops accepting writes is reported rather than silently dropping rows.
func TestRecorderWriteLine_UnwritableShard_IsReported(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "shard-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp error = %v", err)
	}
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("Close error = %v", closeErr)
	}

	rec := &recorder{dir: t.TempDir(), seen: map[string]bool{}, file: file}
	reporter := &recordingReporter{name: t.Name()}

	rec.writeLine(reporter, `{"path":"/projects"}`)

	if !strings.Contains(reporter.joined(), "could not write") {
		t.Errorf("report = %q, want a write failure", reporter.joined())
	}
}

// TestInventoryRecorder_Environment_DecidesWhetherRecordingHappens verifies
// that recording is off by default and that one directory has one recorder, so
// the thousands of clients a suite builds share one shard and one dedup set.
func TestInventoryRecorder_Environment_DecidesWhetherRecordingHappens(t *testing.T) {
	t.Setenv(InventoryDirEnv, "")
	if rec := inventoryRecorder(); rec != nil {
		t.Fatalf("inventoryRecorder() = %+v with no directory set, want nil", rec)
	}

	dir := t.TempDir()
	t.Setenv(InventoryDirEnv, dir)
	t.Cleanup(releaseRecorders)
	first := inventoryRecorder()
	if first == nil {
		t.Fatal("inventoryRecorder() = nil with a directory set")
	}
	if inventoryRecorder() != first {
		t.Error("inventoryRecorder() built a second recorder for the same directory")
	}
}

// inertHandler answers nothing and is comparable, which [http.HandlerFunc] is
// not: comparing two func values panics, and the assertion below is that one
// particular handler came back.
type inertHandler struct{}

// ServeHTTP does nothing, since no request reaches this handler.
func (inertHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

// TestRecordingHandler_RecordingOff_ReturnsTheHandlerUnchanged verifies that a
// suite running without recording pays nothing for it, not even a wrapper.
func TestRecordingHandler_RecordingOff_ReturnsTheHandlerUnchanged(t *testing.T) {
	t.Setenv(InventoryDirEnv, "")
	var handler http.Handler = inertHandler{}

	if recordingHandler(t, handler) != handler {
		t.Error("recordingHandler wrapped the handler with recording off")
	}
}

// TestNewTestClient_Recording_WritesTheRequestTheClientMade is the end to end
// of part one: a client built the way every domain test builds one, a call
// made the way a handler makes it, and a row naming the endpoint that call
// reached.
//
// The project is addressed by path, so the row also proves the encoded
// identifier collapses: without that, one endpoint reached by id and by path
// would be two rows and the inventory would count fixtures.
//
// A client built here would ordinarily record nothing, since this package's
// own fixtures are not requests this server sends GitLab, so the test moves
// that exemption aside to drive the wiring. This is the only place the whole
// chain from [NewTestClient] to a shard line is exercised, which is what makes
// it worth the seam: nothing else would notice the wrapper being dropped.
func TestNewTestClient_Recording_WritesTheRequestTheClientMade(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(InventoryDirEnv, dir)
	// The recorder this creates lives in the package-wide registry, so the test
	// cannot close its shard directly; releasing the registry is what lets the
	// temporary directory be removed on Windows.
	t.Cleanup(releaseRecorders)
	restoreRecorderPackage(t, "internal/testutil/not-this-package")

	client := NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		RespondJSON(w, http.StatusOK, `{"id":1,"path_with_namespace":"group/project"}`)
	}))
	if _, _, err := client.GL().Projects.GetProject("group/project", nil); err != nil {
		t.Fatalf("GetProject error = %v", err)
	}

	lines := shardLines(t, dir)
	if len(lines) != 1 {
		t.Fatalf("wrote %d line(s), want 1:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	var record requestRecord
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", lines[0], err)
	}
	if record.Path != "/projects/:project_id" || record.Method != http.MethodGet {
		t.Errorf("record = %+v, want a GET of /projects/:project_id", record)
	}
	if record.Package != "internal/testutil" || record.Test != t.Name() {
		t.Errorf("attribution = %s/%s, want internal/testutil/%s", record.Package, record.Test, t.Name())
	}
}

// restoreRecorderPackage points the self-exemption at another name for one
// test and puts the real one back afterwards.
func restoreRecorderPackage(t *testing.T, name string) {
	t.Helper()
	previous := recorderPackage
	recorderPackage = name
	t.Cleanup(func() { recorderPackage = previous })
}

// TestRecordingHandler_ThisPackagesOwnClient_RecordsNothing verifies the
// exemption itself: this package's fixtures exercise its own mock, and a
// request nothing in internal/tools issued has no place in an inventory read
// as what this server sends GitLab.
func TestRecordingHandler_ThisPackagesOwnClient_RecordsNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(InventoryDirEnv, dir)
	t.Cleanup(releaseRecorders)

	client := NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		RespondJSON(w, http.StatusOK, `{"id":1}`)
	}))
	if _, _, err := client.GL().Projects.GetProject(1, nil); err != nil {
		t.Fatalf("GetProject error = %v", err)
	}

	if lines := shardLines(t, dir); len(lines) != 0 {
		t.Errorf("wrote %d line(s), want none:\n%s", len(lines), strings.Join(lines, "\n"))
	}
}

// TestCallerPackage_FromThisTest_NamesThisPackage verifies the attribution the
// whole inventory rests on, including that an external test package is folded
// into the package it tests.
func TestCallerPackage_FromThisTest_NamesThisPackage(t *testing.T) {
	if got := callerPackage(); got != "internal/testutil" {
		t.Errorf("callerPackage() = %q, want internal/testutil", got)
	}
}

// TestPackageOutside_Frames_NameTheCallingPackage verifies the walk on the
// stack shapes it has to survive: a domain test calling in, this package's own
// test calling in, a frame the runtime gives no name to, and a stack so deep
// the walk runs out before it leaves.
func TestPackageOutside_Frames_NameTheCallingPackage(t *testing.T) {
	const self = modulePath + "/internal/testutil"
	tests := []struct {
		name      string
		functions []string
		want      string
	}{
		{
			name:      "a domain test",
			functions: []string{self + ".callerPackage", self + ".NewTestClient", modulePath + "/internal/tools/issues.TestList", "testing.tRunner"},
			want:      "internal/tools/issues",
		},
		{
			name:      "this package's own test, which reaches the harness first",
			functions: []string{self + ".callerPackage", self + ".NewTestClient", self + ".TestSomething", "testing.tRunner"},
			want:      "internal/testutil",
		},
		{
			name:      "a frame the runtime names nothing",
			functions: []string{self + ".callerPackage", "", modulePath + "/internal/tools/tags.TestList"},
			want:      "internal/tools/tags",
		},
		{
			name:      "a stack that runs out without leaving",
			functions: []string{self + ".callerPackage", self + ".NewTestClient"},
			want:      unknownPackage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := 0
			next := func() (string, bool) {
				function := tt.functions[at]
				at++
				return function, at < len(tt.functions)
			}
			if got := packageOutside(next); got != tt.want {
				t.Errorf("packageOutside(%v) = %q, want %q", tt.functions, got, tt.want)
			}
		})
	}
}

// TestPackageOf_FunctionName_SplitsAtTheFirstDotAfterTheLastSlash verifies the
// one parsing rule the attribution needs, on the shapes the runtime produces.
func TestPackageOf_FunctionName_SplitsAtTheFirstDotAfterTheLastSlash(t *testing.T) {
	tests := []struct {
		name     string
		function string
		want     string
	}{
		{"a function", modulePath + "/internal/tools/issues.List", modulePath + "/internal/tools/issues"},
		{"a method", modulePath + "/internal/tools/issues.(*Client).List", modulePath + "/internal/tools/issues"},
		{"a closure inside a test", modulePath + "/internal/tools/issues_test.TestList.func1", modulePath + "/internal/tools/issues_test"},
		{"a package with no path", "main.main", "main"},
		{"a frame with no function name at all", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := packageOf(tt.function); got != tt.want {
				t.Errorf("packageOf(%q) = %q, want %q", tt.function, got, tt.want)
			}
		})
	}
}

// TestShortPackage_ImportPath_IsRepositoryRelative verifies how a row names a
// package: relative to the repository, with an external test package folded
// into the one it tests.
func TestShortPackage_ImportPath_IsRepositoryRelative(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"a package of this module", modulePath + "/internal/tools/issues", "internal/tools/issues"},
		{"an external test package", modulePath + "/internal/tools/issues_test", "internal/tools/issues"},
		{"a package of another module", "gitlab.com/gitlab-org/api/client-go/v3", "gitlab.com/gitlab-org/api/client-go/v3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortPackage(tt.path); got != tt.want {
				t.Errorf("shortPackage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
