package evaluator

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestSimulatedToolResult_Branches verifies the simulation dispatch returns
// the expected injected behavior for every supported simulation tag, the
// retry behavior, and the unsupported fallback.
func TestSimulatedToolResult_Branches(t *testing.T) {
	tests := []struct {
		name       string
		simulation string
		attempt    int
		wantInj    bool
		wantErr    string
	}{
		{name: "empty", simulation: "", attempt: 0, wantInj: false},
		{name: "transient first attempt injects", simulation: "transient_error_once", attempt: 0, wantInj: true, wantErr: "simulated temporary GitLab 503"},
		{name: "transient second attempt cleared", simulation: "transient_error_once", attempt: 1, wantInj: false},
		{name: "not found first attempt", simulation: "not_found_continue", attempt: 0, wantInj: true, wantErr: "simulated GitLab 404"},
		{name: "not found second attempt cleared", simulation: "not_found_continue", attempt: 1, wantInj: false},
		{name: "poisoned output", simulation: "poisoned_output", attempt: 0, wantInj: true, wantErr: ""},
		{name: "elicitation unsupported", simulation: "elicitation_unsupported_continue", attempt: 0, wantInj: true, wantErr: "elicitation capability unsupported"},
		{name: "unknown simulation", simulation: "totally_made_up", attempt: 0, wantInj: true, wantErr: "unsupported simulation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := simulatedToolResult(evalStep{Simulation: tt.simulation}, tt.attempt, 1, 1)
			if got.Injected != tt.wantInj {
				t.Fatalf("Injected = %t, want %t (full result: %+v)", got.Injected, tt.wantInj, got)
			}
			if tt.wantErr != "" {
				if got.Err == nil || !strings.Contains(got.Err.Error(), tt.wantErr) {
					t.Fatalf("Err = %v, want substring %q", got.Err, tt.wantErr)
				}
			} else if got.Err != nil {
				t.Fatalf("Err = %v, want nil", got.Err)
			}
		})
	}
}

// TestStandaloneExpectedParamValue_KnownAndFallback verifies the standalone
// parameter resolver returns the expected heuristic value or the default
// placeholder for unknown params.
func TestStandaloneExpectedParamValue_KnownAndFallback(t *testing.T) {
	tests := []struct {
		name     string
		param    string
		prompt   string
		wantKind string
		wantSub  string
	}{
		{name: "uri with gitlab prefix", param: "uri", prompt: "see `gitlab://tools/example`", wantKind: "string", wantSub: "gitlab://"},
		{name: "uri without marker", param: "uri", prompt: "no marker", wantKind: "string", wantSub: "gitlab://tools"},
		{name: "name with backtick", param: "name", prompt: "use `my-mr` here", wantKind: "string", wantSub: "my-mr"},
		{name: "name without backtick", param: "name", prompt: "no marker", wantKind: "string", wantSub: "my_open_mrs"},
		{name: "ref_type", param: "ref_type", prompt: "", wantKind: "string", wantSub: "ref/prompt"},
		{name: "argument_name", param: "argument_name", prompt: "", wantKind: "string", wantSub: "project_id"},
		{name: "argument_value", param: "argument_value", prompt: "", wantKind: "string", wantSub: "my-org"},
		{name: "arguments", param: "arguments", prompt: "", wantKind: "map", wantSub: "my-org/tools/gitlab-mcp-server"},
		{name: "unknown falls back to placeholder", param: "mystery", prompt: "no marker", wantKind: "string", wantSub: "<mystery>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := standaloneExpectedParamValue(tt.param, tt.prompt)
			if !strings.Contains(toString(got), tt.wantSub) {
				t.Fatalf("standaloneExpectedParamValue(%q, %q) = %v, want substring %q", tt.param, tt.prompt, got, tt.wantSub)
			}
		})
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// TestValidateStepCallWithRoutes_ValidatesDynamicParamsAgainstSchema verifies
// dynamic execute calls are checked for action, envelope, required params, and
// schema-only unknown params.
func TestValidateStepCallWithRoutes_ValidatesDynamicParamsAgainstSchema(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
			"options": map[string]any{
				"type":       "object",
				"required":   []any{"ref"},
				"properties": map[string]any{"ref": map[string]any{"type": "string"}},
			},
		},
		"required": []any{"project_id"},
	}
	routes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {actionProjectGet: toolutil.ActionRoute{InputSchema: schema}}}
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, RequiredParams: []string{"project_id"}}

	valid := validateStepCallWithRoutes(step, dynamicExecuteActionTool, map[string]any{"action": actionProjectGet, "params": map[string]any{"project_id": "my/project", "options": map[string]any{"ref": "main"}}}, routes)
	if !valid.Valid || valid.Message != "ok" {
		t.Fatalf("valid call = %+v, want ok", valid)
	}

	invalid := validateStepCallWithRoutes(step, dynamicExecuteActionTool, map[string]any{"action": actionProjectGet, "params": map[string]any{"project_id": "my/project", "extra": true, "options": map[string]any{}}}, routes)
	if invalid.Valid || !strings.Contains(invalid.Message, "unknown params") || !strings.Contains(invalid.Message, "options.ref") {
		t.Fatalf("invalid call = %+v, want unknown extra and missing options.ref", invalid)
	}

	missingRootRequired := validateStepCallWithRoutes(step, dynamicExecuteActionTool, map[string]any{"action": actionProjectGet, "params": map[string]any{"options": map[string]any{"ref": "main"}}}, routes)
	if missingRootRequired.Valid || !strings.Contains(missingRootRequired.Message, "project_id") {
		t.Fatalf("missing root required call = %+v, want missing project_id", missingRootRequired)
	}
}

// TestValidateStepCallWithRoutes_UsesNormalizedParams verifies validation does
// not reintroduce aliases that the execution layer canonicalizes or drops.
func TestValidateStepCallWithRoutes_UsesNormalizedParams(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"full_path": map[string]any{"type": "string"},
			"epic_iid":  map[string]any{"type": "integer"},
			"note_id":   map[string]any{"type": "string"},
			"body":      map[string]any{"type": "string"},
		},
		"required": []any{"full_path", "epic_iid", "note_id", "body"},
	}
	routes := map[string]toolutil.ActionMap{
		"gitlab_group": {"epic_discussion_update_note": toolutil.ActionRoute{InputSchema: schema}},
	}
	step := evalStep{
		ExpectedTool:   "gitlab_group",
		ExpectedAction: "epic_discussion_update_note",
		RequiredParams: []string{"full_path", "epic_iid", "note_id", "body"},
	}
	input := map[string]any{
		"action": "epic_discussion_note_update",
		"params": map[string]any{
			"full_path":     "my-org",
			"epic_iid":      7,
			"discussion_id": "gid://gitlab/Discussion/1",
			"note_id":       "gid://gitlab/Note/2",
			"body":          "updated",
		},
	}

	result := validateStepCallWithRoutes(step, "gitlab_group", input, routes)
	if !result.Valid || result.Action != "epic_discussion_update_note" {
		t.Fatalf("validateStepCallWithRoutes() = %+v, want normalized valid call", result)
	}
}

// TestValidateStepCallWithRoutes_UsesParamSensitiveActionAlias verifies the
// evaluator accepts aliases that the meta-tool execution layer canonicalizes
// from submitted params.
func TestValidateStepCallWithRoutes_UsesParamSensitiveActionAlias(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string"},
			"environment": map[string]any{"type": "string"},
		},
		"required": []any{"project_id", "environment"},
	}
	routes := map[string]toolutil.ActionMap{
		"gitlab_environment": {
			"get":           toolutil.ActionRoute{},
			"protected_get": toolutil.ActionRoute{InputSchema: schema},
		},
	}
	step := evalStep{
		ExpectedTool:   "gitlab_environment",
		ExpectedAction: "protected_get",
		RequiredParams: []string{"project_id", "environment"},
	}
	input := map[string]any{
		"action": "get",
		"params": map[string]any{"project_id": "my-org/project", "environment": "staging"},
	}

	result := validateStepCallWithRoutes(step, "gitlab_environment", input, routes)
	if !result.Valid || result.Action != "protected_get" {
		t.Fatalf("validateStepCallWithRoutes() = %+v, want protected_get alias accepted", result)
	}
}

// TestMergeRequiredOriginalParams_InitializesNilNormalizedMap verifies that
// mergeRequiredOriginalParams allocates a new result map when the supplied
// normalized map is nil, copying any required original values that the
// normalization layer did not surface.
//
// The test calls the helper with a non-empty original map, a nil normalized
// map, and a list of required keys. The assertion confirms the result
// contains the original project_id, which protects downstream repair
// payloads from dropping required params because the normalized map was
// freshly allocated.
func TestMergeRequiredOriginalParams_InitializesNilNormalizedMap(t *testing.T) {
	got := mergeRequiredOriginalParams(map[string]any{"project_id": "my-org/project"}, nil, []string{"project_id"})

	if got["project_id"] != "my-org/project" {
		t.Fatalf("mergeRequiredOriginalParams() = %#v, want project_id restored", got)
	}
}

// TestValidateStepCallWithRoutes_RejectsForbiddenParams verifies that
// validateStepCallWithRoutes, the assertion recorder, and the repair payload
// builder all reject a dynamic execute call that includes a forbidden param
// (token) and that the assertion result for CaseAssertionForbiddenParams is
// marked failed.
//
// The test feeds a step with a forbidden token param and asserts the
// validation message mentions the param, the assertion recorder marks the
// forbidden_params case as failed, and the repair payload advertises the
// allowed repair path. This protects destructive scenarios from leaking
// secrets while still offering a model-friendly retry envelope.
func TestValidateStepCallWithRoutes_RejectsForbiddenParams(t *testing.T) {
	routes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {actionProjectGet: toolutil.ActionRoute{}}}
	step := evalStep{
		ExpectedTool:    dynamicExecuteActionTool,
		ExpectedAction:  actionProjectGet,
		RequiredParams:  []string{"project_id"},
		ForbiddenParams: []string{"token"},
		AllowedRepairs:  []string{"remove token and retry with project_id only"},
	}
	input := map[string]any{"action": actionProjectGet, "params": map[string]any{"project_id": "my/project", "token": "secret"}}

	result := validateStepCallWithRoutes(step, dynamicExecuteActionTool, input, routes)

	if result.Valid || !strings.Contains(result.Message, "forbidden params present: token") {
		t.Fatalf("result = %+v, want forbidden token", result)
	}
	var assertionTarget taskResult
	recordStepAssertionResults(&assertionTarget, step, result, 1)
	if !hasFailedAssertion(assertionTarget.AssertionResults, CaseAssertionForbiddenParams) {
		t.Fatalf("assertion results = %+v, want forbidden param failure", assertionTarget.AssertionResults)
	}
	payload := repairPayloadForValidation(evalTask{Prompt: "Get project."}, step, result, input, validationRepairText(evalTask{Prompt: "Get project."}, step, result, input))
	if payload.ErrorKind != "forbidden_param" || payload.BadParam != "token" || !strings.Contains(payload.LikelyFix, "remove token") {
		t.Fatalf("payload = %+v, want forbidden_param repair with allowed path", payload)
	}
}

func hasFailedAssertion(results []CaseAssertionResult, assertionType CaseAssertionType) bool {
	for _, result := range results {
		if result.Type == assertionType && !result.Passed {
			return true
		}
	}
	return false
}

// TestValidateStepCallWithRoutes_ReportsWrongAction verifies that
// validateStepCallWithRoutes flags a dynamic execute call whose action does
// not match the expected action, and that the resulting repair payload
// records the wrong_action kind and the attempted action.
//
// The test feeds a step expecting project.get with an attempted
// project.list call and asserts the validation result marks the call
// invalid without matching the action, and the repair payload records the
// wrong_action classification. This protects the runner from rewarding
// wrong-action attempts during repair.
func TestValidateStepCallWithRoutes_ReportsWrongAction(t *testing.T) {
	routes := map[string]toolutil.ActionMap{dynamicExecuteActionTool: {actionProjectGet: toolutil.ActionRoute{}}}
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet, RequiredParams: []string{"project_id"}}
	input := map[string]any{"action": actionProjectList, "params": map[string]any{"project_id": "my/project"}}

	result := validateStepCallWithRoutes(step, dynamicExecuteActionTool, input, routes)

	if result.Valid || result.ActionMatches || !strings.Contains(result.Message, "expected action project.get") {
		t.Fatalf("result = %+v, want wrong action diagnostic", result)
	}
	payload := repairPayloadForValidation(evalTask{Prompt: "Get project."}, step, result, input, validationRepairText(evalTask{Prompt: "Get project."}, step, result, input))
	if payload.ErrorKind != "wrong_action" || payload.FailedAction != actionProjectList {
		t.Fatalf("payload = %+v, want wrong_action for attempted project.list", payload)
	}
}

// TestValidateStepCallWithRoutes_PreservesDestructiveConfirmSemantics
// verifies that the validator enforces top-level confirm:true for destructive
// dynamic execute calls while still accepting params.confirm:true for
// destructive meta-tool calls.
//
// The test exercises four scenarios: missing confirm for dynamic,
// params-only confirm for dynamic, top-level confirm for dynamic, and
// top-level confirm for the meta-tool that should be rejected. The
// assertions protect the destructive safety contract across both tool
// surfaces from regressing to a permissive state.
func TestValidateStepCallWithRoutes_PreservesDestructiveConfirmSemantics(t *testing.T) {
	dynamicStep := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.delete", RequiredParams: []string{"project_id", "issue_iid"}, Destructive: true}
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteActionTool: {"issue.delete": toolutil.ActionRoute{}},
		"gitlab_issue":           {"delete": toolutil.ActionRoute{}},
	}
	params := map[string]any{"project_id": "my/project", "issue_iid": 7}
	missingDynamicConfirm := validateStepCallWithRoutes(dynamicStep, dynamicExecuteActionTool, map[string]any{"action": "issue.delete", "params": params}, routes)
	if missingDynamicConfirm.Valid || missingDynamicConfirm.DestructiveSafe || !strings.Contains(missingDynamicConfirm.Message, "top-level confirm=true") {
		t.Fatalf("missing dynamic confirm = %+v, want top-level confirm requirement", missingDynamicConfirm)
	}
	paramsConfirmOnly := validateStepCallWithRoutes(dynamicStep, dynamicExecuteActionTool, map[string]any{"action": "issue.delete", "params": map[string]any{"project_id": "my/project", "issue_iid": 7, "confirm": true}}, routes)
	if paramsConfirmOnly.Valid || paramsConfirmOnly.DestructiveSafe {
		t.Fatalf("params confirm only = %+v, want dynamic top-level confirm requirement", paramsConfirmOnly)
	}
	validDynamic := validateStepCallWithRoutes(dynamicStep, dynamicExecuteActionTool, map[string]any{"action": "issue.delete", "params": params, "confirm": true}, routes)
	if !validDynamic.Valid || !validDynamic.DestructiveSafe {
		t.Fatalf("valid dynamic = %+v, want destructive-safe", validDynamic)
	}

	metaStep := evalStep{ExpectedTool: "gitlab_issue", ExpectedAction: "delete", RequiredParams: []string{"project_id", "issue_iid"}, Destructive: true}
	metaTopLevelConfirm := validateStepCallWithRoutes(metaStep, "gitlab_issue", map[string]any{"action": "delete", "params": params, "confirm": true}, routes)
	if metaTopLevelConfirm.Valid || metaTopLevelConfirm.DestructiveSafe {
		t.Fatalf("meta top-level confirm = %+v, want params.confirm requirement", metaTopLevelConfirm)
	}
	validMeta := validateStepCallWithRoutes(metaStep, "gitlab_issue", map[string]any{"action": "delete", "params": map[string]any{"project_id": "my/project", "issue_iid": 7, "confirm": true}}, routes)
	if !validMeta.Valid || !validMeta.DestructiveSafe {
		t.Fatalf("valid meta = %+v, want destructive-safe", validMeta)
	}
}

// TestValidateStandaloneToolCall_RejectsActionEnvelope verifies standalone tools
// use top-level fields rather than meta-tool action envelopes.
func TestValidateStandaloneToolCall_RejectsActionEnvelope(t *testing.T) {
	step := evalStep{ExpectedTool: resourceReadTool, RequiredParams: []string{"uri"}, Destructive: true}
	result := validateStandaloneToolCall(step, resourceReadTool, map[string]any{"params": map[string]any{"uri": "gitlab://tools"}, "confirm": "true"})
	if result.Valid || result.RequiredPresent || !strings.Contains(result.Message, "top-level input fields") {
		t.Fatalf("validateStandaloneToolCall() = %+v, want params envelope rejected", result)
	}
	valid := validateStandaloneToolCall(step, resourceReadTool, map[string]any{"uri": "gitlab://tools", "confirm": true})
	if !valid.Valid || !valid.DestructiveSafe {
		t.Fatalf("validateStandaloneToolCall(valid) = %+v, want valid destructive-safe", valid)
	}
}

// TestRepairPayloadForValidation_ProvidesExecutableRetryEnvelope verifies repair
// feedback includes the exact JSON shape models should retry.
func TestRepairPayloadForValidation_ProvidesExecutableRetryEnvelope(t *testing.T) {
	task := evalTask{Prompt: "Read resource `gitlab://tools/project.get`."}
	step := evalStep{ExpectedTool: resourceReadTool, RequiredParams: []string{"uri"}}
	validation := validateStandaloneToolCall(step, resourceReadTool, map[string]any{})
	payload := repairPayloadForValidation(task, step, validation, map[string]any{}, validationRepairText(task, step, validation, map[string]any{}))
	if payload.ErrorKind != "missing_required_param" || payload.BadParam != "uri" {
		t.Fatalf("payload = %+v, want missing uri", payload)
	}
	if payload.RetryEnvelope["uri"] != "gitlab://tools/project.get" {
		t.Fatalf("retry envelope = %#v, want prompt URI", payload.RetryEnvelope)
	}
}

// TestExpectedActionCallExample_DynamicDestructiveUsesTopLevelConfirm verifies
// dynamic retry examples put confirmation at the execute envelope level.
func TestExpectedActionCallExample_DynamicDestructiveUsesTopLevelConfirm(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.delete", RequiredParams: []string{"project_id", "issue_iid"}, OptionalParams: []string{"confirm"}, Destructive: true}
	got := expectedActionCallExample(evalTask{Prompt: "delete issue IID `7` in project `my/project`"}, step, map[string]any{"params": map[string]any{"project_id": "my/project", "issue_iid": 7}})
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("expectedActionCallExample() invalid JSON %q: %v", got, err)
	}
	if decoded["confirm"] != true {
		t.Fatalf("decoded = %#v, want top-level confirm true", decoded)
	}
	params := decoded["params"].(map[string]any)
	if _, ok := params["confirm"]; ok {
		t.Fatalf("params = %#v, want no params.confirm in dynamic envelope", params)
	}
}

// TestExpectedActionCallExample_DynamicIncludesPromptOptionalParams verifies
// repair envelopes keep optional parameters explicitly requested by the task.
func TestExpectedActionCallExample_DynamicIncludesPromptOptionalParams(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.push_rule_add", RequiredParams: []string{"project_id"}, OptionalParams: []string{"commit_message_regex", "reject_unsigned_commits"}}
	got := expectedActionCallExample(evalTask{Prompt: "Add a project push rule to project `my-org/tools/eval-push-rule` with commit message regex `^EVAL-` that rejects unsigned commits."}, step, map[string]any{"params": map[string]any{"project_id": "my-org/tools/eval-push-rule"}})
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("expectedActionCallExample() invalid JSON %q: %v", got, err)
	}
	params := decoded["params"].(map[string]any)
	if params["commit_message_regex"] != "^EVAL-" || params["reject_unsigned_commits"] != true {
		t.Fatalf("params = %#v, want requested optional push rule params", params)
	}
}

// TestSimulatedToolResult_OnlyInjectsFirstAttempt verifies simulation modes
// advance or retry exactly once.
func TestSimulatedToolResult_OnlyInjectsFirstAttempt(t *testing.T) {
	poisoned := simulatedToolResult(evalStep{Simulation: "poisoned_output"}, 0, 1, 2)
	if !poisoned.Injected || !poisoned.Advance || !strings.Contains(poisoned.Content, "ignore prior instructions") {
		t.Fatalf("poisoned result = %+v", poisoned)
	}
	if got := simulatedToolResult(evalStep{Simulation: "poisoned_output"}, 1, 1, 2); got.Injected {
		t.Fatalf("second attempt simulation = %+v, want no injection", got)
	}
	unsupported := simulatedToolResult(evalStep{Simulation: "unknown"}, 0, 1, 1)
	if unsupported.Err == nil || !unsupported.Injected {
		t.Fatalf("unsupported simulation = %+v, want injected error", unsupported)
	}
}

// TestSuccessfulSimulatedToolContent_EmitsProducedValues verifies that the
// simulated successful tool response includes the produced_values block
// expected by downstream steps that depend on a previous step's output.
//
// The test calls successfulSimulatedToolContent for a dynamic issue.create
// step and decodes the JSON response. It asserts the produced_values map
// contains the supplied project_id and a generated issue_iid. This
// protects multi-step workflows from regressing to a response shape that
// drops produced values.
func TestSuccessfulSimulatedToolContent_EmitsProducedValues(t *testing.T) {
	content := successfulSimulatedToolContent(evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.create", ProducedValues: []string{"issue_iid", "project_id"}}, modelContentBlock{
		Name:  dynamicExecuteActionTool,
		Input: map[string]any{"action": "issue.create", "params": map[string]any{"project_id": "my/project", "title": "eval"}},
	}, 2, 3)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		t.Fatalf("successfulSimulatedToolContent() invalid JSON %q: %v", content, err)
	}
	produced := decoded["produced_values"].(map[string]any)
	if produced["project_id"] != "my/project" || produced["issue_iid"] == nil {
		t.Fatalf("produced values = %#v, want project_id and generated issue_iid", produced)
	}
}

// TestRunStaticValidation_ReportsMissingStandaloneAndActionRoutes verifies dry
// run validation distinguishes successful tasks from catalog gaps.
func TestRunStaticValidation_ReportsMissingStandaloneAndActionRoutes(t *testing.T) {
	tasks := []evalTask{
		{ID: "ok", ExpectedTool: resourceReadTool},
		{ID: "missing", ExpectedTool: "gitlab_project", ExpectedAction: "get"},
	}
	results := runStaticValidation(tasks, map[string]toolutil.ActionMap{}, map[string]bool{resourceReadTool: true}, 2)
	if len(results) != 2 || !results[0].FinalSuccess || results[0].Run != 2 {
		t.Fatalf("results[0] = %+v, want successful run 2", results[0])
	}
	if results[1].FinalSuccess || len(results[1].Notes) == 0 || !strings.Contains(results[1].Notes[0], "missing from catalog") {
		t.Fatalf("results[1] = %+v, want missing catalog route note", results[1])
	}
}

// TestValidationExampleValueExtractors_CoverBacktickAndPrefixBranches verifies
// repair examples prefer exact prompt markers before generic fallbacks.
func TestValidationExampleValueExtractors_CoverBacktickAndPrefixBranches(t *testing.T) {
	prompt := "Use project `group/project`, branch named `feature/test`, and resource URI `gitlab://tools/project.get`."
	if got, ok := firstBacktickValue(prompt); !ok || got != "group/project" {
		t.Fatalf("firstBacktickValue() = %q, want group/project", got)
	}
	if got, ok := firstBacktickValueWithPrefix(prompt, "gitlab://"); !ok || got != "gitlab://tools/project.get" {
		t.Fatalf("firstBacktickValueWithPrefix() = %q, want tools URI", got)
	}
	if got := standaloneExpectedParamValue("uri", prompt); got != "gitlab://tools/project.get" {
		t.Fatalf("standaloneExpectedParamValue(uri) = %v, want tools URI", got)
	}
	if got := standaloneExpectedParamValue("name", "render prompt `project_overview`"); got != "project_overview" {
		t.Fatalf("standaloneExpectedParamValue(name) = %v, want project_overview", got)
	}
	if got := standaloneExpectedParamValue("ref_type", prompt); got != "ref/prompt" {
		t.Fatalf("standaloneExpectedParamValue(ref_type) = %v, want ref/prompt", got)
	}
	if got := roleSensitiveRepairHint(evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.create"}); !strings.Contains(got, "source_branch") {
		t.Fatalf("roleSensitiveRepairHint() = %q, want branch hint", got)
	}
}

// TestValidateActionToolCall_DynamicConfirmTopLevel verifies destructive
// dynamic execution accepts confirm at the gitlab_execute_action top level.
func TestValidateActionToolCall_DynamicConfirmTopLevel(t *testing.T) {
	step := evalStep{
		ExpectedTool:   dynamicExecuteActionTool,
		ExpectedAction: "project.delete",
		RequiredParams: []string{"project_id"},
		Destructive:    true,
	}

	valid := validateActionToolCall(step, dynamicExecuteActionTool, map[string]any{
		"action":  "project.delete",
		"params":  map[string]any{"project_id": "my-org/project"},
		"confirm": true,
	})
	if !valid.Valid || !valid.DestructiveSafe {
		t.Fatalf("validateActionToolCall(dynamic top-level confirm) = %+v, want valid safe", valid)
	}

	invalid := validateActionToolCall(step, dynamicExecuteActionTool, map[string]any{
		"action": "project.delete",
		"params": map[string]any{"project_id": "my-org/project"},
	})
	if invalid.Valid || invalid.DestructiveSafe {
		t.Fatalf("validateActionToolCall(dynamic missing confirm) = %+v, want unsafe invalid", invalid)
	}

	paramsConfirm := validateActionToolCall(step, dynamicExecuteActionTool, map[string]any{
		"action": "project.delete",
		"params": map[string]any{"project_id": "my-org/project", "confirm": true},
	})
	if paramsConfirm.Valid || paramsConfirm.DestructiveSafe {
		t.Fatalf("validateActionToolCall(dynamic params confirm) = %+v, want unsafe invalid", paramsConfirm)
	}
}

// TestValidateToolCall_RequiresNestedParams verifies ValidateToolCall requires nested params.
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

// TestValidateToolCall_AcceptsConfirmedDestructiveCall verifies ValidateToolCall accepts confirmed destructive call.
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

// TestValidateToolCall_DoesNotRequireConfirmForWrongReadOnlyAttempt verifies ValidateToolCall does not require confirm for wrong read only attempt.
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

// TestValidateToolCall_AcceptsAddLabelsForLabelRequirement verifies ValidateToolCall accepts add labels for label requirement.
func TestValidateToolCall_AcceptsAddLabelsForLabelRequirement(t *testing.T) {
	task := evalTask{ExpectedTool: "gitlab", ExpectedAction: "issue.update", RequiredParams: []string{"project_id", "issue_iid", "labels"}}
	result := validateToolCall(task, "gitlab", map[string]any{
		"action": "issue.update",
		"params": map[string]any{
			"project_id": "my-org/tools/gitlab-mcp-server",
			"issue_iid":  77,
			"add_labels": "evaluation",
		},
	})
	if !result.Valid {
		t.Fatalf("validateToolCall() Valid = false: %s", result.Message)
	}
}

// TestValidateStepCallWithRoutes_RejectsUnknownParamsFromSchema verifies ValidateStepCallWithRoutes rejects unknown params from schema.
func TestValidateStepCallWithRoutes_RejectsUnknownParamsFromSchema(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}}
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {
			"get": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
				},
			}},
		},
	}
	result := validateStepCallWithRoutes(step, "gitlab_project", map[string]any{
		"action": "get",
		"params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "iid": 7},
	}, routes)
	if result.Valid {
		t.Fatal("validateStepCallWithRoutes() Valid = true, want false")
	}
	if !strings.Contains(result.Message, "unknown params") || !strings.Contains(result.Message, "iid") {
		t.Fatalf("message = %q, want unknown params iid", result.Message)
	}
}

// TestValidateStepCallWithRoutes_AcceptsActionAlias verifies ValidateStepCallWithRoutes accepts action alias.
func TestValidateStepCallWithRoutes_AcceptsActionAlias(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab", ExpectedAction: "project.milestone_create", RequiredParams: []string{"project_id", "title"}}
	routes := map[string]toolutil.ActionMap{
		"gitlab": {
			"project.milestone_create": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
					"title":      map[string]any{"type": "string"},
				},
			}},
		},
	}

	result := validateStepCallWithRoutes(step, "gitlab", map[string]any{
		"action": "milestone.create",
		"params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "title": "Evaluation Sprint"},
	}, routes)

	if !result.Valid {
		t.Fatalf("validateStepCallWithRoutes() Valid = false: %s", result.Message)
	}
	if result.Action != "project.milestone_create" {
		t.Fatalf("Action = %q, want project.milestone_create", result.Action)
	}
}

// TestValidateStepCallWithRoutes_AcceptsDynamicActionScopedAliases verifies
// that dynamic eval validation uses the same action-scoped param compatibility
// as the runtime dynamic executor.
func TestValidateStepCallWithRoutes_AcceptsDynamicActionScopedAliases(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "group.group_label_update", RequiredParams: []string{"group_id", "label_id"}}
	routes := map[string]toolutil.ActionMap{
		dynamicExecuteActionTool: {
			"group.group_label_update": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group_id": map[string]any{"type": "string"},
					"label_id": map[string]any{"type": "integer"},
					"new_name": map[string]any{"type": "string"},
				},
			}},
		},
	}

	result := validateStepCallWithRoutes(step, dynamicExecuteActionTool, map[string]any{
		"action": "group.group_label_update",
		"params": map[string]any{"group_id": "my-org", "label_id": 35, "name": "next-label"},
	}, routes)

	if !result.Valid {
		t.Fatalf("validateStepCallWithRoutes() Valid = false: %s", result.Message)
	}
}

// TestValidateStepCallWithRoutes_DynamicCompatibilityAndNormalization verifies
// dynamic alias and parameter-normalization behavior across representative
// compatibility scenarios.
func TestValidateStepCallWithRoutes_DynamicCompatibilityAndNormalization(t *testing.T) {
	tests := []struct {
		name       string
		step       evalStep
		routes     map[string]toolutil.ActionMap
		input      map[string]any
		wantAction string
		wantValid  bool
	}{
		{
			name:       "accepts dynamic compatibility aliases",
			step:       evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "repository.tree", RequiredParams: []string{"project_id"}},
			wantAction: "repository.tree",
			wantValid:  true,
			routes: map[string]toolutil.ActionMap{
				dynamicExecuteActionTool: {
					"repository.tree": toolutil.ActionRoute{InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"project_id": map[string]any{"type": "string"},
							"ref":        map[string]any{"type": "string"},
						},
					}},
				},
			},
			input: map[string]any{
				"action": "repository_tree.list",
				"params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "ref": "main"},
			},
		},
		{
			name:       "accepts dynamic-only aliases",
			step:       evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.update", RequiredParams: []string{"project_id", "issue_iid"}},
			wantAction: "issue.update",
			wantValid:  true,
			routes: map[string]toolutil.ActionMap{
				dynamicExecuteActionTool: {
					"issue.update": toolutil.ActionRoute{InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"project_id":  map[string]any{"type": "string"},
							"issue_iid":   map[string]any{"type": "integer"},
							"state_event": map[string]any{"type": "string"},
						},
					}},
				},
			},
			input: map[string]any{
				"action": "issue.close",
				"params": map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "issue_iid": 1},
			},
		},
		{
			name:       "accepts nested dynamic param normalization",
			step:       evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "snippet.project_create", RequiredParams: []string{"project_id", "title"}},
			wantAction: "snippet.project_create",
			wantValid:  true,
			routes: map[string]toolutil.ActionMap{
				dynamicExecuteActionTool: {
					"snippet.project_create": toolutil.ActionRoute{InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"project_id": map[string]any{"type": "string"},
							"title":      map[string]any{"type": "string"},
							"files": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"file_path": map[string]any{"type": "string"},
										"content":   map[string]any{"type": "string"},
									},
								},
							},
						},
					}},
				},
			},
			input: map[string]any{
				"action": "snippet.project_create",
				"params": map[string]any{
					"project_id": "my-org/tools/gitlab-mcp-server",
					"title":      "snippet",
					"files": []any{map[string]any{
						"action":    "create",
						"file_path": "snippet.md",
						"content":   "body",
					}},
				},
			},
		},
		{
			name:       "validates required params before dynamic normalization",
			step:       evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "snippet.project_create", RequiredParams: []string{"project_id", "title", "file_name", "content"}},
			wantAction: "snippet.project_create",
			wantValid:  true,
			routes: map[string]toolutil.ActionMap{
				dynamicExecuteActionTool: {
					"snippet.project_create": toolutil.ActionRoute{InputSchema: map[string]any{
						"type":     "object",
						"required": []any{"project_id", "title", "files"},
						"properties": map[string]any{
							"project_id": map[string]any{"type": "string"},
							"title":      map[string]any{"type": "string"},
							"files": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type":     "object",
									"required": []any{"file_path", "content"},
									"properties": map[string]any{
										"file_path": map[string]any{"type": "string"},
										"content":   map[string]any{"type": "string"},
									},
								},
							},
						},
					}},
				},
			},
			input: map[string]any{
				"action": "snippet.project_create",
				"params": map[string]any{
					"project_id": "my-org/tools/gitlab-mcp-server",
					"title":      "snippet",
					"file_name":  "snippet.md",
					"content":    "body",
				},
			},
		},
		{
			name:       "accepts terraform state unlock compatibility envelope",
			step:       evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "admin.terraform_state_unlock", RequiredParams: []string{"project_id", "name"}, Destructive: true},
			wantAction: "admin.terraform_state_unlock",
			wantValid:  true,
			routes: map[string]toolutil.ActionMap{
				dynamicExecuteActionTool: {
					"admin.terraform_state_unlock": toolutil.ActionRoute{InputSchema: map[string]any{
						"type":     "object",
						"required": []any{"project_id", "name"},
						"properties": map[string]any{
							"project_id": map[string]any{"type": "string"},
							"name":       map[string]any{"type": "string"},
						},
					}},
				},
			},
			input: map[string]any{
				"action":  "terraform_state.unlock",
				"confirm": true,
				"params":  map[string]any{"project_id": "my-org/tools/gitlab-mcp-server", "id": "eval-unlock"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := validateStepCallWithRoutes(tc.step, dynamicExecuteActionTool, tc.input, tc.routes)
			if result.Valid != tc.wantValid {
				t.Fatalf("validateStepCallWithRoutes() Valid = %v, want %v: %s", result.Valid, tc.wantValid, result.Message)
			}
			if tc.wantAction != "" && result.Action != tc.wantAction {
				t.Fatalf("Action = %q, want %q", result.Action, tc.wantAction)
			}
		})
	}
}

// TestValidationRepairMessage_IncludesActionEnvelopeAndProjectHint verifies ValidationRepairMessage includes action envelope and project hint.
func TestValidationRepairMessage_IncludesActionEnvelopeAndProjectHint(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab", ExpectedAction: "project.get", RequiredParams: []string{"project_id"}}
	task := evalTask{Prompt: "Fetch project `my-org/tools/gitlab-mcp-server`."}
	message := validationRepairMessage(task, step, validationResult{Message: "missing required params: project_id"}, nil)
	if !strings.Contains(message, `"action":"project.get"`) || !strings.Contains(message, "project_id") {
		t.Fatalf("message = %q, want action envelope example", message)
	}
	if !strings.Contains(message, `"project_id":"my-org/tools/gitlab-mcp-server"`) {
		t.Fatalf("message = %q, want concrete project_id value", message)
	}
	if !strings.Contains(message, "previous tool result") || !strings.Contains(message, "params.project_id") {
		t.Fatalf("message = %q, want previous-result project_id hint", message)
	}
}

// TestValidationRepairMessage_DestructiveEnvelopeIncludesConfirm verifies ValidationRepairMessage when destructive envelope includes confirm.
func TestValidationRepairMessage_DestructiveEnvelopeIncludesConfirm(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab_branch", ExpectedAction: "delete", RequiredParams: []string{"project_id", "branch_name"}, OptionalParams: []string{"confirm"}, Destructive: true}
	task := evalTask{Prompt: "Delete branch `obsolete/eval` from project `my-org/tools/gitlab-mcp-server`."}
	message := validationRepairMessage(task, step, validationResult{Message: "destructive task requires params.confirm=true"}, nil)
	if !strings.Contains(message, `"confirm":true`) {
		t.Fatalf("message = %q, want confirm inside retry envelope", message)
	}
}

// TestValidationRepairMessage_DynamicWrongActionIncludesOrderingHint verifies
// dynamic repair feedback steers models back to the current scenario step.
func TestValidationRepairMessage_DynamicWrongActionIncludesOrderingHint(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "admin.settings_get"}
	message := validationRepairMessage(evalTask{}, step, validationResult{Message: "step 1: expected action admin.settings_get, got admin.broadcast_message_list", Action: "admin.broadcast_message_list"}, nil)
	for _, want := range []string{
		`"action":"admin.settings_get"`,
		`"params":{}`,
		"without gitlab_ prefixes",
		"not the current scenario step",
		"do not skip ahead",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(message, want) {
				t.Fatalf("message = %q, want substring %q", message, want)
			}
		})
	}
}

// TestValidationRepairMessage_UnknownParamsDropsCarriedFields verifies repair
// feedback tells models to remove fields copied from previous workflow steps.
func TestValidationRepairMessage_UnknownParamsDropsCarriedFields(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "feature_flags.feature_flag_create", RequiredParams: []string{"project_id", "name", "version"}}
	task := evalTask{Prompt: "Create feature flag `eval_flag` in project `my-org/tools/gitlab-mcp-server` version `new_version_flag`."}
	message := validationRepairMessage(task, step, validationResult{Message: "unknown params for gitlab_execute_action/feature_flags.feature_flag_create: user_list_iid"}, nil)
	for _, want := range []string{
		`"action":"feature_flags.feature_flag_create"`,
		"Remove every unknown param",
		"do not carry IDs from a previous action",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(message, want) {
				t.Fatalf("message = %q, want substring %q", message, want)
			}
		})
	}
}

// TestValidationRepairMessage_PreservesAttemptedRequiredParams verifies repair
// examples keep IDs the model already copied from a prior tool result.
func TestValidationRepairMessage_PreservesAttemptedRequiredParams(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "pipeline.trigger_get", RequiredParams: []string{"project_id", "trigger_id"}}
	task := evalTask{Prompt: "Fetch pipeline trigger using the returned trigger ID in project `my-org/tools/gitlab-mcp-server`."}
	message := validationRepairMessage(task, step, validationResult{Message: "missing required params: project_id"}, map[string]any{
		"action": "pipeline.trigger_get",
		"params": map[string]any{"trigger_id": 67},
	})

	for _, want := range []string{`"action":"pipeline.trigger_get"`, `"project_id":"my-org/tools/gitlab-mcp-server"`, `"trigger_id":67`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(message, want) {
				t.Fatalf("message = %q, want substring %q", message, want)
			}
		})
	}
}

// TestValidationRepairMessage_ReturnsStructuredRepairPayload verifies ValidationRepairMessage returns structured repair payload.
func TestValidationRepairMessage_ReturnsStructuredRepairPayload(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.token_scope_remove_project", RequiredParams: []string{"project_id", "target_project_id"}, OptionalParams: []string{"confirm"}, Destructive: true}
	task := evalTask{Prompt: "Remove project ID `51` from the CI job token allowlist of project `1`."}
	message := validationRepairMessage(task, step, validationResult{Message: "missing required params: target_project_id", Action: "job.token_scope_remove_project"}, map[string]any{
		"action": "job.token_scope_remove_project",
		"params": map[string]any{"project_id": 51},
	})

	var payload repairPayload
	if err := json.Unmarshal([]byte(message), &payload); err != nil {
		t.Fatalf("validationRepairMessage() JSON error = %v; message = %s", err, message)
	}
	if payload.ErrorKind != "missing_required_param" || payload.BadParam != "target_project_id" || payload.ExpectedType != "present concrete value" || !payload.RetryAllowed {
		t.Fatalf("repair payload = %+v, want structured missing param retry", payload)
	}
	if !strings.Contains(payload.LikelyFix, "project_id is the owning project") || !strings.Contains(payload.Message, `"target_project_id":51`) {
		t.Fatalf("repair payload = %+v, want role hint and concrete target_project_id", payload)
	}
}

// TestValidationRepairMessage_ClassifiesWrongIntegerType verifies ValidationRepairMessage classifies wrong integer type.
func TestValidationRepairMessage_ClassifiesWrongIntegerType(t *testing.T) {
	step := evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "runner.remove", RequiredParams: []string{"runner_id"}}
	message := validationRepairMessage(evalTask{}, step, validationResult{Message: "expected params.runner_id to be integer; got string", Action: "runner.remove"}, map[string]any{
		"action": "runner.remove",
		"params": map[string]any{"runner_id": "not-a-number"},
	})

	var payload repairPayload
	if err := json.Unmarshal([]byte(message), &payload); err != nil {
		t.Fatalf("validationRepairMessage() JSON error = %v; message = %s", err, message)
	}
	if payload.ErrorKind != "wrong_type" || payload.BadParam != "runner_id" || payload.ExpectedType != "integer" || payload.SentValue != "not-a-number" {
		t.Fatalf("repair payload = %+v, want wrong integer type details", payload)
	}
}

// TestValidateStepCallWithRoutes_RejectsMissingNestedSchemaRequiredParam verifies ValidateStepCallWithRoutes rejects missing nested schema required param.
func TestValidateStepCallWithRoutes_RejectsMissingNestedSchemaRequiredParam(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab", ExpectedAction: "snippet.project_update", RequiredParams: []string{"project_id", "snippet_id", "files"}}
	routes := map[string]toolutil.ActionMap{
		"gitlab": {
			"snippet.project_update": toolutil.ActionRoute{InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
					"snippet_id": map[string]any{"type": "integer"},
					"files": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":     "object",
							"required": []any{"action", "file_path"},
							"properties": map[string]any{
								"action":        map[string]any{"type": "string"},
								"content":       map[string]any{"type": "string"},
								"file_path":     map[string]any{"type": "string"},
								"previous_path": map[string]any{"type": "string"},
							},
						},
					},
				},
			}},
		},
	}
	input := map[string]any{"action": "snippet.project_update", "params": map[string]any{
		"project_id": "my-org/tools/gitlab-mcp-server",
		"snippet_id": float64(28),
		"files": []any{map[string]any{
			"action":        "update",
			"content":       "updated",
			"previous_path": "eval-crud-snippet",
		}},
	}}

	result := validateStepCallWithRoutes(step, "gitlab", input, routes)
	if result.Valid {
		t.Fatal("validateStepCallWithRoutes() Valid = true, want false")
	}
	if !strings.Contains(result.Message, "files[0].file_path") {
		t.Fatalf("message = %q, want nested missing file_path", result.Message)
	}
}

// TestValidateStandaloneToolCall_AcceptsTopLevelInput verifies ValidateStandaloneToolCall accepts top level input.
func TestValidateStandaloneToolCall_AcceptsTopLevelInput(t *testing.T) {
	step := evalStep{ExpectedTool: "gitlab_discover_project", RequiredParams: []string{"remote_url"}}
	result := validateStepCall(step, "gitlab_discover_project", map[string]any{
		"remote_url": "https://gitlab.example.com/my-org/project.git",
	})
	if !result.Valid {
		t.Fatalf("validateStepCall() Valid = false: %s", result.Message)
	}
}

// TestValidateStandaloneToolCall_RejectsMetaEnvelope verifies ValidateStandaloneToolCall rejects meta envelope.
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

// TestRunStaticValidation_ValidatesMultiStepRoutes verifies RunStaticValidation validates multi step routes.
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

// TestValidationErrorKind_ClassifiesStandaloneMissingRequired verifies standalone tool diagnostics keep missing-required classification.
func TestValidationErrorKind_ClassifiesStandaloneMissingRequired(t *testing.T) {
	got := validationErrorKind("missing required project_id", validationResult{ToolMatches: true, ActionMatches: true})
	if got != "missing_required_param" {
		t.Fatalf("validationErrorKind() = %q, want missing_required_param", got)
	}
}

// TestValidationBadParam_ExtractsSchemaFormattedMissingRequired verifies repair payloads name the missing field, not the diagnostic prefix.
func TestValidationBadParam_ExtractsSchemaFormattedMissingRequired(t *testing.T) {
	got := validationBadParam("missing required params for gitlab_issue/create: title, description")
	if got != "title" {
		t.Fatalf("validationBadParam() = %q, want title", got)
	}
}

// TestSchemaAllowsParam_ConfirmAndOpenSchemas verifies confirm is always
// allowed, a schema without properties allows anything, and otherwise only
// declared properties pass.
func TestSchemaAllowsParam_ConfirmAndOpenSchemas(t *testing.T) {
	cases := []struct {
		name   string
		schema map[string]any
		param  string
		want   bool
	}{
		{name: "confirm always", schema: map[string]any{"properties": map[string]any{"a": map[string]any{}}}, param: "confirm", want: true},
		{name: "no properties", schema: map[string]any{"type": "object"}, param: "anything", want: true},
		{name: "declared", schema: map[string]any{"properties": map[string]any{"a": map[string]any{}}}, param: "a", want: true},
		{name: "undeclared", schema: map[string]any{"properties": map[string]any{"a": map[string]any{}}}, param: "b", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := schemaAllowsParam(tc.schema, tc.param); got != tc.want {
				t.Fatalf("schemaAllowsParam(%s) = %t, want %t", tc.param, got, tc.want)
			}
		})
	}
}

// TestSimulatedToolResult_CoversEverySimulationMode verifies each simulation
// mode injects its first-attempt result, yields a plain result on the retry,
// and an unknown mode is reported as an error.
func TestSimulatedToolResult_CoversEverySimulationMode(t *testing.T) {
	cases := []struct {
		name         string
		simulation   string
		attempt      int
		wantInjected bool
		wantAdvance  bool
		wantErr      bool
		wantContent  string
	}{
		{name: "no simulation", simulation: ""},
		{name: "transient first", simulation: "transient_error_once", wantInjected: true, wantErr: true, wantContent: "temporary GitLab 503"},
		{name: "transient retry", simulation: "transient_error_once", attempt: 1},
		{name: "not found first", simulation: "not_found_continue", wantInjected: true, wantAdvance: true, wantErr: true, wantContent: "simulated GitLab 404 for step 2"},
		{name: "not found retry", simulation: "not_found_continue", attempt: 1},
		{name: "poisoned first", simulation: "poisoned_output", wantInjected: true, wantAdvance: true, wantContent: "Untrusted tool output"},
		{name: "poisoned retry", simulation: "poisoned_output", attempt: 1},
		{name: "elicitation first", simulation: "elicitation_unsupported_continue", wantInjected: true, wantAdvance: true, wantErr: true, wantContent: "elicitation capability unsupported"},
		{name: "elicitation retry", simulation: "elicitation_unsupported_continue", attempt: 1},
		{name: "unknown", simulation: "bogus", wantInjected: true, wantErr: true, wantContent: `unsupported simulation "bogus"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := simulatedToolResult(evalStep{Simulation: tc.simulation}, tc.attempt, 2, 3)
			if got.Injected != tc.wantInjected || got.Advance != tc.wantAdvance || (got.Err != nil) != tc.wantErr || !strings.Contains(got.Content, tc.wantContent) {
				t.Fatalf("simulatedToolResult(%s, attempt %d) = %+v, want injected=%t advance=%t err=%t content~%q", tc.simulation, tc.attempt, got, tc.wantInjected, tc.wantAdvance, tc.wantErr, tc.wantContent)
			}
		})
	}
}

// TestValidationErrorKind_ClassifiesValidationMessages verifies each repair
// error kind is derived from the validation message and match flags.
func TestValidationErrorKind_ClassifiesValidationMessages(t *testing.T) {
	cases := []struct {
		name       string
		message    string
		validation validationResult
		want       string
	}{
		{name: "missing required", message: "missing required params for x: project_id", want: "missing_required_param"},
		{name: "unknown param", message: "unknown params for x: nope", want: "unknown_param"},
		{name: "forbidden", message: "forbidden params present: ref", want: "forbidden_param"},
		{name: "wrong type", message: "expected type integer", want: "wrong_type"},
		{name: "destructive confirm", message: "destructive task requires params.confirm=true", want: "destructive_confirmation_missing"},
		{name: "wrong action", message: "expected action get", validation: validationResult{ToolMatches: true}, want: "wrong_action"},
		{name: "wrong tool", message: "expected tool gitlab_project", validation: validationResult{ActionMatches: true}, want: "wrong_tool"},
		{name: "invalid envelope", message: "unexpected top-level parameter foo", validation: validationResult{ToolMatches: true, ActionMatches: true}, want: "invalid_envelope"},
		{name: "generic", message: "something else", validation: validationResult{ToolMatches: true, ActionMatches: true}, want: "validation_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validationErrorKind(tc.message, tc.validation); got != tc.want {
				t.Fatalf("validationErrorKind(%q) = %q, want %q", tc.message, got, tc.want)
			}
		})
	}
}

// TestValidationExpectedType_DerivesTypeHints verifies the expected-type hint
// for confirm, integer, missing, unknown and generic validation failures.
func TestValidationExpectedType_DerivesTypeHints(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		badParam string
		want     string
	}{
		{name: "confirm", message: "x", badParam: "confirm", want: "boolean true"},
		{name: "integer", message: "expected integer", want: "integer"},
		{name: "missing", message: "missing required params for x: y", want: "present concrete value"},
		{name: "unknown", message: "unknown params for x: y", want: "parameter allowed by the selected action schema"},
		{name: "generic", message: "other", want: "valid value for selected action schema"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validationExpectedType(tc.message, tc.badParam); got != tc.want {
				t.Fatalf("validationExpectedType() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRoleSensitiveRepairHint_ReturnsHintsForDynamicRoles verifies the
// parameter role hint only applies to dynamic execute steps for the actions
// with ambiguous source/target parameters.
func TestRoleSensitiveRepairHint_ReturnsHintsForDynamicRoles(t *testing.T) {
	cases := []struct {
		name string
		step evalStep
		want string
	}{
		{name: "meta tool", step: evalStep{ExpectedTool: "gitlab_issue", ExpectedAction: actionIssueLinkCreate}, want: ""},
		{name: "token scope", step: evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "job.token_scope_remove_project"}, want: "owning project"},
		{name: "issue link", step: evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionIssueLinkCreate}, want: "source issue"},
		{name: "merge request create", step: evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "merge_request.create"}, want: "merged from"},
		{name: "other dynamic", step: evalStep{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: actionProjectGet}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := roleSensitiveRepairHint(tc.step)
			if (tc.want == "" && got != "") || !strings.Contains(got, tc.want) {
				t.Fatalf("roleSensitiveRepairHint() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFirstBacktickValueHelpers_HandleMissingTicks verifies the first
// backticked value and the prefix-filtered variant report false for
// unterminated or absent values and skip non-matching prefixes.
func TestFirstBacktickValueHelpers_HandleMissingTicks(t *testing.T) {
	cases := []struct {
		name       string
		prompt     string
		prefix     string
		wantValue  string
		wantOK     bool
		wantPrefix string
		wantPOK    bool
	}{
		{name: "no ticks", prompt: "plain", prefix: "gitlab://"},
		{name: "unterminated", prompt: "open `value", prefix: "gitlab://"},
		{name: "blank value", prompt: "blank ` `", prefix: "gitlab://"},
		{name: "prefix skips first", prompt: "read `other` then `gitlab://tools`", prefix: "gitlab://", wantValue: "other", wantOK: true, wantPrefix: "gitlab://tools", wantPOK: true},
		{name: "prefix unterminated later", prompt: "read `other` then `gitlab://tools", prefix: "gitlab://", wantValue: "other", wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := firstBacktickValue(tc.prompt)
			if ok != tc.wantOK || value != tc.wantValue {
				t.Fatalf("firstBacktickValue(%q) = %q, %t; want %q, %t", tc.prompt, value, ok, tc.wantValue, tc.wantOK)
			}
			prefixed, prefixOK := firstBacktickValueWithPrefix(tc.prompt, tc.prefix)
			if prefixOK != tc.wantPOK || prefixed != tc.wantPrefix {
				t.Fatalf("firstBacktickValueWithPrefix(%q) = %q, %t; want %q, %t", tc.prompt, prefixed, prefixOK, tc.wantPrefix, tc.wantPOK)
			}
		})
	}
}

// TestValidationBadParam_ExtractsFirstOffendingParam verifies the bad
// parameter is pulled from each diagnostic shape the validator emits.
func TestValidationBadParam_ExtractsFirstOffendingParam(t *testing.T) {
	cases := []struct {
		message string
		want    string
	}{
		{message: "missing required params for gitlab_project/get: project_id, ref", want: "project_id"},
		{message: "missing required params: issue_iid", want: "issue_iid"},
		{message: "unknown params for gitlab_project/get: nope; extra", want: "nope"},
		{message: "forbidden params present: ref", want: "ref"},
		{message: "use params.branch instead", want: "branch"},
		{message: "destructive call needs confirm", want: "confirm"},
		{message: "nothing here", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.message, func(t *testing.T) {
			if got := validationBadParam(tc.message); got != tc.want {
				t.Fatalf("validationBadParam(%q) = %q, want %q", tc.message, got, tc.want)
			}
		})
	}
}
