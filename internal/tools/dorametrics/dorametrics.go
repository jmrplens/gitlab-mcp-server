package dorametrics

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

func validateMetricsInput(resourceID toolutil.StringOrInt, resourceField, metric string) error {
	if resourceID == "" {
		return toolutil.ErrFieldRequired(resourceField)
	}
	if metric == "" {
		return toolutil.ErrFieldRequired("metric")
	}
	return nil
}

func wrapMetricsError(operation string, err error, notFoundHint, scope string) error {
	if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return toolutil.WrapErrWithHint(operation, err,
			"verify metric, interval, and date filters; omit environment_tiers unless the "+scope+
				" has matching deployment environment tiers such as production or staging")
	}
	return toolutil.WrapErrWithStatusHint(operation, err, http.StatusNotFound, notFoundHint)
}

// GetProjectMetrics retrieves DORA metrics for a project.
func GetProjectMetrics(ctx context.Context, client *gitlabclient.Client, input ProjectInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if err := validateMetricsInput(input.ProjectID, "project_id", input.Metric); err != nil {
		return Output{}, err
	}
	opts := buildOpts(input.Metric, input.StartDate, input.EndDate, input.Interval, input.EnvironmentTiers)
	metrics, _, err := client.GL().DORAMetrics.GetProjectDORAMetrics(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, wrapMetricsError("doraProjectMetrics", err, "verify project_id with gitlab_project_get \u2014 DORA metrics require Ultimate license", "project")
	}
	return toOutput(metrics), nil
}

// GetGroupMetrics retrieves DORA metrics for a group.
func GetGroupMetrics(ctx context.Context, client *gitlabclient.Client, input GroupInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if err := validateMetricsInput(input.GroupID, "group_id", input.Metric); err != nil {
		return Output{}, err
	}
	opts := buildOpts(input.Metric, input.StartDate, input.EndDate, input.Interval, input.EnvironmentTiers)
	metrics, _, err := client.GL().DORAMetrics.GetGroupDORAMetrics(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, wrapMetricsError("doraGroupMetrics", err, "verify group_id with gitlab_group_get \u2014 DORA metrics require Ultimate license", "group")
	}
	return toOutput(metrics), nil
}
