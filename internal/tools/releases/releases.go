// Package releases implements MCP tool handlers for GitLab release operations
// including create, update, delete, get, and list.
// It wraps the Releases service from client-go v2.
package releases

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CreateInput defines parameters for creating a GitLab release.
type CreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	TagName     string               `json:"tag_name"              jsonschema:"Tag name associated with the release,required"`
	Name        string               `json:"name,omitempty"        jsonschema:"Release title"`
	Description string               `json:"description,omitempty" jsonschema:"Release notes (Markdown supported)"`
	ReleasedAt  string               `json:"released_at,omitempty" jsonschema:"Date of the release in ISO 8601 format"`
	Ref         string               `json:"ref,omitempty"         jsonschema:"Branch or commit SHA to create tag from (if tag does not exist)"`
	Milestones  []string             `json:"milestones,omitempty"  jsonschema:"Milestone titles to associate with the release"`
	TagMessage  string               `json:"tag_message,omitempty" jsonschema:"Message to use for the annotated tag (creates annotated tag instead of lightweight)"`
}

// AssetSourceOutput represents a single release asset source (auto-generated archive).
type AssetSourceOutput struct {
	Format string `json:"format"`
	URL    string `json:"url"`
}

// EvidenceOutput represents a release evidence record.
type EvidenceOutput struct {
	SHA         string `json:"sha"`
	Filepath    string `json:"filepath"`
	CollectedAt string `json:"collected_at,omitempty"`
}

// Output represents a GitLab release.
type Output struct {
	toolutil.HintableOutput
	TagName         string                `json:"tag_name"`
	Name            string                `json:"name"`
	Description     string                `json:"description"`
	DescriptionHTML string                `json:"description_html,omitempty"`
	CreatedAt       string                `json:"created_at"`
	ReleasedAt      string                `json:"released_at"`
	Author          string                `json:"author,omitempty"`
	CommitSHA       string                `json:"commit_sha,omitempty"`
	UpcomingRelease bool                  `json:"upcoming_release,omitempty"`
	Milestones      []string              `json:"milestones,omitempty"`
	CommitPath      string                `json:"commit_path,omitempty"`
	TagPath         string                `json:"tag_path,omitempty"`
	AssetsCount     int64                 `json:"assets_count,omitempty"`
	AssetsSources   []AssetSourceOutput   `json:"assets_sources,omitempty"`
	AssetsLinks     []releaselinks.Output `json:"assets_links,omitempty"`
	Evidences       []EvidenceOutput      `json:"evidences,omitempty"`
	WebURL          string                `json:"web_url,omitempty"`
}

// UpdateInput defines parameters for updating a release.
type UpdateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	TagName     string               `json:"tag_name"              jsonschema:"Tag name of the release,required"`
	Name        string               `json:"name,omitempty"        jsonschema:"New release title"`
	Description string               `json:"description,omitempty" jsonschema:"Updated release notes"`
	ReleasedAt  string               `json:"released_at,omitempty" jsonschema:"New release date in ISO 8601 format"`
	Milestones  []string             `json:"milestones,omitempty"  jsonschema:"Milestone titles to associate with the release"`
}

// DeleteInput defines parameters for deleting a release.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Tag name of the release to delete,required"`
}

// GetInput defines parameters for getting a release.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Tag name of the release,required"`
}

// GetLatestInput defines parameters for retrieving the latest release.
type GetLatestInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// ListInput defines parameters for listing releases.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order by field (released_at, created_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
}

// ListOutput holds a list of releases.
type ListOutput struct {
	toolutil.HintableOutput
	Releases   []Output                  `json:"releases"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ToOutput converts a GitLab API [gl.Release] to the MCP tool output
// format, formatting timestamps as RFC 3339 strings.
func ToOutput(r *gl.Release) Output {
	out := Output{
		TagName:         r.TagName,
		Name:            r.Name,
		Description:     r.Description,
		DescriptionHTML: r.DescriptionHTML,
		Author:          r.Author.Username,
		UpcomingRelease: r.UpcomingRelease,
		CommitPath:      r.CommitPath,
		TagPath:         r.TagPath,
		AssetsCount:     r.Assets.Count,
	}
	if editURL := r.Links.EditURL; editURL != "" {
		out.WebURL = strings.TrimSuffix(editURL, "/edit")
	}
	if r.Commit.ID != "" {
		out.CommitSHA = r.Commit.ID
	}
	if len(r.Milestones) > 0 {
		ms := make([]string, len(r.Milestones))
		for i, m := range r.Milestones {
			ms[i] = m.Title
		}
		out.Milestones = ms
	}
	if r.CreatedAt != nil {
		out.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	if r.ReleasedAt != nil {
		out.ReleasedAt = r.ReleasedAt.Format(time.RFC3339)
	}
	if len(r.Assets.Sources) > 0 {
		sources := make([]AssetSourceOutput, len(r.Assets.Sources))
		for i, s := range r.Assets.Sources {
			sources[i] = AssetSourceOutput{Format: s.Format, URL: s.URL}
		}
		out.AssetsSources = sources
	}
	if len(r.Assets.Links) > 0 {
		links := make([]releaselinks.Output, len(r.Assets.Links))
		for i, l := range r.Assets.Links {
			links[i] = releaselinks.Output{
				ID:             l.ID,
				Name:           l.Name,
				URL:            l.URL,
				DirectAssetURL: l.DirectAssetURL,
				External:       l.External,
				LinkType:       string(l.LinkType),
			}
		}
		out.AssetsLinks = links
	}
	if len(r.Evidences) > 0 {
		evidences := make([]EvidenceOutput, len(r.Evidences))
		for i, e := range r.Evidences {
			ev := EvidenceOutput{SHA: e.SHA, Filepath: e.Filepath}
			if e.CollectedAt != nil {
				ev.CollectedAt = e.CollectedAt.Format(time.RFC3339)
			}
			evidences[i] = ev
		}
		out.Evidences = evidences
	}
	return out
}

// Create creates a new release in a GitLab project for the specified tag.
// Returns the created release details or an error if the tag is not found.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("releaseCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.CreateReleaseOptions{
		TagName: new(input.TagName),
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Description != "" {
		opts.Description = new(toolutil.NormalizeText(input.Description))
	}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	if len(input.Milestones) > 0 {
		opts.Milestones = &input.Milestones
	}
	if input.TagMessage != "" {
		opts.TagMessage = new(input.TagMessage)
	}
	r, _, err := client.GL().Releases.CreateRelease(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusConflict):
			return Output{}, toolutil.WrapErrWithHint("releaseCreate", err, "a release for this tag may already exist — use gitlab_release_update to modify it, or choose a different tag_name")
		case toolutil.IsHTTPStatus(err, http.StatusForbidden):
			return Output{}, toolutil.WrapErrWithHint("releaseCreate", err, "creating releases requires Developer role or higher")
		default:
			return Output{}, toolutil.WrapErrWithMessage("releaseCreate", err)
		}
	}
	return ToOutput(r), nil
}

// Update modifies an existing release identified by project and tag name.
// Only non-empty fields in the input are applied as updates.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("releaseUpdate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.UpdateReleaseOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Description != "" {
		opts.Description = new(toolutil.NormalizeText(input.Description))
	}
	if input.ReleasedAt != "" {
		t, err := time.Parse(time.RFC3339, input.ReleasedAt)
		if err != nil {
			return Output{}, fmt.Errorf("releaseUpdate: invalid released_at format (expected ISO 8601/RFC 3339): %w", err)
		}
		opts.ReleasedAt = &t
	}
	if len(input.Milestones) > 0 {
		opts.Milestones = &input.Milestones
	}
	r, _, err := client.GL().Releases.UpdateRelease(string(input.ProjectID), input.TagName, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("releaseUpdate", err, http.StatusNotFound,
			"verify tag_name with gitlab_release_list; updating releases requires Developer role or higher")
	}
	return ToOutput(r), nil
}

// Delete removes a release from a GitLab project by tag name.
// Returns the deleted release details or an error if the release does not exist.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("releaseDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	r, _, err := client.GL().Releases.DeleteRelease(string(input.ProjectID), input.TagName, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("releaseDelete", err, http.StatusForbidden,
			"deleting releases requires Maintainer role or higher; verify tag_name with gitlab_release_list")
	}
	return ToOutput(r), nil
}

// Get retrieves a single release from a GitLab project by tag name.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("releaseGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	r, _, err := client.GL().Releases.GetRelease(string(input.ProjectID), input.TagName, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("releaseGet", err, http.StatusNotFound,
			"verify tag_name with gitlab_release_list; tag_name is case-sensitive")
	}
	return ToOutput(r), nil
}

// GetLatest retrieves the latest release for a GitLab project.
func GetLatest(ctx context.Context, client *gitlabclient.Client, input GetLatestInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("releaseGetLatest: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	r, _, err := client.GL().Releases.GetLatestRelease(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("releaseGetLatest", err, http.StatusNotFound,
			"the project has no releases; create one with gitlab_release_create")
	}
	return ToOutput(r), nil
}

// List returns a paginated list of releases for a GitLab project.
// Results can be ordered by released_at or created_at and sorted ascending or descending.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("releaseList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.ListReleasesOptions{}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	releases, resp, err := client.GL().Releases.ListReleases(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("releaseList", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; the project may have no releases yet")
	}
	out := make([]Output, len(releases))
	for i, r := range releases {
		out[i] = ToOutput(r)
	}
	return ListOutput{Releases: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}
