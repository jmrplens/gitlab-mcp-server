// Package securefiles implements MCP tools for GitLab CI/CD Secure Files.
package securefiles

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// List.

// ListInput contains parameters for listing secure files.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Page      int64                `json:"page" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page" jsonschema:"Number of items per page"`
}

// SecureFileItem represents a single secure file.
type SecureFileItem struct {
	toolutil.HintableOutput
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Checksum          string `json:"checksum"`
	ChecksumAlgorithm string `json:"checksum_algorithm"`
}

// ListOutput contains a list of secure files.
type ListOutput struct {
	toolutil.HintableOutput
	Files      []SecureFileItem          `json:"files"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves secure files for a project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListProjectSecureFilesOptions{}
	if input.Page > 0 || input.PerPage > 0 {
		opts.ListOptions = gl.ListOptions{Page: input.Page, PerPage: input.PerPage}
	}
	files, resp, err := client.GL().SecureFiles.ListProjectSecureFiles(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("gitlab_list_secure_files", err)
	}
	items := make([]SecureFileItem, 0, len(files))
	for _, f := range files {
		items = append(items, SecureFileItem{
			ID:                f.ID,
			Name:              f.Name,
			Checksum:          f.Checksum,
			ChecksumAlgorithm: f.ChecksumAlgorithm,
		})
	}
	return ListOutput{
		Files:      items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Show.

// ShowInput contains parameters for showing a secure file.
type ShowInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	FileID    int64                `json:"file_id" jsonschema:"Secure file ID,required"`
}

// Show retrieves details for a specific secure file.
func Show(ctx context.Context, client *gitlabclient.Client, input ShowInput) (SecureFileItem, error) {
	if input.FileID <= 0 {
		return SecureFileItem{}, toolutil.ErrRequiredInt64("gitlab_show_secure_file", "file_id")
	}
	f, _, err := client.GL().SecureFiles.ShowSecureFileDetails(string(input.ProjectID), input.FileID, gl.WithContext(ctx))
	if err != nil {
		return SecureFileItem{}, toolutil.WrapErrWithMessage("gitlab_show_secure_file", err)
	}
	return SecureFileItem{
		ID:                f.ID,
		Name:              f.Name,
		Checksum:          f.Checksum,
		ChecksumAlgorithm: f.ChecksumAlgorithm,
	}, nil
}

// Create.

// CreateInput contains parameters for creating a secure file.
// Exactly one of FilePath or ContentBase64 must be provided.
type CreateInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name          string               `json:"name" jsonschema:"Name for the secure file,required"`
	FilePath      string               `json:"file_path,omitempty" jsonschema:"Absolute path to a local file on the MCP server filesystem. Alternative to content_base64. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string               `json:"content_base64,omitempty" jsonschema:"Base64-encoded file content. Only one of file_path or content_base64 should be provided."`
}

// Create uploads a new secure file.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (SecureFileItem, error) {
	hasFilePath := input.FilePath != ""
	hasBase64 := input.ContentBase64 != ""

	if hasFilePath && hasBase64 {
		return SecureFileItem{}, errors.New("gitlab_create_secure_file: provide either file_path or content_base64, not both")
	}
	if !hasFilePath && !hasBase64 {
		return SecureFileItem{}, errors.New("gitlab_create_secure_file: either file_path or content_base64 is required")
	}

	var reader *bytes.Reader

	if hasFilePath {
		cfg := toolutil.GetUploadConfig()
		f, info, err := toolutil.OpenAndValidateFile(input.FilePath, cfg.MaxFileSize)
		if err != nil {
			return SecureFileItem{}, fmt.Errorf("gitlab_create_secure_file: %w", err)
		}
		defer f.Close()

		data := make([]byte, info.Size())
		if _, err = io.ReadFull(f, data); err != nil {
			return SecureFileItem{}, fmt.Errorf("gitlab_create_secure_file: reading file: %w", err)
		}
		reader = bytes.NewReader(data)
	} else {
		decoded, err := base64.StdEncoding.DecodeString(input.ContentBase64)
		if err != nil {
			return SecureFileItem{}, fmt.Errorf("gitlab_create_secure_file: invalid base64 content: %w", err)
		}
		reader = bytes.NewReader(decoded)
	}

	opts := &gl.CreateSecureFileOptions{
		Name: new(input.Name),
	}
	f, _, err := client.GL().SecureFiles.CreateSecureFile(string(input.ProjectID), reader, opts, gl.WithContext(ctx))
	if err != nil {
		return SecureFileItem{}, toolutil.WrapErrWithMessage("gitlab_create_secure_file", err)
	}
	return SecureFileItem{
		ID:                f.ID,
		Name:              f.Name,
		Checksum:          f.Checksum,
		ChecksumAlgorithm: f.ChecksumAlgorithm,
	}, nil
}

// Remove.

// RemoveInput contains parameters for removing a secure file.
type RemoveInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	FileID    int64                `json:"file_id" jsonschema:"Secure file ID,required"`
}

// Remove deletes a secure file.
func Remove(ctx context.Context, client *gitlabclient.Client, input RemoveInput) error {
	if input.FileID <= 0 {
		return toolutil.ErrRequiredInt64("gitlab_remove_secure_file", "file_id")
	}
	_, err := client.GL().SecureFiles.RemoveSecureFile(string(input.ProjectID), input.FileID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("gitlab_remove_secure_file", err)
	}
	return nil
}

// formatters.
