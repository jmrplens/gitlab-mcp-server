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
		if !strings.Contains(content, want) {
			t.Fatalf("find result = %s, want %q", content, want)
		}
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
		if !strings.Contains(find, want) {
			t.Fatalf("find result = %s, want %q", find, want)
		}
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
		if !strings.Contains(content, want) {
			t.Fatalf("successfulSimulatedToolContent(prelude) = %s, want %q", content, want)
		}
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
		if !traceHasKind(result.Trace, kind) {
			t.Fatalf("trace events = %+v, want kind %s", result.Trace.Events, kind)
		}
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

// toolUseResponse converts the GitLab API response to the tool output format.
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
