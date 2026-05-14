package deploytokens

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for deploy token actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("deploy_token_delete_project",
			toolutil.DestructiveVoidAction(client, DeleteProject),
			toolutil.ActionSpecOptions{
				Tags:           []string{"access", "deploy-token"},
				Usage:          "Use to delete a deploy token owned by a project; pass the deploy token ID, not another token type.",
				RelatedActions: []string{"access.deploy_token_list_project", "access.deploy_token_get_project", "access.deploy_token_create_project"},
				ParameterGuidance: map[string]toolutil.ParameterGuidance{
					"project_id": {
						SemanticRole: "scope_owner_project",
						ValueSource:  "Project that owns the deploy token.",
					},
					"deploy_token_id": {
						SemanticRole:     "deploy_token",
						ValueSource:      "Deploy token ID, not a project, deploy key, personal token, or runner ID.",
						CommonConfusions: []string{"Do not use deploy_key_id or token_id for project deploy token deletion."},
					},
				},
				Destructive:    true,
				Idempotent:     true,
				OpenWorld:      true,
				OwnerPackage:   "deploytokens",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_deploy_token_delete_project", Title: toolutil.TitleFromName("gitlab_deploy_token_delete_project")},
			}),
	}
}
