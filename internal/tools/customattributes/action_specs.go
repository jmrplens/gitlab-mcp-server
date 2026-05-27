package customattributes

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for custom attribute tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		customAttributeReadSpec("custom_attr_list", toolutil.RouteAction(client, List), "gitlab_list_custom_attributes"),
		customAttributeReadSpec("custom_attr_get", toolutil.RouteAction(client, Get), "gitlab_get_custom_attribute"),
		customAttributeSetSpec(client),
		customAttributeDeleteSpec("custom_attr_delete", toolutil.DestructiveAction(client, DeleteOutput), "gitlab_delete_custom_attribute"),
	}
}

func customAttributeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, customAttributeOptions(individualTool))
}

func customAttributeSetSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualIdempotent := false
	options := customAttributeOptions("gitlab_set_custom_attribute")
	options.IndividualTool.AnnotationOverrides.Idempotent = &individualIdempotent
	return toolutil.NewUpdateActionSpec("custom_attr_set", toolutil.RouteAction(client, Set), options)
}

func customAttributeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, customAttributeOptions(individualTool))
}

func customAttributeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute customattributes domain action.", Tags: []string{"admin", "custom-attribute"},
		OpenWorld:      true,
		OwnerPackage:   "customattributes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// DeleteOutput deletes a custom attribute and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted custom_attribute."}, nil
}
