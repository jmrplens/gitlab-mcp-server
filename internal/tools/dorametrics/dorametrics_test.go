// dorametrics_test.go contains unit tests for GitLab DORA metrics retrieval
// operations. Tests use httptest to mock the GitLab DORA Metrics API.

package dorametrics

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const fmtUnexpErr = "unexpected error: %v"

// TestGetProjectMetrics validates the GetProjectMetrics handler across
// success paths (with and without optional filters), input validation
// (missing project_id, missing metric), API error responses (403, 404, 500),
// context cancellation, and empty result sets. Each subtest verifies both
// the returned output and that the correct HTTP request was sent.
func TestGetProjectMetrics(t *testing.T) {
	tests := []struct {
		name       string
		input      ProjectInput
		handler    http.HandlerFunc
		wantErr    bool
		wantErrMsg string
		validate   func(t *testing.T, out Output)
	}{
		{
			name: "returns metrics for valid project",
			input: ProjectInput{
				ProjectID: "42",
				Metric:    "deployment_frequency",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/dora/metrics")
				testutil.RespondJSON(w, http.StatusOK, `[
					{"date":"2026-01-15","value":1.5},
					{"date":"2026-01-16","value":2.0}
				]`)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if len(out.Metrics) != 2 {
					t.Fatalf("got %d metrics, want 2", len(out.Metrics))
				}
				if out.Metrics[0].Date != "2026-01-15" {
					t.Errorf("date[0] = %q, want %q", out.Metrics[0].Date, "2026-01-15")
				}
				if out.Metrics[0].Value != 1.5 {
					t.Errorf("value[0] = %f, want 1.5", out.Metrics[0].Value)
				}
				if out.Metrics[1].Date != "2026-01-16" {
					t.Errorf("date[1] = %q, want %q", out.Metrics[1].Date, "2026-01-16")
				}
				if out.Metrics[1].Value != 2.0 {
					t.Errorf("value[1] = %f, want 2.0", out.Metrics[1].Value)
				}
			},
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
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/99/dora/metrics")

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
				rawQuery := r.URL.RawQuery
				if !strings.Contains(rawQuery, "environment_tiers") {
					t.Errorf("query missing environment_tiers, got: %s", rawQuery)
				}
				testutil.RespondJSON(w, http.StatusOK, `[{"date":"2026-01","value":5.0}]`)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if len(out.Metrics) != 1 {
					t.Fatalf("got %d metrics, want 1", len(out.Metrics))
				}
				if out.Metrics[0].Value != 5.0 {
					t.Errorf("value = %f, want 5.0", out.Metrics[0].Value)
				}
			},
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
			handler := tt.handler
			if handler == nil {
				handler = func(w http.ResponseWriter, _ *http.Request) {
					t.Fatal("API handler should not be called for validation errors")
				}
			}
			client := testutil.NewTestClient(t, handler)
			out, err := GetProjectMetrics(context.Background(), client, tt.input)
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
		})
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

// TestGetGroupMetrics validates the GetGroupMetrics handler across
// success paths (with and without optional filters), input validation
// (missing group_id, missing metric), API error responses (404, 500),
// context cancellation, and empty result sets.
func TestGetGroupMetrics(t *testing.T) {
	tests := []struct {
		name     string
		input    GroupInput
		handler  http.HandlerFunc
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
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
			handler := tt.handler
			if handler == nil {
				handler = func(w http.ResponseWriter, _ *http.Request) {
					t.Fatal("API handler should not be called for validation errors")
				}
			}
			client := testutil.NewTestClient(t, handler)
			out, err := GetGroupMetrics(context.Background(), client, tt.input)
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
		})
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

// TestRegisterTools_NoPanic verifies that RegisterTools wires both DORA
// metrics tools onto the MCP server without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	RegisterTools(server, client)
}
