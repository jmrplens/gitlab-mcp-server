// markdown_test.go contains unit tests for group SCIM Markdown formatting functions.
package groupscim

import (
	"strings"
	"testing"
)

// TestFormatOutputMarkdown verifies single SCIM identity rendering covers
// all output fields (external UID, user ID, active status) and the empty case.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		input     Output
		contains  []string
		wantEmpty bool
	}{
		{
			name: "active identity with all fields",
			input: Output{
				ExternalUID: "ext-uid-123",
				UserID:      42,
				Active:      true,
			},
			contains: []string{
				"SCIM Identity",
				"ext-uid-123",
				"42",
				"true",
				"gitlab_update_group_scim_identity",
				"gitlab_delete_group_scim_identity",
			},
		},
		{
			name: "inactive identity",
			input: Output{
				ExternalUID: "ext-uid-456",
				UserID:      99,
				Active:      false,
			},
			contains: []string{
				"ext-uid-456",
				"99",
				"false",
			},
		},
		{
			name:      "zero-value identity returns empty",
			input:     Output{},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty string, got:\n%s", got)
				}
				return
			}
			if got == "" {
				t.Fatal("expected non-empty markdown, got empty string")
			}
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatListMarkdown verifies the ListMarkdown Markdown formatter for a representative list input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
	}{
		{
			name:     "empty list",
			input:    ListOutput{},
			contains: []string{"No SCIM identities found"},
		},
		{
			name: "empty identities slice",
			input: ListOutput{
				Identities: []Output{},
			},
			contains: []string{"No SCIM identities found"},
		},
		{
			name: "single identity",
			input: ListOutput{
				Identities: []Output{
					{ExternalUID: "uid-1", UserID: 10, Active: true},
				},
			},
			contains: []string{
				"SCIM Identities (1)",
				"External UID",
				"User ID",
				"Active",
				"uid-1",
				"10",
				"true",
				"gitlab_get_group_scim_identity",
			},
		},
		{
			name: "multiple identities",
			input: ListOutput{
				Identities: []Output{
					{ExternalUID: "uid-1", UserID: 10, Active: true},
					{ExternalUID: "uid-2", UserID: 20, Active: false},
					{ExternalUID: "uid-3", UserID: 30, Active: true},
				},
			},
			contains: []string{
				"SCIM Identities (3)",
				"uid-1", "uid-2", "uid-3",
				"10", "20", "30",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			if got == "" {
				t.Fatal("expected non-empty markdown, got empty string")
			}
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}

// TestToOutput_Nil verifies the ToOutput_Nil handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToOutput_Nil(t *testing.T) {
	out := toOutput(nil)
	if out.ExternalUID != "" {
		t.Errorf("expected empty ExternalUID, got %q", out.ExternalUID)
	}
	if out.UserID != 0 {
		t.Errorf("expected UserID 0, got %d", out.UserID)
	}
	if out.Active {
		t.Error("expected Active false, got true")
	}
}

// TestFormatOutputMarkdown_EmptyAndPopulated verifies the single-identity
// formatter returns an empty string for a zero-value identity (the
// nothing-to-render guard) and renders UID/UserID/Active for a populated one.
func TestFormatOutputMarkdown_EmptyAndPopulated(t *testing.T) {
	if got := FormatOutputMarkdown(Output{}); got != "" {
		t.Errorf("FormatOutputMarkdown(zero) = %q, want empty", got)
	}
	got := FormatOutputMarkdown(Output{ExternalUID: "uid-1", UserID: 7, Active: true})
	for _, want := range []string{"## SCIM Identity", "`uid-1`", "**User ID**: 7", "**Active**: true"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("FormatOutputMarkdown missing %q in:\n%s", want, got)
			}
		})
	}
}

// TestFormatListMarkdown_EmptyAndRows verifies the list formatter emits the
// no-identities message for an empty list and a table row per identity plus
// the count header otherwise.
func TestFormatListMarkdown_EmptyAndRows(t *testing.T) {
	if got := FormatListMarkdown(ListOutput{}); got != "No SCIM identities found." {
		t.Errorf("FormatListMarkdown(empty) = %q", got)
	}
	got := FormatListMarkdown(ListOutput{Identities: []Output{
		{ExternalUID: "a", UserID: 1, Active: true},
		{ExternalUID: "b", UserID: 2},
	}})
	for _, want := range []string{"## SCIM Identities (2)", "| `a` | 1 | true |", "| `b` | 2 | false |"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("FormatListMarkdown missing %q in:\n%s", want, got)
			}
		})
	}
}

// TestFormatUpdateMarkdown_RendersConfirmation verifies the update formatter
// surfaces the Updated flag and message text.
func TestFormatUpdateMarkdown_RendersConfirmation(t *testing.T) {
	got := FormatUpdateMarkdown(UpdateOutput{Updated: true, Message: "ok"})
	for _, want := range []string{"## SCIM Identity Updated", "**Updated**: true", "**Message**: ok"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("FormatUpdateMarkdown missing %q in:\n%s", want, got)
			}
		})
	}
}
