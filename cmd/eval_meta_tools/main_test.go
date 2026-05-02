package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func TestParseTasksMarkdown_ParsesTaskRows(t *testing.T) {
	markdown := `# Test

| ID | Prompt | Expected tool/action | Required params | Optional params | Destructive | Success verifier |
| --- | --- | --- | --- | --- | --- | --- |
| MT-001 | Show me. | ` + "`gitlab_user` / `current`" + ` | none | none | No | ok |
| MT-002 | Delete it. | ` + "`gitlab_issue` / `delete`" + ` | ` + "`project_id`, `issue_iid`" + ` | ` + "`confirm`" + ` | Yes | ok |
`
	tasks, err := parseTasksMarkdown(markdown)
	if err != nil {
		t.Fatalf("parseTasksMarkdown() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(tasks))
	}
	if tasks[0].ExpectedTool != "gitlab_user" || tasks[0].ExpectedAction != "current" {
		t.Fatalf("task[0] = %+v", tasks[0])
	}
	if !tasks[1].Destructive {
		t.Fatal("task[1].Destructive = false, want true")
	}
	if got := strings.Join(tasks[1].RequiredParams, ","); got != "project_id,issue_iid" {
		t.Fatalf("required params = %q", got)
	}
	if got := strings.Join(tasks[1].OptionalParams, ","); got != "confirm" {
		t.Fatalf("optional params = %q", got)
	}
}

func TestParseTasksMarkdown_ParsesMultiStepRows(t *testing.T) {
	markdown := `# Test

| ID | Prompt | Expected sequence | Required params by step | Optional params by step | Destructive steps | Success verifier |
| --- | --- | --- | --- | --- | --- | --- |
| MS-001 | Resolve a remote and inspect a file. | ` + "`gitlab_discover_project` -> `gitlab_project` / `get` -> `gitlab_repository` / `file_get`" + ` | ` + "`remote_url`; `project_id`; `project_id`, `file_path`, `ref`" + ` | none; none; none | none | ok |
| MS-002 | Remove stale project hook after listing hooks in project ` + "`my-org/tools/gitlab-mcp-server`" + `. | ` + "`gitlab_project` / `hook_list` -> `gitlab_project` / `hook_delete`" + ` | ` + "`project_id`; `project_id`, `hook_id`" + ` | none; ` + "`confirm`" + ` | 2 | ok |
`
	tasks, err := parseTasksMarkdown(markdown)
	if err != nil {
		t.Fatalf("parseTasksMarkdown() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(tasks))
	}
	if len(tasks[0].Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(tasks[0].Steps))
	}
	if tasks[0].Steps[0].ExpectedTool != "gitlab_discover_project" || tasks[0].Steps[0].ExpectedAction != "" {
		t.Fatalf("first step = %+v, want standalone discover_project", tasks[0].Steps[0])
	}
	if got := strings.Join(tasks[0].Steps[2].RequiredParams, ","); got != "project_id,file_path,ref" {
		t.Fatalf("third step required params = %q", got)
	}
	if !tasks[1].Steps[1].Destructive {
		t.Fatal("second scenario step is not destructive, want destructive")
	}
}

func TestValidateTaskFixture_RequiresProjectGrounding(t *testing.T) {
	tasks := []evalTask{{
		ID:             "MT-001",
		Prompt:         "Cancel pipeline `123`.",
		ExpectedTool:   "gitlab_pipeline",
		ExpectedAction: "cancel",
		RequiredParams: []string{"project_id", "pipeline_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}}
	problems := validateTaskFixture(tasks)
	if len(problems) != 1 || !strings.Contains(problems[0], "project_id") {
		t.Fatalf("problems = %+v, want project_id grounding problem", problems)
	}
}

func TestValidateTaskFixture_AcceptsGroundedProject(t *testing.T) {
	tasks := []evalTask{{
		ID:             "MT-001",
		Prompt:         "Cancel pipeline `123` in project `my-org/tools/gitlab-mcp-server`.",
		ExpectedTool:   "gitlab_pipeline",
		ExpectedAction: "cancel",
		RequiredParams: []string{"project_id", "pipeline_id"},
		OptionalParams: []string{"confirm"},
		Destructive:    true,
	}}
	if problems := validateTaskFixture(tasks); len(problems) != 0 {
		t.Fatalf("problems = %+v, want none", problems)
	}
}

func TestValidateToolCall_RequiresNestedParams(t *testing.T) {
	task := evalTask{ExpectedTool: "gitlab_issue", ExpectedAction: "delete", RequiredParams: []string{"project_id", "issue_iid"}, Destructive: true}
	result := validateToolCall(task, "gitlab_issue", map[string]any{
		"action":     "delete",
		"project_id": "42",
	})
	if result.Valid {
		t.Fatal("validateToolCall() Valid = true, want false")
	}
	if !strings.Contains(result.Message, "unexpected top-level parameter project_id") {
		t.Fatalf("message = %q, want top-level parameter guidance", result.Message)
	}
}

func TestValidateToolCall_AcceptsConfirmedDestructiveCall(t *testing.T) {
	task := evalTask{ExpectedTool: "gitlab_issue", ExpectedAction: "delete", RequiredParams: []string{"project_id", "issue_iid"}, Destructive: true}
	result := validateToolCall(task, "gitlab_issue", map[string]any{
		"action": "delete",
		"params": map[string]any{
			"project_id": "42",
			"issue_iid":  7,
			"confirm":    true,
		},
	})
	if !result.Valid {
		t.Fatalf("validateToolCall() Valid = false: %s", result.Message)
	}
	if !result.DestructiveSafe {
		t.Fatal("DestructiveSafe = false, want true")
	}
}

func TestValidateToolCall_DoesNotRequireConfirmForWrongReadOnlyAttempt(t *testing.T) {
	task := evalTask{ExpectedTool: "gitlab_repository", ExpectedAction: "file_delete", RequiredParams: []string{"project_id", "file_path", "branch"}, Destructive: true}
	result := validateToolCall(task, "gitlab_repository", map[string]any{
		"action": "file_metadata",
		"params": map[string]any{
			"project_id": "42",
			"file_path":  "README.md",
			"ref":        "main",
		},
	})
	if result.Valid {
		t.Fatal("validateToolCall() Valid = true, want false")
	}
	if !result.DestructiveSafe {
		t.Fatal("DestructiveSafe = false for a wrong read-only attempt, want true")
	}
}

func TestValidateStandaloneToolCall_AcceptsTopLevelInput(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab_discover_project", RequiredParams: []string{"remote_url"}}
	result := validateStepCall(step, "gitlab_discover_project", map[string]any{
		"remote_url": "https://gitlab.example.com/my-org/project.git",
	})
	if !result.Valid {
		t.Fatalf("validateStepCall() Valid = false: %s", result.Message)
	}
}

func TestValidateStandaloneToolCall_RejectsMetaEnvelope(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab_discover_project", RequiredParams: []string{"remote_url"}}
	result := validateStepCall(step, "gitlab_discover_project", map[string]any{
		"action": "resolve",
		"params": map[string]any{"remote_url": "https://gitlab.example.com/my-org/project.git"},
	})
	if result.Valid {
		t.Fatal("validateStepCall() Valid = true, want false")
	}
	if !strings.Contains(result.Message, "standalone tool") {
		t.Fatalf("message = %q, want standalone guidance", result.Message)
	}
}

func TestRunStaticValidation_ValidatesMultiStepRoutes(t *testing.T) {
	tasks := []evalTask{{
		ID: "MS-001",
		Steps: []evalStep{
			{ExpectedTool: "gitlab_discover_project"},
			{ExpectedTool: "gitlab_project", ExpectedAction: "get"},
			{ExpectedTool: "gitlab_repository", ExpectedAction: "file_get"},
		},
	}}
	routes := map[string]toolutil.ActionMap{
		"gitlab_project":    {"get": {}},
		"gitlab_repository": {"file_get": {}},
	}
	toolNames := map[string]bool{"gitlab_discover_project": true, "gitlab_project": true, "gitlab_repository": true}
	results := runStaticValidation(tasks, routes, toolNames, 1)
	if len(results) != 1 || !results[0].FinalSuccess || results[0].CompletedSteps != 3 {
		t.Fatalf("results = %+v, want completed multi-step validation", results)
	}
}

func TestLoadToolsSnapshot_DerivesRoutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.json")
	snapshot := `[
  {
    "name": "gitlab_project",
    "description": "Manage projects.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "action": {"type": "string", "enum": ["get", "list"]},
        "params": {"type": "object"}
      }
    }
  }
]`
	if err := os.WriteFile(path, []byte(snapshot), 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	tools, routes, err := loadToolsSnapshot(path)
	if err != nil {
		t.Fatalf("loadToolsSnapshot() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "gitlab_project" {
		t.Fatalf("tools = %+v, want gitlab_project", tools)
	}
	if _, ok := routes["gitlab_project"]["get"]; !ok {
		t.Fatalf("routes = %+v, want gitlab_project/get", routes)
	}
	if _, ok := routes["gitlab_project"]["list"]; !ok {
		t.Fatalf("routes = %+v, want gitlab_project/list", routes)
	}
}

func TestCalculateMetrics_HandlesNoRepairs(t *testing.T) {
	results := []taskResult{{
		Task:            evalTask{ExpectedTool: "gitlab_user", ExpectedAction: "current"},
		FirstTool:       "gitlab_user",
		FirstAction:     "current",
		FirstPass:       true,
		FinalSuccess:    true,
		DestructiveSafe: true,
	}}
	measured := calculateMetrics(results)
	if measured.ToolSelection != 100 || measured.ActionSelection != 100 || measured.RepairSuccess != 100 {
		t.Fatalf("metrics = %+v, want all applicable metrics at 100", measured)
	}
}

func TestCalculateMetrics_AggregatesRepeatedAttempts(t *testing.T) {
	results := []taskResult{
		{
			Run:             1,
			Task:            evalTask{ExpectedTool: "gitlab_user", ExpectedAction: "current"},
			FirstTool:       "gitlab_user",
			FirstAction:     "current",
			FirstPass:       true,
			FinalSuccess:    true,
			DestructiveSafe: true,
		},
		{
			Run:             2,
			Task:            evalTask{ExpectedTool: "gitlab_user", ExpectedAction: "current"},
			FirstTool:       "gitlab_project",
			FirstAction:     "get",
			FinalSuccess:    false,
			DestructiveSafe: true,
		},
	}
	measured := calculateMetrics(results)
	if measured.ToolSelection != 50 || measured.ActionSelection != 50 || measured.FinalSuccess != 50 {
		t.Fatalf("metrics = %+v, want repeated attempts aggregated at 50%%", measured)
	}
}

func TestAggregateUsage_SumsRequestsToolCallsAndTokens(t *testing.T) {
	results := []taskResult{
		{AnthropicCalls: 2, ToolCalls: 3, Usage: anthropicUsage{InputTokens: 100, OutputTokens: 20, CacheCreationInputTokens: 50}},
		{AnthropicCalls: 1, ToolCalls: 1, Usage: anthropicUsage{InputTokens: 25, OutputTokens: 5, CacheReadInputTokens: 200}},
	}
	summary := aggregateUsage(results)
	if summary.AnthropicCalls != 3 || summary.ToolCalls != 4 {
		t.Fatalf("summary calls = %+v, want 3 requests and 4 tool calls", summary)
	}
	if summary.Usage.InputTokens != 125 || summary.Usage.OutputTokens != 25 || summary.Usage.CacheCreationInputTokens != 50 || summary.Usage.CacheReadInputTokens != 200 {
		t.Fatalf("usage = %+v, want summed tokens", summary.Usage)
	}
}

func TestEstimateCostUSD_UsesPerMillionPricing(t *testing.T) {
	cost := estimateCostUSD(anthropicUsage{InputTokens: 1_000_000, OutputTokens: 100_000}, pricingOptions{InputPerMTok: 3, OutputPerMTok: 15})
	if cost != 4.5 {
		t.Fatalf("cost = %v, want 4.5", cost)
	}
}
