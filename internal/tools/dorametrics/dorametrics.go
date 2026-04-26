// Package dorametrics implements MCP tool handlers for GitLab DORA metrics
// retrieval at project and group levels. It wraps the DORAMetrics service
// from client-go v2.
package dorametrics

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ProjectInput defines parameters for retrieving project-level DORA metrics.
type ProjectInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id"                  jsonschema:"Project ID or URL-encoded path,required"`
	Metric           string               `json:"metric"                      jsonschema:"DORA metric type: deployment_frequency, lead_time_for_changes, time_to_restore_service, change_failure_rate,required"`
	StartDate        string               `json:"start_date,omitempty"        jsonschema:"Start date (YYYY-MM-DD)"`
	EndDate          string               `json:"end_date,omitempty"          jsonschema:"End date (YYYY-MM-DD)"`
	Interval         string               `json:"interval,omitempty"          jsonschema:"Aggregation interval: daily, monthly, all (default: daily)"`
	EnvironmentTiers []string             `json:"environment_tiers,omitempty" jsonschema:"Filter by environment tiers (e.g. production, staging)"`
}

// GroupInput defines parameters for retrieving group-level DORA metrics.
type GroupInput struct {
	GroupID          toolutil.StringOrInt `json:"group_id"                    jsonschema:"Group ID or URL-encoded path,required"`
	Metric           string               `json:"metric"                      jsonschema:"DORA metric type: deployment_frequency, lead_time_for_changes, time_to_restore_service, change_failure_rate,required"`
	StartDate        string               `json:"start_date,omitempty"        jsonschema:"Start date (YYYY-MM-DD)"`
	EndDate          string               `json:"end_date,omitempty"          jsonschema:"End date (YYYY-MM-DD)"`
	Interval         string               `json:"interval,omitempty"          jsonschema:"Aggregation interval: daily, monthly, all (default: daily)"`
	EnvironmentTiers []string             `json:"environment_tiers,omitempty" jsonschema:"Filter by environment tiers (e.g. production, staging)"`
}

// MetricOutput represents a single DORA metric data point.
type MetricOutput struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// Output holds a list of DORA metric data points.
type Output struct {
	toolutil.HintableOutput
	Metrics []MetricOutput `json:"metrics"`
}

func buildOpts(metric, startDate, endDate, interval string, tiers []string) gl.GetDORAMetricsOptions {
	opts := gl.GetDORAMetricsOptions{
		Metric: new(gl.DORAMetricType(metric)),
	}
	if startDate != "" {
		if t, err := time.Parse("2006-01-02", startDate); err == nil {
			iso := gl.ISOTime(t)
			opts.StartDate = &iso
		}
	}
	if endDate != "" {
		if t, err := time.Parse("2006-01-02", endDate); err == nil {
			iso := gl.ISOTime(t)
			opts.EndDate = &iso
		}
	}
	if interval != "" {
		opts.Interval = new(gl.DORAMetricInterval(interval))
	}
	if len(tiers) > 0 {
		opts.EnvironmentTiers = &tiers
	}
	return opts
}

func toOutput(metrics []gl.DORAMetric) Output {
	out := make([]MetricOutput, len(metrics))
	for i, m := range metrics {
		out[i] = MetricOutput{Date: m.Date, Value: m.Value}
	}
	return Output{Metrics: out}
}

// GetProjectMetrics retrieves DORA metrics for a project.
func GetProjectMetrics(ctx context.Context, client *gitlabclient.Client, input ProjectInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Metric == "" {
		return Output{}, toolutil.ErrFieldRequired("metric")
	}
	opts := buildOpts(input.Metric, input.StartDate, input.EndDate, input.Interval, input.EnvironmentTiers)
	metrics, _, err := client.GL().DORAMetrics.GetProjectDORAMetrics(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("doraProjectMetrics", err, http.StatusNotFound, "verify project_id with gitlab_get_project \u2014 DORA metrics require Ultimate license")
	}
	return toOutput(metrics), nil
}

// GetGroupMetrics retrieves DORA metrics for a group.
func GetGroupMetrics(ctx context.Context, client *gitlabclient.Client, input GroupInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Metric == "" {
		return Output{}, toolutil.ErrFieldRequired("metric")
	}
	opts := buildOpts(input.Metric, input.StartDate, input.EndDate, input.Interval, input.EnvironmentTiers)
	metrics, _, err := client.GL().DORAMetrics.GetGroupDORAMetrics(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("doraGroupMetrics", err, http.StatusNotFound, "verify group_id with gitlab_get_group \u2014 DORA metrics require Ultimate license")
	}
	return toOutput(metrics), nil
}
