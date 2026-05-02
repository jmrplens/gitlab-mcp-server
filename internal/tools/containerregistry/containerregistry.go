package containerregistry

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// RepositoryOutput represents a container registry repository.
type RepositoryOutput struct {
	toolutil.HintableOutput
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	Path                   string `json:"path"`
	ProjectID              int64  `json:"project_id"`
	Location               string `json:"location"`
	CreatedAt              string `json:"created_at,omitempty"`
	CleanupPolicyStartedAt string `json:"cleanup_policy_started_at,omitempty"`
	Status                 string `json:"status,omitempty"`
	TagsCount              int64  `json:"tags_count"`
}

// RepositoryListOutput represents a paginated list of registry repositories.
type RepositoryListOutput struct {
	toolutil.HintableOutput
	Repositories []RepositoryOutput        `json:"repositories"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// TagOutput represents a container registry image tag.
type TagOutput struct {
	toolutil.HintableOutput
	Name          string `json:"name"`
	Path          string `json:"path"`
	Location      string `json:"location"`
	Revision      string `json:"revision,omitempty"`
	ShortRevision string `json:"short_revision,omitempty"`
	Digest        string `json:"digest,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	TotalSize     int64  `json:"total_size"`
}

// TagListOutput represents a paginated list of registry tags.
type TagListOutput struct {
	toolutil.HintableOutput
	Tags       []TagOutput               `json:"tags"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// convertRepository is an internal helper for the containerregistry package.
func convertRepository(r *gl.RegistryRepository) RepositoryOutput {
	o := RepositoryOutput{
		ID:        r.ID,
		Name:      r.Name,
		Path:      r.Path,
		ProjectID: r.ProjectID,
		Location:  r.Location,
		TagsCount: r.TagsCount,
	}
	if r.CreatedAt != nil {
		o.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	if r.CleanupPolicyStartedAt != nil {
		o.CleanupPolicyStartedAt = r.CleanupPolicyStartedAt.Format(time.RFC3339)
	}
	if r.Status != nil {
		o.Status = string(*r.Status)
	}
	return o
}

// convertTag is an internal helper for the containerregistry package.
func convertTag(t *gl.RegistryRepositoryTag) TagOutput {
	o := TagOutput{
		Name:          t.Name,
		Path:          t.Path,
		Location:      t.Location,
		Revision:      t.Revision,
		ShortRevision: t.ShortRevision,
		Digest:        t.Digest,
		TotalSize:     t.TotalSize,
	}
	if t.CreatedAt != nil {
		o.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	return o
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ListProjectRegistryRepositories
// ---------------------------------------------------------------------------.

// ListProjectInput represents the input for listing project registry repositories.
type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Tags      bool                 `json:"tags,omitempty" jsonschema:"Include tags in response"`
	TagsCount bool                 `json:"tags_count,omitempty" jsonschema:"Include tags count in response"`
	Page      int                  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage   int                  `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// ListProject lists container registry repositories for a project.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (RepositoryListOutput, error) {
	if input.ProjectID == "" {
		return RepositoryListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.ListProjectRegistryRepositoriesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	if input.Tags {
		opts.Tags = new(true)
	}
	if input.TagsCount {
		opts.TagsCount = new(true)
	}
	repos, resp, err := client.GL().ContainerRegistry.ListProjectRegistryRepositories(
		string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return RepositoryListOutput{}, toolutil.WrapErrWithStatusHint("registry_list_project", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; the project may have container registry disabled or no repositories yet")
	}
	out := RepositoryListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, r := range repos {
		out.Repositories = append(out.Repositories, convertRepository(r))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// ListGroupRegistryRepositories
// ---------------------------------------------------------------------------.

// ListGroupInput represents the input for listing group registry repositories.
type ListGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	Page    int                  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int                  `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// ListGroup lists container registry repositories for a group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (RepositoryListOutput, error) {
	if input.GroupID == "" {
		return RepositoryListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListGroupRegistryRepositoriesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	repos, resp, err := client.GL().ContainerRegistry.ListGroupRegistryRepositories(
		string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return RepositoryListOutput{}, toolutil.WrapErrWithStatusHint("registry_list_group", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; the group may have no projects with container registry enabled")
	}
	out := RepositoryListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, r := range repos {
		out.Repositories = append(out.Repositories, convertRepository(r))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// GetSingleRegistryRepository
// ---------------------------------------------------------------------------.

// GetRepositoryInput represents the input for getting a single registry repository.
type GetRepositoryInput struct {
	RepositoryID int64 `json:"repository_id" jsonschema:"Registry repository ID,required"`
	Tags         bool  `json:"tags,omitempty" jsonschema:"Include tags in response"`
	TagsCount    bool  `json:"tags_count,omitempty" jsonschema:"Include tags count in response"`
}

// GetRepository gets details of a single registry repository by its ID.
func GetRepository(ctx context.Context, client *gitlabclient.Client, input GetRepositoryInput) (RepositoryOutput, error) {
	if input.RepositoryID == 0 {
		return RepositoryOutput{}, toolutil.ErrFieldRequired("repository_id")
	}
	opts := &gl.GetSingleRegistryRepositoryOptions{}
	if input.Tags {
		opts.Tags = new(true)
	}
	if input.TagsCount {
		opts.TagsCount = new(true)
	}
	repo, _, err := client.GL().ContainerRegistry.GetSingleRegistryRepository(
		input.RepositoryID, opts, gl.WithContext(ctx))
	if err != nil {
		return RepositoryOutput{}, toolutil.WrapErrWithStatusHint("registry_get_repository", err, http.StatusNotFound,
			"verify repository_id with gitlab_registry_list_project; container repositories must be queried by ID, not name")
	}
	return convertRepository(repo), nil
}

// ---------------------------------------------------------------------------
// DeleteRegistryRepository
// ---------------------------------------------------------------------------.

// DeleteRepositoryInput represents the input for deleting a registry repository.
type DeleteRepositoryInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	RepositoryID int64                `json:"repository_id" jsonschema:"Registry repository ID,required"`
}

// DeleteRepository deletes a container registry repository.
func DeleteRepository(ctx context.Context, client *gitlabclient.Client, input DeleteRepositoryInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.RepositoryID == 0 {
		return toolutil.ErrFieldRequired("repository_id")
	}
	_, err := client.GL().ContainerRegistry.DeleteRegistryRepository(
		string(input.ProjectID), input.RepositoryID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("registry_delete_repository", err, http.StatusForbidden,
			"deleting container repositories requires Maintainer role or higher; verify repository_id with gitlab_registry_list_project")
	}
	return nil
}

// ---------------------------------------------------------------------------
// ListRegistryRepositoryTags
// ---------------------------------------------------------------------------.

// ListTagsInput represents the input for listing registry repository tags.
type ListTagsInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	RepositoryID int64                `json:"repository_id" jsonschema:"Registry repository ID,required"`
	Page         int                  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage      int                  `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// ListTags lists tags for a registry repository.
func ListTags(ctx context.Context, client *gitlabclient.Client, input ListTagsInput) (TagListOutput, error) {
	if input.ProjectID == "" {
		return TagListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.RepositoryID == 0 {
		return TagListOutput{}, toolutil.ErrFieldRequired("repository_id")
	}
	opts := &gl.ListRegistryRepositoryTagsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	tags, resp, err := client.GL().ContainerRegistry.ListRegistryRepositoryTags(
		string(input.ProjectID), input.RepositoryID, opts, gl.WithContext(ctx))
	if err != nil {
		return TagListOutput{}, toolutil.WrapErrWithStatusHint("registry_list_tags", err, http.StatusNotFound,
			"verify repository_id with gitlab_registry_list_project; the repository may have no tags or be in the process of being created")
	}
	out := TagListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, t := range tags {
		out.Tags = append(out.Tags, convertTag(t))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// GetRegistryRepositoryTagDetail
// ---------------------------------------------------------------------------.

// GetTagInput represents the input for getting a registry tag detail.
type GetTagInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	RepositoryID int64                `json:"repository_id" jsonschema:"Registry repository ID,required"`
	TagName      string               `json:"tag_name" jsonschema:"Tag name,required"`
}

// GetTag gets details of a specific registry repository tag.
func GetTag(ctx context.Context, client *gitlabclient.Client, input GetTagInput) (TagOutput, error) {
	if input.ProjectID == "" {
		return TagOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.RepositoryID == 0 {
		return TagOutput{}, toolutil.ErrFieldRequired("repository_id")
	}
	if input.TagName == "" {
		return TagOutput{}, toolutil.ErrFieldRequired("tag_name")
	}
	tag, _, err := client.GL().ContainerRegistry.GetRegistryRepositoryTagDetail(
		string(input.ProjectID), input.RepositoryID, input.TagName, gl.WithContext(ctx))
	if err != nil {
		return TagOutput{}, toolutil.WrapErrWithStatusHint("registry_get_tag", err, http.StatusNotFound,
			"verify tag_name with gitlab_registry_list_tags; tag names are case-sensitive and must match exactly")
	}
	return convertTag(tag), nil
}

// ---------------------------------------------------------------------------
// DeleteRegistryRepositoryTag
// ---------------------------------------------------------------------------.

// DeleteTagInput represents the input for deleting a single registry tag.
type DeleteTagInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	RepositoryID int64                `json:"repository_id" jsonschema:"Registry repository ID,required"`
	TagName      string               `json:"tag_name" jsonschema:"Tag name to delete,required"`
}

// DeleteTag deletes a single registry repository tag.
func DeleteTag(ctx context.Context, client *gitlabclient.Client, input DeleteTagInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.RepositoryID == 0 {
		return toolutil.ErrFieldRequired("repository_id")
	}
	if input.TagName == "" {
		return toolutil.ErrFieldRequired("tag_name")
	}
	_, err := client.GL().ContainerRegistry.DeleteRegistryRepositoryTag(
		string(input.ProjectID), input.RepositoryID, input.TagName, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("registry_delete_tag", err, http.StatusForbidden,
			"deleting registry tags requires Developer role or higher; verify tag_name with gitlab_registry_list_tags")
	}
	return nil
}

// ---------------------------------------------------------------------------
// DeleteRegistryRepositoryTags (bulk)
// ---------------------------------------------------------------------------.

// DeleteTagsBulkInput represents the input for bulk deleting registry tags.
type DeleteTagsBulkInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	RepositoryID    int64                `json:"repository_id" jsonschema:"Registry repository ID,required"`
	NameRegexDelete string               `json:"name_regex_delete,omitempty" jsonschema:"Regex pattern of tag names to delete"`
	NameRegexKeep   string               `json:"name_regex_keep,omitempty" jsonschema:"Regex pattern of tag names to keep"`
	KeepN           int64                `json:"keep_n,omitempty" jsonschema:"Number of latest tags to keep"`
	OlderThan       string               `json:"older_than,omitempty" jsonschema:"Delete tags older than this (e.g. 1h, 2d, 1month)"`
}

// DeleteTagsBulk deletes registry repository tags in bulk using regex patterns.
func DeleteTagsBulk(ctx context.Context, client *gitlabclient.Client, input DeleteTagsBulkInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.RepositoryID == 0 {
		return toolutil.ErrFieldRequired("repository_id")
	}
	opts := &gl.DeleteRegistryRepositoryTagsOptions{}
	if input.NameRegexDelete != "" {
		opts.NameRegexpDelete = new(input.NameRegexDelete)
	}
	if input.NameRegexKeep != "" {
		opts.NameRegexpKeep = new(input.NameRegexKeep)
	}
	if input.KeepN > 0 {
		opts.KeepN = new(input.KeepN)
	}
	if input.OlderThan != "" {
		opts.OlderThan = new(input.OlderThan)
	}
	_, err := client.GL().ContainerRegistry.DeleteRegistryRepositoryTags(
		string(input.ProjectID), input.RepositoryID, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("registry_delete_tags_bulk", err, http.StatusBadRequest,
			"name_regex_delete is required and must be a valid regex; older_than format like '7d' or '1month'; deletion is async and may not be immediate")
	}
	return nil
}
