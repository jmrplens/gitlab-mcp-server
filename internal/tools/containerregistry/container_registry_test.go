// container_registry_test.go contains unit tests for the container registry MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package containerregistry

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestListProject_Success verifies that ListProject succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListProject_MissingProjectID verifies that ListProject returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestListProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListProject(context.Background(), client, ListProjectInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// ListGroup
// ---------------------------------------------------------------------------.

// TestListGroup_Success verifies that ListGroup succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListGroup_MissingGroupID verifies that ListGroup returns a validation error when group_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing group_id field.
func TestListGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListGroup(context.Background(), client, ListGroupInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetRepository
// ---------------------------------------------------------------------------.

// TestGetRepository_Success verifies that GetRepository succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGetRepository_MissingID verifies that GetRepository returns a validation error when repository_id is zero.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing repository_id field.
func TestGetRepository_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetRepository(context.Background(), client, GetRepositoryInput{})
	if err == nil || !strings.Contains(err.Error(), errRepoIDRequired) {
		t.Fatalf(fmtExpectedRepoIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// DeleteRepository
// ---------------------------------------------------------------------------.

// TestDeleteRepository_Success verifies that DeleteRepository succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDeleteRepository_MissingRepoID verifies that DeleteRepository returns a validation error when repository_id is zero.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing repository_id field.
func TestDeleteRepository_MissingRepoID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestListTags_Success verifies that ListTags succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListTags_MissingRepoID verifies that ListTags returns a validation error when repository_id is zero.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing repository_id field.
func TestListTags_MissingRepoID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestGetTag_Success verifies that GetTag succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGetTag_MissingTagName verifies that GetTag returns a validation error when tag_name is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing tag_name field.
func TestGetTag_MissingTagName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetTag(context.Background(), client, GetTagInput{
		ProjectID: toolutil.StringOrInt("10"), RepositoryID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "tag_name is required") {
		t.Fatalf("expected tag_name required error, got %v", err)
	}
}

// TestListTags_MultiPage verifies the ListTags_MultiPage handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDeleteTag_Success verifies that DeleteTag succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDeleteTag_MissingTagName verifies that DeleteTag returns a validation error when tag_name is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing tag_name field.
func TestDeleteTag_MissingTagName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestDeleteTagsBulk_Success verifies that DeleteTagsBulk succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDeleteTagsBulk_MissingRepoID verifies that DeleteTagsBulk returns a validation error when repository_id is zero.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing repository_id field.
func TestDeleteTagsBulk_MissingRepoID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestConvertRepository_AllFields verifies the converter fills every output
// field from the field of the same meaning, with no two fixture values alike.
// It compares the whole output because the two timestamps used to be asserted
// only as non-empty, and swapping their assignments failed nothing.
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
		Tags: []*gl.RegistryRepositoryTag{
			{Name: "v1.0", Path: "g/p/img:v1.0", Location: "loc:v1.0", TotalSize: 2048},
		},
	}
	got := convertRepository(r, toolutil.RegistryRepositoryExtra{Size: 4096, DeleteAPIPath: "/api/v4/registry/repositories/1"})
	want := RepositoryOutput{
		ID: 100, Name: "img", Path: testCovRepoPath, ProjectID: 42,
		Location:               "loc",
		CreatedAt:              "2026-01-15T10:00:00Z",
		CleanupPolicyStartedAt: "2026-01-16T12:00:00Z",
		Status:                 "delete_scheduled",
		TagsCount:              5,
		Size:                   4096,
		DeleteAPIPath:          "/api/v4/registry/repositories/1",
		Tags:                   []TagOutput{{Name: "v1.0", Path: "g/p/img:v1.0", Location: "loc:v1.0", TotalSize: 2048}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("convertRepository() =\n%#v\nwant:\n%#v", got, want)
	}
}

// TestConvertRepository_NilOptionalFields verifies the converter leaves the
// timestamp and status rows empty when GitLab sent none, rather than the zero
// time or a zero-value status.
func TestConvertRepository_NilOptionalFields(t *testing.T) {
	r := &gl.RegistryRepository{ID: 1, Name: "n", Path: "p", ProjectID: 1}
	out := convertRepository(r, toolutil.RegistryRepositoryExtra{})
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

// TestConvertTag_AllFields verifies the converter fills every output field
// from the field of the same meaning. The revision and the short revision are
// the pair a swap would confuse, and asserting only the timestamp let one by.
func TestConvertTag_AllFields(t *testing.T) {
	now := time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC)
	tag := &gl.RegistryRepositoryTag{
		Name: "v1.0", Path: "p", Location: "loc",
		Revision: "abc", ShortRevision: "a", Digest: "sha256:x",
		TotalSize: 4096, CreatedAt: &now,
	}
	got := convertTag(tag)
	want := TagOutput{
		Name: "v1.0", Path: "p", Location: "loc",
		Revision: "abc", ShortRevision: "a", Digest: "sha256:x",
		CreatedAt: "2026-02-01T08:00:00Z", TotalSize: 4096,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("convertTag() =\n%#v\nwant:\n%#v", got, want)
	}
}

// TestConvertTag_NilCreatedAt verifies the converter leaves the creation time
// empty when GitLab sent none, rather than the zero time.
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

// repositoryCardHints is the guidance a registry repository card closes with.
const repositoryCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'registry_tag_list' to list tags in this repository\n" +
	"- Use action 'registry_delete' to delete this repository\n"

// TestFormatRepositoryMarkdown_Full verifies the whole card of a fully
// populated registry repository. The ID row is what the audit found missing:
// every follow-up action on a repository takes repository_id, and the card
// that named the repository did not carry it.
func TestFormatRepositoryMarkdown_Full(t *testing.T) {
	out := RepositoryOutput{
		ID: 100, Name: "img", Path: testCovRepoPath, ProjectID: 42,
		Location: "loc", TagsCount: 3,
		Status: "delete_scheduled", CreatedAt: "2026-01-15T10:00:00Z",
	}
	got := FormatRepositoryMarkdown(out)
	want := "## Registry Repository: " + testCovRepoPath + "\n\n" +
		"- **ID**: 100\n" +
		"- **Name**: img\n" +
		"- **Path**: " + testCovRepoPath + "\n" +
		"- **Location**: loc\n" +
		"- **Tags Count**: 3\n" +
		"- **Status**: delete_scheduled\n" +
		"- **Created At**: 15 Jan 2026 10:00 UTC\n" +
		repositoryCardHints
	if got != want {
		t.Errorf("FormatRepositoryMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatRepositoryMarkdown_EmptyOptionalFields verifies the card omits
// every row GitLab did not fill, the tag count included: the count is sent
// only when the caller asked for it, so a zero is silence rather than a
// repository with no tags.
func TestFormatRepositoryMarkdown_EmptyOptionalFields(t *testing.T) {
	out := RepositoryOutput{ID: 1, Name: "n", Path: "p", ProjectID: 1}
	got := FormatRepositoryMarkdown(out)
	want := "## Registry Repository: p\n\n" +
		"- **ID**: 1\n" +
		"- **Name**: n\n" +
		"- **Path**: p\n" +
		repositoryCardHints
	if got != want {
		t.Errorf("FormatRepositoryMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatRepositoryMarkdown_NoNameNoPath_HeadsWithTheResourceAlone verifies
// a repository GitLab sent neither a path nor a name for is headed by the
// resource alone, rather than by a heading ending in a colon with nothing
// behind it.
func TestFormatRepositoryMarkdown_NoNameNoPath_HeadsWithTheResourceAlone(t *testing.T) {
	got := FormatRepositoryMarkdown(RepositoryOutput{ID: 7})
	want := "## Registry Repository\n\n" +
		"- **ID**: 7\n" +
		repositoryCardHints
	if got != want {
		t.Errorf("FormatRepositoryMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatRepositoryListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatRepositoryListMarkdown_WithItems verifies the whole table. The ID
// column is the one every follow-up action needs, and a repository whose tag
// count GitLab did not report shows a dash rather than a zero it never sent.
func TestFormatRepositoryListMarkdown_WithItems(t *testing.T) {
	out := RepositoryListOutput{
		Repositories: []RepositoryOutput{
			{ID: 1, Name: "a", Path: "x/a", TagsCount: 2},
			{ID: 2, Name: "b", Path: "x/b", TagsCount: 0},
		},
	}
	got := FormatRepositoryListMarkdown(out)
	want := "## Registry Repositories (2)\n\n" +
		"| ID | Name | Path | Tags Count |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 1 | a | x/a | 2 |\n" +
		"| 2 | b | x/b | - |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'registry_get' with repository_id for full details\n"
	if got != want {
		t.Errorf("FormatRepositoryListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatRepositoryListMarkdown_Empty verifies an empty list is the one
// sentence and nothing else: no heading counting zero above it.
func TestFormatRepositoryListMarkdown_Empty(t *testing.T) {
	got := FormatRepositoryListMarkdown(RepositoryListOutput{})
	want := "No registry repositories found.\n"
	if got != want {
		t.Errorf("FormatRepositoryListMarkdown() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatTagMarkdown
// ---------------------------------------------------------------------------.

// tagCardHints is the guidance a registry tag card closes with.
const tagCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'registry_tag_delete' to remove this tag\n"

// TestFormatTagMarkdown_Full verifies the whole card of a fully populated
// tag, with the digest and revision as code spans and the size carrying its
// unit.
func TestFormatTagMarkdown_Full(t *testing.T) {
	out := TagOutput{
		Name: "v1.0", Path: "p", Location: "loc",
		Digest: "sha256:abc", Revision: "rev1", ShortRevision: "rev",
		TotalSize: 1024, CreatedAt: "2026-02-01T08:00:00Z",
	}
	got := FormatTagMarkdown(out)
	want := "## Registry Tag: v1.0\n\n" +
		"- **Name**: v1.0\n" +
		"- **Path**: p\n" +
		"- **Location**: loc\n" +
		"- **Digest**: `sha256:abc`\n" +
		"- **Revision**: `rev1`\n" +
		"- **Short Revision**: `rev`\n" +
		"- **Total Size**: 1024 bytes\n" +
		"- **Created At**: 1 Feb 2026 08:00 UTC\n" +
		tagCardHints
	if got != want {
		t.Errorf("FormatTagMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatTagMarkdown_EmptyOptionalFields verifies the card omits every row
// GitLab did not fill.
func TestFormatTagMarkdown_EmptyOptionalFields(t *testing.T) {
	got := FormatTagMarkdown(TagOutput{Name: "latest", Path: "p", Location: "loc"})
	want := "## Registry Tag: latest\n\n" +
		"- **Name**: latest\n" +
		"- **Path**: p\n" +
		"- **Location**: loc\n" +
		"- **Total Size**: 0 bytes\n" +
		tagCardHints
	if got != want {
		t.Errorf("FormatTagMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatTagMarkdown_NoName_HeadsWithTheResourceAlone verifies a tag GitLab
// sent no name for is headed by the resource alone.
func TestFormatTagMarkdown_NoName_HeadsWithTheResourceAlone(t *testing.T) {
	got := FormatTagMarkdown(TagOutput{Path: "p"})
	want := "## Registry Tag\n\n" +
		"- **Path**: p\n" +
		"- **Total Size**: 0 bytes\n" +
		tagCardHints
	if got != want {
		t.Errorf("FormatTagMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatTagListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatTagListMarkdown_WithItems verifies the whole table, whose size
// column names its unit in the header rather than showing a bare number.
func TestFormatTagListMarkdown_WithItems(t *testing.T) {
	out := TagListOutput{
		Tags: []TagOutput{
			{Name: "v1", Path: "p", TotalSize: 100},
			{Name: "v2", Path: "p", TotalSize: 200},
		},
	}
	got := FormatTagListMarkdown(out)
	want := "## Registry Tags (2)\n\n" +
		"| Name | Path | Total Size (bytes) |\n" +
		"| --- | --- | --- |\n" +
		"| v1 | p | 100 |\n" +
		"| v2 | p | 200 |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'registry_tag_get' with tag name for full details\n" +
		"- Use action 'registry_tag_delete_bulk' to clean up old tags\n"
	if got != want {
		t.Errorf("FormatTagListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatTagListMarkdown_Empty verifies an empty list is the one sentence
// and nothing else.
func TestFormatTagListMarkdown_Empty(t *testing.T) {
	got := FormatTagListMarkdown(TagListOutput{})
	want := "No registry tags found.\n"
	if got != want {
		t.Errorf("FormatTagListMarkdown() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatProtectionRuleListMarkdown — empty case
// ---------------------------------------------------------------------------.

// TestFormatProtectionRuleListMarkdown_Empty verifies an empty list is the
// one sentence and nothing else.
func TestFormatProtectionRuleListMarkdown_Empty(t *testing.T) {
	got := FormatProtectionRuleListMarkdown(ProtectionRuleListOutput{})
	want := "No protection rules found.\n"
	if got != want {
		t.Errorf("FormatProtectionRuleListMarkdown() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// ListProject — API error, with Tags/TagsCount options
// ---------------------------------------------------------------------------.

// TestListProject_APIError verifies that ListProject returns the failure when GitLab answers a status the handler attaches no hint to.
// The test answers 400 to a handler that hints on 404 alone.
// It asserts a non-nil error; the hinted status is TestHandlers_RefusedAtTheHintedStatus_CarryTheHint's.
func TestListProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	_, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListProject_WithTagOptions verifies the ListProject_WithTagOptions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListGroup_APIError verifies that ListGroup returns the failure when GitLab answers a status the handler attaches no hint to.
// The test answers 400 to a handler that hints on 404 alone.
// It asserts a non-nil error; the hinted status is TestHandlers_RefusedAtTheHintedStatus_CarryTheHint's.
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

// TestGetRepository_APIError verifies that GetRepository returns the failure when GitLab answers a status the handler attaches no hint to.
// The test answers 400 to a handler that hints on 404 alone.
// It asserts a non-nil error; the hinted status is TestHandlers_RefusedAtTheHintedStatus_CarryTheHint's.
func TestGetRepository_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, jsonBadReq)
	}))
	_, err := GetRepository(context.Background(), client, GetRepositoryInput{RepositoryID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetRepository_WithTagOptions verifies the GetRepository_WithTagOptions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDeleteRepository_MissingProjectID verifies that DeleteRepository returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestDeleteRepository_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteRepository(context.Background(), client, DeleteRepositoryInput{RepositoryID: 1})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestDeleteRepository_APIError verifies that DeleteRepository returns the failure when GitLab answers a status the handler attaches no hint to.
// The test answers 400 to a handler that hints on 403 alone.
// It asserts a non-nil error; the hinted status is TestHandlers_RefusedAtTheHintedStatus_CarryTheHint's.
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

// TestListTags_MissingProjectID verifies that ListTags returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestListTags_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListTags(context.Background(), client, ListTagsInput{RepositoryID: 1})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestListTags_APIError verifies that ListTags returns the failure when GitLab answers a status the handler attaches no hint to.
// The test answers 400 to a handler that hints on 404 alone.
// It asserts a non-nil error; the hinted status is TestHandlers_RefusedAtTheHintedStatus_CarryTheHint's.
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

// TestGetTag_MissingProjectID verifies that GetTag returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestGetTag_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetTag(context.Background(), client, GetTagInput{RepositoryID: 1, TagName: "x"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestGetTag_MissingRepositoryID verifies that GetTag returns a validation error when repository_id is zero.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing repository_id field.
func TestGetTag_MissingRepositoryID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetTag(context.Background(), client, GetTagInput{ProjectID: "1", TagName: "x"})
	if err == nil || !strings.Contains(err.Error(), errRepoIDRequired) {
		t.Fatalf(fmtExpectedRepoIDErr, err)
	}
}

// TestGetTag_APIError verifies that GetTag returns the failure when GitLab answers a status the handler attaches no hint to.
// The test answers 400 to a handler that hints on 404 alone.
// It asserts a non-nil error; the hinted status is TestHandlers_RefusedAtTheHintedStatus_CarryTheHint's.
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

// TestDeleteTag_MissingProjectID verifies that DeleteTag returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestDeleteTag_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteTag(context.Background(), client, DeleteTagInput{RepositoryID: 1, TagName: "x"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestDeleteTag_MissingRepositoryID verifies that DeleteTag returns a validation error when repository_id is zero.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing repository_id field.
func TestDeleteTag_MissingRepositoryID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteTag(context.Background(), client, DeleteTagInput{ProjectID: "1", TagName: "x"})
	if err == nil || !strings.Contains(err.Error(), errRepoIDRequired) {
		t.Fatalf(fmtExpectedRepoIDErr, err)
	}
}

// TestDeleteTag_APIError verifies that DeleteTag returns the failure when GitLab answers a status the handler attaches no hint to.
// The test answers 400 to a handler that hints on 403 alone.
// It asserts a non-nil error; the hinted status is TestHandlers_RefusedAtTheHintedStatus_CarryTheHint's.
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

// TestDeleteTagsBulk_MissingProjectID verifies that DeleteTagsBulk returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestDeleteTagsBulk_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteTagsBulk(context.Background(), client, DeleteTagsBulkInput{RepositoryID: 1})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestDeleteTagsBulk_APIError verifies that DeleteTagsBulk returns the failure GitLab answered with.
// The test answers 400, which is the status this handler hints on.
// It asserts a non-nil error alone; the hint's content is asserted in TestHandlers_RefusedAtTheHintedStatus_CarryTheHint.
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

// TestDeleteTagsBulk_AllOptionalFields verifies the DeleteTagsBulk_AllOptionalFields handler.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListProtectionRules_APIError verifies that ListProtectionRules returns the failure when GitLab answers a status the handler attaches no hint to.
// The test answers 400 to a handler that hints on 404 alone.
// It asserts a non-nil error; the hinted status is TestHandlers_RefusedAtTheHintedStatus_CarryTheHint's.
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

// TestCreateProtectionRule_APIError verifies that CreateProtectionRule returns the failure GitLab answered with.
// The test answers 400, which is the status this handler hints on.
// It asserts a non-nil error alone; the hint's content is asserted in TestHandlers_RefusedAtTheHintedStatus_CarryTheHint.
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

// TestCreateProtectionRule_NoAccessLevels verifies the CreateProtectionRule_NoAccessLevels handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestUpdateProtectionRule_APIError verifies that UpdateProtectionRule returns the failure when GitLab answers a status the handler attaches no hint to.
// The test answers 400 to a handler that hints on 404 alone.
// It asserts a non-nil error; the hinted status is TestHandlers_RefusedAtTheHintedStatus_CarryTheHint's.
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

// TestUpdateProtectionRule_AllOptionalFields verifies the UpdateProtectionRule_AllOptionalFields handler.
// The test exercises the PATCH path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDeleteProtectionRule_APIError verifies that DeleteProtectionRule returns the failure when GitLab answers a status the handler attaches no hint to.
// The test answers 400 to a handler that hints on 404 alone.
// It asserts a non-nil error; the hinted status is TestHandlers_RefusedAtTheHintedStatus_CarryTheHint's.
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

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	specs := ActionSpecs(client)
	byTool := registrySpecsByTool(t, specs)

	if len(specs) != 16 {
		t.Fatalf("len(ActionSpecs) = %d, want 16", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, toolName := range []string{"gitlab_registry_delete_repository", "gitlab_registry_delete_tag", "gitlab_registry_delete_tags_bulk", "gitlab_registry_protection_delete", "gitlab_registry_tag_protection_delete"} {
		t.Run(toolName, func(t *testing.T) {
			if !byTool[toolName].Route.Destructive {
				t.Fatalf("%s should be destructive", toolName)
			}
		})
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

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestFormatRepositoryMarkdown_FallbackToName verifies the heading falls back
// to the repository's name when GitLab sent no path.
func TestFormatRepositoryMarkdown_FallbackToName(t *testing.T) {
	got := FormatRepositoryMarkdown(RepositoryOutput{ID: 1, Name: "my-img", Path: ""})
	want := "## Registry Repository: my-img\n\n" +
		"- **ID**: 1\n" +
		"- **Name**: my-img\n" +
		repositoryCardHints
	if got != want {
		t.Errorf("FormatRepositoryMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatProtectionRuleMarkdown_CarriesTheRuleID verifies the whole card of
// a repository-path protection rule: the pattern in the heading and again as a
// code span, and the rule id every follow-up action takes.
func TestFormatProtectionRuleMarkdown_CarriesTheRuleID(t *testing.T) {
	out := ProtectionRuleOutput{
		ID: 77, ProjectID: 42,
		RepositoryPathPattern:       testStagingPattern,
		MinimumAccessLevelForPush:   "owner",
		MinimumAccessLevelForDelete: "admin",
	}
	got := FormatProtectionRuleMarkdown(out)
	want := "## Protection Rule: " + testStagingPattern + "\n\n" +
		"- **ID**: 77\n" +
		"- **Repository Path Pattern**: `" + testStagingPattern + "`\n" +
		"- **Min Access Level (Push)**: owner\n" +
		"- **Min Access Level (Delete)**: admin\n" +
		protectionRuleCardHints
	if got != want {
		t.Errorf("FormatProtectionRuleMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatProtectionRuleListMarkdown_WithItems verifies the whole table. The
// ID column is the one registry_rule_update and registry_rule_delete both
// take, and a list that showed only patterns left a reader with no way to name
// the rule it had just found.
func TestFormatProtectionRuleListMarkdown_WithItems(t *testing.T) {
	out := ProtectionRuleListOutput{
		Rules: []ProtectionRuleOutput{
			{ID: 1, RepositoryPathPattern: testProdPattern, MinimumAccessLevelForPush: "maintainer", MinimumAccessLevelForDelete: "admin"},
			{ID: 2, RepositoryPathPattern: testStagingPattern, MinimumAccessLevelForPush: "owner", MinimumAccessLevelForDelete: "owner"},
		},
	}
	got := FormatProtectionRuleListMarkdown(out)
	want := "## Protection Rules (2)\n\n" +
		"| ID | Pattern | Min Push | Min Delete |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 1 | `" + testProdPattern + "` | maintainer | admin |\n" +
		"| 2 | `" + testStagingPattern + "` | owner | owner |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'registry_rule_create' to add a new rule\n"
	if got != want {
		t.Errorf("FormatProtectionRuleListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// 1:1 audit additions: keyset pagination, order_by/sort, deprecated name_regex
// ---------------------------------------------------------------------------.

// TestListProject_KeysetAndOrdering verifies that ListProject forwards keyset
// pagination (pagination, page_token), order_by, and sort query parameters to
// the GitLab API. It asserts each parameter reaches the request unchanged.
func TestListProject_KeysetAndOrdering(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/repositories", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pagination") != "keyset" {
			t.Errorf("pagination = %q, want keyset", q.Get("pagination"))
		}
		if q.Get("page_token") != "tok99" {
			t.Errorf("page_token = %q, want tok99", q.Get("page_token"))
		}
		if q.Get("order_by") != "name" {
			t.Errorf("order_by = %q, want name", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("sort = %q, want desc", q.Get("sort"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "0", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID:  toolutil.StringOrInt("10"),
		OrderBy:    "name",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok99",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestListGroup_KeysetAndOrdering verifies that ListGroup forwards keyset
// pagination, order_by, and sort query parameters to the GitLab API.
func TestListGroup_KeysetAndOrdering(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/registry/repositories", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "id" || q.Get("sort") != "asc" || q.Get("pagination") != "keyset" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "0", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID:    toolutil.StringOrInt("5"),
		OrderBy:    "id",
		Sort:       "asc",
		Pagination: "keyset",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestListTags_KeysetAndOrdering verifies that ListTags forwards keyset
// pagination, order_by, and sort query parameters to the GitLab API.
func TestListTags_KeysetAndOrdering(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/repositories/1/tags", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "name" || q.Get("sort") != "desc" || q.Get("page_token") != "t1" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "0", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := ListTags(context.Background(), client, ListTagsInput{
		ProjectID:    toolutil.StringOrInt("10"),
		RepositoryID: 1,
		OrderBy:      "name",
		Sort:         "desc",
		Pagination:   "keyset", PageToken: "t1",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestRegistryRepositories_UnreadableCapturedDeleteAPIPath verifies that every
// registry repository handler returns an error rather than a half-filled
// repository when GitLab sends delete_api_path as something that is not a
// string. The SDK ignores the key its own RegistryRepository does not model, so
// the read of the captured response is the only thing that can notice.
func TestRegistryRepositories_UnreadableCapturedDeleteAPIPath(t *testing.T) {
	// A list answers with an array and a get with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	// Each of these three handlers wraps the capture's failure under an
	// operation of its own, a second copy of the label its hinted refusal
	// carries, and the shared assertion below judges only the decode message.
	// The errors are therefore kept here and their operations asserted once
	// the cases have run, which leaves every assertion on the test goroutine.
	refused := map[string]error{}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list_project", Call: func() error {
			client := poisoned(`[{"id":1,"name":"app","delete_api_path":42}]`)
			_, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "10"})
			refused["list_project"] = err
			return err
		}},
		{Name: "list_group", Call: func() error {
			client := poisoned(`[{"id":1,"name":"app","delete_api_path":42}]`)
			_, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "7"})
			refused["list_group"] = err
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"id":1,"name":"app","delete_api_path":42}`)
			_, err := GetRepository(context.Background(), client, GetRepositoryInput{RepositoryID: 1})
			refused["get"] = err
			return err
		}},
	})
	for name, op := range map[string]string{
		"list_project": "registry_list_project",
		"list_group":   "registry_list_group",
		"get":          "registry_get_repository",
	} {
		t.Run(name+"_operation", func(t *testing.T) {
			err := refused[name]
			if err == nil {
				t.Fatalf("%s error = nil, want the capture's decode failure", name)
			}
			if !strings.HasPrefix(err.Error(), op+": ") {
				t.Errorf("error = %q, want it to open with the operation %q", err, op)
			}
		})
	}
}

// TestHandlers_RefusedAtTheHintedStatus_CarryTheHint verifies that every
// handler, refused at the one status it attaches a hint to, returns an error
// naming that handler's operation and carrying that hint. The operation, the
// status and the hint are straight-line constants no gate can see moved, and
// until this existed a handler could hint on the wrong status, or on none,
// with nothing failing: the API-error tests below answer 400 to handlers that
// hint on 404 and 403, and assert only that an error came back.
//
// The operation is the prefix of every message a handler produces, through
// both branches of WrapErrWithStatusHint, and it is what tells a reader which
// of the sixteen refused. Nothing else in the package reads it: the tool names
// and catalog IDs the other tests assert are different strings, so two of the
// sixteen labels could be swapped and every other test would still pass. The
// assertion takes the separator with it, since one label is a prefix of
// another (registry_delete_tag of registry_delete_tags_bulk) and the bare
// prefix would accept the bulk handler answering under the single one.
func TestHandlers_RefusedAtTheHintedStatus_CarryTheHint(t *testing.T) {
	cases := []struct {
		name   string
		op     string
		status int
		hint   string
		call   func(client *gitlabclient.Client) error
	}{
		{"list_project", "registry_list_project", http.StatusNotFound, "verify project_id", func(c *gitlabclient.Client) error {
			_, err := ListProject(context.Background(), c, ListProjectInput{ProjectID: "1"})
			return err
		}},
		{"list_group", "registry_list_group", http.StatusNotFound, "verify group_id", func(c *gitlabclient.Client) error {
			_, err := ListGroup(context.Background(), c, ListGroupInput{GroupID: "1"})
			return err
		}},
		{"get_repository", "registry_get_repository", http.StatusNotFound, "queried by ID, not name", func(c *gitlabclient.Client) error {
			_, err := GetRepository(context.Background(), c, GetRepositoryInput{RepositoryID: 1})
			return err
		}},
		{"delete_repository", "registry_delete_repository", http.StatusForbidden, "requires Maintainer role", func(c *gitlabclient.Client) error {
			return DeleteRepository(context.Background(), c, DeleteRepositoryInput{ProjectID: "1", RepositoryID: 1})
		}},
		{"list_tags", "registry_list_tags", http.StatusNotFound, "may have no tags", func(c *gitlabclient.Client) error {
			_, err := ListTags(context.Background(), c, ListTagsInput{ProjectID: "1", RepositoryID: 1})
			return err
		}},
		{"get_tag", "registry_get_tag", http.StatusNotFound, "tag names are case-sensitive", func(c *gitlabclient.Client) error {
			_, err := GetTag(context.Background(), c, GetTagInput{ProjectID: "1", RepositoryID: 1, TagName: "v1"})
			return err
		}},
		{"delete_tag", "registry_delete_tag", http.StatusForbidden, "requires Developer role", func(c *gitlabclient.Client) error {
			return DeleteTag(context.Background(), c, DeleteTagInput{ProjectID: "1", RepositoryID: 1, TagName: "v1"})
		}},
		{"delete_tags_bulk", "registry_delete_tags_bulk", http.StatusBadRequest, "deletion is async", func(c *gitlabclient.Client) error {
			return DeleteTagsBulk(context.Background(), c, DeleteTagsBulkInput{ProjectID: "1", RepositoryID: 1, NameRegexDelete: ".*"})
		}},
		{"list_protection_rules", "registry_protection_list", http.StatusNotFound, "requires GitLab 16.7+", func(c *gitlabclient.Client) error {
			_, err := ListProtectionRules(context.Background(), c, ListProtectionRulesInput{ProjectID: "1"})
			return err
		}},
		{"create_protection_rule", "registry_protection_create", http.StatusBadRequest, "repository_path_pattern must be a glob", func(c *gitlabclient.Client) error {
			_, err := CreateProtectionRule(context.Background(), c, CreateProtectionRuleInput{ProjectID: "1", RepositoryPathPattern: "x"})
			return err
		}},
		{"update_protection_rule", "registry_protection_update", http.StatusNotFound, "pattern uniqueness still applies on rename", func(c *gitlabclient.Client) error {
			_, err := UpdateProtectionRule(context.Background(), c, UpdateProtectionRuleInput{ProjectID: "1", RuleID: 1})
			return err
		}},
		{"delete_protection_rule", "registry_protection_delete", http.StatusNotFound, "managing protection rules requires Maintainer", func(c *gitlabclient.Client) error {
			return DeleteProtectionRule(context.Background(), c, DeleteProtectionRuleInput{ProjectID: "1", RuleID: 1})
		}},
		{"list_tag_protection_rules", "registry_tag_protection_list", http.StatusNotFound, "requires GitLab 17.8+", func(c *gitlabclient.Client) error {
			_, err := ListTagProtectionRules(context.Background(), c, ListTagProtectionRulesInput{ProjectID: "1"})
			return err
		}},
		{"create_tag_protection_rule", "registry_tag_protection_create", http.StatusBadRequest, "valid RE2 regular expression", func(c *gitlabclient.Client) error {
			_, err := CreateTagProtectionRule(context.Background(), c, CreateTagProtectionRuleInput{ProjectID: "1", TagNamePattern: "["})
			return err
		}},
		{"update_tag_protection_rule", "registry_tag_protection_update", http.StatusNotFound, "tag_name_pattern uniqueness still applies", func(c *gitlabclient.Client) error {
			_, err := UpdateTagProtectionRule(context.Background(), c, UpdateTagProtectionRuleInput{ProjectID: "1", RuleID: 1})
			return err
		}},
		{"delete_tag_protection_rule", "registry_tag_protection_delete", http.StatusNotFound, "managing tag protection rules requires Maintainer", func(c *gitlabclient.Client) error {
			return DeleteTagProtectionRule(context.Background(), c, DeleteTagProtectionRuleInput{ProjectID: "1", RuleID: 1})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tc.status, `{"message":"refused"}`)
			}))
			err := tc.call(client)
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.HasPrefix(err.Error(), tc.op+": ") {
				t.Errorf("error = %q, want it to open with the operation %q", err, tc.op)
			}
			if !strings.Contains(err.Error(), "Suggestion: ") || !strings.Contains(err.Error(), tc.hint) {
				t.Errorf("error = %q, want a suggestion containing %q", err, tc.hint)
			}
		})
	}
}

// TestDeleteTagsBulk_AllCriteria verifies that DeleteTagsBulk forwards every
// cleanup criterion, including the deprecated name_regex parameter, name_regex_keep,
// and older_than. It asserts each query parameter reaches the request unchanged.
func TestDeleteTagsBulk_AllCriteria(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/repositories/1/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf(fmtExpectedDELETE, r.Method)
		}
		q := r.URL.Query()
		if q.Get("name_regex_keep") != "release.*" {
			t.Errorf("name_regex_keep = %q, want release.*", q.Get("name_regex_keep"))
		}
		if q.Get("older_than") != "7d" {
			t.Errorf("older_than = %q, want 7d", q.Get("older_than"))
		}
		if q.Get("name_regex") != "old.*" {
			t.Errorf("name_regex = %q, want old.*", q.Get("name_regex"))
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteTagsBulk(context.Background(), client, DeleteTagsBulkInput{
		ProjectID:     toolutil.StringOrInt("10"),
		RepositoryID:  1,
		NameRegexKeep: "release.*",
		OlderThan:     "7d",
		NameRegex:     "old.*",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// What each request carries, one input at a time
// ---------------------------------------------------------------------------.

// registryQuery flattens the query string a handler sent GitLab. It returns
// the whole map rather than one key, because a parameter the handler dropped
// and a parameter it invented are both a wrong request and only the whole map
// shows the second.
func registryQuery(r *http.Request) map[string]string {
	got := map[string]string{}
	for k, v := range r.URL.Query() {
		got[k] = v[0]
	}
	return got
}

// registryRequestBody decodes the JSON body a handler sent GitLab, whole, for
// the same reason registryQuery returns the whole query.
func registryRequestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode body: %v", err)
		return nil
	}
	return body
}

// TestListProject_OneTagFlagAtATime_SendsThatFlagAndNothingElse drives one
// list per tag flag and compares the whole query. The two flags carry the same
// value, so a list that set both could not tell them apart: with tags and
// tags_count swapped in the handler, every existing test still passed.
func TestListProject_OneTagFlagAtATime_SendsThatFlagAndNothingElse(t *testing.T) {
	tests := []struct {
		name  string
		input ListProjectInput
		want  map[string]string
	}{
		{"nothing", ListProjectInput{ProjectID: "42"}, map[string]string{}},
		{"tags", ListProjectInput{ProjectID: "42", Tags: true}, map[string]string{"tags": "true"}},
		{"tags_count", ListProjectInput{ProjectID: "42", TagsCount: true}, map[string]string{"tags_count": "true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]string
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v4/projects/42/registry/repositories", func(w http.ResponseWriter, r *http.Request) {
				got = registryQuery(r)
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "1"})
			})
			client := testutil.NewTestClient(t, mux)

			if _, err := ListProject(context.Background(), client, tt.input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("query = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetRepository_OneTagFlagAtATime_SendsThatFlagAndNothingElse is the same
// property for the single-repository read, whose only test with the flags set
// never looked at the query at all.
func TestGetRepository_OneTagFlagAtATime_SendsThatFlagAndNothingElse(t *testing.T) {
	tests := []struct {
		name  string
		input GetRepositoryInput
		want  map[string]string
	}{
		{"nothing", GetRepositoryInput{RepositoryID: 99}, map[string]string{}},
		{"tags", GetRepositoryInput{RepositoryID: 99, Tags: true}, map[string]string{"tags": "true"}},
		{"tags_count", GetRepositoryInput{RepositoryID: 99, TagsCount: true}, map[string]string{"tags_count": "true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]string
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v4/registry/repositories/99", func(w http.ResponseWriter, r *http.Request) {
				got = registryQuery(r)
				testutil.RespondJSON(w, http.StatusOK, covRepoJSON)
			})
			client := testutil.NewTestClient(t, mux)

			if _, err := GetRepository(context.Background(), client, tt.input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("query = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDeleteTagsBulk_OneCriterionAtATime_SendsThatCriterionAndNothingElse
// drives one bulk delete per cleanup criterion and compares the whole query.
// The empty case is the one that matters most: a keep_n nobody set must not
// reach GitLab as keep_n=0, which asks it to keep no tag at all, and the guard
// that stops it could be loosened to >= 0 with nothing failing.
func TestDeleteTagsBulk_OneCriterionAtATime_SendsThatCriterionAndNothingElse(t *testing.T) {
	base := DeleteTagsBulkInput{ProjectID: "42", RepositoryID: 1}
	with := func(f func(*DeleteTagsBulkInput)) DeleteTagsBulkInput {
		in := base
		f(&in)
		return in
	}
	tests := []struct {
		name  string
		input DeleteTagsBulkInput
		want  map[string]string
	}{
		{"nothing", base, map[string]string{}},
		{"name_regex_delete", with(func(in *DeleteTagsBulkInput) { in.NameRegexDelete = "v.*" }), map[string]string{"name_regex_delete": "v.*"}},
		{"name_regex_keep", with(func(in *DeleteTagsBulkInput) { in.NameRegexKeep = "release.*" }), map[string]string{"name_regex_keep": "release.*"}},
		{"keep_n", with(func(in *DeleteTagsBulkInput) { in.KeepN = 5 }), map[string]string{"keep_n": "5"}},
		{"older_than", with(func(in *DeleteTagsBulkInput) { in.OlderThan = "7d" }), map[string]string{"older_than": "7d"}},
		{"name_regex", with(func(in *DeleteTagsBulkInput) { in.NameRegex = "old.*" }), map[string]string{"name_regex": "old.*"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]string
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v4/projects/42/registry/repositories/1/tags", func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				got = registryQuery(r)
				w.WriteHeader(http.StatusNoContent)
			})
			client := testutil.NewTestClient(t, mux)

			if err := DeleteTagsBulk(context.Background(), client, tt.input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("query = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// What a caller is handed, field for field
// ---------------------------------------------------------------------------.

// covRepoFullJSON is a repository carrying every field the output publishes,
// the two the capture reads beside the decode included, with no two values
// alike, so a field filled from its neighbor is a mismatch rather than a
// coincidence.
const covRepoFullJSON = `{
	"id":100,"name":"cov-img","path":"group/project/cov-img",
	"project_id":42,"location":"registry.example.com/group/project/cov-img",
	"tags_count":3,"status":"delete_scheduled",
	"created_at":"2026-01-15T10:00:00Z",
	"cleanup_policy_started_at":"2026-01-16T12:00:00Z",
	"size":65536,"delete_api_path":"/api/v4/projects/42/registry/repositories/100",
	"tags":[` + covTagJSON + `]
}`

// TestGetRepository_PublishesEveryFieldGitLabSent verifies the whole output a
// caller is handed for a fully populated repository, through the handler
// rather than the converter, so the capture of size and delete_api_path is
// part of what is asserted. The two timestamps are the pair that used to be
// checked as non-empty only, and a converter that swapped them passed.
func TestGetRepository_PublishesEveryFieldGitLabSent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/registry/repositories/100", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covRepoFullJSON)
	})
	client := testutil.NewTestClient(t, mux)

	got, err := GetRepository(context.Background(), client, GetRepositoryInput{RepositoryID: 100, Tags: true})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := RepositoryOutput{
		ID: 100, Name: "cov-img", Path: "group/project/cov-img", ProjectID: 42,
		Location:               "registry.example.com/group/project/cov-img",
		CreatedAt:              "2026-01-15T10:00:00Z",
		CleanupPolicyStartedAt: "2026-01-16T12:00:00Z",
		Status:                 "delete_scheduled",
		TagsCount:              3,
		Size:                   65536,
		DeleteAPIPath:          "/api/v4/projects/42/registry/repositories/100",
		Tags:                   []TagOutput{covTagOutput()},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetRepository() =\n%#v\nwant:\n%#v", got, want)
	}
}

// covTagOutput is what covTagJSON decodes and converts to.
func covTagOutput() TagOutput {
	return TagOutput{
		Name: "v1.0", Path: "group/project/cov-img:v1.0",
		Location: "registry.example.com/group/project/cov-img:v1.0",
		Revision: "abc123", ShortRevision: "abc1", Digest: "sha256:deadbeef",
		CreatedAt: "2026-02-01T08:00:00Z", TotalSize: 4096,
	}
}

// TestGetTag_PublishesEveryFieldGitLabSent verifies the whole output a caller
// is handed for a fully populated tag. The revision and the short revision are
// the pair a swap would confuse, and no test read either before this one.
func TestGetTag_PublishesEveryFieldGitLabSent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/registry/repositories/100/tags/v1.0", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covTagJSON)
	})
	client := testutil.NewTestClient(t, mux)

	got, err := GetTag(context.Background(), client, GetTagInput{ProjectID: "42", RepositoryID: 100, TagName: "v1.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if want := covTagOutput(); !reflect.DeepEqual(got, want) {
		t.Errorf("GetTag() =\n%#v\nwant:\n%#v", got, want)
	}
}

// TestRegistryOptions_ToolWithoutAUsageLine_FallsBackToTheDomainUsage verifies
// an action added to the family before its own usage line is written is served
// the domain's usage rather than none, beside the shared metadata every
// registry action carries. Every tool served today has a line of its own, so
// this is the only way the fallback is ever reached.
func TestRegistryOptions_ToolWithoutAUsageLine_FallsBackToTheDomainUsage(t *testing.T) {
	const tool = "gitlab_registry_not_yet_described"
	got := registryOptions(tool)
	if got.Usage != "Manage container registry repositories, tags, and protection rules for projects or groups." {
		t.Errorf("Usage = %q, want the domain usage", got.Usage)
	}
	if got.IndividualTool.Name != tool || got.OwnerPackage != "containerregistry" || len(got.Tags) == 0 {
		t.Errorf("shared metadata = %+v, want the tool name, the owner package and the tags", got)
	}
}
