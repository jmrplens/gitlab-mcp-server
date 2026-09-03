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
	actionAdminSettingsGet = actionSettingsGet
	actionAdminMetadataGet = actionMetadataGet
)

const (
	actionSystemHookGet      = "admin.system_hook_get"
	actionSettingsGet        = "admin.settings_get"
	actionBulkImportGet      = "admin.bulk_import_get"
	actionBulkImportEntList  = "admin.bulk_import_entity_list"
	actionUsageDataPing      = "admin.usage_data_service_ping"
	actionTerraformStateList = "admin.terraform_state_list"
	actionTerraformStateGet  = "admin.terraform_state_get"
	actionSystemHookList     = "admin.system_hook_list"
	actionSystemHookEdit     = "admin.system_hook_edit"
	actionClusterAgentTokLst = "admin.cluster_agent_token_list"
	actionTopicList          = "admin.topic_list"
	actionSystemHookTest     = "admin.system_hook_test"
	actionMetadataGet        = "admin.metadata_get"
	actionImportGitHub       = "admin.import_github"
	actionErrTrackingList    = "admin.error_tracking_list"
	actionErrTrackingGet     = "admin.error_tracking_get_settings"
	actionClusterAgentTokCrt = "admin.cluster_agent_token_create"
	actionBroadcastMsgList   = "admin.broadcast_message_list"
	tagAppearance            = "appearance"
	actionUsageDataQueries   = "admin.usage_data_queries"
	actionUsageDataNonSQL    = "admin.usage_data_non_sql_metrics"
	actionUsageDataMetaDefs  = "admin.usage_data_metric_definitions"
	actionTopicUpdate        = "admin.topic_update"
	actionTopicGet           = "admin.topic_get"
	actionTopicDelete        = "admin.topic_delete"
	actionTerraformStateLock = "admin.terraform_state_lock"
	actionSidekiqQueueMet    = "admin.sidekiq_queue_metrics"
	actionSidekiqProcMet     = "admin.sidekiq_process_metrics"
	actionSidekiqJobStats    = "admin.sidekiq_job_stats"
	actionSidekiqCmpMet      = "admin.sidekiq_compound_metrics"
	actionSecureFileList     = "admin.secure_file_list"
	actionSecureFileGet      = "admin.secure_file_get"
	actionSecureFileDel      = "admin.secure_file_delete"
	actionSecureFileCreate   = "admin.secure_file_create"
	actionFeatureSet         = "admin.feature_set"
	actionFeatureList        = "admin.feature_list"
	actionFeatureListDefs    = "admin.feature_list_definitions"
	actionFeatureDelete      = "admin.feature_delete"
	actionCustomAttrSet      = "admin.custom_attr_set"
	actionCustomAttrList     = "admin.custom_attr_list"
	actionCustomAttrGet      = "admin.custom_attr_get"
	actionCustomAttrDel      = "admin.custom_attr_delete"
	actionApplicationList    = "admin.application_list"
	actionApplicationCreate  = "admin.application_create"
	actionApplicationDelete  = "admin.application_delete"
	actionClusterAgentTokRev = "admin.cluster_agent_token_revoke"
	actionClusterAgentTokGet = "admin.cluster_agent_token_get"
	actionClusterAgentList   = "admin.cluster_agent_list"
	actionBulkImportList     = "admin.bulk_import_list"
	actionBroadcastMsgUpd    = "admin.broadcast_message_update"
	actionBroadcastMsgGet    = "admin.broadcast_message_get"
	actionBroadcastMsgDel    = "admin.broadcast_message_delete"
	actionAlertImgUpload     = "admin.alert_metric_image_upload"
	actionAlertImgUpdate     = "admin.alert_metric_image_update"
	actionAlertImgList       = "admin.alert_metric_image_list"
	actionAlertImgDelete     = "admin.alert_metric_image_delete"
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
		// Classified as a create, not a read, and deliberately without an
		// annotation override. Firing a test event makes GitLab POST a sample
		// payload to the hook's URL, which is an observable side effect on a
		// third-party endpoint even though nothing on the instance changes.
		// The override that used to claim read-only here was inert anyway:
		// IndividualToolAnnotationOverrides.NarrowingOnly drops any claim that
		// widens the base classification, so it never reached a served tool.
		adminCreateSpec("system_hook_test", toolutil.RouteAction(client, systemhooks.Test), "gitlab_test_system_hook"),
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
		adminCreateSpec("application_renew_secret", toolutil.RouteAction(client, applications.RenewSecret), "gitlab_renew_application_secret"),
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
	options.RelatedActions = []string{"admin.settings_update", "admin.appearance_get", actionAdminMetadataGet}
	options.IndividualTool.Description = "Get the current GitLab application (instance) settings. Returns: the full application settings object including sign-up, visibility, CI/CD, and rate-limit configuration. See also: gitlab_update_settings, gitlab_get_appearance, gitlab_get_metadata."
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
	options.Aliases = []string{tagAppearance, "application appearance", "instance appearance", "branding settings", "gitlab appearance"}
	options.Tags = append(options.Tags, tagAppearance, "branding")
	options.RelatedActions = []string{actionAdminSettingsGet, actionAdminMetadataGet, "admin.appearance_update"}
	options.IndividualTool.Description = "Get the current GitLab application appearance and branding settings. Returns: the instance appearance object including title, messages, logos, and PWA labels. See also: gitlab_update_appearance, gitlab_get_settings, gitlab_get_metadata."
	return toolutil.NewReadActionSpec("appearance_get", toolutil.RouteAction(client, appearance.Get), options)
}

func adminAppearanceUpdateSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := adminOptions("gitlab_update_appearance")
	options.Usage = "Update GitLab application appearance and branding settings such as title, messages, colors, PWA labels, and profile guidance text. Requires administrator access and changes the instance UI immediately."
	options.Aliases = []string{"update appearance", "change appearance", "update branding", "change branding", "appearance settings update"}
	options.Tags = append(options.Tags, tagAppearance, "branding")
	options.RelatedActions = []string{"admin.appearance_get", actionAdminSettingsGet, actionAdminMetadataGet}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"message_background_color": {
			SemanticRole:     "hex_color",
			ValueSource:      "Hex color string such as #e75e40 for the appearance banner background.",
			CommonConfusions: []string{"Provide a CSS-style hex color such as #ffffff. Do not send color names or RGB tuples."},
		},
		"message_font_color": {
			SemanticRole:     "hex_color",
			ValueSource:      "Hex color string such as #ffffff for the appearance banner text.",
			CommonConfusions: []string{"Provide a CSS-style hex color such as #000000. Do not send color names or RGB tuples."},
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
	options.Usage = "Unlock a GitLab Terraform state by project_id and state name. Use params.name for the Terraform state name. Do not send the state name as id."
	options.Aliases = []string{"terraform_state.unlock", "unlock terraform state", "unlock terraform state lock", "terraform state unlock"}
	options.RelatedActions = []string{actionTerraformStateGet, actionTerraformStateLock, actionTerraformStateList}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"name": {
			SemanticRole: "terraform_state_name",
			ValueSource:  "Terraform state name from the prompt or admin.terraform_state_list output.",
			CommonConfusions: []string{
				"Do not send the state name as id. Use params.name.",
			},
		},
	}
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("name", map[string]any{"description": "Terraform state name. Use params.name for values such as production or eval-unlock-123. Do not use id."}),
	}
	options.IndividualTool.Description = "Unlock a project Terraform state by project_id and state name. Returns: the state with its lock released. See also: gitlab_lock_terraform_state, gitlab_get_terraform_state, gitlab_list_terraform_states."
	return toolutil.NewDeleteActionSpec("terraform_state_unlock", toolutil.DestructiveAction(client, terraformstates.Unlock), options)
}

func adminOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute adminspecs domain action.", Tags: []string{"admin"},
		OpenWorld:      true,
		OwnerPackage:   "adminspecs",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateAdminMeta(&options, individualTool)
	return options
}

// adminActionMetaEntry is the discovery metadata for one admin action.
type adminActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// decorateAdminMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for every admin action that would otherwise inherit the generic placeholder
// metadata from adminOptions. Actions whose dedicated builders already set rich
// metadata (settings_get, metadata_get, appearance_*, app_statistics_get,
// system_hook_edit/set/delete URL variable, terraform_state_unlock) are absent
// from the map and left untouched. When an entry omits usage, any tailored Usage
// already set by a dedicated builder is preserved while aliases, related actions,
// and the description are still added.
func decorateAdminMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := adminActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// adminActionMeta maps each individual admin tool to its discovery metadata.
// Entries cover every admin action that routes through the shared adminOptions
// placeholder; actions handled by dedicated builders with bespoke metadata are
// intentionally omitted so their tailored text is preserved.
//
//nolint:funlen // A flat, reviewable lookup table; one entry per admin action.
var adminActionMeta = map[string]adminActionMetaEntry{
	"gitlab_list_topics": {
		usage:       "List instance project topics, optionally filtered by a search term. Topics group related projects across the instance.",
		aliases:     []string{"list topics", "show project topics", "browse topics"},
		related:     []string{actionTopicGet, "admin.topic_create", actionTopicUpdate},
		description: "List instance project topics. Returns: an array of topics with id, name, title, description, and project counts. See also: gitlab_get_topic, gitlab_create_topic.",
	},
	"gitlab_get_topic": {
		usage:       "Get one instance project topic by its numeric topic id.",
		aliases:     []string{"get topic", "show topic", "topic details"},
		related:     []string{actionTopicList, actionTopicUpdate, actionTopicDelete},
		description: "Get a single project topic by id. Returns: the topic with id, name, title, description, avatar, and project count. See also: gitlab_list_topics, gitlab_update_topic.",
	},
	"gitlab_create_topic": {
		usage:       "Create a new instance project topic (admin only). Provide name and optionally title, description, and avatar.",
		aliases:     []string{"create topic", "add topic", "new project topic"},
		related:     []string{actionTopicList, actionTopicUpdate, actionTopicDelete},
		description: "Create an instance project topic. Returns: the created topic with id, name, title, and description. See also: gitlab_update_topic, gitlab_list_topics.",
	},
	"gitlab_update_topic": {
		usage:       "Update an existing project topic by id (admin only). Send only the fields to change: name, title, description, or avatar.",
		aliases:     []string{"update topic", "edit topic", "rename topic"},
		related:     []string{actionTopicGet, actionTopicList, actionTopicDelete},
		description: "Update a project topic. Returns: the updated topic. See also: gitlab_get_topic, gitlab_delete_topic.",
	},
	"gitlab_delete_topic": {
		usage:       "Delete a project topic by id (admin only). Projects keep their other topics. This only removes the topic definition.",
		aliases:     []string{"delete topic", "remove topic", "drop project topic"},
		related:     []string{actionTopicGet, actionTopicList, "admin.topic_create"},
		description: "Delete a project topic. Returns: a success status. See also: gitlab_get_topic, gitlab_create_topic.",
	},
	"gitlab_update_settings": {
		usage:       "Update GitLab application (instance) settings such as sign-up restrictions, default visibility, CI/CD defaults, rate limits, and feature toggles. Send only the keys to change. Requires administrator access.",
		aliases:     []string{"update application settings", "change instance settings", "configure gitlab settings", "edit admin settings"},
		related:     []string{actionSettingsGet, "admin.appearance_update", actionMetadataGet},
		description: "Update GitLab application settings. Returns: the full updated application settings object. See also: gitlab_get_settings, gitlab_update_appearance.",
	},
	"gitlab_list_broadcast_messages": {
		usage:       "List all broadcast messages shown to users across the instance, including active and scheduled banners and notifications.",
		aliases:     []string{"list broadcast messages", "show announcements", "instance banners"},
		related:     []string{actionBroadcastMsgGet, "admin.broadcast_message_create", actionBroadcastMsgUpd},
		description: "List instance broadcast messages. Returns: an array of broadcast messages with id, message, theme, target, starts_at, and ends_at. See also: gitlab_get_broadcast_message, gitlab_create_broadcast_message.",
	},
	"gitlab_get_broadcast_message": {
		usage:       "Get one broadcast message by id.",
		aliases:     []string{"get broadcast message", "show announcement", "broadcast details"},
		related:     []string{actionBroadcastMsgList, actionBroadcastMsgUpd, actionBroadcastMsgDel},
		description: "Get a broadcast message by id. Returns: the message with text, theme, target, and schedule. See also: gitlab_list_broadcast_messages, gitlab_update_broadcast_message.",
	},
	"gitlab_create_broadcast_message": {
		usage:       "Create a broadcast message banner or notification (admin only). Provide message text and optionally theme, target_path, broadcast_type, dismissable, starts_at, and ends_at.",
		aliases:     []string{"create broadcast message", "add announcement", "post instance banner"},
		related:     []string{actionBroadcastMsgList, actionBroadcastMsgUpd, actionBroadcastMsgDel},
		description: "Create a broadcast message. Returns: the created message with id, theme, target, and schedule. See also: gitlab_update_broadcast_message, gitlab_list_broadcast_messages.",
	},
	"gitlab_update_broadcast_message": {
		usage:       "Update a broadcast message by id (admin only). Send only the fields to change such as message, theme, schedule, or target.",
		aliases:     []string{"update broadcast message", "edit announcement", "change instance banner"},
		related:     []string{actionBroadcastMsgGet, actionBroadcastMsgList, actionBroadcastMsgDel},
		description: "Update a broadcast message. Returns: the updated message. See also: gitlab_get_broadcast_message, gitlab_delete_broadcast_message.",
	},
	"gitlab_delete_broadcast_message": {
		usage:       "Delete a broadcast message by id (admin only), removing the banner or notification from the instance.",
		aliases:     []string{"delete broadcast message", "remove announcement", "dismiss instance banner"},
		related:     []string{actionBroadcastMsgGet, actionBroadcastMsgList, "admin.broadcast_message_create"},
		description: "Delete a broadcast message. Returns: a success status. See also: gitlab_get_broadcast_message, gitlab_create_broadcast_message.",
	},
	"gitlab_list_features": {
		usage:       "List all defined feature flags and their current gate state on the instance. Use list_feature_definitions for the flag catalog with metadata.",
		aliases:     []string{"list feature flags", "show feature toggles", "instance features"},
		related:     []string{actionFeatureListDefs, actionFeatureSet, actionFeatureDelete},
		description: "List instance feature flags. Returns: an array of features with name, state, and gates (boolean, actors, groups, percentages). See also: gitlab_list_feature_definitions, gitlab_set_feature_flag.",
	},
	"gitlab_list_feature_definitions": {
		usage:       "List feature flag definitions (the catalog) with metadata such as type, group, default state, and introduction milestone.",
		aliases:     []string{"list feature definitions", "feature flag catalog", "show feature metadata"},
		related:     []string{actionFeatureList, actionFeatureSet, actionFeatureDelete},
		description: "List feature flag definitions. Returns: an array of definitions with name, type, default_enabled, group, and milestone. See also: gitlab_list_features, gitlab_set_feature_flag.",
	},
	"gitlab_set_feature_flag": {
		usage:       "Set or update a feature flag's gate (admin only). Provide name and a value such as true/false, a percentage, or scope it to feature_group, user, group, project, or namespace.",
		aliases:     []string{"set feature flag", "enable feature flag", "toggle feature", "configure feature gate"},
		related:     []string{actionFeatureList, actionFeatureListDefs, actionFeatureDelete},
		description: "Set a feature flag gate. Returns: the updated feature with its name, state, and gate values. See also: gitlab_list_features, gitlab_delete_feature_flag.",
	},
	"gitlab_delete_feature_flag": {
		usage:       "Delete a feature flag by name (admin only), removing all of its gates and resetting it to its default state.",
		aliases:     []string{"delete feature flag", "remove feature toggle", "reset feature gate"},
		related:     []string{actionFeatureList, actionFeatureSet, actionFeatureListDefs},
		description: "Delete a feature flag. Returns: a success status. See also: gitlab_set_feature_flag, gitlab_list_features.",
	},
	"gitlab_get_license": {
		usage:       "Get the currently installed GitLab Enterprise license, including plan, expiration, user limits, and add-on entitlements. Requires administrator access.",
		aliases:     []string{"get license", "show current license", "instance license details", "license status"},
		related:     []string{"admin.license_add", "admin.license_delete", actionMetadataGet},
		description: "Get the active instance license. Returns: the license with plan, expires_at, user_limit, active_users, and add-ons. See also: gitlab_add_license, gitlab_delete_license.",
	},
	"gitlab_add_license": {
		usage:       "Install a new GitLab Enterprise license (admin only). Provide the license key string. Replaces the active license entitlements.",
		aliases:     []string{"add license", "install license", "upload license key", "activate license"},
		related:     []string{"admin.license_get", "admin.license_delete"},
		description: "Add a GitLab Enterprise license. Returns: the installed license with plan, expiration, and entitlements. See also: gitlab_get_license, gitlab_delete_license.",
	},
	"gitlab_delete_license": {
		usage:       "Delete an installed license by id (admin only). Removing the active license downgrades the instance to Community/Free features.",
		aliases:     []string{"delete license", "remove license", "uninstall license"},
		related:     []string{"admin.license_get", "admin.license_add"},
		description: "Delete an instance license. Returns: a success status. See also: gitlab_get_license, gitlab_add_license.",
	},
	"gitlab_list_system_hooks": {
		usage:       "List instance-wide system hooks that fire on global events such as project, group, user, and key changes. Distinct from per-project webhooks.",
		aliases:     []string{"list system hooks", "show instance webhooks", "system hook list"},
		related:     []string{actionSystemHookGet, "admin.system_hook_add", actionSystemHookTest},
		description: "List instance system hooks. Returns: an array of system hooks with id, url, and event triggers. See also: gitlab_get_system_hook, gitlab_add_system_hook.",
	},
	"gitlab_get_system_hook": {
		usage:       "Get one instance system hook by its numeric hook id.",
		aliases:     []string{"get system hook", "show instance webhook", "system hook details"},
		related:     []string{actionSystemHookList, actionSystemHookEdit, actionSystemHookTest},
		description: "Get a system hook by id. Returns: the system hook with url, event triggers, and SSL verification setting. See also: gitlab_list_system_hooks, gitlab_edit_system_hook.",
	},
	"gitlab_add_system_hook": {
		usage:       "Add an instance system hook (admin only). Provide url and optionally a token, SSL verification flag, and which event triggers to enable.",
		aliases:     []string{"add system hook", "create instance webhook", "register system hook"},
		related:     []string{actionSystemHookList, actionSystemHookEdit, actionSystemHookTest},
		description: "Add an instance system hook. Returns: the created system hook with id, url, and triggers. See also: gitlab_edit_system_hook, gitlab_test_system_hook.",
	},
	"gitlab_test_system_hook": {
		usage:       "Fire a test event against an instance system hook by id to verify it is reachable and configured correctly.",
		aliases:     []string{"test system hook", "ping system hook", "verify instance webhook"},
		related:     []string{actionSystemHookGet, actionSystemHookEdit, actionSystemHookList},
		description: "Send a test event to a system hook. Returns: the delivery status of the test event. See also: gitlab_get_system_hook, gitlab_edit_system_hook.",
	},
	"gitlab_delete_system_hook": {
		usage:       "Delete an instance system hook by id (admin only), stopping it from receiving global event notifications.",
		aliases:     []string{"delete system hook", "remove instance webhook", "unregister system hook"},
		related:     []string{actionSystemHookGet, actionSystemHookList, "admin.system_hook_add"},
		description: "Delete an instance system hook. Returns: a success status. See also: gitlab_get_system_hook, gitlab_add_system_hook.",
	},
	// system_hook_edit/set_url_variable/delete_url_variable have dedicated builders
	// that set bespoke "Returns: … See also: …" descriptions; only Usage, Aliases,
	// and RelatedActions are filled here (description omitted to preserve the
	// builder text).
	"gitlab_edit_system_hook": {
		usage:   "Edit an instance system hook by id (admin only). Send only the fields to change: url, token, event triggers, or SSL verification.",
		aliases: []string{"edit system hook", "update instance webhook", "change system hook"},
		related: []string{actionSystemHookGet, actionSystemHookList, actionSystemHookTest},
	},
	"gitlab_set_system_hook_url_variable": {
		usage:   "Create or update one URL variable for an instance system hook (admin only). Provide the hook id, variable key, and value.",
		aliases: []string{"set system hook url variable", "add system hook url variable", "configure system hook variable"},
		related: []string{"admin.system_hook_delete_url_variable", actionSystemHookEdit, actionSystemHookGet},
	},
	"gitlab_delete_system_hook_url_variable": {
		usage:   "Delete one URL variable from an instance system hook (admin only). Provide the hook id and variable key.",
		aliases: []string{"delete system hook url variable", "remove system hook url variable", "clear system hook variable"},
		related: []string{"admin.system_hook_set_url_variable", actionSystemHookEdit, actionSystemHookGet},
	},
	"gitlab_get_sidekiq_queue_metrics": {
		usage:       "Get Sidekiq background-job queue metrics for the instance, including per-queue backlog and latency.",
		aliases:     []string{"sidekiq queue metrics", "background job queues", "show sidekiq queues"},
		related:     []string{actionSidekiqProcMet, actionSidekiqJobStats, actionSidekiqCmpMet},
		description: "Get Sidekiq queue metrics. Returns: per-queue backlog and latency figures. See also: gitlab_get_sidekiq_process_metrics, gitlab_get_sidekiq_compound_metrics.",
	},
	"gitlab_get_sidekiq_process_metrics": {
		usage:       "Get Sidekiq process metrics for the instance, listing running Sidekiq processes and their state.",
		aliases:     []string{"sidekiq process metrics", "background job processes", "show sidekiq workers"},
		related:     []string{actionSidekiqQueueMet, actionSidekiqJobStats, actionSidekiqCmpMet},
		description: "Get Sidekiq process metrics. Returns: the list of Sidekiq processes with concurrency and queue assignments. See also: gitlab_get_sidekiq_queue_metrics, gitlab_get_sidekiq_compound_metrics.",
	},
	"gitlab_get_sidekiq_job_stats": {
		usage:       "Get aggregate Sidekiq job statistics for the instance such as processed, failed, and enqueued counts.",
		aliases:     []string{"sidekiq job stats", "background job statistics", "show sidekiq jobs"},
		related:     []string{actionSidekiqQueueMet, actionSidekiqProcMet, actionSidekiqCmpMet},
		description: "Get Sidekiq job statistics. Returns: aggregate counts of processed, failed, and enqueued jobs. See also: gitlab_get_sidekiq_queue_metrics, gitlab_get_sidekiq_compound_metrics.",
	},
	"gitlab_get_sidekiq_compound_metrics": {
		usage:       "Get all Sidekiq metrics in one call: queue, process, and job statistics combined.",
		aliases:     []string{"sidekiq compound metrics", "all sidekiq metrics", "combined background job metrics"},
		related:     []string{actionSidekiqQueueMet, actionSidekiqProcMet, actionSidekiqJobStats},
		description: "Get combined Sidekiq metrics. Returns: queue, process, and job statistics in a single object. See also: gitlab_get_sidekiq_queue_metrics, gitlab_get_sidekiq_job_stats.",
	},
	"gitlab_get_plan_limits": {
		usage:       "Get the configured plan limits for a plan (admin only). Provide plan_name (e.g. default, free, premium) to read its resource caps.",
		aliases:     []string{"get plan limits", "show plan caps", "instance plan limits"},
		related:     []string{"admin.plan_limits_change", actionSettingsGet},
		description: "Get plan limits for a plan. Returns: the limit values for CI, registry, import, and other resource caps. See also: gitlab_change_plan_limits, gitlab_get_settings.",
	},
	"gitlab_change_plan_limits": {
		usage:       "Change plan limits for a plan (admin only). Provide plan_name and only the limit keys to change, such as ci_pipeline_size or import file sizes.",
		aliases:     []string{"change plan limits", "update plan caps", "set plan limits"},
		related:     []string{"admin.plan_limits_get", actionSettingsGet},
		description: "Change plan limits. Returns: the updated plan limit values. See also: gitlab_get_plan_limits, gitlab_get_settings.",
	},
	"gitlab_get_service_ping": {
		usage:       "Get the latest Service Ping (usage data) payload that GitLab reports for the instance.",
		aliases:     []string{"get service ping", "usage ping", "instance usage data"},
		related:     []string{actionUsageDataNonSQL, actionUsageDataQueries, actionUsageDataMetaDefs},
		description: "Get the Service Ping payload. Returns: the aggregated instance usage data report. See also: gitlab_get_non_sql_metrics, gitlab_get_usage_queries.",
	},
	"gitlab_get_non_sql_metrics": {
		usage:       "Get the non-SQL Service Ping metrics for the instance (metrics not derived from database queries).",
		aliases:     []string{"non-sql metrics", "service ping non-sql", "usage data non-sql metrics"},
		related:     []string{actionUsageDataPing, actionUsageDataQueries, actionUsageDataMetaDefs},
		description: "Get non-SQL usage metrics. Returns: the non-SQL portion of the Service Ping data. See also: gitlab_get_service_ping, gitlab_get_usage_queries.",
	},
	"gitlab_get_usage_queries": {
		usage:       "Get the SQL queries that GitLab uses to build Service Ping metrics, useful for auditing what usage data is collected.",
		aliases:     []string{"usage queries", "service ping queries", "usage data sql queries"},
		related:     []string{actionUsageDataPing, actionUsageDataNonSQL, actionUsageDataMetaDefs},
		description: "Get usage data queries. Returns: the SQL query definitions backing Service Ping metrics. See also: gitlab_get_service_ping, gitlab_get_metric_definitions.",
	},
	"gitlab_get_metric_definitions": {
		usage:       "Get the Service Ping metric definitions (the dictionary describing each usage metric, its category, and data type).",
		aliases:     []string{"metric definitions", "usage metric dictionary", "service ping metric definitions"},
		related:     []string{actionUsageDataPing, actionUsageDataQueries, actionUsageDataNonSQL},
		description: "Get usage metric definitions. Returns: the metric dictionary with key, description, and category for each Service Ping metric. See also: gitlab_get_service_ping, gitlab_get_usage_queries.",
	},
	"gitlab_track_event": {
		usage:       "Track a single internal usage event (admin only). Provide the event name and optional context for usage analytics.",
		aliases:     []string{"track event", "record usage event", "send analytics event"},
		related:     []string{"admin.usage_data_track_events", actionUsageDataPing},
		description: "Track one usage event. Returns: a success status. See also: gitlab_track_events, gitlab_get_service_ping.",
	},
	"gitlab_track_events": {
		usage:       "Track multiple internal usage events in one call (admin only). Provide an array of events with names and context.",
		aliases:     []string{"track events", "record usage events batch", "send analytics events"},
		related:     []string{"admin.usage_data_track_event", actionUsageDataPing},
		description: "Track multiple usage events. Returns: a success status. See also: gitlab_track_event, gitlab_get_service_ping.",
	},
	"gitlab_mark_migration": {
		usage:       "Mark a background database migration as successfully completed (admin only). Provide the database name and migration version. Use with care. Intended for recovering stuck migrations.",
		aliases:     []string{"mark migration", "mark database migration done", "force migration complete"},
		related:     []string{actionSettingsGet, actionMetadataGet},
		description: "Mark a database migration as complete. Returns: a success status. See also: gitlab_get_settings, gitlab_get_metadata.",
	},
	"gitlab_list_applications": {
		usage:       "List instance-level OAuth applications registered for the GitLab instance (admin only).",
		aliases:     []string{"list applications", "list oauth applications", "instance oauth apps"},
		related:     []string{actionApplicationCreate, actionApplicationDelete},
		description: "List instance OAuth applications. Returns: an array of applications with id, application_id, name, redirect URIs, and scopes. See also: gitlab_create_application, gitlab_delete_application.",
	},
	"gitlab_create_application": {
		usage:       "Create an instance-level OAuth application (admin only). Provide name, redirect_uri, and scopes. Optionally mark it confidential or trusted.",
		aliases:     []string{"create application", "register oauth application", "add instance oauth app"},
		related:     []string{actionApplicationList, actionApplicationDelete},
		description: "Create an instance OAuth application. Returns: the application with application_id and secret (shown once). See also: gitlab_list_applications, gitlab_delete_application.",
	},
	"gitlab_renew_application_secret": {
		usage:       "Renew (rotate) the secret of an instance-level OAuth application by id (admin only). The previous secret is invalidated immediately, so update every client that uses it with the new value returned.",
		aliases:     []string{"renew application secret", "rotate oauth secret", "regenerate application secret", "reset oauth client secret"},
		related:     []string{actionApplicationList, actionApplicationCreate},
		description: "Renew an instance OAuth application secret. Returns: the application with its freshly generated secret (shown once). See also: gitlab_list_applications, gitlab_create_application.",
	},
	"gitlab_delete_application": {
		usage:       "Delete an instance-level OAuth application by id (admin only), revoking its credentials.",
		aliases:     []string{"delete application", "remove oauth application", "revoke instance oauth app"},
		related:     []string{actionApplicationList, actionApplicationCreate},
		description: "Delete an instance OAuth application. Returns: a success status. See also: gitlab_list_applications, gitlab_create_application.",
	},
	"gitlab_list_custom_attributes": {
		usage:       "List custom attributes set on a user, group, or project (admin only). Custom attributes are admin-only key/value metadata.",
		aliases:     []string{"list custom attributes", "show custom attributes", "custom metadata list"},
		related:     []string{actionCustomAttrGet, actionCustomAttrSet, actionCustomAttrDel},
		description: "List custom attributes for a resource. Returns: an array of key/value custom attributes. See also: gitlab_get_custom_attribute, gitlab_set_custom_attribute.",
	},
	"gitlab_get_custom_attribute": {
		usage:       "Get one custom attribute by key for a user, group, or project (admin only).",
		aliases:     []string{"get custom attribute", "show custom attribute", "read custom metadata"},
		related:     []string{actionCustomAttrList, actionCustomAttrSet, actionCustomAttrDel},
		description: "Get a custom attribute by key. Returns: the attribute with key and value. See also: gitlab_list_custom_attributes, gitlab_set_custom_attribute.",
	},
	"gitlab_set_custom_attribute": {
		usage:       "Set (create or update) a custom attribute by key on a user, group, or project (admin only). Provide key and value.",
		aliases:     []string{"set custom attribute", "create custom attribute", "update custom metadata"},
		related:     []string{actionCustomAttrList, actionCustomAttrGet, actionCustomAttrDel},
		description: "Set a custom attribute. Returns: the stored attribute with key and value. See also: gitlab_get_custom_attribute, gitlab_delete_custom_attribute.",
	},
	"gitlab_delete_custom_attribute": {
		usage:       "Delete a custom attribute by key from a user, group, or project (admin only).",
		aliases:     []string{"delete custom attribute", "remove custom attribute", "clear custom metadata"},
		related:     []string{actionCustomAttrList, actionCustomAttrGet, actionCustomAttrSet},
		description: "Delete a custom attribute. Returns: a success status. See also: gitlab_get_custom_attribute, gitlab_set_custom_attribute.",
	},
	"gitlab_start_bulk_import": {
		usage:       "Start a group/project migration by direct transfer from another GitLab instance (admin/maintainer). Provide the source instance URL, access token, and entities to import.",
		aliases:     []string{"start bulk import", "migrate by direct transfer", "begin gitlab migration"},
		related:     []string{actionBulkImportList, actionBulkImportGet, actionBulkImportEntList},
		description: "Start a bulk import (direct transfer). Returns: the created bulk import with id and status. See also: gitlab_list_bulk_imports, gitlab_get_bulk_import.",
	},
	"gitlab_list_bulk_imports": {
		usage:       "List bulk imports (direct-transfer migrations) started by the authenticated user, with their status.",
		aliases:     []string{"list bulk imports", "show migrations", "direct transfer imports"},
		related:     []string{"admin.bulk_import_start", actionBulkImportGet, actionBulkImportEntList},
		description: "List bulk imports. Returns: an array of bulk imports with id, status, source_type, and timestamps. See also: gitlab_start_bulk_import, gitlab_get_bulk_import.",
	},
	"gitlab_get_bulk_import": {
		usage:       "Get one bulk import (direct-transfer migration) by id to inspect its overall status.",
		aliases:     []string{"get bulk import", "show migration", "bulk import status"},
		related:     []string{actionBulkImportList, "admin.bulk_import_cancel", actionBulkImportEntList},
		description: "Get a bulk import by id. Returns: the bulk import with status, source URL, and timestamps. See also: gitlab_list_bulk_imports, gitlab_list_bulk_import_entities.",
	},
	"gitlab_cancel_bulk_import": {
		usage:       "Cancel an in-progress bulk import (direct-transfer migration) by id.",
		aliases:     []string{"cancel bulk import", "stop migration", "abort direct transfer"},
		related:     []string{actionBulkImportGet, actionBulkImportList, actionBulkImportEntList},
		description: "Cancel a bulk import. Returns: the bulk import with its updated (canceled) status. See also: gitlab_get_bulk_import, gitlab_list_bulk_imports.",
	},
	"gitlab_list_bulk_import_entities": {
		usage:       "List the entities (groups and projects) being migrated within a bulk import, with per-entity status. Provide the bulk import id.",
		aliases:     []string{"list bulk import entities", "migration entities", "show import entities"},
		related:     []string{actionBulkImportGet, "admin.bulk_import_entity_get", "admin.bulk_import_entity_failures"},
		description: "List bulk import entities. Returns: an array of migrated entities with id, source_full_path, entity_type, and status. See also: gitlab_get_bulk_import_entity, gitlab_list_bulk_import_entity_failures.",
	},
	"gitlab_get_bulk_import_entity": {
		usage:       "Get one entity (group or project) within a bulk import by import id and entity id to inspect its migration status.",
		aliases:     []string{"get bulk import entity", "migration entity status", "show import entity"},
		related:     []string{actionBulkImportEntList, "admin.bulk_import_entity_failures", actionBulkImportGet},
		description: "Get a bulk import entity. Returns: the entity with source path, destination path, entity_type, and status. See also: gitlab_list_bulk_import_entities, gitlab_list_bulk_import_entity_failures.",
	},
	"gitlab_list_bulk_import_entity_failures": {
		usage:       "List the failures recorded for one bulk import entity, useful for diagnosing partial migrations. Provide import id and entity id.",
		aliases:     []string{"list bulk import entity failures", "migration failures", "show import errors"},
		related:     []string{"admin.bulk_import_entity_get", actionBulkImportEntList, actionBulkImportGet},
		description: "List bulk import entity failures. Returns: an array of failure records with relation, exception, and correlation id. See also: gitlab_get_bulk_import_entity, gitlab_list_bulk_import_entities.",
	},
	"gitlab_list_error_tracking_client_keys": {
		usage:       "List the error-tracking client keys configured for a project (used by the integrated error tracking feature). Provide project_id.",
		aliases:     []string{"list error tracking client keys", "error tracking keys", "show sentry client keys"},
		related:     []string{"admin.error_tracking_create", "admin.error_tracking_delete", actionErrTrackingGet},
		description: "List error-tracking client keys. Returns: an array of client keys with id, public_key, and sentry_dsn. See also: gitlab_create_error_tracking_client_key, gitlab_get_error_tracking_settings.",
	},
	"gitlab_create_error_tracking_client_key": {
		usage:       "Create an error-tracking client key for a project's integrated error tracking. Provide project_id.",
		aliases:     []string{"create error tracking client key", "add error tracking key", "new sentry client key"},
		related:     []string{actionErrTrackingList, "admin.error_tracking_delete", actionErrTrackingGet},
		description: "Create an error-tracking client key. Returns: the created client key with public_key and sentry_dsn. See also: gitlab_list_error_tracking_client_keys, gitlab_get_error_tracking_settings.",
	},
	"gitlab_delete_error_tracking_client_key": {
		usage:       "Delete an error-tracking client key from a project by project_id and key id.",
		aliases:     []string{"delete error tracking client key", "remove error tracking key", "revoke sentry client key"},
		related:     []string{actionErrTrackingList, "admin.error_tracking_create", actionErrTrackingGet},
		description: "Delete an error-tracking client key. Returns: a success status. See also: gitlab_list_error_tracking_client_keys, gitlab_create_error_tracking_client_key.",
	},
	"gitlab_get_error_tracking_settings": {
		usage:       "Get the integrated error-tracking settings for a project, including whether it is enabled. Provide project_id.",
		aliases:     []string{"get error tracking settings", "error tracking config", "show error tracking status"},
		related:     []string{"admin.error_tracking_update_settings", actionErrTrackingList},
		description: "Get error-tracking settings. Returns: the settings with active flag and integrated mode. See also: gitlab_enable_disable_error_tracking, gitlab_list_error_tracking_client_keys.",
	},
	"gitlab_enable_disable_error_tracking": {
		usage:       "Enable or disable integrated error tracking for a project. Provide project_id and the active flag (and integrated mode where applicable).",
		aliases:     []string{"enable error tracking", "disable error tracking", "toggle error tracking"},
		related:     []string{actionErrTrackingGet, actionErrTrackingList},
		description: "Enable or disable error tracking. Returns: the updated error-tracking settings. See also: gitlab_get_error_tracking_settings, gitlab_list_error_tracking_client_keys.",
	},
	"gitlab_list_alert_metric_images": {
		usage:       "List the metric images attached to an alert in a project's incident/alert management. Provide project_id and alert_iid.",
		aliases:     []string{"list alert metric images", "alert metric screenshots", "show alert images"},
		related:     []string{actionAlertImgUpload, actionAlertImgUpdate, actionAlertImgDelete},
		description: "List alert metric images. Returns: an array of metric images with id, filename, url, and url_text. See also: gitlab_upload_alert_metric_image, gitlab_update_alert_metric_image.",
	},
	"gitlab_upload_alert_metric_image": {
		usage:       "Upload a metric image to an alert in a project. Provide project_id, alert_iid, and the image file. Optionally a url and url_text.",
		aliases:     []string{"upload alert metric image", "attach alert screenshot", "add alert metric image"},
		related:     []string{actionAlertImgList, actionAlertImgUpdate, actionAlertImgDelete},
		description: "Upload an alert metric image. Returns: the uploaded metric image with id, filename, and url. See also: gitlab_list_alert_metric_images, gitlab_update_alert_metric_image.",
	},
	"gitlab_update_alert_metric_image": {
		usage:       "Update the url or url_text of an existing alert metric image. Provide project_id, alert_iid, and image id.",
		aliases:     []string{"update alert metric image", "edit alert image link", "change alert metric image"},
		related:     []string{actionAlertImgList, actionAlertImgUpload, actionAlertImgDelete},
		description: "Update an alert metric image. Returns: the updated metric image. See also: gitlab_list_alert_metric_images, gitlab_delete_alert_metric_image.",
	},
	"gitlab_delete_alert_metric_image": {
		usage:       "Delete a metric image from an alert. Provide project_id, alert_iid, and image id.",
		aliases:     []string{"delete alert metric image", "remove alert screenshot", "drop alert metric image"},
		related:     []string{actionAlertImgList, actionAlertImgUpload, actionAlertImgUpdate},
		description: "Delete an alert metric image. Returns: a success status. See also: gitlab_list_alert_metric_images, gitlab_upload_alert_metric_image.",
	},
	"gitlab_list_secure_files": {
		usage:       "List the CI/CD secure files stored for a project (certificates, provisioning profiles, and similar). Provide project_id.",
		aliases:     []string{"list secure files", "ci secure files", "show project secure files"},
		related:     []string{actionSecureFileGet, actionSecureFileCreate, actionSecureFileDel},
		description: "List project secure files. Returns: an array of secure files with id, name, checksum, and created_at. See also: gitlab_show_secure_file, gitlab_create_secure_file.",
	},
	"gitlab_show_secure_file": {
		usage:       "Get metadata for one CI/CD secure file by project_id and secure file id.",
		aliases:     []string{"show secure file", "get secure file", "secure file details"},
		related:     []string{actionSecureFileList, actionSecureFileCreate, actionSecureFileDel},
		description: "Show a secure file. Returns: the secure file with id, name, checksum, and metadata. See also: gitlab_list_secure_files, gitlab_create_secure_file.",
	},
	"gitlab_create_secure_file": {
		usage:       "Upload a CI/CD secure file to a project. Provide project_id, the name, and the file content.",
		aliases:     []string{"create secure file", "upload secure file", "add ci secure file"},
		related:     []string{actionSecureFileList, actionSecureFileGet, actionSecureFileDel},
		description: "Create a secure file. Returns: the created secure file with id, name, and checksum. See also: gitlab_list_secure_files, gitlab_show_secure_file.",
	},
	"gitlab_remove_secure_file": {
		usage:       "Remove a CI/CD secure file from a project by project_id and secure file id.",
		aliases:     []string{"remove secure file", "delete secure file", "drop ci secure file"},
		related:     []string{actionSecureFileList, actionSecureFileGet, actionSecureFileCreate},
		description: "Remove a secure file. Returns: a success status. See also: gitlab_list_secure_files, gitlab_create_secure_file.",
	},
	"gitlab_list_terraform_states": {
		usage:       "List the Terraform states stored in a project's Terraform state backend. Provide project_id.",
		aliases:     []string{"list terraform states", "show terraform states", "project terraform states"},
		related:     []string{actionTerraformStateGet, actionTerraformStateLock, "admin.terraform_state_delete"},
		description: "List Terraform states. Returns: an array of states with name, lock status, and latest version. See also: gitlab_get_terraform_state, gitlab_lock_terraform_state.",
	},
	"gitlab_get_terraform_state": {
		usage:       "Get one Terraform state by project_id and state name, including its lock status.",
		aliases:     []string{"get terraform state", "show terraform state", "terraform state details"},
		related:     []string{actionTerraformStateList, actionTerraformStateLock, "admin.terraform_state_unlock"},
		description: "Get a Terraform state. Returns: the state with name, lock info, and latest version. See also: gitlab_list_terraform_states, gitlab_lock_terraform_state.",
	},
	"gitlab_delete_terraform_state": {
		usage:       "Delete a Terraform state by project_id and state name (admin/maintainer). Removes all versions of the state.",
		aliases:     []string{"delete terraform state", "remove terraform state", "drop terraform state"},
		related:     []string{actionTerraformStateGet, actionTerraformStateList, "admin.terraform_version_delete"},
		description: "Delete a Terraform state. Returns: a success status. See also: gitlab_get_terraform_state, gitlab_delete_terraform_state_version.",
	},
	"gitlab_lock_terraform_state": {
		usage:       "Lock a Terraform state to prevent concurrent applies. Provide project_id and the state name.",
		aliases:     []string{"lock terraform state", "acquire terraform state lock", "terraform state lock"},
		related:     []string{"admin.terraform_state_unlock", actionTerraformStateGet, actionTerraformStateList},
		description: "Lock a Terraform state. Returns: the state with its lock acquired. See also: gitlab_unlock_terraform_state, gitlab_get_terraform_state.",
	},
	"gitlab_delete_terraform_state_version": {
		usage:       "Delete a single version of a Terraform state by project_id, state name, and serial/version number.",
		aliases:     []string{"delete terraform state version", "remove terraform state version", "drop terraform state serial"},
		related:     []string{"admin.terraform_state_delete", actionTerraformStateGet, actionTerraformStateList},
		description: "Delete a Terraform state version. Returns: a success status. See also: gitlab_delete_terraform_state, gitlab_get_terraform_state.",
	},
	"gitlab_list_cluster_agents": {
		usage:       "List the GitLab Agents for Kubernetes registered in a project. Provide project_id.",
		aliases:     []string{"list cluster agents", "list kubernetes agents", "show k8s agents"},
		related:     []string{"admin.cluster_agent_get", "admin.cluster_agent_register", actionClusterAgentTokLst},
		description: "List project cluster agents. Returns: an array of agents with id, name, and created_at. See also: gitlab_get_cluster_agent, gitlab_register_cluster_agent.",
	},
	"gitlab_get_cluster_agent": {
		usage:       "Get one GitLab Agent for Kubernetes by project_id and agent id.",
		aliases:     []string{"get cluster agent", "get kubernetes agent", "show k8s agent"},
		related:     []string{actionClusterAgentList, "admin.cluster_agent_delete", actionClusterAgentTokLst},
		description: "Get a cluster agent. Returns: the agent with id, name, config project, and created_at. See also: gitlab_list_cluster_agents, gitlab_list_cluster_agent_tokens.",
	},
	"gitlab_register_cluster_agent": {
		usage:       "Register a new GitLab Agent for Kubernetes in a project. Provide project_id and the agent name.",
		aliases:     []string{"register cluster agent", "create kubernetes agent", "add k8s agent"},
		related:     []string{actionClusterAgentList, "admin.cluster_agent_delete", actionClusterAgentTokCrt},
		description: "Register a cluster agent. Returns: the created agent with id and name. See also: gitlab_list_cluster_agents, gitlab_create_cluster_agent_token.",
	},
	"gitlab_delete_cluster_agent": {
		usage:       "Delete a GitLab Agent for Kubernetes from a project by project_id and agent id.",
		aliases:     []string{"delete cluster agent", "remove kubernetes agent", "unregister k8s agent"},
		related:     []string{actionClusterAgentList, "admin.cluster_agent_get", "admin.cluster_agent_register"},
		description: "Delete a cluster agent. Returns: a success status. See also: gitlab_get_cluster_agent, gitlab_register_cluster_agent.",
	},
	"gitlab_list_cluster_agent_tokens": {
		usage:       "List the tokens for a GitLab Agent for Kubernetes. Provide project_id and agent id.",
		aliases:     []string{"list cluster agent tokens", "kubernetes agent tokens", "show k8s agent tokens"},
		related:     []string{actionClusterAgentTokGet, actionClusterAgentTokCrt, actionClusterAgentTokRev},
		description: "List cluster agent tokens. Returns: an array of agent tokens with id, name, and status. See also: gitlab_get_cluster_agent_token, gitlab_create_cluster_agent_token.",
	},
	"gitlab_get_cluster_agent_token": {
		usage:       "Get one cluster agent token by project_id, agent id, and token id.",
		aliases:     []string{"get cluster agent token", "kubernetes agent token details", "show k8s agent token"},
		related:     []string{actionClusterAgentTokLst, actionClusterAgentTokCrt, actionClusterAgentTokRev},
		description: "Get a cluster agent token. Returns: the token with id, name, status, and last_used_at. See also: gitlab_list_cluster_agent_tokens, gitlab_create_cluster_agent_token.",
	},
	"gitlab_create_cluster_agent_token": {
		usage:       "Create a token for a GitLab Agent for Kubernetes. Provide project_id, agent id, and a token name.",
		aliases:     []string{"create cluster agent token", "new kubernetes agent token", "add k8s agent token"},
		related:     []string{actionClusterAgentTokLst, actionClusterAgentTokGet, actionClusterAgentTokRev},
		description: "Create a cluster agent token. Returns: the created token including the secret value (shown once). See also: gitlab_list_cluster_agent_tokens, gitlab_revoke_cluster_agent_token.",
	},
	"gitlab_revoke_cluster_agent_token": {
		usage:       "Revoke a cluster agent token by project_id, agent id, and token id, immediately invalidating it.",
		aliases:     []string{"revoke cluster agent token", "delete kubernetes agent token", "invalidate k8s agent token"},
		related:     []string{actionClusterAgentTokLst, actionClusterAgentTokGet, actionClusterAgentTokCrt},
		description: "Revoke a cluster agent token. Returns: a success status. See also: gitlab_get_cluster_agent_token, gitlab_create_cluster_agent_token.",
	},
	"gitlab_purge_dependency_proxy": {
		usage:       "Purge the dependency proxy cache for a group, freeing storage used by cached upstream images. Provide the group id.",
		aliases:     []string{"purge dependency proxy", "clear dependency proxy cache", "flush dependency proxy"},
		related:     []string{"group.get", actionSettingsGet},
		description: "Purge a group's dependency proxy cache. Returns: a success status. See also: gitlab_group_get, gitlab_get_settings.",
	},
	"gitlab_import_from_github": {
		usage:       "Import a repository from GitHub into GitLab. Provide a GitHub personal access token, the repo_id, and the target namespace.",
		aliases:     []string{"import from github", "migrate github repository", "github import"},
		related:     []string{"admin.import_cancel_github", "admin.import_gists", "admin.import_bitbucket"},
		description: "Import a repository from GitHub. Returns: the created project for the imported repository with its import status. See also: gitlab_cancel_github_import, gitlab_import_github_gists.",
	},
	"gitlab_cancel_github_import": {
		usage:       "Cancel an in-progress GitHub import for a project. Provide the project_id of the importing project.",
		aliases:     []string{"cancel github import", "stop github migration", "abort github import"},
		related:     []string{actionImportGitHub, "admin.import_gists"},
		description: "Cancel a GitHub import. Returns: the project with its updated import status. See also: gitlab_import_from_github, gitlab_import_github_gists.",
	},
	"gitlab_import_github_gists": {
		usage:       "Import the authenticated user's GitHub gists into GitLab snippets. Provide a GitHub personal access token.",
		aliases:     []string{"import github gists", "migrate gists", "import gists to snippets"},
		related:     []string{actionImportGitHub, "admin.import_cancel_github"},
		description: "Import GitHub gists into snippets. Returns: a success status. The import runs asynchronously. See also: gitlab_import_from_github, gitlab_cancel_github_import.",
	},
	"gitlab_import_from_bitbucket_cloud": {
		usage:       "Import a repository from Bitbucket Cloud into GitLab. Provide Bitbucket credentials, the source repo, and the target namespace.",
		aliases:     []string{"import from bitbucket cloud", "migrate bitbucket cloud repository", "bitbucket cloud import"},
		related:     []string{"admin.import_bitbucket_server", actionImportGitHub},
		description: "Import a repository from Bitbucket Cloud. Returns: the created project for the imported repository. See also: gitlab_import_from_bitbucket_server, gitlab_import_from_github.",
	},
	"gitlab_import_from_bitbucket_server": {
		usage:       "Import a repository from a self-hosted Bitbucket Server into GitLab. Provide the Bitbucket Server URL, credentials, the source project/repo, and the target namespace.",
		aliases:     []string{"import from bitbucket server", "migrate bitbucket server repository", "bitbucket server import"},
		related:     []string{"admin.import_bitbucket", actionImportGitHub},
		description: "Import a repository from Bitbucket Server. Returns: the created project for the imported repository. See also: gitlab_import_from_bitbucket_cloud, gitlab_import_from_github.",
	},
}
