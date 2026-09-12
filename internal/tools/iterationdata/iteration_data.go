package iterationdata

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Output represents a GitLab iteration shared by project and group tools.
type Output struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	Sequence    int64  `json:"sequence"`
	GroupID     int64  `json:"group_id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	State       int64  `json:"state"`
	WebURL      string `json:"web_url,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// GroupOutput converts a GitLab group iteration to shared output fields.
func GroupOutput(it *gl.GroupIteration) Output {
	if it == nil {
		return Output{}
	}
	return outputFromFields(iterationFields{
		ID:          it.ID,
		IID:         it.IID,
		Sequence:    it.Sequence,
		GroupID:     it.GroupID,
		Title:       it.Title,
		Description: it.Description,
		State:       it.State,
		WebURL:      it.WebURL,
		StartDate:   it.StartDate,
		DueDate:     it.DueDate,
		CreatedAt:   it.CreatedAt,
		UpdatedAt:   it.UpdatedAt,
	})
}

// ProjectOutput converts a GitLab project iteration to shared output fields.
func ProjectOutput(it *gl.ProjectIteration) Output {
	if it == nil {
		return Output{}
	}
	return outputFromFields(iterationFields{
		ID:          it.ID,
		IID:         it.IID,
		Sequence:    it.Sequence,
		GroupID:     it.GroupID,
		Title:       it.Title,
		Description: it.Description,
		State:       it.State,
		WebURL:      it.WebURL,
		StartDate:   it.StartDate,
		DueDate:     it.DueDate,
		CreatedAt:   it.CreatedAt,
		UpdatedAt:   it.UpdatedAt,
	})
}

// NewGroupListOptions builds GitLab options for listing group iterations.
func NewGroupListOptions(page, perPage int, state, search string, includeAncestors bool) *gl.ListGroupIterationsOptions {
	opts := &gl.ListGroupIterationsOptions{ListOptions: listOptions(page, perPage)}
	applyGroupListFilters(opts, state, search, includeAncestors)
	return opts
}

// NewProjectListOptions builds GitLab options for listing project iterations.
func NewProjectListOptions(page, perPage int, state, search string, includeAncestors bool) *gl.ListProjectIterationsOptions {
	opts := &gl.ListProjectIterationsOptions{ListOptions: listOptions(page, perPage)}
	applyProjectListFilters(opts, state, search, includeAncestors)
	return opts
}

type iterationFields struct {
	ID          int64
	IID         int64
	Sequence    int64
	GroupID     int64
	Title       string
	Description string
	State       int64
	WebURL      string
	StartDate   *gl.ISOTime
	DueDate     *gl.ISOTime
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
}

func outputFromFields(fields iterationFields) Output {
	out := Output{
		ID:          fields.ID,
		IID:         fields.IID,
		Sequence:    fields.Sequence,
		GroupID:     fields.GroupID,
		Title:       fields.Title,
		Description: fields.Description,
		State:       fields.State,
		WebURL:      fields.WebURL,
	}
	if fields.StartDate != nil {
		out.StartDate = fields.StartDate.String()
	}
	if fields.DueDate != nil {
		out.DueDate = fields.DueDate.String()
	}
	// The wire form is RFC 3339 in UTC. The layout this used to format with
	// ended in a literal Z, which stamped whatever wall clock the value carried
	// with a zone it may not have been in.
	out.CreatedAt = toolutil.RFC3339Ptr(fields.CreatedAt)
	out.UpdatedAt = toolutil.RFC3339Ptr(fields.UpdatedAt)
	return out
}

func listOptions(page, perPage int) gl.ListOptions {
	return gl.ListOptions{Page: int64(page), PerPage: int64(perPage)}
}

func applyGroupListFilters(opts *gl.ListGroupIterationsOptions, state, search string, includeAncestors bool) {
	if state != "" {
		opts.State = &state
	}
	if search != "" {
		opts.Search = &search
	}
	if includeAncestors {
		opts.IncludeAncestors = new(true)
	}
}

func applyProjectListFilters(opts *gl.ListProjectIterationsOptions, state, search string, includeAncestors bool) {
	if state != "" {
		opts.State = &state
	}
	if search != "" {
		opts.Search = &search
	}
	if includeAncestors {
		opts.IncludeAncestors = new(true)
	}
}
