// usage_data_test.go contains unit tests for the usage data MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package usagedata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestGetNonSQLMetrics_EveryFieldDistinct_ReadsItsOwnValue verifies that each
// of the twenty-one fields this report publishes is read from the field of the
// same name rather than from a neighbor.
//
// No two values in the fixture agree, which is the only way to see the class:
// the two license dates used to carry the same day, so a converter assigning
// LicenseStartsAt from LicenseExpiresAt rendered identically, and an assignment
// has no branch for either gate to flip. The comparison is of the whole struct
// because asserting six fields left the other fifteen free to be crossed or
// dropped in silence.
func TestGetNonSQLMetrics_EveryFieldDistinct_ReadsItsOwnValue(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/usage_data/non_sql_metrics")
		testutil.RespondJSON(w, http.StatusOK, `{
			"recorded_at": "2026-01-15T08:30:00Z",
			"uuid": "uuid-abc-123",
			"hostname": "gitlab.example.com",
			"version": "16.8.0",
			"installation_type": "omnibus",
			"active_user_count": 150,
			"edition": "EE",
			"license_md5": "md5-of-the-license",
			"license_sha256": "sha256-of-the-license",
			"license_id": "license-7",
			"historical_max_users": 200,
			"licensee": {"name": "ACME"},
			"license_user_count": 300,
			"license_starts_at": "2026-02-01",
			"license_expires_at": "2027-03-02",
			"license_plan": "premium",
			"license_add_ons": {"code_suggestions": 50},
			"license_trial": "false",
			"license_subscription_id": "subscription-9",
			"license": {"plan": "ultimate"},
			"settings": {"signup_enabled": "true"}
		}`)
	})
	out, err := GetNonSQLMetrics(t.Context(), testutil.NewTestClient(t, handler), GetNonSQLMetricsInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	want := NonSQLMetricsOutput{
		RecordedAt:            "2026-01-15T08:30:00Z",
		UUID:                  "uuid-abc-123",
		Hostname:              "gitlab.example.com",
		Version:               "16.8.0",
		InstallationType:      "omnibus",
		ActiveUserCount:       150,
		Edition:               "EE",
		LicenseMD5:            "md5-of-the-license",
		LicenseSHA256:         "sha256-of-the-license",
		LicenseID:             "license-7",
		HistoricalMaxUsers:    200,
		Licensee:              map[string]string{"name": "ACME"},
		LicenseUserCount:      300,
		LicenseStartsAt:       "2026-02-01",
		LicenseExpiresAt:      "2027-03-02",
		LicensePlan:           "premium",
		LicenseAddOns:         map[string]int64{"code_suggestions": 50},
		LicenseTrial:          "false",
		LicenseSubscriptionID: "subscription-9",
		License:               map[string]string{"plan": "ultimate"},
		Settings:              map[string]string{"signup_enabled": "true"},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("GetNonSQLMetrics() =\n%+v\nwant\n%+v", out, want)
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
		!strings.Contains(err.Error(), "admin.usage_data_metric_definitions") {
		t.Errorf("error = %q, want GitLab 19 unavailability hint with alternatives", err.Error())
	}
}

// TestGetQueries_EveryFieldDistinct_ReadsItsOwnValue verifies that each of the
// twenty-two fields the queries report publishes is read from the field of the
// same name, and that the recording time is rendered in RFC 3339.
//
// The fixture used to send the empty string for fifteen of them, which made
// every string field interchangeable with every other while two assertions
// passed; here no two values agree.
func TestGetQueries_EveryFieldDistinct_ReadsItsOwnValue(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/usage_data/queries")
		testutil.RespondJSON(w, http.StatusOK, `{
			"recorded_at": "2026-01-15T10:00:00Z",
			"uuid": "uuid-abc-123",
			"hostname": "gitlab.example.com",
			"version": "16.8.0",
			"installation_type": "omnibus",
			"active_user_count": "SELECT COUNT(*) FROM users WHERE state='active'",
			"edition": "EE",
			"license_md5": "md5-of-the-license",
			"license_sha256": "sha256-of-the-license",
			"license_id": "license-7",
			"historical_max_users": 200,
			"licensee": {"name": "ACME"},
			"license_user_count": 300,
			"license_starts_at": "2026-02-01",
			"license_expires_at": "2027-03-02",
			"license_plan": "premium",
			"license_add_ons": {"code_suggestions": 50},
			"license_trial": "false",
			"license_subscription_id": "subscription-9",
			"license": {"plan": "ultimate"},
			"settings": {"signup_enabled": "true"},
			"counts": {"users_count": "SELECT COUNT(*) FROM users"}
		}`)
	})
	out, err := GetQueries(t.Context(), testutil.NewTestClient(t, handler), GetQueriesInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	want := QueriesOutput{
		RecordedAt:            "2026-01-15T10:00:00Z",
		UUID:                  "uuid-abc-123",
		Hostname:              "gitlab.example.com",
		Version:               "16.8.0",
		InstallationType:      "omnibus",
		ActiveUserCount:       "SELECT COUNT(*) FROM users WHERE state='active'",
		Edition:               "EE",
		LicenseMD5:            "md5-of-the-license",
		LicenseSHA256:         "sha256-of-the-license",
		LicenseID:             "license-7",
		HistoricalMaxUsers:    200,
		Licensee:              map[string]string{"name": "ACME"},
		LicenseUserCount:      300,
		LicenseStartsAt:       "2026-02-01",
		LicenseExpiresAt:      "2027-03-02",
		LicensePlan:           "premium",
		LicenseAddOns:         map[string]int64{"code_suggestions": 50},
		LicenseTrial:          "false",
		LicenseSubscriptionID: "subscription-9",
		License:               map[string]string{"plan": "ultimate"},
		Settings:              map[string]string{"signup_enabled": "true"},
		Counts:                map[string]string{"users_count": "SELECT COUNT(*) FROM users"},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("GetQueries() =\n%+v\nwant\n%+v", out, want)
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
	want := "## Metric Definitions (YAML)\n\n" +
		"- " + toolutil.EmojiWarning + " **Truncated**\n\n" +
		"```yaml\nmetrics: []\n```\n\n" +
		"The document exceeded the size this action returns, so it was cut short. Read the remainder from GitLab directly (GET /usage_data/metric_definitions).\n" +
		definitionsHints

	got := FormatMetricDefinitionsMarkdown(MetricDefinitionsOutput{YAML: "metrics: []", Truncated: true})
	if got != want {
		t.Errorf("FormatMetricDefinitionsMarkdown() =\n%q\nwant\n%q", got, want)
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

// TestTrackEvent_Error verifies that a refusal of one tracked event carries the
// hint keyed to 400, which nothing asserted: the status the hint hangs on could
// have been any other and this test would still have passed.
func TestTrackEvent_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := TrackEvent(t.Context(), client, TrackEventInput{Event: "bad_event"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	// The tail rather than the opening: the batch hint begins "each event name
	// must be a valid Snowplow event identifier", so the opening cannot tell
	// the two sentences apart and this handler wearing the batch one, which
	// talks about a ceiling a single-event call has not got, would pass.
	if !strings.Contains(err.Error(), "verify namespace_id/project_id exist if provided") {
		t.Errorf("error = %q, want the hint keyed to a bad request on a single event", err.Error())
	}
}

// captureTrackBody drives a call and returns the JSON body the handler put on
// the wire, so an assertion is about what GitLab receives rather than about
// what the input struct held.
func captureTrackBody(t *testing.T, call func(client *gitlabclient.Client)) map[string]any {
	t.Helper()
	var body map[string]any
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		if err = json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode request body %q: %v", raw, err)
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	call(client)
	return body
}

// TestTrackEvent_SendsEachFieldTheCallerSetUnderItsOwnKey verifies that one
// tracked event reaches GitLab carrying every field the caller supplied, under
// the key client-go spells for it.
//
// Nothing used to read the request at all, so the two identifiers could trade
// places and the event would be attributed to the wrong object with the suite
// green: they are the same type, and an assignment offers no branch for either
// gate to flip. They are distinct here for that reason.
func TestTrackEvent_SendsEachFieldTheCallerSetUnderItsOwnKey(t *testing.T) {
	sendToSnowplow := true
	namespaceID := int64(11)
	projectID := int64(22)

	body := captureTrackBody(t, func(client *gitlabclient.Client) {
		out, err := TrackEvent(t.Context(), client, TrackEventInput{
			Event:                "test_event",
			SendToSnowplow:       &sendToSnowplow,
			NamespaceID:          &namespaceID,
			ProjectID:            &projectID,
			AdditionalProperties: map[string]string{"label": "value"},
		})
		if err != nil {
			t.Errorf(fmtUnexpErr, err)
		}
		if out.Status != "accepted" {
			t.Errorf("Status = %q, want accepted", out.Status)
		}
	})

	want := map[string]any{
		"event":                 "test_event",
		"send_to_snowplow":      true,
		"namespace_id":          float64(11),
		"project_id":            float64(22),
		"additional_properties": map[string]any{"label": "value"},
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("request body =\n%+v\nwant\n%+v", body, want)
	}
}

// TestTrackEvents_SendsEveryEventInOrderWithItsOwnFields verifies that a batch
// reaches GitLab as the array of events the caller passed, each carrying its
// own fields, and that the count answered is the number sent.
//
// The two events differ in every field they set, so an event built from its
// neighbor's values, or a loop that sends the first event twice, fails here.
// The second carries send_to_snowplow at false to pin that a pointer to false
// is sent rather than omitted, which is what distinguishes "the caller said no"
// from "the caller said nothing".
func TestTrackEvents_SendsEveryEventInOrderWithItsOwnFields(t *testing.T) {
	sendToSnowplow := false
	namespaceID := int64(11)
	projectID := int64(22)

	body := captureTrackBody(t, func(client *gitlabclient.Client) {
		out, err := TrackEvents(t.Context(), client, TrackEventsInput{
			Events: []TrackEventInput{
				{
					Event:                "event_1",
					NamespaceID:          &namespaceID,
					ProjectID:            &projectID,
					AdditionalProperties: map[string]string{"label": "first"},
				},
				{Event: "event_2", SendToSnowplow: &sendToSnowplow},
			},
		})
		if err != nil {
			t.Errorf(fmtUnexpErr, err)
		}
		if out.Count != 2 {
			t.Errorf("Count = %d, want 2", out.Count)
		}
	})

	want := map[string]any{
		"events": []any{
			map[string]any{
				"event":                 "event_1",
				"namespace_id":          float64(11),
				"project_id":            float64(22),
				"additional_properties": map[string]any{"label": "first"},
			},
			map[string]any{
				"event":            "event_2",
				"send_to_snowplow": false,
			},
		},
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("request body =\n%+v\nwant\n%+v", body, want)
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

// servicePingHints is the guidance section the Service Ping card closes with.
const servicePingHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'admin.usage_data_non_sql_metrics' to read the non-SQL half of the same report\n" +
	"- Use action 'admin.usage_data_metric_definitions' to look a metric key up in the definitions\n"

// queriesHints is the guidance section the Service Ping queries card closes
// with.
const queriesHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'admin.usage_data_service_ping' to read the counts these queries produce\n" +
	"- Use action 'admin.usage_data_metric_definitions' to look a metric key up in the definitions\n"

// definitionsHints is the guidance section the metric definitions card closes
// with.
const definitionsHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'admin.usage_data_service_ping' to read the values these metrics are reported with\n"

// nonSQLMetricsHints is the guidance section the non-SQL metrics card closes
// with.
const nonSQLMetricsHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'admin.usage_data_service_ping' to read the full Service Ping report\n"

// TestFormatServicePingMarkdown_LicenseAndCounts_RendersTheWholeCard verifies
// that the report renders as a card whose two keyed collections are tables
// under their own headings, with the recording time in the display form.
func TestFormatServicePingMarkdown_LicenseAndCounts_RendersTheWholeCard(t *testing.T) {
	out := GetServicePingOutput{
		RecordedAt: "2026-01-15T10:00:00Z",
		License:    map[string]string{"plan": "premium"},
		Counts:     map[string]int64{"users": 100},
	}

	want := "## Service Ping Data\n\n" +
		"- **Recorded At**: 15 Jan 2026 10:00 UTC\n\n" +
		"### License\n\n" +
		"| Key | Value |\n| --- | --- |\n" +
		"| plan | premium |\n\n" +
		"### Counts\n\n" +
		"| Metric | Count |\n| --- | --- |\n" +
		"| users | 100 |\n" +
		servicePingHints

	if got := FormatServicePingMarkdown(out); got != want {
		t.Errorf("FormatServicePingMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatNonSQLMetricsMarkdown_RendersTheWholeCard verifies that the
// non-SQL half of the report renders as card rows rather than as a
// "| Property | Value |" table, and that the two counters are written at zero:
// an instance with no active user is an answer.
func TestFormatNonSQLMetricsMarkdown_RendersTheWholeCard(t *testing.T) {
	out := NonSQLMetricsOutput{
		UUID:     "abc-123",
		Hostname: "gitlab.example.com",
		Version:  "16.8.0",
		Edition:  "EE",
	}

	want := "## Non-SQL Metrics\n\n" +
		"- **UUID**: abc-123\n" +
		"- **Hostname**: gitlab.example.com\n" +
		"- **Version**: 16.8.0\n" +
		"- **Edition**: EE\n" +
		"- **Active Users**: 0\n" +
		"- **Historical Max Users**: 0\n" +
		nonSQLMetricsHints

	if got := FormatNonSQLMetricsMarkdown(out); got != want {
		t.Errorf("FormatNonSQLMetricsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatNonSQLMetricsMarkdown_EveryRowPopulated_ReadsItsOwnField verifies
// that each of the nine rows the card writes is filled from its own field.
//
// The zero case above cannot say that: it leaves both counters at 0 and three
// rows unset, so the two Int rows could trade places, and so could Installation
// Type and License Plan, and the card would render the same either way. Here no
// two values agree, which is the only thing that separates them.
func TestFormatNonSQLMetricsMarkdown_EveryRowPopulated_ReadsItsOwnField(t *testing.T) {
	out := NonSQLMetricsOutput{
		UUID:               "uuid-abc-123",
		Hostname:           "gitlab.example.com",
		Version:            "16.8.0",
		Edition:            "EE",
		InstallationType:   "omnibus",
		ActiveUserCount:    150,
		HistoricalMaxUsers: 200,
		LicensePlan:        "premium",
		RecordedAt:         "2026-01-15T10:00:00Z",
	}

	want := "## Non-SQL Metrics\n\n" +
		"- **UUID**: uuid-abc-123\n" +
		"- **Hostname**: gitlab.example.com\n" +
		"- **Version**: 16.8.0\n" +
		"- **Edition**: EE\n" +
		"- **Installation Type**: omnibus\n" +
		"- **Active Users**: 150\n" +
		"- **Historical Max Users**: 200\n" +
		"- **License Plan**: premium\n" +
		"- **Recorded At**: 15 Jan 2026 10:00 UTC\n" +
		nonSQLMetricsHints

	if got := FormatNonSQLMetricsMarkdown(out); got != want {
		t.Errorf("FormatNonSQLMetricsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatMetricDefinitionsMarkdown_ShortDocument_RendersOneFence verifies
// that a document that fits renders inside one fence with no notice of any
// kind, since neither cut happened.
func TestFormatMetricDefinitionsMarkdown_ShortDocument_RendersOneFence(t *testing.T) {
	want := "## Metric Definitions (YAML)\n\n" +
		"```yaml\nkey: value\n```\n" +
		definitionsHints

	if got := FormatMetricDefinitionsMarkdown(MetricDefinitionsOutput{YAML: "key: value"}); got != want {
		t.Errorf("FormatMetricDefinitionsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatMetricDefinitionsMarkdown_LongDocument_SaysTheCardShortenedIt
// verifies that a document longer than the card shows is cut for display and
// says so, without claiming the action stopped reading.
func TestFormatMetricDefinitionsMarkdown_LongDocument_SaysTheCardShortenedIt(t *testing.T) {
	want := "## Metric Definitions (YAML)\n\n" +
		"```yaml\n" + strings.Repeat("a", maxRenderedYAMLBytes) + "\n```\n\n" +
		"The card shows the first 10000 bytes of the document; the whole of what this action read is in the structured result.\n" +
		definitionsHints

	got := FormatMetricDefinitionsMarkdown(MetricDefinitionsOutput{YAML: strings.Repeat("a", 15000)})
	if got != want {
		t.Errorf("FormatMetricDefinitionsMarkdown() = %d bytes, want %d bytes; first difference at %d",
			len(got), len(want), firstDifference(got, want))
	}
}

// TestFormatMetricDefinitionsMarkdown_ExactlyAtTheCardsCeiling_SaysNothingWasCut
// verifies that a document of exactly the size the card shows renders whole and
// with no notice under it.
//
// The boundary is the whole difference between the two sentences a reader can
// see: one byte more and the card says it showed a prefix. A guard reading `<`
// instead of `<=` prints that sentence under a document it did not cut, and
// every other case here sits clear of the boundary on one side or the other.
func TestFormatMetricDefinitionsMarkdown_ExactlyAtTheCardsCeiling_SaysNothingWasCut(t *testing.T) {
	document := strings.Repeat("a", maxRenderedYAMLBytes)

	want := "## Metric Definitions (YAML)\n\n" +
		"```yaml\n" + document + "\n```\n" +
		definitionsHints

	got := FormatMetricDefinitionsMarkdown(MetricDefinitionsOutput{YAML: document})
	if got != want {
		t.Errorf("FormatMetricDefinitionsMarkdown() = %d bytes, want %d bytes; first difference at %d",
			len(got), len(want), firstDifference(got, want))
	}
}

// TestDisplayYAML_MultiByteDocument_CutsOnARuneBoundary verifies that the
// display cut never leaves half a character at the end of the fence.
//
// The ceiling is a byte count and lands wherever it lands: cutting at it
// directly through a multi-byte character leaves the leading bytes of one
// behind, which every client renders as a replacement glyph.
func TestDisplayYAML_MultiByteDocument_CutsOnARuneBoundary(t *testing.T) {
	// One character short of the ceiling, then a three-byte character that
	// straddles it.
	document := strings.Repeat("a", maxRenderedYAMLBytes-1) + "€" + "tail"

	got, shortened := displayYAML(document)
	if !shortened {
		t.Fatal("displayYAML() shortened = false, want true for a document over the ceiling")
	}
	if want := strings.Repeat("a", maxRenderedYAMLBytes-1); got != want {
		t.Errorf("displayYAML() kept %d bytes ending %q, want the %d bytes before the split character",
			len(got), got[max(0, len(got)-4):], len(want))
	}
	if !utf8.ValidString(got) {
		t.Error("displayYAML() left an incomplete character at the cut")
	}
}

// TestTruncateAtRuneBoundary_Tails_AreCutWhole verifies the boundary rule the
// read ceiling and the display cut share.
func TestTruncateAtRuneBoundary_Tails_AreCutWhole(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		limit int
		want  string
	}{
		{name: "nothing to cut", data: "abc", limit: 3, want: "abc"},
		// Exactly at the limit and ending in a byte that is not a character:
		// nothing was cut, so nothing may be given back. Every other
		// at-the-limit case ends in valid UTF-8, where the boundary loop is a
		// no-op, so a guard reading `<` instead of `<=` ate this tail alone.
		{name: "nothing to cut, and the tail is not UTF-8", data: "ab\xff", limit: 3, want: "ab\xff"},
		{name: "cut between ASCII characters", data: "abcd", limit: 2, want: "ab"},
		{name: "cut inside a two-byte character", data: "abé", limit: 3, want: "ab"},
		{name: "cut inside a three-byte character", data: "ab€", limit: 4, want: "ab"},
		{name: "cut after a whole character", data: "ab€c", limit: 5, want: "ab€"},
		// A document may legitimately end in an encoded U+FFFD, which
		// DecodeLastRune reports as RuneError with a size of three rather than
		// one. That is a whole character and is kept; reading the error alone
		// would eat three good bytes off every such document.
		{name: "cut after a whole replacement character", data: "ab�z", limit: 5, want: "ab�"},
		{name: "tail that is not UTF-8 at all", data: "ab\xff\xff\xff\xffz", limit: 6, want: "ab\xff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(truncateAtRuneBoundary([]byte(tt.data), tt.limit)); got != tt.want {
				t.Errorf("truncateAtRuneBoundary(%q, %d) = %q, want %q", tt.data, tt.limit, got, tt.want)
			}
		})
	}
}

// TestFormatTrackEventMarkdown_RendersTheWholeCard verifies that the answer to
// one tracked event is a card row rather than a bold run-on line.
func TestFormatTrackEventMarkdown_RendersTheWholeCard(t *testing.T) {
	want := "## Track Event\n\n" +
		"- **Status**: accepted\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.usage_data_track_events' to send a batch of events in one call\n"

	if got := FormatTrackEventMarkdown(TrackEventOutput{Status: "accepted"}); got != want {
		t.Errorf("FormatTrackEventMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatTrackEventsMarkdown_RendersTheWholeCard verifies that a batch
// answers with the status and the count as two rows.
func TestFormatTrackEventsMarkdown_RendersTheWholeCard(t *testing.T) {
	want := "## Track Events\n\n" +
		"- **Status**: accepted\n" +
		"- **Events**: 3\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.usage_data_metric_definitions' to review the metrics these events feed\n"

	if got := FormatTrackEventsMarkdown(TrackEventsOutput{Status: "accepted", Count: 3}); got != want {
		t.Errorf("FormatTrackEventsMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// firstDifference reports the index of the first byte at which a and b differ,
// or -1 when one is a prefix of the other and they differ only in length.
func firstDifference(a, b string) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	return -1
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// GetQueries: API error
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
// GetQueries: nil recorded_at
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
// TrackEvents: API error
// ---------------------------------------------------------------------------.

// TestTrackEvents_APIError verifies that a refused batch carries the hint keyed
// to 400, which names the batch ceiling rather than the single-event sentence.
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
	if !strings.Contains(err.Error(), "max batch size applies") {
		t.Errorf("error = %q, want the batch hint keyed to a bad request", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Formatters: empty service ping
// ---------------------------------------------------------------------------.

// TestFormatServicePingMarkdown_NothingReported_IsTheHeadingAndTheHints
// verifies that a report carrying nothing writes no empty table and no row with
// nothing after it.
func TestFormatServicePingMarkdown_NothingReported_IsTheHeadingAndTheHints(t *testing.T) {
	want := "## Service Ping Data\n" + servicePingHints

	if got := FormatServicePingMarkdown(GetServicePingOutput{}); got != want {
		t.Errorf("FormatServicePingMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Formatters: queries with many counts
// ---------------------------------------------------------------------------.

// TestFormatQueriesMarkdown_MoreThanTheCardShows_SaysHowManyItLeftOut verifies
// that the queries card writes its three identity rows as rows, shows the first
// twenty queries, and says how much of the map it left in the structured
// result.
func TestFormatQueriesMarkdown_MoreThanTheCardShows_SaysHowManyItLeftOut(t *testing.T) {
	counts := make(map[string]string, 25)
	for i := range 25 {
		counts["metric_"+string(rune('a'+i))] = "SELECT 1"
	}

	var rows strings.Builder
	for i := range maxRenderedMetrics {
		rows.WriteString("| metric_" + string(rune('a'+i)) + " | SELECT 1 |\n")
	}

	want := "## Service Ping Queries\n\n" +
		"- **Version**: 16.8.0\n" +
		"- **Edition**: EE\n\n" +
		"### SQL Queries\n\n" +
		"| Metric | Query |\n| --- | --- |\n" +
		rows.String() + "\n" +
		"Showing the first 20 of 25 queries; the rest are in the structured result.\n" +
		queriesHints

	got := FormatQueriesMarkdown(QueriesOutput{Version: "16.8.0", Edition: "EE", Counts: counts})
	if got != want {
		t.Errorf("FormatQueriesMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatQueriesMarkdown_HostileIdentity_StaysInsideItsRows verifies that
// the instance version, edition and recording time cannot leave their rows.
//
// All three used to share one line with no list marker and no escaping, so a
// value carrying a tag, a pipe or a line break wrote whatever it liked into the
// response. Each is a row of its own now, escaped at the write.
func TestFormatQueriesMarkdown_HostileIdentity_StaysInsideItsRows(t *testing.T) {
	out := QueriesOutput{
		Version:    "16.8.0|x",
		Edition:    `<a href="http://attacker.invalid">x</a>`,
		RecordedAt: "2026-01-15T10:00:00Z",
	}

	want := "## Service Ping Queries\n\n" +
		"- **Version**: 16.8.0&#124;x\n" +
		"- **Edition**: &lt;a href=\"http://attacker.invalid\">x&lt;/a>\n" +
		"- **Recorded At**: 15 Jan 2026 10:00 UTC\n" +
		queriesHints

	if got := FormatQueriesMarkdown(out); got != want {
		t.Errorf("FormatQueriesMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Formatters: service ping with many counts
// ---------------------------------------------------------------------------.

// TestFormatServicePingMarkdown_ManyCounts_SaysHowManyItLeftOut verifies that
// the counts table stops at the twenty rows the card shows and says how many
// metrics the response carries.
func TestFormatServicePingMarkdown_ManyCounts_SaysHowManyItLeftOut(t *testing.T) {
	counts := make(map[string]int64, 25)
	for i := range 25 {
		counts["metric_"+string(rune('a'+i))] = int64(i)
	}

	var rows strings.Builder
	for i := range maxRenderedMetrics {
		fmt.Fprintf(&rows, "| metric_%c | %d |\n", rune('a'+i), i)
	}

	want := "## Service Ping Data\n\n" +
		"- **Recorded At**: 15 Jan 2026 10:00 UTC\n\n" +
		"### Counts\n\n" +
		"| Metric | Count |\n| --- | --- |\n" +
		rows.String() + "\n" +
		"Showing the first 20 of 25 metrics; the rest are in the structured result.\n" +
		servicePingHints

	got := FormatServicePingMarkdown(GetServicePingOutput{
		RecordedAt: "2026-01-15T10:00:00Z",
		Counts:     counts,
	})
	if got != want {
		t.Errorf("FormatServicePingMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// usageDataCalls names every usage-data handler beside a call of it, so the
// two tests below can drive the same set against a working instance and a
// refusing one.
//
// forbiddenHint is the sentence that handler adds to a 403, or empty where it
// adds none: the three reads key a hint to that status and the two writes key
// theirs to 400, so the same refusal reads differently depending on which
// handler received it, and the distinction is what the hint is for.
func usageDataCalls(t *testing.T, client *gitlabclient.Client) []struct {
	name          string
	forbiddenHint string
	call          func() error
} {
	t.Helper()
	return []struct {
		name          string
		forbiddenHint string
		call          func() error
	}{
		{name: "service_ping", forbiddenHint: "only available on self-managed instances", call: func() error {
			_, err := GetServicePing(t.Context(), client, GetServicePingInput{})
			return err
		}},
		{name: "non_sql_metrics", forbiddenHint: "self-managed only; service ping must be enabled", call: func() error {
			_, err := GetNonSQLMetrics(t.Context(), client, GetNonSQLMetricsInput{})
			return err
		}},
		{name: "usage_queries", forbiddenHint: "returns the SQL queries that produce service ping counts", call: func() error {
			_, err := GetQueries(t.Context(), client, GetQueriesInput{})
			return err
		}},
		{name: "metric_definitions", forbiddenHint: "returns YAML metric definitions", call: func() error {
			_, err := GetMetricDefinitions(t.Context(), client, GetMetricDefinitionsInput{})
			return err
		}},
		{name: "track_event", call: func() error {
			_, err := TrackEvent(t.Context(), client, TrackEventInput{Event: "test_event"})
			return err
		}},
		{name: "track_events", call: func() error {
			_, err := TrackEvents(t.Context(), client, TrackEventsInput{Events: []TrackEventInput{{Event: "e1"}}})
			return err
		}},
	}
}

// TestUsageData_EachHandlerReachesItsOwnEndpoint drives every usage-data
// handler against the endpoint GitLab serves it on, so one pointed at a
// sibling's path fails rather than being answered by a catch-all.
func TestUsageData_EachHandlerReachesItsOwnEndpoint(t *testing.T) {
	client := usageDataRouteClient(t)

	for _, tt := range usageDataCalls(t, client) {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("%s error = %v, want nil", tt.name, err)
			}
		})
	}
}

// TestUsageData_RefusalsPropagate verifies that an instance refusing the read
// or the write is reported rather than swallowed, and that a read refused with
// 403 carries the hint written for that handler.
//
// The hint is checked because it is the part no gate can be wrong about: it is
// a string argument rather than a branch, so a handler wired to a sibling's
// sentence would tell an administrator to enable the wrong thing and every
// measurement here would still read clean.
func TestUsageData_RefusalsPropagate(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)

	for _, tt := range usageDataCalls(t, client) {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatalf("%s error = nil, want the instance refusal", tt.name)
			}
			if tt.forbiddenHint != "" && !strings.Contains(err.Error(), tt.forbiddenHint) {
				t.Errorf("%s error = %q, want the hint %q", tt.name, err.Error(), tt.forbiddenHint)
			}
		})
	}
}

// TestGetMetricDefinitions_ReadError covers the io.ReadAll error path when the
// response body cannot be read because of a truncated Content-Length.
//
// The error is inspected rather than merely counted: the SDK answers this
// request without touching the body, so a failure here can come from either
// layer, and the comment's claim about which one is only true if the message
// the read path wraps is what came back.
func TestGetMetricDefinitions_ReadError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/usage_data/metric_definitions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		// Write no data: mismatch with Content-Length triggers read error.
	})
	client := testutil.NewTestClient(t, mux)
	ctx := context.Background()
	_, err := GetMetricDefinitions(ctx, client, GetMetricDefinitionsInput{})
	if err == nil {
		t.Fatal("expected error from truncated response body")
	}
	if !strings.Contains(err.Error(), "reading response body") {
		t.Errorf("error = %q, want the failure the body read wraps", err.Error())
	}
}

// usageDataRouteClient returns a client answering every usage-data endpoint
// and nothing else.
func usageDataRouteClient(t *testing.T) *gitlabclient.Client {
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

	return testutil.NewTestClient(t, handler)
}
