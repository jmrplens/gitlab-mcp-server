package cilint

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionPipelineCreate = "pipeline.create"

// ActionSpecs returns canonical specs for CI lint actions exposed as MCP
// tools. Both routes mirror the non-deprecated client-go ValidateService
// methods one-to-one: lint maps to ProjectNamespaceLint (validate arbitrary
// YAML in a project namespace context) and lint_project maps to ProjectLint
// (validate a project's committed .gitlab-ci.yml). The routes are projected
// into the dynamic, meta, individual, and audit surfaces by the action
// catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_ci_lint — validate arbitrary .gitlab-ci.yml content within a project namespace.
		ciLintSpec("lint", toolutil.RouteAction(client, LintContent), "gitlab_ci_lint"),
		// gitlab_ci_lint_project — validate a project's committed CI configuration at a ref.
		ciLintSpec("lint_project", toolutil.RouteAction(client, LintProject), "gitlab_ci_lint_project"),
	}
}

// ciLintSpec builds a read-only [toolutil.ActionSpec] for a CI lint action and
// fills in non-generic discovery metadata: an action-specific Usage sentence,
// distinctive natural-language Aliases phrased around CI YAML validation, the
// canonical RelatedActions that callers reach for before or after linting, and
// an IndividualTool.Description in the "Returns: … See also: …" form. The two
// CI lint tools differ in whether they validate inline YAML or a committed
// file, so their metadata is set independently rather than from a shared
// generic placeholder (1:1 audit R-META).
func ciLintSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"template", "ci", "lint"},
		RelatedActions: []string{"template.ci_yml_get", actionPipelineCreate, "repository.file_get"},
		OpenWorld:      true,
		OwnerPackage:   "cilint",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch individualTool {
	case "gitlab_ci_lint":
		options.Usage = "Validate a .gitlab-ci.yml snippet against a project's namespace without committing it. Use when the prompt provides raw CI YAML to check, or to confirm edits before pushing; pass content with the YAML and project_id for include/component resolution, and set dry_run to simulate pipeline creation."
		options.Aliases = []string{"validate ci yaml", "lint ci config", "check gitlab-ci.yml syntax", "validate pipeline yaml", "test ci configuration"}
		options.RelatedActions = []string{"template.lint_project", actionPipelineCreate, "ci_catalog.list"}
		options.IndividualTool.Description = "Validate inline .gitlab-ci.yml content within a project namespace. Returns: validity flag, errors, warnings, the merged YAML, and resolved includes. See also: gitlab_ci_lint_project, gitlab_pipeline_create, gitlab_list_catalog_resources."
	case "gitlab_ci_lint_project":
		options.Usage = "Validate the .gitlab-ci.yml already committed to a project at a given ref. Use when the prompt names a project rather than supplying YAML; pass project_id and optionally content_ref/ref to choose the branch or tag, and dry_run to simulate pipeline creation for that ref."
		options.Aliases = []string{"validate project ci config", "lint committed gitlab-ci.yml", "check project pipeline yaml", "validate ci file in repo", "lint project ci yaml"}
		options.RelatedActions = []string{"template.lint", actionPipelineCreate, "repository.file_get"}
		options.IndividualTool.Description = "Validate a project's committed .gitlab-ci.yml at a branch or tag. Returns: validity flag, errors, warnings, the merged YAML, and resolved includes. See also: gitlab_ci_lint, gitlab_pipeline_create, gitlab_file_get."
	}

	return toolutil.NewReadActionSpec(name, route, options)
}
