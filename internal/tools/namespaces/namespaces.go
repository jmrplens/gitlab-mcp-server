package namespaces

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Input types.

// ListInput contains parameters for listing namespaces.
type ListInput struct {
	Search       string `json:"search,omitempty" jsonschema:"Filter namespaces by search term"`
	OwnedOnly    bool   `json:"owned_only,omitempty" jsonschema:"If true return only namespaces owned by the authenticated user"`
	TopLevelOnly bool   `json:"top_level_only,omitempty" jsonschema:"If true return only top-level namespaces"`
	OrderBy      string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id)"`
	Sort         string `json:"sort,omitempty" jsonschema:"Sort direction for keyset-paginated results (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	// ProjectsCount and RootRepositorySize are sent to an administrator asking
	// about a group.
	ProjectsCount      int64 `json:"projects_count,omitempty"`
	RootRepositorySize int64 `json:"root_repository_size,omitempty"`
	// The compute-minute and purchased-storage fields are sent to a caller
	// allowed to change the namespace's limits.
	SharedRunnersMinutesLimit        *int64 `json:"shared_runners_minutes_limit,omitempty"`
	ExtraSharedRunnersMinutesLimit   *int64 `json:"extra_shared_runners_minutes_limit,omitempty"`
	AdditionalPurchasedStorageSize   *int64 `json:"additional_purchased_storage_size,omitempty"`
	AdditionalPurchasedStorageEndsOn string `json:"additional_purchased_storage_ends_on,omitempty"`
	BillableMembersCount             int64  `json:"billable_members_count,omitempty" tier:"premium"`
	Plan                             string `json:"plan,omitempty" tier:"premium"`
	TrialEndsOn                      string `json:"trial_ends_on,omitempty" tier:"premium"`
	Trial                            bool   `json:"trial,omitempty" tier:"premium"`
	MaxSeatsUsed                     *int64 `json:"max_seats_used,omitempty"`
	SeatsInUse                       *int64 `json:"seats_in_use,omitempty"`
	// MaxSeatsUsedChangedAt and EndDate are sent for a namespace that has a
	// subscription.
	MaxSeatsUsedChangedAt string `json:"max_seats_used_changed_at,omitempty"`
	EndDate               string `json:"end_date,omitempty"`
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
	opts := &gl.ListNamespacesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	nss, resp, err := client.GL().Namespaces.ListNamespaces(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("namespace_list", err, http.StatusForbidden,
			"requires authentication; only namespaces visible to the token are returned; use search to filter, owned_only=true for namespaces you own, top_level_only=true for top-level groups")
	}
	extras, err := toolutil.CapturedNamespaces(captured, len(nss))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("namespace_list", err)
	}

	out := ListOutput{
		Namespaces: make([]Output, 0, len(nss)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, ns := range nss {
		out.Namespaces = append(out.Namespaces, toOutput(ns, extras[i]))
	}
	return out, nil
}

// Get retrieves a single namespace by ID or path.
// Uses a raw HTTP request to work around upstream client-go issue where
// GetNamespace expects a single JSON object but some GitLab versions
// return an array for path-based lookups. Tracked, unreported so far, in
// docs/development/upstream-bugs.md.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	ns, _, err := client.GL().Namespaces.GetNamespace(input.ID, gl.WithContext(ctx))
	if err != nil {
		if !strings.Contains(err.Error(), "cannot unmarshal array") {
			return Output{}, toolutil.WrapErrWithStatusHint("namespace_get", err, http.StatusNotFound,
				"verify id (numeric) or path (URL-encoded full path) with gitlab_namespace_list or gitlab_namespace_search")
		}
		return getFromArray(ctx, client, input.ID)
	}
	extra, err := toolutil.CapturedNamespace(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("namespace_get", err)
	}
	return toOutput(ns, extra), nil
}

// getFromArray asks for the namespace again and reads the answer as the array
// some GitLab versions send for a path lookup, taking the first entry.
func getFromArray(ctx context.Context, client *gitlabclient.Client, id string) (Output, error) {
	req, reqErr := client.GL().NewRequest("GET", "namespaces/"+gl.PathEscape(id), nil, nil)
	if reqErr != nil {
		return Output{}, toolutil.WrapErrWithMessage("namespace_get", reqErr)
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	req = req.WithContext(ctx)

	var nsList []*gl.Namespace
	if _, doErr := client.GL().Do(req, &nsList); doErr != nil {
		return Output{}, toolutil.WrapErrWithMessage("namespace_get", doErr)
	}
	if len(nsList) == 0 {
		return Output{}, toolutil.WrapErrWithMessage("namespace_get", fmt.Errorf("namespace %q not found", id))
	}
	extras, listErr := toolutil.CapturedNamespaces(captured, len(nsList))
	if listErr != nil {
		return Output{}, toolutil.WrapErr("namespace_get", listErr)
	}
	return toOutput(nsList[0], extras[0]), nil
}

// Exists checks whether a namespace path is available.
func Exists(ctx context.Context, client *gitlabclient.Client, input ExistsInput) (ExistsOutput, error) {
	opts := &gl.NamespaceExistsOptions{}
	if input.ParentID > 0 {
		opts.ParentID = new(input.ParentID)
	}

	result, _, err := client.GL().Namespaces.NamespaceExists(input.ID, opts, gl.WithContext(ctx))
	if err != nil {
		return ExistsOutput{}, toolutil.WrapErrWithStatusHint("namespace_exists", err, http.StatusBadRequest,
			"id must be a valid namespace path; parent_id is optional and must reference an existing namespace; suggests field provides alternatives if path is taken")
	}
	return ExistsOutput{
		Exists:   result.Exists,
		Suggests: result.Suggests,
	}, nil
}

// Search searches namespaces by query string.
func Search(ctx context.Context, client *gitlabclient.Client, input SearchInput) (ListOutput, error) {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	nss, resp, err := client.GL().Namespaces.SearchNamespace(input.Query, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("namespace_search", err, http.StatusForbidden,
			"query is required; only namespaces visible to the authenticated user are returned")
	}
	extras, err := toolutil.CapturedNamespaces(captured, len(nss))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("namespace_search", err)
	}

	out := ListOutput{
		Namespaces: make([]Output, 0, len(nss)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, ns := range nss {
		out.Namespaces = append(out.Namespaces, toOutput(ns, extras[i]))
	}
	return out, nil
}

// Converters.

// toOutput converts the GitLab API response to the tool output format,
// filling from the decoded namespace and from what the capture read beside it.
func toOutput(ns *gl.Namespace, extra toolutil.NamespaceExtra) Output {
	o := Output{
		ID:                               ns.ID,
		Name:                             ns.Name,
		Path:                             ns.Path,
		Kind:                             ns.Kind,
		FullPath:                         ns.FullPath,
		ParentID:                         ns.ParentID,
		WebURL:                           ns.WebURL,
		MembersCountWithDescendants:      ns.MembersCountWithDescendants,
		ProjectsCount:                    extra.ProjectsCount,
		RootRepositorySize:               extra.RootRepositorySize,
		SharedRunnersMinutesLimit:        extra.SharedRunnersMinutesLimit,
		ExtraSharedRunnersMinutesLimit:   extra.ExtraSharedRunnersMinutesLimit,
		AdditionalPurchasedStorageSize:   extra.AdditionalPurchasedStorageSize,
		AdditionalPurchasedStorageEndsOn: extra.AdditionalPurchasedStorageEndsOn,
		BillableMembersCount:             ns.BillableMembersCount,
		Plan:                             ns.Plan,
		Trial:                            ns.Trial,
		MaxSeatsUsed:                     ns.MaxSeatsUsed,
		SeatsInUse:                       ns.SeatsInUse,
		MaxSeatsUsedChangedAt:            toolutil.FormatTimePtr(extra.MaxSeatsUsedChangedAt),
		EndDate:                          extra.EndDate,
	}
	if ns.AvatarURL != nil {
		o.AvatarURL = *ns.AvatarURL
	}
	if ns.TrialEndsOn != nil {
		o.TrialEndsOn = time.Time(*ns.TrialEndsOn).Format("2006-01-02")
	}
	return o
}

// Formatters.
