// Package usagedata implements MCP tools for GitLab Usage Data / Service Ping API.
package usagedata

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// GetServicePing
// ---------------------------------------------------------------------------.

// GetServicePingInput is the input (no params).
type GetServicePingInput struct{}

// GetServicePingOutput is the output for getting service ping data.
type GetServicePingOutput struct {
	toolutil.HintableOutput
	RecordedAt string            `json:"recorded_at"`
	License    map[string]string `json:"license"`
	Counts     map[string]int64  `json:"counts"`
}

// GetServicePing retrieves service ping data (admin-only).
func GetServicePing(ctx context.Context, client *gitlabclient.Client, _ GetServicePingInput) (GetServicePingOutput, error) {
	data, _, err := client.GL().UsageData.GetServicePing(gl.WithContext(ctx))
	if err != nil {
		return GetServicePingOutput{}, toolutil.WrapErrWithMessage("get_service_ping", err)
	}
	recordedAt := ""
	if data.RecordedAt != nil {
		recordedAt = data.RecordedAt.Format(time.RFC3339)
	}
	return GetServicePingOutput{
		RecordedAt: recordedAt,
		License:    data.License,
		Counts:     data.Counts,
	}, nil
}

// ---------------------------------------------------------------------------
// GetNonSQLMetrics
// ---------------------------------------------------------------------------.

// GetNonSQLMetricsInput is the input (no params).
type GetNonSQLMetricsInput struct{}

// NonSQLMetricsOutput is the output for non-SQL metrics.
type NonSQLMetricsOutput struct {
	toolutil.HintableOutput
	RecordedAt            string            `json:"recorded_at"`
	UUID                  string            `json:"uuid"`
	Hostname              string            `json:"hostname"`
	Version               string            `json:"version"`
	InstallationType      string            `json:"installation_type"`
	ActiveUserCount       int64             `json:"active_user_count"`
	Edition               string            `json:"edition"`
	LicenseMD5            string            `json:"license_md5"`
	LicenseSHA256         string            `json:"license_sha256"`
	LicenseID             string            `json:"license_id"`
	HistoricalMaxUsers    int64             `json:"historical_max_users"`
	Licensee              map[string]string `json:"licensee"`
	LicenseUserCount      int64             `json:"license_user_count"`
	LicenseStartsAt       string            `json:"license_starts_at"`
	LicenseExpiresAt      string            `json:"license_expires_at"`
	LicensePlan           string            `json:"license_plan"`
	LicenseAddOns         map[string]int64  `json:"license_add_ons"`
	LicenseTrial          string            `json:"license_trial"`
	LicenseSubscriptionID string            `json:"license_subscription_id"`
	License               map[string]string `json:"license"`
	Settings              map[string]string `json:"settings"`
}

// GetNonSQLMetrics retrieves non-SQL service ping metrics (admin-only).
func GetNonSQLMetrics(ctx context.Context, client *gitlabclient.Client, _ GetNonSQLMetricsInput) (NonSQLMetricsOutput, error) {
	data, _, err := client.GL().UsageData.GetNonSQLMetrics(gl.WithContext(ctx))
	if err != nil {
		return NonSQLMetricsOutput{}, toolutil.WrapErrWithMessage("get_non_sql_metrics", err)
	}
	return NonSQLMetricsOutput{
		RecordedAt:            data.RecordedAt,
		UUID:                  data.UUID,
		Hostname:              data.Hostname,
		Version:               data.Version,
		InstallationType:      data.InstallationType,
		ActiveUserCount:       data.ActiveUserCount,
		Edition:               data.Edition,
		LicenseMD5:            data.LicenseMD5,
		LicenseSHA256:         data.LicenseSHA256,
		LicenseID:             data.LicenseID,
		HistoricalMaxUsers:    data.HistoricalMaxUsers,
		Licensee:              data.Licensee,
		LicenseUserCount:      data.LicenseUserCount,
		LicenseStartsAt:       data.LicenseStartsAt,
		LicenseExpiresAt:      data.LicenseExpiresAt,
		LicensePlan:           data.LicensePlan,
		LicenseAddOns:         data.LicenseAddOns,
		LicenseTrial:          data.LicenseTrial,
		LicenseSubscriptionID: data.LicenseSubscriptionID,
		License:               data.License,
		Settings:              data.Settings,
	}, nil
}

// ---------------------------------------------------------------------------
// GetQueries
// ---------------------------------------------------------------------------.

// GetQueriesInput is the input (no params).
type GetQueriesInput struct{}

// QueriesOutput is the output for service ping queries.
type QueriesOutput struct {
	toolutil.HintableOutput
	RecordedAt            string            `json:"recorded_at"`
	UUID                  string            `json:"uuid"`
	Hostname              string            `json:"hostname"`
	Version               string            `json:"version"`
	InstallationType      string            `json:"installation_type"`
	ActiveUserCount       string            `json:"active_user_count"`
	Edition               string            `json:"edition"`
	LicenseMD5            string            `json:"license_md5"`
	LicenseSHA256         string            `json:"license_sha256"`
	LicenseID             string            `json:"license_id"`
	HistoricalMaxUsers    int64             `json:"historical_max_users"`
	Licensee              map[string]string `json:"licensee"`
	LicenseUserCount      int64             `json:"license_user_count"`
	LicenseStartsAt       string            `json:"license_starts_at"`
	LicenseExpiresAt      string            `json:"license_expires_at"`
	LicensePlan           string            `json:"license_plan"`
	LicenseAddOns         map[string]int64  `json:"license_add_ons"`
	LicenseTrial          string            `json:"license_trial"`
	LicenseSubscriptionID string            `json:"license_subscription_id"`
	License               map[string]string `json:"license"`
	Settings              map[string]string `json:"settings"`
	Counts                map[string]string `json:"counts"`
}

// GetQueries retrieves service ping SQL queries (admin-only).
func GetQueries(ctx context.Context, client *gitlabclient.Client, _ GetQueriesInput) (QueriesOutput, error) {
	data, _, err := client.GL().UsageData.GetQueries(gl.WithContext(ctx))
	if err != nil {
		return QueriesOutput{}, toolutil.WrapErrWithMessage("get_usage_queries", err)
	}
	recordedAt := ""
	if data.RecordedAt != nil {
		recordedAt = data.RecordedAt.Format(time.RFC3339)
	}
	return QueriesOutput{
		RecordedAt:            recordedAt,
		UUID:                  data.UUID,
		Hostname:              data.Hostname,
		Version:               data.Version,
		InstallationType:      data.InstallationType,
		ActiveUserCount:       data.ActiveUserCount,
		Edition:               data.Edition,
		LicenseMD5:            data.LicenseMD5,
		LicenseSHA256:         data.LicenseSHA256,
		LicenseID:             data.LicenseID,
		HistoricalMaxUsers:    data.HistoricalMaxUsers,
		Licensee:              data.Licensee,
		LicenseUserCount:      data.LicenseUserCount,
		LicenseStartsAt:       data.LicenseStartsAt,
		LicenseExpiresAt:      data.LicenseExpiresAt,
		LicensePlan:           data.LicensePlan,
		LicenseAddOns:         data.LicenseAddOns,
		LicenseTrial:          data.LicenseTrial,
		LicenseSubscriptionID: data.LicenseSubscriptionID,
		License:               data.License,
		Settings:              data.Settings,
		Counts:                data.Counts,
	}, nil
}

// ---------------------------------------------------------------------------
// GetMetricDefinitions (YAML)
// ---------------------------------------------------------------------------.

// GetMetricDefinitionsInput is the input (no params).
type GetMetricDefinitionsInput struct{}

// MetricDefinitionsOutput is the output for metric definitions.
type MetricDefinitionsOutput struct {
	toolutil.HintableOutput
	YAML string `json:"yaml"`
}

// GetMetricDefinitions retrieves metric definitions as YAML (admin-only).
func GetMetricDefinitions(ctx context.Context, client *gitlabclient.Client, _ GetMetricDefinitionsInput) (MetricDefinitionsOutput, error) {
	reader, _, err := client.GL().UsageData.GetMetricDefinitionsAsYAML(gl.WithContext(ctx))
	if err != nil {
		return MetricDefinitionsOutput{}, toolutil.WrapErrWithMessage("get_metric_definitions", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return MetricDefinitionsOutput{}, toolutil.WrapErrWithMessage("get_metric_definitions", fmt.Errorf("reading response body: %w", err))
	}
	return MetricDefinitionsOutput{YAML: string(data)}, nil
}

// ---------------------------------------------------------------------------
// TrackEvent
// ---------------------------------------------------------------------------.

// TrackEventInput is the input for tracking a single event.
type TrackEventInput struct {
	Event          string `json:"event" jsonschema:"Event name to track,required"`
	SendToSnowplow *bool  `json:"send_to_snowplow,omitempty" jsonschema:"Whether to send event to Snowplow"`
	NamespaceID    *int64 `json:"namespace_id,omitempty" jsonschema:"Namespace ID"`
	ProjectID      *int64 `json:"project_id,omitempty" jsonschema:"Project ID"`
}

// TrackEventOutput is the output for tracking a single event.
type TrackEventOutput struct {
	toolutil.HintableOutput
	Status string `json:"status"`
}

// TrackEvent tracks a single usage event.
func TrackEvent(ctx context.Context, client *gitlabclient.Client, input TrackEventInput) (TrackEventOutput, error) {
	opts := &gl.TrackEventOptions{
		Event:          input.Event,
		SendToSnowplow: input.SendToSnowplow,
		NamespaceID:    input.NamespaceID,
		ProjectID:      input.ProjectID,
	}

	_, err := client.GL().UsageData.TrackEvent(opts, gl.WithContext(ctx))
	if err != nil {
		return TrackEventOutput{}, toolutil.WrapErrWithMessage("track_event", err)
	}
	return TrackEventOutput{Status: "accepted"}, nil
}

// ---------------------------------------------------------------------------
// TrackEvents (batch)
// ---------------------------------------------------------------------------.

// TrackEventsInput is the input for tracking multiple events.
type TrackEventsInput struct {
	Events []TrackEventInput `json:"events" jsonschema:"Array of events to track,required"`
}

// TrackEventsOutput is the output for tracking multiple events.
type TrackEventsOutput struct {
	toolutil.HintableOutput
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// TrackEvents tracks multiple usage events in batch.
func TrackEvents(ctx context.Context, client *gitlabclient.Client, input TrackEventsInput) (TrackEventsOutput, error) {
	events := make([]gl.TrackEventOptions, 0, len(input.Events))
	for _, e := range input.Events {
		events = append(events, gl.TrackEventOptions{
			Event:          e.Event,
			SendToSnowplow: e.SendToSnowplow,
			NamespaceID:    e.NamespaceID,
			ProjectID:      e.ProjectID,
		})
	}

	_, err := client.GL().UsageData.TrackEvents(&gl.TrackEventsOptions{Events: events}, gl.WithContext(ctx))
	if err != nil {
		return TrackEventsOutput{}, toolutil.WrapErrWithMessage("track_events", err)
	}
	return TrackEventsOutput{Status: "accepted", Count: len(input.Events)}, nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------.

// sortedKeys is an internal helper for the usagedata package.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedKeysInt64 is an internal helper for the usagedata package.
func sortedKeysInt64(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
