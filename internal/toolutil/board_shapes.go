// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// BoardUserOutput is the documented 6-field board assignee object shared by
// the project boards and group boards APIs (doc/api/boards.md and
// doc/api/group_boards.md). The documented update-board responses show only
// id, name, username, state, avatar_url, and web_url; gl.BasicUser's
// created_at is not part of the documented board assignee shape.
type BoardUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

// NewBoardUserOutput converts a gl.BasicUser into the documented board
// assignee subset, returning nil when the SDK value is nil.
func NewBoardUserOutput(u *gl.BasicUser) *BoardUserOutput {
	if u == nil {
		return nil
	}
	return &BoardUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL,
	}
}

// BoardListAssigneeOutput is the compact assignee object on a board list
// (Premium/Ultimate assignee list type), matching gl.BoardListAssignee. The
// documented response examples cover only label and milestone lists, so this
// premium list-type sub-object has no fuller documented shape to trim against.
type BoardListAssigneeOutput struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

// NewBoardListAssigneeOutput converts a gl.BoardListAssignee into the shared
// output shape, returning nil when the SDK value is nil.
func NewBoardListAssigneeOutput(a *gl.BoardListAssignee) *BoardListAssigneeOutput {
	if a == nil {
		return nil
	}
	return &BoardListAssigneeOutput{ID: a.ID, Name: a.Name, Username: a.Username}
}

// WorkItemStatusOutput is a work item status as
// ee/lib/api/entities/work_items/status.rb renders it. Every key is exposed
// with expose_nil, so a null description, which a default status always has,
// is an empty string here.
type WorkItemStatusOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	IconName    string `json:"icon_name"`
	Color       string `json:"color"`
	Position    int64  `json:"position"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category"`
}

// NewBoardListStatusOutput converts a gl.BoardListStatus, the status GitLab
// sends on a status list and on no other, returning nil when the SDK value is
// nil.
func NewBoardListStatusOutput(s *gl.BoardListStatus) *WorkItemStatusOutput {
	if s == nil {
		return nil
	}
	return &WorkItemStatusOutput{
		ID: s.ID, Name: s.Name, IconName: s.IconName, Color: s.Color,
		Position: s.Position, Description: s.Description, Category: s.Category,
	}
}

// BoardLabelOutput is the documented board-list label object. Every
// documented board-list response shows the list's `label` object with only
// name, color, and description; gl.Label's remaining fields are not part of
// the documented board-list label shape.
type BoardLabelOutput struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// NewBoardLabelOutput converts a gl.Label into the documented board-list
// label subset, returning nil when the SDK value is nil.
func NewBoardLabelOutput(l *gl.Label) *BoardLabelOutput {
	if l == nil {
		return nil
	}
	return &BoardLabelOutput{Name: l.Name, Color: l.Color, Description: l.Description}
}

// BoardLabelDetailsOutput is the documented board `labels[]` entry. The
// documented update-board responses show only id, name, color, and
// description for each label; the SDK label types' remaining fields are not
// part of the documented board labels shape.
type BoardLabelDetailsOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// NewBoardLabelDetailsOutputs converts a slice of gl.LabelDetails into the
// documented board label subset, skipping nil elements and returning nil
// when no labels remain (consistent with NewCustomAttributeOutputs, so
// callers never serialize an empty array for an effectively-empty input).
func NewBoardLabelDetailsOutputs(details []*gl.LabelDetails) []*BoardLabelDetailsOutput {
	if len(details) == 0 {
		return nil
	}
	out := make([]*BoardLabelDetailsOutput, 0, len(details))
	for _, d := range details {
		if d == nil {
			continue
		}
		out = append(out, &BoardLabelDetailsOutput{
			ID: d.ID, Name: d.Name, Color: d.Color, Description: d.Description,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NewIterationOutputFromProjectIteration converts a gl.ProjectIteration
// pointer (the type surfaced on board lists) into the canonical iteration
// object. gl.ProjectIteration has the same field layout as gl.Iteration but
// with a separate type identity for documentation.
func NewIterationOutputFromProjectIteration(it *gl.ProjectIteration) *IterationOutput {
	if it == nil {
		return nil
	}
	return NewIterationOutput((*gl.Iteration)(it))
}
