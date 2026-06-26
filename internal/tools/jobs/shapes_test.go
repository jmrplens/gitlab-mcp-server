// shapes_test.go exercises the client-go sub-object mirrors and list-option
// helpers added for the 1:1 audit (full nested objects on canonical keys).
package jobs

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestObjectConverters_NilAndEmpty verifies that every sub-object converter
// returns nil for a nil pointer or zero-valued input, exercising the guard
// branches that the success-path tests do not reach.
func TestObjectConverters_NilAndEmpty(t *testing.T) {
	if commitObject(nil) != nil {
		t.Error("commitObject(nil) != nil")
	}
	if pipelineObject(gl.JobPipeline{}) != nil {
		t.Error("pipelineObject(zero) != nil")
	}
	if pipelineInfoObject(nil) != nil {
		t.Error("pipelineInfoObject(nil) != nil")
	}
	if pipelineInfoValueObject(gl.PipelineInfo{}) != nil {
		t.Error("pipelineInfoValueObject(zero) != nil")
	}
	if runnerObject(gl.JobRunner{}) != nil {
		t.Error("runnerObject(zero) != nil")
	}
	if artifactObjects(nil) != nil {
		t.Error("artifactObjects(nil) != nil")
	}
	if artifactsFileObject(gl.JobArtifactsFile{}) != nil {
		t.Error("artifactsFileObject(zero) != nil")
	}
	if userObject(nil) != nil {
		t.Error("userObject(nil) != nil")
	}
	if projectObject(nil) != nil {
		t.Error("projectObject(nil) != nil")
	}
	if toolutil.FormatTimePtr(nil) != "" {
		t.Error("toolutil.FormatTimePtr(nil) != \"\"")
	}
}

// TestCommitObject_DocumentedFields verifies that commitObject surfaces the
// documented `commit` sub-object fields (id, short_id, title, author_name,
// author_email, created_at, message) and that fields not documented for the
// job `commit` shape are not retained on the trimmed output type.
func TestCommitObject_DocumentedFields(t *testing.T) {
	now := time.Now()
	out := commitObject(&gl.Commit{
		ID:          "deadbeef",
		ShortID:     "dead",
		Title:       "fix",
		AuthorName:  "Ada",
		AuthorEmail: "ada@example.com",
		CreatedAt:   &now,
		Message:     "fix things",
		// Fields below are not part of the documented job `commit` shape and
		// must not appear on the trimmed CommitObject.
		CommitterName: "Ada",
		ProjectID:     7,
		WebURL:        "https://gitlab.example.com/commit",
	})
	if out == nil {
		t.Fatal("commitObject returned nil for populated commit")
	}
	if out.ID != "deadbeef" || out.ShortID != "dead" || out.Title != "fix" {
		t.Errorf("identity fields = %+v, want deadbeef/dead/fix", out)
	}
	if out.AuthorName != "Ada" || out.AuthorEmail != "ada@example.com" {
		t.Errorf("author fields = %+v, want Ada/ada@example.com", out)
	}
	if out.Message != "fix things" {
		t.Errorf("Message = %q, want %q", out.Message, "fix things")
	}
	if out.CreatedAt == "" {
		t.Error("expected created_at to be formatted")
	}
}

// TestPipelineInfoObject_FullFields verifies pipelineInfoObject formats the
// optional updated_at/created_at timestamps.
func TestPipelineInfoObject_FullFields(t *testing.T) {
	now := time.Now()
	out := pipelineInfoObject(&gl.PipelineInfo{ID: 9, UpdatedAt: &now, CreatedAt: &now})
	if out == nil {
		t.Fatal("pipelineInfoObject returned nil")
	}
	if out.UpdatedAt == "" || out.CreatedAt == "" {
		t.Error("expected pipeline info timestamps to be formatted")
	}
}

// TestArtifactObjects_FullFields verifies artifactObjects maps every entry of a
// job's artifacts array.
func TestArtifactObjects_FullFields(t *testing.T) {
	out := artifactObjects([]gl.JobArtifact{
		{FileType: "archive", Filename: "artifacts.zip", Size: 10, FileFormat: "zip"},
	})
	if len(out) != 1 || out[0].Filename != "artifacts.zip" || out[0].Size != 10 {
		t.Errorf("artifactObjects = %+v, want one zip entry", out)
	}
}

// TestArtifactsFileObject_FullFields verifies artifactsFileObject maps a
// populated artifacts_file object.
func TestArtifactsFileObject_FullFields(t *testing.T) {
	out := artifactsFileObject(gl.JobArtifactsFile{Filename: "a.zip", Size: 5})
	if out == nil || out.Filename != "a.zip" || out.Size != 5 {
		t.Errorf("artifactsFileObject = %+v, want a.zip/5", out)
	}
}

// TestProjectObject_DocumentedField verifies projectObject surfaces the single
// documented `project` sub-object field (ci_job_token_scope_enabled).
func TestProjectObject_DocumentedField(t *testing.T) {
	out := projectObject(&gl.Project{ID: 1, Name: "demo", CIJobTokenScopeEnabled: true})
	if out == nil || !out.CIJobTokenScopeEnabled {
		t.Errorf("projectObject = %+v, want ci_job_token_scope_enabled true", out)
	}
}

// TestUserObject_DocumentedFields verifies userObject surfaces the documented
// `user` sub-object fields, including the extended profile fields.
func TestUserObject_DocumentedFields(t *testing.T) {
	now := time.Now()
	out := userObject(&gl.User{
		ID: 2, Name: "Ada", Username: "ada", State: "active",
		AvatarURL: "http://a", WebURL: "http://w", CreatedAt: &now,
		Bio: "b", Location: "loc", PublicEmail: "e@x", Linkedin: "li",
		Twitter: "tw", WebsiteURL: "site", Organization: "org",
	})
	if out == nil {
		t.Fatal("userObject returned nil")
	}
	if out.ID != 2 || out.Username != "ada" || out.Name != "Ada" || out.State != "active" {
		t.Errorf("identity fields = %+v", out)
	}
	if out.CreatedAt == "" || out.Bio != "b" || out.Location != "loc" ||
		out.PublicEmail != "e@x" || out.Linkedin != "li" || out.Twitter != "tw" ||
		out.WebsiteURL != "site" || out.Organization != "org" {
		t.Errorf("extended profile fields = %+v", out)
	}
}

// TestPipelineInfoObject_TrimmedFields verifies pipelineInfoObject drops the
// non-documented iid/source/name fields while keeping the documented bridge
// pipeline fields.
func TestPipelineInfoObject_TrimmedFields(t *testing.T) {
	out := pipelineInfoObject(&gl.PipelineInfo{
		ID: 5, ProjectID: 1, Status: "pending", Ref: "main",
		SHA: "abc", WebURL: "http://p", IID: 9, Source: "push", Name: "n",
	})
	if out == nil {
		t.Fatal("pipelineInfoObject returned nil")
	}
	if out.ID != 5 || out.ProjectID != 1 || out.Status != "pending" ||
		out.Ref != "main" || out.SHA != "abc" || out.WebURL != "http://p" {
		t.Errorf("documented fields = %+v", out)
	}
}

// TestApplyListOpts_OrderAndSort verifies applyListOpts copies order_by, sort,
// offset, and keyset parameters onto the embedded gl.ListOptions.
func TestApplyListOpts_OrderAndSort(t *testing.T) {
	opts := &gl.ListJobsOptions{}
	applyListOpts(opts,
		toolutil.PaginationInput{Page: 2, PerPage: 50},
		toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "tok"},
		"id", "desc",
	)
	if opts.OrderBy != "id" || opts.Sort != "desc" {
		t.Errorf("OrderBy/Sort = %q/%q, want id/desc", opts.OrderBy, opts.Sort)
	}
	if opts.Page != 2 || opts.PerPage != 50 {
		t.Errorf("Page/PerPage = %d/%d, want 2/50", opts.Page, opts.PerPage)
	}
	if opts.Pagination != "keyset" || opts.PageToken != "tok" {
		t.Errorf("Pagination/PageToken = %q/%q, want keyset/tok", opts.Pagination, opts.PageToken)
	}
}

// TestApplyListOpts_EmptyKeepsZero verifies applyListOpts leaves order_by and
// sort empty when not supplied.
func TestApplyListOpts_EmptyKeepsZero(t *testing.T) {
	opts := &gl.ListJobsOptions{}
	applyListOpts(opts, toolutil.PaginationInput{}, toolutil.KeysetPaginationInput{}, "", "")
	if opts.OrderBy != "" || opts.Sort != "" {
		t.Errorf("OrderBy/Sort = %q/%q, want empty", opts.OrderBy, opts.Sort)
	}
}

// TestApplyScope_EmptyAndPopulated verifies applyScope leaves Scope nil for an
// empty filter and maps status strings to BuildStateValue otherwise.
func TestApplyScope_EmptyAndPopulated(t *testing.T) {
	empty := &gl.ListJobsOptions{}
	applyScope(empty, nil)
	if empty.Scope != nil {
		t.Error("applyScope(nil) set Scope")
	}

	populated := &gl.ListJobsOptions{}
	applyScope(populated, []string{"failed", "success"})
	if populated.Scope == nil || len(*populated.Scope) != 2 {
		t.Fatalf("Scope = %v, want 2 entries", populated.Scope)
	}
	if (*populated.Scope)[0] != gl.BuildStateValue("failed") {
		t.Errorf("Scope[0] = %q, want failed", (*populated.Scope)[0])
	}
}
