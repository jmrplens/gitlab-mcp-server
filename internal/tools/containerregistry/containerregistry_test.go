// containerregistry_test.go contains unit tests for the container registry MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package containerregistry

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// fmtExpectedDELETE identifies the fmt expected delete constant used by this package.
	fmtExpectedDELETE = "expected DELETE, got %s"
)

// ---------------------------------------------------------------------------
// ListProject
// ---------------------------------------------------------------------------.

// TestListProject_Success verifies ListProject when success.
func TestListProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/repositories", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":1,"name":"my-image","path":"group/project/my-image","project_id":10,"location":"registry.example.com/group/project/my-image","tags_count":5}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: toolutil.StringOrInt("10")})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Repositories) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(out.Repositories))
	}
	if out.Repositories[0].Name != "my-image" {
		t.Errorf("expected name my-image, got %s", out.Repositories[0].Name)
	}
	if out.Repositories[0].Path != "group/project/my-image" {
		t.Errorf("Path = %q, want %q", out.Repositories[0].Path, "group/project/my-image")
	}
	if out.Repositories[0].ProjectID != 10 {
		t.Errorf("ProjectID = %d, want 10", out.Repositories[0].ProjectID)
	}
	if out.Repositories[0].Location != "registry.example.com/group/project/my-image" {
		t.Errorf("Location = %q, want %q", out.Repositories[0].Location, "registry.example.com/group/project/my-image")
	}
	if out.Repositories[0].TagsCount != 5 {
		t.Errorf("TagsCount = %d, want 5", out.Repositories[0].TagsCount)
	}
}

// TestListProject_MissingProjectID verifies ListProject when missing project ID.
func TestListProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListProject(context.Background(), client, ListProjectInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// ListGroup
// ---------------------------------------------------------------------------.

// TestListGroup_Success verifies ListGroup when success.
func TestListGroup_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/registry/repositories", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":2,"name":"group-img","path":"group/group-img","project_id":10,"location":"registry.example.com/group/group-img","tags_count":2}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: toolutil.StringOrInt("5")})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Repositories) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(out.Repositories))
	}
	if out.Repositories[0].Name != "group-img" {
		t.Errorf("Name = %q, want %q", out.Repositories[0].Name, "group-img")
	}
	if out.Repositories[0].Path != "group/group-img" {
		t.Errorf("Path = %q, want %q", out.Repositories[0].Path, "group/group-img")
	}
	if out.Repositories[0].TagsCount != 2 {
		t.Errorf("TagsCount = %d, want 2", out.Repositories[0].TagsCount)
	}
}

// TestListGroup_MissingGroupID verifies ListGroup when missing group ID.
func TestListGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListGroup(context.Background(), client, ListGroupInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetRepository
// ---------------------------------------------------------------------------.

// TestGetRepository_Success verifies GetRepository when success.
func TestGetRepository_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/registry/repositories/1", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":1,"name":"my-image","path":"group/project/my-image","project_id":10,"location":"registry.example.com/group/project/my-image","tags_count":5}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetRepository(context.Background(), client, GetRepositoryInput{RepositoryID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
	if out.Name != "my-image" {
		t.Errorf("Name = %q, want %q", out.Name, "my-image")
	}
	if out.Path != "group/project/my-image" {
		t.Errorf("Path = %q, want %q", out.Path, "group/project/my-image")
	}
	if out.ProjectID != 10 {
		t.Errorf("ProjectID = %d, want 10", out.ProjectID)
	}
	if out.Location != "registry.example.com/group/project/my-image" {
		t.Errorf("Location = %q, want %q", out.Location, "registry.example.com/group/project/my-image")
	}
	if out.TagsCount != 5 {
		t.Errorf("TagsCount = %d, want 5", out.TagsCount)
	}
}

// TestGetRepository_MissingID verifies GetRepository when missing ID.
func TestGetRepository_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetRepository(context.Background(), client, GetRepositoryInput{})
	if err == nil || !strings.Contains(err.Error(), errRepoIDRequired) {
		t.Fatalf(fmtExpectedRepoIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// DeleteRepository
// ---------------------------------------------------------------------------.

// TestDeleteRepository_Success verifies DeleteRepository when success.
func TestDeleteRepository_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/repositories/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf(fmtExpectedDELETE, r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteRepository(context.Background(), client, DeleteRepositoryInput{
		ProjectID: toolutil.StringOrInt("10"), RepositoryID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteRepository_MissingRepoID verifies DeleteRepository when missing repo ID.
func TestDeleteRepository_MissingRepoID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteRepository(context.Background(), client, DeleteRepositoryInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err == nil || !strings.Contains(err.Error(), errRepoIDRequired) {
		t.Fatalf(fmtExpectedRepoIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// ListTags
// ---------------------------------------------------------------------------.

// TestListTags_Success verifies ListTags when success.
func TestListTags_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/repositories/1/tags", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"name":"latest","path":"group/project/my-image:latest","location":"registry.example.com/group/project/my-image:latest","total_size":1024}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListTags(context.Background(), client, ListTagsInput{
		ProjectID: toolutil.StringOrInt("10"), RepositoryID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(out.Tags))
	}
	if out.Tags[0].Name != "latest" {
		t.Errorf("expected name latest, got %s", out.Tags[0].Name)
	}
	if out.Tags[0].Path != "group/project/my-image:latest" {
		t.Errorf("Path = %q, want %q", out.Tags[0].Path, "group/project/my-image:latest")
	}
	if out.Tags[0].Location != "registry.example.com/group/project/my-image:latest" {
		t.Errorf("Location = %q, want %q", out.Tags[0].Location, "registry.example.com/group/project/my-image:latest")
	}
	if out.Tags[0].TotalSize != 1024 {
		t.Errorf("TotalSize = %d, want 1024", out.Tags[0].TotalSize)
	}
}

// TestListTags_MissingRepoID verifies ListTags when missing repo ID.
func TestListTags_MissingRepoID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListTags(context.Background(), client, ListTagsInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err == nil || !strings.Contains(err.Error(), errRepoIDRequired) {
		t.Fatalf(fmtExpectedRepoIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// GetTag
// ---------------------------------------------------------------------------.

// TestGetTag_Success verifies GetTag when success.
func TestGetTag_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/repositories/1/tags/latest", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"name":"latest","path":"group/project/my-image:latest","location":"registry.example.com","digest":"sha256:abc123","total_size":2048}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetTag(context.Background(), client, GetTagInput{
		ProjectID: toolutil.StringOrInt("10"), RepositoryID: 1, TagName: "latest",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Digest != "sha256:abc123" {
		t.Errorf("expected digest sha256:abc123, got %s", out.Digest)
	}
	if out.Name != "latest" {
		t.Errorf("Name = %q, want %q", out.Name, "latest")
	}
	if out.TotalSize != 2048 {
		t.Errorf("TotalSize = %d, want 2048", out.TotalSize)
	}
	if out.Path != "group/project/my-image:latest" {
		t.Errorf("Path = %q, want %q", out.Path, "group/project/my-image:latest")
	}
}

// TestGetTag_MissingTagName verifies GetTag when missing tag name.
func TestGetTag_MissingTagName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetTag(context.Background(), client, GetTagInput{
		ProjectID: toolutil.StringOrInt("10"), RepositoryID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "tag_name is required") {
		t.Fatalf("expected tag_name required error, got %v", err)
	}
}

// TestListTags_MultiPage verifies pagination metadata is correctly returned
// when the API indicates multiple pages of tags exist.
func TestListTags_MultiPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/repositories/1/tags", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"name":"v1","path":"p:v1","location":"loc","total_size":512}]`,
			testutil.PaginationHeaders{TotalPages: "3", Total: "25", Page: "1", PerPage: "10", NextPage: "2"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListTags(context.Background(), client, ListTagsInput{
		ProjectID: toolutil.StringOrInt("10"), RepositoryID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.TotalItems != 25 {
		t.Errorf("TotalItems = %d, want 25", out.Pagination.TotalItems)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("NextPage = %d, want 2", out.Pagination.NextPage)
	}
}

// ---------------------------------------------------------------------------
// DeleteTag
// ---------------------------------------------------------------------------.

// TestDeleteTag_Success verifies DeleteTag when success.
func TestDeleteTag_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/repositories/1/tags/old", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf(fmtExpectedDELETE, r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteTag(context.Background(), client, DeleteTagInput{
		ProjectID: toolutil.StringOrInt("10"), RepositoryID: 1, TagName: "old",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteTag_MissingTagName verifies DeleteTag when missing tag name.
func TestDeleteTag_MissingTagName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteTag(context.Background(), client, DeleteTagInput{
		ProjectID: toolutil.StringOrInt("10"), RepositoryID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "tag_name is required") {
		t.Fatalf("expected tag_name required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteTagsBulk
// ---------------------------------------------------------------------------.

// TestDeleteTagsBulk_Success verifies DeleteTagsBulk when success.
func TestDeleteTagsBulk_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/repositories/1/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf(fmtExpectedDELETE, r.Method)
		}
		q := r.URL.Query()
		if q.Get("name_regex_delete") != "v.*" {
			t.Errorf("name_regex_delete = %q, want %q", q.Get("name_regex_delete"), "v.*")
		}
		if q.Get("keep_n") != "2" {
			t.Errorf("keep_n = %q, want %q", q.Get("keep_n"), "2")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteTagsBulk(context.Background(), client, DeleteTagsBulkInput{
		ProjectID:       toolutil.StringOrInt("10"),
		RepositoryID:    1,
		NameRegexDelete: "v.*",
		KeepN:           2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteTagsBulk_MissingRepoID verifies DeleteTagsBulk when missing repo ID.
func TestDeleteTagsBulk_MissingRepoID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteTagsBulk(context.Background(), client, DeleteTagsBulkInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err == nil || !strings.Contains(err.Error(), errRepoIDRequired) {
		t.Fatalf(fmtExpectedRepoIDErr, err)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const (
	// errProjectIDRequired identifies the err project ID required constant used by this package.
	errProjectIDRequired = "project_id is required"
	// errRepoIDRequired identifies the err repo ID required constant used by this package.
	errRepoIDRequired = "repository_id is required"
	// errExpectedAPI identifies the err expected API constant used by this package.
	errExpectedAPI = "expected API error, got nil"
	// jsonBadReq identifies the JSON bad req constant used by this package.
	jsonBadReq = `{"message":"bad request"}`
	// fmtExpectedProjectIDErr identifies the fmt expected project ID err constant used by this package.
	fmtExpectedProjectIDErr = "expected project_id required error, got %v"
	// fmtExpectedRepoIDErr identifies the fmt expected repo ID err constant used by this package.
	fmtExpectedRepoIDErr = "expected repository_id required error, got %v"
	// fmtExpectedInMarkdown identifies the fmt expected in markdown constant used by this package.
	fmtExpectedInMarkdown = "expected %q in markdown, got:\n%s"
	// fmtExpectedEmptyMsg identifies the fmt expected empty msg constant used by this package.
	fmtExpectedEmptyMsg = "expected empty message, got:\n%s"
	// testMethodNotAllowed identifies the test method not allowed constant used by this package.
	testMethodNotAllowed = "method not allowed"
	// testProdPattern identifies the test prod pattern constant used by this package.
	testProdPattern = "prod/*"
	// testStagingPattern identifies the test staging pattern constant used by this package.
	testStagingPattern = "staging/*"
	// testCovRepoPath identifies the test cov repo path constant used by this package.
	testCovRepoPath = "g/p/img"
)

// ---------------------------------------------------------------------------
// Constants — prefixed with cov to avoid collisions with existing tests
// ---------------------------------------------------------------------------.

// covRepoJSON identifies the cov repo JSON constant used by this package.
const covRepoJSON = `{
	"id":100,"name":"cov-img","path":"group/project/cov-img",
	"project_id":42,"location":"registry.example.com/group/project/cov-img",
	"tags_count":3,"status":"delete_scheduled",
	"created_at":"2026-01-15T10:00:00Z",
	"cleanup_policy_started_at":"2026-01-16T12:00:00Z"
}`

// covTagJSON identifies the cov tag JSON constant used by this package.
const covTagJSON = `{
	"name":"v1.0","path":"group/project/cov-img:v1.0",
	"location":"registry.example.com/group/project/cov-img:v1.0",
	"revision":"abc123","short_revision":"abc1","digest":"sha256:deadbeef",
	"total_size":4096,"created_at":"2026-02-01T08:00:00Z"
}`

// covRuleJSON identifies the cov rule JSON constant used by this package.
const covRuleJSON = `{
	"id":77,"project_id":42,
	"repository_path_pattern":"prod/*",
	"minimum_access_level_for_push":"maintainer",
	"minimum_access_level_for_delete":"admin"
}`

// ---------------------------------------------------------------------------
// convertRepository — cover optional-field branches
// ---------------------------------------------------------------------------.

// TestConvertRepository_AllFields verifies ConvertRepository when all fields.
func TestConvertRepository_AllFields(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	cleanup := time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)
	status := gl.ContainerRegistryStatus("delete_scheduled")
	r := &gl.RegistryRepository{
		ID: 100, Name: "img", Path: testCovRepoPath, ProjectID: 42,
		Location:               "loc",
		TagsCount:              5,
		CreatedAt:              &now,
		CleanupPolicyStartedAt: &cleanup,
		Status:                 &status,
	}
	out := convertRepository(r)
	if out.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
	if out.CleanupPolicyStartedAt == "" {
		t.Error("expected CleanupPolicyStartedAt to be set")
	}
	if out.Status != "delete_scheduled" {
		t.Errorf("expected Status=delete_scheduled, got %s", out.Status)
	}
}

// TestConvertRepository_NilOptionalFields verifies ConvertRepository when nil optional fields.
func TestConvertRepository_NilOptionalFields(t *testing.T) {
	r := &gl.RegistryRepository{ID: 1, Name: "n", Path: "p", ProjectID: 1}
	out := convertRepository(r)
	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %s", out.CreatedAt)
	}
	if out.CleanupPolicyStartedAt != "" {
		t.Errorf("expected empty CleanupPolicyStartedAt, got %s", out.CleanupPolicyStartedAt)
	}
	if out.Status != "" {
		t.Errorf("expected empty Status, got %s", out.Status)
	}
}

// ---------------------------------------------------------------------------
// convertTag — cover optional-field branches
// ---------------------------------------------------------------------------.

// TestConvertTag_AllFields verifies ConvertTag when all fields.
func TestConvertTag_AllFields(t *testing.T) {
	now := time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC)
	tag := &gl.RegistryRepositoryTag{
		Name: "v1.0", Path: "p", Location: "loc",
		Revision: "abc", ShortRevision: "a", Digest: "sha256:x",
		TotalSize: 4096, CreatedAt: &now,
	}
	out := convertTag(tag)
	if out.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
}

// TestConvertTag_NilCreatedAt verifies ConvertTag when nil created at.
func TestConvertTag_NilCreatedAt(t *testing.T) {
	tag := &gl.RegistryRepositoryTag{Name: "latest"}
	out := convertTag(tag)
	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %s", out.CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// FormatRepositoryMarkdown
// ---------------------------------------------------------------------------.

// TestFormatRepositoryMarkdown_Full verifies FormatRepositoryMarkdown when full.
func TestFormatRepositoryMarkdown_Full(t *testing.T) {
	out := RepositoryOutput{
		ID: 100, Name: "img", Path: testCovRepoPath, ProjectID: 42,
		Location: "loc", TagsCount: 3,
		Status: "delete_scheduled", CreatedAt: "2026-01-15T10:00:00Z",
	}
	md := FormatRepositoryMarkdown(out)
	for _, want := range []string{"Registry Repository: " + testCovRepoPath, "img", testCovRepoPath, "loc", "3", "delete_scheduled", "15 Jan 2026"} {
		if !strings.Contains(md, want) {
			t.Errorf(fmtExpectedInMarkdown, want, md)
		}
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain numeric ID column")
	}
	if strings.Contains(md, "| Project ID |") {
		t.Error("should not contain numeric Project ID column")
	}
}

// TestFormatRepositoryMarkdown_EmptyOptionalFields verifies FormatRepositoryMarkdown when empty optional fields.
func TestFormatRepositoryMarkdown_EmptyOptionalFields(t *testing.T) {
	out := RepositoryOutput{ID: 1, Name: "n", Path: "p", ProjectID: 1}
	md := FormatRepositoryMarkdown(out)
	if strings.Contains(md, "Status") {
		t.Error("should not contain Status row when empty")
	}
	if strings.Contains(md, "Created At") {
		t.Error("should not contain Created At row when empty")
	}
	if !strings.Contains(md, "Registry Repository: p") {
		t.Errorf("expected heading with path, got:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatRepositoryListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatRepositoryListMarkdown_WithItems verifies FormatRepositoryListMarkdown when with items.
func TestFormatRepositoryListMarkdown_WithItems(t *testing.T) {
	out := RepositoryListOutput{
		Repositories: []RepositoryOutput{
			{ID: 1, Name: "a", Path: "x/a", TagsCount: 2},
			{ID: 2, Name: "b", Path: "x/b", TagsCount: 0},
		},
	}
	md := FormatRepositoryListMarkdown(out)
	if !strings.Contains(md, "Repositories (2)") {
		t.Errorf("expected header with count 2, got:\n%s", md)
	}
	if strings.Contains(md, "| ID ") {
		t.Error("should not contain numeric ID column in list")
	}
	for _, want := range []string{"| a |", "| x/a |", "| b |", "| x/b |"} {
		if !strings.Contains(md, want) {
			t.Errorf(fmtExpectedInMarkdown, want, md)
		}
	}
}

// TestFormatRepositoryListMarkdown_Empty verifies FormatRepositoryListMarkdown when empty.
func TestFormatRepositoryListMarkdown_Empty(t *testing.T) {
	out := RepositoryListOutput{}
	md := FormatRepositoryListMarkdown(out)
	if !strings.Contains(md, "No registry repositories found") {
		t.Errorf(fmtExpectedEmptyMsg, md)
	}
}

// ---------------------------------------------------------------------------
// FormatTagMarkdown
// ---------------------------------------------------------------------------.

// TestFormatTagMarkdown_Full verifies FormatTagMarkdown when full.
func TestFormatTagMarkdown_Full(t *testing.T) {
	out := TagOutput{
		Name: "v1.0", Path: "p", Location: "loc",
		Digest: "sha256:abc", Revision: "rev1",
		TotalSize: 1024, CreatedAt: "2026-02-01T08:00:00Z",
	}
	md := FormatTagMarkdown(out)
	for _, want := range []string{"v1.0", "sha256:abc", "rev1", "1024", "1 Feb 2026"} {
		if !strings.Contains(md, want) {
			t.Errorf(fmtExpectedInMarkdown, want, md)
		}
	}
}

// TestFormatTagMarkdown_EmptyOptionalFields verifies FormatTagMarkdown when empty optional fields.
func TestFormatTagMarkdown_EmptyOptionalFields(t *testing.T) {
	out := TagOutput{Name: "latest", Path: "p", Location: "loc"}
	md := FormatTagMarkdown(out)
	if strings.Contains(md, "Digest") {
		t.Error("should not contain Digest row when empty")
	}
	if strings.Contains(md, "Revision") {
		t.Error("should not contain Revision row when empty")
	}
	if strings.Contains(md, "Created At") {
		t.Error("should not contain Created At row when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatTagListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatTagListMarkdown_WithItems verifies FormatTagListMarkdown when with items.
func TestFormatTagListMarkdown_WithItems(t *testing.T) {
	out := TagListOutput{
		Tags: []TagOutput{
			{Name: "v1", Path: "p", TotalSize: 100},
			{Name: "v2", Path: "p", TotalSize: 200},
		},
	}
	md := FormatTagListMarkdown(out)
	if !strings.Contains(md, "Tags (2)") {
		t.Errorf("expected header with count 2, got:\n%s", md)
	}
}

// TestFormatTagListMarkdown_Empty verifies FormatTagListMarkdown when empty.
func TestFormatTagListMarkdown_Empty(t *testing.T) {
	out := TagListOutput{}
	md := FormatTagListMarkdown(out)
	if !strings.Contains(md, "No registry tags found") {
		t.Errorf(fmtExpectedEmptyMsg, md)
	}
}

// ---------------------------------------------------------------------------
// FormatProtectionRuleListMarkdown — empty case
// ---------------------------------------------------------------------------.

// TestFormatProtectionRuleListMarkdown_Empty verifies FormatProtectionRuleListMarkdown when empty.
func TestFormatProtectionRuleListMarkdown_Empty(t *testing.T) {
	out := ProtectionRuleListOutput{}
	md := FormatProtectionRuleListMarkdown(out)
	if !strings.Contains(md, "No protection rules found") {
		t.Errorf(fmtExpectedEmptyMsg, md)
	}
}

// ---------------------------------------------------------------------------
// ListProject — API error, with Tags/TagsCount options
// ---------------------------------------------------------------------------.

// TestListProject_APIError verifies ListProject when API error.
func TestListProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	_, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListProject_WithTagOptions verifies ListProject when with tag options.
func TestListProject_WithTagOptions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/registry/repositories", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("tags") != "true" || q.Get("tags_count") != "true" {
			t.Errorf("expected tags=true&tags_count=true, got %v", q)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covRepoJSON+`]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, mux)
	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID: "42", Tags: true, TagsCount: true,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Repositories) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(out.Repositories))
	}
}

// ---------------------------------------------------------------------------
// ListGroup — API error
// ---------------------------------------------------------------------------.

// TestListGroup_APIError verifies ListGroup when API error.
func TestListGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	_, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// GetRepository — API error, with Tags/TagsCount options
// ---------------------------------------------------------------------------.

// TestGetRepository_APIError verifies GetRepository when API error.
func TestGetRepository_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	_, err := GetRepository(context.Background(), client, GetRepositoryInput{RepositoryID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetRepository_WithTagOptions verifies GetRepository when with tag options.
func TestGetRepository_WithTagOptions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/registry/repositories/99", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covRepoJSON)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := GetRepository(context.Background(), client, GetRepositoryInput{
		RepositoryID: 99, Tags: true, TagsCount: true,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 100 {
		t.Errorf("expected ID 100, got %d", out.ID)
	}
}

// ---------------------------------------------------------------------------
// DeleteRepository — missing project_id, API error
// ---------------------------------------------------------------------------.

// TestDeleteRepository_MissingProjectID verifies DeleteRepository when missing project ID.
func TestDeleteRepository_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteRepository(context.Background(), client, DeleteRepositoryInput{RepositoryID: 1})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestDeleteRepository_APIError verifies DeleteRepository when API error.
func TestDeleteRepository_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	err := DeleteRepository(context.Background(), client, DeleteRepositoryInput{
		ProjectID: "42", RepositoryID: 1,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListTags — missing project_id, API error
// ---------------------------------------------------------------------------.

// TestListTags_MissingProjectID verifies ListTags when missing project ID.
func TestListTags_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListTags(context.Background(), client, ListTagsInput{RepositoryID: 1})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestListTags_APIError verifies ListTags when API error.
func TestListTags_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	_, err := ListTags(context.Background(), client, ListTagsInput{ProjectID: "1", RepositoryID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// GetTag — missing project_id, missing repository_id, API error
// ---------------------------------------------------------------------------.

// TestGetTag_MissingProjectID verifies GetTag when missing project ID.
func TestGetTag_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetTag(context.Background(), client, GetTagInput{RepositoryID: 1, TagName: "x"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestGetTag_MissingRepositoryID verifies GetTag when missing repository ID.
func TestGetTag_MissingRepositoryID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetTag(context.Background(), client, GetTagInput{ProjectID: "1", TagName: "x"})
	if err == nil || !strings.Contains(err.Error(), errRepoIDRequired) {
		t.Fatalf(fmtExpectedRepoIDErr, err)
	}
}

// TestGetTag_APIError verifies GetTag when API error.
func TestGetTag_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	_, err := GetTag(context.Background(), client, GetTagInput{ProjectID: "1", RepositoryID: 1, TagName: "v1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// DeleteTag — missing project_id, missing repository_id, API error
// ---------------------------------------------------------------------------.

// TestDeleteTag_MissingProjectID verifies DeleteTag when missing project ID.
func TestDeleteTag_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteTag(context.Background(), client, DeleteTagInput{RepositoryID: 1, TagName: "x"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestDeleteTag_MissingRepositoryID verifies DeleteTag when missing repository ID.
func TestDeleteTag_MissingRepositoryID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteTag(context.Background(), client, DeleteTagInput{ProjectID: "1", TagName: "x"})
	if err == nil || !strings.Contains(err.Error(), errRepoIDRequired) {
		t.Fatalf(fmtExpectedRepoIDErr, err)
	}
}

// TestDeleteTag_APIError verifies DeleteTag when API error.
func TestDeleteTag_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	err := DeleteTag(context.Background(), client, DeleteTagInput{ProjectID: "1", RepositoryID: 1, TagName: "v1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// DeleteTagsBulk — missing project_id, API error, all optional fields
// ---------------------------------------------------------------------------.

// TestDeleteTagsBulk_MissingProjectID verifies DeleteTagsBulk when missing project ID.
func TestDeleteTagsBulk_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteTagsBulk(context.Background(), client, DeleteTagsBulkInput{RepositoryID: 1})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestDeleteTagsBulk_APIError verifies DeleteTagsBulk when API error.
func TestDeleteTagsBulk_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	err := DeleteTagsBulk(context.Background(), client, DeleteTagsBulkInput{
		ProjectID: "1", RepositoryID: 1, NameRegexDelete: ".*",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteTagsBulk_AllOptionalFields verifies DeleteTagsBulk when all optional fields.
func TestDeleteTagsBulk_AllOptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/registry/repositories/1/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)
	err := DeleteTagsBulk(context.Background(), client, DeleteTagsBulkInput{
		ProjectID:       "42",
		RepositoryID:    1,
		NameRegexDelete: "v.*",
		NameRegexKeep:   "latest",
		KeepN:           5,
		OlderThan:       "7d",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// ListProtectionRules — API error
// ---------------------------------------------------------------------------.

// TestListProtectionRules_APIError verifies ListProtectionRules when API error.
func TestListProtectionRules_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	_, err := ListProtectionRules(context.Background(), client, ListProtectionRulesInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// CreateProtectionRule — API error, no access levels
// ---------------------------------------------------------------------------.

// TestCreateProtectionRule_APIError verifies CreateProtectionRule when API error.
func TestCreateProtectionRule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	_, err := CreateProtectionRule(context.Background(), client, CreateProtectionRuleInput{
		ProjectID: "1", RepositoryPathPattern: "x",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateProtectionRule_NoAccessLevels verifies CreateProtectionRule when no access levels.
func TestCreateProtectionRule_NoAccessLevels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/registry/protection/repository/rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, covRuleJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := CreateProtectionRule(context.Background(), client, CreateProtectionRuleInput{
		ProjectID:             "42",
		RepositoryPathPattern: testProdPattern,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 77 {
		t.Errorf("expected ID 77, got %d", out.ID)
	}
}

// ---------------------------------------------------------------------------
// UpdateProtectionRule — API error, with access levels
// ---------------------------------------------------------------------------.

// TestUpdateProtectionRule_APIError verifies UpdateProtectionRule when API error.
func TestUpdateProtectionRule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	_, err := UpdateProtectionRule(context.Background(), client, UpdateProtectionRuleInput{
		ProjectID: "1", RuleID: 1,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateProtectionRule_AllOptionalFields verifies UpdateProtectionRule when all optional fields.
func TestUpdateProtectionRule_AllOptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/registry/protection/repository/rules/77", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			testutil.RespondJSON(w, http.StatusOK, covRuleJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := UpdateProtectionRule(context.Background(), client, UpdateProtectionRuleInput{
		ProjectID:                   "42",
		RuleID:                      77,
		RepositoryPathPattern:       testStagingPattern,
		MinimumAccessLevelForPush:   "owner",
		MinimumAccessLevelForDelete: "admin",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 77 {
		t.Errorf("expected ID 77, got %d", out.ID)
	}
}

// ---------------------------------------------------------------------------
// DeleteProtectionRule — API error
// ---------------------------------------------------------------------------.

// TestDeleteProtectionRule_APIError verifies DeleteProtectionRule when API error.
func TestDeleteProtectionRule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	err := DeleteProtectionRule(context.Background(), client, DeleteProtectionRuleInput{ProjectID: "1", RuleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for container registry actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	specs := ActionSpecs(client)
	byTool := registrySpecsByTool(t, specs)

	if len(specs) != 12 {
		t.Fatalf("len(ActionSpecs) = %d, want 12", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, toolName := range []string{"gitlab_registry_delete_repository", "gitlab_registry_delete_tag", "gitlab_registry_delete_tags_bulk", "gitlab_registry_protection_delete"} {
		if !byTool[toolName].Route.Destructive {
			t.Fatalf("%s should be destructive", toolName)
		}
	}
	for _, spec := range byTool {
		if spec.Usage == "" {
			t.Fatalf("Usage for %s is empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s are empty", spec.Name)
		}
	}
}

// newRegistryMCPTestMux creates a ServeMux that handles all registry API endpoints
// used by the MCP round-trip tests.
func newRegistryMCPTestMux() *http.ServeMux {
	const basePath = "/api/v4"
	mux := http.NewServeMux()

	mux.HandleFunc(basePath+"/projects/42/registry/repositories", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covRepoJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc(basePath+"/projects/42/registry/repositories/100", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc(basePath+"/groups/10/registry/repositories", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covRepoJSON+`]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})

	mux.HandleFunc(basePath+"/registry/repositories/100", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covRepoJSON)
	})

	mux.HandleFunc(basePath+"/projects/42/registry/repositories/100/tags", covTagsHandler)

	mux.HandleFunc(basePath+"/projects/42/registry/repositories/100/tags/v1.0", covSingleTagHandler)

	mux.HandleFunc(basePath+"/projects/42/registry/protection/repository/rules", covProtectionRulesHandler)

	mux.HandleFunc(basePath+"/projects/42/registry/protection/repository/rules/77", covProtectionRuleHandler)

	return mux
}

// covTagsHandler supports cov tags handler assertions in containerregistry tests.
func covTagsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covTagJSON+`]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// covSingleTagHandler supports cov single tag handler assertions in containerregistry tests.
func covSingleTagHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		testutil.RespondJSON(w, http.StatusOK, covTagJSON)
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// covProtectionRulesHandler supports cov protection rules handler assertions in containerregistry tests.
func covProtectionRulesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covRuleJSON+`]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	case http.MethodPost:
		testutil.RespondJSON(w, http.StatusCreated, covRuleJSON)
	default:
		http.NotFound(w, r)
	}
}

// covProtectionRuleHandler supports cov protection rule handler assertions in containerregistry tests.
func covProtectionRuleHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPatch:
		testutil.RespondJSON(w, http.StatusOK, covRuleJSON)
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage for all 12 individual tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates container registry routes through canonical specs.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := registrySpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, newRegistryMCPTestMux())))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_registry_list_project", map[string]any{"project_id": "42"}},
		{"gitlab_registry_list_group", map[string]any{"group_id": "10"}},
		{"gitlab_registry_get_repository", map[string]any{"repository_id": 100}},
		{"gitlab_registry_delete_repository", map[string]any{"project_id": "42", "repository_id": 100}},
		{"gitlab_registry_list_tags", map[string]any{"project_id": "42", "repository_id": 100}},
		{"gitlab_registry_get_tag", map[string]any{"project_id": "42", "repository_id": 100, "tag_name": "v1.0"}},
		{"gitlab_registry_delete_tag", map[string]any{"project_id": "42", "repository_id": 100, "tag_name": "v1.0"}},
		{"gitlab_registry_delete_tags_bulk", map[string]any{"project_id": "42", "repository_id": 100, "name_regex_delete": ".*"}},
		{"gitlab_registry_protection_list", map[string]any{"project_id": "42"}},
		{"gitlab_registry_protection_create", map[string]any{"project_id": "42", "repository_path_pattern": testProdPattern}},
		{"gitlab_registry_protection_update", map[string]any{"project_id": "42", "rule_id": 77}},
		{"gitlab_registry_protection_delete", map[string]any{"project_id": "42", "rule_id": 77}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			result, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s): %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s): nil result", tc.name)
			}
		})
	}
}

// registrySpecsByTool supports registry specs by tool assertions in containerregistry tests.
func registrySpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// Additional formatter tests for TASK-053 improvements
// ---------------------------------------------------------------------------.

// TestFormatRepositoryMarkdown_FallbackToName verifies FormatRepositoryMarkdown when fallback to name.
func TestFormatRepositoryMarkdown_FallbackToName(t *testing.T) {
	out := RepositoryOutput{ID: 1, Name: "my-img", Path: ""}
	md := FormatRepositoryMarkdown(out)
	if !strings.Contains(md, "Registry Repository: my-img") {
		t.Errorf("expected heading with name fallback, got:\n%s", md)
	}
}

// TestFormatProtectionRuleMarkdown_NoNumericIDs verifies FormatProtectionRuleMarkdown when no numeric IDs.
func TestFormatProtectionRuleMarkdown_NoNumericIDs(t *testing.T) {
	out := ProtectionRuleOutput{
		ID: 77, ProjectID: 42,
		RepositoryPathPattern:       testStagingPattern,
		MinimumAccessLevelForPush:   "owner",
		MinimumAccessLevelForDelete: "admin",
	}
	md := FormatProtectionRuleMarkdown(out)
	if !strings.Contains(md, "Protection Rule: staging/*") {
		t.Errorf("expected heading with pattern, got:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain numeric ID column")
	}
	if strings.Contains(md, "| Project ID |") {
		t.Error("should not contain numeric Project ID column")
	}
	for _, want := range []string{testStagingPattern, "owner", "admin"} {
		if !strings.Contains(md, want) {
			t.Errorf(fmtExpectedInMarkdown, want, md)
		}
	}
}

// TestFormatProtectionRuleListMarkdown_NoNumericIDs verifies FormatProtectionRuleListMarkdown when no numeric IDs.
func TestFormatProtectionRuleListMarkdown_NoNumericIDs(t *testing.T) {
	out := ProtectionRuleListOutput{
		Rules: []ProtectionRuleOutput{
			{ID: 1, RepositoryPathPattern: testProdPattern, MinimumAccessLevelForPush: "maintainer", MinimumAccessLevelForDelete: "admin"},
			{ID: 2, RepositoryPathPattern: testStagingPattern, MinimumAccessLevelForPush: "owner", MinimumAccessLevelForDelete: "owner"},
		},
	}
	md := FormatProtectionRuleListMarkdown(out)
	if strings.Contains(md, "| ID ") {
		t.Error("should not contain numeric ID column in list")
	}
	for _, want := range []string{testProdPattern, testStagingPattern, "maintainer", "owner"} {
		if !strings.Contains(md, want) {
			t.Errorf(fmtExpectedInMarkdown, want, md)
		}
	}
}
