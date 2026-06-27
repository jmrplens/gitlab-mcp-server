// releaseshapes_test.go contains unit tests for the shared release-shape
// converters exposed by toolutil. The previous per-package tests were
// retained by deleting the local shapes.go definitions; this file replaces
// the shared unit-test surface so future regressions in any consumer
// (releases, groupreleases) are caught from the canonical home.
package toolutil

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestNewAssetsOutput verifies the converter omits the assets object when
// the SDK value is fully empty, copies sources / links when populated, and
// skips nil link entries inside assets.links.
func TestNewAssetsOutput(t *testing.T) {
	if got := NewAssetsOutput(gl.ReleaseAssets{}); got != nil {
		t.Errorf("empty assets must return nil, got %+v", got)
	}
	got := NewAssetsOutput(gl.ReleaseAssets{
		Count:   3,
		Sources: []gl.ReleaseAssetsSource{{Format: "package", URL: "u1"}, {Format: "image", URL: "u2"}},
		Links: []*gl.ReleaseLink{
			{ID: 1, Name: "binary", URL: "https://b", DirectAssetURL: "/d", External: true, LinkType: "package"},
			nil,
			{ID: 2, Name: "docs", URL: "https://d", LinkType: "other"},
		},
		EvidenceFilePath: "evidence.json",
	})
	if got == nil {
		t.Fatal("populated assets must return non-nil")
	}
	if got.Count != 3 || got.EvidenceFilePath != "evidence.json" {
		t.Errorf("count / evidence: %+v", got)
	}
	if len(got.Sources) != 2 || got.Sources[0].Format != "package" || got.Sources[1].URL != "u2" {
		t.Errorf("sources: %+v", got.Sources)
	}
	if len(got.Links) != 2 || got.Links[0].LinkType != "package" || got.Links[1].ID != 2 {
		t.Errorf("links (nil entry should be skipped): %+v", got.Links)
	}
}

// TestNewLinksOutput verifies the converter omits the links object when
// every URL field is empty and copies only the populated URLs.
func TestNewLinksOutput(t *testing.T) {
	if got := NewLinksOutput(gl.ReleaseLinks{}); got != nil {
		t.Errorf("empty links must return nil, got %+v", got)
	}
	got := NewLinksOutput(gl.ReleaseLinks{
		Self:               "https://self",
		EditURL:            "https://edit",
		OpenedIssues:       "https://o-i",
		OpenedMergeRequest: "https://o-m",
		MergedMergeRequest: "https://m-m",
		ClosedIssueURL:     "https://c-i",
		ClosedMergeRequest: "https://c-m",
	})
	if got == nil {
		t.Fatal("populated links must return non-nil")
	}
	want := `{"self":"https://self","edit_url":"https://edit","opened_issues_url":"https://o-i","opened_merge_requests_url":"https://o-m","merged_merge_requests_url":"https://m-m","closed_issues_url":"https://c-i","closed_merge_requests_url":"https://c-m"}`
	raw, _ := json.Marshal(got)
	if string(raw) != want {
		t.Errorf("links JSON mismatch:\n want %s\n  got %s", want, raw)
	}
}

// TestNewMilestoneOutputs verifies the milestone list skips nil entries,
// formats timestamps via FormatTimePtr / FormatISOTimePtr, and returns nil
// for an empty input slice.
func TestNewMilestoneOutputs(t *testing.T) {
	if got := NewMilestoneOutputs(nil); got != nil {
		t.Errorf("nil slice must return nil, got %+v", got)
	}
	created := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	due := gl.ISOTime(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	_ = due // already typed
	got := NewMilestoneOutputs([]*gl.ReleaseMilestone{
		nil,
		{
			ID: 1, IID: 2, ProjectID: 3, Title: "M", Description: "D", State: "active", WebURL: "w",
			CreatedAt: &created, UpdatedAt: &updated, StartDate: nil, DueDate: &due,
			IssueStats: &gl.ReleaseMilestoneIssueStats{Total: 5, Closed: 2},
		},
	})
	if got == nil || len(got) != 1 {
		t.Fatalf("want exactly one milestone (nil skipped), got %+v", got)
	}
	m := got[0]
	if m.ID != 1 || m.Title != "M" || m.State != "active" {
		t.Errorf("scalar fields: %+v", m)
	}
	if m.CreatedAt != "2026-06-01T00:00:00Z" {
		t.Errorf("CreatedAt: %q", m.CreatedAt)
	}
	if m.DueDate != "2026-07-01" {
		t.Errorf("DueDate: %q", m.DueDate)
	}
	if m.IssueStats == nil || m.IssueStats.Total != 5 || m.IssueStats.Closed != 2 {
		t.Errorf("IssueStats: %+v", m.IssueStats)
	}
}

// TestNewEvidenceOutputs verifies the evidence list skips nil entries,
// formats collected_at via timeToRFC3339, and returns nil for an empty
// input slice.
func TestNewEvidenceOutputs(t *testing.T) {
	if got := NewEvidenceOutputs(nil); got != nil {
		t.Errorf("nil slice must return nil, got %+v", got)
	}
	collected := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	got := NewEvidenceOutputs([]*gl.ReleaseEvidence{
		nil,
		{SHA: "abc", Filepath: "/p", CollectedAt: &collected},
		{SHA: "", Filepath: "", CollectedAt: nil},
	})
	if got == nil || len(got) != 2 {
		t.Fatalf("want two evidences (nil skipped), got %+v", got)
	}
	if got[0].SHA != "abc" || got[0].Filepath != "/p" || got[0].CollectedAt != "2026-06-01T12:00:00Z" {
		t.Errorf("evidence[0]: %+v", got[0])
	}
	if got[1].CollectedAt != "" {
		t.Errorf("evidence[1] should have empty CollectedAt for nil pointer, got %q", got[1].CollectedAt)
	}
}

// TestNewMilestoneIssueStatsOutput verifies the stats converter drops a nil
// pointer and only surfaces Total / Closed (the SDK has no Open field).
func TestNewMilestoneIssueStatsOutput(t *testing.T) {
	if got := NewMilestoneIssueStatsOutput(nil); got != nil {
		t.Errorf("nil stats must return nil, got %+v", got)
	}
	got := NewMilestoneIssueStatsOutput(&gl.ReleaseMilestoneIssueStats{Total: 9, Closed: 4})
	if got == nil || got.Total != 9 || got.Closed != 4 {
		t.Errorf("stats: %+v", got)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), `"open"`) {
		t.Errorf("stats JSON must not contain an open field (SDK has none): %s", raw)
	}
}

// TestNewAuthorOutputFromBasicUser verifies the release-author converter
// copies every field of a BasicUser verbatim.
func TestNewAuthorOutputFromBasicUser(t *testing.T) {
	got := NewAuthorOutputFromBasicUser(gl.BasicUser{
		ID: 9, Username: "admin", Name: "Admin", State: "active",
		AvatarURL: "https://a", WebURL: "https://w",
	})
	if got.ID != 9 || got.Username != "admin" || got.Name != "Admin" || got.State != "active" ||
		got.AvatarURL != "https://a" || got.WebURL != "https://w" {
		t.Errorf("author: %+v", got)
	}
}
