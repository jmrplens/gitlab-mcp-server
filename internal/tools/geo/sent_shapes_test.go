// sent_shapes_test.go covers the readers that take what a Geo answer carries
// beside client-go's decode, and the decomposition of the flat status keys
// into the replicable matrix.
package geo

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// decompose runs the status decomposition over a JSON object literal, so a
// case can be written as the answer GitLab would send.
func decompose(t *testing.T, body string) statusExtra {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("fixture does not parse: %v", err)
	}
	extra, err := statusExtraFrom(raw)
	if err != nil {
		t.Fatalf("statusExtraFrom() error: %v", err)
	}
	return extra
}

// refuse runs the same over an answer expected to be refused, and returns the
// error so a case can say which key it should name.
func refuse(t *testing.T, body string) error {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("fixture does not parse: %v", err)
	}
	_, err := statusExtraFrom(raw)
	if err == nil {
		t.Fatal("statusExtraFrom() accepted a value the shape cannot hold")
	}
	return err
}

// TestSplitReplicableKey_LongestMetricWins verifies the key decomposition
// takes the longest metric name that ends the key, which is what keeps
// "lfs_objects_checksum_failed_count" from reading as "count" under a
// replicable called "lfs_objects_checksum_failed", and refuses a key that is
// a bare metric name or carries none.
func TestSplitReplicableKey_LongestMetricWins(t *testing.T) {
	cases := []struct {
		name           string
		key            string
		wantReplicable string
		wantMetric     string
		wantOK         bool
	}{
		{name: "plain count", key: "lfs_objects_count", wantReplicable: "lfs_objects", wantMetric: "count", wantOK: true},
		{
			name: "checksum failed is not failed", key: "lfs_objects_checksum_failed_count",
			wantReplicable: "lfs_objects", wantMetric: "checksum_failed_count", wantOK: true,
		},
		{
			name: "verification failed is not failed", key: "uploads_verification_failed_count",
			wantReplicable: "uploads", wantMetric: "verification_failed_count", wantOK: true,
		},
		{
			name: "checksummed is not count", key: "uploads_checksummed_count",
			wantReplicable: "uploads", wantMetric: "checksummed_count", wantOK: true,
		},
		{
			name: "percentage", key: "uploads_verified_in_percentage",
			wantReplicable: "uploads", wantMetric: "verified_in_percentage", wantOK: true,
		},
		{
			name: "oldest unsynced", key: "uploads_oldest_unsynced_time",
			wantReplicable: "uploads", wantMetric: "oldest_unsynced_time", wantOK: true,
		},
		{
			name: "replication enabled", key: "container_repositories_replication_enabled",
			wantReplicable: "container_repositories", wantMetric: "replication_enabled", wantOK: true,
		},
		{name: "no metric", key: "health_status"},
		{name: "bare metric name", key: "count"},
		{name: "metric with an empty prefix", key: "_count"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			replicable, metric, ok := splitReplicableKey(testCase.key)
			if ok != testCase.wantOK || replicable != testCase.wantReplicable || metric != testCase.wantMetric {
				t.Errorf("splitReplicableKey(%q) = %q, %q, %t; want %q, %q, %t",
					testCase.key, replicable, metric, ok, testCase.wantReplicable, testCase.wantMetric, testCase.wantOK)
			}
		})
	}
}

// TestOrderedMetricSuffixes_LongestFirstAndStable verifies the suffix order
// the decomposition relies on: every name of the metric table appears once,
// longest first, and equal lengths in alphabetical order so two runs agree.
func TestOrderedMetricSuffixes_LongestFirstAndStable(t *testing.T) {
	suffixes := orderedMetricSuffixes()
	if len(suffixes) != len(replicableMetrics) {
		t.Fatalf("orderedMetricSuffixes() has %d entries, want the %d of the metric table", len(suffixes), len(replicableMetrics))
	}
	for i := 1; i < len(suffixes); i++ {
		previous, current := suffixes[i-1], suffixes[i]
		if len(previous) < len(current) {
			t.Errorf("%q precedes the longer %q", previous, current)
		}
		if len(previous) == len(current) && previous >= current {
			t.Errorf("%q and %q are the same length and out of alphabetical order", previous, current)
		}
	}
	for _, suffix := range suffixes {
		if _, ok := replicableMetrics[suffix]; !ok {
			t.Errorf("%q is ordered and is not a metric", suffix)
		}
	}
}

// TestStatusExtraFrom_TheMatrixIsKeyedByReplicable verifies the flat keys of a
// status answer become one entry per replicable carrying the metrics that
// belong to it, with the two fields the type publishes under GitLab's own
// name read out separately and nothing left over.
func TestStatusExtraFrom_TheMatrixIsKeyedByReplicable(t *testing.T) {
	extra := decompose(t, `{
		"geo_node_id": 1,
		"repositories_count": 19,
		"storage_shards": [{"name": "default"}],
		"lfs_objects_count": 120,
		"lfs_objects_synced_count": 118,
		"lfs_objects_failed_count": 2,
		"lfs_objects_synced_in_percentage": "98.33%",
		"lfs_objects_oldest_unsynced_time": "2026-01-15T09:00:00Z",
		"container_repositories_count": 4,
		"container_repositories_synced_count": 4,
		"container_repositories_replication_enabled": true
	}`)

	if extra.RepositoriesCount != 19 {
		t.Errorf("RepositoriesCount = %d, want 19", extra.RepositoriesCount)
	}
	if !reflect.DeepEqual(extra.StorageShards, []StorageShard{{Name: "default"}}) {
		t.Errorf("StorageShards = %+v, want the one shard", extra.StorageShards)
	}
	if len(extra.Replicables) != 2 {
		t.Fatalf("Replicables = %+v, want lfs_objects and container_repositories", extra.Replicables)
	}
	lfs := extra.Replicables["lfs_objects"]
	if lfs.Count != 120 || lfs.SyncedCount != 118 || lfs.FailedCount != 2 || lfs.SyncedInPercentage != "98.33%" {
		t.Errorf("lfs_objects = %+v, want the four metrics of the answer", lfs)
	}
	want := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	if lfs.OldestUnsyncedTime == nil || !lfs.OldestUnsyncedTime.Equal(want) {
		t.Errorf("lfs_objects.OldestUnsyncedTime = %v, want %v", lfs.OldestUnsyncedTime, want)
	}
	containers := extra.Replicables["container_repositories"]
	if containers.ReplicationEnabled == nil || !*containers.ReplicationEnabled {
		t.Errorf("container_repositories.ReplicationEnabled = %v, want true", containers.ReplicationEnabled)
	}
	if len(extra.Additional) != 0 {
		t.Errorf("Additional = %+v, want nothing left over", extra.Additional)
	}
}

// TestStatusExtraFrom_APrefixUnderTheThresholdIsNotAReplicable verifies the
// rule that separates the matrix from the singular counters whose names end
// in a metric: two metrics leave the keys alone, three group them. Without
// it, projects_count and repositories_checked_failed_count would invent
// replicables called "projects" and "repositories_checked".
func TestStatusExtraFrom_APrefixUnderTheThresholdIsNotAReplicable(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		grasp bool
	}{
		{
			name: "two metrics stay singular",
			body: `{"repositories_checked_count": 3, "repositories_checked_failed_count": 1}`,
		},
		{
			name:  "three metrics are a replicable",
			body:  `{"widgets_count": 3, "widgets_failed_count": 1, "widgets_synced_count": 2}`,
			grasp: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			extra := decompose(t, testCase.body)
			if (len(extra.Replicables) > 0) != testCase.grasp {
				t.Errorf("Replicables = %+v, want a replicable: %t", extra.Replicables, testCase.grasp)
			}
		})
	}
}

// TestStatusExtraFrom_AKeyItCannotDecomposeIsKept verifies a key that is
// neither a matrix cell nor a name StatusOutput publishes reaches the caller
// rather than being dropped, and that the map is filled once and added to.
func TestStatusExtraFrom_AKeyItCannotDecomposeIsKept(t *testing.T) {
	extra := decompose(t, `{"future_gitlab_field": "value", "another_new_thing": 3, "health_status": "Healthy"}`)
	want := map[string]any{"future_gitlab_field": "value", "another_new_thing": float64(3)}
	if !reflect.DeepEqual(extra.Additional, want) {
		t.Errorf("Additional = %+v, want %+v", extra.Additional, want)
	}
}

// TestStatusExtraFrom_AValueTheShapeCannotHoldIsRefused verifies the other
// half of ADR-0021: a key that does decompose, whose value the field or the
// metric cannot hold, is an error naming that key rather than a status
// missing what GitLab sent.
func TestStatusExtraFrom_AValueTheShapeCannotHoldIsRefused(t *testing.T) {
	cases := []struct {
		name string
		body string
		key  string
	}{
		{
			name: "a metric value the cell cannot hold",
			body: `{"widgets_count": "many", "widgets_failed_count": 1, "widgets_synced_count": 2}`,
			key:  "widgets_count",
		},
		{
			name: "a repository count that is not a number",
			body: `{"repositories_count": "nineteen"}`,
			key:  "repositories_count",
		},
		{
			name: "storage shards that are not shards",
			body: `{"storage_shards": "default"}`,
			key:  "storage_shards",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := refuse(t, testCase.body)
			if !strings.Contains(err.Error(), testCase.key) {
				t.Errorf("statusExtraFrom() error = %v, want it to name %q", err, testCase.key)
			}
			if !strings.Contains(err.Error(), "decode the captured response") {
				t.Errorf("statusExtraFrom() error = %v, want the fragment a handler test looks for", err)
			}
		})
	}
}

// TestStatusExtraFrom_APublishedNameIsNotKeptTwice verifies a key the type
// publishes under its own name is left to that field rather than repeated
// among the ones this build does not model.
func TestStatusExtraFrom_APublishedNameIsNotKeptTwice(t *testing.T) {
	extra := decompose(t, `{"health_status": "Healthy", "version": "19.3.1", "projects_count": 42}`)
	if len(extra.Additional) != 0 {
		t.Errorf("Additional = %+v, want nothing: every key is a field of StatusOutput", extra.Additional)
	}
}

// TestStatusPublishedNames_ReadsTheStructIncludingItsEmbed verifies the set of
// names the decomposition compares against is taken off StatusOutput itself,
// so it carries the fields declared there and stays right when one is added.
func TestStatusPublishedNames_ReadsTheStructIncludingItsEmbed(t *testing.T) {
	names := statusPublishedNames()
	for _, want := range []string{"geo_node_id", "health_status", "storage_shards", "replicables", "next_steps"} {
		t.Run(want, func(t *testing.T) {
			if !names[want] {
				t.Errorf("statusPublishedNames() does not carry %q", want)
			}
		})
	}
	if names["lfs_objects_oldest_unsynced_time"] {
		t.Error("statusPublishedNames() carries a matrix cell StatusOutput does not declare")
	}
}

// TestCollectJSONNames_TagShapes verifies the reader of a struct's json names
// takes the name before the options, descends into an untagged embed because
// encoding/json promotes its fields, and skips a field named "-" or none. The
// last two members of the subject are there for the conjunction: a struct
// field that is not embedded, and an embed that carries a name of its own,
// are both named rather than descended into.
func TestCollectJSONNames_TagShapes(t *testing.T) {
	type embedded struct {
		Promoted string `json:"promoted"`
	}
	type tagged struct {
		Under string `json:"under"`
	}
	type subject struct {
		embedded
		tagged        `json:"tagged"`
		time.Duration        // an embed that is not a struct, so there is nothing to descend into
		Named         string `json:"named,omitempty"`
		Excluded      string `json:"-"`
		Untagged      string
		Plain         tagged
	}

	names := map[string]bool{}
	collectJSONNames(reflect.TypeFor[subject](), names)

	want := map[string]bool{"promoted": true, "tagged": true, "named": true}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("collectJSONNames() = %v, want %v", names, want)
	}
}

// TestCapturedSite_ReadsWhatTheSDKDoesNotModel verifies the site reader takes
// the four fields client-go's GeoSite has no room for, and reports a body
// that does not fit rather than answering with zero values.
func TestCapturedSite_ReadsWhatTheSDKDoesNotModel(t *testing.T) {
	extra, err := capturedSite(gitlabclient.CapturedBody([]byte(`{
		"selective_sync_organization_ids": [7, 9],
		"blob_download_timeout": 28800,
		"checksum_mismatch_report_threshold": 5,
		"checksum_mismatch_self_heal_cooldown_minutes": 60
	}`)))
	if err != nil {
		t.Fatalf("capturedSite() error: %v", err)
	}
	want := siteExtra{
		SelectiveSyncOrganizationIDs:            []int64{7, 9},
		BlobDownloadTimeout:                     28800,
		ChecksumMismatchReportThreshold:         5,
		ChecksumMismatchSelfHealCooldownMinutes: 60,
	}
	if !reflect.DeepEqual(extra, want) {
		t.Errorf("capturedSite() = %+v, want %+v", extra, want)
	}

	if _, refused := capturedSite(gitlabclient.CapturedBody([]byte(`{"blob_download_timeout": "soon"}`))); refused == nil {
		t.Error("capturedSite() accepted a timeout that is not a number")
	}
}

// TestCapturedSites_HoldsTheCountToTheSDK verifies the list reader pairs one
// extra per site, and refuses an answer whose length disagrees with what the
// SDK decoded, since both read the same bytes.
func TestCapturedSites_HoldsTheCountToTheSDK(t *testing.T) {
	body := []byte(`[{"blob_download_timeout": 1}, {"blob_download_timeout": 2}]`)
	extras, err := capturedSites(gitlabclient.CapturedBody(body), 2)
	if err != nil {
		t.Fatalf("capturedSites() error: %v", err)
	}
	if len(extras) != 2 || extras[0].BlobDownloadTimeout != 1 || extras[1].BlobDownloadTimeout != 2 {
		t.Errorf("capturedSites() = %+v, want the two sites in order", extras)
	}

	if _, refused := capturedSites(gitlabclient.CapturedBody(body), 1); refused == nil {
		t.Error("capturedSites() accepted two sites where the SDK decoded one")
	} else if !strings.Contains(refused.Error(), "the SDK decoded 1") {
		t.Errorf("capturedSites() error = %v, want it to name both counts", refused)
	}
	if _, refused := capturedSites(gitlabclient.CapturedBody([]byte(`{}`)), 1); refused == nil {
		t.Error("capturedSites() accepted an object where GitLab sends a list")
	}
}

// TestCapturedStatus_DecomposesOrReports verifies the status reader hands the
// decomposition an object, and reports a body that is not one instead of
// answering with an empty matrix.
func TestCapturedStatus_DecomposesOrReports(t *testing.T) {
	extra, err := capturedStatus(gitlabclient.CapturedBody([]byte(`{"repositories_count": 19}`)))
	if err != nil {
		t.Fatalf("capturedStatus() error: %v", err)
	}
	if extra.RepositoriesCount != 19 {
		t.Errorf("capturedStatus() = %+v, want the repository count", extra)
	}
	if _, refused := capturedStatus(gitlabclient.CapturedBody([]byte(`[]`))); refused == nil {
		t.Error("capturedStatus() accepted a list where GitLab sends one status")
	}
	if _, refused := capturedStatus(gitlabclient.CapturedBody([]byte(`{"repositories_count": "x"}`))); refused == nil {
		t.Error("capturedStatus() accepted a repository count that is not a number")
	}
}

// TestCapturedStatuses_HoldsTheCountToTheSDK verifies the status list reader
// decomposes each element and refuses a length the SDK's own decode
// disagrees with.
func TestCapturedStatuses_HoldsTheCountToTheSDK(t *testing.T) {
	body := []byte(`[{"repositories_count": 19}, {"repositories_count": 4}]`)
	extras, err := capturedStatuses(gitlabclient.CapturedBody(body), 2)
	if err != nil {
		t.Fatalf("capturedStatuses() error: %v", err)
	}
	if len(extras) != 2 || extras[0].RepositoriesCount != 19 || extras[1].RepositoriesCount != 4 {
		t.Errorf("capturedStatuses() = %+v, want the two statuses in order", extras)
	}

	if _, refused := capturedStatuses(gitlabclient.CapturedBody(body), 3); refused == nil {
		t.Error("capturedStatuses() accepted two statuses where the SDK decoded three")
	} else if !strings.Contains(refused.Error(), "the SDK decoded 3") {
		t.Errorf("capturedStatuses() error = %v, want it to name both counts", refused)
	}
	if _, refused := capturedStatuses(gitlabclient.CapturedBody([]byte(`{}`)), 1); refused == nil {
		t.Error("capturedStatuses() accepted an object where GitLab sends a list")
	}
	bad := []byte(`[{"repositories_count": 19}, {"repositories_count": "x"}]`)
	if _, refused := capturedStatuses(gitlabclient.CapturedBody(bad), 2); refused == nil {
		t.Error("capturedStatuses() accepted a second status the shape cannot hold")
	}
}
