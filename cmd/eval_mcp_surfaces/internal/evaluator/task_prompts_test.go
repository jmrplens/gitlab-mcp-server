package evaluator

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
	for _, want := range []string{"Use MCP capability bridge tools directly", "do not use bridge tools as a substitute for a required catalog action", "gitlab://tools"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dynamic prompt missing %q:\n%s", want, got)
		}
	}
}

func TestTaskPromptForSurface_DynamicRemoteURLDiscoveryGuidance(t *testing.T) {
	task := evalTask{ID: "MS-002", Prompt: "Resolve remote URL `https://gitlab.example.com/group/project.git` then inspect pipeline `1`.", Steps: []evalStep{{ExpectedTool: "gitlab_execute_action", ExpectedAction: "discover_project.resolve", RequiredParams: []string{"remote_url"}}, {ExpectedTool: "gitlab_execute_action", ExpectedAction: "pipeline.get", RequiredParams: []string{"project_id", "pipeline_id"}}}}
	got := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	for _, want := range []string{"first gitlab_find_action query for that discovery step must explicitly describe resolving the provided remote URL", "must use the project-discovery action with params.remote_url set to that exact URL"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dynamic prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "discover_project.resolve") {
		t.Fatalf("dynamic prompt leaked exact discovery action:\n%s", got)
	}
}

func TestTaskPromptForSurface_DynamicRemoteURLGuidanceIsScoped(t *testing.T) {
	task := evalTask{ID: "MT-002", Prompt: "Find project `my-org/tools/gitlab-mcp-server` and give me its ID and default branch.", Steps: []evalStep{{ExpectedTool: "gitlab_execute_action", ExpectedAction: "project.get", RequiredParams: []string{"project_id"}}}}
	got := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	forbidden := []string{"first gitlab_find_action query for that discovery step must explicitly describe resolving the provided remote URL", "must use the project-discovery action with params.remote_url set to that exact URL"}
	for _, text := range forbidden {
		if strings.Contains(got, text) {
			t.Fatalf("dynamic prompt unexpectedly included remote URL guidance %q:\n%s", text, got)
		}
	}
	if strings.Contains(got, "discover_project.resolve") {
		t.Fatalf("dynamic prompt unexpectedly leaked exact discovery action:\n%s", got)
	}
}

func TestTaskPromptForSurface_DynamicAvoidsPlaceholderRetries(t *testing.T) {
	task := evalTask{ID: "MS-012", Prompt: "Prepare an LLM-assisted release summary for project `my-org/tools/gitlab-mcp-server`: inspect releases, compare refs `main` and `v0.0.0-eval-ms`, then generate release notes.", Steps: []evalStep{{ExpectedTool: "gitlab_execute_action", ExpectedAction: "release.list", RequiredParams: []string{"project_id"}}, {ExpectedTool: "gitlab_execute_action", ExpectedAction: "repository.compare", RequiredParams: []string{"project_id", "from", "to"}}, {ExpectedTool: "gitlab_execute_action", ExpectedAction: "analyze.release_notes", RequiredParams: []string{"project_id", "from", "to"}}}}
	got := taskPromptForSurface(task, config.ToolSurfaceDynamic)
	if !strings.Contains(got, "Never use placeholder values like <to>, <from>, or <project_id> in retries") {
		t.Fatalf("dynamic prompt missing placeholder guidance:\n%s", got)
	}
	if !strings.Contains(got, "do not call gitlab_execute_action for an unrelated action just because it ranked higher") {
		t.Fatalf("dynamic prompt missing find retry guidance:\n%s", got)
	}
	if !strings.Contains(got, "keep the same two refs across the compare step and the final analyzer step") {
		t.Fatalf("dynamic prompt missing release compare guidance:\n%s", got)
	}
}

func TestDynamicExampleParamValue_CompareRefsExtractsFromAndTo(t *testing.T) {
	prompt := "Prepare an LLM-assisted release summary for project `my-org/tools/gitlab-mcp-server`: inspect releases, compare refs `main` and `v0.0.0-eval-ms`, then generate release notes."
	if got := dynamicExampleParamValue("repository.compare", "from", prompt); got != "main" {
		t.Fatalf("dynamicExampleParamValue(from) = %v, want main", got)
	}
	if got := dynamicExampleParamValue("repository.compare", "to", prompt); got != "v0.0.0-eval-ms" {
		t.Fatalf("dynamicExampleParamValue(to) = %v, want v0.0.0-eval-ms", got)
	}
	if got := dynamicExampleParamValue("analyze.release_notes", "from", prompt); got != "main" {
		t.Fatalf("dynamicExampleParamValue(analyze.from) = %v, want main", got)
	}
	if got := dynamicExampleParamValue("analyze.release_notes", "to", prompt); got != "v0.0.0-eval-ms" {
		t.Fatalf("dynamicExampleParamValue(analyze.to) = %v, want v0.0.0-eval-ms", got)
	}
}

// TestTaskForSurface_RewritesToolDetailResourceIDs verifies capability tasks use
// detail resource IDs from the active surface instead of dynamic-only IDs.
func TestTaskForSurface_RewritesToolDetailResourceIDs(t *testing.T) {
	evalCase, ok := CaseByID("MS-040")
	if !ok {
		t.Fatal("CaseByID(MS-040) = false")
	}
	task := taskFromCase(evalCase)

	metaTask := taskForSurface(task, config.ToolSurfaceMeta)
	if !strings.Contains(metaTask.Prompt, "`gitlab://tools/gitlab_project.get`") {
		t.Fatalf("meta prompt = %q, want meta project detail URI", metaTask.Prompt)
	}
	if strings.Contains(metaTask.Prompt, dynamicProjectGetToolDetailURI) {
		t.Fatalf("meta prompt kept dynamic project detail URI: %q", metaTask.Prompt)
	}

	dynamicTask := taskForSurface(task, config.ToolSurfaceDynamic)
	if !strings.Contains(dynamicTask.Prompt, "`"+dynamicProjectGetToolDetailURI+"`") {
		t.Fatalf("dynamic prompt = %q, want dynamic project detail URI", dynamicTask.Prompt)
	}
}

// TestJoinNonEmpty_TrimAndSkipBlanks verifies prompt fragments are composed
// without introducing empty paragraphs.
func TestJoinNonEmpty_TrimAndSkipBlanks(t *testing.T) {
	if got := joinNonEmpty("|", " first ", " ", "second"); got != "first|second" {
		t.Fatalf("joinNonEmpty() = %q, want first|second", got)
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
	if !strings.Contains(dynamicPrompt, "first call gitlab_find_action") {
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
