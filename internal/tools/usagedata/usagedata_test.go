// usagedata_test.go contains unit tests for the usage data MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package usagedata

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const fmtUnexpPath = "unexpected path: %s"

const errExpectedNil = "expected error, got nil"

const fmtUnexpErr = "unexpected error: %v"

// TestGetServicePing verifies the behavior of get service ping.
func TestGetServicePing(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/usage_data/service_ping" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
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

// TestGetServicePing_NilRecordedAt verifies that GetServicePing handles the nil recorded at scenario correctly.
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

// TestGetServicePing_Error verifies that GetServicePing handles the error scenario correctly.
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

// TestGetNonSQLMetrics verifies the behavior of get non s q l metrics.
func TestGetNonSQLMetrics(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/usage_data/non_sql_metrics" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
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

// TestGetNonSQLMetrics_Error verifies that GetNonSQLMetrics handles the error scenario correctly.
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

// TestGetQueries verifies the behavior of get queries.
func TestGetQueries(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/usage_data/queries" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
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

// TestGetMetricDefinitions verifies the behavior of get metric definitions.
func TestGetMetricDefinitions(t *testing.T) {
	yamlContent := "---\nmetrics:\n  - name: users_count\n    description: Total users\n"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/usage_data/metric_definitions" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
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
}

// TestGetMetricDefinitions_Error verifies that GetMetricDefinitions handles the error scenario correctly.
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

// TestTrackEvent verifies the behavior of track event.
func TestTrackEvent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/usage_data/track_event" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, handler)
	boolTrue := true
	nsID := int64(1)
	projID := int64(2)
	out, err := TrackEvent(t.Context(), client, TrackEventInput{
		Event:          "test_event",
		SendToSnowplow: &boolTrue,
		NamespaceID:    &nsID,
		ProjectID:      &projID,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "accepted" {
		t.Errorf("Status = %q, want accepted", out.Status)
	}
}

// TestTrackEvent_Error verifies that TrackEvent handles the error scenario correctly.
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

// TestTrackEvents verifies the behavior of track events.
func TestTrackEvents(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/usage_data/track_events" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := TrackEvents(t.Context(), client, TrackEventsInput{
		Events: []TrackEventInput{
			{Event: "event_1"},
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

// TestFormatServicePingMarkdown verifies the behavior of format service ping markdown.
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

// TestFormatNonSQLMetricsMarkdown verifies the behavior of format non s q l metrics markdown.
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

// TestFormatMetricDefinitionsMarkdown verifies the behavior of format metric definitions markdown.
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

// TestFormatMetricDefinitionsMarkdown_Truncation verifies that FormatMetricDefinitionsMarkdown handles the truncation scenario correctly.
func TestFormatMetricDefinitionsMarkdown_Truncation(t *testing.T) {
	longYAML := strings.Repeat("a", 15000)
	out := MetricDefinitionsOutput{YAML: longYAML}
	md := FormatMetricDefinitionsMarkdown(out)
	if !strings.Contains(md, "truncated") {
		t.Error("expected truncation notice")
	}
}

// TestFormatTrackEventMarkdown verifies the behavior of format track event markdown.
func TestFormatTrackEventMarkdown(t *testing.T) {
	md := FormatTrackEventMarkdown(TrackEventOutput{Status: "accepted"})
	if !strings.Contains(md, "accepted") {
		t.Error("missing status")
	}
}

// TestFormatTrackEventsMarkdown verifies the behavior of format track events markdown.
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

// TestGetQueries_APIError verifies the behavior of get queries a p i error.
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

// TestGetQueries_NilRecordedAt verifies the behavior of get queries nil recorded at.
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

// TestTrackEvents_APIError verifies the behavior of track events a p i error.
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

// TestFormatServicePingMarkdown_Empty verifies the behavior of format service ping markdown empty.
func TestFormatServicePingMarkdown_Empty(t *testing.T) {
	md := FormatServicePingMarkdown(GetServicePingOutput{})
	if !strings.Contains(md, "Service Ping Data") {
		t.Error("missing header")
	}
}

// ---------------------------------------------------------------------------
// Formatters — queries with many counts
// ---------------------------------------------------------------------------.

// TestFormatQueriesMarkdown verifies the behavior of format queries markdown.
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

// TestFormatServicePingMarkdown_ManyCounts verifies the behavior of format service ping markdown many counts.
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
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip for all tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newUsageDataMCPSession(t)
	ctx := context.Background()

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
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newUsageDataMCPSession is an internal helper for the usagedata package.
func newUsageDataMCPSession(t *testing.T) *mcp.ClientSession {
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
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
