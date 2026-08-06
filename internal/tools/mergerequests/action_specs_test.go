package mergerequests

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// mrMetaActions enumerates the 29 R-META-flagged merge request tools that must
// each carry action-specific Usage, natural-language aliases, canonical
// related actions, and a "Returns: … See also: …" individual-tool description.
var mrMetaActions = []string{
	"gitlab_mr_approve", "gitlab_mr_cancel_auto_merge", "gitlab_mr_commits", "gitlab_mr_create",
	"gitlab_mr_create_pipeline", "gitlab_mr_create_todo", "gitlab_mr_delete", "gitlab_mr_dependencies_list",
	"gitlab_mr_dependency_create", "gitlab_mr_dependency_delete", "gitlab_mr_get", "gitlab_mr_issues_closed",
	"gitlab_mr_list", "gitlab_mr_list_global", "gitlab_mr_list_group", "gitlab_mr_participants",
	"gitlab_mr_pipelines", "gitlab_mr_rebase", "gitlab_mr_related_issues", "gitlab_mr_reviewers",
	"gitlab_mr_add_spent_time", "gitlab_mr_reset_spent_time", "gitlab_mr_subscribe",
	"gitlab_mr_reset_time_estimate", "gitlab_mr_set_time_estimate", "gitlab_mr_time_stats",
	"gitlab_mr_unapprove", "gitlab_mr_unsubscribe", "gitlab_mr_update",
}

// TestActionSpecs_Metadata_NoGenericPlaceholders guards that every flagged
// merge request action exposes non-generic discovery metadata: a purpose
// Usage sentence, at least one natural-language alias beyond the tool name,
// non-empty related actions, and a "Returns: … See also: …" description.
func TestActionSpecs_Metadata_NoGenericPlaceholders(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	byTool := mergeRequestSpecsByTool(t, ActionSpecs(client))

	for _, tool := range mrMetaActions {
		spec, ok := byTool[tool]
		if !ok {
			t.Fatalf("missing action spec for %s", tool)
		}
		if u := strings.ToLower(strings.TrimSpace(spec.Usage)); u == "" || strings.HasPrefix(u, "use to execute") {
			t.Errorf("%s: generic or empty Usage = %q", tool, spec.Usage)
		}
		if !hasNaturalLanguageAlias(spec, tool) {
			t.Errorf("%s: aliases lack a natural-language entry: %v", tool, spec.Aliases)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: RelatedActions is empty", tool)
		}
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s: individual description lacks Returns/See also: %q", tool, desc)
		}
	}
}

// hasNaturalLanguageAlias reports whether the spec has at least one alias that
// is neither the canonical action name nor the individual tool name, matching
// the R-META aliases_only_toolname detector.
func hasNaturalLanguageAlias(spec toolutil.ActionSpec, tool string) bool {
	canonical := strings.ToLower(strings.TrimSpace(spec.Name))
	toolLower := strings.ToLower(strings.TrimSpace(tool))
	for _, alias := range spec.Aliases {
		a := strings.ToLower(strings.TrimSpace(alias))
		if a == "" || a == canonical || a == toolLower {
			continue
		}
		return true
	}
	return false
}
