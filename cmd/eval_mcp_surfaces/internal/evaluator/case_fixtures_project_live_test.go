package evaluator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	mrSourceFixtureTestBranch   = "feature/eval-gpt54mini-r1-abc123"
	mrSourceFixtureTestFilePath = "/api/v4/projects/101/repository/files/tmp%2Feval-mr-feature-eval-gpt54mini-r1-abc123.txt"
)

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

func newMergeRequestSourceFixtureServer(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.Method+" "+r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		handleMergeRequestSourceFixtureRequest(t, w, r)
	}))
}

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

func assertMergeRequestSourceFileRef(t *testing.T, r *http.Request) {
	t.Helper()
	if r.URL.Query().Get("ref") != mrSourceFixtureTestBranch {
		t.Errorf("file ref = %q, want %q", r.URL.Query().Get("ref"), mrSourceFixtureTestBranch)
	}
}

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

func writeNotFound(w http.ResponseWriter, body string) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(body))
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
