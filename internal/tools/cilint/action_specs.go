package cilint

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const actionPipelineCreate = "pipeline.create"

// lintMetadata is everything that tells one CI lint action from the other: the
// individual tool's name, an action-specific Usage sentence, distinctive
// natural-language Aliases phrased around CI YAML validation, the canonical
// RelatedActions a caller reaches for before or after linting, and a
// Description in the "Returns: … See also: …" form. The two tools differ in
// whether they validate inline YAML or a committed file, so each states its own
// rather than sharing a generic placeholder (1:1 audit R-META).
//
// It is stated beside the route it belongs to instead of being looked up from
// the tool name, because the caller already knows which action it is building.
// The lookup that used to do it was a switch over the only two names
// [ActionSpecs] passes, so its last case had a branch no call could reach.
type lintMetadata struct {
	individualTool string
	usage          string
	aliases        []string
	related        []string
	description    string
}

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
		ciLintSpec("lint", toolutil.RouteAction(client, LintContent), lintMetadata{
			individualTool: "gitlab_ci_lint",
			usage:          "Validate a .gitlab-ci.yml snippet against a project's namespace without committing it. Use when the prompt provides raw CI YAML to check, or to confirm edits before pushing. Pass content with the YAML and project_id for include/component resolution, and set dry_run to simulate pipeline creation.",
			aliases:        []string{"validate ci yaml", "lint ci config", "check gitlab-ci.yml syntax", "validate pipeline yaml", "test ci configuration"},
			related:        []string{"template.lint_project", actionPipelineCreate, "ci_catalog.list"},
			description:    "Validate inline .gitlab-ci.yml content within a project namespace. Returns: validity flag, errors, warnings, the merged YAML, and resolved includes. See also: gitlab_ci_lint_project, gitlab_pipeline_create, gitlab_list_catalog_resources.",
		}),
		// gitlab_ci_lint_project — validate a project's committed CI configuration at a ref.
		ciLintSpec("lint_project", toolutil.RouteAction(client, LintProject), lintMetadata{
			individualTool: "gitlab_ci_lint_project",
			usage:          "Validate the .gitlab-ci.yml already committed to a project at a given ref. Use when the prompt names a project rather than supplying YAML. Pass project_id and optionally content_ref/ref to choose the branch or tag, and dry_run to simulate pipeline creation for that ref.",
			aliases:        []string{"validate project ci config", "lint committed gitlab-ci.yml", "check project pipeline yaml", "validate ci file in repo", "lint project ci yaml"},
			related:        []string{"template.lint", actionPipelineCreate, "repository.file_get"},
			description:    "Validate a project's committed .gitlab-ci.yml at a branch or tag. Returns: validity flag, errors, warnings, the merged YAML, and resolved includes. See also: gitlab_ci_lint, gitlab_pipeline_create, gitlab_file_get.",
		}),
	}
}

// ciLintSpec builds a read-only [toolutil.ActionSpec] for a CI lint action,
// joining the metadata that action states for itself to what every CI lint
// action shares: the tags it is discovered under, the open-world annotation,
// and the package that owns the handler.
func ciLintSpec(name string, route toolutil.ActionRoute, meta lintMetadata) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Usage:          meta.usage,
		Aliases:        meta.aliases,
		Tags:           []string{"template", "ci", "lint"},
		RelatedActions: meta.related,
		OpenWorld:      true,
		OwnerPackage:   "cilint",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        meta.individualTool,
			Title:       toolutil.TitleFromName(meta.individualTool),
			Description: meta.description,
		},
	})
}
