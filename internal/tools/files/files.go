// Package files implements MCP tool handlers for GitLab repository file
// operations including get, create, update, delete, blame, metadata, and raw
// content retrieval. It wraps the RepositoryFiles service from client-go v2.
package files

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GetInput defines parameters for retrieving a file from a repository.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	FilePath  string               `json:"file_path"  jsonschema:"URL-encoded full path of the file (e.g. src%2Fmain.go or src/main.go),required"`
	Ref       string               `json:"ref,omitempty" jsonschema:"Branch name, tag, or commit SHA (defaults to default branch)"`
}

// Output represents a file retrieved from a repository.
type Output struct {
	toolutil.HintableOutput
	FileName        string `json:"file_name"`
	FilePath        string `json:"file_path"`
	Size            int64  `json:"size"`
	Encoding        string `json:"encoding"`
	Content         string `json:"content"`
	ContentCategory string `json:"content_category"`
	Ref             string `json:"ref"`
	BlobID          string `json:"blob_id"`
	CommitID        string `json:"commit_id"`
	LastCommitID    string `json:"last_commit_id"`
	ExecuteFilemode bool   `json:"execute_filemode"`

	// ImageData holds raw image bytes for ImageContent responses (not serialized).
	ImageData []byte `json:"-"`
	// ImageMIMEType holds the MIME type for image files (not serialized).
	ImageMIMEType string `json:"-"`
}

// Get retrieves a single file from a GitLab repository by its path and
// optional ref (branch, tag, or commit SHA). If the file content is
// base64-encoded by the API, it is automatically decoded to plain text.
// Returns an error if the file is not found or decoding fails.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("fileGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.GetFileOptions{}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}

	f, _, err := client.GL().RepositoryFiles.GetFile(string(input.ProjectID), input.FilePath, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("fileGet", err, "verify the file_path and ref exist; use gitlab_repository_tree to browse files")
		}
		return Output{}, toolutil.WrapErrWithMessage("fileGet", err)
	}

	content := f.Content
	var imageData []byte
	imageMIME := toolutil.ImageMIMEType(f.FileName)
	isBinary := toolutil.IsBinaryFile(f.FileName)

	if f.Encoding == "base64" {
		var decoded []byte
		decoded, err = base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			return Output{}, fmt.Errorf("fileGet: decode base64 content: %w", err)
		}
		switch {
		case imageMIME != "":
			imageData = decoded
			content = ""
		case isBinary:
			content = ""
		default:
			content = string(decoded)
		}
	}

	category := "text"
	if imageMIME != "" {
		category = "image"
	} else if isBinary {
		category = "binary"
	}

	return Output{
		FileName:        f.FileName,
		FilePath:        f.FilePath,
		Size:            f.Size,
		Encoding:        f.Encoding,
		Content:         content,
		ContentCategory: category,
		Ref:             f.Ref,
		BlobID:          f.BlobID,
		CommitID:        f.CommitID,
		LastCommitID:    f.LastCommitID,
		ExecuteFilemode: f.ExecuteFilemode,
		ImageData:       imageData,
		ImageMIMEType:   imageMIME,
	}, nil
}

// ---------------------------------------------------------------------------
// CreateFile
// ---------------------------------------------------------------------------.

// CreateInput defines parameters for creating a new file in a repository.
type CreateInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	FilePath        string               `json:"file_path"               jsonschema:"URL-encoded full path of the new file,required"`
	Branch          string               `json:"branch"                  jsonschema:"Branch to create the file on,required"`
	Content         string               `json:"content"                 jsonschema:"File content,required"`
	CommitMessage   string               `json:"commit_message"          jsonschema:"Commit message,required"`
	StartBranch     string               `json:"start_branch,omitempty"  jsonschema:"Branch to start from (creates new branch if different from branch)"`
	Encoding        string               `json:"encoding,omitempty"      jsonschema:"Content encoding: text or base64 (default: text)"`
	AuthorEmail     string               `json:"author_email,omitempty"  jsonschema:"Commit author email"`
	AuthorName      string               `json:"author_name,omitempty"   jsonschema:"Commit author name"`
	ExecuteFilemode *bool                `json:"execute_filemode,omitempty" jsonschema:"Enable execute permission on the file"`
}

// FileInfoOutput represents the result of a file create or update operation.
type FileInfoOutput struct {
	toolutil.HintableOutput
	FilePath string `json:"file_path"`
	Branch   string `json:"branch"`
}

// Create creates a new file in a GitLab repository.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (FileInfoOutput, error) {
	if err := ctx.Err(); err != nil {
		return FileInfoOutput{}, err
	}
	if input.ProjectID == "" {
		return FileInfoOutput{}, errors.New("fileCreate: project_id is required")
	}
	if input.Branch == "" {
		return FileInfoOutput{}, errors.New("fileCreate: branch is required")
	}
	if input.CommitMessage == "" {
		return FileInfoOutput{}, errors.New("fileCreate: commit_message is required")
	}
	opts := &gl.CreateFileOptions{
		Branch:        new(input.Branch),
		Content:       new(input.Content),
		CommitMessage: new(input.CommitMessage),
	}
	if input.StartBranch != "" {
		opts.StartBranch = new(input.StartBranch)
	}
	if input.Encoding != "" {
		opts.Encoding = new(input.Encoding)
	}
	if input.AuthorEmail != "" {
		opts.AuthorEmail = new(input.AuthorEmail)
	}
	if input.AuthorName != "" {
		opts.AuthorName = new(input.AuthorName)
	}
	if input.ExecuteFilemode != nil {
		opts.ExecuteFilemode = input.ExecuteFilemode
	}
	info, _, err := client.GL().RepositoryFiles.CreateFile(string(input.ProjectID), input.FilePath, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return FileInfoOutput{}, toolutil.WrapErrWithHint("fileCreate", err, "the file may already exist — use gitlab_file_update to modify an existing file, or verify the branch name")
		}
		return FileInfoOutput{}, toolutil.WrapErrWithMessage("fileCreate", err)
	}
	return FileInfoOutput{FilePath: info.FilePath, Branch: info.Branch}, nil
}

// ---------------------------------------------------------------------------
// UpdateFile
// ---------------------------------------------------------------------------.

// UpdateInput defines parameters for updating an existing file in a repository.
type UpdateInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	FilePath        string               `json:"file_path"               jsonschema:"URL-encoded full path of the file to update,required"`
	Branch          string               `json:"branch"                  jsonschema:"Branch to update the file on,required"`
	Content         string               `json:"content"                 jsonschema:"New file content,required"`
	CommitMessage   string               `json:"commit_message"          jsonschema:"Commit message,required"`
	StartBranch     string               `json:"start_branch,omitempty"  jsonschema:"Branch to start from"`
	Encoding        string               `json:"encoding,omitempty"      jsonschema:"Content encoding: text or base64 (default: text)"`
	AuthorEmail     string               `json:"author_email,omitempty"  jsonschema:"Commit author email"`
	AuthorName      string               `json:"author_name,omitempty"   jsonschema:"Commit author name"`
	LastCommitID    string               `json:"last_commit_id,omitempty" jsonschema:"Last known commit ID for optimistic locking"`
	ExecuteFilemode *bool                `json:"execute_filemode,omitempty" jsonschema:"Enable execute permission on the file"`
}

// Update updates an existing file in a GitLab repository.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (FileInfoOutput, error) {
	if err := ctx.Err(); err != nil {
		return FileInfoOutput{}, err
	}
	if input.ProjectID == "" {
		return FileInfoOutput{}, errors.New("fileUpdate: project_id is required")
	}
	if input.Branch == "" {
		return FileInfoOutput{}, errors.New("fileUpdate: branch is required")
	}
	if input.CommitMessage == "" {
		return FileInfoOutput{}, errors.New("fileUpdate: commit_message is required")
	}
	opts := &gl.UpdateFileOptions{
		Branch:        new(input.Branch),
		Content:       new(input.Content),
		CommitMessage: new(input.CommitMessage),
	}
	if input.StartBranch != "" {
		opts.StartBranch = new(input.StartBranch)
	}
	if input.Encoding != "" {
		opts.Encoding = new(input.Encoding)
	}
	if input.AuthorEmail != "" {
		opts.AuthorEmail = new(input.AuthorEmail)
	}
	if input.AuthorName != "" {
		opts.AuthorName = new(input.AuthorName)
	}
	if input.LastCommitID != "" {
		opts.LastCommitID = new(input.LastCommitID)
	}
	if input.ExecuteFilemode != nil {
		opts.ExecuteFilemode = input.ExecuteFilemode
	}
	info, _, err := client.GL().RepositoryFiles.UpdateFile(string(input.ProjectID), input.FilePath, opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
			return FileInfoOutput{}, toolutil.WrapErrWithHint("fileUpdate", err, "verify the file exists and encoding is 'text' or 'base64'; check that the branch name is correct")
		case toolutil.IsHTTPStatus(err, http.StatusConflict):
			return FileInfoOutput{}, toolutil.WrapErrWithHint("fileUpdate", err, "the file was modified since last_commit_id — re-read the file to get the current last_commit_id and retry")
		default:
			return FileInfoOutput{}, toolutil.WrapErrWithMessage("fileUpdate", err)
		}
	}
	return FileInfoOutput{FilePath: info.FilePath, Branch: info.Branch}, nil
}

// ---------------------------------------------------------------------------
// DeleteFile
// ---------------------------------------------------------------------------.

// DeleteInput defines parameters for deleting a file from a repository.
type DeleteInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	FilePath      string               `json:"file_path"               jsonschema:"URL-encoded full path of the file to delete,required"`
	Branch        string               `json:"branch"                  jsonschema:"Branch to delete the file from,required"`
	CommitMessage string               `json:"commit_message"          jsonschema:"Commit message,required"`
	StartBranch   string               `json:"start_branch,omitempty"  jsonschema:"Branch to start from"`
	AuthorEmail   string               `json:"author_email,omitempty"  jsonschema:"Commit author email"`
	AuthorName    string               `json:"author_name,omitempty"   jsonschema:"Commit author name"`
	LastCommitID  string               `json:"last_commit_id,omitempty" jsonschema:"Last known commit ID for optimistic locking"`
}

// Delete deletes a file from a GitLab repository.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("fileDelete: project_id is required")
	}
	if input.Branch == "" {
		return errors.New("fileDelete: branch is required")
	}
	if input.CommitMessage == "" {
		return errors.New("fileDelete: commit_message is required")
	}
	opts := &gl.DeleteFileOptions{
		Branch:        new(input.Branch),
		CommitMessage: new(input.CommitMessage),
	}
	if input.StartBranch != "" {
		opts.StartBranch = new(input.StartBranch)
	}
	if input.AuthorEmail != "" {
		opts.AuthorEmail = new(input.AuthorEmail)
	}
	if input.AuthorName != "" {
		opts.AuthorName = new(input.AuthorName)
	}
	if input.LastCommitID != "" {
		opts.LastCommitID = new(input.LastCommitID)
	}
	_, err := client.GL().RepositoryFiles.DeleteFile(string(input.ProjectID), input.FilePath, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return toolutil.WrapErrWithHint("fileDelete", err, "the file does not exist at the specified path or branch — verify with gitlab_file_get first")
		}
		return toolutil.WrapErrWithMessage("fileDelete", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// GetFileBlame
// ---------------------------------------------------------------------------.

// BlameInput defines parameters for retrieving file blame information.
type BlameInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	FilePath   string               `json:"file_path"             jsonschema:"URL-encoded full path of the file,required"`
	Ref        string               `json:"ref,omitempty"         jsonschema:"Branch, tag, or commit SHA (defaults to default branch)"`
	RangeStart int                  `json:"range_start,omitempty" jsonschema:"Start line number for blame range"`
	RangeEnd   int                  `json:"range_end,omitempty"   jsonschema:"End line number for blame range"`
}

// BlameRangeCommitOutput represents the commit info for a blame range.
type BlameRangeCommitOutput struct {
	ID            string `json:"id"`
	Message       string `json:"message"`
	AuthorName    string `json:"author_name"`
	AuthorEmail   string `json:"author_email"`
	AuthoredDate  string `json:"authored_date,omitempty"`
	CommittedDate string `json:"committed_date,omitempty"`
}

// BlameRangeOutput represents one blame range with commit and lines.
type BlameRangeOutput struct {
	Commit BlameRangeCommitOutput `json:"commit"`
	Lines  []string               `json:"lines"`
}

// BlameOutput holds blame information for a file.
type BlameOutput struct {
	toolutil.HintableOutput
	FilePath string             `json:"file_path"`
	Ranges   []BlameRangeOutput `json:"ranges"`
}

// Blame retrieves blame information for a file in a GitLab repository.
func Blame(ctx context.Context, client *gitlabclient.Client, input BlameInput) (BlameOutput, error) {
	if err := ctx.Err(); err != nil {
		return BlameOutput{}, err
	}
	if input.ProjectID == "" {
		return BlameOutput{}, errors.New("fileBlame: project_id is required")
	}
	opts := &gl.GetFileBlameOptions{}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	if input.RangeStart > 0 {
		opts.RangeStart = new(int64(input.RangeStart))
	}
	if input.RangeEnd > 0 {
		opts.RangeEnd = new(int64(input.RangeEnd))
	}
	ranges, _, err := client.GL().RepositoryFiles.GetFileBlame(string(input.ProjectID), input.FilePath, opts, gl.WithContext(ctx))
	if err != nil {
		return BlameOutput{}, toolutil.WrapErrWithMessage("fileBlame", err)
	}
	out := make([]BlameRangeOutput, len(ranges))
	for i, r := range ranges {
		c := BlameRangeCommitOutput{
			ID:          r.Commit.ID,
			Message:     r.Commit.Message,
			AuthorName:  r.Commit.AuthorName,
			AuthorEmail: r.Commit.AuthorEmail,
		}
		if r.Commit.AuthoredDate != nil {
			c.AuthoredDate = r.Commit.AuthoredDate.Format(time.RFC3339)
		}
		if r.Commit.CommittedDate != nil {
			c.CommittedDate = r.Commit.CommittedDate.Format(time.RFC3339)
		}
		out[i] = BlameRangeOutput{Commit: c, Lines: r.Lines}
	}
	return BlameOutput{FilePath: input.FilePath, Ranges: out}, nil
}

// minLen is an internal helper for the files package.
func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extToLang maps file extensions to code fence language hints.
var extToLang = map[string]string{
	".go":         "go",
	".py":         "python",
	".js":         "javascript",
	".ts":         "typescript",
	".rb":         "ruby",
	".rs":         "rust",
	".java":       "java",
	".kt":         "kotlin",
	".kts":        "kotlin",
	".cs":         "csharp",
	".cpp":        "cpp",
	".cc":         "cpp",
	".cxx":        "cpp",
	".hpp":        "cpp",
	".c":          "c",
	".h":          "c",
	".swift":      "swift",
	".sh":         "bash",
	".bash":       "bash",
	".ps1":        "powershell",
	".psm1":       "powershell",
	".yaml":       "yaml",
	".yml":        "yaml",
	".json":       "json",
	".xml":        "xml",
	".html":       "html",
	".htm":        "html",
	".css":        "css",
	".scss":       "scss",
	".sql":        "sql",
	".md":         "markdown",
	".markdown":   "markdown",
	".dockerfile": "dockerfile",
	".toml":       "toml",
	".ini":        "ini",
	".cfg":        "ini",
	".r":          "r",
	".lua":        "lua",
	".pl":         "perl",
	".pm":         "perl",
	".php":        "php",
	".proto":      "protobuf",
	".tf":         "hcl",
	".graphql":    "graphql",
	".gql":        "graphql",
}

// langFromPath returns a code fence language hint based on the file extension.
func langFromPath(filePath string) string {
	return extToLang[strings.ToLower(path.Ext(filePath))]
}

// ---------------------------------------------------------------------------
// GetFileMetaData
// ---------------------------------------------------------------------------.

// MetaDataInput defines parameters for retrieving file metadata (no content).
type MetaDataInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	FilePath  string               `json:"file_path"  jsonschema:"URL-encoded full path of the file,required"`
	Ref       string               `json:"ref,omitempty" jsonschema:"Branch, tag, or commit SHA (defaults to default branch)"`
}

// MetaDataOutput holds file metadata without content.
type MetaDataOutput struct {
	toolutil.HintableOutput
	FileName        string `json:"file_name"`
	FilePath        string `json:"file_path"`
	Size            int64  `json:"size"`
	Encoding        string `json:"encoding"`
	Ref             string `json:"ref"`
	BlobID          string `json:"blob_id"`
	CommitID        string `json:"commit_id"`
	LastCommitID    string `json:"last_commit_id"`
	ExecuteFilemode bool   `json:"execute_filemode"`
	SHA256          string `json:"content_sha256"`
}

// GetMetaData retrieves file metadata without content from a GitLab repository.
func GetMetaData(ctx context.Context, client *gitlabclient.Client, input MetaDataInput) (MetaDataOutput, error) {
	if err := ctx.Err(); err != nil {
		return MetaDataOutput{}, err
	}
	if input.ProjectID == "" {
		return MetaDataOutput{}, errors.New("fileGetMetaData: project_id is required")
	}
	opts := &gl.GetFileMetaDataOptions{}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	f, _, err := client.GL().RepositoryFiles.GetFileMetaData(string(input.ProjectID), input.FilePath, opts, gl.WithContext(ctx))
	if err != nil {
		return MetaDataOutput{}, toolutil.WrapErrWithMessage("fileGetMetaData", err)
	}
	return MetaDataOutput{
		FileName:        f.FileName,
		FilePath:        f.FilePath,
		Size:            f.Size,
		Encoding:        f.Encoding,
		Ref:             f.Ref,
		BlobID:          f.BlobID,
		CommitID:        f.CommitID,
		LastCommitID:    f.LastCommitID,
		ExecuteFilemode: f.ExecuteFilemode,
		SHA256:          f.SHA256,
	}, nil
}

// ---------------------------------------------------------------------------
// GetRawFile
// ---------------------------------------------------------------------------.

// RawInput defines parameters for retrieving raw file content.
type RawInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	FilePath  string               `json:"file_path"  jsonschema:"URL-encoded full path of the file,required"`
	Ref       string               `json:"ref,omitempty" jsonschema:"Branch, tag, or commit SHA (defaults to default branch)"`
}

// RawOutput holds the raw content of a file.
type RawOutput struct {
	toolutil.HintableOutput
	FilePath        string `json:"file_path"`
	Size            int    `json:"size"`
	Content         string `json:"content"`
	ContentCategory string `json:"content_category"`

	// ImageData holds raw image bytes for ImageContent responses (not serialized).
	ImageData []byte `json:"-"`
	// ImageMIMEType holds the MIME type for image files (not serialized).
	ImageMIMEType string `json:"-"`
}

// GetRaw retrieves the raw content of a file from a GitLab repository.
func GetRaw(ctx context.Context, client *gitlabclient.Client, input RawInput) (RawOutput, error) {
	if err := ctx.Err(); err != nil {
		return RawOutput{}, err
	}
	if input.ProjectID == "" {
		return RawOutput{}, errors.New("fileGetRaw: project_id is required")
	}
	opts := &gl.GetRawFileOptions{}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	data, _, err := client.GL().RepositoryFiles.GetRawFile(string(input.ProjectID), input.FilePath, opts, gl.WithContext(ctx))
	if err != nil {
		return RawOutput{}, toolutil.WrapErrWithMessage("fileGetRaw", err)
	}

	imageMIME := toolutil.ImageMIMEType(input.FilePath)
	isBinary := toolutil.IsBinaryFile(input.FilePath)

	var content string
	var imageData []byte
	category := "text"

	switch {
	case imageMIME != "":
		imageData = data
		category = "image"
	case isBinary:
		category = "binary"
	default:
		content = string(data)
	}

	return RawOutput{
		FilePath:        input.FilePath,
		Size:            len(data),
		Content:         content,
		ContentCategory: category,
		ImageData:       imageData,
		ImageMIMEType:   imageMIME,
	}, nil
}

// ---------------------------------------------------------------------------
// GetRawFileMetaData
// ---------------------------------------------------------------------------.

// RawMetaDataInput defines parameters for retrieving raw file metadata via HEAD request.
type RawMetaDataInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	FilePath  string               `json:"file_path"  jsonschema:"URL-encoded full path of the file,required"`
	Ref       string               `json:"ref,omitempty" jsonschema:"Branch, tag, or commit SHA (defaults to default branch)"`
}

// GetRawFileMetaData retrieves file metadata from the raw file endpoint
// (HEAD request). Returns the same metadata as GetFileMetaData but uses
// a different API endpoint. Useful for checking file existence efficiently.
func GetRawFileMetaData(ctx context.Context, client *gitlabclient.Client, input RawMetaDataInput) (MetaDataOutput, error) {
	if err := ctx.Err(); err != nil {
		return MetaDataOutput{}, err
	}
	if input.ProjectID == "" {
		return MetaDataOutput{}, errors.New("fileGetRawMetaData: project_id is required. Use gitlab_list_projects to find the ID first, then pass it as project_id")
	}
	opts := &gl.GetRawFileOptions{}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	f, _, err := client.GL().RepositoryFiles.GetRawFileMetaData(string(input.ProjectID), input.FilePath, opts, gl.WithContext(ctx))
	if err != nil {
		return MetaDataOutput{}, toolutil.WrapErrWithMessage("fileGetRawMetaData", err)
	}
	return MetaDataOutput{
		FileName:        f.FileName,
		FilePath:        f.FilePath,
		Size:            f.Size,
		Encoding:        f.Encoding,
		Ref:             f.Ref,
		BlobID:          f.BlobID,
		CommitID:        f.CommitID,
		LastCommitID:    f.LastCommitID,
		ExecuteFilemode: f.ExecuteFilemode,
		SHA256:          f.SHA256,
	}, nil
}
