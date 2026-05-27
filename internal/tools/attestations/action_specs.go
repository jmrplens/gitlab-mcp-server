package attestations

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for build attestation actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		attestationReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_attestations"),
		attestationReadSpec("download", toolutil.RouteAction(client, Download), "gitlab_download_attestation"),
	}
}

func attestationReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	usage := "List attestations for a subject digest in a project."
	guidance := map[string]toolutil.ParameterGuidance{}
	if name == "list" {
		guidance["project_id"] = toolutil.ParameterGuidance{SemanticRole: "scope_project", ValueSource: "Project ID or path owning the artifact.", ExampleBinding: `params.project_id:"group/project"`}
		guidance["subject_digest"] = toolutil.ParameterGuidance{SemanticRole: "artifact_digest", ValueSource: "OCI-style digest (for example sha256:...).", ExampleBinding: `params.subject_digest:"sha256:abc123"`}
	}
	if name == "download" {
		usage = "Download one attestation by IID from a project."
		guidance["project_id"] = toolutil.ParameterGuidance{SemanticRole: "scope_project", ValueSource: "Project ID or path owning the attestation.", ExampleBinding: `params.project_id:"group/project"`}
		guidance["attestation_iid"] = toolutil.ParameterGuidance{SemanticRole: "attestation_iid", ValueSource: "Attestation IID returned by list action.", ExampleBinding: "params.attestation_iid:1"}
	}

	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"security", "attestation"},
		Usage:             usage,
		RelatedActions:    []string{"project.get", "package.list"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		Edition:           "premium",
		OwnerPackage:      "attestations",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
