package evaluator

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

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

func TestRenderCasePrompt_MissingTemplateValueFails(t *testing.T) {
	evalCase := EvalCase{ID: "MT-PROMPT-003", PromptTemplate: CasePromptTemplate{Text: "Read {{ .Project.Path }}."}}
	if _, err := RenderCasePrompt(evalCase, nil); err == nil {
		t.Fatal("RenderCasePrompt() error = nil, want missing data error")
	}
}

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
		if _, exists := out[name]; exists {
			t.Fatalf("out[%q] exists in %#v, want skipped", name, out)
		}
	}
}

func TestTaskPromptForSurface_DynamicDestructiveConfirmUsesTopLevel(t *testing.T) {
	task := evalTask{ID: "MT-PROMPT-004", Prompt: "Delete issue `42` from project `my-org/project`.", Steps: []evalStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.delete", RequiredParams: []string{"project_id", "issue_iid"}, OptionalParams: []string{"confirm"}, Destructive: true}}}
	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	if !strings.Contains(prompt, "top-level confirm:true") || strings.Contains(prompt, "params.confirm") {
		t.Fatalf("dynamic destructive prompt = %q, want top-level confirm guidance", prompt)
	}
}

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
