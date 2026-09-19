// sidekiq_test.go contains unit tests for the Sidekiq MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package sidekiq

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// queueMetricsJSON is two queues whose four counters are all different, so
// backlog and latency cannot trade places unnoticed.
const queueMetricsJSON = `{
	"queues": {
		"default": {"backlog": 10, "latency": 5},
		"mailers": {"backlog": 2, "latency": 1}
	}
}`

// processMetricsJSON is one Sidekiq process in which no two values agree, so a
// field read from a neighbour's key cannot pass unnoticed.
const processMetricsJSON = `{
	"processes": [
		{
			"hostname": "sidekiq-worker-07",
			"pid": 4242,
			"tag": "mailers-only",
			"started_at": "2026-01-15T10:00:00Z",
			"queues": ["default", "mailers"],
			"labels": ["reliable", "arm64"],
			"concurrency": 25,
			"busy": 11
		}
	]
}`

// processNoStartJSON is a process GitLab reported without a start time. The
// SDK models started_at as a pointer, so this is the one shape that reaches the
// nil branch of the conversion.
const processNoStartJSON = `{
	"processes": [
		{
			"hostname": "sidekiq-worker-11",
			"pid": 909,
			"tag": "quiet",
			"started_at": null,
			"queues": ["default"],
			"labels": ["spot"],
			"concurrency": 5,
			"busy": 0
		}
	]
}`

// jobStatsJSON identifies the job stats JSON constant used by this package.
const jobStatsJSON = `{
	"jobs": {
		"processed": 100000,
		"failed": 50,
		"enqueued": 25
	}
}`

// compoundMetricsJSON carries all three sections at once, again with no two
// values agreeing: the compound handler fills its job counters from a literal
// of its own rather than through GetJobStats, so a key read from the wrong
// field here is a separate defect from the same mistake in the job stats call.
const compoundMetricsJSON = `{
	"queues": {
		"default": {"backlog": 12, "latency": 3}
	},
	"processes": [
		{
			"hostname": "sidekiq-worker-07",
			"pid": 4242,
			"tag": "mailers-only",
			"started_at": "2026-01-15T10:00:00Z",
			"queues": ["default"],
			"labels": ["reliable"],
			"concurrency": 30,
			"busy": 7
		}
	],
	"jobs": {
		"processed": 100000,
		"failed": 50,
		"enqueued": 25
	}
}`

// TestGetQueueMetrics_Success verifies that each queue keeps its own name,
// backlog and latency. GitLab sends the queues as an object keyed by name, so
// the conversion has to carry the key onto the item; and backlog and latency
// are two counters of one shape, which is exactly the pair a reader can swap
// without the response looking wrong.
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

	want := map[string]QueueItem{
		"default": {Name: "default", Backlog: 10, Latency: 5},
		"mailers": {Name: "mailers", Backlog: 2, Latency: 1},
	}
	got := make(map[string]QueueItem, len(out.Queues))
	for _, q := range out.Queues {
		got[q.Name] = q
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetQueueMetrics() queues = %#v, want %#v", got, want)
	}
}

// TestGetQueueMetrics_Error verifies GetQueueMetrics when error.
func TestGetQueueMetrics_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetQueueMetrics(t.Context(), client, GetQueueMetricsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetProcessMetrics_Success verifies that every published field of a
// process is read from its own key. Half of them are pairs a reader can confuse
// without the answer looking wrong (a tag beside a hostname, the queue names
// beside the labels), and asserting one field at a time is what let that pass:
// nothing here noticed queues and labels swapping places.
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

	want := []ProcessItem{{
		Hostname:    "sidekiq-worker-07",
		Pid:         4242,
		Tag:         "mailers-only",
		StartedAt:   "2026-01-15T10:00:00Z",
		Queues:      []string{"default", "mailers"},
		Labels:      []string{"reliable", "arm64"},
		Concurrency: 25,
		Busy:        11,
	}}
	if !reflect.DeepEqual(out.Processes, want) {
		t.Errorf("GetProcessMetrics() processes = %#v, want %#v", out.Processes, want)
	}
}

// TestGetProcessMetrics_NoStartTime_LeavesStartedAtEmpty verifies that a
// process GitLab reported without a start time is published with an empty
// StartedAt and the rest of its fields intact. The SDK models started_at as a
// pointer, so the conversion has to check it before formatting: reaching for
// the value on the nil side would take the whole call down.
func TestGetProcessMetrics_NoStartTime_LeavesStartedAtEmpty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4//sidekiq/process_metrics" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, processNoStartJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetProcessMetrics(t.Context(), client, GetProcessMetricsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	want := []ProcessItem{{
		Hostname:    "sidekiq-worker-11",
		Pid:         909,
		Tag:         "quiet",
		StartedAt:   "",
		Queues:      []string{"default"},
		Labels:      []string{"spot"},
		Concurrency: 5,
		Busy:        0,
	}}
	if !reflect.DeepEqual(out.Processes, want) {
		t.Errorf("GetProcessMetrics() processes = %#v, want %#v", out.Processes, want)
	}
}

// TestGetProcessMetrics_Error verifies GetProcessMetrics when error.
func TestGetProcessMetrics_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetProcessMetrics(t.Context(), client, GetProcessMetricsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetJobStats_Success verifies GetJobStats when success.
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

// TestGetJobStats_Error verifies GetJobStats when error.
func TestGetJobStats_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetJobStats(t.Context(), client, GetJobStatsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetCompoundMetrics_Success verifies that the compound call publishes all
// three sections whole. Its job counters are assembled by a literal of its own
// rather than by calling the job stats handler, so a counter read from the
// wrong key here survives every assertion made about the standalone call; only
// Processed was ever checked, and Failed and Enqueued could trade places.
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

	wantQueues := []QueueItem{{Name: "default", Backlog: 12, Latency: 3}}
	if !reflect.DeepEqual(out.Queues, wantQueues) {
		t.Errorf("GetCompoundMetrics() queues = %#v, want %#v", out.Queues, wantQueues)
	}
	wantProcesses := []ProcessItem{{
		Hostname:    "sidekiq-worker-07",
		Pid:         4242,
		Tag:         "mailers-only",
		StartedAt:   "2026-01-15T10:00:00Z",
		Queues:      []string{"default"},
		Labels:      []string{"reliable"},
		Concurrency: 30,
		Busy:        7,
	}}
	if !reflect.DeepEqual(out.Processes, wantProcesses) {
		t.Errorf("GetCompoundMetrics() processes = %#v, want %#v", out.Processes, wantProcesses)
	}
	wantJobs := JobStatsItem{Processed: 100000, Failed: 50, Enqueued: 25}
	if out.Jobs != wantJobs {
		t.Errorf("GetCompoundMetrics() jobs = %#v, want %#v", out.Jobs, wantJobs)
	}
}

// TestGetCompoundMetrics_Error verifies GetCompoundMetrics when error.
func TestGetCompoundMetrics_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetCompoundMetrics(t.Context(), client, GetCompoundMetricsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestFormatQueueMetricsMarkdown_TwoQueues_RendersTheWholeTable verifies that a
// collection of queues renders as a table under the heading that counts them.
func TestFormatQueueMetricsMarkdown_TwoQueues_RendersTheWholeTable(t *testing.T) {
	out := GetQueueMetricsOutput{
		Queues: []QueueItem{
			{Name: "default", Backlog: 10, Latency: 5},
			{Name: "mailers", Backlog: 2, Latency: 1},
		},
	}

	want := "## Sidekiq Queue Metrics (2)\n\n" +
		"| Queue | Backlog | Latency |\n| --- | --- | --- |\n" +
		"| default | 10 | 5 |\n" +
		"| mailers | 2 | 1 |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Monitor queues with high backlog or latency for potential issues\n" +
		"- Use action 'admin.sidekiq_compound_metrics' to read the queues, processes and job counts in one call\n"

	if got := FormatQueueMetricsMarkdown(out); got != want {
		t.Errorf("FormatQueueMetricsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatProcessMetricsMarkdown_OneProcess_RendersTheStartTimeInDisplayForm
// verifies that the process table renders whole, and that the start time is
// rendered in the display form rather than as the RFC 3339 string GitLab sent.
func TestFormatProcessMetricsMarkdown_OneProcess_RendersTheStartTimeInDisplayForm(t *testing.T) {
	out := GetProcessMetricsOutput{
		Processes: []ProcessItem{{
			Hostname:    "worker-01",
			Pid:         1234,
			Tag:         "default",
			StartedAt:   "2026-03-20T15:45:00Z",
			Queues:      []string{"default", "mailers"},
			Concurrency: 25,
			Busy:        10,
		}},
	}

	want := "## Sidekiq Process Metrics (1)\n\n" +
		"| Hostname | PID | Tag | Started At | Concurrency | Busy | Queues |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| worker-01 | 1234 | default | 20 Mar 2026 15:45 UTC | 25 | 10 | default, mailers |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Check process resource usage to identify overloaded workers\n" +
		"- Use action 'admin.sidekiq_compound_metrics' to read the queues, processes and job counts in one call\n"

	if got := FormatProcessMetricsMarkdown(out); got != want {
		t.Errorf("FormatProcessMetricsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatProcessMetricsMarkdown_HostileProcess_StaysInsideItsCells verifies
// that a hostname, a tag and a queue name the instance chose cannot split the
// row or end the table: all three are free strings nothing here constrains.
func TestFormatProcessMetricsMarkdown_HostileProcess_StaysInsideItsCells(t *testing.T) {
	out := GetProcessMetricsOutput{
		Processes: []ProcessItem{{
			Hostname:  "worker|01",
			Pid:       7,
			Tag:       "tag\nwith a break",
			StartedAt: "not a timestamp|either",
			Queues:    []string{"a|b"},
		}},
	}

	want := "## Sidekiq Process Metrics (1)\n\n" +
		"| Hostname | PID | Tag | Started At | Concurrency | Busy | Queues |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| worker&#124;01 | 7 | tag with a break | not a timestamp&#124;either | 0 | 0 | a&#124;b |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Check process resource usage to identify overloaded workers\n" +
		"- Use action 'admin.sidekiq_compound_metrics' to read the queues, processes and job counts in one call\n"

	if got := FormatProcessMetricsMarkdown(out); got != want {
		t.Errorf("FormatProcessMetricsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatJobStatsMarkdown_Counters_RenderAsACard verifies that the three job
// counters render as the rows of one object rather than as a two-column table.
func TestFormatJobStatsMarkdown_Counters_RenderAsACard(t *testing.T) {
	out := GetJobStatsOutput{
		Jobs: JobStatsItem{Processed: 100000, Failed: 50, Enqueued: 25},
	}

	want := "## Sidekiq Job Statistics\n\n" +
		"- **Processed**: 100000\n" +
		"- **Failed**: 50\n" +
		"- **Enqueued**: 25\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.sidekiq_compound_metrics' to read the queues, processes and job counts in one call\n"

	if got := FormatJobStatsMarkdown(out); got != want {
		t.Errorf("FormatJobStatsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatCompoundMetricsMarkdown_EverySection_RendersWhole verifies that the
// compound result is one card with a section each, that the two collections are
// the same tables the standalone results render, and that the counters are card
// rows.
func TestFormatCompoundMetricsMarkdown_EverySection_RendersWhole(t *testing.T) {
	out := GetCompoundMetricsOutput{
		Queues: []QueueItem{{Name: "default", Backlog: 10, Latency: 5}},
		Processes: []ProcessItem{{
			Hostname:    "worker-01",
			Pid:         1234,
			Tag:         "default",
			StartedAt:   "2026-03-20T15:45:00Z",
			Queues:      []string{"default"},
			Concurrency: 25,
			Busy:        10,
		}},
		Jobs: JobStatsItem{Processed: 100000, Failed: 50, Enqueued: 25},
	}

	want := "## Sidekiq Compound Metrics\n\n" +
		"### Queues\n\n" +
		"| Queue | Backlog | Latency |\n| --- | --- | --- |\n" +
		"| default | 10 | 5 |\n\n" +
		"### Processes\n\n" +
		"| Hostname | PID | Tag | Started At | Concurrency | Busy | Queues |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| worker-01 | 1234 | default | 20 Mar 2026 15:45 UTC | 25 | 10 | default |\n\n" +
		"### Job Statistics\n\n" +
		"- **Processed**: 100000\n" +
		"- **Failed**: 50\n" +
		"- **Enqueued**: 25\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.sidekiq_queue_metrics' to read the queues on their own\n" +
		"- Use action 'admin.sidekiq_process_metrics' to read the worker processes on their own\n" +
		"- Use action 'admin.sidekiq_job_stats' to read the job counters on their own\n"

	if got := FormatCompoundMetricsMarkdown(out); got != want {
		t.Errorf("FormatCompoundMetricsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Formatters — empty states
// ---------------------------------------------------------------------------.

// TestFormatQueueMetricsMarkdown_NoQueues_IsOneSentence verifies that an empty
// collection renders the one sentence an empty list renders, heading included:
// a heading counting zero above a sentence saying so said it twice.
func TestFormatQueueMetricsMarkdown_NoQueues_IsOneSentence(t *testing.T) {
	want := "No Sidekiq queues found.\n"
	if got := FormatQueueMetricsMarkdown(GetQueueMetricsOutput{}); got != want {
		t.Errorf("FormatQueueMetricsMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatProcessMetricsMarkdown_NoProcesses_IsOneSentence verifies the same
// for an instance running no Sidekiq process.
func TestFormatProcessMetricsMarkdown_NoProcesses_IsOneSentence(t *testing.T) {
	want := "No Sidekiq processes found.\n"
	if got := FormatProcessMetricsMarkdown(GetProcessMetricsOutput{}); got != want {
		t.Errorf("FormatProcessMetricsMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatCompoundMetricsMarkdown_NothingRunning_KeepsEverySection verifies
// that the compound card keeps its three sections when two of them are empty,
// each saying so, and that the counters still render: zero processed jobs is an
// answer.
func TestFormatCompoundMetricsMarkdown_NothingRunning_KeepsEverySection(t *testing.T) {
	want := "## Sidekiq Compound Metrics\n\n" +
		"### Queues\n\n" +
		"No Sidekiq queues found.\n\n" +
		"### Processes\n\n" +
		"No Sidekiq processes found.\n\n" +
		"### Job Statistics\n\n" +
		"- **Processed**: 0\n" +
		"- **Failed**: 0\n" +
		"- **Enqueued**: 0\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.sidekiq_queue_metrics' to read the queues on their own\n" +
		"- Use action 'admin.sidekiq_process_metrics' to read the worker processes on their own\n" +
		"- Use action 'admin.sidekiq_job_stats' to read the job counters on their own\n"

	if got := FormatCompoundMetricsMarkdown(GetCompoundMetricsOutput{}); got != want {
		t.Errorf("FormatCompoundMetricsMarkdown() =\n%q\nwant\n%q", got, want)
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
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
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

// newSidekiqRouteSpecs constructs sidekiq route specs test fixtures.
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

// sidekiqSpecsByTool supports sidekiq specs by tool assertions in sidekiq tests.
func sidekiqSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
