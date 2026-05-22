// Package labeldata contains shared GitLab label conversion helpers.
package labeldata

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Output represents a GitLab label shared by project and group label tools.
type Output struct {
	toolutil.HintableOutput
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	Color                  string `json:"color"`
	TextColor              string `json:"text_color"`
	Description            string `json:"description"`
	OpenIssuesCount        int64  `json:"open_issues_count"`
	ClosedIssuesCount      int64  `json:"closed_issues_count"`
	OpenMergeRequestsCount int64  `json:"open_merge_requests_count"`
	Priority               int64  `json:"priority"`
	PrioritySpecified      bool   `json:"-"`
	IsProjectLabel         bool   `json:"is_project_label"`
	Subscribed             bool   `json:"subscribed"`
}

// ProjectOutput converts a GitLab project label to shared output fields.
func ProjectOutput(label *gl.Label) Output {
	if label == nil {
		return Output{}
	}
	return outputFromFields(labelFields{
		ID:                     label.ID,
		Name:                   label.Name,
		Color:                  label.Color,
		TextColor:              label.TextColor,
		Description:            label.Description,
		OpenIssuesCount:        label.OpenIssuesCount,
		ClosedIssuesCount:      label.ClosedIssuesCount,
		OpenMergeRequestsCount: label.OpenMergeRequestsCount,
		Priority:               label.Priority,
		IsProjectLabel:         label.IsProjectLabel,
		Subscribed:             label.Subscribed,
	})
}

// GroupOutput converts a GitLab group label to shared output fields.
func GroupOutput(label *gl.GroupLabel) Output {
	if label == nil {
		return Output{}
	}
	return outputFromFields(labelFields{
		ID:                     label.ID,
		Name:                   label.Name,
		Color:                  label.Color,
		TextColor:              label.TextColor,
		Description:            label.Description,
		OpenIssuesCount:        label.OpenIssuesCount,
		ClosedIssuesCount:      label.ClosedIssuesCount,
		OpenMergeRequestsCount: label.OpenMergeRequestsCount,
		Priority:               label.Priority,
		IsProjectLabel:         label.IsProjectLabel,
		Subscribed:             label.Subscribed,
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
	return toolutil.LabelMarkdown{ID: label.ID, Name: label.Name, Color: label.Color, Description: label.Description, OpenIssuesCount: label.OpenIssuesCount, ClosedIssuesCount: label.ClosedIssuesCount, OpenMergeRequestsCount: label.OpenMergeRequestsCount, Priority: label.Priority, PrioritySpecified: label.PrioritySpecified, IsProjectLabel: label.IsProjectLabel, Subscribed: label.Subscribed}
}

type labelFields struct {
	ID                     int64
	Name                   string
	Color                  string
	TextColor              string
	Description            string
	OpenIssuesCount        int64
	ClosedIssuesCount      int64
	OpenMergeRequestsCount int64
	Priority               gl.Nullable[int64]
	IsProjectLabel         bool
	Subscribed             bool
}

func outputFromFields(fields labelFields) Output {
	priority, prioritySpecified := priorityFromNullable(fields.Priority)
	return Output{
		ID:                     fields.ID,
		Name:                   fields.Name,
		Color:                  fields.Color,
		TextColor:              fields.TextColor,
		Description:            fields.Description,
		OpenIssuesCount:        fields.OpenIssuesCount,
		ClosedIssuesCount:      fields.ClosedIssuesCount,
		OpenMergeRequestsCount: fields.OpenMergeRequestsCount,
		Priority:               priority,
		PrioritySpecified:      prioritySpecified,
		IsProjectLabel:         fields.IsProjectLabel,
		Subscribed:             fields.Subscribed,
	}
}

func priorityFromNullable(value gl.Nullable[int64]) (int64, bool) {
	if !value.IsSpecified() || value.IsNull() {
		return 0, false
	}
	return value.MustGet(), true
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
