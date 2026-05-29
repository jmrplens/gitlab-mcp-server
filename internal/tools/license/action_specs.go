package license

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for license tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		licenseReadSpec("license_get", toolutil.RouteAction(client, Get), "gitlab_get_license"),
		licenseCreateSpec("license_add", toolutil.RouteAction(client, Add), "gitlab_add_license"),
		licenseDeleteSpec("license_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_license"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted license."}, nil
}

func licenseReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, licenseOptions(name, individualTool))
}

func licenseCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, licenseOptions(name, individualTool))
}

func licenseDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, licenseOptions(name, individualTool))
}

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
