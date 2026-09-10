// sent_shapes_test.go covers the reader that takes off a captured response the
// twelve keys API::Entities::Ci::Pipeline adds to the basic entity, and the
// converter that places them on the list output.
package pipelines

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// pipelineCaptureBody is what GitLab answers a merge request pipeline creation
// with: API::Entities::Ci::Pipeline, the basic entity plus the twelve keys the
// SDK's PipelineInfo does not model. The user object carries the two keys
// lib/api/entities/user_basic.rb sends and the SDK's BasicUser lacks, so a
// shape that reused BasicUser fails here.
const pipelineCaptureBody = `{
	"id": 2,
	"iid": 7,
	"project_id": 4,
	"sha": "b83d6e391c22777fca1ed3012fce84f633d7fed0",
	"ref": "refs/merge-requests/1/head",
	"status": "pending",
	"source": "merge_request_event",
	"web_url": "http://e.com/user1/project1/pipelines/2",
	"created_at": "2026-09-04T19:20:18Z",
	"updated_at": "2026-09-04T19:20:19Z",
	"before_sha": "0000000000000000000000000000000000000000",
	"tag": true,
	"yaml_errors": "widgets:build: needs 'widgets:test'",
	"user": {"id": 1, "username": "user1", "public_email": "user1@e.com", "name": "John Doe1",
		"state": "active", "locked": false, "avatar_url": "http://e.com/a.png", "web_url": "http://e.com/user1"},
	"started_at": "2026-09-04T19:21:00Z",
	"finished_at": "2026-09-04T19:23:07Z",
	"committed_at": "2026-09-04T19:19:00Z",
	"duration": 127,
	"queued_duration": 63,
	"coverage": "98.29",
	"detailed_status": {
		"icon": "status_pending", "text": "pending", "label": "pending", "group": "pending",
		"tooltip": "pending", "has_details": true, "details_path": "/user1/project1/pipelines/2",
		"illustration": {"image": "illustrations/empty.svg"},
		"favicon": "/assets/ci_favicons/favicon_status_pending.png"
	},
	"archived": true
}`

// TestCapturedInfo_ReadsEveryKeyTheFullEntityAdds drives the reader over that
// body and checks each of the twelve keys arrives, including the two objects
// whose shapes are the ones GitLab's own entities render rather than the SDK's.
func TestCapturedInfo_ReadsEveryKeyTheFullEntityAdds(t *testing.T) {
	extra, err := capturedInfo(gitlabclient.CapturedBody([]byte(pipelineCaptureBody)))
	if err != nil {
		t.Fatalf("capturedInfo() error: %v", err)
	}

	scalars := map[string][2]any{
		"before_sha":      {extra.BeforeSHA, "0000000000000000000000000000000000000000"},
		"tag":             {extra.Tag, true},
		"yaml_errors":     {extra.YamlErrors, "widgets:build: needs 'widgets:test'"},
		"duration":        {extra.Duration, int64(127)},
		"queued_duration": {extra.QueuedDuration, int64(63)},
		"coverage":        {extra.Coverage, "98.29"},
		"archived":        {extra.Archived, true},
	}
	for name, pair := range scalars {
		t.Run(name, func(t *testing.T) {
			if pair[0] != pair[1] {
				t.Errorf("%s = %v, want %v", name, pair[0], pair[1])
			}
		})
	}

	t.Run("user carries the UserBasic keys the SDK's BasicUser lacks", func(t *testing.T) {
		if extra.User == nil || extra.User.PublicEmail != "user1@e.com" || extra.User.Locked {
			t.Errorf("user = %+v, want the public_email and locked keys", extra.User)
		}
	})
	t.Run("detailed_status is an object rather than a scalar", func(t *testing.T) {
		if extra.DetailedStatus == nil || extra.DetailedStatus.Group != "pending" ||
			!extra.DetailedStatus.HasDetails || extra.DetailedStatus.Illustration == nil {
			t.Errorf("detailed_status = %+v, want the DetailedStatusEntity keys", extra.DetailedStatus)
		}
	})
	t.Run("the three timestamps", func(t *testing.T) {
		if extra.StartedAt == nil || extra.FinishedAt == nil || extra.CommittedAt == nil {
			t.Errorf("started_at/finished_at/committed_at = %v/%v/%v", extra.StartedAt, extra.FinishedAt, extra.CommittedAt)
		}
	})
}

// TestCapturedInfo_AbsentKeysStayEmpty covers the other side of every one of
// the twelve: the two pipeline lists present the basic entity, which sends
// none of them, and the reader must invent nothing.
func TestCapturedInfo_AbsentKeysStayEmpty(t *testing.T) {
	extra, err := capturedInfo(gitlabclient.CapturedBody([]byte(`{"id": 2, "status": "success"}`)))
	if err != nil {
		t.Fatalf("capturedInfo() error: %v", err)
	}
	if extra.User != nil || extra.DetailedStatus != nil ||
		extra.StartedAt != nil || extra.FinishedAt != nil || extra.CommittedAt != nil {
		t.Errorf("capturedInfo() invented an object or a timestamp: %+v", extra)
	}
	if extra.BeforeSHA != "" || extra.Tag || extra.YamlErrors != "" || extra.Coverage != "" ||
		extra.Duration != 0 || extra.QueuedDuration != 0 || extra.Archived {
		t.Errorf("capturedInfo() invented a scalar: %+v", extra)
	}
}

// TestCapturedInfo_RefusesAnAnswerItCannotHold covers the reader's refusals.
func TestCapturedInfo_RefusesAnAnswerItCannotHold(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"a list where GitLab sends an object", `[{"id": 2}]`},
		{"a duration that is not a number", `{"duration": "quick"}`},
		{"detailed_status as a scalar", `{"detailed_status": "pending"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := capturedInfo(gitlabclient.CapturedBody([]byte(tc.body))); err == nil {
				t.Errorf("capturedInfo() accepted %s", tc.name)
			}
		})
	}
	if _, err := capturedInfo(&gitlabclient.ResponseCapture{}); err == nil {
		t.Error("capturedInfo() accepted a capture that saw no response")
	}
}

// TestCapturedOutput_PlacesTheCapturedKeysOnTheListShape covers the exported
// converter the merge request package calls: the SDK's own fields keep coming
// from the decode and the twelve arrive beside them, timestamps rendered the
// way the sibling created_at and updated_at are.
func TestCapturedOutput_PlacesTheCapturedKeysOnTheListShape(t *testing.T) {
	info := &gl.PipelineInfo{ID: 2, IID: 7, ProjectID: 4, Status: "pending", Ref: "refs/merge-requests/1/head"}
	out, err := CapturedOutput("mrCreatePipeline", info, gitlabclient.CapturedBody([]byte(pipelineCaptureBody)))
	if err != nil {
		t.Fatalf("CapturedOutput() error: %v", err)
	}
	if out.ID != 2 || out.IID != 7 || out.Status != "pending" {
		t.Errorf("CapturedOutput() lost an SDK field: %+v", out)
	}
	if out.StartedAt != "2026-09-04T19:21:00Z" || out.FinishedAt != "2026-09-04T19:23:07Z" ||
		out.CommittedAt != "2026-09-04T19:19:00Z" {
		t.Errorf("timestamps = %q/%q/%q, want RFC 3339", out.StartedAt, out.FinishedAt, out.CommittedAt)
	}
	if !out.Archived || !out.Tag || out.Duration != 127 || out.QueuedDuration != 63 || out.Coverage != "98.29" {
		t.Errorf("CapturedOutput() = %+v, want the captured scalars", out)
	}
	if out.User == nil || out.User.Username != "user1" || out.DetailedStatus == nil {
		t.Errorf("CapturedOutput() = %+v, want the two objects", out)
	}
}

// TestCapturedOutput_ReportsABodyItCannotHold verifies the converter names the
// operation rather than serving a pipeline with the keys silently missing.
func TestCapturedOutput_ReportsABodyItCannotHold(t *testing.T) {
	_, err := CapturedOutput("mrCreatePipeline", &gl.PipelineInfo{ID: 2},
		gitlabclient.CapturedBody([]byte(`{"duration": "quick"}`)))
	if err == nil {
		t.Fatal("CapturedOutput() accepted a body the reader cannot hold")
	}
}
