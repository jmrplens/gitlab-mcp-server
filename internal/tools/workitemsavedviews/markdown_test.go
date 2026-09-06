// markdown_test.go contains unit tests for the Markdown renderings of work item
// saved views, covering the detail, list, and mutation shapes plus the empty and
// undecodable edge cases.
package workitemsavedviews

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestFormatGetMarkdown verifies that the detail rendering carries the scalar
// fields and pretty-prints both opaque JSON scalars.
func TestFormatGetMarkdown(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{
		NamespacePath: "my-group",
		SavedView: Item{
			ID:              7,
			GID:             "gid://gitlab/WorkItems::SavedViews::SavedView/7",
			Name:            "My open tasks",
			Description:     "Everything assigned to me",
			IsPrivate:       true,
			Subscribed:      true,
			Sort:            "CREATED_DESC",
			Filters:         map[string]any{"assigneeUsernames": []any{"alice"}},
			DisplaySettings: map[string]any{"viewMode": "board"},
		},
	})
	for _, want := range []string{
		"Saved View: My open tasks",
		"gid://gitlab/WorkItems::SavedViews::SavedView/7",
		"Everything assigned to me",
		"CREATED_DESC",
		"### Filters",
		"assigneeUsernames",
		"### Display Settings",
		"board",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatGetMarkdown_WithoutOpaqueScalars verifies that the two optional
// sections are omitted when the API returned neither, which is what a view with
// no filters and no display settings looks like.
func TestFormatGetMarkdown_WithoutOpaqueScalars(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{SavedView: Item{ID: 1, Name: "Bare"}})
	for _, absent := range []string{"### Filters", "### Display Settings", "Global ID"} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(md, absent) {
				t.Errorf("markdown should omit %q:\n%s", absent, md)
			}
		})
	}
}

// TestFormatListMarkdown verifies the table rendering, the cursor line, and the
// hint pointing at get for the filters the list omits.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		NamespacePath: "my-group",
		SavedViews: []Item{
			{ID: 7, Name: "My open tasks", IsPrivate: true, Subscribed: false, Sort: "CREATED_DESC", Description: "Mine"},
			{ID: 8, Name: "Team backlog", IsPrivate: false, Subscribed: true, Sort: "TITLE_ASC"},
		},
		Pagination: toolutil.GraphQLPaginationOutput{HasNextPage: true, EndCursor: "CURSOR"},
	})
	for _, want := range []string{
		"Saved Views: my-group",
		"| 7 | My open tasks | true | false | CREATED_DESC | Mine |",
		"| 8 | Team backlog | false | true | TITLE_ASC |  |",
		"Next page cursor: `CURSOR`",
		"work_item_saved_view.get",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatListMarkdown_Empty verifies the empty rendering says so rather than
// emitting a header with no rows.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{NamespacePath: "my-group"})
	if !strings.Contains(md, "No saved views found.") {
		t.Errorf("markdown = %q, want the empty message", md)
	}
	if strings.Contains(md, "Next page cursor") {
		t.Errorf("markdown should not offer a cursor:\n%s", md)
	}
}

// TestFormatMutateMarkdown verifies that the mutation confirmation carries the
// message and the resulting view.
func TestFormatMutateMarkdown(t *testing.T) {
	md := FormatMutateMarkdown(MutateOutput{
		Status:    "success",
		Message:   "Successfully created saved view \"My open tasks\".",
		SavedView: Item{ID: 7, Name: "My open tasks", Sort: "CREATED_DESC"},
	})
	for _, want := range []string{"Successfully created saved view", "**ID**: 7", "CREATED_DESC"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestPrettyJSON_Unmarshalable verifies that a value json cannot marshal falls
// back to its Go rendering instead of producing an empty section.
func TestPrettyJSON_Unmarshalable(t *testing.T) {
	if prettyJSON(make(chan int)) == "" {
		t.Error("prettyJSON() = empty, want a fallback rendering")
	}
}

// TestMarkdownFormattersRegistered verifies that every output type resolves
// through the shared registry, which is how the surfaces reach these renderers.
func TestMarkdownFormattersRegistered(t *testing.T) {
	outputs := map[string]any{
		"get":    GetOutput{SavedView: Item{ID: 1, Name: "n"}},
		"list":   ListOutput{NamespacePath: "g"},
		"mutate": MutateOutput{Message: "done"},
	}
	for name, out := range outputs {
		t.Run(name, func(t *testing.T) {
			if toolutil.MarkdownForResult(out) == nil {
				t.Errorf("MarkdownForResult(%T) = nil, want a registered formatter", out)
			}
		})
	}
}
