package epics

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
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name,omitempty"`
	State     string `json:"state,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
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

// basicUserFromEpicAuthor converts a gl.EpicAuthor (REST Epics API author) to
// its output shape, returning nil when the SDK value is nil. gl.EpicAuthor has
// no created_at field, so CreatedAt is left empty.
func basicUserFromEpicAuthor(a *gl.EpicAuthor) *BasicUserOutput {
	if a == nil {
		return nil
	}
	return &BasicUserOutput{
		ID: a.ID, Username: a.Username, Name: a.Name, State: a.State,
		AvatarURL: a.AvatarURL, WebURL: a.WebURL,
	}
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
	OrderBy             string   `json:"order_by,omitempty" jsonschema:"Order epics by field (created_at, updated_at, title). Accepted by the REST epics endpoint only, so it is dropped when another filter routes the request through the Work Items API, where sort is the equivalent"`
	Sort                string   `json:"sort,omitempty" jsonschema:"Sort order (asc or desc)"`
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
	Status         string  `json:"status,omitempty" jsonschema:"Work item status: TODO, IN_PROGRESS, DONE, WONT_DO, or DUPLICATE"`
}

// DeleteInput defines parameters for deleting an epic.
type DeleteInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid" jsonschema:"Epic IID within the group,required"`
}

// Output represents a single epic (backed by a Work Item of type Epic, or by
// the REST gl.Epic for the child-epic links endpoint). Per the 1:1 audit
// policy it carries the union of fields exposed by gl.WorkItem and gl.Epic;
// fields absent on a given source stay at their zero value.
type Output struct {
	toolutil.HintableOutput
	ID           int64              `json:"id"`
	IID          int64              `json:"iid"`
	Type         string             `json:"type"`
	State        string             `json:"state"`
	Status       string             `json:"status,omitempty"`
	Title        string             `json:"title"`
	Description  string             `json:"description,omitempty"`
	WebURL       string             `json:"web_url,omitempty"`
	URL          string             `json:"url,omitempty"`
	GroupID      int64              `json:"group_id,omitempty"`
	ParentID     int64              `json:"parent_id,omitempty"`
	Author       *BasicUserOutput   `json:"author,omitempty"`
	Assignees    []*BasicUserOutput `json:"assignees,omitempty"`
	Labels       []string           `json:"labels,omitempty"`
	LinkedItems  []LinkedItem       `json:"linked_items,omitempty" tier:"ultimate"`
	Children     []ChildItem        `json:"children,omitempty"`
	Confidential bool               `json:"confidential,omitempty"`
	Color        string             `json:"color,omitempty"`

	StartDate               string `json:"start_date,omitempty"`
	StartDateIsFixed        bool   `json:"start_date_is_fixed,omitempty"`
	StartDateFixed          string `json:"start_date_fixed,omitempty"`
	StartDateFromMilestones string `json:"start_date_from_milestones,omitempty"`
	DueDate                 string `json:"due_date,omitempty"`
	DueDateIsFixed          bool   `json:"due_date_is_fixed,omitempty"`
	DueDateFixed            string `json:"due_date_fixed,omitempty"`
	DueDateFromMilestones   string `json:"due_date_from_milestones,omitempty"`

	HealthStatus   string `json:"health_status,omitempty" tier:"ultimate"`
	Weight         *int64 `json:"weight,omitempty" tier:"premium"`
	MilestoneID    *int64 `json:"milestone_id,omitempty"`
	IterationID    *int64 `json:"iteration_id,omitempty" tier:"premium"`
	Upvotes        int64  `json:"upvotes,omitempty"`
	Downvotes      int64  `json:"downvotes,omitempty"`
	UserNotesCount int64  `json:"user_notes_count,omitempty"`
	ParentIID      int64  `json:"parent_iid,omitempty"`
	ParentPath     string `json:"parent_path,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
	ClosedAt       string `json:"closed_at,omitempty"`
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

// LinksItem is the child-epic output for the GetLinks REST endpoint, mirroring
// gl.Epic. Per the 1:1 audit policy it surfaces every gl.Epic field; the author
// is a full nested object.
type LinksItem struct {
	ID           int64            `json:"id"`
	IID          int64            `json:"iid"`
	GroupID      int64            `json:"group_id,omitempty"`
	ParentID     int64            `json:"parent_id,omitempty"`
	Title        string           `json:"title"`
	Description  string           `json:"description,omitempty"`
	State        string           `json:"state"`
	WebURL       string           `json:"web_url,omitempty"`
	URL          string           `json:"url,omitempty"`
	Author       *BasicUserOutput `json:"author,omitempty"`
	Labels       []string         `json:"labels,omitempty"`
	Confidential bool             `json:"confidential,omitempty"`

	StartDate               string `json:"start_date,omitempty"`
	StartDateIsFixed        bool   `json:"start_date_is_fixed,omitempty"`
	StartDateFixed          string `json:"start_date_fixed,omitempty"`
	StartDateFromMilestones string `json:"start_date_from_milestones,omitempty"`
	DueDate                 string `json:"due_date,omitempty"`
	DueDateIsFixed          bool   `json:"due_date_is_fixed,omitempty"`
	DueDateFixed            string `json:"due_date_fixed,omitempty"`
	DueDateFromMilestones   string `json:"due_date_from_milestones,omitempty"`

	Upvotes        int64  `json:"upvotes,omitempty"`
	Downvotes      int64  `json:"downvotes,omitempty"`
	UserNotesCount int64  `json:"user_notes_count,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
	ClosedAt       string `json:"closed_at,omitempty"`
}

// toOutput converts a GitLab Work Item to the epic Output format.
func toOutput(wi *gl.WorkItem) Output {
	out := Output{
		ID:           wi.ID,
		IID:          wi.IID,
		Type:         wi.Type,
		State:        wi.State,
		Title:        wi.Title,
		Description:  wi.Description,
		WebURL:       wi.WebURL,
		Confidential: wi.Confidential,
		Weight:       wi.Weight,
		MilestoneID:  wi.MilestoneID,
		IterationID:  wi.IterationID,
	}
	if wi.Status != nil {
		out.Status = *wi.Status
	}
	out.Author = basicUserFromUser(wi.Author)
	out.Assignees = basicUsersFromUsers(wi.Assignees)
	for _, l := range wi.Labels {
		out.Labels = append(out.Labels, l.Name)
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

// toLinkItem converts a GitLab REST Epic to the LinksItem format.
func toLinkItem(e *gl.Epic) LinksItem {
	return LinksItem{
		ID:                      e.ID,
		IID:                     e.IID,
		GroupID:                 e.GroupID,
		ParentID:                e.ParentID,
		Title:                   e.Title,
		Description:             e.Description,
		State:                   e.State,
		WebURL:                  e.WebURL,
		URL:                     e.URL,
		Author:                  basicUserFromEpicAuthor(e.Author),
		Labels:                  e.Labels,
		Confidential:            e.Confidential,
		StartDate:               formatISODate(e.StartDate),
		StartDateIsFixed:        e.StartDateIsFixed,
		StartDateFixed:          formatISODate(e.StartDateFixed),
		StartDateFromMilestones: formatISODate(e.StartDateFromMilestones),
		DueDate:                 formatISODate(e.DueDate),
		DueDateIsFixed:          e.DueDateIsFixed,
		DueDateFixed:            formatISODate(e.DueDateFixed),
		DueDateFromMilestones:   formatISODate(e.DueDateFromMilestones),
		Upvotes:                 e.Upvotes,
		Downvotes:               e.Downvotes,
		UserNotesCount:          e.UserNotesCount,
		CreatedAt:               toolutil.FormatTimePtr(e.CreatedAt),
		UpdatedAt:               toolutil.FormatTimePtr(e.UpdatedAt),
		ClosedAt:                toolutil.FormatTimePtr(e.ClosedAt),
	}
}

// epicToOutput converts a REST gl.Epic to the epic Output format.
//
// parent_iid stays empty here. GitLab's own epic response carries parent_id
// and parent_iid as two different numbers and gl.Epic declares only the first,
// so filling parent_iid from ParentID published a global ID under the name of
// an internal one, and a caller feeding it back to epic_get looked up an epic
// that was not the parent. parent_id already carries that value.
func epicToOutput(e *gl.Epic) Output {
	return Output{
		ID:                      e.ID,
		IID:                     e.IID,
		Type:                    "Epic",
		State:                   e.State,
		Title:                   e.Title,
		Description:             e.Description,
		WebURL:                  e.WebURL,
		URL:                     e.URL,
		GroupID:                 e.GroupID,
		ParentID:                e.ParentID,
		Author:                  basicUserFromEpicAuthor(e.Author),
		Labels:                  e.Labels,
		Confidential:            e.Confidential,
		StartDate:               formatISODate(e.StartDate),
		StartDateIsFixed:        e.StartDateIsFixed,
		StartDateFixed:          formatISODate(e.StartDateFixed),
		StartDateFromMilestones: formatISODate(e.StartDateFromMilestones),
		DueDate:                 formatISODate(e.DueDate),
		DueDateIsFixed:          e.DueDateIsFixed,
		DueDateFixed:            formatISODate(e.DueDateFixed),
		DueDateFromMilestones:   formatISODate(e.DueDateFromMilestones),
		Upvotes:                 e.Upvotes,
		Downvotes:               e.Downvotes,
		UserNotesCount:          e.UserNotesCount,
		CreatedAt:               toolutil.FormatTimePtr(e.CreatedAt),
		UpdatedAt:               toolutil.FormatTimePtr(e.UpdatedAt),
		ClosedAt:                toolutil.FormatTimePtr(e.ClosedAt),
	}
}

// mapStatusToID maps a human-readable status string to the GitLab WorkItemStatusID.
func mapStatusToID(s string) gl.WorkItemStatusID {
	switch s {
	case "TODO":
		return gl.WorkItemStatusToDo
	case "IN_PROGRESS":
		return gl.WorkItemStatusInProgress
	case "DONE":
		return gl.WorkItemStatusDone
	case "WONT_DO":
		return gl.WorkItemStatusWontDo
	case "DUPLICATE":
		return gl.WorkItemStatusDuplicate
	default:
		return gl.WorkItemStatusID(s)
	}
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
	if usesWorkItemsPath(input) {
		return listWithWorkItems(ctx, client, input)
	}

	opts := buildEpicListOptions(input)
	items, resp, err := client.GL().Epics.ListGroupEpics(input.FullPath, opts, gl.WithContext(ctx))
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
		// status, color, health_status, iteration), so opt into them
		// explicitly or they silently come back empty.
		ReturnedFields: append(append([]string(nil), gl.WorkItemDefaultListFields()...),
			"color", "healthStatus", "iteration", "status", "weight"),
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
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}
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
	epics, _, err := client.GL().Epics.GetEpicLinks(input.FullPath, input.IID, gl.WithContext(ctx))
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
	if input.Status != "" {
		status := mapStatusToID(input.Status)
		opts.Status = &status
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
