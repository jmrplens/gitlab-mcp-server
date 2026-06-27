// events_mirror_test.go covers the full client-go object mirrors (push data,
// notes, authors, project event data with repository and commits) and the
// keyset/order_by pagination wiring added for the 1:1 audit.
package events

import (
	"net/http"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestToContributionEventOutput_FullMirror verifies that every nested sub-object
// of a ContributionEvent (push_data, note with author/position/resolved_by, and
// the author BasicUser) is mirrored onto the output.
func TestToContributionEventOutput_FullMirror(t *testing.T) {
	ts := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	noteType := gl.NoteTypeValue("DiffNote")
	e := &gl.ContributionEvent{
		ID:        1,
		ProjectID: 2,
		CreatedAt: &ts,
		PushData: gl.ContributionEventPushData{
			CommitCount: 3, Action: "pushed", RefType: "branch",
			CommitFrom: "aaa", CommitTo: "bbb", Ref: "main", CommitTitle: "fix",
		},
		Note: &gl.Note{
			ID: 10, Type: noteType, Body: "hello", Title: "t", FileName: "f.go",
			Author:    gl.NoteAuthor{ID: 5, Username: "u", Email: "u@e", Name: "User", State: "active", AvatarURL: "a", WebURL: "w"},
			System:    true,
			CreatedAt: &ts, UpdatedAt: &ts, ExpiresAt: &ts, ResolvedAt: &ts,
			CommitID:   "c1",
			Resolvable: true, Resolved: true,
			ResolvedBy:   gl.NoteResolvedBy{ID: 6, Username: "r"},
			Internal:     true,
			Confidential: true,
			Position: &gl.NotePosition{
				BaseSHA: "b", StartSHA: "s", HeadSHA: "h", PositionType: "text",
				NewPath: "new.go", NewLine: 12, OldPath: "old.go", OldLine: 8,
				LineRange: &gl.LineRange{
					StartRange: &gl.LinePosition{LineCode: "lc1", Type: "new", OldLine: 1, NewLine: 2},
					EndRange:   &gl.LinePosition{LineCode: "lc2", Type: "old", OldLine: 3, NewLine: 4},
				},
			},
		},
		Author: gl.BasicUser{ID: 5, Username: "u", Name: "User", State: "active", CreatedAt: &ts, AvatarURL: "a", WebURL: "w"},
	}

	out := toContributionEventOutput(e)
	assertTrue(t, out.PushData != nil && out.PushData.CommitCount == 3 && out.PushData.CommitTitle == "fix", "push_data")
	assertTrue(t, out.Author != nil && out.Author.ID == 5 && out.Author.CreatedAt != "", "author")
	assertContributionNote(t, out.Note)
}

// assertContributionNote validates the deeply nested note mirror.
func assertContributionNote(t *testing.T, n *NoteOutput) {
	t.Helper()
	assertTrue(t, n != nil && n.ID == 10 && n.Type == "DiffNote" && n.Internal, "note core")
	assertTrue(t, n.Author != nil && n.Author.Email == "u@e", "note author")
	assertTrue(t, n.ResolvedBy != nil && n.ResolvedBy.ID == 6, "note resolved_by")
	assertTrue(t, n.CreatedAt != "" && n.UpdatedAt != "" && n.ExpiresAt != "" && n.ResolvedAt != "", "note timestamps")
	p := n.Position
	assertTrue(t, p != nil && p.NewLine == 12 && p.OldPath == "old.go", "note position")
	assertTrue(t, p.LineRange != nil && p.LineRange.StartRange != nil && p.LineRange.StartRange.LineCode == "lc1", "line range start")
	assertTrue(t, p.LineRange.EndRange != nil && p.LineRange.EndRange.NewLine == 4, "line range end")
}

// assertTrue fails the test with a labeled message when cond is false.
func assertTrue(t *testing.T, cond bool, label string) {
	t.Helper()
	if !cond {
		t.Fatalf("%s not mirrored correctly", label)
	}
}

// TestToContributionEventOutput_EmptySubObjects verifies that zero-valued sub
// objects are omitted (nil) so the output stays clean.
func TestToContributionEventOutput_EmptySubObjects(t *testing.T) {
	out := toContributionEventOutput(&gl.ContributionEvent{ID: 1})
	if out.PushData != nil {
		t.Errorf("expected nil push_data, got %+v", out.PushData)
	}
	if out.Note != nil {
		t.Errorf("expected nil note, got %+v", out.Note)
	}
	if out.Author != nil {
		t.Errorf("expected nil author, got %+v", out.Author)
	}
}

// TestToBasicUserOutput_NilCreatedAt verifies the BasicUser mirror handles a nil
// created timestamp.
func TestToBasicUserOutput_NilCreatedAt(t *testing.T) {
	out := toBasicUserOutput(gl.BasicUser{ID: 7, Username: "x"})
	if out == nil || out.ID != 7 || out.CreatedAt != "" {
		t.Fatalf("unexpected user output: %+v", out)
	}
}

// TestNoteAuthorOutput_Empty verifies the shared note-author mirror returns
// nil for zero-valued authors and a populated value otherwise.
func TestNoteAuthorOutput_Empty(t *testing.T) {
	if noteAuthorOutput(0, "", "", "", "", "", "") != nil {
		t.Error("expected nil for empty author fields")
	}
	if got := noteAuthorOutput(1, "u", "", "", "", "", ""); got == nil || got.ID != 1 {
		t.Errorf("expected populated author, got %+v", got)
	}
}

// TestNilSubObjectMirrors verifies the nil-input branches of the nested
// position/line-range converters and the project event note author.
func TestNilSubObjectMirrors(t *testing.T) {
	if toNotePositionOutput(nil) != nil {
		t.Error("expected nil note position")
	}
	if toLineRangeOutput(nil) != nil {
		t.Error("expected nil line range")
	}
	if toLinePositionOutput(nil) != nil {
		t.Error("expected nil line position")
	}
	if toProjectEventNoteAuthorOutput(gl.ProjectEventNoteAuthor{}) != nil {
		t.Error("expected nil project event note author")
	}
	// LineRange present but with nil endpoints exercises the start/end nil paths.
	r := toLineRangeOutput(&gl.LineRange{})
	if r == nil || r.StartRange != nil || r.EndRange != nil {
		t.Errorf("expected non-nil line range with nil endpoints, got %+v", r)
	}
}

// TestToProjectEventOutput_FullMirror verifies that ProjectEvent push_data,
// note (with author), and data (ref, repository, commits with stats and
// pipeline) are all mirrored.
func TestToProjectEventOutput_FullMirror(t *testing.T) {
	ts := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	status := gl.BuildStateValue("success")
	e := &gl.ProjectEvent{
		ID: 1, ProjectID: 2, CreatedAt: "2026-03-07T12:00:00Z",
		Author: gl.BasicUser{ID: 5, Username: "u"},
		PushData: gl.ProjectEventPushData{
			CommitCount: 2, Action: "pushed", RefType: "branch",
			CommitFrom: "a", CommitTo: "b", Ref: "main", CommitTitle: "ct",
		},
		Note: gl.ProjectEventNote{
			ID: 9, Body: "body", Attachment: "att", System: true,
			NoteableID: 3, NoteableType: "Issue", NoteableIID: 4, CreatedAt: &ts,
			Author: gl.ProjectEventNoteAuthor{ID: 6, Username: "na", Email: "n@e", Name: "N", State: "active", AvatarURL: "av", WebURL: "wu"},
		},
		Data: gl.ProjectEventData{
			Before: "x", After: "y", Ref: "refs/heads/main", UserID: 8, UserName: "dev",
			TotalCommitsCount: 1,
			Repository: &gl.Repository{
				Name: "repo", Description: "d", WebURL: "wu", AvatarURL: "av",
				GitSSHURL: "gs", GitHTTPURL: "gh", Namespace: "ns", Visibility: gl.PublicVisibility,
				PathWithNamespace: "ns/repo", DefaultBranch: "main", Homepage: "hp", URL: "u", SSHURL: "ssh", HTTPURL: "http",
			},
			Commits: []*gl.Commit{
				nil,
				{
					ID: "c1", ShortID: "c", Title: "t", AuthorName: "an", AuthorEmail: "ae",
					AuthoredDate: &ts, CommitterName: "cn", CommitterEmail: "ce", CommittedDate: &ts, CreatedAt: &ts,
					Message: "m", ParentIDs: []string{"p"}, ProjectID: 2, WebURL: "wu",
					Trailers: map[string]string{"k": "v"}, ExtendedTrailers: map[string]string{"k2": "v2"},
					Stats:        &gl.CommitStats{Additions: 1, Deletions: 2, Total: 3},
					Status:       &status,
					LastPipeline: &gl.PipelineInfo{ID: 100, IID: 1, ProjectID: 2, Status: "success", Source: "push", Ref: "main", SHA: "sha", Name: "p", WebURL: "wu", UpdatedAt: &ts, CreatedAt: &ts},
				},
			},
		},
	}

	out := toProjectEventOutput(e)
	assertTrue(t, out.Author != nil && out.Author.ID == 5, "author")
	assertTrue(t, out.PushData != nil && out.PushData.CommitTitle == "ct", "push_data")
	assertTrue(t, out.Note != nil && out.Note.NoteableType == "Issue" && out.Note.Author != nil && out.Note.Author.Email == "n@e" && out.Note.CreatedAt != "", "note")
	assertProjectData(t, out.Data)
}

// assertProjectData validates the project event data mirror, including the
// repository and the nested commit with stats and pipeline.
func assertProjectData(t *testing.T, d *ProjectEventDataOutput) {
	t.Helper()
	assertTrue(t, d != nil && d.Ref == "refs/heads/main" && d.UserName == "dev" && d.TotalCommitsCount == 1, "data")
	assertTrue(t, d.Repository != nil && d.Repository.PathWithNamespace == "ns/repo" && d.Repository.Visibility == "public", "repository")
	assertTrue(t, len(d.Commits) == 1, "commit count (nil skipped)")
	c := d.Commits[0]
	assertTrue(t, c.ID == "c1" && c.Status == "success" && c.AuthoredDate != "" && c.CommittedDate != "" && c.CreatedAt != "", "commit")
	assertTrue(t, c.Stats != nil && c.Stats.Total == 3, "commit stats")
	assertTrue(t, c.LastPipeline != nil && c.LastPipeline.ID == 100 && c.LastPipeline.UpdatedAt != "" && c.LastPipeline.CreatedAt != "", "commit last_pipeline")
	assertTrue(t, c.Trailers["k"] == "v" && c.ExtendedTrailers["k2"] == "v2", "commit trailers")
}

// TestToProjectEventOutput_EmptySubObjects verifies zero-valued ProjectEvent sub
// objects are omitted.
func TestToProjectEventOutput_EmptySubObjects(t *testing.T) {
	out := toProjectEventOutput(&gl.ProjectEvent{ID: 1})
	if out.PushData != nil || out.Note != nil || out.Data != nil || out.Author != nil {
		t.Errorf("expected nil sub-objects, got %+v", out)
	}
}

// TestToProjectEventDataOutput_MinimalCommit covers a commit
// with nil stats/pipeline and timestamps, and a data block with nil repository.
func TestToProjectEventDataOutput_MinimalCommit(t *testing.T) {
	d := gl.ProjectEventData{
		Ref:     "main",
		Commits: []*gl.Commit{{ID: "c1"}},
	}
	out := toProjectEventDataOutput(d)
	if out == nil || out.Repository != nil || len(out.Commits) != 1 {
		t.Fatalf("unexpected data output: %+v", out)
	}
	c := out.Commits[0]
	if c.Stats != nil || c.LastPipeline != nil || c.AuthoredDate != "" || c.Status != "" {
		t.Fatalf("expected minimal commit, got %+v", c)
	}
}

// TestListProjectEvents_KeysetAndOrderBy verifies that keyset pagination and
// order_by parameters are forwarded as query parameters.
func TestListProjectEvents_KeysetAndOrderBy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pagination") != "keyset" {
			t.Errorf("expected pagination=keyset, got %q", q.Get("pagination"))
		}
		if q.Get("page_token") != "99" {
			t.Errorf("expected page_token=99, got %q", q.Get("page_token"))
		}
		if q.Get("order_by") != "id" {
			t.Errorf("expected order_by=id, got %q", q.Get("order_by"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListProjectEvents(t.Context(), client, ListProjectEventsInput{
		ProjectID:             "42",
		OrderBy:               "id",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "99"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestListCurrentUserContributionEvents_KeysetAndOrderBy verifies keyset and
// order_by forwarding for the contribution-events endpoint.
func TestListCurrentUserContributionEvents_KeysetAndOrderBy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pagination") != "keyset" {
			t.Errorf("expected pagination=keyset, got %q", q.Get("pagination"))
		}
		if q.Get("page_token") != "7" {
			t.Errorf("expected page_token=7, got %q", q.Get("page_token"))
		}
		if q.Get("order_by") != "id" {
			t.Errorf("expected order_by=id, got %q", q.Get("order_by"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListCurrentUserContributionEvents(t.Context(), client, ListContributionEventsInput{
		OrderBy:               "id",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "7"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUserActionSpecs_Descriptions verifies the R-META individual-tool
// descriptions follow the "Returns: … See also: …" form.
func TestUserActionSpecs_Descriptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	for _, spec := range UserActionSpecs(client) {
		desc := spec.IndividualTool.Description
		if desc == "" {
			t.Fatalf("%s: empty description", spec.IndividualTool.Name)
		}
		if !contains(desc, "Returns:") || !contains(desc, "See also:") {
			t.Errorf("%s: description missing Returns/See also: %q", spec.IndividualTool.Name, desc)
		}
	}
}
