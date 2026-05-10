package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

// TestApplyLiveFixtureState_ReplacesPromptPlaceholders verifies that ApplyLiveFixtureState handles the replaces prompt placeholders scenario correctly.
func TestApplyLiveFixtureState_ReplacesPromptPlaceholders(t *testing.T) {
	state := &liveFixtureState{
		ProjectPath:            liveFixtureProjectPath,
		ProjectID:              101,
		RemoteURL:              "http://localhost:8929/my-org/tools/gitlab-mcp-server.git",
		IssueIID:               12,
		IssueDeleteIID:         13,
		MergeRequestIID:        14,
		MergeRequestMergeIID:   15,
		MergeRequestAwardIID:   25,
		PipelineID:             16,
		PipelineIID:            17,
		FailedJobID:            18,
		ManualJobID:            19,
		RunnerID:               20,
		IssueAwardID:           21,
		MergeRequestAwardID:    22,
		MergeRequestThreadID:   "thread-123",
		PipelineScheduleID:     23,
		PipelineSchedulePlayID: 24,
		CleanupReleaseTag:      "v0.0.0-eval-delete",
		ReleaseSummaryTag:      "v0.0.0-eval-summary",
	}
	tasks := []evalTask{
		{ID: "MT-013", Prompt: "Delete issue `42` from project `my-org/tools/gitlab-mcp-server`."},
		{ID: "MT-017", Prompt: "Merge merge request `7` when ready."},
		{ID: "MT-021", Prompt: "List failed jobs in pipeline `12345` for project `my-org/tools/gitlab-mcp-server`."},
		{ID: "MT-064", Prompt: "Play manual job `999` with variable."},
		{ID: "MT-109", Prompt: "Remove award emoji ID `12` from merge request `7`."},
		{ID: "MS-008", Prompt: "Troubleshoot runner ID `99` and fetch trace for job `999`."},
		{ID: "MS-017", Prompt: "Create file `tmp/eval-crud.txt` on branch `feature/eval`."},
		{ID: "MS-025", Prompt: "Create scoped CI variable `EVAL_CRUD_TOKEN`."},
		{ID: "MT-061", Prompt: "Resolve merge request discussion with discussion_id `abc123` on merge request IID `7`."},
		{ID: "MT-061", Prompt: "Resolve merge request discussion with discussion_id `abc123` on merge_request_iid `7`."},
		{ID: taskPipelineScheduleID, Prompt: "Delete pipeline schedule ID `12`."},
		{ID: taskPipelineScheduleID, Prompt: "play pipeline schedule ID `12`."},
		{ID: "MS-004", Prompt: "Clean up release `v0.0.0-eval` in project `my-org/tools/gitlab-mcp-server`."},
		{ID: "MS-012", Prompt: "Compare refs `main` and `v0.0.0-eval-ms` in project `my-org/tools/gitlab-mcp-server`."},
		{ID: "MS-033", Prompt: "Set estimate `1h` on MR `1`, add spent time `15m`, add award emoji `eyes`."},
	}

	got := applyLiveFixtureState(tasks, state)

	assertContains(t, got[0].Prompt, "issue `13`")
	assertContains(t, got[1].Prompt, "merge request `15`")
	assertContains(t, got[2].Prompt, "pipeline `16`")
	assertContains(t, got[3].Prompt, "job `19`")
	assertContains(t, got[4].Prompt, "award emoji ID `22`")
	assertContains(t, got[5].Prompt, "runner ID `20`")
	assertContains(t, got[5].Prompt, "job `18`")
	assertContains(t, got[6].Prompt, "`tmp/eval-crud-16.txt`")
	assertContains(t, got[6].Prompt, "branch `feature/eval`")
	assertContains(t, got[7].Prompt, "`EVAL_CRUD_TOKEN_16`")
	assertContains(t, got[8].Prompt, "discussion_id `thread-123`")
	assertContains(t, got[8].Prompt, "merge request IID `14`")
	assertContains(t, got[9].Prompt, "discussion_id `thread-123`")
	assertContains(t, got[9].Prompt, "merge_request_iid `14`")
	assertContains(t, got[10].Prompt, "pipeline schedule ID `23`")
	assertContains(t, got[11].Prompt, "pipeline schedule ID `24`")
	assertContains(t, got[12].Prompt, "release `v0.0.0-eval-delete`")
	assertContains(t, got[13].Prompt, "`v0.0.0-eval-summary`")
	assertContains(t, got[14].Prompt, "MR `25`")
}

// TestFilterTasksByLiveFixtureState_SkipsMissingJobResources verifies that missing Docker job fixtures do not become model failures.
func TestFilterTasksByLiveFixtureState_SkipsMissingJobResources(t *testing.T) {
	tasks := []evalTask{
		{ID: "MT-022"},
		{ID: "MT-064"},
		{ID: "MT-065"},
		{ID: "MS-008"},
		{ID: "MT-003"},
	}
	state := &liveFixtureState{ManualJobID: 19, RunnerID: 20}

	filtered := filterTasksByLiveFixtureState(tasks, state)

	if got := taskIDs(filtered); got != "MT-064,MT-003" {
		t.Fatalf("filtered IDs = %q, want MT-064,MT-003", got)
	}
}

// TestFilterTasksByLiveFixtureState_KeepsSeededJobResources verifies seeded Docker jobs keep dependent tasks eligible.
func TestFilterTasksByLiveFixtureState_KeepsSeededJobResources(t *testing.T) {
	tasks := []evalTask{
		{ID: "MT-022"},
		{ID: "MT-064"},
		{ID: "MT-065"},
		{ID: "MS-008"},
	}
	state := &liveFixtureState{FailedJobID: 18, ManualJobID: 19, RunnerID: 20}

	filtered := filterTasksByLiveFixtureState(tasks, state)

	if got := taskIDs(filtered); got != "MT-022,MT-064,MT-065,MS-008" {
		t.Fatalf("filtered IDs = %q, want MT-022,MT-064,MT-065,MS-008", got)
	}
}

// TestFixtureCI_IsValidYAMLShape verifies that FixtureCI handles the is valid y a m l shape scenario correctly.
func TestFixtureCI_IsValidYAMLShape(t *testing.T) {
	ci := fixtureCI()
	if strings.Contains(ci, "\t") {
		t.Fatal("fixture CI must not contain tabs because GitLab YAML rejects them")
	}
	assertContains(t, ci, "failing_fixture:")
	assertContains(t, ci, "manual_deploy:")
	assertContains(t, ci, "stage: test")
}

// TestFixtureRemoteURL verifies the behavior of fixture remote u r l.
func TestFixtureRemoteURL(t *testing.T) {
	got := fixtureRemoteURL("http://localhost:8929/", liveFixtureProjectPath)
	want := "http://localhost:8929/my-org/tools/gitlab-mcp-server.git"
	if got != want {
		t.Fatalf("fixtureRemoteURL() = %q, want %q", got, want)
	}
}

// TestEnsureLiveProjectActive_UnarchivesArchivedFixtureProject verifies that EnsureLiveProjectActive handles the unarchives archived fixture project scenario correctly.
func TestEnsureLiveProjectActive_UnarchivesArchivedFixtureProject(t *testing.T) {
	calls := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                  101,
				"path_with_namespace": liveFixtureProjectPath,
				"archived":            true,
			})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/101/unarchive":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                  101,
				"path_with_namespace": liveFixtureProjectPath,
				"archived":            false,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:       server.URL,
		GitLabToken:     "eval-token",
		MetaTools:       true,
		MetaParamSchema: config.DefaultMetaParamSchema,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if activeErr := ensureLiveProjectActive(t.Context(), client); activeErr != nil {
		t.Fatalf("ensureLiveProjectActive() error = %v", activeErr)
	}

	if got := strings.Join(calls, ","); got != "GET /api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server,POST /api/v4/projects/101/unarchive" {
		t.Fatalf("calls = %q", got)
	}
}

// TestEnsureLiveProjectVariableDeleteTarget_SeedsProductionScopedVariable verifies that EnsureLiveProjectVariableDeleteTarget handles the seeds production scoped variable scenario correctly.
func TestEnsureLiveProjectVariableDeleteTarget_SeedsProductionScopedVariable(t *testing.T) {
	calls := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/variables/EVAL_TOKEN":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Variable Not Found"}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/variables":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
				http.Error(w, "decode request", http.StatusBadRequest)
				return
			}
			if request["key"] != "EVAL_TOKEN" || request["environment_scope"] != "production" {
				t.Errorf("request = %+v, want EVAL_TOKEN production", request)
				http.Error(w, "unexpected variable request", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"key":               "EVAL_TOKEN",
				"environment_scope": "production",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:       server.URL,
		GitLabToken:     "eval-token",
		MetaTools:       true,
		MetaParamSchema: config.DefaultMetaParamSchema,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = ensureLiveProjectVariableDeleteTarget(t.Context(), client, "Delete CI variable `EVAL_TOKEN` from production scope in project `my-org/tools/gitlab-mcp-server`.")
	if err != nil {
		t.Fatalf("ensureLiveProjectVariableDeleteTarget() error = %v", err)
	}

	if got := strings.Join(calls, ","); got != "DELETE /api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/variables/EVAL_TOKEN,DELETE /api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/variables/EVAL_TOKEN,POST /api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/variables" {
		t.Fatalf("calls = %q", got)
	}
}

// TestAddLiveAttemptResourceSuffix_IsolatesCreatedResources verifies that AddLiveAttemptResourceSuffix handles the isolates created resources scenario correctly.
func TestAddLiveAttemptResourceSuffix_IsolatesCreatedResources(t *testing.T) {
	task := evalTask{
		ID:     "MT-036",
		Prompt: "Create release `v0.0.0-eval-248` for tag `v0.0.0-eval-248` from ref `main` in project `my-org/tools/gitlab-mcp-server`.",
	}

	got := addLiveAttemptResourceSuffix(task, "google:gemini-3-flash-preview", 2, "abc123")

	assertContains(t, got.Prompt, "`v0.0.0-eval-248-gemini3flash-r2-abc123`")
	assertContains(t, got.Prompt, "`main`")
	assertContains(t, got.Prompt, "`my-org/tools/gitlab-mcp-server`")
}

// TestAddLiveAttemptResourceSuffix_LeavesLookupTasksAlone verifies that AddLiveAttemptResourceSuffix handles the leaves lookup tasks alone scenario correctly.
func TestAddLiveAttemptResourceSuffix_LeavesLookupTasksAlone(t *testing.T) {
	task := evalTask{
		ID:     "MT-027",
		Prompt: "Update CI variable `EVAL_TOKEN` for production scope in project `my-org/tools/gitlab-mcp-server`.",
	}

	got := addLiveAttemptResourceSuffix(task, "openai:gpt-5.4-mini", 1, "abc123")

	if got.Prompt != task.Prompt {
		t.Fatalf("Prompt = %q, want unchanged %q", got.Prompt, task.Prompt)
	}
}

// TestAddLiveAttemptResourceSuffix_UsesUnderscoresForCIVariableKeys verifies that AddLiveAttemptResourceSuffix handles the uses underscores for c i variable keys scenario correctly.
func TestAddLiveAttemptResourceSuffix_UsesUnderscoresForCIVariableKeys(t *testing.T) {
	task := evalTask{
		ID:     "MT-026",
		Prompt: "Create masked CI variable `EVAL_TOKEN` with value `masked-value-123` in project `my-org/tools/gitlab-mcp-server`.",
	}

	got := addLiveAttemptResourceSuffix(task, "qwen:qwen3.6-flash", 3, "abc123")

	assertContains(t, got.Prompt, "`EVAL_TOKEN_qwen36flash_r3_abc123`")
}

// TestAddLiveAttemptResourceSuffix_IsolatesWorkflowResources verifies that AddLiveAttemptResourceSuffix handles the isolates workflow resources scenario correctly.
func TestAddLiveAttemptResourceSuffix_IsolatesWorkflowResources(t *testing.T) {
	task := evalTask{
		ID:     "MS-018",
		Prompt: "Create release `v0.0.0-crud-248` named `Evaluation CRUD release 248`, add asset link `eval-crud-link-248`, then delete the release and tag.",
	}

	got := addLiveAttemptResourceSuffix(task, "google:gemini-3.1-flash-lite-preview", 2, "abc123")

	assertContains(t, got.Prompt, "`v0.0.0-crud-248-gemini31flas-r2-abc123`")
	assertContains(t, got.Prompt, "`Evaluation CRUD release 248 gemini31flas-r2-abc123`")
	assertContains(t, got.Prompt, "`eval-crud-link-248-gemini31flas-r2-abc123`")
}

// TestAddLiveAttemptResourceSuffix_FileCreateKeepsFixtureBranch verifies that AddLiveAttemptResourceSuffix handles the file create keeps fixture branch scenario correctly.
func TestAddLiveAttemptResourceSuffix_FileCreateKeepsFixtureBranch(t *testing.T) {
	task := evalTask{
		ID:     "MT-030",
		Prompt: "Create file `tmp/eval.txt` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.",
	}

	got := addLiveAttemptResourceSuffix(task, "openai:gpt-5.4-mini", 1, "abc123")

	assertContains(t, got.Prompt, "`tmp/eval.txt-gpt54mini-r1-abc123`")
	assertContains(t, got.Prompt, "`feature/eval`")
}

// TestAddLiveAttemptResourceSuffix_FileCreateKeepsFixtureBranchAfterFixtureReplacement verifies that AddLiveAttemptResourceSuffix handles the file create keeps fixture branch after fixture replacement scenario correctly.
func TestAddLiveAttemptResourceSuffix_FileCreateKeepsFixtureBranchAfterFixtureReplacement(t *testing.T) {
	task := evalTask{
		ID:     "MT-030",
		Prompt: "Create file `tmp/eval-248.txt` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.",
	}

	got := addLiveAttemptResourceSuffix(task, "anthropic:claude-haiku-4-5-20251001", 1, "abc123")

	assertContains(t, got.Prompt, "`tmp/eval-248.txt-claudehaiku4-r1-abc123`")
	assertContains(t, got.Prompt, "`feature/eval`")
}

// TestAddLiveAttemptResourceSuffix_MRCreateIsolatesSourceBranch verifies that AddLiveAttemptResourceSuffix handles the m r create isolates source branch scenario correctly.
func TestAddLiveAttemptResourceSuffix_MRCreateIsolatesSourceBranch(t *testing.T) {
	task := evalTask{
		ID:     "MT-015",
		Prompt: "Create a merge request in project `my-org/tools/gitlab-mcp-server` from `feature/eval` into `main` titled `Evaluation MR`.",
	}

	got := addLiveAttemptResourceSuffix(task, "openai:gpt-5.4-mini", 1, "abc123")

	assertContains(t, got.Prompt, "`feature/eval-gpt54mini-r1-abc123`")
	assertContains(t, got.Prompt, "`Evaluation MR gpt54mini-r1-abc123`")
	assertContains(t, got.Prompt, "`main`")
	assertContains(t, got.Prompt, "`my-org/tools/gitlab-mcp-server`")
}

// TestBacktickValueAfter verifies the behavior of backtick value after.
func TestBacktickValueAfter(t *testing.T) {
	prompt := "Create a merge request in project `my-org/tools/gitlab-mcp-server` from `feature/eval-x` into `main`."

	got, ok := backtickValueAfter(prompt, " from ")

	if !ok || got != "feature/eval-x" {
		t.Fatalf("backtickValueAfter() = %q, %t; want feature/eval-x, true", got, ok)
	}
}

// TestReplacePromptJobID verifies the behavior of replace prompt job i d.
func TestReplacePromptJobID(t *testing.T) {
	prompt := "Play manual job `496` in project `my-org/tools/gitlab-mcp-server` with variable `DEPLOY_ENV=staging`."

	got, err := replacePromptJobID(prompt, 1234)

	if err != nil {
		t.Fatalf("replacePromptJobID() error = %v", err)
	}
	assertContains(t, got, "manual job `1234`")
	if strings.Contains(got, "job `496`") {
		t.Fatalf("replacePromptJobID() = %q, still contains old job ID", got)
	}
}

// TestReplacePromptBacktickValueAfter verifies the behavior of replace prompt backtick value after.
func TestReplacePromptBacktickValueAfter(t *testing.T) {
	prompt := "Delete pipeline trigger token ID `77` from project `my-org/tools/gitlab-mcp-server`."

	got, err := replacePromptBacktickValueAfter(prompt, "pipeline trigger token ID ", 1234)

	if err != nil {
		t.Fatalf("replacePromptBacktickValueAfter() error = %v", err)
	}
	assertContains(t, got, "pipeline trigger token ID `1234`")
	if strings.Contains(got, "`77`") {
		t.Fatalf("replacePromptBacktickValueAfter() = %q, still contains old trigger ID", got)
	}
}

// TestSafeFixturePathPart verifies the behavior of safe fixture path part.
func TestSafeFixturePathPart(t *testing.T) {
	got := safeFixturePathPart("feature/eval-GPT54Mini-r1-abc123")
	want := "feature-eval-gpt54mini-r1-abc123"
	if got != want {
		t.Fatalf("safeFixturePathPart() = %q, want %q", got, want)
	}
}

// TestLiveFixturePreparerDefaultRef_DetectedBranch_ReturnsDetectedBranch verifies fixture setup honors the project default branch discovered from GitLab.
func TestLiveFixturePreparerDefaultRef_DetectedBranch_ReturnsDetectedBranch(t *testing.T) {
	preparer := &liveFixturePreparer{state: &liveFixtureState{DefaultBranch: "trunk"}}
	if got := preparer.defaultRef(); got != "trunk" {
		t.Fatalf("defaultRef() = %q, want trunk", got)
	}
	preparer.state.DefaultBranch = ""
	if got := preparer.defaultRef(); got != liveFixtureDefaultRef {
		t.Fatalf("defaultRef(empty) = %q, want %q", got, liveFixtureDefaultRef)
	}
}

// TestEnsureCIVariables_RecreatesProjectGroupAndInstanceVariables verifies fixture preparation restores every variable scope it removes.
func TestEnsureCIVariables_RecreatesProjectGroupAndInstanceVariables(t *testing.T) {
	created := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/101/variables/EVAL_TOKEN"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Variable Not Found"}`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/groups/202/variables/GROUP_EVAL_TOKEN"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Variable Not Found"}`))
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api/v4/admin/ci/variables/INSTANCE_EVAL_TOKEN":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Variable Not Found"}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/101/variables":
			if !assertVariableCreateRequest(t, w, r, "EVAL_TOKEN", "production") {
				return
			}
			created["project"] = true
			_, _ = w.Write([]byte(`{"key":"EVAL_TOKEN","value":"masked-value-123","environment_scope":"production"}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/groups/202/variables":
			if !assertVariableCreateRequest(t, w, r, "GROUP_EVAL_TOKEN", "production") {
				return
			}
			created["group"] = true
			_, _ = w.Write([]byte(`{"key":"GROUP_EVAL_TOKEN","value":"masked-value-123","environment_scope":"production"}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/admin/ci/variables":
			if !assertVariableCreateRequest(t, w, r, "INSTANCE_EVAL_TOKEN", "") {
				return
			}
			created["instance"] = true
			_, _ = w.Write([]byte(`{"key":"INSTANCE_EVAL_TOKEN","value":"masked-value-123"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newFixtureTestClient(t, server.URL)
	preparer := &liveFixturePreparer{client: client, state: &liveFixtureState{ProjectID: 101, GroupID: 202}}

	if err := preparer.ensureCIVariables(t.Context()); err != nil {
		t.Fatalf("ensureCIVariables() error = %v", err)
	}
	for _, scope := range []string{"project", "group", "instance"} {
		if !created[scope] {
			t.Fatalf("%s variable was not recreated", scope)
		}
	}
}

// TestEnsureFile_UpdateMissingFile_CreatesFile verifies a stale successful GetFile result is recovered with CreateFile.
func TestEnsureFile_UpdateMissingFile_CreatesFile(t *testing.T) {
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/101/repository/files/README.md":
			if r.URL.Query().Get("ref") != "trunk" {
				t.Errorf("ref = %q, want trunk", r.URL.Query().Get("ref"))
			}
			_, _ = w.Write([]byte(`{"file_path":"README.md","branch":"trunk","encoding":"base64","content":"b2xkCg=="}`))
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/101/repository/files/README.md":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"A file with this name doesn't exist"}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/101/repository/files/README.md":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode create request: %v", err)
				http.Error(w, "decode request", http.StatusBadRequest)
				return
			}
			if request["branch"] != "trunk" || request["content"] != "new content\n" {
				t.Errorf("create request = %+v, want trunk branch and content", request)
				http.Error(w, "unexpected create request", http.StatusBadRequest)
				return
			}
			created = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"file_path":"README.md","branch":"trunk"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newFixtureTestClient(t, server.URL)
	preparer := &liveFixturePreparer{client: client, state: &liveFixtureState{ProjectID: 101}}

	if err := preparer.ensureFile(t.Context(), "README.md", "trunk", "new content\n", "Seed README"); err != nil {
		t.Fatalf("ensureFile() error = %v", err)
	}
	if !created {
		t.Fatal("CreateFile was not called after missing-file update error")
	}
}

// newFixtureTestClient creates a GitLab client for fixture unit tests.
func newFixtureTestClient(t *testing.T, gitlabURL string) *gitlabclient.Client {
	t.Helper()
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:       gitlabURL,
		GitLabToken:     "eval-token",
		MetaTools:       true,
		MetaParamSchema: config.DefaultMetaParamSchema,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

// assertVariableCreateRequest verifies a CI variable fixture creation request.
func assertVariableCreateRequest(t *testing.T, w http.ResponseWriter, r *http.Request, key, environmentScope string) bool {
	t.Helper()
	var request map[string]any
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Errorf("decode variable request: %v", err)
		http.Error(w, "decode request", http.StatusBadRequest)
		return false
	}
	if request["key"] != key || request["value"] != "masked-value-123" {
		t.Errorf("variable request = %+v, want key %s and fixture value", request, key)
		http.Error(w, "unexpected variable request", http.StatusBadRequest)
		return false
	}
	if environmentScope == "" {
		if _, ok := request["environment_scope"]; ok {
			t.Errorf("variable request = %+v, want no environment_scope", request)
			http.Error(w, "unexpected variable scope", http.StatusBadRequest)
			return false
		}
		return true
	}
	if request["environment_scope"] != environmentScope {
		t.Errorf("variable request = %+v, want environment_scope %s", request, environmentScope)
		http.Error(w, "unexpected variable scope", http.StatusBadRequest)
		return false
	}
	return true
}

// assertContains is an internal helper for the main package.
func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("%q does not contain %q", text, want)
	}
}
