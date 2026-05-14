package mergerequests

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("create",
			toolutil.RouteAction(client, Create),
			toolutil.ActionSpecOptions{
				Tags:           []string{"merge-request", "branch"},
				Usage:          "Use to open a merge request from a source branch into the target branch in a project.",
				RelatedActions: []string{"merge_request.get", "merge_request.list", "branch.create", "project.get"},
				ParameterGuidance: map[string]toolutil.ParameterGuidance{
					"source_branch": {
						SemanticRole:     "source_branch",
						ValueSource:      "Branch named after 'from'.",
						CommonConfusions: []string{"Do not use ref, tag_name, target_branch, or value for the source branch."},
						ExampleBinding:   "from feature/eval into main => source_branch=feature/eval.",
					},
					"target_branch": {
						SemanticRole:     "target_branch",
						ValueSource:      "Branch named after 'into' or the merge target.",
						CommonConfusions: []string{"Do not use source_branch, ref, tag_name, or to for the target branch."},
						ExampleBinding:   "from feature/eval into main => target_branch=main.",
					},
				},
				OpenWorld:      true,
				OwnerPackage:   "mergerequests",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_mr_create", Title: toolutil.TitleFromName("gitlab_mr_create")},
			}),
	}
}
