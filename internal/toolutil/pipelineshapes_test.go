// pipelineshapes_test.go contains unit tests for the shared pipeline-shape
// converters exposed by toolutil. The previous per-package tests were
// retained by deleting the local shapes.go definitions; this file replaces
// the shared unit-test surface so future regressions in any consumer
// (mergerequests, mergetrains, deploymentmergerequests) are caught from
// the canonical home.
package toolutil

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestNewBasicUserOutput pins the basic-user conversion (nil-on-nil + full
// field coverage when populated).
func TestNewBasicUserOutput(t *testing.T) {
	if got := NewBasicUserOutput(nil); got != nil {
		t.Errorf("nil basic user must return nil, got %+v", got)
	}
	got := NewBasicUserOutput(&gl.BasicUser{
		ID: 7, Username: "u", Name: "U", State: "active",
		AvatarURL: "av", WebURL: "w", CreatedAt: timePtr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	})
	if got == nil || got.ID != 7 || got.Username != "u" || got.Name != "U" || got.State != "active" ||
		got.AvatarURL != "av" || got.WebURL != "w" || got.CreatedAt == "" {
		t.Errorf("populated basic user: %+v", got)
	}
}

// TestNewBasicUserOutputs pins the slice converter (skip-nil + nil-on-empty).
func TestNewBasicUserOutputs(t *testing.T) {
	if got := NewBasicUserOutputs(nil); got != nil {
		t.Errorf("nil slice must return nil, got %+v", got)
	}
	if got := NewBasicUserOutputs([]*gl.BasicUser{}); got != nil {
		t.Errorf("empty slice must return nil, got %+v", got)
	}
	got := NewBasicUserOutputs([]*gl.BasicUser{
		nil,
		{ID: 1, Username: "a"},
		nil,
		{ID: 2, Username: "b"},
	})
	if got == nil || len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("slice with nil elements: %+v", got)
	}
}

// TestNewPipelineDetailedStatusOutput pins the detailed-status conversion
// including the nested illustration image handling.
func TestNewPipelineDetailedStatusOutput(t *testing.T) {
	if got := NewPipelineDetailedStatusOutput(nil); got != nil {
		t.Errorf("nil detailed status must return nil, got %+v", got)
	}
	got := NewPipelineDetailedStatusOutput(&gl.DetailedStatus{
		Icon: "i", Text: "t", Label: "l", Group: "g",
		Tooltip: "tip", HasDetails: true, DetailsPath: "/p", Favicon: "f",
		Illustration: gl.DetailedStatusIllustration{Image: "img.png"},
	})
	if got == nil || got.Icon != "i" || got.Label != "l" || got.Group != "g" ||
		got.Illustration == nil || got.Illustration.Image != "img.png" {
		t.Errorf("populated detailed status: %+v", got)
	}
	// Empty illustration image must NOT surface a nested illustration object.
	got = NewPipelineDetailedStatusOutput(&gl.DetailedStatus{Label: "x"})
	if got == nil || got.Illustration != nil {
		t.Errorf("empty illustration: %+v", got)
	}
}

// TestNewPipelineOutput pins the pipeline conversion (nil-on-nil + full field
// coverage when populated, including the typed PipelineSource).
func TestNewPipelineOutput(t *testing.T) {
	if got := NewPipelineOutput(nil); got != nil {
		t.Errorf("nil pipeline must return nil, got %+v", got)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := NewPipelineOutput(&gl.Pipeline{
		ID: 200, IID: 3, ProjectID: 1, Status: "success",
		Source: "push", Ref: "main", Name: "Build", SHA: "deadbeef", BeforeSHA: "cafe",
		Tag: false, YamlErrors: "none",
		User: &gl.BasicUser{
			ID: 1, Username: "admin", Name: "Admin",
			State: "active", AvatarURL: "https://gl/a.png", WebURL: "https://gl/admin",
			CreatedAt: &now,
		},
		UpdatedAt:      &now, CreatedAt: &now, StartedAt: &now, FinishedAt: &now, CommittedAt: &now,
		Duration: 60, QueuedDuration: 5, Coverage: "90", WebURL: "https://gl/pipe/200",
		DetailedStatus: &gl.DetailedStatus{Label: "passed", Illustration: gl.DetailedStatusIllustration{Image: "img.png"}},
	})
	if got == nil {
		t.Fatal("populated pipeline returned nil")
	}
	if got.ID != 200 || got.IID != 3 || got.Source != "push" || got.SHA != "deadbeef" {
		t.Errorf("pipeline scalar mismatch: %+v", got)
	}
	if got.User == nil || got.User.Username != "admin" || got.User.CreatedAt == "" {
		t.Errorf("pipeline.user mismatch: %+v", got.User)
	}
	if got.UpdatedAt == "" || got.CreatedAt == "" || got.StartedAt == "" ||
		got.FinishedAt == "" || got.CommittedAt == "" {
		t.Errorf("pipeline timestamp fields should be formatted RFC 3339: %+v", got)
	}
	if got.DetailedStatus == nil || got.DetailedStatus.Illustration == nil ||
		got.DetailedStatus.Illustration.Image != "img.png" {
		t.Errorf("pipeline.detailed_status.illustration: %+v", got.DetailedStatus)
	}
	if got.Coverage != "90" || got.WebURL != "https://gl/pipe/200" {
		t.Errorf("pipeline scalars mismatch: %+v", got)
	}
}

// TestPipelineOutputJSONTags pins the on-wire JSON keys. Any change to the
// tag set is a breaking change for MCP consumers. PipelineOutput uses
// `omitempty` only on pointer / time fields; scalar fields (id, iid, status,
// source, ref, name, sha, before_sha, duration, queued_duration, coverage,
// tag, yaml_errors, web_url) are always serialized. Pointer fields
// (User, DetailedStatus) and string-form timestamps use omitempty.
func TestPipelineOutputJSONTags(t *testing.T) {
	raw, err := json.Marshal(&PipelineOutput{
		ID: 1, IID: 1, ProjectID: 1, Status: "success",
		Source: "push", Ref: "main", Name: "Build", SHA: "x", BeforeSHA: "y",
		WebURL: "w",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{
		`"id":1`, `"iid":1`, `"project_id":1`, `"status":"success"`,
		`"source":"push"`, `"ref":"main"`, `"web_url":"w"`,
		`"name":"Build"`, `"sha":"x"`, `"before_sha":"y"`,
		`"tag":false`, `"duration":0`, `"queued_duration":0`,
		`"coverage":""`, `"yaml_errors":""`,
	} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("PipelineOutput JSON missing %q: %s", key, raw)
		}
	}
	// Pointer + time fields use omitempty and must NOT appear when empty.
	for _, key := range []string{`"user"`, `"updated_at"`, `"created_at"`, `"started_at"`, `"finished_at"`, `"committed_at"`, `"detailed_status"`} {
		if strings.Contains(string(raw), key) {
			t.Errorf("PipelineOutput JSON should omit empty %q: %s", key, raw)
		}
	}
}

// timePtr is a tiny helper for the tests above.
func timePtr(t time.Time) *time.Time { return &t }
