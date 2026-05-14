package alertmanagement

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	options := alertMetricImageOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func alertMetricImageCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, alertMetricImageOptions(individualTool))
}

func alertMetricImageUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := alertMetricImageOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func alertMetricImageDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := alertMetricImageOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func alertMetricImageOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"alert", "metric-image"},
		OpenWorld:      true,
		OwnerPackage:   "alertmanagement",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
