// case_prompt_test.go covers the prompt-template rendering pipeline for typed
// evaluation cases and the surface-specific task prompt builders.

package evaluator

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestRenderCasePrompt_RendersDefaultBranchAndProjectPath verifies that
// RenderCasePrompt substitutes project and branch values from the supplied
// FixtureOutput into the case prompt template.
//
// The test renders a template that references .Project.Path and
// .Branch.Default and asserts the output matches the expected concatenated
// string. This protects the prompt-rendering pipeline from regressing on
// the most common fixture values.
func TestRenderCasePrompt_RendersDefaultBranchAndProjectPath(t *testing.T) {
	evalCase := EvalCase{ID: "MT-PROMPT-001", PromptTemplate: CasePromptTemplate{Text: "Read {{ .Project.Path }} on {{ .Branch.Default }}."}}
	prompt, err := RenderCasePrompt(evalCase, FixtureOutput{"project_path": "my-org/project", "default_branch": "main"})
	if err != nil {
		t.Fatalf("RenderCasePrompt() error = %v", err)
	}
	if prompt != "Read my-org/project on main." {
		t.Fatalf("prompt = %q, want rendered project/default branch", prompt)
	}
}

// TestRenderCasePrompt_RendersPerAttemptValue verifies that RenderCasePrompt
// resolves .Values.* placeholders using the FixtureOutput map, allowing
// per-attempt values like branch_name to flow into the rendered prompt.
//
// The test supplies a single attempt-scoped branch name and asserts the
// rendered text uses that value. This protects the prompt-rendering path
// from regressing on values that live outside the typed prompt structs.
func TestRenderCasePrompt_RendersPerAttemptValue(t *testing.T) {
	evalCase := EvalCase{ID: "MT-PROMPT-002", PromptTemplate: CasePromptTemplate{Text: "Create branch {{ .Values.branch_name }}."}}
	prompt, err := RenderCasePrompt(evalCase, FixtureOutput{"branch_name": "eval-branch-model-1"})
	if err != nil {
		t.Fatalf("RenderCasePrompt() error = %v", err)
	}
	if prompt != "Create branch eval-branch-model-1." {
		t.Fatalf("prompt = %q, want per-attempt branch name", prompt)
	}
}

// TestRenderCasePrompt_MissingTemplateValueFails verifies that
// RenderCasePrompt returns a non-nil error when the template references a
// value that the FixtureOutput does not provide.
//
// The test renders a template referencing .Project.Path with a nil
// FixtureOutput and asserts the helper returns an error. This protects
// operators from silently rendering a prompt with empty placeholders when a
// fixture preparation step forgets to populate a required key.
func TestRenderCasePrompt_MissingTemplateValueFails(t *testing.T) {
	evalCase := EvalCase{ID: "MT-PROMPT-003", PromptTemplate: CasePromptTemplate{Text: "Read {{ .Project.Path }}."}}
	if _, err := RenderCasePrompt(evalCase, nil); err == nil {
		t.Fatal("RenderCasePrompt() error = nil, want missing data error")
	}
}

// TestAddPromptData_HandlesPointersAndNonStructValues verifies that
// addPromptData unwraps pointers to struct values, skips nil pointers, and
// ignores values that are neither pointers nor structs.
//
// The test exercises four input shapes (non-nil pointer, nil pointer, plain
// string, nil interface) and asserts only the first one contributes a map
// entry. This protects the prompt-template data builder from panicking or
// emitting empty entries for partial fixture data.
func TestAddPromptData_HandlesPointersAndNonStructValues(t *testing.T) {
	out := map[string]any{}
	addPromptData(out, "Project", &PromptProjectData{Path: "my-org/project"})
	addPromptData(out, "NilPointer", (*PromptProjectData)(nil))
	addPromptData(out, "Text", "not-a-struct")
	addPromptData(out, "Nil", nil)

	project, ok := out["Project"].(map[string]string)
	if !ok || project["Path"] != "my-org/project" {
		t.Fatalf("Project data = %#v, want pointer struct fields", out["Project"])
	}
	for _, name := range []string{"NilPointer", "Text", "Nil"} {
		t.Run(name, func(t *testing.T) {
			if _, exists := out[name]; exists {
				t.Fatalf("out[%q] exists in %#v, want skipped", name, out)
			}
		})
	}
}

// TestTaskPromptForSurface_DynamicDestructiveConfirmUsesTopLevel verifies
// that the dynamic-surface task prompt guides the model to use top-level
// confirm:true for destructive calls instead of params.confirm.
//
// The test renders the prompt for a destructive issue.delete step and
// asserts the rendered text contains "top-level confirm:true" and does not
// reference params.confirm. This protects the dynamic surface from
// regressions that would push the model toward an unsupported envelope.
func TestTaskPromptForSurface_DynamicDestructiveConfirmUsesTopLevel(t *testing.T) {
	task := evalTask{ID: "MT-PROMPT-004", Prompt: "Delete issue `42` from project `my-org/project`.", Steps: []evalStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.delete", RequiredParams: []string{"project_id", "issue_iid"}, OptionalParams: []string{"confirm"}, Destructive: true}}}
	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	if !strings.Contains(prompt, "top-level confirm:true") || strings.Contains(prompt, "params.confirm") {
		t.Fatalf("dynamic destructive prompt = %q, want top-level confirm guidance", prompt)
	}
}

// TestTaskPromptForSurface_DynamicFindFirstPromptUsesRenderedPrompt
// verifies that the dynamic-surface task prompt embeds the rendered case
// prompt and instructs the model to begin with gitlab_find_action, while
// suppressing the explicit project.get action name so the model relies on
// the discovery step.
//
// The test builds a typed case whose Case.Prompt matches the rendered
// template and asserts the dynamic prompt contains "rendered" and
// "first call gitlab_find_action" but no project.get reference. This
// protects the find-first guidance that anchors the dynamic surface.
func TestTaskPromptForSurface_DynamicFindFirstPromptUsesRenderedPrompt(t *testing.T) {
	evalCase := EvalCase{ID: "MT-PROMPT-005", PromptTemplate: CasePromptTemplate{Text: "Find project `{{ .Project.Path }}`."}}
	task := taskFromCase(evalCase)
	task.Prompt = ""
	task.Steps = []evalStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.get", RequiredParams: []string{"project_id"}}}
	task.Case.Prompt = "Find project `my-org/rendered`."
	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	hasRenderedValue := strings.Contains(prompt, "rendered")
	if !hasRenderedValue || !strings.Contains(prompt, "first call gitlab_find_action") || strings.Contains(prompt, "project.get") {
		t.Fatalf("dynamic find-first prompt rendered=%t prompt=%q", hasRenderedValue, prompt)
	}
}
