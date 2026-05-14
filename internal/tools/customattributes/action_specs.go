package customattributes

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for custom attribute tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		customAttributeReadSpec("custom_attr_list", toolutil.RouteAction(client, List), "gitlab_list_custom_attributes"),
		customAttributeReadSpec("custom_attr_get", toolutil.RouteAction(client, Get), "gitlab_get_custom_attribute"),
		customAttributeSetSpec(client),
		customAttributeDeleteSpec("custom_attr_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_custom_attribute"),
	}
}

func customAttributeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := customAttributeOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func customAttributeSetSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualIdempotent := false
	options := customAttributeOptions("gitlab_set_custom_attribute")
	options.Idempotent = true
	options.IndividualTool.AnnotationOverrides.Idempotent = &individualIdempotent
	return toolutil.NewActionSpec("custom_attr_set", toolutil.RouteAction(client, Set), options)
}

func customAttributeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := customAttributeOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func customAttributeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "custom-attribute"},
		OpenWorld:      true,
		OwnerPackage:   "customattributes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
