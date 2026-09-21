// dora_metrics_test.go contains unit tests for GitLab DORA metrics retrieval
// operations. Tests use httptest to mock the GitLab DORA Metrics API.
package dorametrics

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

const fmtUnexpErr = "unexpected error: %v"

// metricsRequest is the whole DORA request a handler built: the path and every
// parameter the endpoint takes. Cases state one of these and it is compared as
// a unit, because the five parameters are interchangeable strings: a metric
// sent as a constant, a start date written into end_date, or a tier list the
// caller never named all produce a request that a per-parameter assertion of
// the kind these tests used to make passes over.
type metricsRequest struct {
	Path             string
	Metric           string
	StartDate        string
	EndDate          string
	Interval         string
	EnvironmentTiers string
}

// metricsHandler answers one DORA call with body, after holding the request
// the handler built to want. It reports with [testing.T.Errorf] and still
// answers, so the calling test's own assertions report too rather than the
// serving goroutine aborting them.
func metricsHandler(t *testing.T, want metricsRequest, body string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		q := r.URL.Query()
		got := metricsRequest{
			Path:             r.URL.Path,
			Metric:           q.Get("metric"),
			StartDate:        q.Get("start_date"),
			EndDate:          q.Get("end_date"),
			Interval:         q.Get("interval"),
			EnvironmentTiers: q.Get("environment_tiers"),
		}
		if got != want {
			t.Errorf("DORA request = %+v, want %+v", got, want)
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	})
}

// metricsStatusHandler answers every call with status and body, for the cases
// about what a handler makes of GitLab's refusal.
func metricsStatusHandler(status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, status, body)
	})
}

type projectMetricsCase struct {
	name     string
	input    ProjectInput
	handler  http.Handler
	wantErr  []string
	validate func(t *testing.T, out Output)
}

type groupMetricsCase struct {
	name     string
	input    GroupInput
	handler  http.Handler
	wantErr  []string
	validate func(t *testing.T, out Output)
}

// TestGetProjectMetrics validates the GetProjectMetrics handler across
// success paths (with and without optional filters), input validation
// (missing project_id, missing metric), API error responses (403, 404, 422),
// and empty result sets. Each subtest states the whole request the handler
// must build, so the metric and the filters are held to the caller's values
// rather than to their presence, and each refusal is held to the text it
// returns, so a case cannot pass on an error some other layer produced.
func TestGetProjectMetrics(t *testing.T) {
	tests := []projectMetricsCase{
		{
			name: "returns metrics for valid project",
			input: ProjectInput{
				ProjectID: "42",
				Metric:    "deployment_frequency",
			},
			handler: metricsHandler(t, metricsRequest{
				Path:   "/api/v4/projects/42/dora/metrics",
				Metric: "deployment_frequency",
			}, `[
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
			handler: metricsHandler(t, metricsRequest{
				Path:             "/api/v4/projects/99/dora/metrics",
				Metric:           "lead_time_for_changes",
				StartDate:        "2026-01-01",
				EndDate:          "2026-01-31",
				Interval:         "monthly",
				EnvironmentTiers: "production,staging",
			}, `[{"date":"2026-01","value":5.0}]`),
			validate: assertSingleProjectMetricValue(5.0),
		},
		{
			name: "returns empty output for empty API response",
			input: ProjectInput{
				ProjectID: "42",
				Metric:    "change_failure_rate",
			},
			handler: metricsHandler(t, metricsRequest{
				Path:   "/api/v4/projects/42/dora/metrics",
				Metric: "change_failure_rate",
			}, `[]`),
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
			wantErr: []string{"project_id is required"},
		},
		{
			name:    "returns error when metric is empty",
			input:   ProjectInput{ProjectID: "42"},
			wantErr: []string{"metric is required"},
		},
		{
			name:    "returns error on 403 forbidden",
			input:   ProjectInput{ProjectID: "42", Metric: "deployment_frequency"},
			handler: metricsStatusHandler(http.StatusForbidden, `{"message":"403 Forbidden"}`),
			wantErr: []string{"doraProjectMetrics", "403 Forbidden"},
		},
		{
			name:    "returns error on 404 not found",
			input:   ProjectInput{ProjectID: "999", Metric: "deployment_frequency"},
			handler: metricsStatusHandler(http.StatusNotFound, `{"message":"404 Project Not Found"}`),
			wantErr: []string{
				"doraProjectMetrics",
				"verify project_id with gitlab_project_get",
				"DORA metrics require Ultimate license",
			},
		},
		{
			name:    "returns error on 422 unprocessable entity",
			input:   ProjectInput{ProjectID: "42", Metric: "deployment_frequency"},
			handler: metricsStatusHandler(http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`),
			wantErr: []string{"doraProjectMetrics", "422 Unprocessable"},
		},
		{
			name: "drops a start_date and an end_date it cannot parse",
			input: ProjectInput{
				ProjectID: "42",
				Metric:    "deployment_frequency",
				StartDate: "not-a-date",
				EndDate:   "also-bad",
			},
			handler: metricsHandler(t, metricsRequest{
				Path:   "/api/v4/projects/42/dora/metrics",
				Metric: "deployment_frequency",
			}, `[{"date":"2026-03-01","value":0.5}]`),
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

func assertTwoProjectMetrics(t *testing.T, out Output) {
	t.Helper()
	if len(out.Metrics) != 2 {
		t.Fatalf("got %d metrics, want 2", len(out.Metrics))
	}
	assertMetric(t, out.Metrics[0], "2026-01-15", 1.5, 0)
	assertMetric(t, out.Metrics[1], "2026-01-16", 2.0, 1)
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
	assertMetricsCaseResult(t, out, err, tt.wantErr, tt.validate)
}

// assertMetricsCaseResult holds the outcome of one case to what it expects:
// every substring of wantErr must appear in the error, which is what says the
// refusal came from the layer the case is about and named what a model needs,
// rather than merely that something failed somewhere.
func assertMetricsCaseResult(t *testing.T, out Output, err error, wantErr []string, validate func(*testing.T, Output)) {
	t.Helper()
	if len(wantErr) > 0 {
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		for _, want := range wantErr {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %v missing %q", err, want)
			}
		}
		return
	}
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if validate != nil {
		validate(t, out)
	}
}

// TestGetProjectMetrics_ContextCancelled asserts that a canceled context ends
// the project call before it contacts GitLab: the mock fails the test if any
// request arrives.
func TestGetProjectMetrics_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := GetProjectMetrics(ctx, client, ProjectInput{ProjectID: "42", Metric: "deployment_frequency"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetProjectMetrics_BadRequestHint verifies that invalid DORA filters return
// model-facing guidance instead of only echoing GitLab's 400 response, and that
// the guidance is the project handler's own: the sentence names the project as
// the thing whose environment tiers to check, which is the one word that
// distinguishes it from the group handler's.
func TestGetProjectMetrics_BadRequestHint(t *testing.T) {
	client := testutil.NewTestClient(t, metricsStatusHandler(http.StatusBadRequest, `{"error":"environment_tiers is invalid"}`))

	_, err := GetProjectMetrics(context.Background(), client, ProjectInput{
		ProjectID:        "42",
		Metric:           "deployment_frequency",
		EnvironmentTiers: []string{"production"},
	})
	if err == nil {
		t.Fatal("expected error for invalid DORA filters")
	}
	errText := err.Error()
	wants := []string{
		"doraProjectMetrics",
		"environment_tiers",
		"omit environment_tiers",
		"unless the project has matching deployment environment tiers",
	}
	for _, want := range wants {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errText, want) {
				t.Fatalf("error missing %q: %v", want, err)
			}
		})
	}
}

// TestGetGroupMetrics validates the GetGroupMetrics handler across the same
// dimensions as its project sibling, against the group route: the whole
// request built from the caller's values, input validation (missing group_id,
// missing metric), GitLab's refusals (404, 422) and their group-scoped hints,
// and empty result sets.
func TestGetGroupMetrics(t *testing.T) {
	tests := []groupMetricsCase{
		{
			name: "returns metrics for valid group",
			input: GroupInput{
				GroupID: "5",
				Metric:  "lead_time_for_changes",
			},
			handler: metricsHandler(t, metricsRequest{
				Path:   "/api/v4/groups/5/dora/metrics",
				Metric: "lead_time_for_changes",
			}, `[{"date":"2026-02-01","value":3.0}]`),
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
				EnvironmentTiers: []string{"staging", "production"},
			},
			handler: metricsHandler(t, metricsRequest{
				Path:      "/api/v4/groups/10/dora/metrics",
				Metric:    "time_to_restore_service",
				StartDate: "2026-06-01",
				EndDate:   "2026-06-30",
				Interval:  "daily",
				// The caller's order is kept: GitLab reads the list as
				// written, so a tier list re-sorted on the way out would be a
				// different filter than the one asked for.
				EnvironmentTiers: "staging,production",
			}, `[{"date":"2026-06-15","value":1.0}]`),
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
			handler: metricsHandler(t, metricsRequest{
				Path:   "/api/v4/groups/5/dora/metrics",
				Metric: "change_failure_rate",
			}, `[]`),
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
			wantErr: []string{"group_id is required"},
		},
		{
			name:    "returns error when metric is empty",
			input:   GroupInput{GroupID: "5"},
			wantErr: []string{"metric is required"},
		},
		{
			name:    "returns error on 404 not found",
			input:   GroupInput{GroupID: "999", Metric: "deployment_frequency"},
			handler: metricsStatusHandler(http.StatusNotFound, `{"message":"404 Group Not Found"}`),
			wantErr: []string{
				"doraGroupMetrics",
				"verify group_id with gitlab_group_get",
				"DORA metrics require Ultimate license",
			},
		},
		{
			name:    "returns error on 422 unprocessable entity",
			input:   GroupInput{GroupID: "5", Metric: "deployment_frequency"},
			handler: metricsStatusHandler(http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`),
			wantErr: []string{"doraGroupMetrics", "422 Unprocessable"},
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
	assertMetricsCaseResult(t, out, err, tt.wantErr, tt.validate)
}

// metricsCaseHandler answers with the case's own handler, or, for the cases
// the handler must refuse before it reaches GitLab, with a mock that fails the
// test if any request arrives at all.
func metricsCaseHandler(t *testing.T, handler http.Handler) http.Handler {
	t.Helper()
	if handler != nil {
		return handler
	}
	return testutil.ForbiddenHandler(t)
}

// TestGetGroupMetrics_ContextCancelled asserts that a canceled context ends
// the group call before it contacts GitLab: the mock fails the test if any
// request arrives.
func TestGetGroupMetrics_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := GetGroupMetrics(ctx, client, GroupInput{GroupID: "5", Metric: "deployment_frequency"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetGroupMetrics_BadRequestHint verifies that the group handler answers a
// rejected filter with guidance of its own: the same sentence as the project
// handler's, naming the group. The two differ by that word alone, so a group
// call told to check a project's environment tiers is a defect no fixture
// value can show and only this assertion catches.
func TestGetGroupMetrics_BadRequestHint(t *testing.T) {
	client := testutil.NewTestClient(t, metricsStatusHandler(http.StatusBadRequest, `{"error":"environment_tiers is invalid"}`))

	_, err := GetGroupMetrics(context.Background(), client, GroupInput{
		GroupID:          "5",
		Metric:           "deployment_frequency",
		EnvironmentTiers: []string{"production"},
	})
	if err == nil {
		t.Fatal("expected error for invalid DORA filters")
	}
	errText := err.Error()
	wants := []string{
		"doraGroupMetrics",
		"environment_tiers",
		"omit environment_tiers",
		"unless the group has matching deployment environment tiers",
	}
	for _, want := range wants {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errText, want) {
				t.Fatalf("error missing %q: %v", want, err)
			}
		})
	}
}

// TestBuildOpts_EachOptionCarriesItsOwnValue holds the option struct the
// handlers hand client-go, on both sides: every field the caller filled
// carries that caller's value, and every field the caller left out is nil.
//
// The second half is asserted here rather than through a request because the
// wire cannot answer it. go-querystring skips a slice of length zero, so a
// *[]string pointing at an empty slice and a nil pointer produce byte-identical
// queries, and the guard that keeps environment_tiers unset is observable only
// on the struct. The first half is asserted through the handlers as well; it is
// repeated here because this is where a date written into its neighbour's field
// would show as a value rather than as a missing assertion.
func TestBuildOpts_EachOptionCarriesItsOwnValue(t *testing.T) {
	bare := buildOpts("deployment_frequency", "", "", "", nil)
	if bare.Metric == nil || string(*bare.Metric) != "deployment_frequency" {
		t.Errorf("metric = %v, want deployment_frequency", bare.Metric)
	}
	if bare.StartDate != nil {
		t.Errorf("start_date = %v, want unset", bare.StartDate)
	}
	if bare.EndDate != nil {
		t.Errorf("end_date = %v, want unset", bare.EndDate)
	}
	if bare.Interval != nil {
		t.Errorf("interval = %v, want unset", bare.Interval)
	}
	if bare.EnvironmentTiers != nil {
		t.Errorf("environment_tiers = %v, want unset", *bare.EnvironmentTiers)
	}

	full := buildOpts("change_failure_rate", "2026-01-02", "2026-03-04", "monthly", []string{"production", "staging"})
	if full.Metric == nil || string(*full.Metric) != "change_failure_rate" {
		t.Errorf("metric = %v, want change_failure_rate", full.Metric)
	}
	if full.StartDate == nil || full.StartDate.String() != "2026-01-02" {
		t.Errorf("start_date = %v, want 2026-01-02", full.StartDate)
	}
	if full.EndDate == nil || full.EndDate.String() != "2026-03-04" {
		t.Errorf("end_date = %v, want 2026-03-04", full.EndDate)
	}
	if full.Interval == nil || string(*full.Interval) != "monthly" {
		t.Errorf("interval = %v, want monthly", full.Interval)
	}
	if full.EnvironmentTiers == nil || strings.Join(*full.EnvironmentTiers, ",") != "production,staging" {
		t.Errorf("environment_tiers = %v, want [production staging]", full.EnvironmentTiers)
	}
}

// doraTableHead is the heading-less head of the DORA data point table, and
// doraHints the guidance section it closes with.
const (
	doraTableHead = "| Date | Value |\n| --- | --- |\n"
	doraHints     = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'environment.deployment_list' to correlate these metrics with deployment activity\n"
)

// TestFormatMarkdown validates the whole table the DORA formatter writes: an
// empty series renders the one sentence, a populated one renders each date in
// the display form and each value without the four decimals that stamped false
// precision on a count, and a metric name carrying a pipe changes no structure.
func TestFormatMarkdown(t *testing.T) {
	tests := []struct {
		name   string
		output Output
		metric string
		want   string
	}{
		{
			name:   "empty series renders the one sentence",
			output: Output{},
			metric: "deployment_frequency",
			want:   "No DORA metric data points found.\n",
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
			want: "## DORA Metrics: lead_time_for_changes (2)\n\n" +
				doraTableHead +
				"| 15 Jan 2026 | 1.5 |\n" +
				"| 16 Jan 2026 | 2 |\n" +
				doraHints,
		},
		{
			name: "renders generic title when metric is empty",
			output: Output{
				Metrics: []MetricOutput{{Date: "2026-03-01", Value: 0.0}},
			},
			metric: "",
			want: "## DORA Metrics (1)\n\n" +
				doraTableHead +
				"| 1 Mar 2026 | 0 |\n" +
				doraHints,
		},
		{
			// The metric reaches a heading rather than a cell, and a pipe in a
			// heading is text: the escaper for that slot neutralizes a leading
			// '#', a line break, a tag and a link, which are what could add
			// structure there.
			name: "a metric name carrying a pipe adds no structure",
			output: Output{
				Metrics: []MetricOutput{{Date: "2026-01-01", Value: 1.0}},
			},
			metric: "metric|with|pipes",
			want: "## DORA Metrics: metric|with|pipes (1)\n\n" +
				doraTableHead +
				"| 1 Jan 2026 | 1 |\n" +
				doraHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if md := FormatMarkdown(tt.output, tt.metric); md != tt.want {
				t.Errorf("dora metrics:\n got %q\nwant %q", md, tt.want)
			}
		})
	}
}

// TestActionSpecs_Metadata asserts that the package publishes exactly the two
// scopes GitLab serves DORA metrics at, and that each spec carries the
// metadata every surface reads off it: the owning package, an individual tool
// name, the premium edition that gates it, and the prose a model is served:
// a usage line, a related action and a description. Holding all three prose
// fields non-empty is what stops a scope added without its case in the switch
// from shipping a tool that explains nothing.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		t.Run(spec.Name, func(t *testing.T) {
			if spec.OwnerPackage != "dorametrics" {
				t.Errorf("owner package = %q, want dorametrics", spec.OwnerPackage)
			}
			if spec.IndividualTool.Name == "" {
				t.Error("individual tool name is empty")
			}
			if spec.Edition != "premium" {
				t.Errorf("edition = %q, want premium", spec.Edition)
			}
			if spec.Usage == "" {
				t.Error("usage is empty")
			}
			if len(spec.RelatedActions) == 0 {
				t.Error("related actions are empty")
			}
			if spec.IndividualTool.Description == "" {
				t.Error("individual tool description is empty")
			}
		})
	}
}
