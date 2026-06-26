package releases

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// CreateInput defines parameters for creating a GitLab release.
type CreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	TagName     string               `json:"tag_name"              jsonschema:"Tag name associated with the release,required"`
	Name        string               `json:"name,omitempty"        jsonschema:"Release title"`
	Description string               `json:"description,omitempty" jsonschema:"Release notes (Markdown supported)"`
	ReleasedAt  string               `json:"released_at,omitempty" jsonschema:"Date of the release in ISO 8601 format"`
	Ref         string               `json:"ref,omitempty"         jsonschema:"Branch or commit SHA to create tag from when tag_name does not exist; include this when the prompt says ref/from ref"`
	Milestones  []string             `json:"milestones,omitempty"  jsonschema:"Milestone titles to associate with the release"`
	TagMessage  string               `json:"tag_message,omitempty" jsonschema:"Message to use for the annotated tag (creates annotated tag instead of lightweight)"`
	Assets      *AssetsInput         `json:"assets,omitempty"      jsonschema:"Asset links to attach to the release at creation time"`
}

// AssetsInput mirrors gl.ReleaseAssetsOptions: the assets attached when
// creating a release.
type AssetsInput struct {
	Links []AssetLinkInput `json:"links,omitempty" jsonschema:"Asset links to attach to the release"`
}

// AssetLinkInput mirrors gl.ReleaseAssetLinkOptions: a single release asset
// link supplied at creation time.
type AssetLinkInput struct {
	Name            string `json:"name,omitempty"              jsonschema:"Name of the asset link as shown in the release"`
	URL             string `json:"url,omitempty"               jsonschema:"URL of the asset"`
	FilePath        string `json:"filepath,omitempty"          jsonschema:"Deprecated relative path; prefer direct_asset_path"`
	DirectAssetPath string `json:"direct_asset_path,omitempty" jsonschema:"Relative path to direct asset link (e.g. /binaries/app.zip)"`
	LinkType        string `json:"link_type,omitempty"         jsonschema:"Asset link type: other (default), runbook, image, or package"`
}

// Output represents a GitLab release. Nested sub-objects (author, commit,
// assets, _links, milestones, evidences) mirror the corresponding client-go
// types field-for-field per the 1:1 audit policy. See shapes.go.
type Output struct {
	toolutil.HintableOutput
	TagName         string             `json:"tag_name"`
	Name            string             `json:"name"`
	Description     string             `json:"description"`
	DescriptionHTML string             `json:"description_html,omitempty"`
	CreatedAt       string             `json:"created_at"`
	ReleasedAt      string             `json:"released_at"`
	Author          *AuthorOutput      `json:"author,omitempty"`
	Commit          *CommitOutput      `json:"commit,omitempty"`
	UpcomingRelease bool               `json:"upcoming_release,omitempty"`
	Milestones      []*MilestoneOutput `json:"milestones,omitempty"`
	CommitPath      string             `json:"commit_path,omitempty"`
	TagPath         string             `json:"tag_path,omitempty"`
	Assets          *AssetsOutput      `json:"assets,omitempty"`
	Evidences       []*EvidenceOutput  `json:"evidences,omitempty"`
	Links           *LinksOutput       `json:"_links,omitempty"`
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
	ProjectID              toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy                string               `json:"order_by,omitempty" jsonschema:"Order by field (released_at, created_at)"`
	Sort                   string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	IncludeHTMLDescription bool                 `json:"include_html_description,omitempty" jsonschema:"Include the description_html field rendered from the Markdown description"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListOutput holds a list of releases.
type ListOutput struct {
	toolutil.HintableOutput
	Releases   []Output                  `json:"releases"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ToOutput converts a GitLab API [gl.Release] to the MCP tool output
// format, formatting timestamps as RFC 3339 strings. Nested objects (author,
// commit, assets, _links, milestones, evidences) are mirrored via the
// converters in shapes.go.
func ToOutput(r *gl.Release) Output {
	out := Output{
		TagName:         r.TagName,
		Name:            r.Name,
		Description:     r.Description,
		DescriptionHTML: r.DescriptionHTML,
		Author:          authorOutput(r.Author),
		Commit:          commitOutput(r.Commit),
		UpcomingRelease: r.UpcomingRelease,
		CommitPath:      r.CommitPath,
		TagPath:         r.TagPath,
		Assets:          assetsOutput(r.Assets),
		Milestones:      milestoneOutputs(r.Milestones),
		Evidences:       evidenceOutputs(r.Evidences),
		Links:           linksOutput(r.Links),
	}
	if r.CreatedAt != nil {
		out.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	if r.ReleasedAt != nil {
		out.ReleasedAt = r.ReleasedAt.Format(time.RFC3339)
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
	opts.Assets = assetsOptions(input.Assets)
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
	if input.IncludeHTMLDescription {
		opts.IncludeHTMLDescription = new(true)
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
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
