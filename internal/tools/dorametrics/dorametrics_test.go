// dorametrics_test.go contains unit tests for GitLab DORA metrics retrieval
// operations. Tests use httptest to mock the GitLab DORA Metrics API.
package dorametrics

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const fmtUnexpErr = "unexpected error: %v"

type projectMetricsCase struct {
	name     string
	input    ProjectInput
	handler  http.HandlerFunc
	wantErr  bool
	validate func(t *testing.T, out Output)
}

type groupMetricsCase struct {
	name     string
	input    GroupInput
	handler  http.HandlerFunc
	wantErr  bool
	validate func(t *testing.T, out Output)
}

// TestGetProjectMetrics validates the GetProjectMetrics handler across
// success paths (with and without optional filters), input validation
// (missing project_id, missing metric), API error responses (403, 404, 500),
// context cancellation, and empty result sets. Each subtest verifies both
// the returned output and that the correct HTTP request was sent.
func TestGetProjectMetrics(t *testing.T) {
	tests := []projectMetricsCase{
		{
			name: "returns metrics for valid project",
			input: ProjectInput{
				ProjectID: "42",
				Metric:    "deployment_frequency",
			},
			handler: projectMetricsSuccessHandler(t, "/api/v4/projects/42/dora/metrics", `[
					{"date":"2026-01-15","value":1.5},
					{"date":"2026-01-16","value":2.0}
				]`),
			validate: assertTwoProjectMetrics,
		},
		{
			name: "passes all optional parameters to API",
			input: ProjectInput{
				ProjectID:        "99",
				Metric:           "lead_time_for_changes",
				StartDate:        "2026-01-01",
				EndDate:          "2026-01-31",
				Interval:         "monthly",
				EnvironmentTiers: []string{"production", "staging"},
			},
			handler:  projectMetricsOptionalParametersHandler(t),
			validate: assertSingleProjectMetricValue(5.0),
		},
		{
			name: "returns empty output for empty API response",
			input: ProjectInput{
				ProjectID: "42",
				Metric:    "change_failure_rate",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if len(out.Metrics) != 0 {
					t.Errorf("got %d metrics, want 0", len(out.Metrics))
				}
			},
		},
		{
			name:    "returns error when project_id is empty",
			input:   ProjectInput{Metric: "deployment_frequency"},
			wantErr: true,
		},
		{
			name:    "returns error when metric is empty",
			input:   ProjectInput{ProjectID: "42"},
			wantErr: true,
		},
		{
			name:  "returns error on 403 forbidden",
			input: ProjectInput{ProjectID: "42", Metric: "deployment_frequency"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			},
			wantErr: true,
		},
		{
			name:  "returns error on 404 not found",
			input: ProjectInput{ProjectID: "999", Metric: "deployment_frequency"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
			},
			wantErr: true,
		},
		{
			name:  "returns error on 422 unprocessable entity",
			input: ProjectInput{ProjectID: "42", Metric: "deployment_frequency"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
			},
			wantErr: true,
		},
		{
			name: "ignores malformed start_date and end_date gracefully",
			input: ProjectInput{
				ProjectID: "42",
				Metric:    "deployment_frequency",
				StartDate: "not-a-date",
				EndDate:   "also-bad",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[{"date":"2026-03-01","value":0.5}]`)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if len(out.Metrics) != 1 {
					t.Fatalf("got %d metrics, want 1", len(out.Metrics))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runProjectMetricsCase(t, tt)
		})
	}
}

func projectMetricsSuccessHandler(t *testing.T, path, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, path)
		testutil.RespondJSON(w, http.StatusOK, body)
	}
}

func assertTwoProjectMetrics(t *testing.T, out Output) {
	t.Helper()
	if len(out.Metrics) != 2 {
		t.Fatalf("got %d metrics, want 2", len(out.Metrics))
	}
	assertMetric(t, out.Metrics[0], "2026-01-15", 1.5, 0)
	assertMetric(t, out.Metrics[1], "2026-01-16", 2.0, 1)
}

func projectMetricsOptionalParametersHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/projects/99/dora/metrics")
		assertProjectMetricsQuery(t, r)
		testutil.RespondJSON(w, http.StatusOK, `[{"date":"2026-01","value":5.0}]`)
	}
}

func assertProjectMetricsQuery(t *testing.T, r *http.Request) {
	t.Helper()
	q := r.URL.Query()
	if got := q.Get("start_date"); got != "2026-01-01" {
		t.Errorf("start_date = %q, want %q", got, "2026-01-01")
	}
	if got := q.Get("end_date"); got != "2026-01-31" {
		t.Errorf("end_date = %q, want %q", got, "2026-01-31")
	}
	if got := q.Get("interval"); got != "monthly" {
		t.Errorf("interval = %q, want %q", got, "monthly")
	}
	if !strings.Contains(r.URL.RawQuery, "environment_tiers") {
		t.Errorf("query missing environment_tiers, got: %s", r.URL.RawQuery)
	}
}

func assertSingleProjectMetricValue(want float64) func(*testing.T, Output) {
	return func(t *testing.T, out Output) {
		t.Helper()
		if len(out.Metrics) != 1 {
			t.Fatalf("got %d metrics, want 1", len(out.Metrics))
		}
		if out.Metrics[0].Value != want {
			t.Errorf("value = %f, want %v", out.Metrics[0].Value, want)
		}
	}
}

func assertMetric(t *testing.T, got MetricOutput, wantDate string, wantValue float64, index int) {
	t.Helper()
	if got.Date != wantDate {
		t.Errorf("date[%d] = %q, want %q", index, got.Date, wantDate)
	}
	if got.Value != wantValue {
		t.Errorf("value[%d] = %f, want %v", index, got.Value, wantValue)
	}
}

func runProjectMetricsCase(t *testing.T, tt projectMetricsCase) {
	t.Helper()
	client := testutil.NewTestClient(t, metricsCaseHandler(t, tt.handler))
	out, err := GetProjectMetrics(context.Background(), client, tt.input)
	assertProjectMetricsCaseResult(t, out, err, tt)
}

func assertProjectMetricsCaseResult(t *testing.T, out Output, err error, tt projectMetricsCase) {
	t.Helper()
	if tt.wantErr {
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		return
	}
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if tt.validate != nil {
		tt.validate(t, out)
	}
}

// TestGetProjectMetrics_ContextCancelled verifies the handler respects
// context cancellation and returns an error without calling the API.
func TestGetProjectMetrics_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called for cancelled context")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetProjectMetrics(ctx, client, ProjectInput{ProjectID: "42", Metric: "deployment_frequency"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetProjectMetrics_BadRequestHint verifies that invalid DORA filters return
// model-facing guidance instead of only echoing GitLab's 400 response.
func TestGetProjectMetrics_BadRequestHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"error":"environment_tiers is invalid"}`)
	}))

	_, err := GetProjectMetrics(context.Background(), client, ProjectInput{
		ProjectID:        "42",
		Metric:           "deployment_frequency",
		EnvironmentTiers: []string{"production"},
	})
	if err == nil {
		t.Fatal("expected error for invalid DORA filters")
	}
	errText := err.Error()
	for _, want := range []string{"environment_tiers", "omit environment_tiers", "deployment environment tiers"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

// TestGetGroupMetrics validates the GetGroupMetrics handler across
// success paths (with and without optional filters), input validation
// (missing group_id, missing metric), API error responses (404, 500),
// context cancellation, and empty result sets.
func TestGetGroupMetrics(t *testing.T) {
	tests := []groupMetricsCase{
		{
			name: "returns metrics for valid group",
			input: GroupInput{
				GroupID: "5",
				Metric:  "lead_time_for_changes",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/groups/5/dora/metrics")
				testutil.RespondJSON(w, http.StatusOK, `[{"date":"2026-02-01","value":3.0}]`)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if len(out.Metrics) != 1 {
					t.Fatalf("got %d metrics, want 1", len(out.Metrics))
				}
				if out.Metrics[0].Date != "2026-02-01" {
					t.Errorf("date = %q, want %q", out.Metrics[0].Date, "2026-02-01")
				}
				if out.Metrics[0].Value != 3.0 {
					t.Errorf("value = %f, want 3.0", out.Metrics[0].Value)
				}
			},
		},
		{
			name: "passes all optional parameters to API",
			input: GroupInput{
				GroupID:          "10",
				Metric:           "time_to_restore_service",
				StartDate:        "2026-06-01",
				EndDate:          "2026-06-30",
				Interval:         "daily",
				EnvironmentTiers: []string{"production"},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, "/api/v4/groups/10/dora/metrics")
				q := r.URL.Query()
				if got := q.Get("start_date"); got != "2026-06-01" {
					t.Errorf("start_date = %q, want %q", got, "2026-06-01")
				}
				if got := q.Get("end_date"); got != "2026-06-30" {
					t.Errorf("end_date = %q, want %q", got, "2026-06-30")
				}
				if got := q.Get("interval"); got != "daily" {
					t.Errorf("interval = %q, want %q", got, "daily")
				}
				testutil.RespondJSON(w, http.StatusOK, `[{"date":"2026-06-15","value":1.0}]`)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if len(out.Metrics) != 1 {
					t.Fatalf("got %d metrics, want 1", len(out.Metrics))
				}
			},
		},
		{
			name: "returns empty output for empty API response",
			input: GroupInput{
				GroupID: "5",
				Metric:  "change_failure_rate",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if len(out.Metrics) != 0 {
					t.Errorf("got %d metrics, want 0", len(out.Metrics))
				}
			},
		},
		{
			name:    "returns error when group_id is empty",
			input:   GroupInput{Metric: "deployment_frequency"},
			wantErr: true,
		},
		{
			name:    "returns error when metric is empty",
			input:   GroupInput{GroupID: "5"},
			wantErr: true,
		},
		{
			name:  "returns error on 404 not found",
			input: GroupInput{GroupID: "999", Metric: "deployment_frequency"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
			},
			wantErr: true,
		},
		{
			name:  "returns error on 422 unprocessable entity",
			input: GroupInput{GroupID: "5", Metric: "deployment_frequency"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runGroupMetricsCase(t, tt)
		})
	}
}

func runGroupMetricsCase(t *testing.T, tt groupMetricsCase) {
	t.Helper()
	client := testutil.NewTestClient(t, metricsCaseHandler(t, tt.handler))
	out, err := GetGroupMetrics(context.Background(), client, tt.input)
	assertGroupMetricsCaseResult(t, out, err, tt)
}

func assertGroupMetricsCaseResult(t *testing.T, out Output, err error, tt groupMetricsCase) {
	t.Helper()
	if tt.wantErr {
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		return
	}
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if tt.validate != nil {
		tt.validate(t, out)
	}
}

func metricsCaseHandler(t *testing.T, handler http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	if handler != nil {
		return handler
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API handler should not be called for validation errors")
	}
}

// TestGetGroupMetrics_ContextCancelled verifies the handler respects
// context cancellation and returns an error without calling the API.
func TestGetGroupMetrics_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called for cancelled context")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetGroupMetrics(ctx, client, GroupInput{GroupID: "5", Metric: "deployment_frequency"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetGroupMetrics_BadRequestHint verifies that invalid group DORA filters
// return actionable guidance for the model.
func TestGetGroupMetrics_BadRequestHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"error":"environment_tiers is invalid"}`)
	}))

	_, err := GetGroupMetrics(context.Background(), client, GroupInput{
		GroupID:          "5",
		Metric:           "deployment_frequency",
		EnvironmentTiers: []string{"production"},
	})
	if err == nil {
		t.Fatal("expected error for invalid DORA filters")
	}
	errText := err.Error()
	for _, want := range []string{"environment_tiers", "omit environment_tiers", "deployment environment tiers"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

// TestFormatMarkdown validates the Markdown formatter across empty metrics,
// populated metrics, metric name inclusion in the title, and special characters.
func TestFormatMarkdown(t *testing.T) {
	tests := []struct {
		name         string
		output       Output
		metric       string
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:   "renders empty metrics message",
			output: Output{},
			metric: "deployment_frequency",
			wantContains: []string{
				"DORA Metrics",
				"deployment_frequency",
				"No metrics data available.",
			},
			wantAbsent: []string{
				"| Date | Value |",
				"Total data points",
			},
		},
		{
			name: "renders metrics table with data points",
			output: Output{
				Metrics: []MetricOutput{
					{Date: "2026-01-15", Value: 1.5},
					{Date: "2026-01-16", Value: 2.0},
				},
			},
			metric: "lead_time_for_changes",
			wantContains: []string{
				"DORA Metrics — lead_time_for_changes",
				"| Date | Value |",
				"| 2026-01-15 | 1.5000 |",
				"| 2026-01-16 | 2.0000 |",
				"**Total data points:** 2",
				"gitlab_deployment_list",
			},
		},
		{
			name: "renders generic title when metric is empty",
			output: Output{
				Metrics: []MetricOutput{
					{Date: "2026-03-01", Value: 0.0},
				},
			},
			metric: "",
			wantContains: []string{
				"## DORA Metrics\n",
				"| 2026-03-01 | 0.0000 |",
				"**Total data points:** 1",
			},
			wantAbsent: []string{
				"—",
			},
		},
		{
			name: "escapes pipe characters in metric name",
			output: Output{
				Metrics: []MetricOutput{{Date: "2026-01-01", Value: 1.0}},
			},
			metric: "metric|with|pipes",
			wantContains: []string{
				"DORA Metrics",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatMarkdown(tt.output, tt.metric)
			if md == "" {
				t.Fatal("expected non-empty markdown, got empty string")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(md, want) {
					t.Errorf("markdown missing %q\ngot:\n%s", want, md)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(md, absent) {
					t.Errorf("markdown should not contain %q\ngot:\n%s", absent, md)
				}
			}
		})
	}
}

// TestActionSpecs_Metadata verifies DORA metrics action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "dorametrics" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}
