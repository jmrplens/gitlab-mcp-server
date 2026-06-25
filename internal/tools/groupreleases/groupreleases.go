package groupreleases

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Output represents a release of a project within a group. Nested sub-objects
// (author, commit, assets, _links, milestones, evidences) mirror the
// corresponding client-go types field-for-field per the 1:1 audit policy. The
// group releases endpoint returns the same gl.Release payload as project
// releases. See shapes.go.
type Output struct {
	TagName         string             `json:"tag_name"`
	Name            string             `json:"name"`
	Description     string             `json:"description,omitempty"`
	DescriptionHTML string             `json:"description_html,omitempty"`
	CreatedAt       string             `json:"created_at"`
	ReleasedAt      string             `json:"released_at,omitempty"`
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
		o.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	if r.ReleasedAt != nil {
		o.ReleasedAt = r.ReleasedAt.Format(time.RFC3339)
	}
	return o
}

// ListInput defines parameters for the List action which retrieves all releases
// across projects in a group. It mirrors gl.ListGroupReleasesOptions (the
// embedded gl.ListOptions plus the simple flag) and exposes ordering and keyset
// pagination supported by the underlying gl.ListOptions.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id"           jsonschema:"Group ID or URL-encoded path,required"`
	Simple  bool                 `json:"simple,omitempty"   jsonschema:"Return only limited fields for each release"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Order releases by a column (e.g. released_at, created_at)"`
	Sort    string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// List retrieves all releases across projects in a group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListGroupReleasesOptions{}
	if input.Simple {
		opts.Simple = new(true)
	}
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	releases, resp, err := client.GL().GroupReleases.ListGroupReleases(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("listGroupReleases", err, http.StatusNotFound, "verify group_id with gitlab_group_get")
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
