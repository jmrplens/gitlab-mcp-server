package mergerequests

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestActionSpecs_MergePipelineGuidance verifies MR merge metadata separates
// auto-merge requests from pipeline waiting workflows.
func TestActionSpecs_MergePipelineGuidance(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	byTool := mergeRequestSpecsByTool(t, ActionSpecs(client))

	merge := byTool["gitlab_mr_merge"]
	if !strings.Contains(merge.Usage, "auto_merge=true") || !strings.Contains(merge.Usage, "pipeline.wait") {
		t.Fatalf("gitlab_mr_merge Usage = %q, want auto_merge and pipeline.wait guidance", merge.Usage)
	}
	if !slices.Contains(merge.Aliases, "merge when pipeline succeeds") {
		t.Fatalf("gitlab_mr_merge Aliases = %v, want merge when pipeline succeeds", merge.Aliases)
	}
	if !slices.Contains(merge.RelatedActions, "pipeline.wait") {
		t.Fatalf("gitlab_mr_merge RelatedActions = %v, want pipeline.wait", merge.RelatedActions)
	}
	if guidance := merge.ParameterGuidance["auto_merge"]; guidance.SemanticRole != "merge_scheduling" || !strings.Contains(guidance.ValueSource, "pipeline succeeds") {
		t.Fatalf("gitlab_mr_merge auto_merge guidance = %+v, want merge scheduling hint", guidance)
	}

	pipelines := byTool["gitlab_mr_pipelines"]
	if !strings.Contains(pipelines.Usage, "returned pipeline_id") || !slices.Contains(pipelines.RelatedActions, "merge_request.merge") {
		t.Fatalf("gitlab_mr_pipelines metadata = usage %q related %v, want pipeline_id workflow guidance", pipelines.Usage, pipelines.RelatedActions)
	}
}
