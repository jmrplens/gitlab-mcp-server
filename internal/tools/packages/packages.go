package packages

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	fmtCtxCancelled = "context canceled: %w"
	fmtPkgPublish   = "packagePublish: %w"
)

// Publish.

// PublishInput defines input for publishing a file to the Generic Package Registry.
type PublishInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageName    string               `json:"package_name" jsonschema:"Package name (alphanumeric, dots, dashes, underscores),required"`
	PackageVersion string               `json:"package_version" jsonschema:"Package version (e.g. 1.0.0),required"`
	FileName       string               `json:"file_name" jsonschema:"Name of the file within the package,required"`
	FilePath       string               `json:"file_path,omitempty" jsonschema:"Absolute path to a local file. Alternative to content_base64. Only one of file_path or content_base64 should be provided."`
	ContentBase64  string               `json:"content_base64,omitempty" jsonschema:"Base64-encoded file content. Only one should be provided."`
	Status         string               `json:"status,omitempty" jsonschema:"Package status: default or hidden"`
	Select         string               `json:"select,omitempty" jsonschema:"Response detail selector. Set to 'package_file' to receive the published file metadata (id, size, checksums). Defaults to 'package_file'."`
}

// PublishOutput contains the result of a package file publish operation.
type PublishOutput struct {
	toolutil.HintableOutput
	PackageFileID int64  `json:"package_file_id"`
	PackageID     int64  `json:"package_id"`
	FileName      string `json:"file_name"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256"`
	FileMD5       string `json:"file_md5,omitempty"`
	FileSHA1      string `json:"file_sha1,omitempty"`
	FileStore     int64  `json:"file_store"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	URL           string `json:"url"`
}

// validatePublishInput checks all required fields and the mutual
// exclusion between [PublishInput.FilePath] and
// [PublishInput.ContentBase64] for [Publish]. Returns a wrapped error
// that includes the ctx.Err() when the context is cancelled.
func validatePublishInput(ctx context.Context, input PublishInput) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return errors.New("packagePublish: project_id is required")
	}
	if err := toolutil.ValidatePackageName(input.PackageName); err != nil {
		return fmt.Errorf(fmtPkgPublish, err)
	}
	if input.PackageVersion == "" {
		return errors.New("packagePublish: package_version is required")
	}
	if err := toolutil.ValidatePackageFileName(input.FileName); err != nil {
		return fmt.Errorf(fmtPkgPublish, err)
	}

	hasFile := input.FilePath != ""
	hasBase64 := input.ContentBase64 != ""
	if hasFile && hasBase64 {
		return errors.New("packagePublish: provide either file_path or content_base64, not both")
	}
	if !hasFile && !hasBase64 {
		return errors.New("packagePublish: either file_path or content_base64 is required")
	}
	return nil
}

// Publish publishes a file to the GitLab Generic Package Registry.
func Publish(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input PublishInput) (PublishOutput, error) {
	if err := validatePublishInput(ctx, input); err != nil {
		return PublishOutput{}, err
	}

	reader, fileSize, cleanup, err := toolutil.OpenFileOrBase64Source("packagePublish", input.FilePath, input.ContentBase64)
	if err != nil {
		return PublishOutput{}, err
	}
	defer cleanup()

	tracker := progress.FromRequest(req)
	if tracker.IsActive() {
		reader = toolutil.NewProgressReader(ctx, reader, fileSize, tracker)
	}

	// Default to select=package_file so the response carries the
	// published file metadata (id, size, checksums); honor an explicit
	// select override when supplied.
	selectVal := gl.SelectPackageFile
	if input.Select != "" {
		selectVal = gl.GenericPackageSelectValue(input.Select)
	}

	published, _, err := client.GL().GenericPackages.PublishPackageFile(
		string(input.ProjectID),
		input.PackageName,
		input.PackageVersion,
		input.FileName,
		reader,
		&gl.PublishPackageFileOptions{
			Status: (*gl.GenericPackageStatusValue)(ptrString(input.Status)),
			Select: &selectVal,
		},
	)
	if err != nil {
		return PublishOutput{}, toolutil.WrapErrWithStatusHint("packagePublish", err, http.StatusBadRequest,
			"package_name and package_version must match pattern [A-Za-z0-9.\\-_]+ with version following SemVer; verify file_name does not already exist in this package or use status=hidden to override")
	}

	var pkgURL string
	pkgPath, err := client.GL().GenericPackages.FormatPackageURL(
		string(input.ProjectID),
		input.PackageName,
		input.PackageVersion,
		input.FileName,
	)
	if err == nil {
		pkgURL = strings.TrimRight(client.GL().BaseURL().String(), "/") + "/" + pkgPath
	}

	out := PublishOutput{
		PackageFileID: published.ID,
		PackageID:     published.PackageID,
		FileName:      published.FileName,
		Size:          published.Size,
		SHA256:        published.FileSHA256,
		FileMD5:       published.FileMD5,
		FileSHA1:      published.FileSHA1,
		FileStore:     published.FileStore,
		URL:           pkgURL,
	}
	if published.CreatedAt != nil {
		out.CreatedAt = published.CreatedAt.String()
	}
	if published.UpdatedAt != nil {
		out.UpdatedAt = published.UpdatedAt.String()
	}
	return out, nil
}

// ptrString returns a pointer to s, or nil if s is empty. Used to
// convert optional string fields to the *string shape expected by
// the GitLab client-go options.
func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Download.

// DownloadInput defines input for downloading a package file.
type DownloadInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageName    string               `json:"package_name" jsonschema:"Package name,required"`
	PackageVersion string               `json:"package_version" jsonschema:"Package version,required"`
	FileName       string               `json:"file_name" jsonschema:"File name to download,required"`
	OutputPath     string               `json:"output_path" jsonschema:"Absolute path where the file will be saved on the local filesystem,required"`
}

// DownloadOutput contains the result of a package file download.
type DownloadOutput struct {
	toolutil.HintableOutput
	OutputPath string `json:"output_path"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
}

// Download downloads a single file from the GitLab Generic Package
// Registry via the GitLab Packages download API
// (GET /projects/:id/packages/generic/:package_name/:package_version/:file_name).
// The file is streamed to the local OutputPath, with the resulting
// size and SHA256 surfaced in the response.
func Download(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input DownloadInput) (DownloadOutput, error) {
	if err := ctx.Err(); err != nil {
		return DownloadOutput{}, fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return DownloadOutput{}, errors.New("packageDownload: project_id is required")
	}
	if input.PackageName == "" {
		return DownloadOutput{}, errors.New("packageDownload: package_name is required")
	}
	if input.PackageVersion == "" {
		return DownloadOutput{}, errors.New("packageDownload: package_version is required")
	}
	if input.FileName == "" {
		return DownloadOutput{}, errors.New("packageDownload: file_name is required")
	}
	if input.OutputPath == "" {
		return DownloadOutput{}, errors.New("packageDownload: output_path is required")
	}

	// Stream directly to disk — avoids loading the entire file into memory.
	n, checksum, err := streamDownloadPackageFile(ctx, req, client, input)
	if err != nil {
		return DownloadOutput{}, err
	}

	return DownloadOutput{
		OutputPath: input.OutputPath,
		Size:       n,
		SHA256:     checksum,
	}, nil
}

// List Packages.

// ListInput defines input for listing project packages.
type ListInput struct {
	ProjectID          toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageName        string               `json:"package_name,omitempty" jsonschema:"Filter by package name"`
	PackageVersion     string               `json:"package_version,omitempty" jsonschema:"Filter by package version"`
	PackageType        string               `json:"package_type,omitempty" jsonschema:"Filter by type (generic, npm, maven, etc.)"`
	OrderBy            string               `json:"order_by,omitempty" jsonschema:"Order by: name, created_at, version, type"`
	Sort               string               `json:"sort,omitempty" jsonschema:"Sort direction: asc or desc"`
	IncludeVersionless bool                 `json:"include_versionless,omitempty" jsonschema:"Include versionless packages"`
	Status             string               `json:"status,omitempty" jsonschema:"Filter by status: default, hidden, processing, error"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListItem represents a single package in the list output. It mirrors
// the GitLab Packages API [gl.Package] object, surfacing the full
// nested _links, pipeline, pipelines, and tags sub-objects.
type ListItem struct {
	ID               int64          `json:"id"`
	Name             string         `json:"name"`
	Version          string         `json:"version"`
	PackageType      string         `json:"package_type"`
	Status           string         `json:"status"`
	Links            *LinksItem     `json:"_links,omitempty"`
	Pipeline         *PipelineItem  `json:"pipeline,omitempty"`
	Pipelines        []PipelineItem `json:"pipelines,omitempty"`
	CreatedAt        string         `json:"created_at,omitempty"`
	LastDownloadedAt string         `json:"last_downloaded_at,omitempty"`
	Tags             []TagItem      `json:"tags,omitempty"`
}

// LinksItem mirrors the GitLab Packages API [gl.PackageLinks] object,
// holding the package's web path and delete API path.
type LinksItem struct {
	WebPath       string `json:"web_path,omitempty"`
	DeleteAPIPath string `json:"delete_api_path,omitempty"`
}

// TagItem mirrors the GitLab Packages API [gl.PackageTag] object.
type TagItem struct {
	ID        int64  `json:"id"`
	PackageID int64  `json:"package_id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// PipelineItem represents CI pipeline metadata attached to a package.
type PipelineItem struct {
	ID        int64         `json:"id"`
	Status    string        `json:"status"`
	Ref       string        `json:"ref"`
	SHA       string        `json:"sha"`
	WebURL    string        `json:"web_url"`
	CreatedAt string        `json:"created_at,omitempty"`
	UpdatedAt string        `json:"updated_at,omitempty"`
	User      *PipelineUser `json:"user,omitempty"`
}

// PipelineUser mirrors the GitLab [gl.BasicUser] object attached to
// package pipeline metadata.
type PipelineUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at,omitempty"`
}

// ListOutput contains the paginated list of packages.
type ListOutput struct {
	toolutil.HintableOutput
	Packages   []ListItem                `json:"packages"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// buildListOptions translates ListInput filter fields into GitLab API options.
// buildListOptions assembles a [gl.ListProjectPackagesOptions] from
// the shared [ListInput] filter set, mapping the MCP-friendly
// OrderBy/Sort/Status values to the GitLab Packages API fields.
func buildListOptions(input ListInput) *gl.ListProjectPackagesOptions {
	opts := &gl.ListProjectPackagesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.PackageName != "" {
		opts.PackageName = &input.PackageName
	}
	if input.PackageVersion != "" {
		opts.PackageVersion = &input.PackageVersion
	}
	if input.PackageType != "" {
		opts.PackageType = &input.PackageType
	}
	if input.OrderBy != "" {
		opts.OrderBy = &input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}
	if input.IncludeVersionless {
		opts.IncludeVersionless = new(true)
	}
	if input.Status != "" {
		opts.Status = &input.Status
	}
	return opts
}

// packageToListItem converts a GitLab Package API object into a ListItem.
// packageToListItem converts a [gl.Package] into the package's
// [ListItem] shape, flattening the optional pipeline metadata.
func packageToListItem(p *gl.Package) ListItem {
	item := ListItem{
		ID:          p.ID,
		Name:        p.Name,
		Version:     p.Version,
		PackageType: p.PackageType,
		Status:      p.Status,
	}
	if p.Pipeline != nil {
		item.Pipeline = packagePipelineToOutput(p.Pipeline)
	}
	if len(p.Pipelines) > 0 {
		item.Pipelines = make([]PipelineItem, 0, len(p.Pipelines))
		for _, pipeline := range p.Pipelines {
			pipelineItem := packagePipelineToOutput(pipeline)
			if pipelineItem == nil {
				continue
			}
			item.Pipelines = append(item.Pipelines, *pipelineItem)
		}
	}
	if p.CreatedAt != nil {
		item.CreatedAt = p.CreatedAt.String()
	}
	if p.LastDownloadedAt != nil {
		item.LastDownloadedAt = p.LastDownloadedAt.String()
	}
	if len(p.Tags) > 0 {
		tags := make([]TagItem, 0, len(p.Tags))
		for _, tag := range p.Tags {
			tagItem := TagItem{
				ID:        tag.ID,
				PackageID: tag.PackageID,
				Name:      tag.Name,
			}
			if tag.CreatedAt != nil {
				tagItem.CreatedAt = tag.CreatedAt.String()
			}
			if tag.UpdatedAt != nil {
				tagItem.UpdatedAt = tag.UpdatedAt.String()
			}
			tags = append(tags, tagItem)
		}
		item.Tags = tags
	}
	if p.Links != nil {
		item.Links = &LinksItem{
			WebPath:       p.Links.WebPath,
			DeleteAPIPath: p.Links.DeleteAPIPath,
		}
	}
	return item
}

// packagePipelineToOutput converts GitLab package pipeline metadata.
// packagePipelineToOutput converts a [gl.PackagePipeline] into the
// package's [PipelineItem] shape, or nil when the pipeline pointer
// is nil.
func packagePipelineToOutput(pipeline *gl.PackagePipeline) *PipelineItem {
	if pipeline == nil {
		return nil
	}
	item := &PipelineItem{
		ID:     pipeline.ID,
		Status: pipeline.Status,
		Ref:    pipeline.Ref,
		SHA:    pipeline.SHA,
		WebURL: pipeline.WebURL,
	}
	if pipeline.CreatedAt != nil {
		item.CreatedAt = pipeline.CreatedAt.String()
	}
	if pipeline.UpdatedAt != nil {
		item.UpdatedAt = pipeline.UpdatedAt.String()
	}
	if pipeline.User != nil {
		user := &PipelineUser{
			ID:        pipeline.User.ID,
			Username:  pipeline.User.Username,
			Name:      pipeline.User.Name,
			State:     pipeline.User.State,
			AvatarURL: pipeline.User.AvatarURL,
			WebURL:    pipeline.User.WebURL,
		}
		if pipeline.User.CreatedAt != nil {
			user.CreatedAt = pipeline.User.CreatedAt.String()
		}
		item.User = user
	}
	return item
}

// List retrieves a paginated list of Generic Package Registry
// packages in a project via the GitLab Packages list API
// (GET /projects/:id/packages). Optional filters narrow by package
// name, version, status, and the [buildListOptions] sort field.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("packageList: project_id is required")
	}

	pkgs, resp, err := client.GL().Packages.ListProjectPackages(string(input.ProjectID), buildListOptions(input))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("packageList", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; the project may have no packages yet or package registry may be disabled")
	}

	items := make([]ListItem, 0, len(pkgs))
	for _, p := range pkgs {
		items = append(items, packageToListItem(p))
	}

	return ListOutput{
		Packages:   items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// List Group Packages.

// GroupListInput defines input for listing packages across a group and
// its descendant projects.
type GroupListInput struct {
	GroupID            toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	ExcludeSubgroups   bool                 `json:"exclude_subgroups,omitempty" jsonschema:"Exclude packages from subgroups (only return packages from the group's direct projects)"`
	PackageName        string               `json:"package_name,omitempty" jsonschema:"Filter by package name"`
	PackageType        string               `json:"package_type,omitempty" jsonschema:"Filter by type (generic, npm, maven, etc.)"`
	OrderBy            string               `json:"order_by,omitempty" jsonschema:"Order by: name, created_at, version, type, project_path"`
	Sort               string               `json:"sort,omitempty" jsonschema:"Sort direction: asc or desc"`
	IncludeVersionless bool                 `json:"include_versionless,omitempty" jsonschema:"Include versionless packages"`
	Status             string               `json:"status,omitempty" jsonschema:"Filter by status: default, hidden, processing, error"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GroupListItem represents a single package in the group list output.
// It mirrors the GitLab Packages API [gl.GroupPackage] object, which
// embeds the project-scoped [gl.Package] fields and adds the owning
// project's ID and path.
type GroupListItem struct {
	ListItem
	ProjectID   int64  `json:"project_id"`
	ProjectPath string `json:"project_path,omitempty"`
}

// GroupListOutput contains the paginated list of group packages.
type GroupListOutput struct {
	toolutil.HintableOutput
	Packages   []GroupListItem           `json:"packages"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// buildGroupListOptions assembles a [gl.ListGroupPackagesOptions] from
// the shared [GroupListInput] filter set, mapping the MCP-friendly
// OrderBy/Sort/Status values and the exclude_subgroups flag to the
// GitLab group Packages API fields.
func buildGroupListOptions(input GroupListInput) *gl.ListGroupPackagesOptions {
	opts := &gl.ListGroupPackagesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.ExcludeSubgroups {
		opts.ExcludeSubGroups = new(true)
	}
	if input.PackageName != "" {
		opts.PackageName = &input.PackageName
	}
	if input.PackageType != "" {
		opts.PackageType = &input.PackageType
	}
	if input.OrderBy != "" {
		opts.OrderBy = &input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}
	if input.IncludeVersionless {
		opts.IncludeVersionless = new(true)
	}
	if input.Status != "" {
		opts.Status = &input.Status
	}
	return opts
}

// GroupList retrieves a paginated list of packages across a group and
// its descendant projects via the GitLab group Packages list API
// (GET /groups/:id/packages). Each item carries the owning project's ID
// and path in addition to the project-scoped package fields.
func GroupList(ctx context.Context, client *gitlabclient.Client, input GroupListInput) (GroupListOutput, error) {
	if err := ctx.Err(); err != nil {
		return GroupListOutput{}, fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.GroupID == "" {
		return GroupListOutput{}, errors.New("packageGroupList: group_id is required")
	}

	pkgs, resp, err := client.GL().Packages.ListGroupPackages(string(input.GroupID), buildGroupListOptions(input), gl.WithContext(ctx))
	if err != nil {
		return GroupListOutput{}, toolutil.WrapErrWithStatusHint("packageGroupList", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; the group may have no packages yet or package registry may be disabled")
	}

	items := make([]GroupListItem, 0, len(pkgs))
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		items = append(items, GroupListItem{
			ListItem:    packageToListItem(&p.Package),
			ProjectID:   p.ProjectID,
			ProjectPath: p.ProjectPath,
		})
	}

	return GroupListOutput{
		Packages:   items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// List Package Files.

// FileListInput defines input for listing files within a package.
type FileListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageID toolutil.StringOrInt `json:"package_id" jsonschema:"Package ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order package files by: created_at, file_name, or id"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// FileListItem represents a single file within a package.
type FileListItem struct {
	PackageFileID int64  `json:"package_file_id"`
	PackageID     int64  `json:"package_id"`
	FileName      string `json:"file_name"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256"`
	FileMD5       string `json:"file_md5,omitempty"`
	FileSHA1      string `json:"file_sha1,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// FileListOutput contains the paginated list of package files.
type FileListOutput struct {
	toolutil.HintableOutput
	Files      []FileListItem            `json:"files"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// FileList lists the files within a single Generic Package Registry
// package via the GitLab Packages file list API
// (GET /projects/:id/packages/:package_id/package_files).
func FileList(ctx context.Context, client *gitlabclient.Client, input FileListInput) (FileListOutput, error) {
	if err := ctx.Err(); err != nil {
		return FileListOutput{}, fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return FileListOutput{}, errors.New("packageFileList: project_id is required")
	}
	pkgID, err := input.PackageID.Int64()
	if err != nil || pkgID <= 0 {
		return FileListOutput{}, errors.New("packageFileList: package_id must be a positive integer")
	}

	opts := &gl.ListPackageFilesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}

	files, resp, err := client.GL().Packages.ListPackageFiles(string(input.ProjectID), pkgID, opts)
	if err != nil {
		return FileListOutput{}, toolutil.WrapErrWithStatusHint("packageFileList", err, http.StatusNotFound,
			"verify package_id with gitlab_package_list; the package may have been deleted")
	}

	items := make([]FileListItem, 0, len(files))
	for _, f := range files {
		item := FileListItem{
			PackageFileID: f.ID,
			PackageID:     f.PackageID,
			FileName:      f.FileName,
			Size:          f.Size,
			SHA256:        f.FileSHA256,
			FileMD5:       f.FileMD5,
			FileSHA1:      f.FileSHA1,
		}
		if f.CreatedAt != nil {
			item.CreatedAt = f.CreatedAt.String()
		}
		items = append(items, item)
	}

	return FileListOutput{
		Files:      items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Delete Package.

// DeleteInput defines input for deleting a package.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageID toolutil.StringOrInt `json:"package_id" jsonschema:"Package ID to delete,required"`
}

// Delete deletes a single Generic Package Registry package via the
// GitLab Packages API (DELETE /projects/:id/packages/:package_id).
// Requires Maintainer or Owner role on the project.
func Delete(ctx context.Context, _ *mcp.CallToolRequest, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return errors.New("packageDelete: project_id is required")
	}
	pkgID, err := input.PackageID.Int64()
	if err != nil || pkgID <= 0 {
		return errors.New("packageDelete: package_id must be a positive integer")
	}

	_, err = client.GL().Packages.DeleteProjectPackage(string(input.ProjectID), pkgID)
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return fmt.Errorf("packageDelete: package deletion requires Maintainer role or higher. Your current role may only allow publishing. Contact a project Maintainer to delete packages: %w", err)
		}
		return toolutil.WrapErrWithStatusHint("packageDelete", err, http.StatusNotFound,
			"verify package_id with gitlab_package_list; the package may already have been deleted")
	}
	return nil
}

// Delete Package File.

// FileDeleteInput defines input for deleting a single file from a package.
type FileDeleteInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageID     toolutil.StringOrInt `json:"package_id" jsonschema:"Package ID,required"`
	PackageFileID toolutil.StringOrInt `json:"package_file_id" jsonschema:"Package file ID to delete,required"`
}

// FileDelete deletes a single file from a Generic Package Registry
// package via the GitLab Packages API
// (DELETE /projects/:id/packages/:package_id/package_files/:file_id).
func FileDelete(ctx context.Context, _ *mcp.CallToolRequest, client *gitlabclient.Client, input FileDeleteInput) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return errors.New("packageFileDelete: project_id is required")
	}
	pkgID, err := input.PackageID.Int64()
	if err != nil || pkgID <= 0 {
		return errors.New("packageFileDelete: package_id must be a positive integer")
	}
	fileID, err := input.PackageFileID.Int64()
	if err != nil || fileID <= 0 {
		return errors.New("packageFileDelete: package_file_id must be a positive integer")
	}

	_, err = client.GL().Packages.DeletePackageFile(string(input.ProjectID), pkgID, fileID)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("packageFileDelete", err, http.StatusNotFound,
			"verify package_file_id with gitlab_package_file_list; deleting package files requires Maintainer role or higher")
	}
	return nil
}
