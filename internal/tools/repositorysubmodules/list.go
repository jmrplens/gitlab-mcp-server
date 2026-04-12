// Package repositorysubmodules implements an MCP tool handler for listing Git submodules
// in a GitLab repository by parsing .gitmodules and enriching each
// entry with the commit SHA from the repository tree.
package repositorysubmodules

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing submodules in a repository.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Ref       string               `json:"ref,omitempty" jsonschema:"Branch name, tag, or commit SHA (defaults to default branch)"`
}

// SubmoduleEntry represents a single submodule with its configuration and
// current commit pointer.
type SubmoduleEntry struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	URL             string `json:"url"`
	ResolvedProject string `json:"resolved_project,omitempty"`
	CommitSHA       string `json:"commit_sha"`
}

// ListOutput holds the list of submodules found in the repository.
type ListOutput struct {
	toolutil.HintableOutput
	Submodules []SubmoduleEntry `json:"submodules"`
	Count      int              `json:"count"`
}

// List retrieves all submodules defined in a repository by parsing
// .gitmodules and correlating each entry with its commit SHA from the
// repository tree.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("listRepositorySubmodules: project_id is required")
	}

	fileOpts := &gl.GetFileOptions{}
	if input.Ref != "" {
		fileOpts.Ref = new(input.Ref)
	}

	f, _, err := client.GL().RepositoryFiles.GetFile(string(input.ProjectID), ".gitmodules", fileOpts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("listRepositorySubmodules", fmt.Errorf("could not read .gitmodules: %w", err))
	}

	content := f.Content
	if f.Encoding == "base64" {
		var decoded []byte
		decoded, err = base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			return ListOutput{}, fmt.Errorf("listRepositorySubmodules: decode .gitmodules: %w", err)
		}
		content = string(decoded)
	}

	entries := parseGitmodules(content)
	if len(entries) == 0 {
		return ListOutput{Submodules: []SubmoduleEntry{}, Count: 0}, nil
	}

	enrichSubmoduleCommitSHAs(ctx, client, string(input.ProjectID), input.Ref, entries)

	return ListOutput{Submodules: entries, Count: len(entries)}, nil
}

// parseGitmodules parses .gitmodules INI-like content into SubmoduleEntry
// slices. Each [submodule "name"] section contributes one entry with path
// and url fields.
func parseGitmodules(content string) []SubmoduleEntry {
	var entries []SubmoduleEntry
	var current *SubmoduleEntry

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[submodule ") {
			name := strings.TrimPrefix(line, "[submodule \"")
			name = strings.TrimSuffix(name, "\"]")
			entries = append(entries, SubmoduleEntry{Name: name})
			current = &entries[len(entries)-1]
			continue
		}
		if current == nil {
			continue
		}
		if key, val, ok := parseKeyValue(line); ok {
			switch key {
			case "path":
				current.Path = val
			case "url":
				current.URL = val
				current.ResolvedProject = resolveProjectPath(val)
			}
		}
	}
	return entries
}

// parseKeyValue splits a "key = value" line into its components.
func parseKeyValue(line string) (key, value string, ok bool) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

// resolveProjectPath converts a GitLab remote URL (SSH or HTTPS) to the
// project path used by the API (e.g. "group/subgroup/project").
//
// Supported formats:
//   - git@host:group/project.git
//   - https://host/group/project.git
//   - ssh://git@host/group/project.git
func resolveProjectPath(rawURL string) string {
	if strings.Contains(rawURL, "@") && strings.Contains(rawURL, ":") && !strings.HasPrefix(rawURL, "ssh://") {
		// SCP-style: git@host:path.git
		parts := strings.SplitN(rawURL, ":", 2)
		if len(parts) == 2 {
			return strings.TrimSuffix(strings.TrimPrefix(parts[1], "/"), ".git")
		}
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := strings.TrimPrefix(u.Path, "/")
	return strings.TrimSuffix(path, ".git")
}

// enrichSubmoduleCommitSHAs walks the repository tree to find nodes of type
// "commit" (mode 160000) and fills in the CommitSHA field on matching entries.
func enrichSubmoduleCommitSHAs(ctx context.Context, client *gitlabclient.Client, projectID, ref string, entries []SubmoduleEntry) {
	pathIndex, dirSet := buildSubmoduleIndex(entries)

	for dir := range dirSet {
		if err := ctx.Err(); err != nil {
			return
		}
		matchTreeCommits(ctx, client, projectID, ref, dir, pathIndex)
	}
}

// buildSubmoduleIndex creates a path→entry lookup and a set of unique parent directories.
func buildSubmoduleIndex(entries []SubmoduleEntry) (pathIndex map[string]*SubmoduleEntry, dirSet map[string]struct{}) {
	pathIndex = make(map[string]*SubmoduleEntry, len(entries))
	dirSet = make(map[string]struct{})
	for i := range entries {
		pathIndex[entries[i].Path] = &entries[i]
		dir := parentDir(entries[i].Path)
		dirSet[dir] = struct{}{}
	}
	return pathIndex, dirSet
}

// matchTreeCommits fetches a single directory from the repository tree and fills in
// CommitSHA for any submodule entries whose path matches a "commit" tree node.
func matchTreeCommits(ctx context.Context, client *gitlabclient.Client, projectID, ref, dir string, pathIndex map[string]*SubmoduleEntry) {
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
		return
	}
	for _, n := range nodes {
		if entry, ok := pathIndex[n.Path]; ok && n.Type == "commit" {
			entry.CommitSHA = n.ID
		}
	}
}

// parentDir returns the parent directory of path, or "" for root-level paths.
func parentDir(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return ""
	}
	return path[:idx]
}
