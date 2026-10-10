package workitemsavedviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// emptyDisplaySettings is what an omitted display_settings is sent as. The
// GraphQL server requires the field on create, and the SDK rejects an empty
// value before dispatch, so a caller with nothing to store would otherwise have
// to learn that `{}` is the way to say so.
var emptyDisplaySettings = json.RawMessage(`{}`)

// notFoundHint is appended to a lookup failure. Saved views are addressed by
// their numeric database ID, not by an IID, and confusing the two is the most
// likely reason a view that exists cannot be found.
//
// The action it sends the reader to is the canonical ID the catalog
// registers: the hint used to say "work_item_saved_view.list", which no
// surface resolves, because these actions are routes on the issue domain and
// carry that prefix. It is written out in one literal rather than joined to
// actionList, since a constant expression has no statement a test can cover
// and the mutation gate reports its operators as not covered; a test holds
// the literal to actionList instead.
const notFoundHint = "verify saved_view_id is the view's numeric ID (from issue.work_item_saved_view_list) and that namespace_path is the full group or project path"

// objectSavedView is what the four writes that answer with a saved view
// return, in a reader's words, for the answer a fine-grained session gets
// when GitLab ran the write and answered without it (issue 1103): at 19.4.1
// WorkItemSavedViewType declares no fine-grained permission, and GitLab
// checks the payload's view only after the write ran.
const objectSavedView = "saved view"

// Hints for the 500 GitLab 19.4 answers a caller authenticated with a token
// from the step that subscribes a user to a view: it locks the user row while
// the sign-in tracking has left unsaved changes on it, which Rails refuses
// (docs/development/upstream-bugs.md, entry 72). Nothing on the client side
// avoids it, so the answer has to say what state it left behind.
const (
	// createServerErrorHint answers a create's 500. The view is saved before
	// the step that fails, so the create has usually happened, and a caller
	// told only that it failed creates the view a second time.
	// It names the listing action in one literal, as notFoundHint does.
	createServerErrorHint = "GitLab answered a server error, and on GitLab 19.4 that comes after the view is saved, from subscribing its creator: list the namespace's views with issue.work_item_saved_view_list and look for this name before creating it again, since a second create adds a duplicate"
	// subscribeServerErrorHint answers a subscribe's 500, which fails before
	// anything is recorded.
	subscribeServerErrorHint = "GitLab answered a server error before recording the subscription, which on GitLab 19.4 is what every subscribe made with a token meets: nothing changed, and the view can be followed from the web interface instead"
)

// Item is one work item saved view. It mirrors [gl.WorkItemSavedView].
type Item struct {
	ID              int64  `json:"id"                         jsonschema:"Numeric ID of the saved view"`
	GID             string `json:"gid,omitempty"              jsonschema:"GraphQL global ID of the saved view"`
	Name            string `json:"name"                       jsonschema:"Name of the saved view"`
	Description     string `json:"description,omitempty"      jsonschema:"Description of the saved view"`
	IsPrivate       bool   `json:"is_private"                 jsonschema:"Whether the view is private to the user who created it"`
	Subscribed      bool   `json:"subscribed"                 jsonschema:"Whether the authenticated user is subscribed to the view"`
	Filters         any    `json:"filters,omitempty"          jsonschema:"Filters the view applies. Always absent on list results: GitLab resolves this field at most once per GraphQL request, so only get returns it"`
	Sort            string `json:"sort,omitempty"             jsonschema:"Sort order the view applies"`
	DisplaySettings any    `json:"display_settings,omitempty" jsonschema:"Display settings the consuming UI renders the view with"`
}

// toItem converts an SDK saved view into the MCP output shape, decoding the two
// opaque JSON scalars so the model reads structured data instead of a string.
func toItem(v *gl.WorkItemSavedView) Item {
	if v == nil {
		return Item{}
	}
	return Item{
		ID:              v.ID,
		GID:             v.GID(),
		Name:            v.Name,
		Description:     v.Description,
		IsPrivate:       v.IsPrivate,
		Subscribed:      v.Subscribed,
		Filters:         decodeRaw(v.Filters),
		Sort:            v.Sort,
		DisplaySettings: decodeRaw(v.DisplaySettings),
	}
}

// decodeRaw parses an opaque JSON scalar into a generic value. Anything that
// does not parse is returned as its literal text rather than dropped, so a
// server-side shape change degrades to a readable string instead of silence.
func decodeRaw(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	return decoded
}

// GetInput identifies one saved view under a namespace.
type GetInput struct {
	NamespacePath string `json:"namespace_path" jsonschema:"Full path of the group or project the saved view belongs to (e.g. my-group or my-group/my-project),required"`
	SavedViewID   int64  `json:"saved_view_id"  jsonschema:"Numeric ID of the saved view,required"`
}

// GetOutput carries one saved view, including its filters.
type GetOutput struct {
	toolutil.HintableOutput
	NamespacePath string `json:"namespace_path"`
	SavedView     Item   `json:"saved_view"`
}

// ListInput selects a page of the saved views under a namespace.
type ListInput struct {
	NamespacePath string `json:"namespace_path" jsonschema:"Full path of the group or project whose saved views to list,required"`
	toolutil.GraphQLCursorPaginationInput
}

// ListOutput carries a page of saved views plus its cursor metadata.
type ListOutput struct {
	toolutil.HintableOutput
	NamespacePath string                           `json:"namespace_path"`
	SavedViews    []Item                           `json:"saved_views"`
	Pagination    toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// CreateInput holds the fields of a new saved view. It mirrors
// [gl.CreateWorkItemSavedViewOptions].
type CreateInput struct {
	NamespacePath   string         `json:"namespace_path"             jsonschema:"Full path of the group or project to create the saved view under,required"`
	Name            string         `json:"name"                       jsonschema:"Name of the saved view,required"`
	Description     string         `json:"description,omitempty"      jsonschema:"Description of the saved view"`
	IsPrivate       *bool          `json:"is_private,omitempty"       jsonschema:"Keep the view private to the creating user. Defaults to true when omitted"`
	Filters         *Filters       `json:"filters,omitempty"          jsonschema:"Filters the view applies. Omit for a view with no filters"`
	Sort            string         `json:"sort"                       jsonschema:"Sort order the view applies, a WorkItemSort enum value such as CREATED_DESC or TITLE_ASC,required"`
	DisplaySettings map[string]any `json:"display_settings,omitempty" jsonschema:"Display settings for the consuming UI. GitLab validates this object and rejects unknown keys. The accepted ones are camelCase: viewMode (list, board, or table), hiddenMetadataKeys, collapsedGroups, visibleGroups, groupOrder. Omit to store an empty object"`
}

// UpdateInput holds the fields to change on an existing saved view. It mirrors
// [gl.UpdateWorkItemSavedViewOptions]: every field is optional, and an omitted
// one is left as it is.
type UpdateInput struct {
	SavedViewID     int64          `json:"saved_view_id"              jsonschema:"Numeric ID of the saved view to update,required"`
	Name            string         `json:"name,omitempty"             jsonschema:"New name for the saved view"`
	Description     string         `json:"description,omitempty"      jsonschema:"New description for the saved view"`
	IsPrivate       *bool          `json:"is_private,omitempty"       jsonschema:"Whether the view is private to the user who created it"`
	Filters         *Filters       `json:"filters,omitempty"          jsonschema:"Replacement filters. Omit to leave the existing filters unchanged"`
	Sort            string         `json:"sort,omitempty"             jsonschema:"New sort order, a WorkItemSort enum value such as CREATED_DESC or TITLE_ASC"`
	DisplaySettings map[string]any `json:"display_settings,omitempty" jsonschema:"Replacement display settings, with the camelCase keys GitLab accepts: viewMode (list, board, or table), hiddenMetadataKeys, collapsedGroups, visibleGroups, groupOrder. Omit to leave the existing settings unchanged"`
}

// DeleteInput identifies the saved view to delete.
type DeleteInput struct {
	SavedViewID int64 `json:"saved_view_id" jsonschema:"Numeric ID of the saved view to delete,required"`
}

// SubscribeInput identifies the saved view to subscribe the caller to.
type SubscribeInput struct {
	SavedViewID int64 `json:"saved_view_id" jsonschema:"Numeric ID of the saved view to subscribe to,required"`
}

// UnsubscribeInput identifies the saved view to unsubscribe the caller from.
type UnsubscribeInput struct {
	SavedViewID int64 `json:"saved_view_id" jsonschema:"Numeric ID of the saved view to unsubscribe from,required"`
}

// MutateOutput confirms a saved view mutation and returns the resulting view.
type MutateOutput struct {
	toolutil.HintableOutput
	Status    string `json:"status"`
	Message   string `json:"message"`
	SavedView Item   `json:"saved_view"`
}

// Get returns one saved view under a namespace, filters included.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if err := ctx.Err(); err != nil {
		return GetOutput{}, err
	}
	namespacePath, err := requireNamespacePath(input.NamespacePath)
	if err != nil {
		return GetOutput{}, err
	}
	if input.SavedViewID <= 0 {
		return GetOutput{}, toolutil.ErrRequiredInt64("get_work_item_saved_view", "saved_view_id")
	}

	view, _, err := client.GL().WorkItemSavedViews.GetWorkItemSavedView(namespacePath, input.SavedViewID, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, wrapLookupErr("get_work_item_saved_view", err)
	}
	return GetOutput{NamespacePath: namespacePath, SavedView: toItem(view)}, nil
}

// List returns a page of the saved views under a namespace.
//
// Filters are absent from every entry: GitLab resolves that field at most once
// per GraphQL request, so the SDK's list query does not ask for it. Reading a
// view's filters means calling Get on it.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	namespacePath, err := requireNamespacePath(input.NamespacePath)
	if err != nil {
		return ListOutput{}, err
	}

	// The direction is resolved by the shared helper rather than here, so that
	// this domain and the ones querying GraphQL directly answer a backward
	// request the same way. The SDK ignores Last whenever First is set, so a
	// request that named both would silently page forward.
	cursor, err := input.Resolve()
	if err != nil {
		return ListOutput{}, fmt.Errorf("list_work_item_saved_views: %w", err)
	}
	opts := &gl.ListWorkItemSavedViewsOptions{}
	if cursor.After != "" {
		opts.After = new(cursor.After)
	}
	if cursor.Before != "" {
		opts.Before = new(cursor.Before)
	}
	if cursor.First != nil {
		opts.First = new(int64(*cursor.First))
	}
	if cursor.Last != nil {
		opts.Last = new(int64(*cursor.Last))
	}

	views, resp, err := client.GL().WorkItemSavedViews.ListWorkItemSavedViews(namespacePath, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, wrapLookupErr("list_work_item_saved_views", err)
	}

	out := ListOutput{NamespacePath: namespacePath, SavedViews: make([]Item, 0, len(views))}
	for _, view := range views {
		out.SavedViews = append(out.SavedViews, toItem(view))
	}
	out.Pagination = pagination(resp)
	return out, nil
}

// pagination reads the cursor metadata client-go hangs off a list's response.
// client-go sets it on every list it answers, so the guard is for an answer
// that reaches here without it rather than for one that does today, and a
// test drives it directly.
func pagination(resp *gl.Response) toolutil.GraphQLPaginationOutput {
	if resp == nil || resp.PageInfo == nil {
		return toolutil.GraphQLPaginationOutput{}
	}
	return toolutil.GraphQLPaginationOutput{
		HasNextPage:     resp.PageInfo.HasNextPage,
		HasPreviousPage: resp.PageInfo.HasPreviousPage,
		EndCursor:       resp.PageInfo.EndCursor,
		StartCursor:     resp.PageInfo.StartCursor,
	}
}

// Create creates a saved view under a namespace.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (MutateOutput, error) {
	if err := ctx.Err(); err != nil {
		return MutateOutput{}, err
	}
	namespacePath, err := requireNamespacePath(input.NamespacePath)
	if err != nil {
		return MutateOutput{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return MutateOutput{}, toolutil.ErrFieldRequired("name")
	}
	sort := strings.TrimSpace(input.Sort)
	if sort == "" {
		return MutateOutput{}, toolutil.ErrFieldRequired("sort")
	}
	displaySettings, err := encodeDisplaySettings(input.DisplaySettings)
	if err != nil {
		return MutateOutput{}, err
	}
	filters, err := input.Filters.toSDK()
	if err != nil {
		return MutateOutput{}, err
	}

	opts := &gl.CreateWorkItemSavedViewOptions{
		Name:            name,
		Description:     optionalString(strings.TrimSpace(input.Description)),
		IsPrivate:       input.IsPrivate,
		Sort:            sort,
		DisplaySettings: displaySettings,
	}
	if filters != nil {
		opts.Filters = *filters
	}

	view, resp, err := client.GL().WorkItemSavedViews.CreateWorkItemSavedView(namespacePath, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusInternalServerError) {
			return MutateOutput{}, toolutil.WrapErrWithHint("create_work_item_saved_view", err, createServerErrorHint)
		}
		return MutateOutput{}, toolutil.UnconfirmedWrite(client, resp, "create_work_item_saved_view", objectSavedView, err,
			toolutil.WrapErrWithHint("create_work_item_saved_view", err,
				"verify namespace_path is a full group or project path you can write to, and that sort is a WorkItemSort enum value such as CREATED_DESC"))
	}
	return mutateOutput(fmt.Sprintf("Successfully created saved view %q.", name), toItem(view)), nil
}

// Update changes the supplied fields of an existing saved view.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (MutateOutput, error) {
	if err := ctx.Err(); err != nil {
		return MutateOutput{}, err
	}
	if input.SavedViewID <= 0 {
		return MutateOutput{}, toolutil.ErrRequiredInt64("update_work_item_saved_view", "saved_view_id")
	}
	filters, err := input.Filters.toSDK()
	if err != nil {
		return MutateOutput{}, err
	}
	opts := &gl.UpdateWorkItemSavedViewOptions{
		Name:        optionalString(strings.TrimSpace(input.Name)),
		Description: optionalString(strings.TrimSpace(input.Description)),
		IsPrivate:   input.IsPrivate,
		Filters:     filters,
		Sort:        optionalString(strings.TrimSpace(input.Sort)),
	}
	if len(input.DisplaySettings) > 0 {
		encoded, encodeErr := encodeDisplaySettings(input.DisplaySettings)
		if encodeErr != nil {
			return MutateOutput{}, encodeErr
		}
		opts.DisplaySettings = encoded
	}

	view, resp, err := client.GL().WorkItemSavedViews.UpdateWorkItemSavedView(input.SavedViewID, opts, gl.WithContext(ctx))
	if err != nil {
		return MutateOutput{}, toolutil.UnconfirmedWrite(client, resp, "update_work_item_saved_view", objectSavedView, err,
			toolutil.WrapErrWithHint("update_work_item_saved_view", err, notFoundHint))
	}
	return mutateOutput(fmt.Sprintf("Successfully updated saved view %d.", input.SavedViewID), toItem(view)), nil
}

// Delete permanently removes a saved view.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := ctx.Err(); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	if input.SavedViewID <= 0 {
		return toolutil.DeleteOutput{}, toolutil.ErrRequiredInt64("delete_work_item_saved_view", "saved_view_id")
	}
	if _, err := client.GL().WorkItemSavedViews.DeleteWorkItemSavedView(input.SavedViewID, gl.WithContext(ctx)); err != nil {
		return toolutil.DeleteOutput{}, toolutil.WrapErrWithHint("delete_work_item_saved_view", err, notFoundHint)
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("work item saved view %d", input.SavedViewID))
	return out, nil
}

// Subscribe subscribes the authenticated user to a saved view.
func Subscribe(ctx context.Context, client *gitlabclient.Client, input SubscribeInput) (MutateOutput, error) {
	if err := ctx.Err(); err != nil {
		return MutateOutput{}, err
	}
	if input.SavedViewID <= 0 {
		return MutateOutput{}, toolutil.ErrRequiredInt64("subscribe_work_item_saved_view", "saved_view_id")
	}
	view, resp, err := client.GL().WorkItemSavedViews.SubscribeWorkItemSavedView(input.SavedViewID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusInternalServerError) {
			return MutateOutput{}, toolutil.WrapErrWithHint("subscribe_work_item_saved_view", err, subscribeServerErrorHint)
		}
		return MutateOutput{}, toolutil.UnconfirmedWrite(client, resp, "subscribe_work_item_saved_view", objectSavedView, err,
			toolutil.WrapErrWithHint("subscribe_work_item_saved_view", err, notFoundHint))
	}
	return mutateOutput(fmt.Sprintf("Successfully subscribed to saved view %d.", input.SavedViewID), toItem(view)), nil
}

// Unsubscribe removes the authenticated user's subscription to a saved view.
func Unsubscribe(ctx context.Context, client *gitlabclient.Client, input UnsubscribeInput) (MutateOutput, error) {
	if err := ctx.Err(); err != nil {
		return MutateOutput{}, err
	}
	if input.SavedViewID <= 0 {
		return MutateOutput{}, toolutil.ErrRequiredInt64("unsubscribe_work_item_saved_view", "saved_view_id")
	}
	view, resp, err := client.GL().WorkItemSavedViews.UnsubscribeWorkItemSavedView(input.SavedViewID, gl.WithContext(ctx))
	if err != nil {
		return MutateOutput{}, toolutil.UnconfirmedWrite(client, resp, "unsubscribe_work_item_saved_view", objectSavedView, err,
			toolutil.WrapErrWithHint("unsubscribe_work_item_saved_view", err, notFoundHint))
	}
	return mutateOutput(fmt.Sprintf("Successfully unsubscribed from saved view %d.", input.SavedViewID), toItem(view)), nil
}

// mutateOutput builds the confirmation shared by every saved view mutation.
//
// It takes the already-converted [Item] rather than the SDK type on purpose:
// MutateOutput is a confirmation envelope, not a projection of
// [gl.WorkItemSavedView], and a func taking the SDK type would read as the
// converter for it. Item is that converter's target, and the 1:1 field audit
// pairs the two by exactly this signature shape.
func mutateOutput(message string, view Item) MutateOutput {
	return MutateOutput{Status: "success", Message: message, SavedView: view}
}

// requireNamespacePath trims and validates the namespace path every namespaced
// action needs.
func requireNamespacePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", toolutil.ErrFieldRequired("namespace_path")
	}
	return trimmed, nil
}

// encodeDisplaySettings marshals the opaque display settings object, defaulting
// an omitted one to an empty object.
func encodeDisplaySettings(settings map[string]any) (json.RawMessage, error) {
	if len(settings) == 0 {
		return emptyDisplaySettings, nil
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("display_settings must be a JSON object: %w", err)
	}
	return encoded, nil
}

// wrapLookupErr turns the SDK's not-found sentinel into an actionable message.
// The saved view endpoints are GraphQL, so a missing view arrives as
// [gl.ErrNotFound] rather than an HTTP 404 the status helpers can classify.
func wrapLookupErr(operation string, err error) error {
	if errors.Is(err, gl.ErrNotFound) {
		return toolutil.WrapErrWithHint(operation, err, notFoundHint)
	}
	return toolutil.WrapErr(operation, err)
}
