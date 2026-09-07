package epics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errHintEpicListPath is the hint both list paths attach to a 404: either one
// can be reached with a group path that does not resolve, or on an instance
// whose license does not carry epics at all.
const errHintEpicListPath = "verify full_path with gitlab_group_list; epics require GitLab Premium or Ultimate"

// LinkedItem represents a linked work item summary.
type LinkedItem struct {
	IID      int64  `json:"iid"`
	LinkType string `json:"link_type"`
	Path     string `json:"path,omitempty"`
}

// ChildItem is a summary reference to a child epic in the hierarchy. It
// mirrors the shape the work items domain publishes for the same widget.
type ChildItem struct {
	IID  int64  `json:"iid" jsonschema:"Internal ID (IID) of the child epic"`
	Path string `json:"path,omitempty" jsonschema:"Namespace full path of the child epic"`
}

// CreateLinkedItems specifies epics to link while the epic is being created.
// It mirrors the work items domain's shape for the same widget so one API
// carries one call shape on both tools.
type CreateLinkedItems struct {
	WorkItemIDs []int64 `json:"work_item_ids" jsonschema:"Global IDs of epics or work items to link,required"`
	LinkType    string  `json:"link_type" jsonschema:"Link type: BLOCKS, BLOCKED_BY, or RELATED,required"`
}

// BasicUserOutput mirrors gl.BasicUser (and the compatible gl.EpicAuthor),
// the compact user object embedded on the epic author and assignees keys.
// Per the 1:1 audit policy (full nested objects, C-IMPORTS) the SDK
// sub-object is replicated here rather than imported from a sibling package
// to preserve the zero-import-cycle constraint.
// locked and public_email are on the REST author and on neither SDK type: a
// live GET /api/v4/groups/gitlab-org/epics on 2026-09-07 answered with all
// eight keys of GitLab's user entity, and gl.EpicAuthor declares six. They stay
// empty on the Work Items path, whose author fragment selects id, username,
// name, state, createdAt, avatarUrl and webUrl.
type BasicUserOutput struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name,omitempty"`
	State       string `json:"state,omitempty"`
	Locked      bool   `json:"locked,omitempty"`
	PublicEmail string `json:"public_email,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	WebURL      string `json:"web_url,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// basicUserFromUser converts a gl.BasicUser (Work Items API author/assignee)
// to its output shape, returning nil when the SDK value is nil.
func basicUserFromUser(u *gl.BasicUser) *BasicUserOutput {
	if u == nil {
		return nil
	}
	out := &BasicUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL,
	}
	if u.CreatedAt != nil {
		out.CreatedAt = u.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// basicUsersFromUsers converts a slice of gl.BasicUser, skipping nil elements
// and returning nil for an empty or all-nil slice.
func basicUsersFromUsers(users []*gl.BasicUser) []*BasicUserOutput {
	if len(users) == 0 {
		return nil
	}
	out := make([]*BasicUserOutput, 0, len(users))
	for _, u := range users {
		if v := basicUserFromUser(u); v != nil {
			out = append(out, v)
		}
	}
	return out
}

// epicAuthorAPI decodes the REST epic author in full: gl.EpicAuthor plus the
// two keys GitLab sends that it does not declare. The epic response carries no
// created_at for its author, so that field stays empty on this path.
type epicAuthorAPI struct {
	gl.EpicAuthor
	Locked      bool   `json:"locked"`
	PublicEmail string `json:"public_email"`
}

// basicUserFromEpicAuthor converts a raw-fetched REST epic author to its output
// shape, returning nil when the response carried none.
func basicUserFromEpicAuthor(a *epicAuthorAPI) *BasicUserOutput {
	if a == nil {
		return nil
	}
	return &BasicUserOutput{
		ID: a.ID, Username: a.Username, Name: a.Name, State: a.State,
		Locked: a.Locked, PublicEmail: a.PublicEmail,
		AvatarURL: a.AvatarURL, WebURL: a.WebURL,
	}
}

// ResourceLinksOutput mirrors the `_links` object GitLab renders on a REST
// epic: the API URLs of the epic itself, its issues, its group and its parent.
// gl.Epic declares none of it.
type ResourceLinksOutput struct {
	Self       string `json:"self,omitempty"`
	EpicIssues string `json:"epic_issues,omitempty"`
	Group      string `json:"group,omitempty"`
	Parent     string `json:"parent,omitempty"`
}

// epicLabels decodes GitLab's dual-shape `labels` array on an epic: label names
// by default, and full label objects when with_labels_details is asked for.
//
// gl.Epic types the key []string alone, so with_labels_details=true made the
// whole response undecodable and the action answered a JSON error rather than
// the epics that matched. Both shapes fill Names, so a caller that never asked
// for the detail sees the same output it always did.
type epicLabels struct {
	Names   []string
	Details []*toolutil.LabelDetailsOutput
}

// UnmarshalJSON implements [json.Unmarshaler] for the dual-shape labels array.
func (l *epicLabels) UnmarshalJSON(data []byte) error {
	var names []string
	if err := json.Unmarshal(data, &names); err == nil {
		l.Names = names
		return nil
	}
	var details []*toolutil.LabelDetailsOutput
	if err := json.Unmarshal(data, &details); err != nil {
		return fmt.Errorf("epic labels: %w", err)
	}
	l.Details = details
	for _, d := range details {
		if d != nil {
			l.Names = append(l.Names, d.Name)
		}
	}
	return nil
}

// epicAPI decodes a REST epic in full: gl.Epic plus the fourteen fields GitLab
// sends that it does not declare, and the dual-shape labels array it cannot
// decode.
//
// The fields are GitLab's own, not ours to infer: its generated OpenAPI record
// lists all fourteen on each of the five epic GETs
// (docs/development/gitlab-api-shapes.json, GET /api/v4/groups/{id}/-/epics).
// Two further oracles each corroborate all but a couple, and not the same
// couple, so every field rests on the record plus at least one of them: live
// gitlab.com GETs on 2026-09-07 carried all but reference (the list response
// omits subscribed too, the single-epic one sends it), while doc/api/epics.md
// prints all but web_edit_url, which it never mentions, and text_color, which
// appears only in its with_labels_details parameter row.
// Reading them costs no extra round trip, since they arrive on the response
// client-go already asks for and discards.
//
// Author and Labels shadow the embedded gl.Epic fields of the same name: the
// shallower field is the one encoding/json fills, so the embedded ones stay
// empty and every read goes through these.
type epicAPI struct {
	gl.Epic
	Author                       *epicAuthorAPI             `json:"author"`
	Labels                       epicLabels                 `json:"labels"`
	ParentIID                    int64                      `json:"parent_iid"`
	Color                        string                     `json:"color"`
	TextColor                    string                     `json:"text_color"`
	WebEditURL                   string                     `json:"web_edit_url"`
	WorkItemID                   int64                      `json:"work_item_id"`
	Subscribed                   *bool                      `json:"subscribed"`
	Reference                    string                     `json:"reference"`
	References                   *toolutil.ReferencesOutput `json:"references"`
	Imported                     bool                       `json:"imported"`
	ImportedFrom                 string                     `json:"imported_from"`
	Links                        *ResourceLinksOutput       `json:"_links"`
	EndDate                      *gl.ISOTime                `json:"end_date"`
	StartDateFromInheritedSource *gl.ISOTime                `json:"start_date_from_inherited_source"`
	DueDateFromInheritedSource   *gl.ISOTime                `json:"due_date_from_inherited_source"`
}

// epicsListPath and epicChildrenPath are the two REST paths this package
// fetches raw, spelled the way client-go's own routes spell them.
func epicsListPath(fullPath string) string {
	return fmt.Sprintf("groups/%s/epics", gl.PathEscape(fullPath))
}

func epicChildrenPath(fullPath string, iid int64) string {
	return fmt.Sprintf("groups/%s/epics/%d/epics", gl.PathEscape(fullPath), iid)
}

// rawListEpics issues a raw REST GET against an epics list path, decoding the
// full documented response into the [epicAPI] superset instead of gl.Epic. The
// supplied opts encode the filters and pagination via their url struct tags,
// and the returned gl.Response preserves the pagination headers
// [toolutil.PaginationFromResponse] reads.
func rawListEpics(
	ctx context.Context, client *gitlabclient.Client, path string, opts *gl.ListGroupEpicsOptions,
) ([]*epicAPI, *gl.Response, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var epics []*epicAPI
	resp, err := client.GL().Do(req, &epics)
	return epics, resp, err
}

// ListInput defines parameters for listing group epics.
//
// Two GitLab APIs answer this action and the filters decide which: a request
// naming only what the REST epics endpoint accepts is served by it, and
// anything below that only Namespace.workItems can express routes the whole
// request through the Work Items GraphQL query. [usesWorkItemsPath] is the one
// place that decision is made, so a filter added here and forgotten there is
// silently dropped rather than refused.
type ListInput struct {
	FullPath            string   `json:"full_path" jsonschema:"Full path of the group (e.g. my-group or my-group/sub-group),required"`
	State               string   `json:"state,omitempty" jsonschema:"Filter by state (opened/closed/all)"`
	Search              string   `json:"search,omitempty" jsonschema:"Search in title and description"`
	In                  []string `json:"in,omitempty" jsonschema:"Fields the search term is matched against: TITLE, DESCRIPTION, or both. Both are searched when omitted"`
	AuthorUsername      string   `json:"author_username,omitempty" jsonschema:"Filter by author username"`
	AuthorID            *int64   `json:"author_id,omitempty" jsonschema:"Filter by author user ID. Accepted by the REST epics endpoint only, so it is dropped when another filter routes the request through the Work Items API, where author_username is the equivalent"`
	AssigneeUsernames   []string `json:"assignee_usernames,omitempty" jsonschema:"Filter by assignee usernames"`
	AssigneeWildcardID  string   `json:"assignee_wildcard_id,omitempty" jsonschema:"Filter by assignment state rather than by user: ANY, ME, or NONE"`
	IIDs                []string `json:"iids,omitempty" jsonschema:"Fetch only these epic IIDs, each as a decimal string (e.g. [\"12\", \"34\"])"`
	IDs                 []string `json:"ids,omitempty" jsonschema:"Fetch only these epics by global ID, in full gid://gitlab/WorkItem/123 form. Unlike every other id input here this is not a bare number"`
	ParentIDs           []string `json:"parent_ids,omitempty" jsonschema:"Return only epics whose parent is one of these global IDs, in full gid://gitlab/WorkItem/123 form"`
	LabelName           []string `json:"label_name,omitempty" jsonschema:"Filter by label names"`
	MilestoneTitle      []string `json:"milestone_title,omitempty" jsonschema:"Filter by the titles of the milestones assigned to the epic"`
	MilestoneWildcardID string   `json:"milestone_wildcard_id,omitempty" jsonschema:"Filter by milestone assignment rather than by title: ANY, NONE, STARTED, or UPCOMING"`
	MyReactionEmoji     string   `json:"my_reaction_emoji,omitempty" jsonschema:"Filter by reaction emoji the authenticated user awarded (e.g. thumbsup or None/Any)"`
	Confidential        *bool    `json:"confidential,omitempty" jsonschema:"Filter by confidentiality"`
	Subscribed          string   `json:"subscribed,omitempty" jsonschema:"Filter by the authenticated user's subscription: EXPLICITLY_SUBSCRIBED or EXPLICITLY_UNSUBSCRIBED"`
	HealthStatusFilter  string   `json:"health_status_filter,omitempty" tier:"ultimate" jsonschema:"Filter by health status: onTrack, needsAttention, atRisk, or the wildcards ANY and NONE"`
	Weight              string   `json:"weight,omitempty" tier:"premium" jsonschema:"Filter by weight, as a decimal string (e.g. \"5\")"`
	WeightWildcardID    string   `json:"weight_wildcard_id,omitempty" tier:"premium" jsonschema:"Filter by whether a weight is set rather than by its value: ANY or NONE"`
	OrderBy             string   `json:"order_by,omitempty" jsonschema:"Order epics by field (created_at, updated_at, title). Defaults to created_at"`
	Sort                string   `json:"sort,omitempty" jsonschema:"Sort order (asc or desc). Defaults to desc"`
	CreatedAfter        string   `json:"created_after,omitempty" jsonschema:"Return epics created after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore       string   `json:"created_before,omitempty" jsonschema:"Return epics created before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	UpdatedAfter        string   `json:"updated_after,omitempty" jsonschema:"Return epics updated on or after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	UpdatedBefore       string   `json:"updated_before,omitempty" jsonschema:"Return epics updated on or before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	ClosedAfter         string   `json:"closed_after,omitempty" jsonschema:"Return epics closed on or after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	ClosedBefore        string   `json:"closed_before,omitempty" jsonschema:"Return epics closed on or before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	DueAfter            string   `json:"due_after,omitempty" jsonschema:"Return epics due on or after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	DueBefore           string   `json:"due_before,omitempty" jsonschema:"Return epics due on or before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	WithLabelsDetails   *bool    `json:"with_labels_details,omitempty" jsonschema:"If true, return more details (name, color, description) for each label in the labels field. Accepted by the REST epics endpoint only, so it is dropped when another filter routes the request through the Work Items API"`
	IncludeAncestors    *bool    `json:"include_ancestors,omitempty" jsonschema:"Include epics from ancestor groups"`
	IncludeDescendants  *bool    `json:"include_descendants,omitempty" jsonschema:"Include epics from descendant groups"`
	toolutil.GraphQLCursorPaginationInput
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// usesWorkItemsPath reports whether the request names anything only the Work
// Items query can express.
//
// gl.ListGroupEpicsOptions carries none of these, so a filter left out of this
// list reaches GitLab on neither path: the REST endpoint is asked for an
// unfiltered page and the caller is handed it with no error. Every filter
// added to [ListInput] that the REST endpoint does not accept belongs here.
func usesWorkItemsPath(in ListInput) bool {
	return namesWorkItemsIdentity(in) || namesWorkItemsAttribute(in)
}

// namesWorkItemsIdentity covers the filters that name epics, people or a page
// position outright.
//
// author_username and confidential are here although GitLab's REST epics
// endpoint documents both: client-go's ListGroupEpicsOptions declares neither,
// so the Work Items query is the only way either one reaches GitLab from here.
func namesWorkItemsIdentity(in ListInput) bool {
	return in.AuthorUsername != "" || in.Confidential != nil ||
		in.After != "" || in.Before != "" || in.Last != nil ||
		len(in.AssigneeUsernames) > 0 || in.AssigneeWildcardID != "" ||
		len(in.IIDs) > 0 || len(in.IDs) > 0 || len(in.ParentIDs) > 0
}

// namesWorkItemsAttribute covers the filters that describe an epic rather than
// naming one.
func namesWorkItemsAttribute(in ListInput) bool {
	return len(in.In) > 0 ||
		len(in.MilestoneTitle) > 0 || in.MilestoneWildcardID != "" ||
		in.ClosedAfter != "" || in.ClosedBefore != "" ||
		in.DueAfter != "" || in.DueBefore != "" ||
		in.HealthStatusFilter != "" ||
		in.Weight != "" || in.WeightWildcardID != "" ||
		in.Subscribed != ""
}

// The two directions order_by and sort accept, spelled the way the REST epics
// endpoint spells them and the way this action publishes them.
const (
	sortAscending  = "asc"
	sortDescending = "desc"
)

// epicOrderByFields maps the order_by vocabulary this action publishes onto
// the field half of a WorkItemSort value.
//
// The REST endpoint takes the ordering as two parameters and the Work Items
// query takes it as one enum, so the pair is translated here rather than at
// either call site.
var epicOrderByFields = map[string]string{
	"created_at": "CREATED",
	"updated_at": "UPDATED",
	"title":      "TITLE",
}

// validateListOrdering refuses an ordering value outside the published
// vocabulary before either API is asked for a page.
//
// Neither path diagnoses one on its own. The REST endpoint answers 400 naming
// a parameter, and the Work Items query answers HTTP 200 carrying a GraphQL
// coercion error, which the status-keyed hint on this handler never fires for.
func validateListOrdering(input ListInput) error {
	if input.OrderBy != "" {
		if _, ok := epicOrderByFields[input.OrderBy]; !ok {
			return fmt.Errorf("epicList: order_by must be created_at, updated_at or title, got %q", input.OrderBy)
		}
	}
	switch input.Sort {
	case "", sortAscending, sortDescending:
		return nil
	default:
		return fmt.Errorf("epicList: sort must be %s or %s, got %q", sortAscending, sortDescending, input.Sort)
	}
}

// workItemsSort renders the order_by and sort pair as the single WorkItemSort
// value the Work Items query takes.
//
// The vocabulary this action publishes for sort is the REST endpoint's (asc,
// desc), and WorkItemSort has neither member: forwarding it verbatim asked
// GitLab to coerce "desc" into an enum of CREATED_DESC, TITLE_ASC and the
// rest, so no value of sort was valid on both the schema and this path. An
// omitted half falls back to what the REST endpoint defaults to, created_at
// descending, so the two paths order a page the same way.
func workItemsSort(orderBy, sort string) *string {
	if orderBy == "" && sort == "" {
		return nil
	}
	field, ok := epicOrderByFields[orderBy]
	if !ok {
		field = epicOrderByFields["created_at"]
	}
	direction := "DESC"
	if sort == sortAscending {
		direction = "ASC"
	}
	value := field + "_" + direction
	return &value
}

// GetInput defines parameters for getting a single epic.
type GetInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid" jsonschema:"Epic IID within the group,required"`
}

// GetLinksInput defines parameters for listing child epics (REST).
type GetLinksInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid" jsonschema:"Epic IID within the group,required"`
}

// CreateInput defines parameters for creating a new epic.
type CreateInput struct {
	FullPath     string             `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	Title        string             `json:"title" jsonschema:"Epic title,required"`
	Description  string             `json:"description,omitempty" jsonschema:"Epic description (Markdown supported)"`
	Confidential *bool              `json:"confidential,omitempty" jsonschema:"Whether the epic is confidential"`
	Color        string             `json:"color,omitempty" jsonschema:"Epic color (hex format, e.g. #FF0000)"`
	StartDate    string             `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate      string             `json:"due_date,omitempty" jsonschema:"Due date (YYYY-MM-DD)"`
	CreatedAt    string             `json:"created_at,omitempty" jsonschema:"Creation timestamp to record instead of now (ISO 8601, e.g. 2025-01-01T00:00:00Z). Honored for group owners and administrators only, and ignored when it cannot be parsed"`
	CreateSource string             `json:"create_source,omitempty" jsonschema:"Free-text label recording what triggered the creation. Used by GitLab for tracking only and never shown on the epic"`
	AssigneeIDs  []int64            `json:"assignee_ids,omitempty" jsonschema:"Global IDs of assignees"`
	LabelIDs     []int64            `json:"label_ids,omitempty" jsonschema:"Global IDs of labels"`
	MilestoneID  *int64             `json:"milestone_id,omitempty" jsonschema:"Global ID of the milestone to assign to the epic"`
	ParentID     *int64             `json:"parent_id,omitempty" jsonschema:"Global ID of the parent epic, which makes this one a sub-epic in the same call"`
	LinkedItems  *CreateLinkedItems `json:"linked_items,omitempty" tier:"ultimate" jsonschema:"Epics to link to the new one, all with the same link type"`
	Weight       *int64             `json:"weight,omitempty" tier:"premium" jsonschema:"Weight of the epic"`
	HealthStatus string             `json:"health_status,omitempty" tier:"ultimate" jsonschema:"Health status (onTrack/needsAttention/atRisk)"`
}

// UpdateInput defines parameters for updating an existing epic.
//
// There is no status here. An Epic work item carries no STATUS widget, so the
// mutation refuses the field: the widget list gitlab.com answered on
// 2026-09-07 for
// namespace(fullPath: "gitlab-org") { workItemTypes { nodes { name widgetDefinitions { type } } } }
// gives Epic AI_SESSION, ASSIGNEES, AWARD_EMOJI, COLOR, CURRENT_USER_TODOS,
// CUSTOM_FIELDS, DESCRIPTION, HEALTH_STATUS, HIERARCHY, LABELS, LINKED_ITEMS,
// MILESTONE, NOTES, NOTIFICATIONS, PARTICIPANTS, START_AND_DUE_DATE,
// TIME_TRACKING, VERIFICATION_STATUS and WEIGHT, and neither STATUS nor
// ITERATION nor CRM_CONTACTS. Issue and Task carry all three, which is where
// the field was copied from.
type UpdateInput struct {
	FullPath       string  `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID            int64   `json:"epic_iid" jsonschema:"Epic IID within the group,required"`
	Title          string  `json:"title,omitempty" jsonschema:"Updated epic title"`
	Description    string  `json:"description,omitempty" jsonschema:"Updated description (Markdown supported)"`
	StateEvent     string  `json:"state_event,omitempty" jsonschema:"State event: CLOSE or REOPEN"`
	ParentID       *int64  `json:"parent_id,omitempty" jsonschema:"Global ID of the parent epic work item"`
	Color          string  `json:"color,omitempty" jsonschema:"Epic color (hex format)"`
	StartDate      string  `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD)"`
	DueDate        string  `json:"due_date,omitempty" jsonschema:"Due date (YYYY-MM-DD)"`
	AddLabelIDs    []int64 `json:"add_label_ids,omitempty" jsonschema:"Global IDs of labels to add"`
	RemoveLabelIDs []int64 `json:"remove_label_ids,omitempty" jsonschema:"Global IDs of labels to remove"`
	AssigneeIDs    []int64 `json:"assignee_ids,omitempty" jsonschema:"Global IDs of assignees (empty array to remove all)"`
	MilestoneID    *int64  `json:"milestone_id,omitempty" jsonschema:"Global ID of the milestone to assign to the epic"`
	Weight         *int64  `json:"weight,omitempty" tier:"premium" jsonschema:"Weight of the epic"`
	HealthStatus   string  `json:"health_status,omitempty" tier:"ultimate" jsonschema:"Health status (onTrack/needsAttention/atRisk)"`
}

// DeleteInput defines parameters for deleting an epic.
type DeleteInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid" jsonschema:"Epic IID within the group,required"`
}

// Output represents a single epic (backed by a Work Item of type Epic, or by
// the REST epic for the list and child-epic links endpoints). Per the 1:1 audit
// policy it carries the union of what each source exposes; fields absent on a
// given source stay at their zero value.
//
// There is no status and no iteration_id. Both were always null: an Epic work
// item carries neither the STATUS nor the ITERATION widget, per the widget list
// gitlab.com answered on 2026-09-07 for
// namespace(fullPath: "gitlab-org") { workItemTypes { nodes { name widgetDefinitions { type } } } },
// and neither is a field of the REST epic. There is no user_notes_count and no
// url either: gl.Epic declares both, and GitLab's OpenAPI record, its
// doc/api/epics.md example bodies and a live gitlab.com response agree that no
// epic endpoint sends either one.
type Output struct {
	toolutil.HintableOutput
	ID           int64                          `json:"id"`
	IID          int64                          `json:"iid"`
	Type         string                         `json:"type"`
	State        string                         `json:"state"`
	Title        string                         `json:"title"`
	Description  string                         `json:"description,omitempty"`
	WebURL       string                         `json:"web_url,omitempty"`
	WebEditURL   string                         `json:"web_edit_url,omitempty"`
	GroupID      int64                          `json:"group_id,omitempty"`
	ParentID     int64                          `json:"parent_id,omitempty"`
	WorkItemID   int64                          `json:"work_item_id,omitempty"`
	Author       *BasicUserOutput               `json:"author,omitempty"`
	Assignees    []*BasicUserOutput             `json:"assignees,omitempty"`
	Labels       []string                       `json:"labels,omitempty"`
	LabelDetails []*toolutil.LabelDetailsOutput `json:"label_details,omitempty"`
	LinkedItems  []LinkedItem                   `json:"linked_items,omitempty" tier:"ultimate"`
	Children     []ChildItem                    `json:"children,omitempty"`
	Confidential bool                           `json:"confidential,omitempty"`
	Color        string                         `json:"color,omitempty"`
	TextColor    string                         `json:"text_color,omitempty"`

	StartDate                    string `json:"start_date,omitempty"`
	StartDateIsFixed             bool   `json:"start_date_is_fixed,omitempty"`
	StartDateFixed               string `json:"start_date_fixed,omitempty"`
	StartDateFromMilestones      string `json:"start_date_from_milestones,omitempty"`
	StartDateFromInheritedSource string `json:"start_date_from_inherited_source,omitempty"`
	DueDate                      string `json:"due_date,omitempty"`
	DueDateIsFixed               bool   `json:"due_date_is_fixed,omitempty"`
	DueDateFixed                 string `json:"due_date_fixed,omitempty"`
	DueDateFromMilestones        string `json:"due_date_from_milestones,omitempty"`
	DueDateFromInheritedSource   string `json:"due_date_from_inherited_source,omitempty"`
	EndDate                      string `json:"end_date,omitempty"`

	HealthStatus string                     `json:"health_status,omitempty" tier:"ultimate"`
	Weight       *int64                     `json:"weight,omitempty" tier:"premium"`
	MilestoneID  *int64                     `json:"milestone_id,omitempty"`
	Upvotes      int64                      `json:"upvotes,omitempty"`
	Downvotes    int64                      `json:"downvotes,omitempty"`
	Subscribed   *bool                      `json:"subscribed,omitempty"`
	Reference    string                     `json:"reference,omitempty"`
	References   *toolutil.ReferencesOutput `json:"references,omitempty"`
	Imported     bool                       `json:"imported,omitempty"`
	ImportedFrom string                     `json:"imported_from,omitempty"`
	Links        *ResourceLinksOutput       `json:"_links,omitempty"`
	ParentIID    int64                      `json:"parent_iid,omitempty"`
	ParentPath   string                     `json:"parent_path,omitempty"`
	CreatedAt    string                     `json:"created_at,omitempty"`
	UpdatedAt    string                     `json:"updated_at,omitempty"`
	ClosedAt     string                     `json:"closed_at,omitempty"`
}

// ListOutput holds a page of epics plus the pagination block of whichever API
// answered.
//
// The two blocks are mutually exclusive and both are omitted when empty, so
// the one a response carries is what tells a caller which path served it: the
// REST epics endpoint pages by number and reports X-Next-Page, while the Work
// Items query pages by cursor and reports pageInfo. Publishing one shape for
// both would mean answering a full REST page with has_next_page false, which
// is how a caller stops paging one item short of the rest of the list.
type ListOutput struct {
	toolutil.HintableOutput
	Epics            []Output                          `json:"epics"`
	Pagination       *toolutil.GraphQLPaginationOutput `json:"pagination,omitempty"`
	OffsetPagination *toolutil.PaginationOutput        `json:"offset_pagination,omitempty"`
}

// LinksOutput holds child epics of a parent epic (REST-backed).
type LinksOutput struct {
	toolutil.HintableOutput
	ChildEpics []LinksItem `json:"child_epics"`
}

// LinksItem is the child-epic output for the GetLinks REST endpoint. Per the
// 1:1 audit policy it surfaces the whole REST epic: every field of gl.Epic
// GitLab sends, plus the fourteen it sends that gl.Epic does not declare
// (see [epicAPI]). The author is a full nested object.
//
// user_notes_count and url are absent for the reason [Output] records: gl.Epic
// declares them and no epic endpoint sends them.
type LinksItem struct {
	ID           int64                          `json:"id"`
	IID          int64                          `json:"iid"`
	GroupID      int64                          `json:"group_id,omitempty"`
	ParentID     int64                          `json:"parent_id,omitempty"`
	ParentIID    int64                          `json:"parent_iid,omitempty"`
	WorkItemID   int64                          `json:"work_item_id,omitempty"`
	Title        string                         `json:"title"`
	Description  string                         `json:"description,omitempty"`
	State        string                         `json:"state"`
	WebURL       string                         `json:"web_url,omitempty"`
	WebEditURL   string                         `json:"web_edit_url,omitempty"`
	Author       *BasicUserOutput               `json:"author,omitempty"`
	Labels       []string                       `json:"labels,omitempty"`
	LabelDetails []*toolutil.LabelDetailsOutput `json:"label_details,omitempty"`
	Confidential bool                           `json:"confidential,omitempty"`
	Color        string                         `json:"color,omitempty"`
	TextColor    string                         `json:"text_color,omitempty"`

	StartDate                    string `json:"start_date,omitempty"`
	StartDateIsFixed             bool   `json:"start_date_is_fixed,omitempty"`
	StartDateFixed               string `json:"start_date_fixed,omitempty"`
	StartDateFromMilestones      string `json:"start_date_from_milestones,omitempty"`
	StartDateFromInheritedSource string `json:"start_date_from_inherited_source,omitempty"`
	DueDate                      string `json:"due_date,omitempty"`
	DueDateIsFixed               bool   `json:"due_date_is_fixed,omitempty"`
	DueDateFixed                 string `json:"due_date_fixed,omitempty"`
	DueDateFromMilestones        string `json:"due_date_from_milestones,omitempty"`
	DueDateFromInheritedSource   string `json:"due_date_from_inherited_source,omitempty"`
	EndDate                      string `json:"end_date,omitempty"`

	Upvotes      int64                      `json:"upvotes,omitempty"`
	Downvotes    int64                      `json:"downvotes,omitempty"`
	Subscribed   *bool                      `json:"subscribed,omitempty"`
	Reference    string                     `json:"reference,omitempty"`
	References   *toolutil.ReferencesOutput `json:"references,omitempty"`
	Imported     bool                       `json:"imported,omitempty"`
	ImportedFrom string                     `json:"imported_from,omitempty"`
	Links        *ResourceLinksOutput       `json:"_links,omitempty"`
	CreatedAt    string                     `json:"created_at,omitempty"`
	UpdatedAt    string                     `json:"updated_at,omitempty"`
	ClosedAt     string                     `json:"closed_at,omitempty"`
}

// toOutput converts a GitLab Work Item to the epic Output format.
//
// work_item_id repeats the id here on purpose: on this path the identifier
// GitLab hands back is the work item's, so the key the REST path fills from a
// field of its own carries the same value rather than nothing.
func toOutput(wi *gl.WorkItem) Output {
	out := Output{
		ID:           wi.ID,
		IID:          wi.IID,
		WorkItemID:   wi.ID,
		Type:         wi.Type,
		State:        wi.State,
		Title:        wi.Title,
		Description:  wi.Description,
		WebURL:       wi.WebURL,
		Confidential: wi.Confidential,
		Weight:       wi.Weight,
		MilestoneID:  wi.MilestoneID,
	}
	out.Author = basicUserFromUser(wi.Author)
	out.Assignees = basicUsersFromUsers(wi.Assignees)
	// The work item query fetches each label whole, so label_details costs
	// nothing here: the names alone were the discarded half of what arrived.
	for _, l := range wi.Labels {
		out.Labels = append(out.Labels, l.Name)
		out.LabelDetails = append(out.LabelDetails, &toolutil.LabelDetailsOutput{
			ID: l.ID, Name: l.Name, Color: l.Color,
			Description: l.Description, DescriptionHTML: l.DescriptionHTML, TextColor: l.TextColor,
		})
	}
	for _, li := range wi.LinkedItems {
		out.LinkedItems = append(out.LinkedItems, LinkedItem{
			IID:      li.IID,
			LinkType: li.LinkType,
			Path:     li.NamespacePath,
		})
	}
	for _, child := range wi.Children {
		out.Children = append(out.Children, ChildItem{IID: child.IID, Path: child.NamespacePath})
	}
	if wi.Color != nil {
		out.Color = *wi.Color
	}
	if wi.HealthStatus != nil {
		out.HealthStatus = *wi.HealthStatus
	}
	if wi.Parent != nil {
		out.ParentIID = wi.Parent.IID
		out.ParentPath = wi.Parent.NamespacePath
	}
	if wi.StartDate != nil {
		out.StartDate = time.Time(*wi.StartDate).Format(time.DateOnly)
	}
	if wi.DueDate != nil {
		out.DueDate = time.Time(*wi.DueDate).Format(time.DateOnly)
	}
	if wi.CreatedAt != nil {
		out.CreatedAt = wi.CreatedAt.Format(time.RFC3339)
	}
	if wi.UpdatedAt != nil {
		out.UpdatedAt = wi.UpdatedAt.Format(time.RFC3339)
	}
	if wi.ClosedAt != nil {
		out.ClosedAt = wi.ClosedAt.Format(time.RFC3339)
	}
	return out
}

// formatISODate renders an optional gl.ISOTime as YYYY-MM-DD, or "" when nil.
func formatISODate(t *gl.ISOTime) string {
	if t == nil {
		return ""
	}
	return time.Time(*t).Format(time.DateOnly)
}

// toLinkItem converts a raw-fetched REST epic to the LinksItem format.
func toLinkItem(e *epicAPI) LinksItem {
	item := LinksItem{
		ID:           e.ID,
		IID:          e.IID,
		GroupID:      e.GroupID,
		ParentID:     e.ParentID,
		ParentIID:    e.ParentIID,
		WorkItemID:   e.WorkItemID,
		Title:        e.Title,
		Description:  e.Description,
		State:        e.State,
		WebURL:       e.WebURL,
		WebEditURL:   e.WebEditURL,
		Author:       basicUserFromEpicAuthor(e.Author),
		Labels:       e.Labels.Names,
		LabelDetails: e.Labels.Details,
		Confidential: e.Confidential,
		Color:        e.Color,
		TextColor:    e.TextColor,
		Subscribed:   e.Subscribed,
		Reference:    e.Reference,
		References:   e.References,
		Imported:     e.Imported,
		ImportedFrom: e.ImportedFrom,
		Links:        e.Links,
		Upvotes:      e.Upvotes,
		Downvotes:    e.Downvotes,
		CreatedAt:    toolutil.FormatTimePtr(e.CreatedAt),
		UpdatedAt:    toolutil.FormatTimePtr(e.UpdatedAt),
		ClosedAt:     toolutil.FormatTimePtr(e.ClosedAt),
	}
	applyLinkItemDates(&item, e)
	return item
}

// applyLinkItemDates copies the ten date fields of a REST epic, keeping
// [toLinkItem] to one screen.
func applyLinkItemDates(item *LinksItem, e *epicAPI) {
	item.StartDate = formatISODate(e.StartDate)
	item.StartDateIsFixed = e.StartDateIsFixed
	item.StartDateFixed = formatISODate(e.StartDateFixed)
	item.StartDateFromMilestones = formatISODate(e.StartDateFromMilestones)
	item.StartDateFromInheritedSource = formatISODate(e.StartDateFromInheritedSource)
	item.DueDate = formatISODate(e.DueDate)
	item.DueDateIsFixed = e.DueDateIsFixed
	item.DueDateFixed = formatISODate(e.DueDateFixed)
	item.DueDateFromMilestones = formatISODate(e.DueDateFromMilestones)
	item.DueDateFromInheritedSource = formatISODate(e.DueDateFromInheritedSource)
	item.EndDate = formatISODate(e.EndDate)
}

// epicToOutput converts a raw-fetched REST epic to the epic Output format.
//
// parent_iid comes from GitLab's own field, never from ParentID: the response
// carries the two as different numbers, and filling the internal ID from the
// global one published a parent a caller could feed back to epic_get and reach
// the wrong epic. gl.Epic declares only ParentID, which is why the raw fetch
// exists.
func epicToOutput(e *epicAPI) Output {
	out := Output{
		ID:           e.ID,
		IID:          e.IID,
		Type:         "Epic",
		State:        e.State,
		Title:        e.Title,
		Description:  e.Description,
		WebURL:       e.WebURL,
		WebEditURL:   e.WebEditURL,
		GroupID:      e.GroupID,
		ParentID:     e.ParentID,
		ParentIID:    e.ParentIID,
		WorkItemID:   e.WorkItemID,
		Author:       basicUserFromEpicAuthor(e.Author),
		Labels:       e.Labels.Names,
		LabelDetails: e.Labels.Details,
		Confidential: e.Confidential,
		Color:        e.Color,
		TextColor:    e.TextColor,
		Subscribed:   e.Subscribed,
		Reference:    e.Reference,
		References:   e.References,
		Imported:     e.Imported,
		ImportedFrom: e.ImportedFrom,
		Links:        e.Links,
		Upvotes:      e.Upvotes,
		Downvotes:    e.Downvotes,
		CreatedAt:    toolutil.FormatTimePtr(e.CreatedAt),
		UpdatedAt:    toolutil.FormatTimePtr(e.UpdatedAt),
		ClosedAt:     toolutil.FormatTimePtr(e.ClosedAt),
	}
	applyOutputDates(&out, e)
	return out
}

// applyOutputDates copies the eleven date fields of a REST epic, keeping
// [epicToOutput] to one screen.
func applyOutputDates(out *Output, e *epicAPI) {
	out.StartDate = formatISODate(e.StartDate)
	out.StartDateIsFixed = e.StartDateIsFixed
	out.StartDateFixed = formatISODate(e.StartDateFixed)
	out.StartDateFromMilestones = formatISODate(e.StartDateFromMilestones)
	out.StartDateFromInheritedSource = formatISODate(e.StartDateFromInheritedSource)
	out.DueDate = formatISODate(e.DueDate)
	out.DueDateIsFixed = e.DueDateIsFixed
	out.DueDateFixed = formatISODate(e.DueDateFixed)
	out.DueDateFromMilestones = formatISODate(e.DueDateFromMilestones)
	out.DueDateFromInheritedSource = formatISODate(e.DueDateFromInheritedSource)
	out.EndDate = formatISODate(e.EndDate)
}

// buildEpicListOptions maps a ListInput onto the REST ListGroupEpicsOptions,
// setting only the filter, ordering, date, and pagination parameters the
// caller supplied.
func buildEpicListOptions(input ListInput) *gl.ListGroupEpicsOptions {
	perPage := int64(20)
	if input.First != nil && *input.First > 0 && *input.First <= toolutil.GraphQLMaxFirst {
		perPage = int64(*input.First)
	}
	opts := &gl.ListGroupEpicsOptions{PerPage: perPage}
	if input.State != "" {
		opts.State = &input.State
	}
	if input.Search != "" {
		opts.Search = &input.Search
	}
	if input.AuthorID != nil {
		opts.AuthorID = input.AuthorID
	}
	if len(input.LabelName) > 0 {
		labels := gl.LabelOptions(input.LabelName)
		opts.Labels = &labels
	}
	if input.MyReactionEmoji != "" {
		opts.MyReactionEmoji = &input.MyReactionEmoji
	}
	if input.OrderBy != "" {
		opts.OrderBy = &input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}
	if t := toolutil.ParseOptionalTime(input.CreatedAfter); t != nil {
		opts.CreatedAfter = t
	}
	if t := toolutil.ParseOptionalTime(input.CreatedBefore); t != nil {
		opts.CreatedBefore = t
	}
	if t := toolutil.ParseOptionalTime(input.UpdatedAfter); t != nil {
		opts.UpdatedAfter = t
	}
	if t := toolutil.ParseOptionalTime(input.UpdatedBefore); t != nil {
		opts.UpdatedBefore = t
	}
	if input.WithLabelsDetails != nil {
		opts.WithLabelDetails = input.WithLabelsDetails
	}
	if input.IncludeAncestors != nil {
		opts.IncludeAncestorGroups = input.IncludeAncestors
	}
	if input.IncludeDescendants != nil {
		opts.IncludeDescendantGroups = input.IncludeDescendants
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	return opts
}

// List retrieves epics for a group using the Work Items API with type filter.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.FullPath == "" {
		return ListOutput{}, errors.New("epicList: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if err := validateListOrdering(input); err != nil {
		return ListOutput{}, err
	}
	if usesWorkItemsPath(input) {
		return listWithWorkItems(ctx, client, input)
	}

	opts := buildEpicListOptions(input)
	items, resp, err := rawListEpics(ctx, client, epicsListPath(input.FullPath), opts)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("epicList", err, http.StatusNotFound,
			errHintEpicListPath)
	}
	out := make([]Output, 0, len(items))
	for _, epic := range items {
		out = append(out, epicToOutput(epic))
	}
	offset := toolutil.PaginationFromResponse(resp)
	return ListOutput{Epics: out, OffsetPagination: &offset}, nil
}

func listWithWorkItems(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	// The direction is resolved by the shared helper rather than here, so a
	// backward request is answered the way every other cursor-paged domain
	// answers one, with exactly one of first and last reaching GitLab.
	cursor, err := input.Resolve()
	if err != nil {
		return ListOutput{}, fmt.Errorf("epicList: %w", err)
	}

	items, resp, err := client.GL().WorkItems.ListWorkItems(
		input.FullPath, buildWorkItemsListOptions(input, cursor), gl.WithContext(ctx),
	)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("epicList", err, http.StatusNotFound,
			errHintEpicListPath)
	}
	out := make([]Output, 0, len(items))
	for _, wi := range items {
		out = append(out, toOutput(wi))
	}
	result := ListOutput{Epics: out}
	if resp != nil && resp.PageInfo != nil {
		result.Pagination = &toolutil.GraphQLPaginationOutput{
			HasNextPage:     resp.PageInfo.HasNextPage,
			HasPreviousPage: resp.PageInfo.HasPreviousPage,
			EndCursor:       resp.PageInfo.EndCursor,
			StartCursor:     resp.PageInfo.StartCursor,
		}
	}
	return result, nil
}

// buildWorkItemsListOptions maps a ListInput onto the Work Items query
// options, splitting the assignment across helpers so no single one carries
// the whole filter set.
func buildWorkItemsListOptions(input ListInput, cursor toolutil.GraphQLCursor) *gl.ListWorkItemsOptions {
	opts := &gl.ListWorkItemsOptions{
		Types: []string{"EPIC"},
		// As of client-go v2.49.0, ListWorkItems returns only CE fields by
		// default; EE fields are omitted unless requested via ReturnedFields.
		// Epics are Premium/Ultimate and the output maps EE fields (weight,
		// color, health_status), so opt into them explicitly or they silently
		// come back empty. status and iteration are deliberately not asked
		// for: an Epic carries neither widget, per the widget list gitlab.com
		// answered on 2026-09-07 for namespace(fullPath: "gitlab-org")
		// { workItemTypes { nodes { name widgetDefinitions { type } } } }.
		ReturnedFields: append(append([]string(nil), gl.WorkItemDefaultListFields()...),
			"color", "healthStatus", "weight"),
	}
	applyWorkItemsCursor(opts, cursor)
	applyWorkItemsTextFilters(opts, input)
	applyWorkItemsIdentityFilters(opts, input)
	applyWorkItemsAttributeFilters(opts, input)
	applyWorkItemsTimeFilters(opts, input)
	return opts
}

// applyWorkItemsCursor puts the resolved page request on the options.
func applyWorkItemsCursor(opts *gl.ListWorkItemsOptions, cursor toolutil.GraphQLCursor) {
	if cursor.First != nil {
		opts.First = new(int64(*cursor.First))
	}
	if cursor.Last != nil {
		opts.Last = new(int64(*cursor.Last))
	}
	if cursor.After != "" {
		opts.After = &cursor.After
	}
	if cursor.Before != "" {
		opts.Before = &cursor.Before
	}
}

// applyWorkItemsTextFilters copies the free-text and label filters.
func applyWorkItemsTextFilters(opts *gl.ListWorkItemsOptions, input ListInput) {
	if input.State != "" {
		opts.State = &input.State
	}
	if input.Search != "" {
		opts.Search = &input.Search
	}
	if len(input.In) > 0 {
		opts.In = input.In
	}
	if input.AuthorUsername != "" {
		opts.AuthorUsername = &input.AuthorUsername
	}
	if input.MyReactionEmoji != "" {
		opts.MyReactionEmoji = &input.MyReactionEmoji
	}
	if len(input.LabelName) > 0 {
		opts.LabelName = input.LabelName
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
}

// applyWorkItemsIdentityFilters copies the filters that name epics or people
// outright rather than describing them.
func applyWorkItemsIdentityFilters(opts *gl.ListWorkItemsOptions, input ListInput) {
	if len(input.AssigneeUsernames) > 0 {
		opts.AssigneeUsernames = input.AssigneeUsernames
	}
	if input.AssigneeWildcardID != "" {
		opts.AssigneeWildcardID = &input.AssigneeWildcardID
	}
	if len(input.IIDs) > 0 {
		opts.IIDs = input.IIDs
	}
	if len(input.IDs) > 0 {
		opts.IDs = input.IDs
	}
	if len(input.ParentIDs) > 0 {
		opts.ParentIDs = input.ParentIDs
	}
}

// applyWorkItemsAttributeFilters copies the filters keyed on an epic's own
// attributes, plus ordering and the ancestry scope.
func applyWorkItemsAttributeFilters(opts *gl.ListWorkItemsOptions, input ListInput) {
	if len(input.MilestoneTitle) > 0 {
		opts.MilestoneTitle = input.MilestoneTitle
	}
	if input.MilestoneWildcardID != "" {
		opts.MilestoneWildcardID = &input.MilestoneWildcardID
	}
	if input.HealthStatusFilter != "" {
		opts.HealthStatusFilter = &input.HealthStatusFilter
	}
	if input.Weight != "" {
		opts.Weight = &input.Weight
	}
	if input.WeightWildcardID != "" {
		opts.WeightWildcardID = &input.WeightWildcardID
	}
	if input.Subscribed != "" {
		opts.Subscribed = &input.Subscribed
	}
	opts.Sort = workItemsSort(input.OrderBy, input.Sort)
	if input.IncludeAncestors != nil {
		opts.IncludeAncestors = input.IncludeAncestors
	}
	if input.IncludeDescendants != nil {
		opts.IncludeDescendants = input.IncludeDescendants
	}
}

// applyWorkItemsTimeFilters copies the four date ranges.
//
// The created and updated pairs are here rather than only on the REST path
// because a caller combining one of them with a Work-Items-only filter reaches
// this query, and until they were wired the date half of such a request was
// dropped without a word.
func applyWorkItemsTimeFilters(opts *gl.ListWorkItemsOptions, input ListInput) {
	opts.CreatedAfter = toolutil.ParseOptionalTime(input.CreatedAfter)
	opts.CreatedBefore = toolutil.ParseOptionalTime(input.CreatedBefore)
	opts.UpdatedAfter = toolutil.ParseOptionalTime(input.UpdatedAfter)
	opts.UpdatedBefore = toolutil.ParseOptionalTime(input.UpdatedBefore)
	opts.ClosedAfter = toolutil.ParseOptionalTime(input.ClosedAfter)
	opts.ClosedBefore = toolutil.ParseOptionalTime(input.ClosedBefore)
	opts.DueAfter = toolutil.ParseOptionalTime(input.DueAfter)
	opts.DueBefore = toolutil.ParseOptionalTime(input.DueBefore)
}

// Get retrieves a single epic by its IID using the Work Items API.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicGet: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicGet", "epic_iid")
	}
	wi, _, err := client.GL().WorkItems.GetWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("epicGet", err, http.StatusNotFound,
			"verify iid with gitlab_epic_list; full_path must be the group path (e.g. group/subgroup) where the epic lives")
	}
	return toOutput(wi), nil
}

// GetLinks retrieves all child epics of a parent epic.
// This handler uses the REST API because client-go v2 does not yet expose
// a GraphQL query for work item children.
func GetLinks(ctx context.Context, client *gitlabclient.Client, input GetLinksInput) (LinksOutput, error) {
	if err := ctx.Err(); err != nil {
		return LinksOutput{}, err
	}
	if input.FullPath == "" {
		return LinksOutput{}, errors.New("epicGetLinks: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return LinksOutput{}, toolutil.ErrRequiredInt64("epicGetLinks", "epic_iid")
	}
	epics, _, err := rawListEpics(ctx, client, epicChildrenPath(input.FullPath, input.IID), nil)
	if err != nil {
		return LinksOutput{}, toolutil.WrapErrWithStatusHint("epicGetLinks", err, http.StatusNotFound,
			"verify iid with gitlab_epic_list; child epics are returned only when the parent epic exists in the given group")
	}
	out := make([]LinksItem, len(epics))
	for i, e := range epics {
		out[i] = toLinkItem(e)
	}
	return LinksOutput{ChildEpics: out}, nil
}

// Create creates a new epic using the Work Items API with the Epic type.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicCreate: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.Title == "" {
		return Output{}, errors.New("epicCreate: title is required")
	}
	wi, _, err := client.GL().WorkItems.CreateWorkItem(
		input.FullPath, gl.WorkItemTypeEpic, buildCreateOptions(input), gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("epicCreate", err, http.StatusForbidden,
			"creating epics requires Reporter role or higher; epics require GitLab Premium or Ultimate")
	}
	return toOutput(wi), nil
}

// buildCreateOptions maps a CreateInput onto the Work Items create options.
func buildCreateOptions(input CreateInput) *gl.CreateWorkItemOptions {
	opts := &gl.CreateWorkItemOptions{
		Title:     input.Title,
		CreatedAt: toolutil.ParseOptionalTime(input.CreatedAt),
	}
	if input.Description != "" {
		desc := toolutil.NormalizeText(input.Description)
		opts.Description = &desc
	}
	if input.Confidential != nil {
		opts.Confidential = input.Confidential
	}
	if input.CreateSource != "" {
		opts.CreateSource = &input.CreateSource
	}
	if len(input.AssigneeIDs) > 0 {
		opts.AssigneeIDs = input.AssigneeIDs
	}
	if len(input.LabelIDs) > 0 {
		opts.LabelIDs = input.LabelIDs
	}
	if input.MilestoneID != nil {
		opts.MilestoneID = input.MilestoneID
	}
	if input.ParentID != nil {
		opts.ParentID = input.ParentID
	}
	if input.LinkedItems != nil && len(input.LinkedItems.WorkItemIDs) > 0 {
		opts.LinkedItems = &gl.CreateWorkItemOptionsLinkedItems{
			LinkType:    &input.LinkedItems.LinkType,
			WorkItemIDs: input.LinkedItems.WorkItemIDs,
		}
	}
	if input.Weight != nil {
		opts.Weight = input.Weight
	}
	if input.HealthStatus != "" {
		opts.HealthStatus = &input.HealthStatus
	}
	if input.Color != "" {
		opts.Color = &input.Color
	}
	applyCreateDates(opts, input)
	return opts
}

// applyCreateDates parses the two YYYY-MM-DD inputs onto the options, leaving
// a value that does not parse unset rather than failing the call.
func applyCreateDates(opts *gl.CreateWorkItemOptions, input CreateInput) {
	if d, err := time.Parse(time.DateOnly, input.StartDate); err == nil {
		isoDate := gl.ISOTime(d)
		opts.StartDate = &isoDate
	}
	if d, err := time.Parse(time.DateOnly, input.DueDate); err == nil {
		isoDate := gl.ISOTime(d)
		opts.DueDate = &isoDate
	}
}

// Update modifies an existing epic using the Work Items API.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicUpdate: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicUpdate", "epic_iid")
	}
	wi, _, err := client.GL().WorkItems.UpdateWorkItem(
		input.FullPath, input.IID, buildUpdateOptions(input), gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("epicUpdate", err, http.StatusBadRequest,
			"state_event must be 'close' or 'reopen'; dates must be YYYY-MM-DD; verify iid with gitlab_epic_list")
	}
	return toOutput(wi), nil
}

// buildUpdateOptions maps an UpdateInput onto the Work Items update options.
func buildUpdateOptions(input UpdateInput) *gl.UpdateWorkItemOptions {
	opts := &gl.UpdateWorkItemOptions{}
	if input.Title != "" {
		opts.Title = &input.Title
	}
	if input.Description != "" {
		desc := toolutil.NormalizeText(input.Description)
		opts.Description = &desc
	}
	if input.StateEvent != "" {
		ev := gl.WorkItemStateEvent(input.StateEvent)
		opts.StateEvent = &ev
	}
	if input.ParentID != nil {
		opts.ParentID = input.ParentID
	}
	if input.MilestoneID != nil {
		opts.MilestoneID = input.MilestoneID
	}
	if input.Color != "" {
		opts.Color = &input.Color
	}
	if len(input.AddLabelIDs) > 0 {
		opts.AddLabelIDs = input.AddLabelIDs
	}
	if len(input.RemoveLabelIDs) > 0 {
		opts.RemoveLabelIDs = input.RemoveLabelIDs
	}
	if input.AssigneeIDs != nil {
		opts.AssigneeIDs = input.AssigneeIDs
	}
	applyUpdateWidgets(opts, input)
	return opts
}

// applyUpdateWidgets copies the widget-backed fields onto the update options.
func applyUpdateWidgets(opts *gl.UpdateWorkItemOptions, input UpdateInput) {
	if input.Weight != nil {
		opts.Weight = input.Weight
	}
	if input.HealthStatus != "" {
		opts.HealthStatus = &input.HealthStatus
	}
	if d, err := time.Parse(time.DateOnly, input.StartDate); err == nil {
		isoDate := gl.ISOTime(d)
		opts.StartDate = &isoDate
	}
	if d, err := time.Parse(time.DateOnly, input.DueDate); err == nil {
		isoDate := gl.ISOTime(d)
		opts.DueDate = &isoDate
	}
}

// Delete permanently removes an epic using the Work Items API.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.FullPath == "" {
		return errors.New("epicDelete: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("epicDelete", "epic_iid")
	}
	_, err := client.GL().WorkItems.DeleteWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("epicDelete", err, http.StatusForbidden,
			"deleting epics requires Owner role at the group level")
	}
	return nil
}
