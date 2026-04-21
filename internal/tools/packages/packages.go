// Package packages implements GitLab Generic Packages API operations as MCP tools.
// It provides handlers for publishing, downloading, listing, and deleting
// package files in the GitLab Package Registry.
package packages

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
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

// validatePublishInput checks all required fields and mutual exclusion constraints for Publish.
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

	var reader io.Reader
	var fileSize int64

	if input.FilePath != "" {
		cfg := toolutil.GetUploadConfig()
		f, info, err := toolutil.OpenAndValidateFile(input.FilePath, cfg.MaxFileSize)
		if err != nil {
			return PublishOutput{}, fmt.Errorf(fmtPkgPublish, err)
		}
		fileSize = info.Size()

		defer f.Close()
		reader = f
	} else {
		decoded, err := base64.StdEncoding.DecodeString(input.ContentBase64)
		if err != nil {
			return PublishOutput{}, fmt.Errorf("packagePublish: invalid base64 content: %w", err)
		}
		reader = bytes.NewReader(decoded)
		fileSize = int64(len(decoded))
	}

	tracker := progress.FromRequest(req)
	if tracker.IsActive() {
		reader = toolutil.NewProgressReader(ctx, reader, fileSize, tracker)
	}

	// Use select=package_file to get metadata in response
	selectVal := gl.GenericPackageSelectValue("package_file")

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
		return PublishOutput{}, toolutil.WrapErrWithMessage("packagePublish", err)
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

// ptrString returns a pointer to s, or nil if s is empty.
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

// Download downloads a file from the GitLab Generic Package Registry.
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
}

// ListItem represents a single package in the list output.
type ListItem struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	PackageType      string   `json:"package_type"`
	Status           string   `json:"status"`
	CreatedAt        string   `json:"created_at,omitempty"`
	LastDownloadedAt string   `json:"last_downloaded_at,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	WebPath          string   `json:"web_path,omitempty"`
}

// ListOutput contains the paginated list of packages.
type ListOutput struct {
	toolutil.HintableOutput
	Packages   []ListItem                `json:"packages"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// buildListOptions translates ListInput filter fields into GitLab API options.
func buildListOptions(input ListInput) *gl.ListProjectPackagesOptions {
	opts := &gl.ListProjectPackagesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
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
func packageToListItem(p *gl.Package) ListItem {
	item := ListItem{
		ID:          p.ID,
		Name:        p.Name,
		Version:     p.Version,
		PackageType: p.PackageType,
		Status:      p.Status,
	}
	if p.CreatedAt != nil {
		item.CreatedAt = p.CreatedAt.String()
	}
	if p.LastDownloadedAt != nil {
		item.LastDownloadedAt = p.LastDownloadedAt.String()
	}
	if len(p.Tags) > 0 {
		tags := make([]string, 0, len(p.Tags))
		for _, tag := range p.Tags {
			tags = append(tags, tag.Name)
		}
		item.Tags = tags
	}
	if p.Links != nil {
		item.WebPath = p.Links.WebPath
	}
	return item
}

// List lists packages in a GitLab project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("packageList: project_id is required")
	}

	pkgs, resp, err := client.GL().Packages.ListProjectPackages(string(input.ProjectID), buildListOptions(input))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("packageList", err)
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

// List Package Files.

// FileListInput defines input for listing files within a package.
type FileListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageID toolutil.StringOrInt `json:"package_id" jsonschema:"Package ID,required"`
	toolutil.PaginationInput
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

// FileList lists files within a specific package.
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

	opts := &gl.ListPackageFilesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}

	files, resp, err := client.GL().Packages.ListPackageFiles(string(input.ProjectID), pkgID, opts)
	if err != nil {
		return FileListOutput{}, toolutil.WrapErrWithMessage("packageFileList", err)
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

// Delete deletes a package from the GitLab Package Registry.
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
		return toolutil.WrapErrWithMessage("packageDelete", err)
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

// FileDelete deletes a single file from a package.
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
		return toolutil.WrapErrWithMessage("packageFileDelete", err)
	}
	return nil
}
