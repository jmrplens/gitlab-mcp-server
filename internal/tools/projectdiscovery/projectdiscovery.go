// Package projectdiscovery implements tools to help LLMs discover the GitLab
// project associated with a local workspace. It parses git remote URLs into
// GitLab path_with_namespace and resolves them via the GitLab API.
package projectdiscovery

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ResolveInput defines parameters for resolving a git remote URL to a GitLab project.
type ResolveInput struct {
	RemoteURL string `json:"remote_url" jsonschema:"Full git remote URL (HTTPS or SSH) exactly as shown in .git/config or 'git remote -v' output. IMPORTANT: pass the complete URL including the scheme (https://) or user prefix (git@). Examples: 'https://gitlab.example.com/group/project.git' or 'git@gitlab.example.com:group/project.git'. Do NOT strip the git@ prefix from SSH URLs.,required"`
}

// ResolveOutput holds the resolved GitLab project information.
type ResolveOutput struct {
	toolutil.HintableOutput
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
	DefaultBranch     string `json:"default_branch"`
	Description       string `json:"description"`
	Visibility        string `json:"visibility"`
	HTTPURLToRepo     string `json:"http_url_to_repo,omitempty"`
	SSHURLToRepo      string `json:"ssh_url_to_repo,omitempty"`
	ExtractedPath     string `json:"extracted_path"`
}

// sshRemotePattern matches SSH remote URLs like git@host:group/project.git.
var sshRemotePattern = regexp.MustCompile(`^[\w.-]+@[\w.-]+:(.+?)(?:\.git)?$`)

// sshMissingUserPattern detects truncated SSH URLs like host:group/project.git
// (missing the user@ prefix). This commonly happens when LLMs extract partial
// URLs from git push output instead of using the full remote URL.
var sshMissingUserPattern = regexp.MustCompile(`^[\w.-]+:([\w./-]+)$`)

// ParseRemoteURL extracts the project path_with_namespace from a git remote URL.
// Supports HTTPS, SSH (git@host:path), and ssh:// protocol URLs.
// Returns the path and nil on success, or empty string and an error.
func ParseRemoteURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("remote URL is empty")
	}

	// SSH shorthand: git@host:group/project.git
	if matches := sshRemotePattern.FindStringSubmatch(rawURL); len(matches) == 2 {
		return cleanPath(matches[1]), nil
	}

	// Detect truncated SSH URLs missing the user@ prefix (e.g. "host:group/project.git")
	if sshMissingUserPattern.MatchString(rawURL) && !strings.Contains(rawURL, "://") {
		return "", fmt.Errorf("remote URL %q looks like a truncated SSH remote missing the user prefix — "+
			"use the full URL including the user, e.g. git@%s", rawURL, rawURL)
	}

	// Standard URL parsing (https://, ssh://, git://)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid remote URL %q: %w", rawURL, err)
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("remote URL %q has no host — provide the full URL from 'git remote -v' output, "+
			"e.g. https://host/group/project.git or git@host:group/project.git", rawURL)
	}

	path := parsed.Path
	if path == "" {
		return "", fmt.Errorf("remote URL %q has no path", rawURL)
	}

	return cleanPath(path), nil
}

// cleanPath normalizes a URL path into a GitLab path_with_namespace.
func cleanPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, ".git")
	p = strings.TrimSuffix(p, "/")
	return p
}

// Resolve parses a git remote URL, extracts the project path, and looks up
// the project via the GitLab API. Returns the project details or an error
// with guidance on what to try next.
func Resolve(ctx context.Context, client *gitlabclient.Client, input ResolveInput) (ResolveOutput, error) {
	if err := ctx.Err(); err != nil {
		return ResolveOutput{}, err
	}

	projectPath, err := ParseRemoteURL(input.RemoteURL)
	if err != nil {
		return ResolveOutput{}, fmt.Errorf("could not parse remote URL: %w — provide the URL from 'git remote -v' output", err)
	}

	project, _, err := client.GL().Projects.GetProject(projectPath, nil, gl.WithContext(ctx))
	if err != nil {
		return ResolveOutput{}, fmt.Errorf("project %q not found on GitLab: %w — verify the remote URL matches a project you have access to", projectPath, err)
	}

	return ResolveOutput{
		ID:                project.ID,
		Name:              project.Name,
		Path:              project.Path,
		PathWithNamespace: project.PathWithNamespace,
		WebURL:            project.WebURL,
		DefaultBranch:     project.DefaultBranch,
		Description:       project.Description,
		Visibility:        string(project.Visibility),
		HTTPURLToRepo:     project.HTTPURLToRepo,
		SSHURLToRepo:      project.SSHURLToRepo,
		ExtractedPath:     projectPath,
	}, nil
}
