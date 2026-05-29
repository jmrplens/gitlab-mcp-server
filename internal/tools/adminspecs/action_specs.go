package adminspecs

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/alertmanagement"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/appearance"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/applications"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/appstatistics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/broadcastmessages"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/bulkimports"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/clusteragents"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/customattributes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dbmigrations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dependencyproxy"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/errortracking"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/features"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/importservice"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/license"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/metadata"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/planlimits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securefiles"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/sidekiq"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/systemhooks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/terraformstates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/topics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/usagedata"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionAdminSettingsGet = "admin.settings_get"
	actionAdminMetadataGet = "admin.metadata_get"
)

// ActionSpecs returns canonical specs for gitlab_admin meta-tool actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		adminReadSpec("topic_list", toolutil.RouteAction(client, topics.List), "gitlab_list_topics"),
		adminReadSpec("topic_get", toolutil.RouteAction(client, topics.Get), "gitlab_get_topic"),
		adminCreateSpec("topic_create", toolutil.RouteAction(client, topics.Create), "gitlab_create_topic"),
		adminUpdateSpec("topic_update", toolutil.RouteAction(client, topics.Update), "gitlab_update_topic"),
		adminDeleteSpec("topic_delete", toolutil.DestructiveVoidAction(client, topics.Delete), "gitlab_delete_topic"),
		adminSettingsGetSpec(client),
		adminUpdateSpec("settings_update", toolutil.RouteAction(client, settings.Update), "gitlab_update_settings"),
		adminAppearanceGetSpec(client),
		adminAppearanceUpdateSpec(client),
		adminReadSpec("broadcast_message_list", toolutil.RouteAction(client, broadcastmessages.List), "gitlab_list_broadcast_messages"),
		adminReadSpec("broadcast_message_get", toolutil.RouteAction(client, broadcastmessages.Get), "gitlab_get_broadcast_message"),
		adminCreateSpec("broadcast_message_create", toolutil.RouteAction(client, broadcastmessages.Create), "gitlab_create_broadcast_message"),
		adminUpdateSpec("broadcast_message_update", toolutil.RouteAction(client, broadcastmessages.Update), "gitlab_update_broadcast_message"),
		adminDeleteSpec("broadcast_message_delete", toolutil.DestructiveAction(client, broadcastmessages.DeleteOutput), "gitlab_delete_broadcast_message"),
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
		adminSystemHookEditSpec(client),
		adminSystemHookTestSpec(client),
		adminSystemHookSetURLVariableSpec(client),
		adminSystemHookDeleteURLVariableSpec(client),
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
		adminApplicationStatisticsGetSpec(client),
		adminMetadataGetSpec(client),
		adminReadSpec("custom_attr_list", toolutil.RouteAction(client, customattributes.List), "gitlab_list_custom_attributes"),
		adminReadSpec("custom_attr_get", toolutil.RouteAction(client, customattributes.Get), "gitlab_get_custom_attribute"),
		adminUpdateCreateIndividualSpec("custom_attr_set", toolutil.RouteAction(client, customattributes.Set), "gitlab_set_custom_attribute"),
		adminDeleteSpec("custom_attr_delete", toolutil.DestructiveAction(client, customattributes.DeleteOutput), "gitlab_delete_custom_attribute"),
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
		adminTerraformStateUnlockSpec(client),
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
	return toolutil.NewReadActionSpec(name, route, adminOptions(individualTool))
}

func adminCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, adminOptions(individualTool))
}

func adminUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, adminOptions(individualTool))
}

func adminDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, adminOptions(individualTool))
}

func adminUpdateCreateIndividualSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	individualIdempotent := false
	options := adminOptions(individualTool)
	options.IndividualTool.AnnotationOverrides.Idempotent = &individualIdempotent
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func adminDestructiveUpdateIndividualSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	individualDestructive := false
	options := adminOptions(individualTool)
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func adminSystemHookTestSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualReadOnly := true
	individualIdempotent := true
	options := adminOptions("gitlab_test_system_hook")
	options.IndividualTool.AnnotationOverrides.ReadOnly = &individualReadOnly
	options.IndividualTool.AnnotationOverrides.Idempotent = &individualIdempotent
	return toolutil.NewCreateActionSpec("system_hook_test", toolutil.RouteAction(client, systemhooks.Test), options)
}

func adminSystemHookEditSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := adminOptions("gitlab_edit_system_hook")
	options.IndividualTool.Description = "Edit an instance system hook, including event triggers, SSL verification, and URL settings. Returns: the updated system hook object. See also: gitlab_get_system_hook, gitlab_list_system_hooks, gitlab_test_system_hook."
	return toolutil.NewUpdateActionSpec("system_hook_edit", toolutil.RouteAction(client, systemhooks.Edit), options)
}

func adminSystemHookSetURLVariableSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := adminOptions("gitlab_set_system_hook_url_variable")
	options.IndividualTool.Description = "Create or update one URL variable for an instance system hook. Returns: a success status and message naming the variable key. See also: gitlab_edit_system_hook, gitlab_get_system_hook."
	return toolutil.NewUpdateActionSpec("system_hook_set_url_variable", toolutil.RouteVoidAction(client, systemhooks.SetURLVariable), options)
}

func adminSystemHookDeleteURLVariableSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := adminOptions("gitlab_delete_system_hook_url_variable")
	options.IndividualTool.Description = "Delete one URL variable from an instance system hook. Returns: a success status and message naming the variable key. See also: gitlab_set_system_hook_url_variable, gitlab_get_system_hook."
	return toolutil.NewDeleteActionSpec("system_hook_delete_url_variable", toolutil.DestructiveVoidAction(client, systemhooks.DeleteURLVariable), options)
}

func adminSettingsGetSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := adminOptions("gitlab_get_settings")
	options.Usage = "Read current GitLab application settings. Use this for instance or application settings, not for server metadata or version information."
	options.Aliases = []string{"application settings", "instance settings", "current settings", "admin settings", "gitlab settings"}
	options.Tags = append(options.Tags, "settings", "application_settings")
	return toolutil.NewReadActionSpec("settings_get", toolutil.RouteAction(client, settings.Get), options)
}

func adminMetadataGetSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := adminOptions("gitlab_get_metadata")
	options.Usage = "Read GitLab instance metadata such as version and revision. Do not use this for application settings."
	options.Aliases = []string{"instance metadata", "gitlab version", "server metadata", "gitlab revision"}
	options.Tags = append(options.Tags, "metadata", "version")
	options.RelatedActions = []string{actionAdminSettingsGet, "admin.app_statistics_get", "server.health_check"}
	options.IndividualTool.Description = "Get GitLab instance metadata such as version, revision, KAS endpoints, and enterprise edition flag. Returns: the current instance metadata object. See also: gitlab_server_status, gitlab_get_settings, gitlab_get_application_statistics."
	return toolutil.NewReadActionSpec("metadata_get", toolutil.RouteAction(client, metadata.Get), options)
}

func adminAppearanceGetSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := adminOptions("gitlab_get_appearance")
	options.Usage = "Read the current GitLab application appearance and branding settings. Use this for logos, banners, PWA labels, and instance message colors rather than general application settings or version metadata."
	options.Aliases = []string{"appearance", "application appearance", "instance appearance", "branding settings", "gitlab appearance"}
	options.Tags = append(options.Tags, "appearance", "branding")
	options.RelatedActions = []string{actionAdminSettingsGet, actionAdminMetadataGet, "admin.appearance_update"}
	options.IndividualTool.Description = "Get the current GitLab application appearance and branding settings. Returns: the instance appearance object including title, messages, logos, and PWA labels. See also: gitlab_update_appearance, gitlab_get_settings, gitlab_get_metadata."
	return toolutil.NewReadActionSpec("appearance_get", toolutil.RouteAction(client, appearance.Get), options)
}

func adminAppearanceUpdateSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := adminOptions("gitlab_update_appearance")
	options.Usage = "Update GitLab application appearance and branding settings such as title, messages, colors, PWA labels, and profile guidance text. Requires administrator access and changes the instance UI immediately."
	options.Aliases = []string{"update appearance", "change appearance", "update branding", "change branding", "appearance settings update"}
	options.Tags = append(options.Tags, "appearance", "branding")
	options.RelatedActions = []string{"admin.appearance_get", actionAdminSettingsGet, actionAdminMetadataGet}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"message_background_color": {
			SemanticRole:     "hex_color",
			ValueSource:      "Hex color string such as #e75e40 for the appearance banner background.",
			CommonConfusions: []string{"Provide a CSS-style hex color such as #ffffff; do not send color names or RGB tuples."},
		},
		"message_font_color": {
			SemanticRole:     "hex_color",
			ValueSource:      "Hex color string such as #ffffff for the appearance banner text.",
			CommonConfusions: []string{"Provide a CSS-style hex color such as #000000; do not send color names or RGB tuples."},
		},
	}
	options.IndividualTool.Description = "Update GitLab application appearance and branding settings. Returns: the updated appearance object after GitLab applies the change. See also: gitlab_get_appearance, gitlab_get_settings, gitlab_get_metadata."
	return toolutil.NewUpdateActionSpec("appearance_update", toolutil.RouteAction(client, appearance.Update), options)
}

func adminApplicationStatisticsGetSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := adminOptions("gitlab_get_application_statistics")
	options.Usage = "Read GitLab instance-wide application statistics such as totals for users, groups, projects, issues, and merge requests. Requires administrator access."
	options.Aliases = []string{"application statistics", "instance statistics", "gitlab statistics", "admin statistics"}
	options.Tags = append(options.Tags, "statistics", "instance")
	options.RelatedActions = []string{actionAdminMetadataGet, "server.health_check"}
	options.IndividualTool.Description = "Get GitLab application statistics for the current instance. Returns: aggregate counts for users, groups, projects, issues, merge requests, and related records. See also: gitlab_get_metadata, gitlab_server_status."
	return toolutil.NewReadActionSpec("app_statistics_get", toolutil.RouteAction(client, appstatistics.Get), options)
}

func adminTerraformStateUnlockSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := adminOptions("gitlab_unlock_terraform_state")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	options.Tags = append(options.Tags, "terraform", "terraform_state", "state", "lock", "unlock")
	options.Usage = "Unlock a GitLab Terraform state by project_id and state name. Use params.name for the Terraform state name; do not send the state name as id."
	options.Aliases = []string{"terraform_state.unlock", "unlock terraform state", "unlock terraform state lock", "terraform state unlock"}
	options.RelatedActions = []string{"admin.terraform_state_get", "admin.terraform_state_lock", "admin.terraform_state_list"}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"name": {
			SemanticRole: "terraform_state_name",
			ValueSource:  "Terraform state name from the prompt or admin.terraform_state_list output.",
			CommonConfusions: []string{
				"Do not send the state name as id; use params.name.",
			},
		},
	}
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("name", map[string]any{"description": "Terraform state name. Use params.name for values such as production or eval-unlock-123; do not use id."}),
	}
	return toolutil.NewDeleteActionSpec("terraform_state_unlock", toolutil.DestructiveAction(client, terraformstates.Unlock), options)
}

func adminOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute adminspecs domain action.", Tags: []string{"admin"},
		OpenWorld:      true,
		OwnerPackage:   "adminspecs",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
