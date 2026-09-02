package packages

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	LinkName       string               `json:"link_name,omitempty" jsonschema:"Display name of the release link. Defaults to file_name if omitted. MUST be the exact filename. Never add descriptive suffixes."`
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
		return PublishAndLinkOutput{}, toolutil.WrapErrWithStatusHint("packagePublishAndLink/publish", err, http.StatusBadRequest,
			"package_name/version must follow SemVer; verify file_path is readable and project has Package Registry enabled")
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
		return PublishAndLinkOutput{Package: pubOut}, toolutil.WrapErrWithStatusHint(
			"packagePublishAndLink/link",
			err,
			http.StatusBadRequest,
			"package was published successfully but linking to release failed; verify tag_name with gitlab_release_list and link_type enum {other, runbook, image, package}",
		)
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
	IncludePattern string               `json:"include_pattern,omitempty" jsonschema:"Single glob pattern to filter files within the directory (e.g. *.txt or *.tar.gz). If omitted, all regular files are included. Do not pass comma-separated filenames."`
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

// validatePublishDirInput checks required fields and resolves
// [PublishDirInput.DirectoryPath] to a canonical directory confined to the
// allowed upload roots, returning the path the publish loop should walk.
//
// The whole tree is read and shipped to a project the caller names, so the
// directory is confined exactly as a single file_path is; without it, one call
// exfiltrates a home directory.
func validatePublishDirInput(ctx context.Context, input PublishDirInput) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf(fmtCtxCancelled, err)
	}
	if input.ProjectID == "" {
		return "", errors.New("packagePublishDirectory: project_id is required")
	}
	if err := toolutil.ValidatePackageName(input.PackageName); err != nil {
		return "", fmt.Errorf("packagePublishDirectory: %w", err)
	}
	if input.PackageVersion == "" {
		return "", errors.New("packagePublishDirectory: package_version is required")
	}
	if input.DirectoryPath == "" {
		return "", errors.New("packagePublishDirectory: directory_path is required")
	}

	directoryPath, err := toolutil.CanonicalLocalDirPath(input.DirectoryPath)
	if err != nil {
		return "", fmt.Errorf("packagePublishDirectory: %w", err)
	}
	return directoryPath, nil
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
	directoryPath, err := validatePublishDirInput(ctx, input)
	if err != nil {
		return PublishDirOutput{}, err
	}

	files, err := collectMatchingFiles(directoryPath, input.IncludePattern)
	if err != nil {
		return PublishDirOutput{}, err
	}

	tracker := progress.FromRequest(req)
	// One scale for the whole call, measured in bytes.
	//
	// Counting files here while each upload counts its own bytes gave one
	// progress token two series and two meanings of "total", so what a client
	// received ran 200000 -> 1 -> 0 and its total oscillated. Bytes are the
	// measure both levels can share, and they are the more useful of the two:
	// a directory of one large file and twenty small ones is not five percent
	// done after the first.
	sizes := fileSizes(directoryPath, files)
	var totalBytes, doneBytes int64
	for _, size := range sizes {
		totalBytes += size
	}

	var out PublishDirOutput
	out.Published = make([]PublishDirItem, 0, len(files))

	for i, name := range files {
		if err = ctx.Err(); err != nil {
			return out, fmt.Errorf("context canceled after %d of %d files: %w", i, len(files), err)
		}

		if tracker.IsActive() {
			tracker.Update(ctx, float64(doneBytes), float64(totalBytes),
				fmt.Sprintf("Publishing file %d of %d: %s", i+1, len(files), name))
		}

		pubInput := PublishInput{
			ProjectID:      input.ProjectID,
			PackageName:    input.PackageName,
			PackageVersion: input.PackageVersion,
			FileName:       name,
			FilePath:       filepath.Join(directoryPath, name),
			Status:         input.Status,
		}

		var pubOut PublishOutput
		pubOut, err = publishWithTracker(ctx, client, pubInput,
			tracker.OnScale(float64(doneBytes), float64(totalBytes)))
		doneBytes += sizes[i]
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
		tracker.Update(ctx, float64(totalBytes), float64(totalBytes),
			fmt.Sprintf("Published %d of %d files", out.TotalFiles, len(files)))
	}

	return out, nil
}

// fileSizes returns the size of each named file in dir, in the same order.
//
// A file that cannot be stat'd counts as zero rather than failing the call:
// the sizes only scale a progress bar, and losing the whole publish because one
// file's metadata was unreadable would trade a cosmetic problem for a real one.
// The publish of that file reports its own failure in the usual way.
func fileSizes(dir string, names []string) []int64 {
	sizes := make([]int64, len(names))
	for i, name := range names {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
			sizes[i] = info.Size()
		}
	}
	return sizes
}
