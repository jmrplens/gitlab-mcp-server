package evaluator

import (
	"strings"
	"testing"
)

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

func TestCaseAssertionTypes_CoverPhaseElevenRules(t *testing.T) {
	want := []CaseAssertionType{
		CaseAssertionExpectedAction,
		CaseAssertionRequiredParams,
		CaseAssertionOptionalParams,
		CaseAssertionForbiddenParams,
		CaseAssertionDestructiveConfirm,
		CaseAssertionOutputContains,
		CaseAssertionProducedValue,
		CaseAssertionNoExtraToolCall,
		CaseAssertionAllowRepair,
	}
	for _, assertionType := range want {
		if assertionType == "" {
			t.Fatalf("empty assertion type in %v", want)
		}
	}
}

func joinStrings(values []string) string {
	return strings.Join(values, ",")
}
