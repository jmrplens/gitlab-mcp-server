// sidekiq_test.go contains unit tests for the Sidekiq MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package sidekiq

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const errExpectedNil = "expected error, got nil"

const fmtUnexpErr = "unexpected error: %v"

const queueMetricsJSON = `{
	"queues": {
		"default": {"backlog": 10, "latency": 5},
		"mailers": {"backlog": 2, "latency": 1}
	}
}`

const processMetricsJSON = `{
	"processes": [
		{
			"hostname": "worker-01",
			"pid": 1234,
			"tag": "default",
			"started_at": "2026-01-15T10:00:00Z",
			"queues": ["default", "mailers"],
			"labels": ["reliable"],
			"concurrency": 25,
			"busy": 10
		}
	]
}`

const jobStatsJSON = `{
	"jobs": {
		"processed": 100000,
		"failed": 50,
		"enqueued": 25
	}
}`

const compoundMetricsJSON = `{
	"queues": {
		"default": {"backlog": 10, "latency": 5}
	},
	"processes": [
		{
			"hostname": "worker-01",
			"pid": 1234,
			"tag": "default",
			"started_at": "2026-01-15T10:00:00Z",
			"queues": ["default"],
			"labels": [],
			"concurrency": 25,
			"busy": 10
		}
	],
	"jobs": {
		"processed": 100000,
		"failed": 50,
		"enqueued": 25
	}
}`

// TestGetQueueMetrics_Success verifies that GetQueueMetrics handles the success scenario correctly.
func TestGetQueueMetrics_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4//sidekiq/queue_metrics" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, queueMetricsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetQueueMetrics(t.Context(), client, GetQueueMetricsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Queues) != 2 {
		t.Fatalf("expected 2 queues, got %d", len(out.Queues))
	}
}

// TestGetQueueMetrics_Error verifies that GetQueueMetrics handles the error scenario correctly.
func TestGetQueueMetrics_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetQueueMetrics(t.Context(), client, GetQueueMetricsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetProcessMetrics_Success verifies that GetProcessMetrics handles the success scenario correctly.
func TestGetProcessMetrics_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4//sidekiq/process_metrics" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, processMetricsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetProcessMetrics(t.Context(), client, GetProcessMetricsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Processes) != 1 {
		t.Fatalf("expected 1 process, got %d", len(out.Processes))
	}
	if out.Processes[0].Hostname != "worker-01" {
		t.Fatalf("expected hostname worker-01, got %s", out.Processes[0].Hostname)
	}
	if out.Processes[0].Concurrency != 25 {
		t.Fatalf("expected concurrency 25, got %d", out.Processes[0].Concurrency)
	}
}

// TestGetProcessMetrics_Error verifies that GetProcessMetrics handles the error scenario correctly.
func TestGetProcessMetrics_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetProcessMetrics(t.Context(), client, GetProcessMetricsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetJobStats_Success verifies that GetJobStats handles the success scenario correctly.
func TestGetJobStats_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4//sidekiq/job_stats" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, jobStatsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetJobStats(t.Context(), client, GetJobStatsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Jobs.Processed != 100000 {
		t.Fatalf("expected processed 100000, got %d", out.Jobs.Processed)
	}
	if out.Jobs.Failed != 50 {
		t.Fatalf("expected failed 50, got %d", out.Jobs.Failed)
	}
	if out.Jobs.Enqueued != 25 {
		t.Fatalf("expected enqueued 25, got %d", out.Jobs.Enqueued)
	}
}

// TestGetJobStats_Error verifies that GetJobStats handles the error scenario correctly.
func TestGetJobStats_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetJobStats(t.Context(), client, GetJobStatsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetCompoundMetrics_Success verifies that GetCompoundMetrics handles the success scenario correctly.
func TestGetCompoundMetrics_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4//sidekiq/compound_metrics" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, compoundMetricsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetCompoundMetrics(t.Context(), client, GetCompoundMetricsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Queues) != 1 {
		t.Fatalf("expected 1 queue, got %d", len(out.Queues))
	}
	if len(out.Processes) != 1 {
		t.Fatalf("expected 1 process, got %d", len(out.Processes))
	}
	if out.Jobs.Processed != 100000 {
		t.Fatalf("expected processed 100000, got %d", out.Jobs.Processed)
	}
}

// TestGetCompoundMetrics_Error verifies that GetCompoundMetrics handles the error scenario correctly.
func TestGetCompoundMetrics_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetCompoundMetrics(t.Context(), client, GetCompoundMetricsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestFormatQueueMetricsMarkdown verifies the behavior of format queue metrics markdown.
func TestFormatQueueMetricsMarkdown(t *testing.T) {
	out := GetQueueMetricsOutput{
		Queues: []QueueItem{
			{Name: "default", Backlog: 10, Latency: 5},
			{Name: "mailers", Backlog: 2, Latency: 1},
		},
	}
	md := FormatQueueMetricsMarkdown(out)
	if !strings.Contains(md, "default") {
		t.Fatal("expected 'default' queue in markdown")
	}
	if !strings.Contains(md, "mailers") {
		t.Fatal("expected 'mailers' queue in markdown")
	}
}

// TestFormatProcessMetricsMarkdown verifies the behavior of format process metrics markdown.
func TestFormatProcessMetricsMarkdown(t *testing.T) {
	out := GetProcessMetricsOutput{
		Processes: []ProcessItem{
			{Hostname: "worker-01", Pid: 1234, Tag: "default", Concurrency: 25, Busy: 10},
		},
	}
	md := FormatProcessMetricsMarkdown(out)
	if !strings.Contains(md, "worker-01") {
		t.Fatal("expected 'worker-01' in markdown")
	}
}

// TestFormatJobStatsMarkdown verifies the behavior of format job stats markdown.
func TestFormatJobStatsMarkdown(t *testing.T) {
	out := GetJobStatsOutput{
		Jobs: JobStatsItem{Processed: 100000, Failed: 50, Enqueued: 25},
	}
	md := FormatJobStatsMarkdown(out)
	if !strings.Contains(md, "100000") {
		t.Fatal("expected '100000' in markdown")
	}
}

// TestFormatCompoundMetricsMarkdown verifies the behavior of format compound metrics markdown.
func TestFormatCompoundMetricsMarkdown(t *testing.T) {
	out := GetCompoundMetricsOutput{
		Queues:    []QueueItem{{Name: "default", Backlog: 10, Latency: 5}},
		Processes: []ProcessItem{{Hostname: "worker-01", Pid: 1234}},
		Jobs:      JobStatsItem{Processed: 100000, Failed: 50, Enqueued: 25},
	}
	md := FormatCompoundMetricsMarkdown(out)
	if !strings.Contains(md, "Compound") {
		t.Fatal("expected 'Compound' in markdown")
	}
	if !strings.Contains(md, "default") {
		t.Fatal("expected 'default' queue in markdown")
	}
	if !strings.Contains(md, "worker-01") {
		t.Fatal("expected 'worker-01' in markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Formatters — empty states
// ---------------------------------------------------------------------------.

// TestFormatQueueMetricsMarkdown_Empty verifies the behavior of format queue metrics markdown empty.
func TestFormatQueueMetricsMarkdown_Empty(t *testing.T) {
	md := FormatQueueMetricsMarkdown(GetQueueMetricsOutput{})
	if !strings.Contains(md, "No queues found") {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatProcessMetricsMarkdown_Empty verifies the behavior of format process metrics markdown empty.
func TestFormatProcessMetricsMarkdown_Empty(t *testing.T) {
	md := FormatProcessMetricsMarkdown(GetProcessMetricsOutput{})
	if !strings.Contains(md, "No processes found") {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatCompoundMetricsMarkdown_Empty verifies the behavior of format compound metrics markdown empty.
func TestFormatCompoundMetricsMarkdown_Empty(t *testing.T) {
	md := FormatCompoundMetricsMarkdown(GetCompoundMetricsOutput{})
	if !strings.Contains(md, "No queues found") {
		t.Error("expected empty queues message")
	}
	if !strings.Contains(md, "No processes found") {
		t.Error("expected empty processes message")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies Sidekiq action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "sidekiq" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates Sidekiq canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newSidekiqRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"queue_metrics", "gitlab_get_sidekiq_queue_metrics", map[string]any{}},
		{"process_metrics", "gitlab_get_sidekiq_process_metrics", map[string]any{}},
		{"job_stats", "gitlab_get_sidekiq_job_stats", map[string]any{}},
		{"compound_metrics", "gitlab_get_sidekiq_compound_metrics", map[string]any{}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: route specs factory
// ---------------------------------------------------------------------------.

func newSidekiqRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		switch r.URL.Path {
		case "/api/v4/sidekiq/queue_metrics", "/api/v4//sidekiq/queue_metrics":
			testutil.RespondJSON(w, http.StatusOK, `{"queues":{"default":{"backlog":10,"latency":5}}}`)
		case "/api/v4/sidekiq/process_metrics", "/api/v4//sidekiq/process_metrics":
			testutil.RespondJSON(w, http.StatusOK, `{"processes":[{"hostname":"worker-01","pid":1234,"tag":"default","started_at":"2026-01-15T10:00:00Z","queues":["default"],"labels":[],"concurrency":25,"busy":10}]}`)
		case "/api/v4/sidekiq/job_stats", "/api/v4//sidekiq/job_stats":
			testutil.RespondJSON(w, http.StatusOK, `{"jobs":{"processed":100000,"failed":50,"enqueued":25}}`)
		case "/api/v4/sidekiq/compound_metrics", "/api/v4//sidekiq/compound_metrics":
			testutil.RespondJSON(w, http.StatusOK, `{"queues":{"default":{"backlog":10,"latency":5}},"processes":[{"hostname":"worker-01","pid":1234,"tag":"default","started_at":"2026-01-15T10:00:00Z","queues":["default"],"labels":[],"concurrency":25,"busy":10}],"jobs":{"processed":100000,"failed":50,"enqueued":25}}`)
		default:
			http.NotFound(w, r)
		}
	})

	client := testutil.NewTestClient(t, handler)
	return sidekiqSpecsByTool(ActionSpecs(client))
}

func sidekiqSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
