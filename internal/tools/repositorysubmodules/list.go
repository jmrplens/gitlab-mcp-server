// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package repositorysubmodules

import (
	"bufio"
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"path"
	"regexp"
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
// credentials removed ([redactRemote]); ResolvedProject is the project on this
// instance it names ([submoduleResolver]), and carries no credentials either.
// A submodule naming no project here is marked one of two ways, and cannot be
// read through this server: External when its remote names another host,
// where it can be read instead, and Unresolved otherwise, a relative URL that
// lands inside the superproject or at the root, a remote on this instance's
// host that is not a project's clone URL, or one that names no host at all.
type SubmoduleEntry struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	URL             string `json:"url"`
	ResolvedProject string `json:"resolved_project,omitempty"`
	External        bool   `json:"external,omitempty"`
	Unresolved      bool   `json:"unresolved,omitempty"`
	CommitSHA       string `json:"commit_sha"`

	// remote is the url as .gitmodules writes it, credentials included. It is
	// what the resolver reads, and it is cleared once the entry is resolved,
	// so it never outlives the call that parsed it.
	remote string
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

	resolver, err := newSubmoduleResolver(ctx, client, string(input.ProjectID), "listRepositorySubmodules")
	if err != nil {
		return ListOutput{}, err
	}
	resolver.resolveAll(entries)

	enrichSubmoduleCommitSHAs(ctx, client, string(input.ProjectID), ref, entries)

	return ListOutput{Submodules: entries, Count: len(entries)}, nil
}

// submoduleResolver decides which project on this instance a .gitmodules
// remote names. It follows the rule GitLab's own SubmoduleHelper links a
// submodule with (app/helpers/submodule_helper.rb) except where that helper
// compares text and git, which is what fetches a submodule, compares
// addresses. A URL of "." or "./" is the superproject itself. A relative URL
// resolves against the superproject's path, as File.absolute_path does, and
// names nothing when the namespace it lands in is the root, the
// superproject's own path or a path below it, compared by path segment where
// GitLab compares strings, so a sibling namespace whose name begins with the
// superproject's ("../app-tools/lib" from group/app) is not taken for one
// inside it. An absolute URL names a project when it is an http or https URL
// on the instance's web URL, relative root included
// ([toolutil.PathBelowInstanceWeb]), or an SSH remote, in the SCP or the
// ssh:// form, on the account, host and port of the instance's SSH clone
// URLs ([parseSSHRemote]); a host in capitals, the default port written out
// and the ssh:// spelling of an SCP clone URL name the same instance, which
// GitLab's text comparison leaves unlinked (see "SubmoduleHelper compares
// submodule URLs as text" in docs/development/upstream-bugs.md).
//
// The host used to be discarded and the path kept, so a submodule on any
// other server was read from whichever local project happened to share its
// path, and a relative URL was looked up as its literal "../lib".
type submoduleResolver struct {
	apiBase      *url.URL  // the client's REST API base, from which the web URL comes
	superproject string    // the superproject's path_with_namespace
	ssh          sshRemote // the account, host, port and path prefix of this instance's SSH clone URLs; zero when not known
}

// newSubmoduleResolver reads the superproject, whose path a relative
// submodule URL is resolved against and whose SSH clone URL shows the
// prefix the instance writes before a project path.
func newSubmoduleResolver(ctx context.Context, client *gitlabclient.Client, projectID, operation string) (submoduleResolver, error) {
	//gitlab:request mandatory: a submodule URL resolves against the superproject's path and its SSH clone URL, so the superproject is read on every call that finds a submodule
	project, _, err := client.GL().Projects.GetProject(projectID, nil, gl.WithContext(ctx))
	if err != nil {
		return submoduleResolver{}, toolutil.WrapErrWithStatusHint(operation,
			fmt.Errorf("could not read project %s: %w", projectID, err),
			http.StatusNotFound, "verify project_id with project.get")
	}
	return newResolver(client.GL().BaseURL(), project.PathWithNamespace,
		sshPathPrefix(project.SSHURLToRepo, project.PathWithNamespace)), nil
}

// newResolver builds the resolver for the superproject at superproject on
// the instance whose REST API is at apiBase and whose SSH clone URLs start
// with sshPrefix, "" when that is not known.
func newResolver(apiBase *url.URL, superproject, sshPrefix string) submoduleResolver {
	ssh, _ := parseSSHRemote(sshPrefix)
	return submoduleResolver{apiBase: apiBase, superproject: superproject, ssh: ssh}
}

// sshRemote is an SSH remote as git reads it: the account it logs in as, the
// host and the port it connects to, and the path it asks the server for.
type sshRemote struct {
	user, host, port, path string
}

// parseSSHRemote reads an SSH remote in either form git takes, a URL of the
// ssh scheme (git+ssh and ssh+git included) and the SCP form
// [user@]host:path, whose port is always 22, and reports false for anything
// else. The host is lowercased, as DNS compares it. The SCP form's account is
// everything before the last "@" ahead of the first "/", the way
// [withoutSCPUserinfo] reads it, so the host a remote is filed under is the
// one its published URL shows.
func parseSSHRemote(remote string) (sshRemote, bool) {
	if scheme, rest, isURL := strings.Cut(remote, "://"); isURL {
		if scheme != "ssh" && scheme != "git+ssh" && scheme != "ssh+git" {
			return sshRemote{}, false
		}
		u, err := url.Parse("ssh://" + rest)
		if err != nil {
			return sshRemote{}, false
		}
		return sshRemote{user: u.User.Username(), host: strings.ToLower(u.Hostname()), port: cmp.Or(u.Port(), "22"), path: strings.TrimPrefix(u.Path, "/")}, true
	}
	authority, _, _ := strings.Cut(remote, "/")
	at := strings.LastIndex(authority, "@")
	host, asked, isSCP := strings.Cut(remote[at+1:], ":")
	if !isSCP || host == "" || strings.Contains(host, "/") {
		return sshRemote{}, false
	}
	return sshRemote{user: remote[:max(at, 0)], host: strings.ToLower(host), port: "22", path: asked}, true
}

// remoteHost returns the host a remote names, lowercased, and "" for one
// that names none: a relative URL, a local path, a file URL, or a URL that
// does not parse.
func remoteHost(remote string) string {
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return ""
		}
		return strings.ToLower(u.Hostname())
	}
	ssh, _ := parseSSHRemote(remote)
	return ssh.host
}

// sshPathPrefix returns what an SSH clone URL writes before the project's
// path, which is GitLab's gitlab_shell.ssh_path_prefix ("git@host:" or
// "ssh://git@host:2222/"), and "" when the clone URL does not end in the
// path it is the clone URL of.
func sshPathPrefix(sshURLToRepo, pathWithNamespace string) string {
	prefix, _ := strings.CutSuffix(sshURLToRepo, pathWithNamespace+".git")
	if prefix == sshURLToRepo {
		return ""
	}
	return prefix
}

// submoduleURLShape is the shape GitLab requires of a submodule URL before it
// links it at all: it ends in a segment holding neither "/" nor ":", a "/",
// and a last segment holding no "/".
var submoduleURLShape = regexp.MustCompile(`[^/:]+/[^/]+\z`)

// resolveAll resolves every entry, marks one that names no project here as
// external or unresolved, and clears the remote each was resolved from.
func (r submoduleResolver) resolveAll(entries []SubmoduleEntry) {
	for i := range entries {
		e := &entries[i]
		e.ResolvedProject = r.resolve(e.remote)
		if e.ResolvedProject == "" {
			e.External = r.onAnotherHost(e.remote)
			e.Unresolved = !e.External
		}
		e.remote = ""
	}
}

// onAnotherHost reports whether a remote names a host that is neither this
// instance's web host nor the host of its SSH clone URLs. A remote naming no
// host at all is not on another host: nothing says where it is.
func (r submoduleResolver) onAnotherHost(remote string) bool {
	host := remoteHost(remote)
	return host != "" && host != strings.ToLower(r.apiBase.Hostname()) && host != r.ssh.host
}

// resolve returns the path of the project on this instance the submodule
// remote names, and "" when it names none.
func (r submoduleResolver) resolve(remote string) string {
	if remote == "." || remote == "./" {
		return r.superproject
	}
	if !submoduleURLShape.MatchString(remote) {
		return ""
	}
	if strings.HasPrefix(remote, "../") || strings.HasPrefix(remote, "./") {
		return r.resolveRelative(remote)
	}
	if below, ok := toolutil.PathBelowInstanceWeb(r.apiBase, remote); ok {
		return projectPathIn(below)
	}
	if below, ok := r.pathBelowSSH(remote); ok {
		return projectPathIn(below)
	}
	return ""
}

// pathBelowSSH reports the path an SSH remote asks this instance's SSH
// server for, below the path prefix of its clone URLs, when the remote logs
// in as the account those clone URLs name, on their host and port; ok is
// false for any other remote, and for every remote when the instance's SSH
// clone URLs are not known: the zero sshRemote that stands for them then has
// no port, and every remote that parses has one.
func (r submoduleResolver) pathBelowSSH(remote string) (string, bool) {
	rem, ok := parseSSHRemote(remote)
	if !ok || rem.user != r.ssh.user || rem.host != r.ssh.host || rem.port != r.ssh.port {
		return "", false
	}
	return strings.CutPrefix(rem.path, r.ssh.path)
}

// resolveRelative resolves a relative submodule URL against the
// superproject's path the way GitLab's relative_self_links does:
// "../lib.git" from group/app is group/lib, "../../other/lib" is other/lib,
// and a URL landing in the root namespace, or in the superproject's own path
// or a path below it ("./x" from group/app is group/app/x), names nothing.
// The containment is judged by path segment: GitLab's own check is a string
// prefix, which also refuses a sibling namespace whose name begins with the
// superproject's ("../app-tools/lib" from group/app). path.Join clamps a
// climb above the root at the root, as File.absolute_path does.
func (r submoduleResolver) resolveRelative(remote string) string {
	superproject := "/" + r.superproject
	target := path.Join(superproject, remote)
	namespace := path.Dir(target)
	if namespace == "/" || namespace == superproject || strings.HasPrefix(namespace, superproject+"/") {
		return ""
	}
	return strings.TrimPrefix(namespace, "/") + "/" + strings.TrimSuffix(path.Base(target), ".git")
}

// projectPathIn returns the project path a remote names below the instance's
// prefix, without the ".git" GitLab drops, and "" when it is not a namespace
// and a project.
func projectPathIn(below string) string {
	project := strings.TrimSuffix(below, ".git")
	if !strings.Contains(project, "/") || strings.HasPrefix(project, "/") {
		return ""
	}
	return project
}

// parseGitmodules parses .gitmodules INI-like content into SubmoduleEntry
// slices. Each [submodule "name"] section contributes one entry with path
// and url fields; the entries are resolved afterwards
// ([submoduleResolver.resolveAll]), since that takes a read of the
// superproject a .gitmodules with no submodule does not need.
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
				current.remote = val
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
	for p := range pathIndex {
		dirCounts[parentDir(p)]++
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

// parentDir returns the parent directory of p, or "" for root-level paths.
// A path with no separator and one whose only separator leads it both have
// the root as their parent, so the index is floored at zero rather than
// compared, which would give the same answer for the two on either side of
// the comparison and leave nothing a test could hold it to.
func parentDir(p string) string {
	return p[:max(strings.LastIndex(p, "/"), 0)]
}
