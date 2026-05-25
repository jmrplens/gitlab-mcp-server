package evaluator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

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
