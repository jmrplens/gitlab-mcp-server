package packages

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	FileName       string               `json:"file_name" jsonschema:"Name of the file within the package. May include / to describe a directory structure (segments must not be empty or . or ..),required"`
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
	return publishWithTracker(ctx, client, input, progress.FromRequest(req))
}

// publishWithTracker is Publish with the progress tracker supplied by the
// caller.
//
// A caller publishing several files in one tool call has to pass its own
// tracker: building a second one from the same request would give the same
// progress token two independent monotonic counters, and the sequence the
// client receives would run backwards as the two interleave. See
// [progress.Tracker.OnScale].
func publishWithTracker(
	ctx context.Context,
	client *gitlabclient.Client,
	input PublishInput,
	tracker progress.Tracker,
) (PublishOutput, error) {
	if err := validatePublishInput(ctx, input); err != nil {
		return PublishOutput{}, err
	}

	reader, fileSize, cleanup, err := toolutil.OpenFileOrBase64Source("packagePublish", input.FilePath, input.ContentBase64)
	if err != nil {
		return PublishOutput{}, err
	}
	defer cleanup()

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
		gl.WithContext(ctx),
	)
	if err != nil {
		if errors.Is(err, gl.ErrInvalidFileName) {
			return PublishOutput{}, toolutil.WrapErrWithHint("packagePublish", err,
				"file_name segments between / separators must not be empty, \".\", or \"..\"")
		}
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
		CreatedAt:     toolutil.RFC3339Ptr(published.CreatedAt),
		UpdatedAt:     toolutil.RFC3339Ptr(published.UpdatedAt),
		URL:           pkgURL,
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
	FileName       string               `json:"file_name" jsonschema:"File name to download. May include / to describe a directory structure (segments must not be empty or . or ..),required"`
	OutputPath     string               `json:"output_path" jsonschema:"Absolute path where the file will be saved on the local filesystem, confined to the current working directory, the OS temp directory, or a directory listed in GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS. Symlinks are resolved and destinations outside those roots are rejected,required"`
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

// ListItem represents a single package as the project listing publishes it,
// and as the group listing and a request for one package publish it beside
// what each of them adds. It mirrors the GitLab Packages API [gl.Package]
// object, surfacing the full nested _links, pipeline and tags sub-objects.
//
// Three keys of GitLab's entity are deliberately not here. The owning
// project's id and path are exposed only when it renders a group's listing,
// where [GroupListItem] publishes them, and the package's other versions only
// when it renders one package rather than a page, where [DetailItem] publishes
// them; on this item each was a key the schema offered and no answer filled.
// The entity's pipelines key is not published anywhere: lib/api/entities/
// package.rb renders it as the constant EMPTY_PIPELINES whatever the package,
// which is how GitLab deprecated it in 16.1, and the pipeline that last built
// the package is under pipeline.
type ListItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	PackageType string `json:"package_type"`
	Status      string `json:"status"`
	// ConanPackageName is the recipe's own name, sent for a Conan package.
	ConanPackageName string     `json:"conan_package_name,omitempty"`
	Links            *LinksItem `json:"_links,omitempty"`
	// Pipeline is the pipeline that last built the package, with every key
	// GitLab's pipeline entity sends, five of them read from the captured
	// response because client-go's PackagePipeline and BasicUser leave them
	// out.
	Pipeline         *toolutil.PackagePipelineOutput `json:"pipeline,omitempty"`
	CreatedAt        string                          `json:"created_at,omitempty"`
	LastDownloadedAt string                          `json:"last_downloaded_at,omitempty"`
	CreatorID        int64                           `json:"creator_id,omitempty"`
	Tags             []toolutil.PackageTagOutput     `json:"tags,omitempty"`
}

// DetailItem is one package as a request for it publishes: every field the
// project's listing publishes for it, and the package's other versions, which
// GitLab sends only to this read and never on a page of packages.
type DetailItem struct {
	ListItem
	Versions []toolutil.PackageVersionOutput `json:"versions,omitempty"`
}

// LinksItem mirrors the GitLab Packages API [gl.PackageLinks] object,
// holding the package's web path and delete API path.
type LinksItem struct {
	WebPath       string `json:"web_path,omitempty"`
	DeleteAPIPath string `json:"delete_api_path,omitempty"`
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

// packageToListItem converts a [gl.Package] into the package's [ListItem]
// shape. Every field comes from what client-go decodes, the creator and the
// Conan recipe name included since v3.14.0, except the five keys of the
// pipeline that client-go leaves out, which extra carries from the capture of
// the same answer. The package's other versions, which v3.14.0 decodes too,
// are left to [packageToDetailItem], since no listing sends them, and the
// pipelines client-go decodes are GitLab's constant empty list.
func packageToListItem(p *gl.Package, extra toolutil.PackageExtra) ListItem {
	item := ListItem{
		ID:               p.ID,
		Name:             p.Name,
		Version:          p.Version,
		PackageType:      p.PackageType,
		Status:           p.Status,
		ConanPackageName: p.ConanPackageName,
		Pipeline:         packagePipelineToOutput(p.Pipeline, extra.Pipeline),
		CreatedAt:        toolutil.RFC3339Ptr(p.CreatedAt),
		LastDownloadedAt: toolutil.RFC3339Ptr(p.LastDownloadedAt),
		CreatorID:        p.CreatorID,
		Tags:             packageTagsToOutput(p.Tags),
	}
	if p.Links != nil {
		item.Links = &LinksItem{
			WebPath:       p.Links.WebPath,
			DeleteAPIPath: p.Links.DeleteAPIPath,
		}
	}
	return item
}

// packageToDetailItem converts the one [gl.Package] a request for it returns
// into the [DetailItem] shape: the fields [packageToListItem] converts, and the
// package's other versions, each completed with what the capture of the same
// answer read at its position.
func packageToDetailItem(p *gl.Package, extra toolutil.PackageExtra) DetailItem {
	return DetailItem{
		ListItem: packageToListItem(p, extra),
		Versions: packageVersionsToOutput(p.Versions, extra.Versions),
	}
}

// packageTagsToOutput converts the tags pointing at a package or at one of its
// other versions, or nil when GitLab sent none, so neither publishes an empty
// list for them. It is the one conversion for a tag wherever a package answer
// carries one, so a tag reads the same on the package and on its versions.
func packageTagsToOutput(tags []gl.PackageTag) []toolutil.PackageTagOutput {
	if len(tags) == 0 {
		return nil
	}
	out := make([]toolutil.PackageTagOutput, 0, len(tags))
	for _, tag := range tags {
		out = append(out, toolutil.PackageTagOutput{
			ID:        tag.ID,
			PackageID: tag.PackageID,
			Name:      tag.Name,
			CreatedAt: toolutil.RFC3339Ptr(tag.CreatedAt),
			UpdatedAt: toolutil.RFC3339Ptr(tag.UpdatedAt),
		})
	}
	return out
}

// extraAt is the extra the capture read at position i, or nil where it read
// none. [toolutil.CapturedPackage] holds the count to what client-go decoded,
// so a handler always has one; the nil is for a conversion made without a
// capture, which then publishes what client-go decoded and leaves the rest at
// zero rather than failing.
func extraAt[T any](extras []T, i int) *T {
	if i >= len(extras) {
		return nil
	}
	return &extras[i]
}

// List retrieves a paginated list of Generic Package Registry
// packages in a project via the GitLab Packages list API
// (GET /projects/:id/packages). Optional filters narrow by package
// name, version, status, and the [buildListOptions] sort field.
//
// The five keys of each package's pipeline that client-go does not decode are
// read from the captured response beside that decode (ADR-0021).
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("packageList: project_id is required")
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	pkgs, resp, err := client.GL().Packages.ListProjectPackages(string(input.ProjectID), buildListOptions(input), gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("packageList", err, http.StatusNotFound,
			"verify project_id with project.get; the project may have no packages yet or package registry may be disabled")
	}
	extras, err := toolutil.CapturedPackages(captured, len(pkgs))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("packageList", err)
	}

	items := make([]ListItem, 0, len(pkgs))
	for i, p := range pkgs {
		if p == nil {
			continue
		}
		items = append(items, packageToListItem(p, extras[i]))
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
// and path in addition to the project-scoped package fields, and the
// pipeline's five keys client-go does not decode are read from the captured
// response as [List] reads them.
func GroupList(ctx context.Context, client *gitlabclient.Client, input GroupListInput) (GroupListOutput, error) {
	if err := ctx.Err(); err != nil {
		return GroupListOutput{}, fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.GroupID == "" {
		return GroupListOutput{}, errors.New("packageGroupList: group_id is required")
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	pkgs, resp, err := client.GL().Packages.ListGroupPackages(string(input.GroupID), buildGroupListOptions(input), gl.WithContext(ctx))
	if err != nil {
		return GroupListOutput{}, toolutil.WrapErrWithStatusHint("packageGroupList", err, http.StatusNotFound,
			"verify group_id with group.get; the group may have no packages yet or package registry may be disabled")
	}
	extras, err := toolutil.CapturedPackages(captured, len(pkgs))
	if err != nil {
		return GroupListOutput{}, toolutil.WrapErr("packageGroupList", err)
	}

	items := make([]GroupListItem, 0, len(pkgs))
	for i, p := range pkgs {
		if p == nil {
			continue
		}
		items = append(items, GroupListItem{
			ListItem:    packageToListItem(&p.Package, extras[i]),
			ProjectID:   p.ProjectID,
			ProjectPath: p.ProjectPath,
		})
	}

	return GroupListOutput{
		Packages:   items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Get Package.

// GetInput defines input for retrieving one package of a project.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageID toolutil.StringOrInt `json:"package_id" jsonschema:"Package ID, as package.list or package.group_list returns it,required"`
}

// GetOutput is one package: every field package.list publishes, and the
// package's other versions, which only this read is sent.
type GetOutput struct {
	toolutil.HintableOutput
	Package DetailItem `json:"package"`
}

// packageNotFoundHint is what the handler's own 404 names when a caller meets
// it outside the route, which turns the same 404 into the structured result
// [formatPackageNotFound] renders. It gives both of GitLab's reasons, because
// GitLab reads only a package whose status is default or deprecated here while
// package.list shows one in error status as well, so re-listing alone would
// hand back the same package_id.
const packageNotFoundHint = "check the package's status in package.list, since GitLab answers 404 here for a " +
	"package whose status is not default or deprecated, and whether the package_id is still listed, since a " +
	"deleted version answers 404 as well"

// Get retrieves one package of a project via the GitLab Packages API
// (GET /projects/:id/packages/:package_id), the only endpoint that sends the
// package's other versions.
//
// client-go decodes every field package.list publishes, and the other versions
// with their tags; the three keys of each pipeline that its PackagePipeline
// leaves out, and two of the user who ran it, are read from the captured
// response beside that decode (ADR-0021), on the package's own pipeline and
// on those of its other versions alike.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if err := ctx.Err(); err != nil {
		return GetOutput{}, fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return GetOutput{}, errors.New("packageGet: project_id is required")
	}
	pkgID, err := input.PackageID.Int64()
	if err != nil || pkgID <= 0 {
		return GetOutput{}, errors.New("packageGet: package_id must be a positive integer")
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	pkg, _, err := client.GL().Packages.GetProjectPackage(string(input.ProjectID), pkgID, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("packageGet", err, http.StatusNotFound, packageNotFoundHint)
	}
	extra, err := toolutil.CapturedPackage(captured, len(pkg.Versions))
	if err != nil {
		return GetOutput{}, toolutil.WrapErr("packageGet", err)
	}
	return GetOutput{Package: packageToDetailItem(pkg, extra)}, nil
}

// packageVersionsToOutput converts the other versions client-go decoded,
// completing each one's pipeline with what the capture read at the same
// position. A version GitLab sent as null is skipped. The item publishes its
// versions with omitempty, so a package with no other version publishes none.
func packageVersionsToOutput(versions []*gl.PackageVersion, extras []toolutil.PackageVersionExtra) []toolutil.PackageVersionOutput {
	out := make([]toolutil.PackageVersionOutput, 0, len(versions))
	for i, v := range versions {
		if v == nil {
			continue
		}
		var pipelineExtra *toolutil.PackagePipelineExtra
		if extra := extraAt(extras, i); extra != nil {
			pipelineExtra = extra.Pipeline
		}
		out = append(out, toolutil.PackageVersionOutput{
			ID:        v.ID,
			Version:   v.Version,
			CreatedAt: toolutil.RFC3339Ptr(v.CreatedAt),
			Tags:      packageTagsToOutput(v.Tags),
			Pipeline:  packagePipelineToOutput(v.Pipeline, pipelineExtra),
		})
	}
	return out
}

// packagePipelineToOutput converts the pipeline that last built a package or
// one of its other versions, or nil when GitLab sent none: the eight keys
// client-go's PackagePipeline decodes, and the three the capture read beside
// it.
func packagePipelineToOutput(pipeline *gl.PackagePipeline, extra *toolutil.PackagePipelineExtra) *toolutil.PackagePipelineOutput {
	if pipeline == nil {
		return nil
	}
	out := &toolutil.PackagePipelineOutput{
		ID:        pipeline.ID,
		SHA:       pipeline.SHA,
		Ref:       pipeline.Ref,
		Status:    pipeline.Status,
		CreatedAt: toolutil.RFC3339Ptr(pipeline.CreatedAt),
		UpdatedAt: toolutil.RFC3339Ptr(pipeline.UpdatedAt),
		WebURL:    pipeline.WebURL,
	}
	var userExtra *toolutil.UserBasicExtra
	if extra != nil {
		out.IID = extra.IID
		out.ProjectID = extra.ProjectID
		out.Source = extra.Source
		userExtra = extra.User
	}
	out.User = packagePipelineUserToOutput(pipeline.User, userExtra)
	return out
}

// packagePipelineUserToOutput converts the user who ran one of a package's
// pipelines, or nil when GitLab sent none: the six keys client-go's BasicUser
// decodes that the entity sends, and the two the capture read beside it. The
// seventh key BasicUser decodes, created_at, is not one GitLab's UserBasic
// sends, so it is not published.
func packagePipelineUserToOutput(user *gl.BasicUser, extra *toolutil.UserBasicExtra) *toolutil.UserBasicOutput {
	if user == nil {
		return nil
	}
	out := &toolutil.UserBasicOutput{
		ID:        user.ID,
		Username:  user.Username,
		Name:      user.Name,
		State:     user.State,
		AvatarURL: user.AvatarURL,
		WebURL:    user.WebURL,
	}
	if extra != nil {
		out.PublicEmail = extra.PublicEmail
		out.Locked = extra.Locked
	}
	return out
}

// List Package Files.

// FileListInput defines input for listing files within a package.
type FileListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageID toolutil.StringOrInt `json:"package_id" jsonschema:"Package ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order package files by: id (default), file_name, or created_at"`
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

	files, resp, err := client.GL().Packages.ListPackageFiles(string(input.ProjectID), pkgID, opts, gl.WithContext(ctx))
	if err != nil {
		return FileListOutput{}, toolutil.WrapErrWithStatusHint("packageFileList", err, http.StatusNotFound,
			"verify package_id with package.list; the package may have been deleted")
	}

	items := make([]FileListItem, 0, len(files))
	for _, f := range files {
		items = append(items, FileListItem{
			PackageFileID: f.ID,
			PackageID:     f.PackageID,
			FileName:      f.FileName,
			Size:          f.Size,
			SHA256:        f.FileSHA256,
			FileMD5:       f.FileMD5,
			FileSHA1:      f.FileSHA1,
			CreatedAt:     toolutil.RFC3339Ptr(f.CreatedAt),
		})
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

	_, err = client.GL().Packages.DeleteProjectPackage(string(input.ProjectID), pkgID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return fmt.Errorf("packageDelete: package deletion requires Maintainer role or higher. Your current role may only allow publishing. Contact a project Maintainer to delete packages: %w", err)
		}
		return toolutil.WrapErrWithStatusHint("packageDelete", err, http.StatusNotFound,
			"verify package_id with package.list; the package may already have been deleted")
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

	_, err = client.GL().Packages.DeletePackageFile(string(input.ProjectID), pkgID, fileID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("packageFileDelete", err, http.StatusNotFound,
			"verify package_file_id with package.file_list; deleting package files requires Maintainer role or higher")
	}
	return nil
}
