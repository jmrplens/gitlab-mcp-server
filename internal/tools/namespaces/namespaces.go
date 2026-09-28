package namespaces

import (
	"context"
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

	// CIMinutesUsage is sent for a top-level namespace to its owner or an
	// administrator, on an enterprise build from GitLab 19.4.
	CIMinutesUsage *toolutil.CIMinutesUsageOutput `json:"ci_minutes_usage,omitempty"`
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
//
// A blank id is refused before any request. The schema's required does not
// refuse an empty string, and GitLab's router folds the path namespaces/ onto
// the listing route, so an empty id would be answered with the caller's list
// of namespaces rather than with the one namespace this action promises.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if strings.TrimSpace(input.ID) == "" {
		return Output{}, toolutil.ErrFieldRequired("id")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	ns, _, err := client.GL().Namespaces.GetNamespace(input.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("namespace_get", err, http.StatusNotFound,
			"verify id (numeric) or path (URL-encoded full path) with user.namespace_list or user.namespace_search")
	}
	extra, err := toolutil.CapturedNamespace(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("namespace_get", err)
	}
	return toOutput(ns, extra), nil
}

// Exists checks whether a namespace path is available.
//
// A blank id is refused before any request, as Get refuses one. An empty id
// leaves an empty segment where the path belongs in namespaces/:id/exists,
// and a path of whitespace is one no namespace can take, so GitLab's answer
// for it, that nothing holds the path, would read as the path being free.
func Exists(ctx context.Context, client *gitlabclient.Client, input ExistsInput) (ExistsOutput, error) {
	if strings.TrimSpace(input.ID) == "" {
		return ExistsOutput{}, toolutil.ErrFieldRequired("id")
	}
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
//
// A blank query is refused before any request. client-go leaves an empty
// search parameter off the request, and GitLab filters by one only when it
// is present, so either would be answered with every namespace the caller can
// see, presented as the ones matching a search.
func Search(ctx context.Context, client *gitlabclient.Client, input SearchInput) (ListOutput, error) {
	if strings.TrimSpace(input.Query) == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("query")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	nss, resp, err := client.GL().Namespaces.SearchNamespace(input.Query, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("namespace_search", err, http.StatusForbidden,
			"the credential was refused the namespace listing: a fine-grained personal access token needs the read_namespace permission, and the search only returns namespaces the authenticated user can see")
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
// filling from the decoded namespace and, for the three limit fields and the
// compute-minute usage, from what the capture read beside it: client-go models
// the limits as plain int64, so the null that means "no limit" would reach a
// caller as a limit of zero, and it does not model the usage at all.
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
		ProjectsCount:                    ns.ProjectsCount,
		RootRepositorySize:               ns.RootRepositorySize,
		SharedRunnersMinutesLimit:        extra.SharedRunnersMinutesLimit,
		ExtraSharedRunnersMinutesLimit:   extra.ExtraSharedRunnersMinutesLimit,
		AdditionalPurchasedStorageSize:   extra.AdditionalPurchasedStorageSize,
		AdditionalPurchasedStorageEndsOn: isoDate(ns.AdditionalPurchasedStorageEndsOn),
		CIMinutesUsage:                   extra.CIMinutesUsage,
		BillableMembersCount:             ns.BillableMembersCount,
		Plan:                             ns.Plan,
		Trial:                            ns.Trial,
		MaxSeatsUsed:                     ns.MaxSeatsUsed,
		SeatsInUse:                       ns.SeatsInUse,
		MaxSeatsUsedChangedAt:            toolutil.FormatTimePtr(ns.MaxSeatsUsedChangedAt),
		EndDate:                          isoDate(ns.EndDate),
	}
	if ns.AvatarURL != nil {
		o.AvatarURL = *ns.AvatarURL
	}
	o.TrialEndsOn = isoDate(ns.TrialEndsOn)
	return o
}

// isoDate renders a date-only GitLab field, empty when the instance sent none.
func isoDate(d *gl.ISOTime) string {
	if d == nil {
		return ""
	}
	return time.Time(*d).Format("2006-01-02")
}

// Formatters.
