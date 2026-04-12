// Package namespaces implements MCP tools for GitLab namespace operations
// including listing, getting, checking existence, and searching namespaces.
package namespaces

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Input types.

// ListInput contains parameters for listing namespaces.
type ListInput struct {
	Search       string `json:"search,omitempty" jsonschema:"Filter namespaces by search term"`
	OwnedOnly    bool   `json:"owned_only,omitempty" jsonschema:"If true return only namespaces owned by the authenticated user"`
	TopLevelOnly bool   `json:"top_level_only,omitempty" jsonschema:"If true return only top-level namespaces"`
	Page         int64  `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage      int64  `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// GetInput contains parameters for getting a namespace by ID.
type GetInput struct {
	ID string `json:"id" jsonschema:"Namespace ID or path,required"`
}

// ExistsInput contains parameters for checking namespace existence.
type ExistsInput struct {
	ID       string `json:"id" jsonschema:"Namespace path to check for existence,required"`
	ParentID int64  `json:"parent_id,omitempty" jsonschema:"Parent namespace ID to scope the check"`
}

// SearchInput contains parameters for searching namespaces.
type SearchInput struct {
	Query string `json:"query" jsonschema:"Search query string for namespaces,required"`
}

// Output types.

// Output represents a single namespace.
type Output struct {
	toolutil.HintableOutput
	ID                          int64  `json:"id"`
	Name                        string `json:"name"`
	Path                        string `json:"path"`
	Kind                        string `json:"kind"`
	FullPath                    string `json:"full_path"`
	ParentID                    int64  `json:"parent_id,omitempty"`
	AvatarURL                   string `json:"avatar_url,omitempty"`
	WebURL                      string `json:"web_url,omitempty"`
	MembersCountWithDescendants int64  `json:"members_count_with_descendants,omitempty"`
	BillableMembersCount        int64  `json:"billable_members_count,omitempty"`
	Plan                        string `json:"plan,omitempty"`
	Trial                       bool   `json:"trial,omitempty"`
}

// ListOutput represents a paginated list of namespaces.
type ListOutput struct {
	toolutil.HintableOutput
	Namespaces []Output                  `json:"namespaces"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ExistsOutput represents the result of a namespace existence check.
type ExistsOutput struct {
	toolutil.HintableOutput
	Exists   bool     `json:"exists"`
	Suggests []string `json:"suggests,omitempty"`
}

// Handlers.

// List returns a paginated list of namespaces visible to the user.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListNamespacesOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.OwnedOnly {
		opts.OwnedOnly = new(true)
	}
	if input.TopLevelOnly {
		opts.TopLevelOnly = new(true)
	}

	nss, resp, err := client.GL().Namespaces.ListNamespaces(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("namespace_list", err)
	}

	out := ListOutput{
		Namespaces: make([]Output, 0, len(nss)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, ns := range nss {
		out.Namespaces = append(out.Namespaces, toOutput(ns))
	}
	return out, nil
}

// Get retrieves a single namespace by ID or path.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	ns, _, err := client.GL().Namespaces.GetNamespace(input.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("namespace_get", err)
	}
	return toOutput(ns), nil
}

// Exists checks whether a namespace path is available.
func Exists(ctx context.Context, client *gitlabclient.Client, input ExistsInput) (ExistsOutput, error) {
	opts := &gl.NamespaceExistsOptions{}
	if input.ParentID > 0 {
		opts.ParentID = new(input.ParentID)
	}

	result, _, err := client.GL().Namespaces.NamespaceExists(input.ID, opts, gl.WithContext(ctx))
	if err != nil {
		return ExistsOutput{}, toolutil.WrapErrWithMessage("namespace_exists", err)
	}
	return ExistsOutput{
		Exists:   result.Exists,
		Suggests: result.Suggests,
	}, nil
}

// Search searches namespaces by query string.
func Search(ctx context.Context, client *gitlabclient.Client, input SearchInput) (ListOutput, error) {
	nss, resp, err := client.GL().Namespaces.SearchNamespace(input.Query, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("namespace_search", err)
	}

	out := ListOutput{
		Namespaces: make([]Output, 0, len(nss)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, ns := range nss {
		out.Namespaces = append(out.Namespaces, toOutput(ns))
	}
	return out, nil
}

// Converters.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(ns *gl.Namespace) Output {
	o := Output{
		ID:                          ns.ID,
		Name:                        ns.Name,
		Path:                        ns.Path,
		Kind:                        ns.Kind,
		FullPath:                    ns.FullPath,
		ParentID:                    ns.ParentID,
		WebURL:                      ns.WebURL,
		MembersCountWithDescendants: ns.MembersCountWithDescendants,
		BillableMembersCount:        ns.BillableMembersCount,
		Plan:                        ns.Plan,
		Trial:                       ns.Trial,
	}
	if ns.AvatarURL != nil {
		o.AvatarURL = *ns.AvatarURL
	}
	return o
}

// Formatters.
