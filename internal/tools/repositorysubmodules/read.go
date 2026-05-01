package repositorysubmodules

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ReadInput defines parameters for reading a file inside a submodule.
type ReadInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path of the parent repository,required"`
	SubmodulePath string               `json:"submodule_path" jsonschema:"Path to the submodule as defined in .gitmodules (e.g. libs/core-module),required"`
	FilePath      string               `json:"file_path" jsonschema:"Path of the file inside the submodule (e.g. src/main.c),required"`
	Ref           string               `json:"ref,omitempty" jsonschema:"Branch/tag/SHA in the parent repository (defaults to default branch)"`
}

// ReadOutput holds the file content retrieved from inside a submodule.
type ReadOutput struct {
	toolutil.HintableOutput
	FileName        string `json:"file_name"`
	FilePath        string `json:"file_path"`
	SubmodulePath   string `json:"submodule_path"`
	ResolvedProject string `json:"resolved_project"`
	CommitSHA       string `json:"commit_sha"`
	Size            int64  `json:"size"`
	Content         string `json:"content"`
	Encoding        string `json:"encoding"`
}

// Read resolves a submodule in the parent repository and retrieves a file
// from the submodule's target project at the pinned commit SHA.
//
// Steps:
//  1. Get .gitmodules from parent to resolve submodule URL → project path
//  2. Get tree entry for the submodule path to obtain the pinned commit SHA
//  3. Fetch the file from the resolved project at that commit SHA
func Read(ctx context.Context, client *gitlabclient.Client, input ReadInput) (ReadOutput, error) {
	if err := ctx.Err(); err != nil {
		return ReadOutput{}, err
	}
	if input.ProjectID == "" {
		return ReadOutput{}, errors.New("readRepositorySubmodule: project_id is required")
	}
	if input.SubmodulePath == "" {
		return ReadOutput{}, errors.New("readRepositorySubmodule: submodule_path is required")
	}
	if input.FilePath == "" {
		return ReadOutput{}, errors.New("readRepositorySubmodule: file_path is required")
	}

	projectID := string(input.ProjectID)
	ref := input.Ref

	// Step 1: Parse .gitmodules to find the submodule's remote URL
	resolvedProject, err := resolveSubmoduleProject(ctx, client, projectID, ref, input.SubmodulePath)
	if err != nil {
		return ReadOutput{}, err
	}

	// Step 2: Get the pinned commit SHA from the tree entry
	commitSHA, err := getSubmoduleCommitSHA(ctx, client, projectID, ref, input.SubmodulePath)
	if err != nil {
		return ReadOutput{}, err
	}

	// Step 3: Fetch the file from the resolved project at the pinned commit
	fileOpts := &gl.GetFileOptions{
		Ref: new(commitSHA),
	}
	f, _, err := client.GL().RepositoryFiles.GetFile(resolvedProject, input.FilePath, fileOpts, gl.WithContext(ctx))
	if err != nil {
		return ReadOutput{}, toolutil.WrapErrWithStatusHint("readRepositorySubmodule",
			fmt.Errorf("file %q in submodule %q (project %q, commit %s): %w",
				input.FilePath, input.SubmodulePath, resolvedProject, commitSHA[:minLen(8, len(commitSHA))], err),
			http.StatusNotFound, "verify project_id, ref, submodule_path, and file_path are correct")
	}

	content := f.Content
	if f.Encoding == "base64" {
		var decoded []byte
		decoded, err = base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			return ReadOutput{}, fmt.Errorf("readRepositorySubmodule: decode base64 content: %w", err)
		}
		content = string(decoded)
	}

	return ReadOutput{
		FileName:        f.FileName,
		FilePath:        f.FilePath,
		SubmodulePath:   input.SubmodulePath,
		ResolvedProject: resolvedProject,
		CommitSHA:       commitSHA,
		Size:            f.Size,
		Content:         content,
		Encoding:        f.Encoding,
	}, nil
}

// resolveSubmoduleProject reads .gitmodules from the parent repository and
// extracts the project path for the given submodule.
func resolveSubmoduleProject(ctx context.Context, client *gitlabclient.Client, projectID, ref, submodulePath string) (string, error) {
	fileOpts := &gl.GetFileOptions{}
	if ref != "" {
		fileOpts.Ref = new(ref)
	}

	f, _, err := client.GL().RepositoryFiles.GetFile(projectID, ".gitmodules", fileOpts, gl.WithContext(ctx))
	if err != nil {
		return "", toolutil.WrapErrWithStatusHint("readRepositorySubmodule",
			fmt.Errorf("could not read .gitmodules from project %s: %w", projectID, err),
			http.StatusNotFound, "verify the project contains a .gitmodules file and the ref exists")
	}

	content := f.Content
	if f.Encoding == "base64" {
		var decoded []byte
		decoded, err = base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			return "", fmt.Errorf("readRepositorySubmodule: decode .gitmodules: %w", err)
		}
		content = string(decoded)
	}

	entries := parseGitmodules(content)
	for _, e := range entries {
		if e.Path == submodulePath {
			if e.ResolvedProject == "" {
				return "", fmt.Errorf("readRepositorySubmodule: could not resolve project path from submodule URL %q", e.URL)
			}
			return e.ResolvedProject, nil
		}
	}
	return "", fmt.Errorf("readRepositorySubmodule: submodule %q not found in .gitmodules. Available submodules: %s",
		submodulePath, listSubmodulePaths(entries))
}

// getSubmoduleCommitSHA retrieves the commit SHA that the submodule pointer
// references by looking up the tree entry of type "commit".
func getSubmoduleCommitSHA(ctx context.Context, client *gitlabclient.Client, projectID, ref, submodulePath string) (string, error) {
	dir := parentDir(submodulePath)
	opts := &gl.ListTreeOptions{}
	opts.PerPage = 100
	if dir != "" {
		opts.Path = new(dir)
	}
	if ref != "" {
		opts.Ref = new(ref)
	}

	nodes, _, err := client.GL().Repositories.ListTree(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return "", toolutil.WrapErrWithStatusHint("readRepositorySubmodule",
			fmt.Errorf("could not list tree for submodule path %q: %w", submodulePath, err),
			http.StatusNotFound, "verify the submodule_path exists in the project tree at the given ref")
	}

	for _, n := range nodes {
		if n.Path == submodulePath && n.Type == "commit" {
			return n.ID, nil
		}
	}
	return "", fmt.Errorf("readRepositorySubmodule: submodule %q not found as a tree entry of type 'commit' in the repository tree", submodulePath)
}

// listSubmodulePaths formats available submodule paths for error messages.
func listSubmodulePaths(entries []SubmoduleEntry) string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return strings.Join(paths, ", ")
}

// minLen returns the smaller of a and b.
func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
