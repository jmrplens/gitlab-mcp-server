// shapes_test.go covers the resource-event sub-object mirrors, the list-option
// helper, and the metadata decorator branches not exercised by the
// handler-level tests.
package resourceevents

import (
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestFormatTimePtr_NilAndValue verifies formatTimePtr renders nil as "" and a
// non-nil time as RFC 3339.
func TestFormatTimePtr_NilAndValue(t *testing.T) {
	if got := toolutil.FormatTimePtr(nil); got != "" {
		t.Errorf("toolutil.FormatTimePtr(nil) = %q, want empty", got)
	}
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := toolutil.FormatTimePtr(&ts); got != "2026-01-02T03:04:05Z" {
		t.Errorf("formatTimePtr = %q, want RFC3339", got)
	}
}

// TestIterationOutput_FullAndNil verifies iterationOutput maps all timestamp
// fields and returns nil for a nil input.
func TestIterationOutput_FullAndNil(t *testing.T) {
	if iterationOutput(nil) != nil {
		t.Fatal("iterationOutput(nil) should be nil")
	}
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	start := gl.ISOTime(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	due := gl.ISOTime(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	out := iterationOutput(&gl.Iteration{
		ID: 1, IID: 2, Sequence: 3, GroupID: 4, Title: "Sprint", Description: "d",
		State: 1, WebURL: "u", CreatedAt: &created, UpdatedAt: &updated,
		StartDate: &start, DueDate: &due,
	})
	if out.CreatedAt == "" || out.UpdatedAt == "" || out.StartDate != "2026-03-01" || out.DueDate != "2026-04-01" {
		t.Errorf("iterationOutput dates not mapped: %+v", out)
	}
	if out.Description != "d" {
		t.Errorf("Description = %q, want d", out.Description)
	}
}

// TestMilestoneOutput_Nil verifies milestoneOutput returns nil for nil input.
func TestMilestoneOutput_Nil(t *testing.T) {
	if milestoneOutput(nil) != nil {
		t.Fatal("milestoneOutput(nil) should be nil")
	}
}

// TestEventUserOutput_Nil verifies eventUserOutput returns nil for nil input.
func TestEventUserOutput_Nil(t *testing.T) {
	if eventUserOutput(nil) != nil {
		t.Fatal("eventUserOutput(nil) should be nil")
	}
}

// TestMarkdownAccessors_NilBranches verifies the nil-safe markdown accessors.
func TestMarkdownAccessors_NilBranches(t *testing.T) {
	if eventUsername(nil) != "" {
		t.Error("eventUsername(nil) should be empty")
	}
	if labelName(nil) != "" {
		t.Error("labelName(nil) should be empty")
	}
	if milestoneTitle(nil) != "" {
		t.Error("milestoneTitle(nil) should be empty")
	}
	if milestoneID(nil) != 0 {
		t.Error("milestoneID(nil) should be 0")
	}
	if iterationTitle(nil) != "" {
		t.Error("iterationTitle(nil) should be empty")
	}
	if iterationID(nil) != 0 {
		t.Error("iterationID(nil) should be 0")
	}
}

// TestApplyEventListOptions verifies order_by/sort/keyset/offset are copied and
// that a nil options pointer is a no-op.
func TestApplyEventListOptions(t *testing.T) {
	applyEventListOptions(nil, "id", "asc", toolutil.PaginationInput{}, toolutil.KeysetPaginationInput{})

	var opts gl.ListOptions
	applyEventListOptions(&opts, "id", "desc",
		toolutil.PaginationInput{Page: 2, PerPage: 50},
		toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "tok"})
	if opts.OrderBy != "id" || opts.Sort != "desc" {
		t.Errorf("order_by/sort not applied: %+v", opts)
	}
	if opts.Page != 2 || opts.PerPage != 50 {
		t.Errorf("offset pagination not applied: %+v", opts)
	}
	if opts.Pagination != "keyset" || opts.PageToken != "tok" {
		t.Errorf("keyset pagination not applied: %+v", opts)
	}

	var empty gl.ListOptions
	applyEventListOptions(&empty, "", "", toolutil.PaginationInput{}, toolutil.KeysetPaginationInput{})
	if empty.OrderBy != "" || empty.Sort != "" {
		t.Errorf("empty inputs should leave order_by/sort unset: %+v", empty)
	}
}

// TestDecorateEventMeta_NoOp verifies decorateEventMeta leaves options unchanged
// when the tool has no metadata table entry.
func TestDecorateEventMeta_NoOp(t *testing.T) {
	opts := issueEventOptions("gitlab_unknown_tool")
	before := opts.Usage
	decorateEventMeta(&opts, "gitlab_unknown_tool")
	if opts.Usage != before {
		t.Errorf("decorateEventMeta mutated options for unknown tool: %q", opts.Usage)
	}
}
