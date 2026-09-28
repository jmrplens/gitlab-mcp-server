package repositorysubmodules

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ListInput defines parameters for listing submodules in a repository.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Ref       string               `json:"ref,omitempty" jsonschema:"Branch name, tag, or commit SHA (defaults to default branch)"`
}

// SubmoduleEntry represents a single submodule with its configuration and
// current commit pointer. URL is the remote as .gitmodules writes it with any
// credentials removed ([redactRemote]); ResolvedProject is the project path it
// names, and carries no credentials either ([resolveProjectPath]).
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

	// The repository files API rejects requests without a ref, so honor the
	// documented "defaults to default branch" contract with the HEAD alias.
	ref := input.Ref
	if ref == "" {
		ref = "HEAD"
	}
	fileOpts := &gl.GetFileOptions{Ref: &ref}

	f, _, err := client.GL().RepositoryFiles.GetFile(string(input.ProjectID), ".gitmodules", fileOpts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("listRepositorySubmodules", fmt.Errorf("could not read .gitmodules: %w", err), http.StatusNotFound, "verify project_id and ref. Ensure .gitmodules file exists")
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

	enrichSubmoduleCommitSHAs(ctx, client, string(input.ProjectID), ref, entries)

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
				current.URL = redactRemote(val)
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
//
// The SCP branch is taken on the absence of a scheme and not on the presence
// of an "@" and a ":", which every credentialed https remote also has. Read
// the old way, "https://user:password@host/group/project.git" split on its
// first colon and resolved to "/user:password@host/group/project", so the
// password was rendered into the submodule table as the project name. What a
// scheme introduces is parsed with net/url, whose User is dropped and never
// read.
func resolveProjectPath(rawURL string) string {
	if !strings.Contains(rawURL, "://") {
		// SCP-style: [user@]host:path.git, and the userinfo goes first so that
		// a password written where ssh would never accept one cannot survive
		// the colon split either.
		remote := withoutSCPUserinfo(rawURL)
		if _, path, found := strings.Cut(remote, ":"); found {
			return strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".git")
		}
		return strings.TrimSuffix(strings.TrimPrefix(remote, "/"), ".git")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := strings.TrimPrefix(u.Path, "/")
	return strings.TrimSuffix(path, ".git")
}

// withoutSCPUserinfo removes the credentials from an SCP-style remote,
// "[user[:password]@]host:path". A bare "git@" is left alone: it is the ssh
// account every such remote is written with and identifies nobody, while a
// userinfo carrying a colon carries a password.
//
// With no "@" the userinfo is empty, read as authority[:0], so one test
// decides both cases: a comparison of the index against zero would treat an
// "@" in first place the same way on either side of it.
func withoutSCPUserinfo(remote string) string {
	authority, _, _ := strings.Cut(remote, "/")
	at := strings.LastIndex(authority, "@")
	if !strings.Contains(authority[:max(at, 0)], ":") {
		return remote
	}
	return remote[at+1:]
}

// redactRemote removes the credentials a .gitmodules remote can carry before
// the remote is published on the output. The file is part of the repository,
// so whoever can push chooses the text, and a submodule pinned through
// "https://user:password@host/group/project.git" would otherwise hand that
// password to the model in the structured result beside the table.
//
// Only the userinfo goes: the host and the path are what identify the
// submodule, which is the whole point of reporting the remote.
func redactRemote(rawURL string) string {
	if !strings.Contains(rawURL, "://") {
		return withoutSCPUserinfo(rawURL)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		// A remote carrying a scheme that will not parse is not something this
		// can judge, so nothing of it is published.
		return ""
	}
	if u.User == nil {
		return rawURL
	}
	u.User = nil
	return u.String()
}

// enrichSubmoduleCommitSHAs walks the repository tree to find nodes of type
// "commit" (mode 160000) and fills in the CommitSHA field on matching entries.
func enrichSubmoduleCommitSHAs(ctx context.Context, client *gitlabclient.Client, projectID, ref string, entries []SubmoduleEntry) {
	pathIndex, dirCounts := buildSubmoduleIndex(entries)

	for dir, want := range dirCounts {
		if err := ctx.Err(); err != nil {
			return
		}
		matchTreeCommits(ctx, client, projectID, ref, dir, pathIndex, want)
	}
}

// buildSubmoduleIndex creates a path→entry lookup and, for each parent
// directory, the number of distinct submodule paths it holds, which is how
// many commit entries a listing of that directory can match.
func buildSubmoduleIndex(entries []SubmoduleEntry) (pathIndex map[string]*SubmoduleEntry, dirCounts map[string]int) {
	pathIndex = make(map[string]*SubmoduleEntry, len(entries))
	for i := range entries {
		pathIndex[entries[i].Path] = &entries[i]
	}
	dirCounts = make(map[string]int)
	for path := range pathIndex {
		dirCounts[parentDir(path)]++
	}
	return pathIndex, dirCounts
}

// matchTreeCommits lists a single directory of the repository tree and fills
// in CommitSHA for any submodule entries whose path matches a "commit" tree
// node, stopping once it has matched the want submodules the directory holds.
//
// It follows the pages GitLab announces, through client-go's own pagination
// iterator, because a directory holding more entries than one page does is
// ordinary (a vendor directory, a monorepo's root), and a submodule sorted onto
// a later page used to be reported with no commit at all. It asks for no page
// past the one that holds the last submodule, since leaving the loop stops
// the iterator: a large directory costs the pages up to its last submodule
// rather than all of them.
//
// ref is always named: [List] resolves an empty one to the HEAD alias before
// any of this runs, so there is no second check for it here.
func matchTreeCommits(ctx context.Context, client *gitlabclient.Client, projectID, ref, dir string, pathIndex map[string]*SubmoduleEntry, want int) {
	found := 0
	for n, err := range treeNodes(ctx, client, projectID, ref, dir) {
		if err != nil {
			return
		}
		entry, ok := pathIndex[n.Path]
		if !ok || n.Type != "commit" {
			continue
		}
		entry.CommitSHA = n.ID
		found++
		if found == want {
			return
		}
	}
}

// treeNodes lists one directory of the repository tree at ref, a hundred
// entries a page, and yields every entry of every page GitLab announces.
// Paging is client-go's [gl.Scan2], which asks for the next page for as long
// as GitLab's answer names one; a consumer that stops early stops the listing.
func treeNodes(ctx context.Context, client *gitlabclient.Client, projectID, ref, dir string) iter.Seq2[*gl.TreeNode, error] {
	opts := &gl.ListTreeOptions{}
	opts.PerPage = 100
	opts.Ref = new(ref)
	if dir != "" {
		opts.Path = new(dir)
	}
	return gl.Scan2(func(page gl.PaginationOptionFunc) ([]*gl.TreeNode, *gl.Response, error) {
		return client.GL().Repositories.ListTree(projectID, opts, gl.WithContext(ctx), page)
	})
}

// parentDir returns the parent directory of path, or "" for root-level paths.
// A path with no separator and one whose only separator leads it both have
// the root as their parent, so the index is floored at zero rather than
// compared, which would give the same answer for the two on either side of
// the comparison and leave nothing a test could hold it to.
func parentDir(path string) string {
	return path[:max(strings.LastIndex(path, "/"), 0)]
}
