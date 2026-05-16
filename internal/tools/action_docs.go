package tools

import "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

// ensureActionSpecDocs attaches default GitLab API documentation links to specs
// that do not already declare action-specific references.
func ensureActionSpecDocs(specs []toolutil.ActionSpec, toolName string) []toolutil.ActionSpec {
	if len(specs) == 0 {
		return nil
	}
	docs := gitLabAPIDocsForTool(toolName)
	if len(docs) == 0 {
		return specs
	}
	out := cloneActionSpecs(specs)
	for index := range out {
		if len(out[index].Docs) == 0 {
			out[index].Docs = toolutil.CloneDocumentationReferences(docs)
		}
	}
	return out
}

// gitLabAPIDocsForTool returns documentation references for a catalog meta-tool
// group, cloned so callers can safely attach them to specs.
func gitLabAPIDocsForTool(toolName string) []toolutil.DocumentationReference {
	return toolutil.CloneDocumentationReferences(gitLabAPIDocsByTool[toolName])
}

// gitLabAPIDocsByTool maps catalog meta-tool groups to the official GitLab API
// pages that describe the backing endpoints. The links are intentionally kept at
// group scope so every projected action can expose an external reference without
// duplicating URL literals in 160+ domain packages.
var gitLabAPIDocsByTool = map[string][]toolutil.DocumentationReference{
	"gitlab_access": {
		{Title: "Project access tokens API", URL: "https://docs.gitlab.com/api/project_access_tokens/"},
		{Title: "Deploy keys API", URL: "https://docs.gitlab.com/api/deploy_keys/"},
		{Title: "Deploy tokens API", URL: "https://docs.gitlab.com/api/deploy_tokens/"},
		{Title: "Access requests API", URL: "https://docs.gitlab.com/api/access_requests/"},
	},
	"gitlab_admin": {
		{Title: "Application settings API", URL: "https://docs.gitlab.com/api/settings/"},
		{Title: "Broadcast messages API", URL: "https://docs.gitlab.com/api/broadcast_messages/"},
		{Title: "System hooks API", URL: "https://docs.gitlab.com/api/system_hooks/"},
		{Title: "Topics API", URL: "https://docs.gitlab.com/api/topics/"},
	},
	"gitlab_attestation":           {{Title: "Package attestations API", URL: "https://docs.gitlab.com/api/package_attestations/"}},
	"gitlab_audit_event":           {{Title: "Audit events API", URL: "https://docs.gitlab.com/api/audit_events/"}},
	"gitlab_branch":                {{Title: "Branches API", URL: "https://docs.gitlab.com/api/branches/"}, {Title: "Protected branches API", URL: "https://docs.gitlab.com/api/protected_branches/"}},
	"gitlab_ci_catalog":            {{Title: "CI/CD catalog API", URL: "https://docs.gitlab.com/api/ci_catalog/"}},
	"gitlab_ci_variable":           {{Title: "Project variables API", URL: "https://docs.gitlab.com/api/project_level_variables/"}, {Title: "Group variables API", URL: "https://docs.gitlab.com/api/group_level_variables/"}, {Title: "Instance variables API", URL: "https://docs.gitlab.com/api/instance_level_ci_variables/"}},
	"gitlab_compliance_policy":     {{Title: "Security policies API", URL: "https://docs.gitlab.com/api/security_policy_configurations/"}},
	"gitlab_custom_emoji":          {{Title: "GraphQL custom emoji API", URL: "https://docs.gitlab.com/api/graphql/reference/"}},
	"gitlab_dependency":            {{Title: "Dependency list API", URL: "https://docs.gitlab.com/api/dependencies/"}},
	"gitlab_dora_metrics":          {{Title: "DORA metrics API", URL: "https://docs.gitlab.com/api/dora/metrics/"}},
	"gitlab_environment":           {{Title: "Environments API", URL: "https://docs.gitlab.com/api/environments/"}, {Title: "Deployments API", URL: "https://docs.gitlab.com/api/deployments/"}, {Title: "Protected environments API", URL: "https://docs.gitlab.com/api/protected_environments/"}},
	"gitlab_enterprise_user":       {{Title: "Enterprise users API", URL: "https://docs.gitlab.com/api/enterprise_users/"}},
	"gitlab_external_status_check": {{Title: "External status checks API", URL: "https://docs.gitlab.com/api/status_checks/"}},
	"gitlab_feature_flags":         {{Title: "Feature flags API", URL: "https://docs.gitlab.com/api/feature_flags/"}, {Title: "Feature flag user lists API", URL: "https://docs.gitlab.com/api/feature_flag_user_lists/"}},
	"gitlab_geo":                   {{Title: "Geo nodes API", URL: "https://docs.gitlab.com/api/geo_nodes/"}},
	"gitlab_group":                 {{Title: "Groups API", URL: "https://docs.gitlab.com/api/groups/"}, {Title: "Group badges API", URL: "https://docs.gitlab.com/api/group_badges/"}, {Title: "Group milestones API", URL: "https://docs.gitlab.com/api/group_milestones/"}, {Title: "Epics API", URL: "https://docs.gitlab.com/api/epics/"}},
	"gitlab_group_scim":            {{Title: "Group SCIM API", URL: "https://docs.gitlab.com/api/scim/"}},
	"gitlab_issue":                 {{Title: "Issues API", URL: "https://docs.gitlab.com/api/issues/"}, {Title: "Issue links API", URL: "https://docs.gitlab.com/api/issue_links/"}, {Title: "Issue notes API", URL: "https://docs.gitlab.com/api/notes/"}, {Title: "Issue discussions API", URL: "https://docs.gitlab.com/api/discussions/"}},
	"gitlab_job":                   {{Title: "Jobs API", URL: "https://docs.gitlab.com/api/jobs/"}, {Title: "Job token scope API", URL: "https://docs.gitlab.com/api/project_job_token_scopes/"}},
	"gitlab_member_role":           {{Title: "Member roles API", URL: "https://docs.gitlab.com/api/member_roles/"}},
	"gitlab_merge_request":         {{Title: "Merge requests API", URL: "https://docs.gitlab.com/api/merge_requests/"}, {Title: "Merge request approvals API", URL: "https://docs.gitlab.com/api/merge_request_approvals/"}, {Title: "Resource state events API", URL: "https://docs.gitlab.com/api/resource_state_events/"}},
	"gitlab_merge_train":           {{Title: "Merge trains API", URL: "https://docs.gitlab.com/api/merge_trains/"}},
	"gitlab_model_registry":        {{Title: "Model registry API", URL: "https://docs.gitlab.com/api/model_registry/"}},
	"gitlab_mr_review":             {{Title: "Merge request notes API", URL: "https://docs.gitlab.com/api/notes/"}, {Title: "Merge request discussions API", URL: "https://docs.gitlab.com/api/discussions/"}, {Title: "Merge request changes API", URL: "https://docs.gitlab.com/api/merge_requests/"}},
	"gitlab_package":               {{Title: "Packages API", URL: "https://docs.gitlab.com/api/packages/"}, {Title: "Container registry API", URL: "https://docs.gitlab.com/api/container_registry/"}, {Title: "Protected packages API", URL: "https://docs.gitlab.com/api/protected_packages/"}},
	"gitlab_pipeline":              {{Title: "Pipelines API", URL: "https://docs.gitlab.com/api/pipelines/"}, {Title: "Pipeline schedules API", URL: "https://docs.gitlab.com/api/pipeline_schedules/"}, {Title: "Pipeline triggers API", URL: "https://docs.gitlab.com/api/pipeline_triggers/"}, {Title: "Resource groups API", URL: "https://docs.gitlab.com/api/resource_groups/"}},
	"gitlab_project":               {{Title: "Projects API", URL: "https://docs.gitlab.com/api/projects/"}, {Title: "Project hooks API", URL: "https://docs.gitlab.com/api/project_webhooks/"}, {Title: "Project badges API", URL: "https://docs.gitlab.com/api/project_badges/"}, {Title: "Project import/export API", URL: "https://docs.gitlab.com/api/project_import_export/"}},
	"gitlab_project_alias":         {{Title: "Project aliases API", URL: "https://docs.gitlab.com/api/project_aliases/"}},
	"gitlab_release":               {{Title: "Releases API", URL: "https://docs.gitlab.com/api/releases/"}, {Title: "Release links API", URL: "https://docs.gitlab.com/api/releases/links/"}},
	"gitlab_repository":            {{Title: "Repositories API", URL: "https://docs.gitlab.com/api/repositories/"}, {Title: "Repository files API", URL: "https://docs.gitlab.com/api/repository_files/"}, {Title: "Commits API", URL: "https://docs.gitlab.com/api/commits/"}},
	"gitlab_runner":                {{Title: "Runners API", URL: "https://docs.gitlab.com/api/runners/"}},
	"gitlab_search":                {{Title: "Search API", URL: "https://docs.gitlab.com/api/search/"}},
	"gitlab_security_finding":      {{Title: "Vulnerability findings API", URL: "https://docs.gitlab.com/api/vulnerability_findings/"}},
	"gitlab_snippet":               {{Title: "Snippets API", URL: "https://docs.gitlab.com/api/snippets/"}, {Title: "Snippet notes API", URL: "https://docs.gitlab.com/api/notes/"}, {Title: "Snippet discussions API", URL: "https://docs.gitlab.com/api/discussions/"}},
	"gitlab_storage_move":          {{Title: "Project repository storage moves API", URL: "https://docs.gitlab.com/api/project_repository_storage_moves/"}, {Title: "Group repository storage moves API", URL: "https://docs.gitlab.com/api/group_repository_storage_moves/"}},
	"gitlab_tag":                   {{Title: "Tags API", URL: "https://docs.gitlab.com/api/tags/"}, {Title: "Protected tags API", URL: "https://docs.gitlab.com/api/protected_tags/"}},
	"gitlab_template":              {{Title: "Templates API", URL: "https://docs.gitlab.com/api/templates/"}, {Title: "CI lint API", URL: "https://docs.gitlab.com/api/lint/"}},
	"gitlab_user":                  {{Title: "Users API", URL: "https://docs.gitlab.com/api/users/"}, {Title: "Todos API", URL: "https://docs.gitlab.com/api/todos/"}, {Title: "Events API", URL: "https://docs.gitlab.com/api/events/"}, {Title: "Namespaces API", URL: "https://docs.gitlab.com/api/namespaces/"}},
	"gitlab_vulnerability":         {{Title: "Vulnerabilities API", URL: "https://docs.gitlab.com/api/vulnerabilities/"}},
	"gitlab_wiki":                  {{Title: "Wikis API", URL: "https://docs.gitlab.com/api/wikis/"}},
}
