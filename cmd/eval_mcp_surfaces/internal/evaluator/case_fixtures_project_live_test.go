// case_fixtures_project_live_test.go covers the live GitLab fixture that
// provisions an attempt-scoped source branch and seed file for merge-request
// evaluation cases. The fixture is exercised against an [httptest] server
// so the test does not require a real GitLab instance.

package evaluator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	// mrSourceFixtureTestBranch is the deterministic branch name produced by
	// the merge-request source fixture when seeded with the matching run
	// suffix and model label.
	mrSourceFixtureTestBranch = "feature/eval-gpt54mini-r1-abc123"
	// mrSourceFixtureTestFilePath is the percent-encoded repository file path
	// the fixture is expected to query when seeding the evaluation file.
	mrSourceFixtureTestFilePath = "/api/v4/projects/101/repository/files/tmp%2Feval-mr-feature-eval-gpt54mini-r1-abc123.txt"
)

// TestMergeRequestSourceFixture_EnsuresAttemptBranchAndFile verifies that
// MergeRequestSourceFixture creates the expected branch and seed file against
// the live GitLab API and exposes attempt-scoped output values for the case
// prompt template.
//
// The test boots an httptest server that records every method+path it sees
// and dispatches to handlers that simulate the group/project/branch/file
// endpoints. It then calls the fixture's Ensure hook with a deterministic
// idempotency key, asserting that the returned outputs match the expected
// branch name and MR title, and that the recorded calls include the branch
// POST and the repository file POST. This protects the live fixture from
// regressions that would lose attempt scoping or skip file seeding.
func TestMergeRequestSourceFixture_EnsuresAttemptBranchAndFile(t *testing.T) {
	var calls []string
	server := newMergeRequestSourceFixtureServer(t, &calls)
	defer server.Close()

	output, err := MergeRequestSourceFixture.Ensure(t.Context(), FixtureContext{
		Client:         newFixtureTestClient(t, server.URL),
		ModelName:      "openai:gpt-5.4-mini",
		RunIndex:       1,
		RunSuffix:      "abc123",
		IdempotencyKey: "test:merge-request-source",
	})
	if err != nil {
		t.Fatalf("MergeRequestSourceFixture.Ensure() error = %v\ncalls=%s", err, strings.Join(calls, ","))
	}
	if output["mr_source_branch"] != mrSourceFixtureTestBranch || output["mr_title"] != "Evaluation MR gpt54mini-r1-abc123" {
		t.Fatalf("output = %+v, want suffixed MR source values", output)
	}
	for _, want := range []string{"POST /api/v4/projects/101/repository/branches", "POST " + mrSourceFixtureTestFilePath} {
		if !strings.Contains(strings.Join(calls, ","), want) {
			t.Fatalf("calls = %s, want %s", strings.Join(calls, ","), want)
		}
	}
}

// newMergeRequestSourceFixtureServer starts an httptest server that records
// every request and dispatches the merge-request source fixture endpoints.
// Calls [testing.T.Helper] so failures are attributed to the test caller.
func newMergeRequestSourceFixtureServer(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.Method+" "+r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		handleMergeRequestSourceFixtureRequest(t, w, r)
	}))
}

// handleMergeRequestSourceFixtureRequest routes httptest requests to the
// merge-request source fixture handler that matches the supplied method and
// URL. Unrecognized routes return 404 so unexpected traffic is surfaced in
// test output rather than silently passing.
func handleMergeRequestSourceFixtureRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	switch {
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/groups/my-org":
		writeJSON(t, w, map[string]any{"id": 11, "full_path": liveFixtureGroupPath})
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/groups/my-org%2Ftools":
		writeJSON(t, w, map[string]any{"id": 12, "full_path": liveFixtureToolsPath})
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server":
		writeJSON(t, w, map[string]any{"id": 101, "path_with_namespace": liveFixtureProjectPath, "default_branch": liveFixtureDefaultRef})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/101/repository/branches/"):
		writeNotFound(w, `{"message":"404 Branch Not Found"}`)
	case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/101/repository/branches":
		handleMergeRequestSourceBranchCreate(t, w, r)
	case r.Method == http.MethodGet && r.URL.EscapedPath() == mrSourceFixtureTestFilePath:
		assertMergeRequestSourceFileRef(t, r)
		writeNotFound(w, `{"message":"404 File Not Found"}`)
	case r.Method == http.MethodPost && r.URL.EscapedPath() == mrSourceFixtureTestFilePath:
		handleMergeRequestSourceFileCreate(t, w, r)
	case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/101/merge_requests":
		writeJSON(t, w, []map[string]any{})
	default:
		http.NotFound(w, r)
	}
}

// handleMergeRequestSourceBranchCreate validates that a branch creation
// request matches the expected name and ref, then returns a 201 response.
// Mismatched inputs log an error and respond 400 so the calling test fails
// fast.
func handleMergeRequestSourceBranchCreate(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	request := decodeJSONMap(t, w, r, "branch")
	if request == nil {
		return
	}
	if request["branch"] != mrSourceFixtureTestBranch || request["ref"] != liveFixtureDefaultRef {
		t.Errorf("branch request = %+v", request)
		http.Error(w, "unexpected branch request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(t, w, map[string]any{"name": mrSourceFixtureTestBranch})
}

// handleMergeRequestSourceFileCreate validates that a repository file
// creation request uses the expected branch and commit message, then
// returns a 201 with the created file metadata. The handler intentionally
// fails the test if the request deviates so the fixture cannot silently
// accept bad inputs.
func handleMergeRequestSourceFileCreate(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	request := decodeJSONMap(t, w, r, "file")
	if request == nil {
		return
	}
	if request["branch"] != mrSourceFixtureTestBranch || request["commit_message"] != "Seed evaluation merge request fixture" {
		t.Errorf("file request = %+v", request)
		http.Error(w, "unexpected file request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(t, w, map[string]any{"file_path": "tmp/eval-mr-feature-eval-gpt54mini-r1-abc123.txt", "branch": mrSourceFixtureTestBranch})
}

// assertMergeRequestSourceFileRef logs an error when the file lookup query
// string does not pin to the attempt-scoped branch, ensuring the fixture's
// idempotency contract is enforced by the test handler.
func assertMergeRequestSourceFileRef(t *testing.T, r *http.Request) {
	t.Helper()
	if r.URL.Query().Get("ref") != mrSourceFixtureTestBranch {
		t.Errorf("file ref = %q, want %q", r.URL.Query().Get("ref"), mrSourceFixtureTestBranch)
	}
}

// decodeJSONMap decodes a JSON request body into a map, logs an error and
// responds 400 on failure. The name argument is used in the log message to
// disambiguate the request type (e.g. "branch" vs "file").
func decodeJSONMap(t *testing.T, w http.ResponseWriter, r *http.Request, name string) map[string]any {
	t.Helper()
	var request map[string]any
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Errorf("decode %s request: %v", name, err)
		http.Error(w, "decode request", http.StatusBadRequest)
		return nil
	}
	return request
}

// writeNotFound writes a 404 response with the supplied body, mirroring
// GitLab's error envelope for the fixture tests.
func writeNotFound(w http.ResponseWriter, body string) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(body))
}

// writeJSON encodes value as JSON and writes it to w, failing the test if
// the encoder returns an error. Used by the httptest handlers to keep the
// response shape consistent.
func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
