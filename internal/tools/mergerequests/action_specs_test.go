package mergerequests

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
		t.Run(tool, func(t *testing.T) {
			spec, ok := byTool[tool]
			if !ok {
				t.Fatal("missing action spec")
			}
			if u := strings.ToLower(strings.TrimSpace(spec.Usage)); u == "" || strings.HasPrefix(u, "use to execute") {
				t.Errorf("generic or empty Usage = %q", spec.Usage)
			}
			if !hasNaturalLanguageAlias(spec, tool) {
				t.Errorf("aliases lack a natural-language entry: %v", spec.Aliases)
			}
			if len(spec.RelatedActions) == 0 {
				t.Error("RelatedActions is empty")
			}
			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("individual description lacks Returns/See also: %q", desc)
			}
		})
	}
}

// TestMergeRequestGetRoute_OnlyANotFoundBecomesTheCard holds the get route's
// wrapper to the one status it rewrites: a 404 is answered with the not-found
// card and no error, while any other failure stays the error GitLab caused,
// since a card saying "verify the IID" over a 500 sends the caller to check a
// number that was right.
func TestMergeRequestGetRoute_OnlyANotFoundBecomesTheCard(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantCard bool
	}{
		{"404 becomes the card", http.StatusNotFound, true},
		{"500 stays an error", http.StatusInternalServerError, false},
		{"403 stays an error", http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, `{"message":"as GitLab said"}`)
			}))
			byTool := mergeRequestSpecsByTool(t, ActionSpecs(client))
			result, err := byTool["gitlab_mr_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "merge_request_iid": 5})
			card, isCard := result.(mergeRequestNotFoundOutput)
			switch {
			case tt.wantCard && (err != nil || !isCard):
				t.Fatalf("route answered (%T, %v), want the not-found card and no error", result, err)
			case tt.wantCard && card.Identifier != "!5 in project 42":
				t.Errorf("card identifier = %q, want the IID and project the caller named", card.Identifier)
			case !tt.wantCard && (err == nil || isCard):
				t.Fatalf("route answered (%T, %v), want GitLab's %d reported as an error", result, err, tt.status)
			case !tt.wantCard && !strings.Contains(err.Error(), "as GitLab said"):
				t.Errorf("route error = %q, want GitLab's message", err)
			}
		})
	}
}

// TestDeleteDependencyOutput_NamesTheDependencyItDeleted holds the delete
// confirmation to the id it was given as the dependency's own, since the
// message used to call that number the blocking merge request, which is the
// other id a caller could have sent and the one GitLab would answer 404 to.
func TestDeleteDependencyOutput_NamesTheDependencyItDeleted(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/merge_requests/1/blocks/57" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := DeleteDependencyOutput(t.Context(), client, DeleteDependencyInput{ProjectID: "42", MRIID: 1, BlockingMergeRequestID: 57})
	if err != nil {
		t.Fatalf("DeleteDependencyOutput() unexpected error: %v", err)
	}
	if want := "Successfully deleted dependency 57 from MR !1 in project 42."; out.Status != "success" || out.Message != want {
		t.Errorf("DeleteDependencyOutput() = %+v, want status success and message %q", out, want)
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
