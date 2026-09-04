// usage_data_test.go contains unit tests for the usage data MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package usagedata

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestGetServicePing verifies GetServicePing.
func TestGetServicePing(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/usage_data/service_ping")
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.RespondJSON(w, http.StatusOK, `{
			"recorded_at": "2026-01-15T10:00:00Z",
			"license": {"plan": "premium"},
			"counts": {"users": 100, "projects": 50}
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetServicePing(t.Context(), client, GetServicePingInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.RecordedAt != "2026-01-15T10:00:00Z" {
		t.Errorf("RecordedAt = %q, want %q", out.RecordedAt, "2026-01-15T10:00:00Z")
	}
	if out.License["plan"] != "premium" {
		t.Errorf("License[plan] = %q, want premium", out.License["plan"])
	}
	if out.Counts["users"] != 100 {
		t.Errorf("Counts[users] = %d, want 100", out.Counts["users"])
	}
	if out.Counts["projects"] != 50 {
		t.Errorf("Counts[projects] = %d, want 50", out.Counts["projects"])
	}
}

// TestGetServicePing_NilRecordedAt verifies GetServicePing when nil recorded at.
func TestGetServicePing_NilRecordedAt(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"license": {}, "counts": {}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetServicePing(t.Context(), client, GetServicePingInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.RecordedAt != "" {
		t.Errorf("RecordedAt = %q, want empty", out.RecordedAt)
	}
}

// TestGetServicePing_Error verifies GetServicePing when error.
func TestGetServicePing_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := GetServicePing(t.Context(), client, GetServicePingInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetNonSQLMetrics verifies GetNonSQLMetrics.
func TestGetNonSQLMetrics(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/usage_data/non_sql_metrics")
		testutil.RespondJSON(w, http.StatusOK, `{
			"recorded_at": "2026-01-15",
			"uuid": "abc-123",
			"hostname": "gitlab.example.com",
			"version": "16.8.0",
			"installation_type": "omnibus",
			"active_user_count": 150,
			"edition": "EE",
			"license_md5": "md5hash",
			"license_sha256": "sha256hash",
			"license_id": "lic-1",
			"historical_max_users": 200,
			"licensee": {"name": "ACME"},
			"license_user_count": 300,
			"license_starts_at": "2026-01-01",
			"license_expires_at": "2026-01-01",
			"license_plan": "premium",
			"license_add_ons": {"code_suggestions": 50},
			"license_trial": "false",
			"license_subscription_id": "sub-1",
			"license": {"plan": "premium"},
			"settings": {"signup_enabled": "true"}
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetNonSQLMetrics(t.Context(), client, GetNonSQLMetricsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.UUID != "abc-123" {
		t.Errorf("UUID = %q, want abc-123", out.UUID)
	}
	if out.Hostname != "gitlab.example.com" {
		t.Errorf("Hostname = %q, want gitlab.example.com", out.Hostname)
	}
	if out.Version != "16.8.0" {
		t.Errorf("Version = %q, want 16.8.0", out.Version)
	}
	if out.ActiveUserCount != 150 {
		t.Errorf("ActiveUserCount = %d, want 150", out.ActiveUserCount)
	}
	if out.Edition != "EE" {
		t.Errorf("Edition = %q, want EE", out.Edition)
	}
	if out.LicensePlan != "premium" {
		t.Errorf("LicensePlan = %q, want premium", out.LicensePlan)
	}
}

// TestGetNonSQLMetrics_Error verifies GetNonSQLMetrics when error.
func TestGetNonSQLMetrics_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := GetNonSQLMetrics(t.Context(), client, GetNonSQLMetricsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetNonSQLMetrics_NotFound_HintsAlternatives verifies that the GitLab 19
// 404 (the endpoint is gone even with the usage_data_non_sql_metrics feature
// flag) is wrapped with a hint pointing at the working alternatives.
func TestGetNonSQLMetrics_NotFound_HintsAlternatives(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"error":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := GetNonSQLMetrics(t.Context(), client, GetNonSQLMetricsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "unavailable on GitLab 19") ||
		!strings.Contains(err.Error(), "gitlab_get_metric_definitions") {
		t.Errorf("error = %q, want GitLab 19 unavailability hint with alternatives", err.Error())
	}
}

// TestGetQueries verifies GetQueries.
func TestGetQueries(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/usage_data/queries")
		testutil.RespondJSON(w, http.StatusOK, `{
			"recorded_at": "2026-01-15T10:00:00Z",
			"uuid": "abc-123",
			"hostname": "gitlab.example.com",
			"version": "16.8.0",
			"installation_type": "omnibus",
			"active_user_count": "SELECT COUNT(*) FROM users WHERE state='active'",
			"edition": "EE",
			"license_md5": "",
			"license_sha256": "",
			"license_id": "",
			"historical_max_users": 0,
			"licensee": {},
			"license_user_count": 0,
			"license_starts_at": "",
			"license_expires_at": "",
			"license_plan": "",
			"license_add_ons": {},
			"license_trial": "",
			"license_subscription_id": "",
			"license": {},
			"settings": {},
			"counts": {"users_count": "SELECT COUNT(*) FROM users"}
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetQueries(t.Context(), client, GetQueriesInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.RecordedAt != "2026-01-15T10:00:00Z" {
		t.Errorf("RecordedAt = %q, want 2026-01-15T10:00:00Z", out.RecordedAt)
	}
	if out.Counts["users_count"] != "SELECT COUNT(*) FROM users" {
		t.Errorf("Counts[users_count] = %q, want SQL query", out.Counts["users_count"])
	}
}

// TestGetMetricDefinitions verifies GetMetricDefinitions.
func TestGetMetricDefinitions(t *testing.T) {
	yamlContent := "---\nmetrics:\n  - name: users_count\n    description: Total users\n"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/usage_data/metric_definitions")
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(yamlContent))
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetMetricDefinitions(t.Context(), client, GetMetricDefinitionsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.YAML != yamlContent {
		t.Errorf("YAML = %q, want %q", out.YAML, yamlContent)
	}
	if out.Truncated {
		t.Error("Truncated = true for a document well under the ceiling, want false")
	}
}

// countingReader serves a fixed number of bytes and records how many were read.
//
// Deliberately finite. An endless reader also proves the point, by hanging
// until the test binary's timeout, but it proves it by making a regression cost
// a 30-second stall and however much memory io.ReadAll manages to claim first.
// A stream a few times the ceiling fails on the byte count instead, which is
// the same evidence delivered as an assertion.
type countingReader struct {
	remaining int
	read      int
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.remaining)
	for i := range p[:n] {
		p[i] = 'y'
	}
	r.remaining -= n
	r.read += n
	return n, nil
}

// TestMetricDefinitionsOutput_StopsReadingAtTheCeiling verifies that the
// ceiling bounds the read itself rather than only the value returned.
//
// This is the assertion that separates a real ceiling from a cosmetic one.
// Slicing the buffer after reading it produces exactly the same output either
// way, so a test written against YAML and Truncated alone still passes with the
// io.LimitReader deleted. The defect being fixed is the memory the process
// holds while an instance streams at it, and the byte count is the only thing
// in the returned value that can see it.
func TestMetricDefinitionsOutput_StopsReadingAtTheCeiling(t *testing.T) {
	reader := &countingReader{remaining: 8 * maxMetricDefinitionsBytes}
	out, err := metricDefinitionsOutput(reader)
	if err != nil {
		t.Fatalf("metricDefinitionsOutput() error = %v", err)
	}
	if !out.Truncated {
		t.Error("Truncated = false for an oversized document, want true")
	}
	if len(out.YAML) != maxMetricDefinitionsBytes {
		t.Errorf("len(YAML) = %d, want %d", len(out.YAML), maxMetricDefinitionsBytes)
	}
	// io.ReadAll grows its buffer, so it asks for a little more than it keeps;
	// what matters is that the stream was not drained, not an exact count.
	if reader.read > 2*maxMetricDefinitionsBytes {
		t.Errorf("read %d bytes of an %d-byte stream, want the ceiling to stop it near %d",
			reader.read, 8*maxMetricDefinitionsBytes, maxMetricDefinitionsBytes)
	}
}

// TestMetricDefinitionsOutput_DocumentSizes_AreTruncatedAndFlagged verifies
// that a document above the ceiling comes back cut and marked, and one at or
// below it comes back whole and unmarked.
//
// The flag is half the fix: a prefix of this document reads as valid
// definitions, so a caller told nothing would treat a truncated answer as the
// complete set. The exactly-at-the-ceiling case is here because an off-by-one
// in the limit reader would flag a document that fits.
func TestMetricDefinitionsOutput_DocumentSizes_AreTruncatedAndFlagged(t *testing.T) {
	tests := []struct {
		name          string
		size          int
		wantTruncated bool
	}{
		{name: "document under the ceiling is returned whole", size: 1024},
		{name: "document exactly at the ceiling is returned whole", size: maxMetricDefinitionsBytes},
		{name: "document over the ceiling is cut and flagged", size: maxMetricDefinitionsBytes + 1, wantTruncated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := metricDefinitionsOutput(strings.NewReader(strings.Repeat("y", tt.size)))
			if err != nil {
				t.Fatalf("metricDefinitionsOutput(%d bytes) error = %v", tt.size, err)
			}
			if out.Truncated != tt.wantTruncated {
				t.Errorf("Truncated = %v, want %v", out.Truncated, tt.wantTruncated)
			}
			wantLen := min(tt.size, maxMetricDefinitionsBytes)
			if len(out.YAML) != wantLen {
				t.Errorf("len(YAML) = %d, want %d", len(out.YAML), wantLen)
			}
		})
	}
}

// TestFormatMetricDefinitionsMarkdown_Truncated verifies that a truncated
// document says so in the Markdown a model reads.
//
// The formatter already shortens a long document for display, which looks the
// same in the rendered output and means something different: that cut is
// cosmetic and the whole document is still in the response, while this one
// means the rest was never read. Without a distinct line the model cannot tell
// a complete answer from a partial one.
func TestFormatMetricDefinitionsMarkdown_Truncated(t *testing.T) {
	truncated := FormatMetricDefinitionsMarkdown(MetricDefinitionsOutput{YAML: "metrics: []", Truncated: true})
	if !strings.Contains(truncated, "Truncated") {
		t.Errorf("markdown for a truncated document = %q, want it to say so", truncated)
	}
	whole := FormatMetricDefinitionsMarkdown(MetricDefinitionsOutput{YAML: "metrics: []"})
	if strings.Contains(whole, "Truncated") {
		t.Errorf("markdown for a whole document = %q, want no truncation notice", whole)
	}
}

// TestGetMetricDefinitions_Error verifies GetMetricDefinitions when error.
func TestGetMetricDefinitions_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := GetMetricDefinitions(t.Context(), client, GetMetricDefinitionsInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestTrackEvent verifies TrackEvent.
func TestTrackEvent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/usage_data/track_event")
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, handler)
	boolTrue := true
	nsID := int64(1)
	projID := int64(2)
	out, err := TrackEvent(t.Context(), client, TrackEventInput{
		Event:                "test_event",
		SendToSnowplow:       &boolTrue,
		NamespaceID:          &nsID,
		ProjectID:            &projID,
		AdditionalProperties: map[string]string{"label": "value"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "accepted" {
		t.Errorf("Status = %q, want accepted", out.Status)
	}
}

// TestTrackEvent_Error verifies TrackEvent when error.
func TestTrackEvent_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := TrackEvent(t.Context(), client, TrackEventInput{Event: "bad_event"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestTrackEvents verifies TrackEvents.
func TestTrackEvents(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/usage_data/track_events")
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := TrackEvents(t.Context(), client, TrackEventsInput{
		Events: []TrackEventInput{
			{Event: "event_1", AdditionalProperties: map[string]string{"label": "value"}},
			{Event: "event_2"},
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "accepted" {
		t.Errorf("Status = %q, want accepted", out.Status)
	}
	if out.Count != 2 {
		t.Errorf("Count = %d, want 2", out.Count)
	}
}

// Formatter tests.

// TestFormatServicePingMarkdown verifies FormatServicePingMarkdown.
func TestFormatServicePingMarkdown(t *testing.T) {
	out := GetServicePingOutput{
		RecordedAt: "2026-01-15T10:00:00Z",
		License:    map[string]string{"plan": "premium"},
		Counts:     map[string]int64{"users": 100},
	}
	md := FormatServicePingMarkdown(out)
	if !strings.Contains(md, "Service Ping Data") {
		t.Error("missing header")
	}
	if !strings.Contains(md, "15 Jan 2026 10:00 UTC") {
		t.Error("missing recorded_at")
	}
	if !strings.Contains(md, "premium") {
		t.Error("missing license plan")
	}
	if !strings.Contains(md, "100") {
		t.Error("missing counts")
	}
}

// TestFormatNonSQLMetricsMarkdown verifies FormatNonSQLMetricsMarkdown.
func TestFormatNonSQLMetricsMarkdown(t *testing.T) {
	out := NonSQLMetricsOutput{
		UUID:     "abc-123",
		Hostname: "gitlab.example.com",
		Version:  "16.8.0",
		Edition:  "EE",
	}
	md := FormatNonSQLMetricsMarkdown(out)
	if !strings.Contains(md, "Non-SQL Metrics") {
		t.Error("missing header")
	}
	if !strings.Contains(md, "abc-123") {
		t.Error("missing UUID")
	}
}

// TestFormatMetricDefinitionsMarkdown verifies FormatMetricDefinitionsMarkdown.
func TestFormatMetricDefinitionsMarkdown(t *testing.T) {
	out := MetricDefinitionsOutput{YAML: "key: value"}
	md := FormatMetricDefinitionsMarkdown(out)
	if !strings.Contains(md, "```yaml") {
		t.Error("missing yaml code block")
	}
	if !strings.Contains(md, "key: value") {
		t.Error("missing yaml content")
	}
}

// TestFormatMetricDefinitionsMarkdown_Truncation verifies FormatMetricDefinitionsMarkdown when truncation.
func TestFormatMetricDefinitionsMarkdown_Truncation(t *testing.T) {
	longYAML := strings.Repeat("a", 15000)
	out := MetricDefinitionsOutput{YAML: longYAML}
	md := FormatMetricDefinitionsMarkdown(out)
	if !strings.Contains(md, "truncated") {
		t.Error("expected truncation notice")
	}
}

// TestFormatTrackEventMarkdown verifies FormatTrackEventMarkdown.
func TestFormatTrackEventMarkdown(t *testing.T) {
	md := FormatTrackEventMarkdown(TrackEventOutput{Status: "accepted"})
	if !strings.Contains(md, "accepted") {
		t.Error("missing status")
	}
}

// TestFormatTrackEventsMarkdown verifies FormatTrackEventsMarkdown.
func TestFormatTrackEventsMarkdown(t *testing.T) {
	md := FormatTrackEventsMarkdown(TrackEventsOutput{Status: "accepted", Count: 3})
	if !strings.Contains(md, "accepted") {
		t.Error("missing status")
	}
	if !strings.Contains(md, "3") {
		t.Error("missing count")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// GetQueries — API error
// ---------------------------------------------------------------------------.

// TestGetQueries_APIError verifies GetQueries when API error.
func TestGetQueries_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := GetQueries(context.Background(), client, GetQueriesInput{})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetQueries — nil recorded_at
// ---------------------------------------------------------------------------.

// TestGetQueries_NilRecordedAt verifies GetQueries when nil recorded at.
func TestGetQueries_NilRecordedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"uuid":"abc","hostname":"h","version":"1","installation_type":"omnibus","active_user_count":"","edition":"CE","license_md5":"","license_sha256":"","license_id":"","historical_max_users":0,"licensee":{},"license_user_count":0,"license_starts_at":"","license_expires_at":"","license_plan":"","license_add_ons":{},"license_trial":"","license_subscription_id":"","license":{},"settings":{},"counts":{}}`)
	}))
	out, err := GetQueries(context.Background(), client, GetQueriesInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.RecordedAt != "" {
		t.Errorf("RecordedAt = %q, want empty", out.RecordedAt)
	}
}

// ---------------------------------------------------------------------------
// TrackEvents — API error
// ---------------------------------------------------------------------------.

// TestTrackEvents_APIError verifies TrackEvents when API error.
func TestTrackEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := TrackEvents(context.Background(), client, TrackEventsInput{
		Events: []TrackEventInput{{Event: "bad"}},
	})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Formatters — empty service ping
// ---------------------------------------------------------------------------.

// TestFormatServicePingMarkdown_Empty verifies FormatServicePingMarkdown when empty.
func TestFormatServicePingMarkdown_Empty(t *testing.T) {
	md := FormatServicePingMarkdown(GetServicePingOutput{})
	if !strings.Contains(md, "Service Ping Data") {
		t.Error("missing header")
	}
}

// ---------------------------------------------------------------------------
// Formatters — queries with many counts
// ---------------------------------------------------------------------------.

// TestFormatQueriesMarkdown verifies FormatQueriesMarkdown.
func TestFormatQueriesMarkdown(t *testing.T) {
	counts := make(map[string]string)
	for i := range 25 {
		counts["metric_"+string(rune('a'+i))] = "SELECT 1"
	}
	md := FormatQueriesMarkdown(QueriesOutput{
		Version: "16.8.0",
		Edition: "EE",
		Counts:  counts,
	})
	if !strings.Contains(md, "more queries") {
		t.Error("expected truncation notice for >20 queries")
	}
}

// ---------------------------------------------------------------------------
// Formatters — service ping with many counts
// ---------------------------------------------------------------------------.

// TestFormatServicePingMarkdown_ManyCounts verifies FormatServicePingMarkdown when many counts.
func TestFormatServicePingMarkdown_ManyCounts(t *testing.T) {
	counts := make(map[string]int64)
	for i := range 25 {
		counts["metric_"+string(rune('a'+i))] = int64(i)
	}
	md := FormatServicePingMarkdown(GetServicePingOutput{
		RecordedAt: "2026-01-15T10:00:00Z",
		Counts:     counts,
	})
	if !strings.Contains(md, "more metrics") {
		t.Error("expected truncation notice for >20 counts")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies usage data action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	specByTool := usageDataSpecsByTool(specs)
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "usagedata" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if specByTool["gitlab_track_event"].ParameterGuidance["event"].SemanticRole == "" {
		t.Fatal("gitlab_track_event should define event parameter guidance")
	}
	if specByTool["gitlab_track_events"].ParameterGuidance["events"].SemanticRole == "" {
		t.Fatal("gitlab_track_events should define events parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates usage data canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newUsageDataRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"service_ping", "gitlab_get_service_ping", map[string]any{}},
		{"non_sql_metrics", "gitlab_get_non_sql_metrics", map[string]any{}},
		{"usage_queries", "gitlab_get_usage_queries", map[string]any{}},
		{"metric_definitions", "gitlab_get_metric_definitions", map[string]any{}},
		{"track_event", "gitlab_track_event", map[string]any{"event": "test_event"}},
		{"track_events", "gitlab_track_events", map[string]any{"events": []any{map[string]any{"event": "e1"}}}},
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

// TestActionSpecs_CallRouteErrors validates usage data route error paths.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	specByTool := usageDataSpecsByTool(ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_service_ping", map[string]any{}},
		{"gitlab_get_non_sql_metrics", map[string]any{}},
		{"gitlab_get_usage_queries", map[string]any{}},
		{"gitlab_track_event", map[string]any{"event": "test_event"}},
		{"gitlab_track_events", map[string]any{"events": []any{map[string]any{"event": "e1"}}}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			if _, err := spec.Route.Handler(t.Context(), tt.args); err == nil {
				t.Fatalf("expected route error for %s", tt.name)
			}
		})
	}
}

// TestGetMetricDefinitions_ReadError covers the io.ReadAll error path
// when the response body cannot be read due to truncated Content-Length.
func TestGetMetricDefinitions_ReadError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/usage_data/metric_definitions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		// Write no data — mismatch with Content-Length triggers read error.
	})
	client := testutil.NewTestClient(t, mux)
	ctx := context.Background()
	_, err := GetMetricDefinitions(ctx, client, GetMetricDefinitionsInput{})
	if err == nil {
		t.Fatal("expected error from truncated response body")
	}
}

// newUsageDataRouteSpecs constructs usage data route specs test fixtures.
func newUsageDataRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/usage_data/service_ping", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"recorded_at":"2026-01-15T10:00:00Z","license":{"plan":"premium"},"counts":{"users":100}}`)
	})

	handler.HandleFunc("GET /api/v4/usage_data/non_sql_metrics", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"recorded_at":"2026-01-15","uuid":"abc-123","hostname":"h","version":"16.8.0","installation_type":"omnibus","active_user_count":150,"edition":"EE","license_md5":"","license_sha256":"","license_id":"","historical_max_users":200,"licensee":{},"license_user_count":300,"license_starts_at":"","license_expires_at":"","license_plan":"premium","license_add_ons":{},"license_trial":"","license_subscription_id":"","license":{},"settings":{}}`)
	})

	handler.HandleFunc("GET /api/v4/usage_data/queries", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"recorded_at":"2026-01-15T10:00:00Z","uuid":"abc","hostname":"h","version":"16.8.0","installation_type":"omnibus","active_user_count":"SELECT 1","edition":"CE","license_md5":"","license_sha256":"","license_id":"","historical_max_users":0,"licensee":{},"license_user_count":0,"license_starts_at":"","license_expires_at":"","license_plan":"","license_add_ons":{},"license_trial":"","license_subscription_id":"","license":{},"settings":{},"counts":{"users":"SELECT COUNT(*) FROM users"}}`)
	})

	handler.HandleFunc("GET /api/v4/usage_data/metric_definitions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("---\nmetrics:\n  - name: test\n"))
	})

	handler.HandleFunc("POST /api/v4/usage_data/track_event", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})

	handler.HandleFunc("POST /api/v4/usage_data/track_events", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})

	client := testutil.NewTestClient(t, handler)
	return usageDataSpecsByTool(ActionSpecs(client))
}

// usageDataSpecsByTool supports usage data specs by tool assertions in usagedata tests.
func usageDataSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
