package geo

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// siteExtra is what ee/lib/api/entities/geo_site.rb sends on a Geo site that
// client-go's GeoSite does not carry, read from the captured response beside
// the SDK's own decode (ADR-0021). The gap is recorded in
// docs/development/upstream-bugs.md.
type siteExtra struct {
	SelectiveSyncOrganizationIDs            []int64 `json:"selective_sync_organization_ids"`
	BlobDownloadTimeout                     int64   `json:"blob_download_timeout"`
	ChecksumMismatchReportThreshold         int64   `json:"checksum_mismatch_report_threshold"`
	ChecksumMismatchSelfHealCooldownMinutes int64   `json:"checksum_mismatch_self_heal_cooldown_minutes"`
}

// capturedSite reads the site fields client-go does not model off the answer
// to a request that returned one site.
func capturedSite(capture *gitlabclient.ResponseCapture) (siteExtra, error) {
	var extra siteExtra
	if err := capture.Decode(&extra); err != nil {
		return siteExtra{}, err
	}
	return extra, nil
}

// capturedSites reads the same off a list answer, one per site in the list's
// order. The count is held to what the SDK decoded: the two read the same
// bytes, so a difference is a fault in this reader and not in the answer.
func capturedSites(capture *gitlabclient.ResponseCapture, decoded int) ([]siteExtra, error) {
	var extras []siteExtra
	if err := capture.Decode(&extras); err != nil {
		return nil, err
	}
	if len(extras) != decoded {
		return nil, fmt.Errorf("the captured answer holds %d geo sites and the SDK decoded %d", len(extras), decoded)
	}
	return extras, nil
}

// ReplicableStatus is the replication and verification state of one replicable
// resource on a Geo site.
//
// GitLab builds these per replicator class and flattens them into the status
// object's key names, `lfs_objects_synced_count` and 572 siblings, so the wire
// form is a matrix pretending to be a list. Publishing it as a map of this
// shape says what the data is and needs no edit when GitLab enables another
// replicable, which it does most releases.
//
// The ten counts and the two percentages are rendered for every replicator
// class. `oldest_unsynced_time` is null until something is behind, and
// `replication_enabled` is exposed for container repositories alone, so both
// are omitted when absent rather than published as a zero value.
type ReplicableStatus struct {
	Count                   int64      `json:"count"`
	RegistryCount           int64      `json:"registry_count"`
	SyncedCount             int64      `json:"synced_count"`
	FailedCount             int64      `json:"failed_count"`
	ChecksummedCount        int64      `json:"checksummed_count"`
	ChecksumFailedCount     int64      `json:"checksum_failed_count"`
	ChecksumTotalCount      int64      `json:"checksum_total_count"`
	VerifiedCount           int64      `json:"verified_count"`
	VerificationFailedCount int64      `json:"verification_failed_count"`
	VerificationTotalCount  int64      `json:"verification_total_count"`
	SyncedInPercentage      string     `json:"synced_in_percentage"`
	VerifiedInPercentage    string     `json:"verified_in_percentage"`
	OldestUnsyncedTime      *time.Time `json:"oldest_unsynced_time,omitempty"`
	ReplicationEnabled      *bool      `json:"replication_enabled,omitempty"`
}

// StorageShard is one repository storage the site reports, as
// StorageShardEntity renders it: the shard's name and nothing else.
type StorageShard struct {
	Name string `json:"name"`
}

// statusExtra is what a Geo site status carries that client-go's
// GeoSiteStatus does not: the whole replicable matrix beyond the fifteen
// resources the SDK spells out, the repository count and the storage shards.
type statusExtra struct {
	RepositoriesCount int64
	StorageShards     []StorageShard
	Replicables       map[string]ReplicableStatus
	Additional        map[string]any
}

// minReplicableMetrics is how many metrics of the family one prefix must carry
// in an answer before it is read as a replicable rather than as a counter
// whose name happens to end in one of them.
//
// GitLab renders thirteen metrics for every replicator class and at most two
// for any of the singular counters that share the shape (`repositories_checked`
// has a count and a failed count), so three separates them with room to spare
// and keeps no list of names that would go stale.
const minReplicableMetrics = 3

// replicableMetrics places one metric of the family on a replicable's status.
//
// A table rather than a switch because the same keys are the suffixes the key
// decomposition looks for, so the two cannot drift apart.
var replicableMetrics = map[string]func(*ReplicableStatus, json.RawMessage) error{
	"count":             func(r *ReplicableStatus, raw json.RawMessage) error { return json.Unmarshal(raw, &r.Count) },
	"registry_count":    func(r *ReplicableStatus, raw json.RawMessage) error { return json.Unmarshal(raw, &r.RegistryCount) },
	"synced_count":      func(r *ReplicableStatus, raw json.RawMessage) error { return json.Unmarshal(raw, &r.SyncedCount) },
	"failed_count":      func(r *ReplicableStatus, raw json.RawMessage) error { return json.Unmarshal(raw, &r.FailedCount) },
	"checksummed_count": func(r *ReplicableStatus, raw json.RawMessage) error { return json.Unmarshal(raw, &r.ChecksummedCount) },
	"checksum_failed_count": func(r *ReplicableStatus, raw json.RawMessage) error {
		return json.Unmarshal(raw, &r.ChecksumFailedCount)
	},
	"checksum_total_count": func(r *ReplicableStatus, raw json.RawMessage) error {
		return json.Unmarshal(raw, &r.ChecksumTotalCount)
	},
	"verified_count": func(r *ReplicableStatus, raw json.RawMessage) error { return json.Unmarshal(raw, &r.VerifiedCount) },
	"verification_failed_count": func(r *ReplicableStatus, raw json.RawMessage) error {
		return json.Unmarshal(raw, &r.VerificationFailedCount)
	},
	"verification_total_count": func(r *ReplicableStatus, raw json.RawMessage) error {
		return json.Unmarshal(raw, &r.VerificationTotalCount)
	},
	"synced_in_percentage": func(r *ReplicableStatus, raw json.RawMessage) error {
		return json.Unmarshal(raw, &r.SyncedInPercentage)
	},
	"verified_in_percentage": func(r *ReplicableStatus, raw json.RawMessage) error {
		return json.Unmarshal(raw, &r.VerifiedInPercentage)
	},
	"oldest_unsynced_time": func(r *ReplicableStatus, raw json.RawMessage) error {
		return json.Unmarshal(raw, &r.OldestUnsyncedTime)
	},
	"replication_enabled": func(r *ReplicableStatus, raw json.RawMessage) error {
		return json.Unmarshal(raw, &r.ReplicationEnabled)
	},
}

// replicableMetricSuffixes are the metric names longest first, which is what
// makes `lfs_objects_checksum_failed_count` decompose as `checksum_failed_count`
// under `lfs_objects` rather than as `count` under a replicable that does not
// exist.
var replicableMetricSuffixes = orderedMetricSuffixes()

// orderedMetricSuffixes sorts the metric names by length, longest first, and
// alphabetically among equals so two runs agree.
func orderedMetricSuffixes() []string {
	suffixes := make([]string, 0, len(replicableMetrics))
	for name := range replicableMetrics {
		suffixes = append(suffixes, name)
	}
	slices.SortFunc(suffixes, func(a, b string) int {
		if diff := cmp.Compare(len(b), len(a)); diff != 0 {
			return diff
		}
		return cmp.Compare(a, b)
	})
	return suffixes
}

// splitReplicableKey splits one flat status key into the replicable it belongs
// to and the metric it carries.
func splitReplicableKey(key string) (replicable, metric string, ok bool) {
	for _, suffix := range replicableMetricSuffixes {
		trimmed, found := strings.CutSuffix(key, "_"+suffix)
		if found && trimmed != "" {
			return trimmed, suffix, true
		}
	}
	return "", "", false
}

// statusPublishedNames is the set of json names StatusOutput publishes under a
// name of its own.
//
// Read off the struct rather than listed, so a field added to the type cannot
// start arriving twice, once under its own name and once among the keys this
// build does not model.
var statusPublishedNames = sync.OnceValue(func() map[string]bool {
	names := map[string]bool{}
	collectJSONNames(reflect.TypeFor[StatusOutput](), names)
	return names
})

// collectJSONNames records the json name of every field of t, descending into
// an untagged embed because encoding/json promotes its fields into the object
// rather than placing them under a key.
func collectJSONNames(t reflect.Type, into map[string]bool) {
	for field := range t.Fields() {
		if field.Anonymous && field.Type.Kind() == reflect.Struct && field.Tag.Get("json") == "" {
			collectJSONNames(field.Type, into)
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			into[name] = true
		}
	}
}

// statusExtraFrom decomposes the flat keys of one status answer into the
// matrix, the two fields published under their own names, and whatever is
// left.
//
// A key that belongs to none of those and that StatusOutput does not publish
// under its own name is kept rather than dropped, so a field GitLab adds to
// the entity reaches the caller before this code knows its name. A key that
// does belong to one, carrying a value the shape cannot hold, is instead an
// error the handler returns (ADR-0021): the SDK decoded the same bytes, so
// the fault is in this reader.
//
// The keys are walked in order because a map is not: two runs over one answer
// with several unreadable values would otherwise name a different one.
func statusExtraFrom(raw map[string]json.RawMessage) (statusExtra, error) {
	families := metricFamilies(raw)
	var extra statusExtra
	for _, key := range slices.Sorted(maps.Keys(raw)) {
		placed, err := extra.place(key, raw[key], families)
		if err != nil {
			return statusExtra{}, fmt.Errorf("decode the captured response: geo site status %q: %w", key, err)
		}
		if !placed && !statusPublishedNames()[key] {
			extra.keep(key, raw[key])
		}
	}
	return extra, nil
}

// place puts one key of the answer where it belongs and reports whether it
// found a home: a cell of the matrix, or one of the two fields StatusOutput
// publishes under GitLab's own name that client-go does not model.
func (e *statusExtra) place(key string, value json.RawMessage, families map[string]int) (placed bool, err error) {
	if prefix, metric, ok := splitReplicableKey(key); ok && families[prefix] >= minReplicableMetrics {
		cell := e.Replicables[prefix]
		if refused := replicableMetrics[metric](&cell, value); refused != nil {
			return false, refused
		}
		if e.Replicables == nil {
			e.Replicables = map[string]ReplicableStatus{}
		}
		e.Replicables[prefix] = cell
		return true, nil
	}
	switch key {
	case "repositories_count":
		return true, json.Unmarshal(value, &e.RepositoriesCount)
	case "storage_shards":
		return true, json.Unmarshal(value, &e.StorageShards)
	}
	return false, nil
}

// metricFamilies counts, per prefix, how many metrics of the family the answer
// carries for it. A JSON object cannot repeat a key, so each metric is counted
// once without a set to hold it.
func metricFamilies(raw map[string]json.RawMessage) map[string]int {
	counts := map[string]int{}
	for key := range raw {
		if prefix, _, ok := splitReplicableKey(key); ok {
			counts[prefix]++
		}
	}
	return counts
}

// keep records a key this build does not model, under the value GitLab sent.
func (e *statusExtra) keep(key string, value json.RawMessage) {
	var decoded any
	// The raw message came out of an object that decoded, so it is valid JSON
	// and this cannot fail; there is nothing to branch on.
	_ = json.Unmarshal(value, &decoded)
	if e.Additional == nil {
		e.Additional = map[string]any{}
	}
	e.Additional[key] = decoded
}

// capturedStatus reads the matrix and the unmodelled fields off the answer to
// a request that returned one site status.
func capturedStatus(capture *gitlabclient.ResponseCapture) (statusExtra, error) {
	var raw map[string]json.RawMessage
	if err := capture.Decode(&raw); err != nil {
		return statusExtra{}, err
	}
	return statusExtraFrom(raw)
}

// capturedStatuses reads the same off a list answer, one per status in the
// list's order, the count held to what the SDK decoded as [capturedSites]
// holds it.
func capturedStatuses(capture *gitlabclient.ResponseCapture, decoded int) ([]statusExtra, error) {
	var raws []map[string]json.RawMessage
	if err := capture.Decode(&raws); err != nil {
		return nil, err
	}
	if len(raws) != decoded {
		return nil, fmt.Errorf("the captured answer holds %d geo site statuses and the SDK decoded %d", len(raws), decoded)
	}
	extras := make([]statusExtra, 0, len(raws))
	for _, raw := range raws {
		extra, err := statusExtraFrom(raw)
		if err != nil {
			return nil, err
		}
		extras = append(extras, extra)
	}
	return extras, nil
}
