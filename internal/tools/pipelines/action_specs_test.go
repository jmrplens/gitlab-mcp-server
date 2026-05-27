package pipelines

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestActionSpecs_WaitGuidance verifies pipeline wait metadata points callers
// to pipeline IDs and away from MR auto-merge workflows.
func TestActionSpecs_WaitGuidance(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	byTool := pipelineSpecsByTool(t, ActionSpecs(client))

	wait := byTool["gitlab_pipeline_wait"]
	if !strings.Contains(wait.Usage, "existing pipeline_id") || !strings.Contains(wait.Usage, "merge_request.merge") {
		t.Fatalf("gitlab_pipeline_wait Usage = %q, want pipeline_id and merge_request guidance", wait.Usage)
	}
	if !slices.Contains(wait.Aliases, "wait for pipeline") {
		t.Fatalf("gitlab_pipeline_wait Aliases = %v, want wait for pipeline", wait.Aliases)
	}
	if !slices.Contains(wait.RelatedActions, "merge_request.merge") {
		t.Fatalf("gitlab_pipeline_wait RelatedActions = %v, want merge_request.merge", wait.RelatedActions)
	}
	if guidance := wait.ParameterGuidance["pipeline_id"]; guidance.SemanticRole != "pipeline_identifier" || !strings.Contains(guidance.ValueSource, "merge_request.pipelines") {
		t.Fatalf("gitlab_pipeline_wait pipeline_id guidance = %+v, want pipeline source hint", guidance)
	}
}

// TestActionSpecs_PrimaryMetadata verifies richer metadata for core pipeline actions.
func TestActionSpecs_PrimaryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	byTool := pipelineSpecsByTool(t, ActionSpecs(client))

	list := byTool["gitlab_pipeline_list"]
	if !slices.Contains(list.Aliases, "list pipelines") {
		t.Fatalf("gitlab_pipeline_list Aliases = %v, want list pipelines", list.Aliases)
	}
	if guidance := list.ParameterGuidance["status"]; guidance.SemanticRole != "pipeline_status_filter" {
		t.Fatalf("gitlab_pipeline_list status guidance = %+v, want pipeline_status_filter", guidance)
	}

	get := byTool["gitlab_pipeline_get"]
	if !strings.Contains(get.Usage, "project_id and pipeline_id") {
		t.Fatalf("gitlab_pipeline_get Usage = %q", get.Usage)
	}
	if guidance := get.ParameterGuidance["pipeline_id"]; guidance.SemanticRole != "pipeline_identifier" {
		t.Fatalf("gitlab_pipeline_get pipeline_id guidance = %+v, want pipeline_identifier", guidance)
	}

	create := byTool["gitlab_pipeline_create"]
	if !slices.Contains(create.Aliases, "run pipeline") {
		t.Fatalf("gitlab_pipeline_create Aliases = %v, want run pipeline", create.Aliases)
	}
	if guidance := create.ParameterGuidance["ref"]; guidance.SemanticRole != "git_ref" {
		t.Fatalf("gitlab_pipeline_create ref guidance = %+v, want git_ref", guidance)
	}
}
