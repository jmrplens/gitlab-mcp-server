package workitemsavedviews

import (
	"fmt"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// timeLayouts are the timestamp spellings accepted for the eight time filters.
// RFC 3339 is the documented form; the two shorter layouts exist because a model
// that is told "2025-01-01" is a date will send exactly that, and rejecting it
// would cost a round trip to learn nothing.
var timeLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"}

// Filters mirrors [gl.WorkItemSavedViewFilters], the WorkItemSavedViewFilterInput
// GraphQL input a saved view stores.
//
// Optional scalars are plain strings rather than pointers: the GraphQL input
// omits empty values, so "" and "field absent" mean the same thing to the
// server. Booleans stay pointers, because false and absent do not.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsavedviewfilterinput
type Filters struct {
	AssigneeUsernames          []string            `json:"assignee_usernames,omitempty"            jsonschema:"Usernames of the assignees to match"`
	AssigneeWildcardID         string              `json:"assignee_wildcard_id,omitempty"          jsonschema:"Assignee wildcard filter: ANY or NONE"`
	AuthorUsername             string              `json:"author_username,omitempty"               jsonschema:"Username of the work item author"`
	ClosedAfter                string              `json:"closed_after,omitempty"                  jsonschema:"Match work items closed after this timestamp (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	ClosedBefore               string              `json:"closed_before,omitempty"                 jsonschema:"Match work items closed before this timestamp (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	Confidential               *bool               `json:"confidential,omitempty"                  jsonschema:"Match only confidential (true) or only non-confidential (false) work items"`
	CreatedAfter               string              `json:"created_after,omitempty"                 jsonschema:"Match work items created after this timestamp (ISO 8601)"`
	CreatedBefore              string              `json:"created_before,omitempty"                jsonschema:"Match work items created before this timestamp (ISO 8601)"`
	CRMContactID               string              `json:"crm_contact_id,omitempty"                jsonschema:"CRM contact global ID whose work items to match"`
	CRMOrganizationID          string              `json:"crm_organization_id,omitempty"           jsonschema:"CRM organization global ID whose work items to match"`
	CustomField                []CustomFieldFilter `json:"custom_field,omitempty"                  jsonschema:"Custom field filters, each matching one custom field by ID or name"`
	DueAfter                   string              `json:"due_after,omitempty"                     jsonschema:"Match work items due after this timestamp (ISO 8601)"`
	DueBefore                  string              `json:"due_before,omitempty"                    jsonschema:"Match work items due before this timestamp (ISO 8601)"`
	ExcludeGroupWorkItems      *bool               `json:"exclude_group_work_items,omitempty"      jsonschema:"Exclude work items owned by the group itself"`
	ExcludeProjects            *bool               `json:"exclude_projects,omitempty"              jsonschema:"Exclude work items owned by projects under the namespace"`
	FullPath                   string              `json:"full_path,omitempty"                     jsonschema:"Full path of the project or group whose work items to match"`
	HealthStatusFilter         string              `json:"health_status_filter,omitempty"          jsonschema:"Health status to match: onTrack, needsAttention, atRisk, ANY, or NONE"`
	HierarchyFilters           *HierarchyFilter    `json:"hierarchy_filters,omitempty"             jsonschema:"Filter by position in the work item hierarchy"`
	IID                        string              `json:"iid,omitempty"                           jsonschema:"Internal ID (IID) of a single work item to match"`
	In                         []string            `json:"in,omitempty"                            jsonschema:"Fields the search term is matched against, e.g. TITLE or DESCRIPTION"`
	IncludeDescendantWorkItems *bool               `json:"include_descendant_work_items,omitempty" jsonschema:"Include work items below the matched ones in the hierarchy"`
	IncludeDescendants         *bool               `json:"include_descendants,omitempty"           jsonschema:"Include work items from descendant namespaces"`
	IterationCadenceID         []string            `json:"iteration_cadence_id,omitempty"          jsonschema:"Iteration cadence global IDs to match"`
	IterationID                []string            `json:"iteration_id,omitempty"                  jsonschema:"Iteration global IDs to match"`
	IterationWildcardID        string              `json:"iteration_wildcard_id,omitempty"         jsonschema:"Iteration wildcard filter: NONE, ANY, CURRENT"`
	LabelName                  []string            `json:"label_name,omitempty"                    jsonschema:"Label names to match"`
	MilestoneTitle             []string            `json:"milestone_title,omitempty"               jsonschema:"Milestone titles to match"`
	MilestoneWildcardID        string              `json:"milestone_wildcard_id,omitempty"         jsonschema:"Milestone wildcard filter: NONE, ANY, STARTED, UPCOMING"`
	MyReactionEmoji            string              `json:"my_reaction_emoji,omitempty"             jsonschema:"Emoji the authenticated user reacted with"`
	Not                        *NegatedFilters     `json:"not,omitempty"                           jsonschema:"Values that exclude a work item from the view"`
	Or                         *UnionedFilters     `json:"or,omitempty"                            jsonschema:"Values where matching any one of them includes the work item"`
	ReleaseTag                 []string            `json:"release_tag,omitempty"                   jsonschema:"Release tags to match"`
	ReleaseTagWildcardID       string              `json:"release_tag_wildcard_id,omitempty"       jsonschema:"Release tag wildcard filter: NONE or ANY"`
	Search                     string              `json:"search,omitempty"                        jsonschema:"Free-text search term"`
	State                      string              `json:"state,omitempty"                         jsonschema:"Work item state: opened, closed, or all"`
	Status                     *StatusFilter       `json:"status,omitempty"                        jsonschema:"Filter by the work item status widget value"`
	Subscribed                 string              `json:"subscribed,omitempty"                    jsonschema:"Subscription state of the authenticated user: EXPLICITLY_SUBSCRIBED, EXPLICITLY_UNSUBSCRIBED"`
	Types                      []string            `json:"types,omitempty"                         jsonschema:"Work item type names to match, e.g. ISSUE, TASK, EPIC"`
	UpdatedAfter               string              `json:"updated_after,omitempty"                 jsonschema:"Match work items updated after this timestamp (ISO 8601)"`
	UpdatedBefore              string              `json:"updated_before,omitempty"                jsonschema:"Match work items updated before this timestamp (ISO 8601)"`
	Weight                     string              `json:"weight,omitempty"                        jsonschema:"Weight to match"`
	WeightWildcardID           string              `json:"weight_wildcard_id,omitempty"            jsonschema:"Weight wildcard filter: NONE or ANY"`
	WorkItemTypeIDs            []string            `json:"work_item_type_ids,omitempty"            jsonschema:"Work item type global IDs to match"`
}

// NegatedFilters mirrors [gl.WorkItemSavedViewNegatedFilters], the "not"
// sub-filter: a work item matching any of these values is excluded.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsavedviewnegatedfilterinput
type NegatedFilters struct {
	AssigneeUsernames   []string            `json:"assignee_usernames,omitempty"    jsonschema:"Assignee usernames to exclude"`
	AuthorUsername      []string            `json:"author_username,omitempty"       jsonschema:"Author usernames to exclude"`
	CustomField         []CustomFieldFilter `json:"custom_field,omitempty"          jsonschema:"Custom field values to exclude"`
	HealthStatusFilter  []string            `json:"health_status_filter,omitempty"  jsonschema:"Health statuses to exclude"`
	IterationID         []string            `json:"iteration_id,omitempty"          jsonschema:"Iteration global IDs to exclude"`
	IterationWildcardID string              `json:"iteration_wildcard_id,omitempty" jsonschema:"Iteration wildcard filter to exclude"`
	LabelName           []string            `json:"label_name,omitempty"            jsonschema:"Label names to exclude"`
	MilestoneTitle      []string            `json:"milestone_title,omitempty"       jsonschema:"Milestone titles to exclude"`
	MilestoneWildcardID string              `json:"milestone_wildcard_id,omitempty" jsonschema:"Milestone wildcard filter to exclude"`
	MyReactionEmoji     string              `json:"my_reaction_emoji,omitempty"     jsonschema:"Emoji reaction to exclude"`
	ParentIDs           []string            `json:"parent_ids,omitempty"            jsonschema:"Parent work item global IDs to exclude"`
	ReleaseTag          []string            `json:"release_tag,omitempty"           jsonschema:"Release tags to exclude"`
	Types               []string            `json:"types,omitempty"                 jsonschema:"Work item type names to exclude"`
	Weight              string              `json:"weight,omitempty"                jsonschema:"Weight to exclude"`
	WorkItemTypeIDs     []string            `json:"work_item_type_ids,omitempty"    jsonschema:"Work item type global IDs to exclude"`
}

// UnionedFilters mirrors [gl.WorkItemSavedViewUnionedFilters], the "or"
// sub-filter: a work item matching any of these values is included.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsavedviewunionedfilterinput
type UnionedFilters struct {
	AssigneeUsernames []string            `json:"assignee_usernames,omitempty" jsonschema:"Assignee usernames where matching any one includes the work item"`
	AuthorUsernames   []string            `json:"author_usernames,omitempty"   jsonschema:"Author usernames where matching any one includes the work item"`
	CustomField       []CustomFieldFilter `json:"custom_field,omitempty"       jsonschema:"Custom field values where matching any one includes the work item"`
	LabelNames        []string            `json:"label_names,omitempty"        jsonschema:"Label names where matching any one includes the work item"`
}

// HierarchyFilter mirrors [gl.WorkItemHierarchyFilter], which filters work
// items by their position in the work item hierarchy.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#hierarchyfilterinput
type HierarchyFilter struct {
	ParentIDs                  []string `json:"parent_ids,omitempty"                    jsonschema:"Parent work item global IDs"`
	IncludeDescendantWorkItems *bool    `json:"include_descendant_work_items,omitempty" jsonschema:"Include work items below the matched parents"`
	ParentWildcardID           string   `json:"parent_wildcard_id,omitempty"            jsonschema:"Parent wildcard filter: NONE or ANY"`
}

// StatusFilter mirrors [gl.WorkItemStatusFilter], which filters work items by
// their status widget value. Supply the status ID or its name, not both.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetstatusfilterinput
type StatusFilter struct {
	ID   string `json:"id,omitempty"   jsonschema:"Status global ID"`
	Name string `json:"name,omitempty" jsonschema:"Status name, e.g. To do or In progress"`
}

// CustomFieldFilter mirrors [gl.WorkItemCustomFieldFilter], which filters work
// items by one custom field value.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemwidgetcustomfieldfilterinputtype
type CustomFieldFilter struct {
	CustomFieldID        string   `json:"custom_field_id,omitempty"        jsonschema:"Custom field global ID"`
	CustomFieldName      string   `json:"custom_field_name,omitempty"      jsonschema:"Custom field name"`
	SelectedOptionIDs    []string `json:"selected_option_ids,omitempty"    jsonschema:"Selected option global IDs to match"`
	SelectedOptionValues []string `json:"selected_option_values,omitempty" jsonschema:"Selected option values to match"`
}

// toSDK converts f into the SDK filter struct. The eight time filters are
// parsed here rather than at the field level so a malformed timestamp names the
// field it came from.
func (f *Filters) toSDK() (*gl.WorkItemSavedViewFilters, error) {
	if f == nil {
		return nil, nil //nolint:nilnil // no filters and no error is "leave the stored filters alone"; see the doc comment
	}
	out := &gl.WorkItemSavedViewFilters{
		AssigneeUsernames:          f.AssigneeUsernames,
		AssigneeWildcardID:         optionalString(f.AssigneeWildcardID),
		AuthorUsername:             optionalString(f.AuthorUsername),
		Confidential:               f.Confidential,
		CRMContactID:               optionalString(f.CRMContactID),
		CRMOrganizationID:          optionalString(f.CRMOrganizationID),
		CustomField:                customFieldsToSDK(f.CustomField),
		ExcludeGroupWorkItems:      f.ExcludeGroupWorkItems,
		ExcludeProjects:            f.ExcludeProjects,
		FullPath:                   optionalString(f.FullPath),
		HealthStatusFilter:         optionalString(f.HealthStatusFilter),
		HierarchyFilters:           f.HierarchyFilters.toSDK(),
		IID:                        optionalString(f.IID),
		In:                         f.In,
		IncludeDescendantWorkItems: f.IncludeDescendantWorkItems,
		IncludeDescendants:         f.IncludeDescendants,
		IterationCadenceID:         f.IterationCadenceID,
		IterationID:                f.IterationID,
		IterationWildcardID:        optionalString(f.IterationWildcardID),
		LabelName:                  f.LabelName,
		MilestoneTitle:             f.MilestoneTitle,
		MilestoneWildcardID:        optionalString(f.MilestoneWildcardID),
		MyReactionEmoji:            optionalString(f.MyReactionEmoji),
		Not:                        f.Not.toSDK(),
		Or:                         f.Or.toSDK(),
		ReleaseTag:                 f.ReleaseTag,
		ReleaseTagWildcardID:       optionalString(f.ReleaseTagWildcardID),
		Search:                     optionalString(f.Search),
		State:                      optionalString(f.State),
		Status:                     f.Status.toSDK(),
		Subscribed:                 optionalString(f.Subscribed),
		Types:                      f.Types,
		Weight:                     optionalString(f.Weight),
		WeightWildcardID:           optionalString(f.WeightWildcardID),
		WorkItemTypeIDs:            f.WorkItemTypeIDs,
	}

	times := []struct {
		field string
		value string
		dest  **time.Time
	}{
		{"filters.closed_after", f.ClosedAfter, &out.ClosedAfter},
		{"filters.closed_before", f.ClosedBefore, &out.ClosedBefore},
		{"filters.created_after", f.CreatedAfter, &out.CreatedAfter},
		{"filters.created_before", f.CreatedBefore, &out.CreatedBefore},
		{"filters.due_after", f.DueAfter, &out.DueAfter},
		{"filters.due_before", f.DueBefore, &out.DueBefore},
		{"filters.updated_after", f.UpdatedAfter, &out.UpdatedAfter},
		{"filters.updated_before", f.UpdatedBefore, &out.UpdatedBefore},
	}
	for _, t := range times {
		parsed, err := parseTimestamp(t.field, t.value)
		if err != nil {
			return nil, err
		}
		*t.dest = parsed
	}
	return out, nil
}

// toSDK converts the "not" sub-filter, returning nil for a nil receiver so the
// caller needs no separate check.
func (f *NegatedFilters) toSDK() *gl.WorkItemSavedViewNegatedFilters {
	if f == nil {
		return nil
	}
	return &gl.WorkItemSavedViewNegatedFilters{
		AssigneeUsernames:   f.AssigneeUsernames,
		AuthorUsername:      f.AuthorUsername,
		CustomField:         customFieldsToSDK(f.CustomField),
		HealthStatusFilter:  f.HealthStatusFilter,
		IterationID:         f.IterationID,
		IterationWildcardID: optionalString(f.IterationWildcardID),
		LabelName:           f.LabelName,
		MilestoneTitle:      f.MilestoneTitle,
		MilestoneWildcardID: optionalString(f.MilestoneWildcardID),
		MyReactionEmoji:     optionalString(f.MyReactionEmoji),
		ParentIDs:           f.ParentIDs,
		ReleaseTag:          f.ReleaseTag,
		Types:               f.Types,
		Weight:              optionalString(f.Weight),
		WorkItemTypeIDs:     f.WorkItemTypeIDs,
	}
}

// toSDK converts the "or" sub-filter, returning nil for a nil receiver.
func (f *UnionedFilters) toSDK() *gl.WorkItemSavedViewUnionedFilters {
	if f == nil {
		return nil
	}
	return &gl.WorkItemSavedViewUnionedFilters{
		AssigneeUsernames: f.AssigneeUsernames,
		AuthorUsernames:   f.AuthorUsernames,
		CustomField:       customFieldsToSDK(f.CustomField),
		LabelNames:        f.LabelNames,
	}
}

// toSDK converts the hierarchy sub-filter, returning nil for a nil receiver.
func (f *HierarchyFilter) toSDK() *gl.WorkItemHierarchyFilter {
	if f == nil {
		return nil
	}
	return &gl.WorkItemHierarchyFilter{
		ParentIDs:                  f.ParentIDs,
		IncludeDescendantWorkItems: f.IncludeDescendantWorkItems,
		ParentWildcardID:           optionalString(f.ParentWildcardID),
	}
}

// toSDK converts the status sub-filter, returning nil for a nil receiver.
func (f *StatusFilter) toSDK() *gl.WorkItemStatusFilter {
	if f == nil {
		return nil
	}
	return &gl.WorkItemStatusFilter{
		ID:   optionalString(f.ID),
		Name: optionalString(f.Name),
	}
}

// customFieldsToSDK converts the custom field filter list, preserving a nil
// slice as nil so the GraphQL input omits the key entirely.
func customFieldsToSDK(in []CustomFieldFilter) []gl.WorkItemCustomFieldFilter {
	if in == nil {
		return nil
	}
	out := make([]gl.WorkItemCustomFieldFilter, 0, len(in))
	for _, f := range in {
		out = append(out, gl.WorkItemCustomFieldFilter{
			CustomFieldID:        optionalString(f.CustomFieldID),
			CustomFieldName:      optionalString(f.CustomFieldName),
			SelectedOptionIDs:    f.SelectedOptionIDs,
			SelectedOptionValues: f.SelectedOptionValues,
		})
	}
	return out
}

// optionalString returns nil for an empty string so the GraphQL input omits the
// key, and a pointer to the value otherwise.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// parseTimestamp parses one filter timestamp, accepting the layouts in
// [timeLayouts]. An empty value yields a nil time rather than an error, because
// every time filter is optional.
func parseTimestamp(field, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil //nolint:nilnil // an unset optional timestamp is not an error
	}
	for _, layout := range timeLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("%s must be an ISO 8601 timestamp (e.g. 2025-01-01T00:00:00Z), got %q", field, value)
}
