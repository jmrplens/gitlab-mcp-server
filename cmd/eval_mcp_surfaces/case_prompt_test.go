package main

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

func TestTaskPromptForSurface_DynamicDestructiveConfirmUsesTopLevel(t *testing.T) {
	task := evalTask{ID: "MT-PROMPT-004", Prompt: "Delete issue `42` from project `my-org/project`.", Steps: []evalStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.delete", RequiredParams: []string{"project_id", "issue_iid"}, OptionalParams: []string{"confirm"}, Destructive: true}}}
	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	if !strings.Contains(prompt, "top-level confirm:true") || strings.Contains(prompt, "params.confirm") {
		t.Fatalf("dynamic destructive prompt = %q, want top-level confirm guidance", prompt)
	}
}

func TestTaskPromptForSurface_DynamicExactCallPreambleUsesRenderedPrompt(t *testing.T) {
	evalCase := EvalCase{ID: "MT-PROMPT-005", PromptTemplate: CasePromptTemplate{Text: "Find project `{{ .Project.Path }}`."}}
	task := taskFromCase(evalCase)
	task.Prompt = ""
	task.Steps = []evalStep{{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.get", RequiredParams: []string{"project_id"}}}
	task.Case.Prompt = "Find project `my-org/rendered`."
	prompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	hasAction := strings.Contains(prompt, "project.get")
	hasRenderedValue := strings.Contains(prompt, "rendered")
	if !hasAction || !hasRenderedValue {
		t.Fatalf("dynamic exact prompt hasAction=%t hasRenderedValue=%t prompt=%q", hasAction, hasRenderedValue, prompt)
	}
}
