package main

import (
	"strings"
	"testing"
)

func TestApplyLiveFixtureState_ReplacesPromptPlaceholders(t *testing.T) {
	state := &liveFixtureState{
		ProjectPath:          liveFixtureProjectPath,
		ProjectID:            101,
		RemoteURL:            "http://localhost:8929/my-org/tools/gitlab-mcp-server.git",
		IssueIID:             12,
		IssueDeleteIID:       13,
		MergeRequestIID:      14,
		MergeRequestMergeIID: 15,
		PipelineID:           16,
		PipelineIID:          17,
		FailedJobID:          18,
		ManualJobID:          19,
		RunnerID:             20,
		IssueAwardID:         21,
		MergeRequestAwardID:  22,
		MergeRequestThreadID: "thread-123",
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
	assertContains(t, got[7].Prompt, "`EVAL_CRUD_TOKEN_16`")
	assertContains(t, got[8].Prompt, "discussion_id `thread-123`")
	assertContains(t, got[8].Prompt, "merge request IID `14`")
	assertContains(t, got[9].Prompt, "discussion_id `thread-123`")
	assertContains(t, got[9].Prompt, "merge_request_iid `14`")
}

func TestFixtureCI_IsValidYAMLShape(t *testing.T) {
	ci := fixtureCI()
	if strings.Contains(ci, "\t") {
		t.Fatal("fixture CI must not contain tabs because GitLab YAML rejects them")
	}
	assertContains(t, ci, "failing_fixture:")
	assertContains(t, ci, "manual_deploy:")
	assertContains(t, ci, "stage: test")
}

func TestFixtureRemoteURL(t *testing.T) {
	got := fixtureRemoteURL("http://localhost:8929/", liveFixtureProjectPath)
	want := "http://localhost:8929/my-org/tools/gitlab-mcp-server.git"
	if got != want {
		t.Fatalf("fixtureRemoteURL() = %q, want %q", got, want)
	}
}

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

func TestAddLiveAttemptResourceSuffix_UsesUnderscoresForCIVariableKeys(t *testing.T) {
	task := evalTask{
		ID:     "MT-026",
		Prompt: "Create masked CI variable `EVAL_TOKEN` with value `masked-value-123` in project `my-org/tools/gitlab-mcp-server`.",
	}

	got := addLiveAttemptResourceSuffix(task, "qwen:qwen3.6-flash", 3, "abc123")

	assertContains(t, got.Prompt, "`EVAL_TOKEN_qwen36flash_r3_abc123`")
}

func TestAddLiveAttemptResourceSuffix_FileCreateKeepsFixtureBranch(t *testing.T) {
	task := evalTask{
		ID:     "MT-030",
		Prompt: "Create file `tmp/eval.txt` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.",
	}

	got := addLiveAttemptResourceSuffix(task, "openai:gpt-5.4-mini", 1, "abc123")

	assertContains(t, got.Prompt, "`tmp/eval.txt-gpt54mini-r1-abc123`")
	assertContains(t, got.Prompt, "`feature/eval`")
}

func TestAddLiveAttemptResourceSuffix_FileCreateKeepsFixtureBranchAfterFixtureReplacement(t *testing.T) {
	task := evalTask{
		ID:     "MT-030",
		Prompt: "Create file `tmp/eval-248.txt` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.",
	}

	got := addLiveAttemptResourceSuffix(task, "anthropic:claude-haiku-4-5-20251001", 1, "abc123")

	assertContains(t, got.Prompt, "`tmp/eval-248.txt-claudehaiku4-r1-abc123`")
	assertContains(t, got.Prompt, "`feature/eval`")
}

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

func TestBacktickValueAfter(t *testing.T) {
	prompt := "Create a merge request in project `my-org/tools/gitlab-mcp-server` from `feature/eval-x` into `main`."

	got, ok := backtickValueAfter(prompt, " from ")

	if !ok || got != "feature/eval-x" {
		t.Fatalf("backtickValueAfter() = %q, %t; want feature/eval-x, true", got, ok)
	}
}

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

func TestSafeFixturePathPart(t *testing.T) {
	got := safeFixturePathPart("feature/eval-GPT54Mini-r1-abc123")
	want := "feature-eval-gpt54mini-r1-abc123"
	if got != want {
		t.Fatalf("safeFixturePathPart() = %q, want %q", got, want)
	}
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("%q does not contain %q", text, want)
	}
}
