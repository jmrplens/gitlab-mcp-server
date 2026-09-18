package dorametrics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// doraScope carries everything the two DORA metric actions do not share: the
// catalog action name, the individual tool it projects to, and the discovery
// metadata naming the resource the metrics are counted over.
//
// It is a parameter rather than a switch on the action name because a switch
// answers an unknown name with silence: a third scope added to ActionSpecs
// would reach the catalog carrying no usage, no aliases, no related action and
// no description — the generic placeholder metadata
// TestActionSpecs_DiscoveryMetadata exists to keep out — and nothing would say
// so. A scope that has to be constructed cannot be left out by omission.
type doraScope struct {
	name           string
	individualTool string
	usage          string
	aliasPhrases   []string
	relatedAction  string
	description    string
}

var (
	projectMetricsScope = doraScope{
		name:           "project",
		individualTool: "gitlab_get_project_dora_metrics",
		usage:          "Retrieves one DORA metric (deployment_frequency, lead_time_for_changes, time_to_restore_service, or change_failure_rate) for a project_id over a date window. interval is the bucket size (daily, monthly, all). For a prompt like `last 30 days`, compute start_date and end_date as YYYY-MM-DD and pass them. There is no `days` or `days_back` parameter.",
		aliasPhrases: []string{
			"project deployment frequency",
			"project lead time for changes",
			"project change failure rate",
			"project time to restore service",
			"project devops performance metrics",
		},
		relatedAction: "dora_metrics.group",
		description:   "Get the four DORA DevOps performance metrics for a project over a date window. Returns: one time series of date/value data points for the requested metric (deployment frequency, lead time for changes, time to restore service, or change failure rate), bucketed by interval. See also: gitlab_get_group_dora_metrics.",
	}

	groupMetricsScope = doraScope{
		name:           "group",
		individualTool: "gitlab_get_group_dora_metrics",
		usage:          "Retrieves one DORA metric (deployment_frequency, lead_time_for_changes, time_to_restore_service, or change_failure_rate) for a group_id over a date window. interval is the bucket size (daily, monthly, all). For a prompt like `last 30 days`, compute start_date and end_date as YYYY-MM-DD and pass them. There is no `days` or `days_back` parameter.",
		aliasPhrases: []string{
			"group deployment frequency",
			"group lead time for changes",
			"group change failure rate",
			"group time to restore service",
			"group devops performance metrics",
		},
		relatedAction: "dora_metrics.project",
		description:   "Get the four DORA DevOps performance metrics aggregated across a group over a date window. Returns: one time series of date/value data points for the requested metric (deployment frequency, lead time for changes, time to restore service, or change failure rate), bucketed by interval. See also: gitlab_get_project_dora_metrics.",
	}
)

// ActionSpecs returns canonical specs for DORA metric actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		doraMetricReadSpec(projectMetricsScope, toolutil.RouteAction(client, GetProjectMetrics)),
		doraMetricReadSpec(groupMetricsScope, toolutil.RouteAction(client, GetGroupMetrics)),
	}
}

func doraMetricReadSpec(scope doraScope, route toolutil.ActionRoute) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		// The individual tool's own name leads the aliases on every scope, so
		// it is composed here instead of being repeated in each scope beside
		// the field it would have to agree with.
		Aliases:        append([]string{scope.individualTool}, scope.aliasPhrases...),
		Tags:           []string{"analytics", "dora"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "dorametrics",
		Usage:          scope.usage,
		RelatedActions: []string{scope.relatedAction},
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        scope.individualTool,
			Title:       toolutil.TitleFromName(scope.individualTool),
			Description: scope.description,
		},
		InputSchemaOverrides: []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("metric", map[string]any{
				"enum": []any{"deployment_frequency", "lead_time_for_changes", "time_to_restore_service", "change_failure_rate"},
			}),
			toolutil.SchemaPropertyOverride("interval", map[string]any{
				"enum": []any{"daily", "monthly", "all"},
			}),
		},
	}
	return toolutil.NewReadActionSpec(scope.name, route, options)
}
