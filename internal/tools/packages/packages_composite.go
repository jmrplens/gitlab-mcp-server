// packages_composite.go implements higher-level GitLab Generic Packages
// operations that combine multiple API calls into a single tool invocation.
// - PublishAndLink: publish a file then create a release asset link
// - PublishDirectory: publish all matching files in a directory.

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Publish and Link to Release.

// PublishAndLinkInput defines input for publishing a file to the
// Generic Package Registry and creating a release asset link in one step.
type PublishAndLinkInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageName    string               `json:"package_name" jsonschema:"Package name (alphanumeric, dots, dashes, underscores),required"`
	PackageVersion string               `json:"package_version" jsonschema:"Package version (e.g. 1.0.0),required"`
	FileName       string               `json:"file_name" jsonschema:"Name of the file within the package,required"`
	FilePath       string               `json:"file_path,omitempty" jsonschema:"Absolute path to a local file. Alternative to content_base64."`
	ContentBase64  string               `json:"content_base64,omitempty" jsonschema:"Base64-encoded file content. Alternative to file_path."`
	Status         string               `json:"status,omitempty" jsonschema:"Package status: default or hidden"`
	TagName        string               `json:"tag_name" jsonschema:"Tag name of the release to link the package file to,required"`
	LinkName       string               `json:"link_name,omitempty" jsonschema:"Display name of the release link. Defaults to file_name if omitted. MUST be the exact filename — never add descriptive suffixes."`
	LinkType       string               `json:"link_type,omitempty" jsonschema:"Type of the release link: package, runbook, image, or other. Defaults to package."`
}

// PublishAndLinkOutput contains the results of both the publish and
// release link creation operations.
type PublishAndLinkOutput struct {
	toolutil.HintableOutput
	Package     PublishOutput       `json:"package"`
	ReleaseLink releaselinks.Output `json:"release_link"`
}

// PublishAndLink publishes a file to the Generic Package Registry and
// then creates a release asset link pointing to it. If the publish succeeds
// but the link creation fails, the package file remains published and the
// error includes the package details so the caller can retry the link.
func PublishAndLink(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input PublishAndLinkInput) (PublishAndLinkOutput, error) {
	if err := ctx.Err(); err != nil {
		return PublishAndLinkOutput{}, fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.TagName == "" {
		return PublishAndLinkOutput{}, errors.New("packagePublishAndLink: tag_name is required")
	}

	pubInput := PublishInput{
		ProjectID:      input.ProjectID,
		PackageName:    input.PackageName,
		PackageVersion: input.PackageVersion,
		FileName:       input.FileName,
		FilePath:       input.FilePath,
		ContentBase64:  input.ContentBase64,
		Status:         input.Status,
	}
	pubOut, err := Publish(ctx, req, client, pubInput)
	if err != nil {
		return PublishAndLinkOutput{}, toolutil.WrapErrWithMessage("packagePublishAndLink/publish", err)
	}

	linkName := input.LinkName
	if linkName == "" {
		linkName = input.FileName
	}
	linkType := input.LinkType
	if linkType == "" {
		linkType = "package"
	}

	linkInput := releaselinks.CreateInput{
		ProjectID: input.ProjectID,
		TagName:   input.TagName,
		Name:      linkName,
		URL:       pubOut.URL,
		LinkType:  linkType,
	}
	linkOut, err := releaselinks.Create(ctx, client, linkInput)
	if err != nil {
		return PublishAndLinkOutput{
			Package: pubOut,
		}, toolutil.WrapErrWithMessage("packagePublishAndLink/link", err)
	}

	return PublishAndLinkOutput{
		Package:     pubOut,
		ReleaseLink: linkOut,
	}, nil
}

// Publish Directory.

// PublishDirInput defines input for publishing all matching files in
// a directory to the Generic Package Registry.
type PublishDirInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PackageName    string               `json:"package_name" jsonschema:"Package name (alphanumeric, dots, dashes, underscores),required"`
	PackageVersion string               `json:"package_version" jsonschema:"Package version (e.g. 1.0.0),required"`
	DirectoryPath  string               `json:"directory_path" jsonschema:"Absolute path to a local directory whose files will be published,required"`
	IncludePattern string               `json:"include_pattern,omitempty" jsonschema:"Glob pattern to filter files within the directory (e.g. *.tar.gz). If omitted, all regular files are included."`
	Status         string               `json:"status,omitempty" jsonschema:"Package status: default or hidden"`
}

// PublishDirItem represents a single file published from a directory.
type PublishDirItem struct {
	FileName      string `json:"file_name"`
	PackageFileID int64  `json:"package_file_id"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256"`
	URL           string `json:"url"`
}

// PublishDirOutput contains the aggregated results of publishing all
// matching files from a directory.
type PublishDirOutput struct {
	toolutil.HintableOutput
	Published  []PublishDirItem `json:"published"`
	TotalFiles int              `json:"total_files"`
	TotalBytes int64            `json:"total_bytes"`
	Errors     []string         `json:"errors,omitempty"`
}

// validatePublishDirInput checks required fields and that DirectoryPath is a valid directory.
func validatePublishDirInput(ctx context.Context, input PublishDirInput) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return errors.New("packagePublishDirectory: project_id is required")
	}
	if err := toolutil.ValidatePackageName(input.PackageName); err != nil {
		return fmt.Errorf("packagePublishDirectory: %w", err)
	}
	if input.PackageVersion == "" {
		return errors.New("packagePublishDirectory: package_version is required")
	}
	if input.DirectoryPath == "" {
		return errors.New("packagePublishDirectory: directory_path is required")
	}

	info, err := os.Stat(input.DirectoryPath)
	if err != nil {
		return fmt.Errorf("packagePublishDirectory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("packagePublishDirectory: %s is not a directory", input.DirectoryPath)
	}
	return nil
}

// shouldIncludeFile reports whether a directory entry is a regular file whose
// name matches the optional glob pattern. An empty pattern matches all files.
func shouldIncludeFile(entry os.DirEntry, pattern string) (bool, error) {
	if entry.IsDir() {
		return false, nil
	}
	info, err := entry.Info()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	if pattern == "" {
		return true, nil
	}
	matched, err := filepath.Match(pattern, entry.Name())
	if err != nil {
		return false, fmt.Errorf("packagePublishDirectory: invalid glob pattern %q: %w", pattern, err)
	}
	return matched, nil
}

// collectMatchingFiles reads directoryPath and returns regular file names that match
// the optional glob pattern. An empty pattern matches all regular files.
func collectMatchingFiles(directoryPath, pattern string) ([]string, error) {
	entries, err := os.ReadDir(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("packagePublishDirectory: read dir %s: %w", directoryPath, err)
	}

	var files []string
	var include bool
	for _, entry := range entries {
		include, err = shouldIncludeFile(entry, pattern)
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}
		files = append(files, entry.Name())
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("packagePublishDirectory: no matching files found in %s", directoryPath)
	}
	return files, nil
}

// PublishDirectory walks a directory, filters files by an optional glob
// pattern, and publishes each matching regular file to the Generic Package
// Registry. It continues on individual file errors and reports them in output.
func PublishDirectory(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input PublishDirInput) (PublishDirOutput, error) {
	if err := validatePublishDirInput(ctx, input); err != nil {
		return PublishDirOutput{}, err
	}

	files, err := collectMatchingFiles(input.DirectoryPath, input.IncludePattern)
	if err != nil {
		return PublishDirOutput{}, err
	}

	tracker := progress.FromRequest(req)
	var out PublishDirOutput
	out.Published = make([]PublishDirItem, 0, len(files))

	for i, name := range files {
		if err = ctx.Err(); err != nil {
			return out, fmt.Errorf("context canceled after %d of %d files: %w", i, len(files), err)
		}

		if tracker.IsActive() {
			tracker.Update(ctx, float64(i), float64(len(files)),
				fmt.Sprintf("Publishing file %d of %d: %s", i+1, len(files), name))
		}

		pubInput := PublishInput{
			ProjectID:      input.ProjectID,
			PackageName:    input.PackageName,
			PackageVersion: input.PackageVersion,
			FileName:       name,
			FilePath:       filepath.Join(input.DirectoryPath, name),
			Status:         input.Status,
		}

		var pubOut PublishOutput
		pubOut, err = Publish(ctx, req, client, pubInput)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}

		out.Published = append(out.Published, PublishDirItem{
			FileName:      pubOut.FileName,
			PackageFileID: pubOut.PackageFileID,
			Size:          pubOut.Size,
			SHA256:        pubOut.SHA256,
			URL:           pubOut.URL,
		})
		out.TotalBytes += pubOut.Size
	}

	out.TotalFiles = len(out.Published)

	if tracker.IsActive() {
		tracker.Update(ctx, float64(len(files)), float64(len(files)),
			fmt.Sprintf("Published %d of %d files", out.TotalFiles, len(files)))
	}

	return out, nil
}
