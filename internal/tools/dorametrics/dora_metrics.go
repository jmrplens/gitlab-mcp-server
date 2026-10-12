package dorametrics

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ProjectInput defines parameters for retrieving project-level DORA metrics.
type ProjectInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id"                  jsonschema:"Project ID or URL-encoded path,required"`
	Metric           string               `json:"metric"                      jsonschema:"DORA metric type: deployment_frequency, lead_time_for_changes, time_to_restore_service, change_failure_rate,required"`
	StartDate        string               `json:"start_date,omitempty"        jsonschema:"Start date (YYYY-MM-DD). A timestamp is read for its date. GitLab defaults to three months ago"`
	EndDate          string               `json:"end_date,omitempty"          jsonschema:"End date (YYYY-MM-DD). A timestamp is read for its date. GitLab defaults to today"`
	Interval         string               `json:"interval,omitempty"          jsonschema:"Aggregation interval: daily, monthly, all (default: daily)"`
	EnvironmentTiers []string             `json:"environment_tiers,omitempty" jsonschema:"Filter by environment tiers (e.g. production, staging)"`
}

// GroupInput defines parameters for retrieving group-level DORA metrics.
type GroupInput struct {
	GroupID          toolutil.StringOrInt `json:"group_id"                    jsonschema:"Group ID or URL-encoded path,required"`
	Metric           string               `json:"metric"                      jsonschema:"DORA metric type: deployment_frequency, lead_time_for_changes, time_to_restore_service, change_failure_rate,required"`
	StartDate        string               `json:"start_date,omitempty"        jsonschema:"Start date (YYYY-MM-DD). A timestamp is read for its date. GitLab defaults to three months ago"`
	EndDate          string               `json:"end_date,omitempty"          jsonschema:"End date (YYYY-MM-DD). A timestamp is read for its date. GitLab defaults to today"`
	Interval         string               `json:"interval,omitempty"          jsonschema:"Aggregation interval: daily, monthly, all (default: daily)"`
	EnvironmentTiers []string             `json:"environment_tiers,omitempty" jsonschema:"Filter by environment tiers (e.g. production, staging)"`
}

// MetricOutput represents a single DORA metric data point.
//
// Value is a pointer because GitLab answers a period with nothing to measure
// with a null value (a median over no rows), and client-go's DORAMetric
// decodes that null as 0, which for lead time and time to restore is the best
// score there is. The value is read from the captured answer instead (register
// row 116). Date is left out of the one bucket interval=all answers with,
// which GitLab sends undated.
type MetricOutput struct {
	Date  string   `json:"date,omitempty"`
	Value *float64 `json:"value"`
}

// capturedMetric is the one key of a data point read from the captured answer
// rather than from client-go's decode.
type capturedMetric struct {
	Value *float64 `json:"value"`
}

// Output holds a list of DORA metric data points.
type Output struct {
	toolutil.HintableOutput
	Metrics []MetricOutput `json:"metrics"`
}

// buildOpts maps a DORA query onto client-go's options, refusing a start or
// end date it cannot read. Both used to be dropped, and GitLab then answered
// for its default three months rather than the range the caller named
// (issue 1308).
func buildOpts(metric, startDate, endDate, interval string, tiers []string) (gl.GetDORAMetricsOptions, error) {
	opts := gl.GetDORAMetricsOptions{
		Metric: new(gl.DORAMetricType(metric)),
	}
	if err := toolutil.ParseDateInputs(
		toolutil.DateInput{Field: "start_date", Value: startDate, Dest: &opts.StartDate},
		toolutil.DateInput{Field: "end_date", Value: endDate, Dest: &opts.EndDate},
	); err != nil {
		return gl.GetDORAMetricsOptions{}, err
	}
	if interval != "" {
		opts.Interval = new(gl.DORAMetricInterval(interval))
	}
	if len(tiers) > 0 {
		opts.EnvironmentTiers = &tiers
	}
	return opts, nil
}

// capturedValues reads each data point's value from the captured answer, one
// per point in the order client-go decoded them. The two read the same bytes,
// so a count that differs is a fault in this reader, reported rather than
// paired wrongly.
func capturedValues(captured *gitlabclient.ResponseCapture, decoded int) ([]capturedMetric, error) {
	var values []capturedMetric
	if err := captured.Decode(&values); err != nil {
		return nil, err
	}
	if len(values) != decoded {
		return nil, fmt.Errorf("captured %d DORA data points, client-go decoded %d", len(values), decoded)
	}
	return values, nil
}

// toOutput pairs each data point client-go decoded with the value the captured
// answer carries for it, or reports under op an answer it cannot pair.
func toOutput(op string, metrics []gl.DORAMetric, captured *gitlabclient.ResponseCapture) (Output, error) {
	values, err := capturedValues(captured, len(metrics))
	if err != nil {
		return Output{}, toolutil.WrapErr(op, err)
	}
	out := make([]MetricOutput, len(metrics))
	for i, m := range metrics {
		out[i] = MetricOutput{Date: m.Date, Value: values[i].Value}
	}
	return Output{Metrics: out}, nil
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
	opts, err := buildOpts(input.Metric, input.StartDate, input.EndDate, input.Interval, input.EnvironmentTiers)
	if err != nil {
		return Output{}, err
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	metrics, _, err := client.GL().DORAMetrics.GetProjectDORAMetrics(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, wrapMetricsError("doraProjectMetrics", err, "verify project_id with project.get. DORA metrics require Ultimate license", "project")
	}
	return toOutput("doraProjectMetrics", metrics, captured)
}

// GetGroupMetrics retrieves DORA metrics for a group.
func GetGroupMetrics(ctx context.Context, client *gitlabclient.Client, input GroupInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if err := validateMetricsInput(input.GroupID, "group_id", input.Metric); err != nil {
		return Output{}, err
	}
	opts, err := buildOpts(input.Metric, input.StartDate, input.EndDate, input.Interval, input.EnvironmentTiers)
	if err != nil {
		return Output{}, err
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	metrics, _, err := client.GL().DORAMetrics.GetGroupDORAMetrics(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, wrapMetricsError("doraGroupMetrics", err, "verify group_id with group.get. DORA metrics require Ultimate license", "group")
	}
	return toOutput("doraGroupMetrics", metrics, captured)
}
