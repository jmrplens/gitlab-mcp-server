package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestCapabilityBridgePredicates_ClassifyBridgeTools verifies runner bridge
// detection is shared by generic and expected-step paths.
func TestCapabilityBridgePredicates_ClassifyBridgeTools(t *testing.T) {
	if !isCapabilityBridge(modelContentBlock{Name: resourceReadTool}) {
		t.Fatal("isCapabilityBridge(resourceReadTool) = false, want true")
	}
	if isCapabilityBridge(modelContentBlock{Name: dynamicFindTool}) {
		t.Fatal("isCapabilityBridge(dynamicFindTool) = true, want false")
	}
	if !expectedCapabilityBridgeStep(evalStep{ExpectedTool: completionTool}) {
		t.Fatal("expectedCapabilityBridgeStep(completion) = false, want true")
	}
	if expectedCapabilityBridgeStep(evalStep{ExpectedTool: completionTool, ExpectedAction: "schema_get"}) {
		t.Fatal("expectedCapabilityBridgeStep(with action) = true, want false")
	}
}

// TestRecordCapabilityBridgeMetrics_CountsResourcesSeparately verifies bridge
// calls update both aggregate capability and resource-specific metrics.
func TestRecordCapabilityBridgeMetrics_CountsResourcesSeparately(t *testing.T) {
	var result taskResult
	recordCapabilityBridgeMetrics(&result, modelContentBlock{Name: resourceListTool})
	recordCapabilityBridgeMetrics(&result, modelContentBlock{Name: promptListTool})
	if !result.CapabilityLookupUsed || result.CapabilityCalls != 2 {
		t.Fatalf("capability metrics = used %t calls %d, want true/2", result.CapabilityLookupUsed, result.CapabilityCalls)
	}
	if !result.ResourceLookupUsed || result.ResourceCalls != 1 {
		t.Fatalf("resource metrics = used %t calls %d, want true/1", result.ResourceLookupUsed, result.ResourceCalls)
	}
}

// TestToolUseBlocks_FiltersNonToolContent verifies only provider tool-use blocks
// participate in validation and execution.
func TestToolUseBlocks_FiltersNonToolContent(t *testing.T) {
	blocks := toolUseBlocks([]modelContentBlock{{Type: "text", Text: "hello"}, {Type: "tool_use", Name: dynamicFindTool}})
	if len(blocks) != 1 || blocks[0].Name != dynamicFindTool {
		t.Fatalf("toolUseBlocks() = %+v, want only dynamic find tool", blocks)
	}
}

// TestValidateDynamicFindResult_UsesFullMCPResponse verifies that
// validateDynamicFindResult inspects the structuredContent of the MCP
// response when the textual payload is truncated, so discovery failures are
// still detected when a stub truncates the body.
//
// The test feeds two exchanges: one whose structuredContent contains the
// expected action and one that does not. The first call must succeed even
// though the textual payload lacks the action; the second must return an
// error. This protects the runner from silently accepting truncated
// responses during dynamic discovery.
func TestValidateDynamicFindResult_UsesFullMCPResponse(t *testing.T) {
	steps := []evalStep{
		{ExpectedTool: dynamicFindTool, RequiredParams: []string{"query"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.get"},
	}
	exchange := &traceMCPExchange{Response: []byte(`{"structuredContent":{"results":[{"id":"project.get"}]}}`)}
	if err := validateDynamicFindResult(steps, 0, "truncated payload without the action", exchange); err != nil {
		t.Fatalf("validateDynamicFindResult() error = %v, want full MCP response match", err)
	}
	missing := &traceMCPExchange{Response: []byte(`{"structuredContent":{"results":[{"id":"project.list"}]}}`)}
	if err := validateDynamicFindResult(steps, 0, "truncated payload without the action", missing); err == nil {
		t.Fatal("validateDynamicFindResult() error = nil, want missing expected action error")
	}
}

// TestRedactResponse_TruncatesLargeProviderBodies verifies provider trace errors
// stay compact in terminal and report diagnostics.
func TestRedactResponse_TruncatesLargeProviderBodies(t *testing.T) {
	large := make([]byte, 1200)
	for i := range large {
		large[i] = 'x'
	}
	got := redactResponse(large)
	if len(got) != 1003 || got[len(got)-3:] != "..." {
		t.Fatalf("redactResponse() length/suffix = %d/%q, want 1003/...", len(got), got[len(got)-3:])
	}
}

// TestRunnerTraceSummaryAndResourceHelpers verifies small runner helpers used by
// simulated execution and trace finalization.
func TestRunnerTraceSummaryAndResourceHelpers(t *testing.T) {
	if got := snippetFilePathFromParams(map[string]any{"files": []any{map[string]any{"file_path": "src/snippet.go"}}}); got != "src/snippet.go" {
		t.Fatalf("snippetFilePathFromParams(files) = %q", got)
	}
	if got := snippetFilePathFromParams(map[string]any{}); got != "snippet.txt" {
		t.Fatalf("snippetFilePathFromParams(default) = %q", got)
	}
	result := map[string]any{}
	addSimulatedResourceIDs(result, "snippet.project_create", map[string]any{"project_id": "p", "file_name": "main.go"})
	if result["snippet_id"] != 103 || result["snippet"].(map[string]any)["file_path"] != "main.go" {
		t.Fatalf("simulated snippet result = %#v", result)
	}
	summary := traceSummaryFromResult(taskResult{Task: evalTask{Steps: []evalStep{{}, {}}}, FirstTool: "tool", FinalTool: "final", FirstPass: true, FinalSuccess: true, CompletedSteps: 2, Notes: []string{"a", "b"}})
	if summary.ExpectedSteps != 2 || summary.Notes != "a; b" || !summary.FinalSuccess {
		t.Fatalf("traceSummaryFromResult() = %+v, want expected steps and notes", summary)
	}
}

// TestEvaluatePreparedCase_UsesRenderedPromptAndTypedSteps verifies that the
// runner's evaluatePreparedCase honors the rendered prompt and typed steps
// from a [PreparedCase], including produced values and expected assertions.
//
// The test runs a scripted runner against a prepared case whose prompt has
// been rendered with fixture values and whose first step declares a
// produced_value. It asserts the result reports success, that the task and
// trace prompts both contain the rendered fixture text, that the produced
// value is preserved, and that the expected-action and required-params
// assertions both pass. This protects the runner's typed-case integration
// from regressing to the legacy unrendered prompt path.
func TestEvaluatePreparedCase_UsesRenderedPromptAndTypedSteps(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("final", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "fixture/project"}}),
	)
	prepared := PreparedCase{
		Case: EvalCase{
			ID:     "MT-PREPARED-001",
			Prompt: "Get project `placeholder/project`.",
			Steps:  []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}},
		},
		Prompt: "Get project `fixture/project`.",
		Steps:  []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}, ProducedValues: []string{"project_id"}}},
	}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}

	result := runner.evaluatePreparedCase(t.Context(), prepared, nil, routes)

	if !result.FinalSuccess || result.CompletedSteps != 1 {
		t.Fatalf("result = %+v, want prepared case success", result)
	}
	if result.Task.Prompt != "Get project `fixture/project`." || !strings.Contains(result.Trace.UserPrompt, "fixture/project") {
		t.Fatalf("prompt task=%q trace=%q, want rendered fixture prompt", result.Task.Prompt, result.Trace.UserPrompt)
	}
	if got := strings.Join(result.Task.Steps[0].ProducedValues, ","); got != "project_id" {
		t.Fatalf("produced values = %q, want project_id", got)
	}
	if !hasPassedAssertion(result.AssertionResults, CaseAssertionExpectedAction) || !hasPassedAssertion(result.AssertionResults, CaseAssertionRequiredParams) {
		t.Fatalf("assertion results = %+v, want expected action and required params pass", result.AssertionResults)
	}
}

// TestEvaluateTask_AcceptsOptionalCapabilityBridgePreludeSkip verifies that
// the runner marks an evaluation successful when the model skips an optional
// capability bridge prelude but completes the required resource bridge
// steps that follow.
//
// The test seeds a three-step task (optional capability list, required
// resource list, required resource read) and a scripted runner that skips
// the optional prelude. It asserts the result reports first-pass success
// without a repair, exposes the expected bridge metrics, and records an
// "accepted optional" note. This protects the runner from penalizing
// models that intelligently skip optional capability preludes.
func TestEvaluateTask_AcceptsOptionalCapabilityBridgePreludeSkip(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("resources", resourceListTool, map[string]any{}),
		toolUseResponse("tools", resourceReadTool, map[string]any{"uri": "gitlab://tools"}),
	)
	runner.mcpSession = newResourceLookupSessionForTest(t)
	task := evalTask{ID: "MS-039", Steps: []evalStep{
		{ExpectedTool: capabilityListTool, OptionalStep: true},
		{ExpectedTool: resourceListTool},
		{ExpectedTool: resourceReadTool, RequiredParams: []string{"uri"}},
	}}

	result := runner.evaluateTask(t.Context(), task, nil, nil)

	if !result.FinalSuccess || !result.FirstPass || result.RepairAttempted {
		t.Fatalf("result = %+v, want first-pass success without repair", result)
	}
	toolOK, actionOK, firstPassOK := effectiveFirstOutcome(result)
	if !toolOK || !actionOK || !firstPassOK {
		t.Fatalf("effective first outcome = %t/%t/%t, want all true", toolOK, actionOK, firstPassOK)
	}
	if result.CompletedSteps != 3 || result.FirstTool != resourceListTool {
		t.Fatalf("completed/first = %d/%s, want 3/%s", result.CompletedSteps, result.FirstTool, resourceListTool)
	}
	if !result.ResourceLookupUsed || result.ResourceCalls != 2 || result.CapabilityCalls != 2 {
		t.Fatalf("bridge metrics = resource:%t resource_calls:%d capability_calls:%d, want two resource bridge calls", result.ResourceLookupUsed, result.ResourceCalls, result.CapabilityCalls)
	}
	if !strings.Contains(strings.Join(result.Notes, "; "), "accepted optional") {
		t.Fatalf("notes = %v, want accepted optional note", result.Notes)
	}
}

// TestEvaluateTask_AcceptsDirectDynamicExecuteWithoutFind verifies that the
// runner accepts a model that skips the gitlab_find_action discovery step and
// calls gitlab_execute_action directly with the expected action.
//
// The test seeds a two-step task (expected find then execute) but scripts
// the runner to call execute directly. It asserts the result reports first-pass
// success without repair, completes both steps, records the direct call as
// the first tool, and emits the "accepted direct" note. This protects the
// runner from rejecting direct execute calls when the prompt supplies
// sufficient context.
func TestEvaluateTask_AcceptsDirectDynamicExecuteWithoutFind(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("exec", dynamicExecuteActionTool, map[string]any{"action": actionProjectGet, "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MT-DYN-DIRECT-001", Steps: []evalStep{
		{ExpectedTool: dynamicFindTool, RequiredParams: []string{"query"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, RequiredParams: []string{"project_id"}},
	}}

	result := runner.evaluateTask(t.Context(), task, nil, nil)

	if !result.FinalSuccess || !result.FirstPass || result.RepairAttempted {
		t.Fatalf("result = %+v, want first-pass success without repair", result)
	}
	if result.CompletedSteps != 2 {
		t.Fatalf("completed steps = %d, want 2", result.CompletedSteps)
	}
	if result.FirstTool != dynamicExecuteActionTool || result.FirstAction != actionProjectGet {
		t.Fatalf("first call = %s/%s, want %s/%s", result.FirstTool, result.FirstAction, dynamicExecuteActionTool, actionProjectGet)
	}
	if !strings.Contains(strings.Join(result.Notes, "; "), "accepted direct") {
		t.Fatalf("notes = %v, want accepted direct note", result.Notes)
	}
}

func hasPassedAssertion(results []CaseAssertionResult, assertionType CaseAssertionType) bool {
	for _, result := range results {
		if result.Type == assertionType && result.Passed {
			return true
		}
	}
	return false
}

// TestDynamicDiscoveryResult_UsesRuntimeIntentIndex verifies that dynamic find
// evaluation uses the runtime intent index for natural-language action discovery.
func TestDynamicDiscoveryResult_UsesRuntimeIntentIndex(t *testing.T) {
	catalogRoutes := map[string]toolutil.ActionMap{
		"gitlab_merge_request": {
			"list": {InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":      map[string]any{"type": "integer"},
					"state":           map[string]any{"type": "string"},
					"author_username": map[string]any{"type": "string"},
				},
			}},
		},
	}
	catalogRoutes, err := dynamictools.AddStandaloneRoutes(catalogRoutes, nil, dynamictools.StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}
	routes := dynamicValidationRoutes(catalogRoutes)

	tests := []struct {
		query string
		want  []string
	}{
		{query: "discover project from remote url", want: []string{"discover_project.resolve"}},
		{query: "merge request list open authored by me project", want: []string{"merge_request.list"}},
		{query: "discover project from remote url merge request list current user open authored", want: []string{"discover_project.resolve", "merge_request.list"}},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			content, contentErr := dynamicDiscoveryResult(t.Context(), routes, modelContentBlock{
				Name: dynamicFindTool,
				Input: map[string]any{
					"query": tt.query,
					"limit": float64(3),
				},
			})
			if contentErr != nil {
				t.Fatalf("dynamicDiscoveryResult() error = %v", contentErr)
			}
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("dynamicDiscoveryResult() = %s, want %s", content, want)
				}
			}
		})
	}
}

// TestDynamicDiscoveryResult_FindIncludesSchema verifies that gitlab_find_action
// returns the schema and execute-tool target needed for the next model call.
func TestDynamicDiscoveryResult_FindIncludesSchema(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteActionTool: {
			"project.get": {InputSchema: map[string]any{
				"type":       "object",
				"required":   []any{"project_id"},
				"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
			}},
		},
	}

	content, err := dynamicDiscoveryResult(t.Context(), routes, modelContentBlock{
		Name: dynamicFindTool,
		Input: map[string]any{
			"query": "project get",
			"limit": float64(3),
		},
	})
	if err != nil {
		t.Fatalf("dynamicDiscoveryResult(find) error = %v", err)
	}
	for _, want := range []string{"project.get", "project_id", dynamicExecuteActionTool, "input_schema"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(content, want) {
				t.Fatalf("find result = %s, want %q", content, want)
			}
		})
	}
}

// TestTaskToolCallLimit_ScalesForLongWorkflows verifies TaskToolCallLimit scales for long workflows.
func TestTaskToolCallLimit_ScalesForLongWorkflows(t *testing.T) {
	if got := taskToolCallLimit(3); got != 13 {
		t.Fatalf("taskToolCallLimit(3) = %d, want enough turns for schema lookups and 3 steps", got)
	}
	if got := taskToolCallLimit(4); got != 16 {
		t.Fatalf("taskToolCallLimit(4) = %d, want enough turns for schema lookups and 4 steps", got)
	}
	if got := taskToolCallLimit(8); got != 28 {
		t.Fatalf("taskToolCallLimit(8) = %d, want enough turns for schema lookups and 8 steps", got)
	}
}

// TestTaskToolCallLimitForSurface_UsesBaseLimit verifies that dynamic and meta
// use the same task call limit.
func TestTaskToolCallLimitForSurface_UsesBaseLimit(t *testing.T) {
	if got := taskToolCallLimitForSurface(4, config.ToolSurfaceDynamic); got != 16 {
		t.Fatalf("taskToolCallLimitForSurface(4, dynamic) = %d, want 16", got)
	}
	if got := taskToolCallLimitForSurface(4, config.ToolSurfaceMeta); got != 16 {
		t.Fatalf("taskToolCallLimitForSurface(4, meta) = %d, want 16", got)
	}
}

// TestRepairAttemptLimitForSurface_DefaultsToOne verifies the evaluator repair
// budget remains one retry per surface.
func TestRepairAttemptLimitForSurface_DefaultsToOne(t *testing.T) {
	if got := repairAttemptLimitForSurface(config.ToolSurfaceDynamic); got != 1 {
		t.Fatalf("repairAttemptLimitForSurface(dynamic) = %d, want 1", got)
	}
	if got := repairAttemptLimitForTask(config.ToolSurfaceDynamic, 7); got != 1 {
		t.Fatalf("repairAttemptLimitForTask(dynamic, 7) = %d, want 1", got)
	}
	if got := repairAttemptLimitForSurface(config.ToolSurfaceMeta); got != 1 {
		t.Fatalf("repairAttemptLimitForSurface(meta) = %d, want 1", got)
	}
}

// TestDiscoveryBudgetFeedback_AllowsFindFirstForExactDynamicCall verifies discovery is no longer suppressed for exact-looking Dynamic calls.
func TestDiscoveryBudgetFeedback_AllowsFindFirstForExactDynamicCall(t *testing.T) {
	task := evalTask{ID: "MT-066", Prompt: "Remove project ID `51` from the CI job token allowlist of project `1`.", Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.token_scope_remove_project", RequiredParams: []string{"project_id", "target_project_id"}, OptionalParams: []string{"confirm"}, Destructive: true},
	}}
	step := taskSteps(task)[0]
	message, blocked := discoveryBudgetFeedback(task, step, modelContentBlock{Name: dynamicFindTool}, callBudgetForTask(task, config.ToolSurfaceDynamic))
	if blocked || message != "" {
		t.Fatalf("discoveryBudgetFeedback() = %q, %t; want allowed find-first discovery", message, blocked)
	}
}

// TestDynamicDiscoveryResult_Find verifies dynamic discovery returns enough
// action metadata for the next execute call.
func TestDynamicDiscoveryResult_Find(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteActionTool: {
			"project.get": {InputSchema: map[string]any{
				"type":       "object",
				"required":   []any{"project_id"},
				"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
			}},
		},
	}

	find, err := dynamicDiscoveryResult(t.Context(), routes, modelContentBlock{Name: dynamicFindTool, Input: map[string]any{"query": "project get"}})
	if err != nil {
		t.Fatalf("dynamicDiscoveryResult(find) error = %v", err)
	}
	for _, want := range []string{"project.get", "project_id", dynamicExecuteActionTool} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(find, want) {
				t.Fatalf("find result = %s, want %q", find, want)
			}
		})
	}
}

// TestAppendLookupFollowup_DynamicFindUsesLiveMCPTool verifies live dynamic
// discovery calls exercise the registered MCP tool instead of bypassing it.
func TestAppendLookupFollowup_DynamicFindUsesLiveMCPTool(t *testing.T) {
	client, cleanup, clientErr := newMockGitLabClient()
	if clientErr != nil {
		t.Fatalf("newMockGitLabClient() error = %v", clientErr)
	}
	defer cleanup()
	session, closeSession, _, routes, sessionErr := buildCatalogSession(client, config.ToolSurfaceDynamic, ServerModeDefault)
	if sessionErr != nil {
		t.Fatalf("buildCatalogSession() error = %v", sessionErr)
	}
	defer closeSession()

	runner := &modelRunner{mcpSession: session}
	result := &taskResult{}
	followups := []modelContentBlock{}
	runner.appendLookupFollowup(t.Context(), lookupFollowupContext{
		routes:    routes,
		toolUse:   modelContentBlock{ID: "find-1", Name: dynamicFindTool, Input: map[string]any{"query": "project get", "limit": 1}},
		result:    result,
		followups: &followups,
		dynamic:   true,
	})

	if len(followups) != 1 || followups[0].IsError {
		t.Fatalf("followups = %#v, want one successful dynamic find result", followups)
	}
	if !strings.Contains(followups[0].Content, actionProjectGet) {
		t.Fatalf("dynamic find content = %s, want %s", followups[0].Content, actionProjectGet)
	}
	if len(result.Trace.Events) != 1 || result.Trace.Events[0].MCP == nil {
		t.Fatalf("trace events = %#v, want MCP exchange", result.Trace.Events)
	}
	if result.Trace.Events[0].MCP.Request.Name != dynamicFindTool {
		t.Fatalf("MCP request = %#v, want %s", result.Trace.Events[0].MCP.Request, dynamicFindTool)
	}
}

// TestSuccessfulSimulatedToolContent_IncludesCreatedResourceIDs verifies that
// simulated mutating responses include resource IDs for follow-up evaluation steps.
func TestSuccessfulSimulatedToolContent_IncludesCreatedResourceIDs(t *testing.T) {
	content := successfulSimulatedToolContent(evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.badge_add"}, modelContentBlock{
		Name: dynamicExecuteActionTool,
		Input: map[string]any{
			"action": "project.badge_add",
			"params": map[string]any{"project_id": "my-org/project"},
		},
	}, 2, 4)

	if !strings.Contains(content, `"badge_id":102`) || !strings.Contains(content, `"id":102`) {
		t.Fatalf("successfulSimulatedToolContent() = %s, want badge id fields", content)
	}

	content = successfulSimulatedToolContent(evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "mr_review.note_create"}, modelContentBlock{
		Name: dynamicExecuteActionTool,
		Input: map[string]any{
			"action": "merge_request_note.create",
			"params": map[string]any{"project_id": "my-org/project", "merge_request_iid": float64(7)},
		},
	}, 2, 4)
	if !strings.Contains(content, `"note_id":104`) {
		t.Fatalf("successfulSimulatedToolContent(alias) = %s, want note_id", content)
	}
}

// TestSuccessfulSimulatedToolContent_IncludesPackageDirectoryURLs verifies simulated package publishes include usable URLs.
func TestSuccessfulSimulatedToolContent_IncludesPackageDirectoryURLs(t *testing.T) {
	content := successfulSimulatedToolContent(evalStep{ExpectedTool: "gitlab_package", ExpectedAction: "publish_directory"}, modelContentBlock{
		Name: "gitlab_package",
		Input: map[string]any{
			"action": "publish_directory",
			"params": map[string]any{
				"project_id":      liveFixtureProjectPath,
				"package_name":    liveFixturePackageReleaseName,
				"package_version": liveFixturePackageReleaseVersion,
				"directory_path":  "/tmp/package-release-files",
			},
		},
	}, 2, 3)

	requireContainsAll(t, "successfulSimulatedToolContent()", content, []string{
		`"published"`,
		`"file_name":"checksums.txt"`,
		`"url":"https://gitlab.example.com/api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server/packages/generic/eval-release-package/0.1.0/checksums.txt"`,
	})
}

// TestInvalidToolUseFingerprint_StableForRepeatedInvalidRetry verifies InvalidToolUseFingerprint when stable for repeated invalid retry.
func TestInvalidToolUseFingerprint_StableForRepeatedInvalidRetry(t *testing.T) {
	toolUse := modelContentBlock{Name: dynamicExecuteActionTool, Input: map[string]any{"action": "project.delete", "params": map[string]any{"project_id": "my-org/project"}}}
	first := invalidToolUseFingerprint(toolUse)
	second := invalidToolUseFingerprint(toolUse)
	if first == "" || first != second {
		t.Fatalf("invalidToolUseFingerprint() = %q then %q, want stable non-empty fingerprint", first, second)
	}
}

// TestToolExecutionNote_ClassifiesGitLabRoleConfusion verifies ToolExecutionNote classifies GitLab role confusion.
func TestToolExecutionNote_ClassifiesGitLabRoleConfusion(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.token_scope_remove_project", RequiredParams: []string{"project_id", "target_project_id"}}
	note := toolExecutionNote(1, step, errors.New("GitLab 400 Bad Request: target project is not in scope"))

	var payload repairPayload
	if err := json.Unmarshal([]byte(note), &payload); err != nil {
		t.Fatalf("toolExecutionNote() JSON error = %v; note = %s", err, note)
	}
	if payload.ErrorKind != "gitlab_bad_request_role_confusion" || payload.BadParam != "project_id,target_project_id" || !payload.RetryAllowed {
		t.Fatalf("execution repair payload = %+v, want role-confusion bad request", payload)
	}
	if !strings.Contains(payload.LikelyFix, "project_id is the owning project") {
		t.Fatalf("execution repair payload = %+v, want role-sensitive likely_fix", payload)
	}
}

// TestSchemaLookupResult_IndexAndActionSchema verifies SchemaLookupResult when index and action schema.
func TestSchemaLookupResult_IndexAndActionSchema(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {
			"delete": toolutil.ActionRoute{Destructive: true, InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
				},
			}},
		},
	}
	indexPayload, err := schemaLookupResult(routes, map[string]any{"action": "schema_index", "params": map[string]any{"tool": "gitlab_project"}})
	if err != nil {
		t.Fatalf("schemaLookupResult(index) error = %v", err)
	}
	if !strings.Contains(indexPayload, "gitlab://schema/meta/gitlab_project/delete") {
		t.Fatalf("index payload = %s, want schema URI", indexPayload)
	}
	schemaPayload, err := schemaLookupResult(routes, map[string]any{"action": "schema_get", "params": map[string]any{"tool": "gitlab_project", "action": "delete"}})
	if err != nil {
		t.Fatalf("schemaLookupResult(schema) error = %v", err)
	}
	if !strings.Contains(schemaPayload, "\"confirm\"") || !strings.Contains(schemaPayload, "\"x_destructive\":true") {
		t.Fatalf("schema payload = %s, want destructive confirmation metadata", schemaPayload)
	}
}

// TestSchemaLookupResult_UnknownToolReturnsError verifies SchemaLookupResult when unknown tool returns error.
func TestSchemaLookupResult_UnknownToolReturnsError(t *testing.T) {
	_, err := schemaLookupResult(map[string]toolutil.ActionMap{}, map[string]any{"action": "schema_index", "params": map[string]any{"tool": "gitlab_missing"}})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("error = %v, want unknown tool", err)
	}
}

// TestSchemaLookupResult_MissingToolReturnsUsageExamples verifies SchemaLookupResult when missing tool returns usage examples.
func TestSchemaLookupResult_MissingToolReturnsUsageExamples(t *testing.T) {
	payload, err := schemaLookupResult(map[string]toolutil.ActionMap{}, map[string]any{"action": "schema_get", "params": map[string]any{}})
	if err != nil {
		t.Fatalf("schemaLookupResult() error = %v, want usage payload", err)
	}
	if !strings.Contains(payload, `"action":"schema_get"`) || !strings.Contains(payload, `"tool":"gitlab"`) || !strings.Contains(payload, "pipeline.get") {
		t.Fatalf("payload = %s, want schema_get usage examples", payload)
	}
}

// TestSuccessfulSimulatedToolContent_IncludesDiscoveredProject verifies SuccessfulSimulatedToolContent includes discovered project.
func TestSuccessfulSimulatedToolContent_IncludesDiscoveredProject(t *testing.T) {
	content := successfulSimulatedToolContent(evalStep{}, modelContentBlock{
		Name:  "gitlab_discover_project",
		Input: map[string]any{"remote_url": "https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git"},
	}, 2, 3)
	if !strings.Contains(content, "my-org/tools/gitlab-mcp-server") || !strings.Contains(content, "default_branch") {
		t.Fatalf("successfulSimulatedToolContent() = %s, want project metadata", content)
	}

	content = successfulSimulatedToolContent(evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "discover_project.resolve"}, modelContentBlock{
		Name: dynamicExecuteActionTool,
		Input: map[string]any{
			"action": "search.projects",
			"params": map[string]any{"search": "gitlab-mcp-server"},
		},
	}, 2, 3)
	for _, want := range []string{"my-org/tools/gitlab-mcp-server", `"projects"`, `"environments"`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(content, want) {
				t.Fatalf("successfulSimulatedToolContent(prelude) = %s, want %q", content, want)
			}
		})
	}
}

// TestEvaluateTask_UsesSchemaLookupThenFinalCall verifies EvaluateTask uses schema lookup then final call.
func TestEvaluateTask_UsesSchemaLookupThenFinalCall(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("schema", "gitlab_server", map[string]any{"action": "schema_get", "params": map[string]any{"tool": "gitlab_project", "action": "get"}}),
		toolUseResponse("final", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MT-002", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	result := runner.evaluateTask(t.Context(), task, nil, routes)
	if !result.SchemaLookupUsed || !result.FinalSuccess || result.ModelCalls != 2 {
		t.Fatalf("result = %+v, want schema lookup and final success in two calls", result)
	}
}

// TestEvaluateTask_UsesResourceLookupThenFinalCall verifies resource bridge calls do not count as final task calls.
func TestEvaluateTask_UsesResourceLookupThenFinalCall(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("resources", resourceListTool, map[string]any{}),
		toolUseResponse("tools-detail", resourceReadTool, map[string]any{"uri": "gitlab://tools"}),
		toolUseResponse("final", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	runner.mcpSession = newResourceLookupSessionForTest(t)
	task := evalTask{ID: "MT-002", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	catalog := appendCapabilityBridgeTools([]modelTool{modelToolFromParts("gitlab_project", "project meta-tool", map[string]any{"type": "object"})}, mcpBridgeSupport{Resources: true})

	result := runner.evaluateTask(t.Context(), task, catalog, routes)

	if !result.ResourceLookupUsed || result.ResourceCalls != 2 {
		t.Fatalf("resource metrics = used:%t calls:%d, want used with two calls", result.ResourceLookupUsed, result.ResourceCalls)
	}
	if result.SchemaLookupUsed {
		t.Fatalf("SchemaLookupUsed = true, want resource lookups tracked separately")
	}
	if !result.FinalSuccess || result.ModelCalls != 3 || result.FirstTool != "gitlab_project" {
		t.Fatalf("result = %+v, want final project call after resource lookup", result)
	}
}

// TestEvaluateTask_ExpectedResourceBridgeStepAdvancesScenario verifies expected bridge calls count as workflow steps.
func TestEvaluateTask_ExpectedResourceBridgeStepAdvancesScenario(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("resources", resourceListTool, map[string]any{}),
		toolUseResponse("tools-detail", resourceReadTool, map[string]any{"uri": "gitlab://tools/project.get"}),
		toolUseResponse("final", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	runner.mcpSession = newResourceLookupSessionForTest(t)
	task := evalTask{ID: "MS-040", Steps: []evalStep{
		{ExpectedTool: resourceListTool},
		{ExpectedTool: resourceReadTool, RequiredParams: []string{"uri"}},
		{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}},
	}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}

	result := runner.evaluateTask(t.Context(), task, nil, routes)

	if !result.FinalSuccess || result.CompletedSteps != 3 {
		t.Fatalf("result = %+v, want bridge steps and final project step completed", result)
	}
	if !result.FirstPass || result.FirstTool != resourceListTool || result.FirstAction != "" {
		t.Fatalf("first call = %s/%s pass=%t, want expected resource bridge", result.FirstTool, result.FirstAction, result.FirstPass)
	}
	if !result.ResourceLookupUsed || result.ResourceCalls != 2 || result.CapabilityCalls != 2 {
		t.Fatalf("bridge metrics = resource:%t resource_calls:%d capability_calls:%d, want two resource bridge calls", result.ResourceLookupUsed, result.ResourceCalls, result.CapabilityCalls)
	}
}

// TestEvaluateTask_RecordsTraceForPromptToolUseAndValidation verifies EvaluateTask when records trace for prompt tool use and validation.
func TestEvaluateTask_RecordsTraceForPromptToolUseAndValidation(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("final", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MT-002", Prompt: "Find project `my-org/tools/gitlab-mcp-server`.", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	result := runner.evaluateTask(t.Context(), task, nil, routes)

	if result.Trace.TaskID != task.ID || !strings.Contains(result.Trace.UserPrompt, task.Prompt) {
		t.Fatalf("trace prompt = %+v, want task prompt recorded", result.Trace)
	}
	wantKinds := []string{"user_prompt", "assistant_message", "tool_use", "validation"}
	for _, kind := range wantKinds {
		t.Run(kind, func(t *testing.T) {
			if !traceHasKind(result.Trace, kind) {
				t.Fatalf("trace events = %+v, want kind %s", result.Trace.Events, kind)
			}
		})
	}
	assistantEvent, ok := traceEventByKind(result.Trace, "assistant_message")
	if !ok || assistantEvent.Provider == nil {
		t.Fatalf("trace events = %+v, want assistant provider exchange", result.Trace.Events)
	}
	if !strings.Contains(string(assistantEvent.Provider.RequestBody), `"system"`) {
		t.Fatalf("provider request = %s, want raw system prompt payload", assistantEvent.Provider.RequestBody)
	}
	if !strings.Contains(string(assistantEvent.Provider.ResponseBody), `"tool_use"`) {
		t.Fatalf("provider response = %s, want raw model response", assistantEvent.Provider.ResponseBody)
	}
}

// TestEvaluateTask_RepairsUnknownSchemaParam verifies EvaluateTask when repairs unknown schema param.
func TestEvaluateTask_RepairsUnknownSchemaParam(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("bad", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "iid": 7}}),
		toolUseResponse("good", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MT-002", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	result := runner.evaluateTask(t.Context(), task, nil, routes)
	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess {
		t.Fatalf("result = %+v, want successful repair after schema validation error", result)
	}
}

// TestEvaluateTask_RepairsNoToolUseResponse verifies the evaluator prompts for
// a tool call when a provider returns prose without a tool_use block.
func TestEvaluateTask_RepairsNoToolUseResponse(t *testing.T) {
	runner := newScriptedRunner(
		t,
		modelResponse{Content: []modelContentBlock{{Type: "text", Text: "I can do that."}}},
		toolUseResponse("good", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MT-002", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}

	result := runner.evaluateTask(t.Context(), task, nil, routes)

	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess {
		t.Fatalf("result = %+v, want successful repair after no tool_use response", result)
	}
	if result.FirstPass {
		t.Fatalf("FirstPass = true, want first no-tool response to remain a first-pass miss")
	}
	if !traceHasKind(result.Trace, "repair_prompt") {
		t.Fatalf("trace events = %+v, want repair_prompt event", result.Trace.Events)
	}
}

// TestEvaluateTask_InvalidMatchingCallUsesMCPErrorWhenExecuting verifies EvaluateTask when invalid matching call uses MCP error when executing.
func TestEvaluateTask_InvalidMatchingCallUsesMCPErrorWhenExecuting(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("bad", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{}}),
		toolUseResponse("good", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	runner.mcpSession = newProjectGetSession(t)
	task := evalTask{ID: "MT-002", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}

	result := runner.evaluateTask(t.Context(), task, nil, routes)

	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess {
		t.Fatalf("result = %+v, want successful repair after MCP error", result)
	}
	if !traceContainsToolResult(result.Trace, "MCP missing params.project_id") {
		t.Fatalf("trace events = %+v, want real MCP error content", result.Trace.Events)
	}
	toolResultEvent, ok := traceEventByKind(result.Trace, "tool_result")
	if !ok || toolResultEvent.MCP == nil {
		t.Fatalf("trace events = %+v, want MCP exchange on tool result", result.Trace.Events)
	}
	if toolResultEvent.MCP.Request.Name != "gitlab_project" || !toolResultEvent.MCP.IsError {
		t.Fatalf("MCP exchange = %+v, want gitlab_project error", toolResultEvent.MCP)
	}
	if !strings.Contains(string(toolResultEvent.MCP.Response), "MCP missing params.project_id") {
		t.Fatalf("MCP response = %s, want complete tool result", toolResultEvent.MCP.Response)
	}
}

// TestEvaluateTask_WrongReadOnlyCallUsesMCPWhenExecuting verifies EvaluateTask when wrong read only call uses MCP when executing.
func TestEvaluateTask_WrongReadOnlyCallUsesMCPWhenExecuting(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("search", "gitlab_search", map[string]any{"action": "projects", "params": map[string]any{"query": "my-org/tools/gitlab-mcp-server"}}),
		toolUseResponse("good", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	runner.mcpSession = newProjectGetSession(t)
	task := evalTask{ID: "MT-002", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {"get": projectGetRoute()},
		"gitlab_search":  {"projects": toolutil.ActionRoute{}},
	}

	result := runner.evaluateTask(t.Context(), task, nil, routes)

	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess {
		t.Fatalf("result = %+v, want successful repair after read-only MCP prefetch", result)
	}
	if !traceContainsToolResult(result.Trace, "search ok") {
		t.Fatalf("trace events = %+v, want real search result content", result.Trace.Events)
	}
}

// TestCanExecuteInvalidToolCallSkipsWrongDomainSameAction verifies wrong-domain
// calls that happen to share an action name receive exact repair feedback.
func TestCanExecuteInvalidToolCallSkipsWrongDomainSameAction(t *testing.T) {
	runner := &modelRunner{mcpSession: &mcp.ClientSession{}}
	step := evalStep{ExpectedTool: "gitlab_project", ExpectedAction: "service_account_list", RequiredParams: []string{"project_id"}}
	validation := validationResult{ToolMatches: false, ActionMatches: true, Action: "service_account_list", RequiredPresent: false, DestructiveSafe: true, Message: "expected tool gitlab_project, got gitlab_group; missing required params: project_id"}
	toolUse := modelContentBlock{Name: "gitlab_group"}
	routes := map[string]toolutil.ActionMap{"gitlab_group": {"service_account_list": toolutil.ActionRoute{}}}

	if runner.canExecuteInvalidToolCall(step, validation, toolUse, routes) {
		t.Fatal("canExecuteInvalidToolCall() = true, want wrong-domain same-action call to receive repair guidance")
	}
}

// TestCanExecuteInvalidToolCallSkipsIncompleteDynamicCalls verifies malformed
// dynamic envelopes receive evaluator repair feedback instead of repeated MCP
// schema errors.
func TestCanExecuteInvalidToolCallSkipsIncompleteDynamicCalls(t *testing.T) {
	runner := &modelRunner{mcpSession: &mcp.ClientSession{}}
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.create", RequiredParams: []string{"project_id", "title"}}
	routes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {"issue.create": toolutil.ActionRoute{}}}

	tests := []struct {
		name       string
		validation validationResult
		toolUse    modelContentBlock
	}{
		{
			name:       "missing top-level params",
			validation: validationResult{ToolMatches: true, ActionMatches: true, Action: "issue.create", RequiredPresent: false, DestructiveSafe: true, Message: `validating "arguments": validating root: required: missing properties: ["params"]`},
			toolUse:    modelContentBlock{Name: dynamicExecuteActionTool, Input: map[string]any{"action": "issue.create"}},
		},
		{
			name:       "missing nested required param",
			validation: validationResult{ToolMatches: true, ActionMatches: true, Action: "issue.create", RequiredPresent: false, DestructiveSafe: true, Message: "missing required params: title"},
			toolUse:    modelContentBlock{Name: dynamicExecuteActionTool, Input: map[string]any{"action": "issue.create", "params": map[string]any{"project_id": 1}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runner.canExecuteInvalidToolCall(step, tt.validation, tt.toolUse, routes) {
				t.Fatal("canExecuteInvalidToolCall() = true, want incomplete dynamic call to receive exact repair guidance")
			}
		})
	}
}

// TestEvaluateTask_RepairsMultipleInvalidToolCallsFromSameTurn verifies EvaluateTask when repairs multiple invalid tool calls from same turn.
func TestEvaluateTask_RepairsMultipleInvalidToolCallsFromSameTurn(t *testing.T) {
	runner := newScriptedRunner(
		t,
		multiToolUseResponse(
			modelContentBlock{Type: "tool_use", ID: "bad-project", Name: "gitlab", Input: map[string]any{"action": "project.get", "project_id": "my-org/tools/gitlab-mcp-server"}},
			modelContentBlock{Type: "tool_use", ID: "bad-file", Name: "gitlab", Input: map[string]any{"action": "repository.file_get", "project_id": "my-org/tools/gitlab-mcp-server", "file_path": "README.md", "ref": "main"}},
		),
		toolUseResponse("good-project", "gitlab", map[string]any{"action": "project.get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
		toolUseResponse("good-file", "gitlab", map[string]any{"action": "repository.file_get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "file_path": "README.md", "ref": "main"}}),
	)
	task := evalTask{ID: "MS-001", Steps: []evalStep{
		{ExpectedTool: "gitlab", ExpectedAction: "project.get", RequiredParams: []string{"project_id"}},
		{ExpectedTool: "gitlab", ExpectedAction: "repository.file_get", RequiredParams: []string{"project_id", "file_path", "ref"}},
	}}
	routes := map[string]toolutil.ActionMap{"gitlab": {"project.get": projectGetRoute(), "repository.file_get": repositoryFileGetRoute()}}
	result := runner.evaluateTask(t.Context(), task, nil, routes)
	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess {
		t.Fatalf("result = %+v, want successful repair after same-turn invalid tool calls", result)
	}
}

// TestEvaluateTask_RetriesTransientSimulation verifies EvaluateTask when retries transient simulation.
func TestEvaluateTask_RetriesTransientSimulation(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("first", "gitlab_pipeline", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "pipeline_id": 12345}}),
		toolUseResponse("retry", "gitlab_pipeline", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "pipeline_id": 12345}}),
	)
	task := evalTask{ID: "MF-001", ExpectedTool: "gitlab_pipeline", ExpectedAction: "get", RequiredParams: []string{"project_id", "pipeline_id"}, Simulation: "transient_error_once"}
	routes := map[string]toolutil.ActionMap{"gitlab_pipeline": {"get": pipelineGetRoute()}}
	result := runner.evaluateTask(t.Context(), task, nil, routes)
	if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess || result.CompletedSteps != 1 {
		t.Fatalf("result = %+v, want transient retry success", result)
	}
}

// TestEvaluateTask_PoisonedOutputDoesNotChangeNextExpectedTool verifies EvaluateTask when poisoned output does not change next expected tool.
func TestEvaluateTask_PoisonedOutputDoesNotChangeNextExpectedTool(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("file", "gitlab_repository", map[string]any{"action": "file_get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "file_path": "README.md", "ref": "main"}}),
		toolUseResponse("project", "gitlab_project", map[string]any{"action": "get", "params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"}}),
	)
	task := evalTask{ID: "MF-002", Steps: []evalStep{
		{ExpectedTool: "gitlab_repository", ExpectedAction: "file_get", RequiredParams: []string{"project_id", "file_path", "ref"}, Simulation: "poisoned_output"},
		{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}},
	}}
	routes := map[string]toolutil.ActionMap{
		"gitlab_repository": {"file_get": repositoryFileGetRoute()},
		"gitlab_project":    {"get": projectGetRoute()},
	}
	result := runner.evaluateTask(t.Context(), task, nil, routes)
	if !result.FinalSuccess || result.CompletedSteps != 2 || result.FinalTool != "gitlab_project" {
		t.Fatalf("result = %+v, want poisoned output ignored and second step completed", result)
	}
}

// newScriptedRunner constructs scripted runner test fixtures.
func newScriptedRunner(t *testing.T, responses ...modelResponse) *modelRunner {
	t.Helper()
	index := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		if index >= len(responses) {
			t.Fatalf("unexpected model request %d; scripted responses exhausted", index+1)
		}
		body, err := json.Marshal(responses[index])
		if err != nil {
			t.Fatalf("marshal scripted response: %v", err)
		}
		index++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	})}
	t.Cleanup(func() {
		if index != len(responses) {
			t.Fatalf("used %d scripted responses, want %d", index, len(responses))
		}
	})
	return &modelRunner{apiKey: "test-key", model: "test-model", maxTokens: 256, client: client, traceBodies: true}
}

// newProjectGetSession constructs project get session test fixtures.
func newProjectGetSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "eval-test", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "gitlab_project", Description: "project meta-tool"}, func(_ context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
		params, _ := input["params"].(map[string]any)
		if params["project_id"] == nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "MCP missing params.project_id"}}}, nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "project ok"}}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "gitlab_search", Description: "search meta-tool"}, func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "search ok"}}}, nil, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "eval-test-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// newResourceLookupSessionForTest constructs a minimal MCP session with listable resources.
func newResourceLookupSessionForTest(t *testing.T) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "eval-resource-test", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "gitlab_project", Description: "project meta-tool"}, func(_ context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
		params, _ := input["params"].(map[string]any)
		if params["project_id"] == nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "MCP missing params.project_id"}}}, nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "project ok"}}}, nil, nil
	})
	server.AddResource(&mcp.Resource{
		URI:      "gitlab://tools",
		Name:     "tool_manifest",
		MIMEType: "application/json",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "gitlab://tools", MIMEType: "application/json", Text: `{"surface":"meta"}`}}}, nil
	})
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://tools/{id}",
		Name:        "tool_detail",
		MIMEType:    "application/json",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: `{"id":"gitlab_project.get"}`}}}, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "eval-resource-test-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// toolUseResponse builds a scripted modelResponse carrying a single tool_use
// content block, so a test can drive the runner without a real model call.
func toolUseResponse(id, name string, input map[string]any) modelResponse {
	return modelResponse{Content: []modelContentBlock{{Type: "tool_use", ID: id, Name: name, Input: input}}}
}

// multiToolUseResponse supports multi tool use response assertions in main tests.
func multiToolUseResponse(blocks ...modelContentBlock) modelResponse {
	return modelResponse{Content: blocks}
}

// traceHasKind supports trace has kind assertions in main tests.
func traceHasKind(trace taskTrace, kind string) bool {
	for _, event := range trace.Events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

// traceEventByKind returns the first trace event with the requested kind.
func traceEventByKind(trace taskTrace, kind string) (traceEvent, bool) {
	for _, event := range trace.Events {
		if event.Kind == kind {
			return event, true
		}
	}
	return traceEvent{}, false
}

// traceContainsToolResult supports trace contains tool result assertions in main tests.
func traceContainsToolResult(trace taskTrace, text string) bool {
	for _, event := range trace.Events {
		if event.Kind == "tool_result" && strings.Contains(event.Content, text) {
			return true
		}
	}
	return false
}

// projectGetRoute supports project get route assertions in main tests.
func projectGetRoute() toolutil.ActionRoute {
	return toolutil.ActionRoute{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
	}}
}

// pipelineGetRoute supports pipeline get route assertions in main tests.
func pipelineGetRoute() toolutil.ActionRoute {
	return toolutil.ActionRoute{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string"},
			"pipeline_id": map[string]any{"type": "integer"},
		},
	}}
}

// repositoryFileGetRoute supports repository file get route assertions in main tests.
func repositoryFileGetRoute() toolutil.ActionRoute {
	return toolutil.ActionRoute{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
			"file_path":  map[string]any{"type": "string"},
			"ref":        map[string]any{"type": "string"},
		},
	}}
}

// TestRecordModelCallError_AttachesProviderTrace verifies a provider call
// error is recorded as a model_error event carrying the provider exchange, and
// a plain error is recorded without one.
func TestRecordModelCallError_AttachesProviderTrace(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		wantProvider bool
	}{
		{name: "plain error", err: errors.New("boom")},
		{name: "provider error", err: &modelProviderCallError{err: errors.New("anthropic status 401"), Trace: &modelProviderTrace{Provider: providerAnthropic, ResponseStatus: 401}}, wantProvider: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := taskResult{ModelCalls: 2}
			recordModelCallError(&result, tc.err)
			if len(result.Notes) != 1 || result.Notes[0] != tc.err.Error() {
				t.Fatalf("notes = %v, want %q", result.Notes, tc.err.Error())
			}
			event := result.Trace.Events[0]
			if event.Kind != "model_error" || !event.IsError || event.Turn != 2 {
				t.Fatalf("event = %+v, want model_error on turn 2", event)
			}
			if (event.Provider != nil) != tc.wantProvider {
				t.Fatalf("event.Provider = %+v, want provider trace = %t", event.Provider, tc.wantProvider)
			}
		})
	}
}

// TestEvaluateTask_ExpectedDynamicFindStep_AdvancesThenExecutes verifies the
// dynamic find-then-execute path: the find call is validated against the
// routes, its result is checked for the action the next step expects, and the
// scenario advances into the execute step.
func TestEvaluateTask_ExpectedDynamicFindStep_AdvancesThenExecutes(t *testing.T) {
	runner := newScriptedRunner(
		t,
		toolUseResponse("find", dynamicFindTool, map[string]any{"query": "get project metadata"}),
		toolUseResponse("exec", dynamicExecuteActionTool, map[string]any{"action": actionProjectGet, "params": map[string]any{"project_id": "my-org/app"}}),
	)
	runner.toolSurface = config.ToolSurfaceDynamic
	routes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {actionProjectGet: projectGetRoute()}}
	task := evalTask{ID: "MS-FIND", Prompt: "Get project `my-org/app`.", Steps: []evalStep{
		{ExpectedTool: dynamicFindTool, RequiredParams: []string{"query"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, RequiredParams: []string{"project_id"}},
	}}

	result := runner.evaluateTask(t.Context(), task, nil, routes)

	if !result.FinalSuccess || result.CompletedSteps != 2 || !result.SchemaLookupUsed {
		t.Fatalf("result = %+v, want find-then-execute success", result)
	}
	if result.FirstTool != dynamicFindTool || result.FinalTool != dynamicExecuteActionTool {
		t.Fatalf("first/final tool = %s/%s, want find then execute", result.FirstTool, result.FinalTool)
	}
}

// TestHandleExpectedDynamicFindStep_MissingExpectedAction_DoesNotAdvance
// verifies a valid find call whose results do not surface the action the next
// step needs records the miss as a note, sends the payload back as an errored
// tool result, and leaves the scenario on the find step.
func TestHandleExpectedDynamicFindStep_MissingExpectedAction_DoesNotAdvance(t *testing.T) {
	runner := &modelRunner{toolSurface: config.ToolSurfaceDynamic}
	routes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {"issue.list": {InputSchema: map[string]any{"type": "object"}}}}
	steps := []evalStep{
		{ExpectedTool: dynamicFindTool, RequiredParams: []string{"query"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.approve", RequiredParams: []string{"project_id"}},
	}
	result := &taskResult{}
	followups := &[]modelContentBlock{}
	state := &modelEvaluationState{firstFinalAttempt: true}
	auxCtx := auxiliaryToolUseContext{
		task:      evalTask{ID: "MS-FIND-MISS", Prompt: "Do something."},
		steps:     steps,
		toolUse:   modelContentBlock{Type: "tool_use", ID: "find", Name: dynamicFindTool, Input: map[string]any{"query": "totally unrelated words"}},
		routes:    routes,
		result:    result,
		state:     state,
		followups: followups,
	}

	if stop := runner.handleExpectedDynamicFindStep(t.Context(), auxCtx); stop {
		t.Fatal("handleExpectedDynamicFindStep() = true, want the attempt to continue")
	}
	if state.stepIndex != 0 || result.CompletedSteps != 0 || !result.SchemaLookupUsed {
		t.Fatalf("state = %+v result = %+v, want no progress past the find step", state, result)
	}
	if !strings.Contains(strings.Join(result.Notes, "; "), "did not include expected action merge_request.approve") {
		t.Fatalf("notes = %v, want missing-action note", result.Notes)
	}
	if len(*followups) != 1 || !(*followups)[0].IsError {
		t.Fatalf("followups = %+v, want one errored tool result", *followups)
	}
}

// TestHandleExpectedDynamicFindStep_MatchingResult_AdvancesScenario verifies a
// find call whose results include the next step's action advances the
// scenario, records the schema lookup, and returns a non-error tool result.
func TestHandleExpectedDynamicFindStep_MatchingResult_AdvancesScenario(t *testing.T) {
	runner := &modelRunner{toolSurface: config.ToolSurfaceDynamic}
	routes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {actionProjectGet: projectGetRoute()}}
	steps := []evalStep{
		{ExpectedTool: dynamicFindTool, RequiredParams: []string{"query"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, RequiredParams: []string{"project_id"}},
	}
	result := &taskResult{}
	followups := &[]modelContentBlock{}
	state := &modelEvaluationState{firstFinalAttempt: true}
	auxCtx := auxiliaryToolUseContext{
		task:      evalTask{ID: "MS-FIND-HIT"},
		steps:     steps,
		toolUse:   modelContentBlock{Type: "tool_use", ID: "find", Name: dynamicFindTool, Input: map[string]any{"query": "get project metadata"}},
		routes:    routes,
		result:    result,
		state:     state,
		followups: followups,
	}

	if stop := runner.handleExpectedDynamicFindStep(t.Context(), auxCtx); stop {
		t.Fatal("handleExpectedDynamicFindStep() = true, want the scenario to continue into the execute step")
	}
	if state.stepIndex != 1 || result.CompletedSteps != 1 || !result.SchemaLookupUsed {
		t.Fatalf("state = %+v result = %+v, want the find step completed", state, result)
	}
	if len(*followups) != 1 || (*followups)[0].IsError {
		t.Fatalf("followups = %+v, want one successful tool result", *followups)
	}
}

// TestEvaluateTask_InvalidDynamicFindCall_SendsRepairThenStopsOnRepeat
// verifies an invalid find call gets one repair message and a byte-identical
// retry ends the attempt instead of looping.
func TestEvaluateTask_InvalidDynamicFindCall_SendsRepairThenStopsOnRepeat(t *testing.T) {
	invalid := toolUseResponse("find", dynamicFindTool, map[string]any{})
	runner := newScriptedRunner(t, invalid, invalid)
	runner.toolSurface = config.ToolSurfaceDynamic
	task := evalTask{ID: "MS-FIND-INVALID", Steps: []evalStep{
		{ExpectedTool: dynamicFindTool, RequiredParams: []string{"query"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet},
	}}

	result := runner.evaluateTask(t.Context(), task, nil, nil)

	if result.FinalSuccess || !result.RepairAttempted {
		t.Fatalf("result = %+v, want repaired but failed attempt", result)
	}
	if !strings.Contains(strings.Join(result.Notes, "; "), "repeated invalid retry") {
		t.Fatalf("notes = %v, want repeated invalid retry note", result.Notes)
	}
}

// TestHandleInvalidCapabilityBridgeCall_RepairsThenStops verifies an invalid
// capability bridge call produces one repair follow-up, that an identical
// retry stops the attempt, and that an exhausted repair budget stops it too.
func TestHandleInvalidCapabilityBridgeCall_RepairsThenStops(t *testing.T) {
	toolUse := modelContentBlock{Type: "tool_use", ID: "read", Name: resourceReadTool, Input: map[string]any{}}
	validation := validationResult{Message: "missing required uri"}
	newContext := func(repairAlreadySent bool, state *modelEvaluationState) (capabilityBridgeStepContext, *taskResult, *[]modelContentBlock) {
		result := &taskResult{}
		followups := &[]modelContentBlock{}
		return capabilityBridgeStepContext{
			steps:             []evalStep{{ExpectedTool: resourceReadTool, RequiredParams: []string{"uri"}}},
			toolUse:           toolUse,
			result:            result,
			repairAlreadySent: repairAlreadySent,
			state:             state,
			followups:         followups,
		}, result, followups
	}

	t.Run("first invalid call repairs", func(t *testing.T) {
		bridgeCtx, result, followups := newContext(false, &modelEvaluationState{})
		if stop := handleInvalidCapabilityBridgeCall(bridgeCtx, bridgeCtx.steps[0], validation); stop {
			t.Fatal("handleInvalidCapabilityBridgeCall() = true, want continue after repair")
		}
		if !result.RepairAttempted || len(*followups) != 1 || !(*followups)[0].IsError {
			t.Fatalf("result = %+v followups = %+v, want one repair follow-up", result, *followups)
		}
	})
	t.Run("repeated fingerprint stops", func(t *testing.T) {
		state := &modelEvaluationState{lastInvalidFingerprint: invalidToolUseFingerprint(toolUse)}
		bridgeCtx, _, followups := newContext(false, state)
		if stop := handleInvalidCapabilityBridgeCall(bridgeCtx, bridgeCtx.steps[0], validation); !stop {
			t.Fatal("handleInvalidCapabilityBridgeCall() = false, want stop on repeated invalid retry")
		}
		if len(*followups) != 0 {
			t.Fatalf("followups = %+v, want none", *followups)
		}
	})
	t.Run("repair budget exhausted stops", func(t *testing.T) {
		bridgeCtx, _, _ := newContext(true, &modelEvaluationState{})
		if stop := handleInvalidCapabilityBridgeCall(bridgeCtx, bridgeCtx.steps[0], validation); !stop {
			t.Fatal("handleInvalidCapabilityBridgeCall() = false, want stop when repair budget is spent")
		}
	})
}

// TestAppendDynamicPreludeFollowup_RecordsFirstCallAndSuccessResult verifies
// an accepted prelude call is recorded as the first (passing) call and gets a
// successful simulated tool result.
func TestAppendDynamicPreludeFollowup_RecordsFirstCallAndSuccessResult(t *testing.T) {
	result := &taskResult{}
	state := &modelEvaluationState{firstFinalAttempt: true}
	followups := &[]modelContentBlock{}
	steps := []evalStep{{ExpectedTool: dynamicFindTool}, {ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet}}
	toolUse := modelContentBlock{Type: "tool_use", ID: "find", Name: dynamicFindTool, Input: map[string]any{"query": "project"}}

	appendDynamicPreludeFollowup(steps, 0, toolUse, validationResult{Valid: true, Action: ""}, result, state, followups)

	if result.FirstTool != dynamicFindTool || !result.FirstPass || state.firstFinalAttempt {
		t.Fatalf("result = %+v state = %+v, want first call recorded as passing", result, state)
	}
	if len(*followups) != 1 || (*followups)[0].IsError || !strings.Contains((*followups)[0].Content, `"ok":true`) {
		t.Fatalf("followups = %+v, want one successful tool result", *followups)
	}
}

// TestIsRedundantDiscoveryTool_ClassifiesDiscoveryTools verifies the discovery
// budget only suppresses the find, dispatcher and server tools.
func TestIsRedundantDiscoveryTool_ClassifiesDiscoveryTools(t *testing.T) {
	cases := []struct {
		tool string
		want bool
	}{
		{tool: dynamicFindTool, want: true},
		{tool: "gitlab", want: true},
		{tool: "gitlab_server", want: true},
		{tool: dynamicExecuteActionTool, want: false},
		{tool: "gitlab_project", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			if got := isRedundantDiscoveryTool(tc.tool); got != tc.want {
				t.Fatalf("isRedundantDiscoveryTool(%s) = %t, want %t", tc.tool, got, tc.want)
			}
		})
	}
}

// TestExactDynamicCallAvailable_RequiresOneSafeExecuteStep verifies the exact
// dynamic call is only claimed for a single execute step whose parameters all
// bind to concrete prompt values.
func TestExactDynamicCallAvailable_RequiresOneSafeExecuteStep(t *testing.T) {
	cases := []struct {
		name string
		task evalTask
		want bool
	}{
		{name: "safe single execute", task: evalTask{Prompt: "Get project `my-org/app`.", Steps: []evalStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, RequiredParams: []string{"project_id"}}}}, want: true},
		{name: "unresolved param", task: evalTask{Prompt: "Get a project.", Steps: []evalStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, RequiredParams: []string{"project_id"}}}}, want: false},
		{name: "two steps", task: evalTask{Steps: []evalStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet}, {ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectList}}}, want: false},
		{name: "not an execute step", task: evalTask{Steps: []evalStep{{ExpectedTool: dynamicFindTool}}}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exactDynamicCallAvailable(tc.task, taskSteps(tc.task)); got != tc.want {
				t.Fatalf("exactDynamicCallAvailable() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestDiscoveryBudgetFeedback_SuppressesRedundantDiscovery verifies the
// discovery budget only blocks a redundant discovery call when the exact
// dynamic call is already provable from the prompt.
func TestDiscoveryBudgetFeedback_SuppressesRedundantDiscovery(t *testing.T) {
	exactStep := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, RequiredParams: []string{"project_id"}}
	task := evalTask{Prompt: "Get project `my-org/app`.", Steps: []evalStep{exactStep}}
	cases := []struct {
		name    string
		budget  taskCallBudget
		toolUse modelContentBlock
		step    evalStep
		want    bool
	}{
		{name: "suppression off", budget: taskCallBudget{}, toolUse: modelContentBlock{Name: dynamicFindTool}, step: exactStep},
		{name: "not a discovery tool", budget: taskCallBudget{SuppressDiscovery: true}, toolUse: modelContentBlock{Name: dynamicExecuteActionTool}, step: exactStep},
		{name: "exact call unavailable", budget: taskCallBudget{SuppressDiscovery: true}, toolUse: modelContentBlock{Name: dynamicFindTool}, step: evalStep{ExpectedTool: dynamicFindTool}},
		{name: "blocked", budget: taskCallBudget{SuppressDiscovery: true}, toolUse: modelContentBlock{Name: dynamicFindTool}, step: exactStep, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			message, blocked := discoveryBudgetFeedback(task, tc.step, tc.toolUse, tc.budget)
			if blocked != tc.want {
				t.Fatalf("discoveryBudgetFeedback() blocked = %t, want %t", blocked, tc.want)
			}
			if blocked && !strings.Contains(message, actionProjectGet) {
				t.Fatalf("message = %q, want the exact action", message)
			}
		})
	}
}

// TestNoToolUseRepairMessage_NamesTheNextExpectedCall verifies the repair
// prompt names the next tool and action when the step index is in range, the
// tool alone for a standalone step, and stays generic when it is not.
func TestNoToolUseRepairMessage_NamesTheNextExpectedCall(t *testing.T) {
	steps := []evalStep{{ExpectedTool: dynamicFindTool}, {ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet}}
	cases := []struct {
		name      string
		stepIndex int
		want      string
	}{
		{name: "standalone step", stepIndex: 0, want: "calling " + dynamicFindTool + " now"},
		{name: "action step", stepIndex: 1, want: "with action " + actionProjectGet},
		{name: "out of range", stepIndex: 5, want: "calling the next required tool now"},
		{name: "negative", stepIndex: -1, want: "calling the next required tool now"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := noToolUseRepairMessage(tc.stepIndex, steps); !strings.Contains(got, tc.want) {
				t.Fatalf("noToolUseRepairMessage(%d) = %q, want %q", tc.stepIndex, got, tc.want)
			}
		})
	}
}

// TestNextDynamicExecuteAction_RequiresAFollowingExecuteStep verifies the
// action a find step must surface comes from the immediately following execute
// step, and is empty at the end of a scenario or before a non-execute step.
func TestNextDynamicExecuteAction_RequiresAFollowingExecuteStep(t *testing.T) {
	cases := []struct {
		name  string
		steps []evalStep
		index int
		want  string
	}{
		{name: "execute follows", steps: []evalStep{{ExpectedTool: dynamicFindTool}, {ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet}}, want: actionProjectGet},
		{name: "last step", steps: []evalStep{{ExpectedTool: dynamicFindTool}}},
		{name: "non execute follows", steps: []evalStep{{ExpectedTool: dynamicFindTool}, {ExpectedTool: resourceListTool}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextDynamicExecuteAction(tc.steps, tc.index); got != tc.want {
				t.Fatalf("nextDynamicExecuteAction() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDynamicFindPayloadIncludesAction_AcceptsJSONAndMarkdownShapes verifies
// the find-result check recognizes the structured results array as well as the
// backticked and inline-JSON text renderings.
func TestDynamicFindPayloadIncludesAction_AcceptsJSONAndMarkdownShapes(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    bool
	}{
		{name: "structured results", payload: `{"results":[{"id":"project.get"}]}`, want: true},
		{name: "structured mismatch", payload: `{"results":[{"id":"issue.list"}]}`, want: false},
		{name: "backticked text", payload: "matched `project.get` for you", want: true},
		{name: "inline json text", payload: `prefix "id":"project.get" suffix`, want: true},
		{name: "absent", payload: "no matches", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dynamicFindPayloadIncludesAction(tc.payload, actionProjectGet); got != tc.want {
				t.Fatalf("dynamicFindPayloadIncludesAction(%q) = %t, want %t", tc.payload, got, tc.want)
			}
		})
	}
}

// TestDynamicFindExchangeIncludesAction_RequiresStructuredResponse verifies the
// MCP exchange check tolerates a missing or unparseable response.
func TestDynamicFindExchangeIncludesAction_RequiresStructuredResponse(t *testing.T) {
	cases := []struct {
		name     string
		exchange *traceMCPExchange
		want     bool
	}{
		{name: "nil exchange"},
		{name: "empty response", exchange: &traceMCPExchange{}},
		{name: "invalid json", exchange: &traceMCPExchange{Response: []byte("not json")}},
		{name: "matching action", exchange: &traceMCPExchange{Response: []byte(`{"structuredContent":{"results":[{"id":"project.get"}]}}`)}, want: true},
		{name: "other action", exchange: &traceMCPExchange{Response: []byte(`{"structuredContent":{"results":[{"id":"issue.list"}]}}`)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dynamicFindExchangeIncludesAction(tc.exchange, actionProjectGet); got != tc.want {
				t.Fatalf("dynamicFindExchangeIncludesAction() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestExecutionErrorKindAndBadParam_ClassifyGitLabFailures verifies live
// execution failures map to their repair kinds, and that the role-confusion
// kind is the only one that names the ambiguous parameters.
func TestExecutionErrorKindAndBadParam_ClassifyGitLabFailures(t *testing.T) {
	dynamicStep := func(action string) evalStep {
		return evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: action}
	}
	cases := []struct {
		name         string
		step         evalStep
		err          error
		wantKind     string
		wantBadParam string
	}{
		{name: "role confusion", step: dynamicStep(actionIssueLinkCreate), err: errors.New("400 Bad Request"), wantKind: "gitlab_bad_request_role_confusion", wantBadParam: "project_id,issue_iid,target_project_id,target_issue_iid"},
		{name: "role confusion token scope", step: dynamicStep("job.token_scope_remove_project"), err: errors.New("400 Bad Request"), wantKind: "gitlab_bad_request_role_confusion", wantBadParam: "project_id,target_project_id"},
		{name: "role confusion merge request", step: dynamicStep("merge_request.create"), err: errors.New("400"), wantKind: "gitlab_bad_request_role_confusion", wantBadParam: "source_branch,target_branch"},
		{name: "role confusion without mapping", step: dynamicStep("merge_request.approve"), err: errors.New("400"), wantKind: "gitlab_bad_request"},
		{name: "plain bad request", step: evalStep{ExpectedTool: "gitlab_project", ExpectedAction: "get"}, err: errors.New("400 bad request"), wantKind: "gitlab_bad_request"},
		{name: "not found", step: evalStep{}, err: errors.New("404 Project Not Found"), wantKind: "gitlab_not_found"},
		{name: "forbidden", step: evalStep{}, err: errors.New("403 Forbidden"), wantKind: "gitlab_forbidden"},
		{name: "other", step: evalStep{}, err: errors.New("connection reset"), wantKind: "mcp_execution_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := executionErrorKind(tc.step, tc.err); got != tc.wantKind {
				t.Fatalf("executionErrorKind() = %q, want %q", got, tc.wantKind)
			}
			if got := executionErrorBadParam(tc.step, tc.err); got != tc.wantBadParam {
				t.Fatalf("executionErrorBadParam() = %q, want %q", got, tc.wantBadParam)
			}
		})
	}
}

// TestToolExecutionNote_SimulatedStep_UsesPlainText verifies a simulated step
// records a readable note while a live execution failure records the JSON
// repair payload.
func TestToolExecutionNote_SimulatedStep_UsesPlainText(t *testing.T) {
	simulated := toolExecutionNote(2, evalStep{Simulation: "transient_error_once"}, errors.New("simulated 503"))
	if simulated != "step 2 simulation transient_error_once: simulated 503" {
		t.Fatalf("toolExecutionNote(simulated) = %q, want plain simulation note", simulated)
	}
	live := toolExecutionNote(1, evalStep{ExpectedTool: "gitlab_project", ExpectedAction: "get"}, errors.New("404 not found"))
	var payload repairPayload
	if err := json.Unmarshal([]byte(live), &payload); err != nil {
		t.Fatalf("unmarshal live note %q: %v", live, err)
	}
	if payload.ErrorKind != "gitlab_not_found" || !payload.RetryAllowed || payload.FailedAction != "get" {
		t.Fatalf("payload = %+v, want gitlab_not_found repair payload", payload)
	}
}

// TestIntFromAny_ConvertsJSONNumericShapes verifies each numeric encoding the
// providers emit converts to int, and anything else falls back.
func TestIntFromAny_ConvertsJSONNumericShapes(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  int
	}{
		{name: "int", value: 7, want: 7},
		{name: "int64", value: int64(8), want: 8},
		{name: "float64", value: float64(9), want: 9},
		{name: "json number", value: json.Number("10"), want: 10},
		{name: "invalid json number", value: json.Number("abc"), want: 20},
		{name: "string", value: "12", want: 20},
		{name: "nil", value: nil, want: 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := intFromAny(tc.value, 20); got != tc.want {
				t.Fatalf("intFromAny(%#v) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}

// TestSchemaLookupAlias_MapsLegacyToolsOntoUnifiedActions verifies a legacy
// meta-tool schema request is rewritten onto the unified dispatcher when it
// carries the action, filtered to that domain when it does not, and passed
// through when there is no dispatcher or no matching action.
func TestSchemaLookupAlias_MapsLegacyToolsOntoUnifiedActions(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab":         {"project.get": projectGetRoute(), "project.list": {}, "issue.list": {}},
		"gitlab_project": {"get": projectGetRoute()},
	}
	cases := []struct {
		name       string
		routes     map[string]toolutil.ActionMap
		tool       string
		action     string
		wantTool   string
		wantAction string
		wantKeys   []string
	}{
		{name: "no dispatcher", routes: map[string]toolutil.ActionMap{"gitlab_project": {"get": {}}}, tool: "gitlab_project", action: "get", wantTool: "gitlab_project", wantAction: "get"},
		{name: "dispatcher itself", routes: routes, tool: "gitlab", action: "project.get", wantTool: "gitlab", wantAction: "project.get"},
		{name: "server tool", routes: routes, tool: "gitlab_server", action: "schema_index", wantTool: "gitlab_server", wantAction: "schema_index"},
		{name: "legacy with action", routes: routes, tool: "gitlab_project", action: "get", wantTool: "gitlab", wantAction: "project.get"},
		{name: "legacy unknown action", routes: routes, tool: "gitlab_project", action: "nope", wantTool: "gitlab_project", wantAction: "nope"},
		{name: "legacy index", routes: routes, tool: "gitlab_project", wantTool: "gitlab_project", wantKeys: []string{"get", "list"}},
		{name: "legacy index without matches", routes: routes, tool: "gitlab_nothing", wantTool: "gitlab_nothing"},
		{name: "non prefixed tool", routes: routes, tool: "custom", action: "get", wantTool: "custom", wantAction: "get"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookupRoutes, lookupTool, lookupAction := schemaLookupAlias(tc.routes, tc.tool, tc.action)
			if lookupTool != tc.wantTool || lookupAction != tc.wantAction {
				t.Fatalf("schemaLookupAlias() = %s/%s, want %s/%s", lookupTool, lookupAction, tc.wantTool, tc.wantAction)
			}
			if len(tc.wantKeys) == 0 {
				return
			}
			for _, key := range tc.wantKeys {
				if _, ok := lookupRoutes[lookupTool][key]; !ok {
					t.Fatalf("filtered routes = %v, want key %q", lookupRoutes[lookupTool], key)
				}
			}
		})
	}
}

// TestSchemaLookupResult_RejectsUnknownActionsAndSchemas verifies the schema
// lookup surface reports an unsupported action, an unknown tool for
// schema_index, and an unknown action for schema_get.
func TestSchemaLookupResult_RejectsUnknownActionsAndSchemas(t *testing.T) {
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	cases := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{name: "unsupported action", input: map[string]any{"action": "nope"}, want: `unsupported schema action "nope"`},
		{name: "unknown index tool", input: map[string]any{"action": "schema_index", "params": map[string]any{"tool": "gitlab_missing"}}, want: `schema_index: unknown tool "gitlab_missing"`},
		{name: "unknown get action", input: map[string]any{"action": "schema_get", "params": map[string]any{"tool": "gitlab_project", "action": "nope"}}, want: `schema_get: unknown action "nope"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := schemaLookupResult(routes, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("schemaLookupResult() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestSchemaLookupResult_ToolOnlySchemaGet_ReturnsDiscoveryIndex verifies a
// schema_get without an action returns that tool's discovery index.
func TestSchemaLookupResult_ToolOnlySchemaGet_ReturnsDiscoveryIndex(t *testing.T) {
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	payload, err := schemaLookupResult(routes, map[string]any{"action": "schema_get", "params": map[string]any{"tool": "gitlab_project"}})
	if err != nil {
		t.Fatalf("schemaLookupResult() error = %v", err)
	}
	if !strings.Contains(payload, "get") {
		t.Fatalf("payload = %s, want the tool's action index", payload)
	}
}

// TestDynamicDiscoveryResult_UnsupportedTool_ReturnsError verifies only the
// dynamic find tool is served by the simulated discovery path.
func TestDynamicDiscoveryResult_UnsupportedTool_ReturnsError(t *testing.T) {
	_, err := dynamicDiscoveryResult(t.Context(), nil, modelContentBlock{Name: "gitlab_project"})
	if err == nil || !strings.Contains(err.Error(), `unsupported dynamic discovery tool "gitlab_project"`) {
		t.Fatalf("dynamicDiscoveryResult() error = %v, want unsupported tool error", err)
	}
}

// TestDynamicFindResultWithRegistry_EmptyRegistry_ReturnsZeroResults verifies
// a find against an empty registry yields a serializable zero-result payload
// rather than failing the attempt.
func TestDynamicFindResultWithRegistry_EmptyRegistry_ReturnsZeroResults(t *testing.T) {
	registry := dynamictools.NewRegistry(nil)
	payload, err := marshalToolResult(dynamicFindResultWithRegistry(t.Context(), registry, "project get", 3))
	if err != nil {
		t.Fatalf("marshalToolResult() error = %v", err)
	}
	if !strings.Contains(payload, `"count":0`) {
		t.Fatalf("payload = %s, want zero results", payload)
	}
}

// TestProjectPathFromRemoteURL_HandlesEveryRemoteShape verifies HTTPS, SSH and
// bare paths all reduce to the namespace path.
func TestProjectPathFromRemoteURL_HandlesEveryRemoteShape(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{name: "https", url: "https://gitlab.example.com/my-org/app.git", want: "my-org/app"},
		{name: "ssh", url: "git@gitlab.example.com:my-org/app.git", want: "my-org/app"},
		{name: "scheme without path", url: "https://gitlab.example.com", want: "//gitlab.example.com"},
		{name: "bare path", url: "my-org/app", want: "my-org/app"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := projectPathFromRemoteURL(tc.url); got != tc.want {
				t.Fatalf("projectPathFromRemoteURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// TestProjectNameFromPath_TakesLastSegment verifies the project name is the
// final path segment, with a stable default for an empty path.
func TestProjectNameFromPath_TakesLastSegment(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{path: "my-org/tools/app", want: "app"},
		{path: "/", want: "gitlab-mcp-server"},
		{path: "app", want: "app"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := projectNameFromPath(tc.path); got != tc.want {
				t.Fatalf("projectNameFromPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestSimulatedProjectPath_PrefersParamsThenInput verifies the simulated
// project path is read from params before input, resolves remote URLs, ignores
// values without a namespace separator, and defaults to the fixture project.
func TestSimulatedProjectPath_PrefersParamsThenInput(t *testing.T) {
	cases := []struct {
		name   string
		input  map[string]any
		params map[string]any
		want   string
	}{
		{name: "params project id", params: map[string]any{"project_id": "my-org/app"}, want: "my-org/app"},
		{name: "params remote url", params: map[string]any{"remote_url": "https://gitlab.example.com/my-org/app.git"}, want: "my-org/app"},
		{name: "input fallback", input: map[string]any{"full_path": "my-org/other"}, want: "my-org/other"},
		{name: "git suffix", params: map[string]any{"search": "my-org/app.git"}, want: "my-org/app"},
		{name: "no separator", params: map[string]any{"query": "app"}, want: liveFixtureProjectPath},
		{name: "empty", want: liveFixtureProjectPath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := simulatedProjectPath(tc.input, tc.params); got != tc.want {
				t.Fatalf("simulatedProjectPath() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAddSimulatedResourceIDs_InjectsIDsForCreateActions verifies every
// create-style action seeds the identifier downstream steps reuse.
func TestAddSimulatedResourceIDs_InjectsIDsForCreateActions(t *testing.T) {
	params := map[string]any{"project_id": "my-org/app", "group_id": "my-org", "issue_iid": 3, "merge_request_iid": 4, "tag_name": "v1"}
	cases := []struct {
		action  string
		wantKey string
		wantID  int
		object  string
	}{
		{action: actionIssueCreate, wantKey: "issue_iid", wantID: 123, object: "issue"},
		{action: actionIssueLinkCreate, wantKey: "issue_link_id", wantID: 124, object: "issue_link"},
		{action: "pipeline.trigger_create", wantKey: "trigger_id", wantID: 119, object: "trigger"},
		{action: "release.link_create", wantKey: "link_id", wantID: 121, object: "link"},
		{action: "group.group_label_create", wantKey: "label_id", wantID: 120, object: "label"},
		{action: "admin.broadcast_message_create", wantKey: "id", wantID: 125, object: "broadcast_message"},
		{action: "project.hook_add", wantKey: "hook_id", wantID: 101, object: "hook"},
		{action: "project.badge_add", wantKey: "badge_id", wantID: 102, object: "badge"},
		{action: "snippet.project_create", wantKey: "snippet_id", wantID: 103, object: "snippet"},
		{action: "mr_review.note_create", wantKey: "note_id", wantID: 104, object: "note"},
		{action: "mr_review.draft_note_create", wantKey: "note_id", wantID: 104, object: "note"},
		{action: "access.deploy_token_create_project", wantKey: "deploy_token_id", wantID: 105, object: "deploy_token"},
		{action: "access.deploy_key_add", wantKey: "deploy_key_id", wantID: 106, object: "deploy_key"},
		{action: "project.member_add", wantKey: "user_id", wantID: 107, object: "member"},
		{action: "group.group_milestone_create", wantKey: "milestone_iid", wantID: 108, object: "milestone"},
		{action: "pipeline.schedule_create", wantKey: "schedule_id", wantID: 109, object: "schedule"},
		{action: "merge_request.emoji_mr_create", wantKey: "award_id", wantID: 110, object: "award"},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			result := map[string]any{}
			addSimulatedResourceIDs(result, tc.action, params)
			if result[tc.wantKey] != tc.wantID || result["id"] != tc.wantID {
				t.Fatalf("result = %#v, want %s = %d", result, tc.wantKey, tc.wantID)
			}
			if _, ok := result[tc.object].(map[string]any); !ok {
				t.Fatalf("result = %#v, want %s object", result, tc.object)
			}
		})
	}
	t.Run("wiki.create", func(t *testing.T) {
		result := map[string]any{}
		addSimulatedResourceIDs(result, "wiki.create", map[string]any{"project_id": "my-org/app"})
		wiki, ok := result["wiki"].(map[string]any)
		if !ok || wiki["slug"] != "eval-wiki-page" {
			t.Fatalf("result = %#v, want default wiki slug", result)
		}
	})
	t.Run("unknown action", func(t *testing.T) {
		result := map[string]any{}
		addSimulatedResourceIDs(result, "project.get", params)
		if len(result) != 0 {
			t.Fatalf("result = %#v, want no injected IDs", result)
		}
	})
}

// TestSnippetFilePathFromParams_PrefersFileNameThenFiles verifies the
// simulated snippet path comes from file_name, then the first files entry,
// then a stable default.
func TestSnippetFilePathFromParams_PrefersFileNameThenFiles(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{name: "file name", params: map[string]any{"file_name": "notes.md"}, want: "notes.md"},
		{name: "files entry", params: map[string]any{"files": []any{map[string]any{"file_path": "src/a.go"}}}, want: "src/a.go"},
		{name: "malformed files", params: map[string]any{"files": []any{"nope", map[string]any{}}}, want: "snippet.txt"},
		{name: "empty", params: map[string]any{}, want: "snippet.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := snippetFilePathFromParams(tc.params); got != tc.want {
				t.Fatalf("snippetFilePathFromParams() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestApplyActionSimulation_ShapesResultsPerAction verifies every simulated
// read action returns the collection or object downstream steps read, and an
// unsimulated action reports that it was not handled.
func TestApplyActionSimulation_ShapesResultsPerAction(t *testing.T) {
	params := map[string]any{"project_id": "my-org/app", "group_id": "my-org", "pipeline_id": 12, "tag_name": "v1", "file_path": "a.txt", "ref": "main", "package_name": "pkg", "package_version": "1.0"}
	cases := []struct {
		action  string
		wantKey string
	}{
		{action: actionProjectGet, wantKey: "project"},
		{action: actionProjectList, wantKey: "projects"},
		{action: "pipeline.trigger_list", wantKey: "triggers"},
		{action: "package.publish_directory", wantKey: "published"},
		{action: "publish_directory", wantKey: "published"},
		{action: "group.group_label_list", wantKey: "labels"},
		{action: "wiki.list", wantKey: "pages"},
		{action: "release.get", wantKey: "release"},
		{action: "environment.list", wantKey: "environments"},
		{action: actionEnvironmentProtectedList, wantKey: "protected_environments"},
		{action: "repository.file_get", wantKey: "file"},
		{action: actionPipelineGet, wantKey: "pipeline"},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			result := map[string]any{}
			if !applyActionSimulation(result, modelContentBlock{Input: map[string]any{"params": params}}, tc.action, params) {
				t.Fatalf("applyActionSimulation(%s) = false, want handled", tc.action)
			}
			if result[tc.wantKey] == nil {
				t.Fatalf("result = %#v, want key %q", result, tc.wantKey)
			}
		})
	}
	t.Run("unsimulated action", func(t *testing.T) {
		result := map[string]any{}
		if applyActionSimulation(result, modelContentBlock{}, "issue.list", params) {
			t.Fatalf("applyActionSimulation(issue.list) = true, want unhandled")
		}
	})
}

// TestSimulatedPackageDirectoryItems_DefaultsPackageCoordinates verifies the
// simulated publish list falls back to the fixture package name, version and
// project when the call omits them, and escapes them into the file URLs.
func TestSimulatedPackageDirectoryItems_DefaultsPackageCoordinates(t *testing.T) {
	items := simulatedPackageDirectoryItems(map[string]any{})
	if len(items) != len(packageReleaseFixtureFiles) {
		t.Fatalf("items = %d, want %d", len(items), len(packageReleaseFixtureFiles))
	}
	url, _ := items[0]["url"].(string)
	if !strings.Contains(url, "my-org%2Ftools%2Fgitlab-mcp-server") || !strings.Contains(url, liveFixturePackageReleaseName) {
		t.Fatalf("url = %q, want escaped fixture coordinates", url)
	}
	custom := simulatedPackageDirectoryItems(map[string]any{"project_id": "a/b", "package_name": "pkg", "package_version": "9.9"})
	customURL, _ := custom[0]["url"].(string)
	if !strings.Contains(customURL, "projects/a%2Fb/packages/generic/pkg/9.9") {
		t.Fatalf("url = %q, want supplied coordinates", customURL)
	}
}

// TestRecordRealStepContent_OnlyRecordsLiveSteps verifies real tool output is
// captured for live steps and skipped without a session, for simulated steps,
// and for a nil state.
func TestRecordRealStepContent_OnlyRecordsLiveSteps(t *testing.T) {
	simulation := simulationResult{Content: "real output"}
	t.Run("no session", func(t *testing.T) {
		state := &modelEvaluationState{}
		(&modelRunner{}).recordRealStepContent(evalStep{}, state, 1, simulation)
		if len(state.realStepContent) != 0 {
			t.Fatalf("realStepContent = %v, want empty without a session", state.realStepContent)
		}
	})
	t.Run("simulated step", func(t *testing.T) {
		runner := &modelRunner{mcpSession: newProjectGetSession(t)}
		state := &modelEvaluationState{}
		runner.recordRealStepContent(evalStep{Simulation: "transient_error_once"}, state, 1, simulation)
		if len(state.realStepContent) != 0 {
			t.Fatalf("realStepContent = %v, want empty for a simulated step", state.realStepContent)
		}
	})
	t.Run("nil state", func(t *testing.T) {
		runner := &modelRunner{mcpSession: newProjectGetSession(t)}
		runner.recordRealStepContent(evalStep{}, nil, 1, simulation)
	})
	t.Run("live step", func(t *testing.T) {
		runner := &modelRunner{mcpSession: newProjectGetSession(t)}
		state := &modelEvaluationState{}
		runner.recordRealStepContent(evalStep{}, state, 2, simulation)
		if state.realStepContent[2] != "real output" {
			t.Fatalf("realStepContent = %v, want step 2 recorded", state.realStepContent)
		}
	})
}

// TestIsTransientMCPToolResult_DetectsGitRefContention verifies only the
// known transient git reference failures are retried.
func TestIsTransientMCPToolResult_DetectsGitRefContention(t *testing.T) {
	cases := []struct {
		name   string
		result simulationResult
		want   bool
	}{
		{name: "no error", result: simulationResult{Content: "packed-refs locked"}},
		{name: "packed refs", result: simulationResult{Content: "packed-refs locked", Err: errors.New("boom")}, want: true},
		{name: "reference update", result: simulationResult{Err: errors.New("Reference update failed")}, want: true},
		{name: "tag removal", result: simulationResult{Err: errors.New("Failed to remove tag")}, want: true},
		{name: "other error", result: simulationResult{Err: errors.New("404 not found")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientMCPToolResult(tc.result); got != tc.want {
				t.Fatalf("isTransientMCPToolResult() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestSetResponse_RecordsMarshalFailureAsText verifies an MCP result that
// cannot be marshaled records a diagnostic instead of a raw response, and a
// nil exchange or result is ignored.
func TestSetResponse_RecordsMarshalFailureAsText(t *testing.T) {
	var nilExchange *traceMCPExchange
	nilExchange.setResponse(&mcp.CallToolResult{})
	exchange := &traceMCPExchange{}
	exchange.setResponse(nil)
	if exchange.Response != nil || exchange.ResponseText != "" {
		t.Fatalf("exchange = %+v, want untouched for a nil result", exchange)
	}
	exchange.setResponse(&mcp.CallToolResult{IsError: true, StructuredContent: make(chan int)})
	if !exchange.IsError || !strings.Contains(exchange.ResponseText, "marshal MCP result") {
		t.Fatalf("exchange = %+v, want marshal failure recorded as text", exchange)
	}
}

// TestPreparedCaseFromTask_UsesTaskCaseAndSteps verifies a prepared case is
// synthesized from a bare task and keeps the task's own prompt over the case
// prompt. A task carrying only case steps still yields one synthesized step,
// because taskSteps always produces at least one from the task's own fields.
func TestPreparedCaseFromTask_UsesTaskCaseAndSteps(t *testing.T) {
	cases := []struct {
		name       string
		task       evalTask
		wantPrompt string
		wantSteps  int
		wantID     string
	}{
		{name: "bare task", task: evalTask{ID: "MT-1", Prompt: "p", ExpectedTool: "gitlab_project", ExpectedAction: "get"}, wantPrompt: "p", wantSteps: 1, wantID: "MT-1"},
		{name: "task prompt overrides case", task: evalTask{ID: "MT-2", Prompt: "task prompt", Case: &EvalCase{ID: "MT-2", Prompt: "case prompt", Steps: []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}}}}, wantPrompt: "task prompt", wantSteps: 1, wantID: "MT-2"},
		{name: "case without task steps", task: evalTask{ID: "MT-3", Case: &EvalCase{ID: "MT-3", Prompt: "case prompt", Steps: []ExpectedStep{{ExpectedTool: "gitlab_issue", ExpectedAction: "list"}, {ExpectedTool: "gitlab_issue", ExpectedAction: "get"}}}}, wantPrompt: "case prompt", wantSteps: 1, wantID: "MT-3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prepared := preparedCaseFromTask(tc.task)
			if prepared.Prompt != tc.wantPrompt || len(prepared.Steps) != tc.wantSteps || string(prepared.Case.ID) != tc.wantID {
				t.Fatalf("preparedCaseFromTask() = %+v, want prompt %q with %d steps", prepared, tc.wantPrompt, tc.wantSteps)
			}
		})
	}
}

// TestAcceptDirectDynamicExecuteStep_RejectsUnacceptableShapes verifies the
// direct-execute shortcut is declined when the call already validated, the
// index is out of range, the current step is not a find, the scenario has no
// following execute step, or the call does not match that next step.
func TestAcceptDirectDynamicExecuteStep_RejectsUnacceptableShapes(t *testing.T) {
	routes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {actionProjectGet: projectGetRoute()}}
	execute := modelContentBlock{Name: dynamicExecuteActionTool, Input: map[string]any{"action": actionProjectGet, "params": map[string]any{"project_id": "my-org/app"}}}
	findThenExecute := []evalStep{{ExpectedTool: dynamicFindTool}, {ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, RequiredParams: []string{"project_id"}}}
	cases := []struct {
		name       string
		steps      []evalStep
		index      int
		validation validationResult
		toolUse    modelContentBlock
		want       bool
	}{
		{name: "already valid", steps: findThenExecute, validation: validationResult{Valid: true}, toolUse: execute},
		{name: "index out of range", steps: findThenExecute, index: 9, toolUse: execute},
		{name: "current step not a find", steps: []evalStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet}}, toolUse: execute},
		{name: "no following step", steps: []evalStep{{ExpectedTool: dynamicFindTool}}, toolUse: execute},
		{name: "next step not execute", steps: []evalStep{{ExpectedTool: dynamicFindTool}, {ExpectedTool: resourceListTool}}, toolUse: execute},
		{name: "next step mismatch", steps: findThenExecute, toolUse: modelContentBlock{Name: dynamicExecuteActionTool, Input: map[string]any{"action": "issue.list"}}},
		{name: "accepted", steps: findThenExecute, toolUse: execute, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := &modelEvaluationState{stepIndex: tc.index}
			_, _, accepted := acceptDirectDynamicExecuteStep(tc.steps, tc.index, tc.toolUse, tc.validation, routes, state)
			if accepted != tc.want {
				t.Fatalf("acceptDirectDynamicExecuteStep() accepted = %t, want %t", accepted, tc.want)
			}
		})
	}
}

// TestAcceptOptionalBridgeStep_RejectsNonSkippableShapes verifies the optional
// bridge skip only applies to an optional bridge step followed by another
// bridge step that the call actually satisfies.
func TestAcceptOptionalBridgeStep_RejectsNonSkippableShapes(t *testing.T) {
	toolUse := modelContentBlock{Name: resourceListTool, Input: map[string]any{}}
	cases := []struct {
		name       string
		steps      []evalStep
		validation validationResult
		want       bool
	}{
		{name: "step not optional", steps: []evalStep{{ExpectedTool: capabilityListTool}, {ExpectedTool: resourceListTool}}},
		{name: "already valid", steps: []evalStep{{ExpectedTool: capabilityListTool, OptionalStep: true}, {ExpectedTool: resourceListTool}}, validation: validationResult{Valid: true}},
		{name: "no next step", steps: []evalStep{{ExpectedTool: capabilityListTool, OptionalStep: true}}},
		{name: "next not a bridge step", steps: []evalStep{{ExpectedTool: capabilityListTool, OptionalStep: true}, {ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet}}},
		{name: "next mismatch", steps: []evalStep{{ExpectedTool: capabilityListTool, OptionalStep: true}, {ExpectedTool: promptListTool}}},
		{name: "accepted", steps: []evalStep{{ExpectedTool: capabilityListTool, OptionalStep: true}, {ExpectedTool: resourceListTool}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bridgeCtx := capabilityBridgeStepContext{steps: tc.steps, toolUse: toolUse, result: &taskResult{}, state: &modelEvaluationState{}}
			_, _, accepted := bridgeCtx.acceptOptionalBridgeStep(tc.steps[0], tc.validation)
			if accepted != tc.want {
				t.Fatalf("acceptOptionalBridgeStep() = %t, want %t", accepted, tc.want)
			}
		})
	}
}

// TestIsReadOnlyUnexpectedAction_ClassifiesLeafActions verifies the harmless
// read-only classification covers the named leaves, the get/list prefixes and
// suffixes, and rejects mutating leaves.
func TestIsReadOnlyUnexpectedAction_ClassifiesLeafActions(t *testing.T) {
	cases := []struct {
		action string
		want   bool
	}{
		{action: "user.current", want: true},
		{action: "gitlab_server.health_check", want: true},
		{action: "job.trace", want: true},
		{action: "search.projects", want: true},
		{action: "project.get", want: true},
		{action: "issue.list", want: true},
		{action: "project.get_protected", want: true},
		{action: "project.list_hooks", want: true},
		{action: "project.hook_get", want: true},
		{action: "project.hook_list", want: true},
		{action: "issue.create", want: false},
		{action: "project.delete", want: false},
		{action: "nodot", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			if got := isReadOnlyUnexpectedAction(tc.action); got != tc.want {
				t.Fatalf("isReadOnlyUnexpectedAction(%s) = %t, want %t", tc.action, got, tc.want)
			}
		})
	}
}

// TestToolResultBlock_UsesErrorTextWhenContentIsEmpty verifies an errored tool
// result block is flagged and falls back to the error text only when no
// content was produced.
func TestToolResultBlock_UsesErrorTextWhenContentIsEmpty(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		err         error
		wantContent string
		wantIsError bool
	}{
		{name: "success", content: "ok", wantContent: "ok"},
		{name: "error with content", content: "detail", err: errors.New("boom"), wantContent: "detail", wantIsError: true},
		{name: "error without content", err: errors.New("boom"), wantContent: "boom", wantIsError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block := toolResultBlock("id", tc.content, tc.err)
			if block.Content != tc.wantContent || block.IsError != tc.wantIsError || block.ToolUseID != "id" {
				t.Fatalf("toolResultBlock() = %+v, want content %q error %t", block, tc.wantContent, tc.wantIsError)
			}
		})
	}
}

// TestMarshalToolResult_UnmarshalableValue_ReturnsError verifies a value the
// JSON encoder rejects surfaces as a marshal error.
func TestMarshalToolResult_UnmarshalableValue_ReturnsError(t *testing.T) {
	if _, err := marshalToolResult(make(chan int)); err == nil || !strings.Contains(err.Error(), "marshal tool result") {
		t.Fatalf("marshalToolResult() error = %v, want marshal failure", err)
	}
}

// TestInvalidToolUseFingerprint_UnmarshalableInput_FallsBackToToolName
// verifies a tool call whose input cannot be marshaled still yields a stable
// fingerprint.
func TestInvalidToolUseFingerprint_UnmarshalableInput_FallsBackToToolName(t *testing.T) {
	got := invalidToolUseFingerprint(modelContentBlock{Name: "gitlab_project", Input: map[string]any{"bad": make(chan int)}})
	if got != "gitlab_project" {
		t.Fatalf("invalidToolUseFingerprint() = %q, want tool name fallback", got)
	}
}

// TestCall_RetriesUntilBudgetExhausted verifies a retryable provider failure
// is retried up to the configured budget and the last error is returned.
func TestCall_RetriesUntilBudgetExhausted(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	var attempts int
	runner := &modelRunner{
		apiKey:  "test-key",
		model:   "m",
		retries: 2,
		client: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"type":"rate_limit","message":"slow down"}}`))}, nil
		})},
	}
	_, err := runner.call(t.Context(), "system", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "status 429") {
		t.Fatalf("call() error = %v, want rate limit error", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3 (initial plus two retries)", attempts)
	}
}

// TestCall_ContextCanceledDuringRetryWait_ReturnsContextError verifies a
// canceled context between retries stops the loop with the context error.
func TestCall_ContextCanceledDuringRetryWait_ReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	runner := &modelRunner{
		apiKey:    "test-key",
		model:     "m",
		retries:   1,
		retryWait: time.Hour,
		client: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			cancel()
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		})},
	}
	_, err := runner.call(ctx, "system", nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("call() error = %v, want context.Canceled", err)
	}
}

// TestLookupToolResult_DynamicFindWithoutSession_UsesSimulatedRegistry
// verifies the schema/discovery lookup falls back to the simulated registry
// when no MCP session is available, and answers meta schema lookups directly.
func TestLookupToolResult_DynamicFindWithoutSession_UsesSimulatedRegistry(t *testing.T) {
	routes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {actionProjectGet: projectGetRoute()}}
	runner := &modelRunner{}
	payload, exchange, err := runner.lookupToolResult(t.Context(), routes, modelContentBlock{Name: dynamicFindTool, Input: map[string]any{"query": "project get"}}, true)
	if err != nil || exchange != nil || !strings.Contains(payload, actionProjectGet) {
		t.Fatalf("lookupToolResult(dynamic) = %q, %+v, %v; want simulated find result", payload, exchange, err)
	}
	metaRoutes := map[string]toolutil.ActionMap{"gitlab_project": {"get": projectGetRoute()}}
	payload, exchange, err = runner.lookupToolResult(t.Context(), metaRoutes, modelContentBlock{Name: "gitlab_server", Input: map[string]any{"action": "schema_index"}}, false)
	if err != nil || exchange != nil || !strings.Contains(payload, "gitlab_project") {
		t.Fatalf("lookupToolResult(schema) = %q, %+v, %v; want schema index", payload, exchange, err)
	}
}
