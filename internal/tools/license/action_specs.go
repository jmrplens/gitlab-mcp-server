package license

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for license actions exposed as
// MCP tools. The get, add, and delete routes are projected into the
// dynamic, meta, individual, and audit surfaces by the action catalog
// (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_get_license — read the currently installed GitLab license.
		licenseReadSpec("license_get", toolutil.RouteAction(client, Get), "gitlab_get_license"),
		// gitlab_add_license — install a new GitLab license from a base64 payload.
		licenseCreateSpec("license_add", toolutil.RouteAction(client, Add), "gitlab_add_license"),
		// gitlab_delete_license — remove an installed license (destructive).
		licenseDeleteSpec("license_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_license"),
	}
}

// deleteOutput adapts the package's [Delete] handler to the
// [toolutil.DestructiveAction] contract, returning a structured success
// result for the destructive confirm flow.
func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted license."}, nil
}

// licenseReadSpec builds a read-only [toolutil.ActionSpec] for a
// license action using the package's default [licenseOptions].
func licenseReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, licenseOptions(name, individualTool))
}

// licenseCreateSpec builds a create-style [toolutil.ActionSpec] for a
// license action using the package's default [licenseOptions].
func licenseCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, licenseOptions(name, individualTool))
}

// licenseDeleteSpec builds a destructive [toolutil.ActionSpec] for a
// license action using the package's default [licenseOptions].
func licenseDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, licenseOptions(name, individualTool))
}

// licenseOptions returns the base [toolutil.ActionSpecOptions] for a
// license action and customises the Usage/ParameterGuidance for the
// add and delete individual tools.
func licenseOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "Get the currently installed GitLab license details."
	guidance := map[string]toolutil.ParameterGuidance{}
	if actionName == "license_add" {
		usage = "Add or replace the GitLab instance license using the encoded license payload."
		guidance["license"] = toolutil.ParameterGuidance{
			SemanticRole:   "license_payload",
			ValueSource:    "License payload value provided by administrators (typically encoded text).",
			ExampleBinding: `params.license:"base64-license-data"`,
		}
	}
	if actionName == "license_delete" {
		usage = "Delete an installed GitLab license by ID."
		guidance["id"] = toolutil.ParameterGuidance{
			SemanticRole:   "license_id",
			ValueSource:    "License numeric ID returned by get/add license operations.",
			ExampleBinding: "params.id:1",
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"admin", "license"},
		Usage:             usage,
		RelatedActions:    []string{"admin.settings_get"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "license",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
