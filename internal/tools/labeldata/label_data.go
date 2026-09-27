package labeldata

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Output represents a GitLab label shared by project and group label tools:
// what the SDK's label structs decode, plus the description rendered as
// HTML, which lib/api/entities/label.rb sends on every label and neither
// struct carries, read from the captured response (ADR-0021).
//
// Priority and IsProjectLabel are the two keys lib/api/entities/project_label.rb
// adds to Entities::Label, so only the project label routes send them;
// Entities::GroupLabel adds nothing. Both are pointers so a group label
// publishes neither, and so a project label without a priority publishes no
// priority rather than a zero, which is a priority GitLab accepts.
type Output struct {
	toolutil.HintableOutput
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	Color                  string `json:"color"`
	TextColor              string `json:"text_color"`
	Description            string `json:"description"`
	DescriptionHTML        string `json:"description_html,omitempty"`
	OpenIssuesCount        int64  `json:"open_issues_count"`
	ClosedIssuesCount      int64  `json:"closed_issues_count"`
	OpenMergeRequestsCount int64  `json:"open_merge_requests_count"`
	Priority               *int64 `json:"priority,omitempty"`
	IsProjectLabel         *bool  `json:"is_project_label,omitempty"`
	Subscribed             bool   `json:"subscribed"`
	Archived               bool   `json:"archived"`
}

// ProjectOutput converts a GitLab project label to shared output fields, and
// takes the field the capture read beside the SDK.
func ProjectOutput(label *gl.Label, extra toolutil.LabelExtra) Output {
	if label == nil {
		return Output{}
	}
	out := outputFromFields(labelFields{
		ID:                     label.ID,
		Name:                   label.Name,
		Color:                  label.Color,
		TextColor:              label.TextColor,
		Description:            label.Description,
		DescriptionHTML:        extra.DescriptionHTML,
		OpenIssuesCount:        label.OpenIssuesCount,
		ClosedIssuesCount:      label.ClosedIssuesCount,
		OpenMergeRequestsCount: label.OpenMergeRequestsCount,
		Subscribed:             label.Subscribed,
		Archived:               label.Archived,
	})
	out.Priority = priorityFromNullable(label.Priority)
	out.IsProjectLabel = new(label.IsProjectLabel)
	return out
}

// GroupOutput converts a GitLab group label to shared output fields, and
// takes the field the capture read beside the SDK. It sets neither Priority
// nor IsProjectLabel: the group label routes present Entities::GroupLabel,
// which sends neither, so the SDK struct's copies of them are always empty
// there and publishing them would state a priority and a scope GitLab never
// gave.
func GroupOutput(label *gl.GroupLabel, extra toolutil.LabelExtra) Output {
	if label == nil {
		return Output{}
	}
	return outputFromFields(labelFields{
		ID:                     label.ID,
		Name:                   label.Name,
		Color:                  label.Color,
		TextColor:              label.TextColor,
		Description:            label.Description,
		DescriptionHTML:        extra.DescriptionHTML,
		OpenIssuesCount:        label.OpenIssuesCount,
		ClosedIssuesCount:      label.ClosedIssuesCount,
		OpenMergeRequestsCount: label.OpenMergeRequestsCount,
		Subscribed:             label.Subscribed,
		Archived:               label.Archived,
	})
}

// NewProjectListOptions builds GitLab options for listing project labels.
func NewProjectListOptions(page, perPage int, search string, withCounts, includeAncestorGroups bool) *gl.ListLabelsOptions {
	opts := &gl.ListLabelsOptions{}
	applyCommonListOptions(&opts.ListOptions, page, perPage)
	applyCommonLabelFilters(&opts.Search, &opts.WithCounts, search, withCounts)
	if includeAncestorGroups {
		opts.IncludeAncestorGroups = new(true)
	}
	return opts
}

// NewGroupListOptions builds GitLab options for listing group labels.
func NewGroupListOptions(page, perPage int, search string, withCounts, includeAncestorGroups, includeDescendantGroups, onlyGroupLabels bool) *gl.ListGroupLabelsOptions {
	opts := &gl.ListGroupLabelsOptions{}
	applyCommonListOptions(&opts.ListOptions, page, perPage)
	applyCommonLabelFilters(&opts.Search, &opts.WithCounts, search, withCounts)
	if includeAncestorGroups {
		opts.IncludeAncestorGroups = new(true)
	}
	if includeDescendantGroups {
		opts.IncludeDescendantGroups = new(true)
	}
	if onlyGroupLabels {
		opts.OnlyGroupLabels = new(true)
	}
	return opts
}

// ToMarkdown converts shared label output to the toolutil markdown model.
func ToMarkdown(label Output) toolutil.LabelMarkdown {
	md := toolutil.LabelMarkdown{ID: label.ID, Name: label.Name, Color: label.Color, Description: label.Description, OpenIssuesCount: label.OpenIssuesCount, ClosedIssuesCount: label.ClosedIssuesCount, OpenMergeRequestsCount: label.OpenMergeRequestsCount, IsProjectLabel: isProjectLabel(label), Subscribed: label.Subscribed, Archived: label.Archived}
	if label.Priority != nil {
		md.Priority, md.PrioritySpecified = *label.Priority, true
	}
	return md
}

// isProjectLabel reads the scope a label states, false for a group label,
// which states none.
func isProjectLabel(label Output) bool {
	return label.IsProjectLabel != nil && *label.IsProjectLabel
}

// The copy each scope's label surfaces are rendered with. It lives here, with
// the type, because both label packages alias this one output type: the
// Markdown registry keys them together and keeps whichever init ran first, so
// the formatter that wins has to answer for both scopes and cannot do that
// from copy held in one of them.
var (
	// ProjectMarkdownOptions is the copy of a project label.
	ProjectMarkdownOptions = toolutil.LabelMarkdownOptions{
		DetailTitle:   "Label",
		ListTitle:     "Labels",
		EmptyListText: toolutil.EmptyMessage("labels"),
		DetailHints: []string{
			"Use action 'label_update' to change label name, color, or description",
			"Use action 'label_delete' to remove this label",
		},
		ListHints: []string{
			toolutil.HintPreserveLinks,
			"Use action 'label_get' with a label_id to see label details",
			"Use action 'label_create' to create a new label",
		},
	}

	// GroupMarkdownOptions is the copy of a group label.
	GroupMarkdownOptions = toolutil.LabelMarkdownOptions{
		DetailTitle:   "Group Label",
		ListTitle:     "Group Labels",
		EmptyListText: toolutil.EmptyMessage("group labels"),
		DetailHints: []string{
			"If the workflow asks to fetch/get before update or delete, use the selected tool surface's group-label get action with the same group_id and this label_id next",
			"Use the selected tool surface's group-label update action with the same group_id and this label_id to modify this label",
			"Use the selected tool surface's group-label delete action with the same group_id, this label_id, and explicit confirm=true to remove this label",
			"Use the selected tool surface's group-label subscribe or unsubscribe actions with the same group_id and this label_id to follow or unfollow",
		},
		ListHints: []string{
			toolutil.HintPreserveLinks,
			"Use the selected tool surface's group-label get action with the same group_id and label_id for full details before update/delete workflows",
			"Use the selected tool surface's group-label create action with group_id to add a new group label",
		},
	}
)

// MarkdownOptionsFor picks the copy of the scope a label belongs to, which is
// the label's own answer rather than the calling package's: a project label
// list carries the group labels the project inherits, and a card that titles
// one of those "Label" and points at the project actions names actions that
// cannot touch it.
func MarkdownOptionsFor(label Output) toolutil.LabelMarkdownOptions {
	if isProjectLabel(label) {
		return ProjectMarkdownOptions
	}
	return GroupMarkdownOptions
}

// FormatMarkdown renders one label as the card of one object, with the copy of
// the scope it belongs to. Both label packages register this same rendering
// for the one type they share, so which of their inits ran first no longer
// decides whether a group label is shown as a project one.
func FormatMarkdown(label Output) string {
	return toolutil.FormatLabelMarkdown(ToMarkdown(label), MarkdownOptionsFor(label))
}

type labelFields struct {
	ID                     int64
	Name                   string
	Color                  string
	TextColor              string
	Description            string
	DescriptionHTML        string
	OpenIssuesCount        int64
	ClosedIssuesCount      int64
	OpenMergeRequestsCount int64
	Subscribed             bool
	Archived               bool
}

func outputFromFields(fields labelFields) Output {
	return Output{
		ID:                     fields.ID,
		Name:                   fields.Name,
		Color:                  fields.Color,
		TextColor:              fields.TextColor,
		Description:            fields.Description,
		DescriptionHTML:        fields.DescriptionHTML,
		OpenIssuesCount:        fields.OpenIssuesCount,
		ClosedIssuesCount:      fields.ClosedIssuesCount,
		OpenMergeRequestsCount: fields.OpenMergeRequestsCount,
		Subscribed:             fields.Subscribed,
		Archived:               fields.Archived,
	}
}

// priorityFromNullable reads the priority a project label route sent, nil
// when it sent null or nothing: GitLab accepts zero as a priority, so a
// missing one cannot be published as zero.
func priorityFromNullable(value gl.Nullable[int64]) *int64 {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	return new(value.MustGet())
}

func applyCommonListOptions(opts *gl.ListOptions, page, perPage int) {
	if page > 0 {
		opts.Page = int64(page)
	}
	if perPage > 0 {
		opts.PerPage = int64(perPage)
	}
}

func applyCommonLabelFilters(search **string, withCounts **bool, searchValue string, includeCounts bool) {
	if searchValue != "" {
		*search = new(searchValue)
	}
	if includeCounts {
		*withCounts = new(true)
	}
}
