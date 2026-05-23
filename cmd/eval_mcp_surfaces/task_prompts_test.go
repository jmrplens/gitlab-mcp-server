package main

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestTaskPromptForSurface_DynamicBridgeGuidance verifies dynamic prompts expose
// capability bridge tools without telling models to wrap those calls in execute.
func TestTaskPromptForSurface_DynamicBridgeGuidance(t *testing.T) {
	task := evalTask{ID: "MS-039", Prompt: "Read `gitlab://tools`.", Steps: []evalStep{{ExpectedTool: resourceReadTool, RequiredParams: []string{"uri"}}}}
	got := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	for _, want := range []string{"visible tools include gitlab_find_action, gitlab_execute_action", "Use bridge tools directly", "gitlab://tools"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dynamic prompt missing %q:\n%s", want, got)
		}
	}
}

// TestJoinNonEmpty_TrimAndSkipBlanks verifies prompt fragments are composed
// without introducing empty paragraphs.
func TestJoinNonEmpty_TrimAndSkipBlanks(t *testing.T) {
	if got := joinNonEmpty("|", " first ", " ", "second"); got != "first|second" {
		t.Fatalf("joinNonEmpty() = %q, want first|second", got)
	}
}

// TestDynamicWorkflowPlanPreamble_RendersOnlyExecutablePlans verifies multi-step
// dynamic workflows become compact action plans when all steps use execute.
func TestDynamicWorkflowPlanPreamble_RendersOnlyExecutablePlans(t *testing.T) {
	task := evalTask{Steps: []evalStep{
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "project.get", RequiredParams: []string{"project_id"}},
		{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "issue.delete", RequiredParams: []string{"project_id", "issue_iid"}, OptionalParams: []string{"confirm"}, Destructive: true},
	}}
	got := dynamicWorkflowPlanPreamble(task)
	for _, want := range []string{"Dynamic workflow plan:", "action=project.get", "action=issue.delete", "destructive_confirm=true"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dynamicWorkflowPlanPreamble() missing %q:\n%s", want, got)
		}
	}
	task.Steps[1].ExpectedTool = "gitlab_issue"
	nonDynamicPlan := dynamicWorkflowPlanPreamble(task)
	if nonDynamicPlan != "" {
		t.Fatalf("dynamicWorkflowPlanPreamble(non-dynamic) = %q, want empty", nonDynamicPlan)
	}
}

// TestDynamicExampleParamValue_UsesPromptMarkers verifies exact-call guidance
// binds role-sensitive parameters from natural-language prompts.
func TestDynamicExampleParamValue_UsesPromptMarkers(t *testing.T) {
	if got := dynamicExampleParamValue("repository.file_create", "file_path", "create file `docs/eval.md`"); got != "docs/eval.md" {
		t.Fatalf("dynamicExampleParamValue(file_path) = %v, want docs/eval.md", got)
	}
	if got := dynamicExampleParamValue("pipeline.schedule_create", "active", "create inactive schedule `nightly`"); got != false {
		t.Fatalf("dynamicExampleParamValue(active) = %v, want false", got)
	}
	if got := dynamicWorkflowOptionalParams([]string{"confirm", "per_page"}); len(got) != 1 || got[0] != "per_page" {
		t.Fatalf("dynamicWorkflowOptionalParams() = %v, want per_page only", got)
	}
}

// TestDynamicConfirmPrompt_RewritesMetaConfirmGuidance verifies destructive
// instructions are converted to the dynamic top-level confirm shape.
func TestDynamicConfirmPrompt_RewritesMetaConfirmGuidance(t *testing.T) {
	got := dynamicConfirmPrompt("Include confirm:true in params for every destructive tool call and confirm must be inside params, never a top-level field. Delete with params.confirm=true.")
	if !strings.Contains(got, "top-level confirm:true") || strings.Contains(got, "confirm must be inside params") {
		t.Fatalf("dynamicConfirmPrompt() = %q, want top-level guidance", got)
	}
}

// TestTaskPrompt_IssueLinkConfirmationStaysSurfaceSpecific verifies shared
// prompts use params.confirm until dynamic rewriting changes the call shape.
func TestTaskPrompt_IssueLinkConfirmationStaysSurfaceSpecific(t *testing.T) {
	task := evalTask{ID: "MS-link", Prompt: "Run issue link CRUD.", Steps: []evalStep{
		{ExpectedTool: "gitlab_issue", ExpectedAction: actionIssueCreate},
		{ExpectedTool: "gitlab_issue", ExpectedAction: "link_create"},
	}}
	metaPrompt := taskPromptForSurface(task, config.ToolSurfaceMeta)
	if !strings.Contains(metaPrompt, "with params.confirm=true") || strings.Contains(metaPrompt, "gitlab_execute_action") {
		t.Fatalf("meta prompt = %s", metaPrompt)
	}
	dynamicPrompt := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	if !strings.Contains(dynamicPrompt, "top-level confirm:true") {
		t.Fatalf("dynamic prompt = %s", dynamicPrompt)
	}
	if strings.Contains(dynamicPrompt, "params.confirm") {
		t.Fatalf("dynamic prompt kept params.confirm guidance: %s", dynamicPrompt)
	}
}

// TestCompactExactTaskPrompt_UsesExpectedToolName verifies compact exact prompts
// do not force unified gitlab when a split meta-tool is expected.
func TestCompactExactTaskPrompt_UsesExpectedToolName(t *testing.T) {
	task := evalTask{ID: "MT-job", Prompt: "Download attestation for project `1` job `2`."}
	step := evalStep{ExpectedTool: "gitlab_attestation", ExpectedAction: "attestation.download", RequiredParams: []string{"project_id", "job_id"}}
	got := compactExactTaskPrompt(task, "No", step)
	if !strings.Contains(got, "Use the gitlab_attestation tool once") {
		t.Fatalf("compact prompt = %s", got)
	}
}

// TestSchemaFirstTaskPrompt_RendersFallbackGuidance verifies unresolved exact
// params produce schema-first instructions instead of placeholder examples.
func TestSchemaFirstTaskPrompt_RendersFallbackGuidance(t *testing.T) {
	got := schemaFirstTaskPrompt(evalTask{ID: "MT-999", Prompt: "Find the thing."}, "no", evalStep{ExpectedTool: "", ExpectedAction: "project.get"})
	for _, want := range []string{"Task MT-999", "Do not use placeholder values", "call gitlab with action project.get"} {
		if !strings.Contains(got, want) {
			t.Fatalf("schemaFirstTaskPrompt() missing %q:\n%s", want, got)
		}
	}
}
