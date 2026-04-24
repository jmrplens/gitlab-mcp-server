// Package groupreleases implements MCP tool handlers for listing
// releases across all projects in a GitLab group.
package groupreleases

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Output represents a release of a project within a group.
type Output struct {
	TagName         string `json:"tag_name"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	CreatedAt       string `json:"created_at"`
	ReleasedAt      string `json:"released_at,omitempty"`
	Author          string `json:"author,omitempty"`
	UpcomingRelease bool   `json:"upcoming_release,omitempty"`
	WebURL          string `json:"web_url,omitempty"`
}

// ListOutput holds a paginated list of group releases.
type ListOutput struct {
	toolutil.HintableOutput
	Releases   []Output                  `json:"releases"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

func toOutput(r *gl.Release) Output {
	o := Output{
		TagName:         r.TagName,
		Name:            r.Name,
		Description:     r.Description,
		UpcomingRelease: r.UpcomingRelease,
	}
	if r.CreatedAt != nil {
		o.CreatedAt = r.CreatedAt.String()
	}
	if r.ReleasedAt != nil {
		o.ReleasedAt = r.ReleasedAt.String()
	}
	if r.Author.Username != "" {
		o.Author = r.Author.Username
	}
	if r.Links.Self != "" {
		o.WebURL = r.Links.Self
	}
	return o
}

// ListInput defines parameters for the List action which retrieves all releases across projects in a group.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id"             jsonschema:"Group ID or URL-encoded path,required"`
	Simple  bool                 `json:"simple,omitempty"     jsonschema:"Return only limited fields"`
	toolutil.PaginationInput
}

// List retrieves all releases across projects in a group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListGroupReleasesOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	if input.Simple {
		opts.Simple = new(true)
	}
	releases, resp, err := client.GL().GroupReleases.ListGroupReleases(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("listGroupReleases", err)
	}
	out := make([]Output, len(releases))
	for i, r := range releases {
		out[i] = toOutput(r)
	}
	return ListOutput{
		Releases:   out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}
