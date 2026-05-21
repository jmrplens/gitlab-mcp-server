package errortracking

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for error tracking tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		errorTrackingReadSpec("error_tracking_get_settings", toolutil.RouteAction(client, GetSettings), "gitlab_get_error_tracking_settings"),
		errorTrackingUpdateSpec("error_tracking_update_settings", toolutil.RouteAction(client, EnableDisable), "gitlab_enable_disable_error_tracking"),
		errorTrackingReadSpec("error_tracking_list", toolutil.RouteAction(client, ListClientKeys), "gitlab_list_error_tracking_client_keys"),
		errorTrackingCreateSpec("error_tracking_create", toolutil.RouteAction(client, CreateClientKey), "gitlab_create_error_tracking_client_key"),
		errorTrackingDeleteSpec("error_tracking_delete", toolutil.DestructiveAction(client, deleteClientKeyOutput), "gitlab_delete_error_tracking_client_key"),
	}
}

func errorTrackingReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, errorTrackingOptions(individualTool))
}

func errorTrackingCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, errorTrackingOptions(individualTool))
}

func errorTrackingUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, errorTrackingOptions(individualTool))
}

func errorTrackingDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, errorTrackingOptions(individualTool))
}

func errorTrackingOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"error-tracking"},
		OpenWorld:      true,
		OwnerPackage:   "errortracking",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
