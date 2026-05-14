package adminspecs

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/alertmanagement"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/appearance"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/applications"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/appstatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/broadcastmessages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/bulkimports"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/clusteragents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customattributes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dbmigrations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dependencyproxy"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/errortracking"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/features"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/importservice"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/license"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/metadata"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/planlimits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securefiles"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/sidekiq"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/systemhooks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/terraformstates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/topics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/usagedata"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for gitlab_admin meta-tool actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		adminReadSpec("topic_list", toolutil.RouteAction(client, topics.List), "gitlab_list_topics"),
		adminReadSpec("topic_get", toolutil.RouteAction(client, topics.Get), "gitlab_get_topic"),
		adminCreateSpec("topic_create", toolutil.RouteAction(client, topics.Create), "gitlab_create_topic"),
		adminUpdateSpec("topic_update", toolutil.RouteAction(client, topics.Update), "gitlab_update_topic"),
		adminDeleteSpec("topic_delete", toolutil.DestructiveVoidAction(client, topics.Delete), "gitlab_delete_topic"),
		adminReadSpec("settings_get", toolutil.RouteAction(client, settings.Get), "gitlab_get_settings"),
		adminUpdateSpec("settings_update", toolutil.RouteAction(client, settings.Update), "gitlab_update_settings"),
		adminReadSpec("appearance_get", toolutil.RouteAction(client, appearance.Get), "gitlab_get_appearance"),
		adminUpdateSpec("appearance_update", toolutil.RouteAction(client, appearance.Update), "gitlab_update_appearance"),
		adminReadSpec("broadcast_message_list", toolutil.RouteAction(client, broadcastmessages.List), "gitlab_list_broadcast_messages"),
		adminReadSpec("broadcast_message_get", toolutil.RouteAction(client, broadcastmessages.Get), "gitlab_get_broadcast_message"),
		adminCreateSpec("broadcast_message_create", toolutil.RouteAction(client, broadcastmessages.Create), "gitlab_create_broadcast_message"),
		adminUpdateSpec("broadcast_message_update", toolutil.RouteAction(client, broadcastmessages.Update), "gitlab_update_broadcast_message"),
		adminDeleteSpec("broadcast_message_delete", toolutil.DestructiveVoidAction(client, broadcastmessages.Delete), "gitlab_delete_broadcast_message"),
		adminReadSpec("feature_list", toolutil.RouteAction(client, features.List), "gitlab_list_features"),
		adminReadSpec("feature_list_definitions", toolutil.RouteAction(client, features.ListDefinitions), "gitlab_list_feature_definitions"),
		adminUpdateCreateIndividualSpec("feature_set", features.SetRoute(client), "gitlab_set_feature_flag"),
		adminDeleteSpec("feature_delete", toolutil.DestructiveVoidAction(client, features.Delete), "gitlab_delete_feature_flag"),
		adminReadSpec("license_get", toolutil.RouteAction(client, license.Get), "gitlab_get_license"),
		adminCreateSpec("license_add", toolutil.RouteAction(client, license.Add), "gitlab_add_license"),
		adminDeleteSpec("license_delete", toolutil.DestructiveVoidAction(client, license.Delete), "gitlab_delete_license"),
		adminReadSpec("system_hook_list", toolutil.RouteAction(client, systemhooks.List), "gitlab_list_system_hooks"),
		adminReadSpec("system_hook_get", toolutil.RouteAction(client, systemhooks.Get), "gitlab_get_system_hook"),
		adminCreateSpec("system_hook_add", toolutil.RouteAction(client, systemhooks.Add), "gitlab_add_system_hook"),
		adminSystemHookTestSpec(client),
		adminDeleteSpec("system_hook_delete", toolutil.DestructiveVoidAction(client, systemhooks.Delete), "gitlab_delete_system_hook"),
		adminReadSpec("sidekiq_queue_metrics", toolutil.RouteAction(client, sidekiq.GetQueueMetrics), "gitlab_get_sidekiq_queue_metrics"),
		adminReadSpec("sidekiq_process_metrics", toolutil.RouteAction(client, sidekiq.GetProcessMetrics), "gitlab_get_sidekiq_process_metrics"),
		adminReadSpec("sidekiq_job_stats", toolutil.RouteAction(client, sidekiq.GetJobStats), "gitlab_get_sidekiq_job_stats"),
		adminReadSpec("sidekiq_compound_metrics", toolutil.RouteAction(client, sidekiq.GetCompoundMetrics), "gitlab_get_sidekiq_compound_metrics"),
		adminReadSpec("plan_limits_get", toolutil.RouteAction(client, planlimits.Get), "gitlab_get_plan_limits"),
		adminUpdateSpec("plan_limits_change", toolutil.RouteAction(client, planlimits.Change), "gitlab_change_plan_limits"),
		adminReadSpec("usage_data_service_ping", toolutil.RouteAction(client, usagedata.GetServicePing), "gitlab_get_service_ping"),
		adminReadSpec("usage_data_non_sql_metrics", toolutil.RouteAction(client, usagedata.GetNonSQLMetrics), "gitlab_get_non_sql_metrics"),
		adminReadSpec("usage_data_queries", toolutil.RouteAction(client, usagedata.GetQueries), "gitlab_get_usage_queries"),
		adminReadSpec("usage_data_metric_definitions", toolutil.RouteAction(client, usagedata.GetMetricDefinitions), "gitlab_get_metric_definitions"),
		adminCreateSpec("usage_data_track_event", toolutil.RouteAction(client, usagedata.TrackEvent), "gitlab_track_event"),
		adminCreateSpec("usage_data_track_events", toolutil.RouteAction(client, usagedata.TrackEvents), "gitlab_track_events"),
		adminDestructiveUpdateIndividualSpec("db_migration_mark", toolutil.DestructiveAction(client, dbmigrations.Mark), "gitlab_mark_migration"),
		adminReadSpec("application_list", toolutil.RouteAction(client, applications.List), "gitlab_list_applications"),
		adminCreateSpec("application_create", toolutil.RouteAction(client, applications.Create), "gitlab_create_application"),
		adminDeleteSpec("application_delete", toolutil.DestructiveVoidAction(client, applications.Delete), "gitlab_delete_application"),
		adminReadSpec("app_statistics_get", toolutil.RouteAction(client, appstatistics.Get), "gitlab_get_application_statistics"),
		adminReadSpec("metadata_get", toolutil.RouteAction(client, metadata.Get), "gitlab_get_metadata"),
		adminReadSpec("custom_attr_list", toolutil.RouteAction(client, customattributes.List), "gitlab_list_custom_attributes"),
		adminReadSpec("custom_attr_get", toolutil.RouteAction(client, customattributes.Get), "gitlab_get_custom_attribute"),
		adminUpdateCreateIndividualSpec("custom_attr_set", toolutil.RouteAction(client, customattributes.Set), "gitlab_set_custom_attribute"),
		adminDeleteSpec("custom_attr_delete", toolutil.DestructiveVoidAction(client, customattributes.Delete), "gitlab_delete_custom_attribute"),
		adminCreateSpec("bulk_import_start", toolutil.RouteAction(client, bulkimports.StartMigration), "gitlab_start_bulk_import"),
		adminReadSpec("bulk_import_list", toolutil.RouteAction(client, bulkimports.List), "gitlab_list_bulk_imports"),
		adminReadSpec("bulk_import_get", toolutil.RouteAction(client, bulkimports.Get), "gitlab_get_bulk_import"),
		adminUpdateSpec("bulk_import_cancel", toolutil.RouteAction(client, bulkimports.Cancel), "gitlab_cancel_bulk_import"),
		adminReadSpec("bulk_import_entity_list", toolutil.RouteAction(client, bulkimports.ListEntities), "gitlab_list_bulk_import_entities"),
		adminReadSpec("bulk_import_entity_get", toolutil.RouteAction(client, bulkimports.GetEntity), "gitlab_get_bulk_import_entity"),
		adminReadSpec("bulk_import_entity_failures", toolutil.RouteAction(client, bulkimports.ListEntityFailures), "gitlab_list_bulk_import_entity_failures"),
		adminReadSpec("error_tracking_list", toolutil.RouteAction(client, errortracking.ListClientKeys), "gitlab_list_error_tracking_client_keys"),
		adminCreateSpec("error_tracking_create", toolutil.RouteAction(client, errortracking.CreateClientKey), "gitlab_create_error_tracking_client_key"),
		adminDeleteSpec("error_tracking_delete", toolutil.DestructiveVoidAction(client, errortracking.DeleteClientKey), "gitlab_delete_error_tracking_client_key"),
		adminReadSpec("error_tracking_get_settings", toolutil.RouteAction(client, errortracking.GetSettings), "gitlab_get_error_tracking_settings"),
		adminUpdateSpec("error_tracking_update_settings", toolutil.RouteAction(client, errortracking.EnableDisable), "gitlab_enable_disable_error_tracking"),
		adminReadSpec("alert_metric_image_list", toolutil.RouteAction(client, alertmanagement.ListMetricImages), "gitlab_list_alert_metric_images"),
		adminCreateSpec("alert_metric_image_upload", toolutil.RouteAction(client, alertmanagement.UploadMetricImage), "gitlab_upload_alert_metric_image"),
		adminUpdateSpec("alert_metric_image_update", toolutil.RouteAction(client, alertmanagement.UpdateMetricImage), "gitlab_update_alert_metric_image"),
		adminDeleteSpec("alert_metric_image_delete", toolutil.DestructiveVoidAction(client, alertmanagement.DeleteMetricImage), "gitlab_delete_alert_metric_image"),
		adminReadSpec("secure_file_list", toolutil.RouteAction(client, securefiles.List), "gitlab_list_secure_files"),
		adminReadSpec("secure_file_get", toolutil.RouteAction(client, securefiles.Show), "gitlab_show_secure_file"),
		adminCreateSpec("secure_file_create", toolutil.RouteAction(client, securefiles.Create), "gitlab_create_secure_file"),
		adminDeleteSpec("secure_file_delete", toolutil.DestructiveVoidAction(client, securefiles.Remove), "gitlab_remove_secure_file"),
		adminReadSpec("terraform_state_list", toolutil.RouteAction(client, terraformstates.List), "gitlab_list_terraform_states"),
		adminReadSpec("terraform_state_get", toolutil.RouteAction(client, terraformstates.Get), "gitlab_get_terraform_state"),
		adminDeleteSpec("terraform_state_delete", toolutil.DestructiveVoidAction(client, terraformstates.Delete), "gitlab_delete_terraform_state"),
		adminUpdateSpec("terraform_state_lock", toolutil.RouteAction(client, terraformstates.Lock), "gitlab_lock_terraform_state"),
		adminDestructiveUpdateIndividualSpec("terraform_state_unlock", toolutil.DestructiveAction(client, terraformstates.Unlock), "gitlab_unlock_terraform_state"),
		adminDeleteSpec("terraform_version_delete", toolutil.DestructiveVoidAction(client, terraformstates.DeleteVersion), "gitlab_delete_terraform_state_version"),
		adminReadSpec("cluster_agent_list", toolutil.RouteAction(client, clusteragents.ListAgents), "gitlab_list_cluster_agents"),
		adminReadSpec("cluster_agent_get", toolutil.RouteAction(client, clusteragents.GetAgent), "gitlab_get_cluster_agent"),
		adminCreateSpec("cluster_agent_register", toolutil.RouteAction(client, clusteragents.RegisterAgent), "gitlab_register_cluster_agent"),
		adminDeleteSpec("cluster_agent_delete", toolutil.DestructiveVoidAction(client, clusteragents.DeleteAgent), "gitlab_delete_cluster_agent"),
		adminReadSpec("cluster_agent_token_list", toolutil.RouteAction(client, clusteragents.ListAgentTokens), "gitlab_list_cluster_agent_tokens"),
		adminReadSpec("cluster_agent_token_get", toolutil.RouteAction(client, clusteragents.GetAgentToken), "gitlab_get_cluster_agent_token"),
		adminCreateSpec("cluster_agent_token_create", toolutil.RouteAction(client, clusteragents.CreateAgentToken), "gitlab_create_cluster_agent_token"),
		adminDeleteSpec("cluster_agent_token_revoke", toolutil.DestructiveVoidAction(client, clusteragents.RevokeAgentToken), "gitlab_revoke_cluster_agent_token"),
		adminDeleteSpec("dependency_proxy_delete", toolutil.DestructiveVoidAction(client, dependencyproxy.Purge), "gitlab_purge_dependency_proxy"),
		adminCreateSpec("import_github", toolutil.RouteAction(client, importservice.ImportFromGitHub), "gitlab_import_from_github"),
		adminUpdateSpec("import_cancel_github", toolutil.RouteAction(client, importservice.CancelGitHubImport), "gitlab_cancel_github_import"),
		adminCreateSpec("import_gists", toolutil.RouteVoidAction(client, importservice.ImportGists), "gitlab_import_github_gists"),
		adminCreateSpec("import_bitbucket", toolutil.RouteAction(client, importservice.ImportFromBitbucketCloud), "gitlab_import_from_bitbucket_cloud"),
		adminCreateSpec("import_bitbucket_server", toolutil.RouteAction(client, importservice.ImportFromBitbucketServer), "gitlab_import_from_bitbucket_server"),
	}
}

func adminReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := adminOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func adminCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, adminOptions(individualTool))
}

func adminUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := adminOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func adminDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := adminOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func adminUpdateCreateIndividualSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	individualIdempotent := false
	options := adminOptions(individualTool)
	options.Idempotent = true
	options.IndividualTool.AnnotationOverrides.Idempotent = &individualIdempotent
	return toolutil.NewActionSpec(name, route, options)
}

func adminDestructiveUpdateIndividualSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	individualDestructive := false
	options := adminOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewActionSpec(name, route, options)
}

func adminSystemHookTestSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualReadOnly := true
	individualIdempotent := true
	options := adminOptions("gitlab_test_system_hook")
	options.IndividualTool.AnnotationOverrides.ReadOnly = &individualReadOnly
	options.IndividualTool.AnnotationOverrides.Idempotent = &individualIdempotent
	return toolutil.NewActionSpec("system_hook_test", toolutil.RouteAction(client, systemhooks.Test), options)
}

func adminOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin"},
		OpenWorld:      true,
		OwnerPackage:   "adminspecs",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
