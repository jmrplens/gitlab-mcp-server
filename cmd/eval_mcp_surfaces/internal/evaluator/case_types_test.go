// case_types_test.go covers the typed [EvalCase] data model and the
// [taskFromCase] projection that flattens typed cases into the legacy
// evalTask shape used by the runtime.

package evaluator

import (
	"strings"
	"testing"
)

// TestTaskFromCase_SingleStepPreservesProjectedFields verifies that
// taskFromCase projects every scalar and slice field of a single-step typed
// case into the resulting evalTask, including route, params, simulation,
// and the destructive flag.
//
// The test feeds an EvalCase with one ExpectedStep and asserts identity,
// prompt, route, required and optional params, simulation, and the destructive
// flag. This protects the projection from dropping fields needed by the
// runtime validators.
func TestTaskFromCase_SingleStepPreservesProjectedFields(t *testing.T) {
	evalCase := EvalCase{
		ID:     "MT-CASE-001",
		Prompt: "Get project `gitlab-org/gitlab` by project path.",
		Steps: []ExpectedStep{{
			ExpectedTool:   "gitlab_project",
			ExpectedAction: "get",
			RequiredParams: []string{"project_id"},
			OptionalParams: []string{"statistics"},
			Simulation:     "project fixture response",
		}},
	}
	task := taskFromCase(evalCase)
	if task.ID != string(evalCase.ID) || task.Prompt != evalCase.Prompt {
		t.Fatalf("task identity = (%q, %q), want (%q, %q)", task.ID, task.Prompt, evalCase.ID, evalCase.Prompt)
	}
	if task.ExpectedTool != "gitlab_project" || task.ExpectedAction != "get" {
		t.Fatalf("task route = %s/%s, want gitlab_project/get", task.ExpectedTool, task.ExpectedAction)
	}
	if got := joinStrings(task.RequiredParams); got != "project_id" {
		t.Fatalf("required params = %q, want project_id", got)
	}
	if got := joinStrings(task.OptionalParams); got != "statistics" {
		t.Fatalf("optional params = %q, want statistics", got)
	}
	if task.Destructive || task.Simulation != "project fixture response" {
		t.Fatalf("task destructive/simulation = %t/%q, want false/project fixture response", task.Destructive, task.Simulation)
	}
}

// TestTaskFromCase_MultiStepPreservesStepFields verifies that taskFromCase
// keeps every step's projected fields intact for multi-step typed cases,
// including per-step destructive flags and simulation values.
//
// The test feeds an EvalCase with create + delete steps and asserts the
// task exposes both steps, the first route summary points at the create
// action, and the second step retains destructive semantics and its custom
// confirm param. This protects multi-step projections from collapsing or
// dropping step-specific attributes.
func TestTaskFromCase_MultiStepPreservesStepFields(t *testing.T) {
	evalCase := EvalCase{
		ID:     "MS-CASE-001",
		Prompt: "Create and then delete a temporary label with confirmation.",
		Steps: []ExpectedStep{
			{
				ExpectedTool:   "gitlab_label",
				ExpectedAction: "create",
				RequiredParams: []string{"project_id", "name", "color"},
				OptionalParams: []string{"description"},
				Simulation:     "label create response",
			},
			{
				ExpectedTool:   "gitlab_label",
				ExpectedAction: "delete",
				RequiredParams: []string{"project_id", "name"},
				OptionalParams: []string{"confirm"},
				Destructive:    true,
				Simulation:     "label delete response",
			},
		},
	}
	task := taskFromCase(evalCase)
	steps := taskSteps(task)
	if len(steps) != 2 {
		t.Fatalf("len(taskSteps()) = %d, want 2", len(steps))
	}
	if task.ExpectedTool != "gitlab_label" || task.ExpectedAction != "create" {
		t.Fatalf("first route = %s/%s, want gitlab_label/create", task.ExpectedTool, task.ExpectedAction)
	}
	if got := joinStrings(steps[0].RequiredParams); got != "project_id,name,color" {
		t.Fatalf("step 1 required params = %q, want project_id,name,color", got)
	}
	if got := joinStrings(steps[1].OptionalParams); got != "confirm" {
		t.Fatalf("step 2 optional params = %q, want confirm", got)
	}
	if !steps[1].Destructive || steps[1].Simulation != "label delete response" {
		t.Fatalf("step 2 destructive/simulation = %t/%q, want true/label delete response", steps[1].Destructive, steps[1].Simulation)
	}
}

// TestExpectedStepTypedAssertionFields_ProjectIntoTaskSteps verifies that
// the new typed-assertion fields on ExpectedStep (ForbiddenParams,
// AllowedRepairs, ProducedValues, OptionalStep) survive the case-to-task
// projection so the runtime validators and metric aggregators can consume
// them.
//
// The test feeds an EvalCase with each new field populated and asserts the
// projected task step retains every value. This protects the typed
// assertion expansion (Phase 11) from regressing through the projection.
func TestExpectedStepTypedAssertionFields_ProjectIntoTaskSteps(t *testing.T) {
	evalCase := EvalCase{
		ID:     "MT-CASE-ASSERTIONS",
		Prompt: "Get a project without unsupported parameters.",
		Steps: []ExpectedStep{{
			ExpectedTool:    "gitlab_project",
			ExpectedAction:  "get",
			RequiredParams:  []string{"project_id"},
			OptionalParams:  []string{"statistics"},
			ForbiddenParams: []string{"token"},
			OptionalStep:    true,
			AllowedRepairs:  []string{"move project_id into params"},
			ProducedValues:  []string{"project_id"},
		}},
		Assertions: []CaseAssertion{{Type: CaseAssertionForbiddenParams, Step: 1, Required: true}},
	}

	task := taskFromCase(evalCase)
	step := task.Steps[0]
	if got := joinStrings(step.ForbiddenParams); got != "token" {
		t.Fatalf("forbidden params = %q, want token", got)
	}
	if got := joinStrings(step.AllowedRepairs); got != "move project_id into params" {
		t.Fatalf("allowed repairs = %q, want move project_id into params", got)
	}
	if got := joinStrings(step.ProducedValues); got != "project_id" {
		t.Fatalf("produced values = %q, want project_id", got)
	}
	if !step.OptionalStep {
		t.Fatal("optional step = false, want true")
	}
}

// TestCaseAssertionTypes_CoverPhaseElevenRules verifies that every
// CaseAssertionType constant introduced for the Phase 11 typed-assertion
// rules is non-empty so the runtime can dispatch on each rule.
//
// The test enumerates the expected assertion type constants, one subtest
// per constant, and asserts none of them is the empty string. This protects
// downstream registries from silently dropping a rule when a future rename
// leaves an empty constant in place.
func TestCaseAssertionTypes_CoverPhaseElevenRules(t *testing.T) {
	want := []struct {
		name  string
		value CaseAssertionType
	}{
		{"expected_action", CaseAssertionExpectedAction},
		{"required_params", CaseAssertionRequiredParams},
		{"optional_params", CaseAssertionOptionalParams},
		{"forbidden_params", CaseAssertionForbiddenParams},
		{"destructive_confirm", CaseAssertionDestructiveConfirm},
		{"output_contains", CaseAssertionOutputContains},
		{"produced_value", CaseAssertionProducedValue},
		{"no_extra_tool_call", CaseAssertionNoExtraToolCall},
		{"allow_repair", CaseAssertionAllowRepair},
	}
	for _, tc := range want {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value == "" {
				t.Fatalf("empty assertion type in %v", want)
			}
		})
	}
}

// joinStrings returns values joined by a single comma, used by the case-type
// tests to produce stable comparison strings for slice fields.
func joinStrings(values []string) string {
	return strings.Join(values, ",")
}
