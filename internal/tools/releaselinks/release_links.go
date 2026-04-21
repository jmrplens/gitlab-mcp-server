// Package releaselinks implements MCP tool handlers for GitLab release asset
// link operations including create, create batch, delete, get, and list.
// It wraps the ReleaseLinks service from client-go v2.
package releaselinks

import (
	"context"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CreateInput defines parameters for adding a release asset link.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Tag name of the release,required"`
	Name      string               `json:"name"       jsonschema:"Name of the link,required"`
	URL       string               `json:"url"        jsonschema:"URL of the link target. For packages use the real url returned by gitlab_package_publish — never construct URLs manually,required"`
	LinkType  string               `json:"link_type,omitempty" jsonschema:"Type of the link (runbook, package, image, other)"`
}

// Output represents a release asset link.
type Output struct {
	toolutil.HintableOutput
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	LinkType       string `json:"link_type"`
	External       bool   `json:"external"`
	DirectAssetURL string `json:"direct_asset_url,omitempty"`
}

// DeleteInput defines parameters for deleting a release link.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Tag name of the release,required"`
	LinkID    int64                `json:"link_id"    jsonschema:"ID of the release link to delete,required"`
}

// GetInput defines parameters for retrieving a single release link.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Tag name of the release,required"`
	LinkID    int64                `json:"link_id"    jsonschema:"ID of the release link,required"`
}

// UpdateInput defines parameters for updating a release link.
type UpdateInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	TagName         string               `json:"tag_name"                  jsonschema:"Tag name of the release,required"`
	LinkID          int64                `json:"link_id"                   jsonschema:"ID of the release link to update,required"`
	Name            string               `json:"name,omitempty"            jsonschema:"New name of the link"`
	URL             string               `json:"url,omitempty"             jsonschema:"New URL of the link"`
	FilePath        string               `json:"filepath,omitempty"        jsonschema:"New filepath for a direct asset link"`
	DirectAssetPath string               `json:"direct_asset_path,omitempty" jsonschema:"New direct asset path for the link"`
	LinkType        string               `json:"link_type,omitempty"       jsonschema:"New link type (runbook, package, image, other)"`
}

// ListInput defines parameters for listing release links.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name" jsonschema:"Tag name of the release,required"`
	toolutil.PaginationInput
}

// ListOutput holds a list of release asset links.
type ListOutput struct {
	toolutil.HintableOutput
	Links      []Output                  `json:"links"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// LinkEntry describes a single link to create in a batch operation.
type LinkEntry struct {
	Name     string `json:"name"                jsonschema:"Name of the link,required"`
	URL      string `json:"url"                 jsonschema:"URL of the link target,required"`
	LinkType string `json:"link_type,omitempty"  jsonschema:"Type of the link (runbook, package, image, other)"`
}

// CreateBatchInput defines parameters for creating multiple release asset
// links in a single tool call.
type CreateBatchInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Tag name of the release,required"`
	Links     []LinkEntry          `json:"links"      jsonschema:"Array of links to create (each with name and url required),required"`
}

// CreateBatchOutput holds the results of a batch link creation.
type CreateBatchOutput struct {
	toolutil.HintableOutput
	Created []Output `json:"created"`
	Failed  []string `json:"failed,omitempty"`
}

// ToOutput converts a GitLab API [gl.ReleaseLink] to the MCP tool
// output format.
func ToOutput(l *gl.ReleaseLink) Output {
	return Output{
		ID:             l.ID,
		Name:           l.Name,
		URL:            l.URL,
		LinkType:       string(l.LinkType),
		External:       l.External,
		DirectAssetURL: l.DirectAssetURL,
	}
}

// Create adds a new asset link to a release identified by project
// and tag name. The link type defaults to "other" if not specified.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("Create: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.CreateReleaseLinkOptions{
		Name: new(input.Name),
		URL:  new(input.URL),
	}
	if input.LinkType != "" {
		opts.LinkType = new(gl.LinkTypeValue(input.LinkType))
	}
	l, _, err := client.GL().ReleaseLinks.CreateReleaseLink(string(input.ProjectID), input.TagName, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("Create", err)
	}
	return ToOutput(l), nil
}

// CreateBatch adds multiple asset links to a release in a single tool call.
// Each link is created sequentially; failures are collected without aborting
// the remaining links.
func CreateBatch(ctx context.Context, client *gitlabclient.Client, input CreateBatchInput) (CreateBatchOutput, error) {
	if err := ctx.Err(); err != nil {
		return CreateBatchOutput{}, err
	}
	if input.ProjectID == "" {
		return CreateBatchOutput{}, errors.New("CreateBatch: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.TagName == "" {
		return CreateBatchOutput{}, errors.New("CreateBatch: tag_name is required")
	}
	if len(input.Links) == 0 {
		return CreateBatchOutput{}, errors.New("CreateBatch: links array is required and must not be empty")
	}

	out := CreateBatchOutput{Created: make([]Output, 0, len(input.Links))}
	for i, entry := range input.Links {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if entry.Name == "" || entry.URL == "" {
			out.Failed = append(out.Failed, fmt.Sprintf("link[%d]: name and url are required", i))
			continue
		}
		opts := &gl.CreateReleaseLinkOptions{
			Name: new(entry.Name),
			URL:  new(entry.URL),
		}
		if entry.LinkType != "" {
			opts.LinkType = new(gl.LinkTypeValue(entry.LinkType))
		}
		l, _, err := client.GL().ReleaseLinks.CreateReleaseLink(string(input.ProjectID), input.TagName, opts, gl.WithContext(ctx))
		if err != nil {
			out.Failed = append(out.Failed, fmt.Sprintf("link[%d] %q: %v", i, entry.Name, err))
			continue
		}
		out.Created = append(out.Created, ToOutput(l))
	}
	return out, nil
}

// Delete removes an asset link from a release by its link ID.
// Returns the deleted link details or an error if the link does not exist.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("Delete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.LinkID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("Delete", "link_id")
	}
	l, _, err := client.GL().ReleaseLinks.DeleteReleaseLink(string(input.ProjectID), input.TagName, input.LinkID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("Delete", err)
	}
	return ToOutput(l), nil
}

// Get retrieves a single release asset link by its ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("Get: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.LinkID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("Get", "link_id")
	}
	l, _, err := client.GL().ReleaseLinks.GetReleaseLink(string(input.ProjectID), input.TagName, input.LinkID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("Get", err)
	}
	return ToOutput(l), nil
}

// Update modifies an existing release asset link. Only non-empty fields are applied.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("Update: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.LinkID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("Update", "link_id")
	}
	opts := &gl.UpdateReleaseLinkOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.URL != "" {
		opts.URL = new(input.URL)
	}
	if input.FilePath != "" {
		opts.FilePath = new(input.FilePath)
	}
	if input.DirectAssetPath != "" {
		opts.DirectAssetPath = new(input.DirectAssetPath)
	}
	if input.LinkType != "" {
		opts.LinkType = new(gl.LinkTypeValue(input.LinkType))
	}
	l, _, err := client.GL().ReleaseLinks.UpdateReleaseLink(string(input.ProjectID), input.TagName, input.LinkID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("Update", err)
	}
	return ToOutput(l), nil
}

// List returns a paginated list of asset links for a release
// identified by project and tag name.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("List: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.ListReleaseLinksOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	links, resp, err := client.GL().ReleaseLinks.ListReleaseLinks(string(input.ProjectID), input.TagName, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("List", err)
	}
	out := make([]Output, len(links))
	for i, l := range links {
		out[i] = ToOutput(l)
	}
	return ListOutput{Links: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}
