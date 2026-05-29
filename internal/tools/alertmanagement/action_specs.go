package alertmanagement

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for alert metric image tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		alertMetricImageReadSpec("alert_metric_image_list", toolutil.RouteAction(client, ListMetricImages), "gitlab_list_alert_metric_images"),
		alertMetricImageCreateSpec("alert_metric_image_upload", toolutil.RouteAction(client, UploadMetricImage), "gitlab_upload_alert_metric_image"),
		alertMetricImageUpdateSpec("alert_metric_image_update", toolutil.RouteAction(client, UpdateMetricImage), "gitlab_update_alert_metric_image"),
		alertMetricImageDeleteSpec("alert_metric_image_delete", toolutil.DestructiveVoidAction(client, DeleteMetricImage), "gitlab_delete_alert_metric_image"),
	}
}

func alertMetricImageReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, alertMetricImageOptions(individualTool))
}

func alertMetricImageCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, alertMetricImageOptions(individualTool))
}

func alertMetricImageUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, alertMetricImageOptions(individualTool))
}

func alertMetricImageDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, alertMetricImageOptions(individualTool))
}

func alertMetricImageOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute alertmanagement domain action.", Tags: []string{"alert", "metric-image"},
		OpenWorld:      true,
		OwnerPackage:   "alertmanagement",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
