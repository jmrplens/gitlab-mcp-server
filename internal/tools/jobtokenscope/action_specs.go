package jobtokenscope

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for CI/CD job token scope actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("token_scope_remove_project",
			toolutil.DestructiveVoidAction(client, RemoveProjectAllowlist),
			toolutil.ActionSpecOptions{
				Tags:           []string{"job", "token-scope", "allowlist"},
				Usage:          "Use when removing a target project from another project's CI job token inbound allowlist.",
				RelatedActions: []string{"job.token_scope_list_inbound", "job.token_scope_add_project", "job.token_scope_remove_group"},
				ParameterGuidance: map[string]toolutil.ParameterGuidance{
					"project_id": {
						SemanticRole:     "scope_owner_project",
						ValueSource:      "Owning project whose CI job token allowlist is being changed.",
						CommonConfusions: []string{"Do not use the project being removed as project_id."},
						ExampleBinding:   "Remove project ID 51 from allowlist of project 1 => project_id=1.",
					},
					"target_project_id": {
						SemanticRole:     "target_project",
						ValueSource:      "Project being removed from or added to the allowlist.",
						CommonConfusions: []string{"Do not put the allowlist owner project here."},
						ExampleBinding:   "Remove project ID 51 from allowlist of project 1 => target_project_id=51.",
					},
				},
				Destructive:    true,
				Idempotent:     true,
				OpenWorld:      true,
				OwnerPackage:   "jobtokenscope",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_remove_project_job_token_allowlist", Title: toolutil.TitleFromName("gitlab_remove_project_job_token_allowlist")},
			}),
	}
}
