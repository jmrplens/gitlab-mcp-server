package iterationdata

import (
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestFormatListMarkdown_Empty verifies an empty page renders the scope's
// one-sentence empty message and nothing else: no heading counting zero, no
// table header with no rows under it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := FormatListMarkdown("Iterations", toolutil.EmptyMessage("iterations"), nil, toolutil.PaginationOutput{})
	if want := "No iterations found.\n"; out != want {
		t.Errorf("FormatListMarkdown() = %q, want %q", out, want)
	}
}

// TestFormatListMarkdown_WithData pins the whole list render: the counted
// heading, one table with the title linked to the iteration, GitLab's own
// state words, the display form of both dates, the pagination line, and the
// guidance section last with the preserve-links hint leading the caller's.
func TestFormatListMarkdown_WithData(t *testing.T) {
	iterations := []Output{
		{ID: 1, IID: 10, Title: "Sprint 1", State: 1, StartDate: "2026-03-01", DueDate: "2026-03-14", WebURL: "https://gitlab.example.com/-/iterations/1"},
		{ID: 2, IID: 11, Title: "Sprint 2", State: 2, WebURL: "https://gitlab.example.com/-/iterations/2"},
	}
	pagination := toolutil.PaginationOutput{TotalItems: 2, TotalPages: 1}

	out := FormatListMarkdown("Group Iterations", toolutil.EmptyMessage("group iterations"), iterations, pagination, "Use action 'issue.iteration_list_group' to page through the rest")

	want := "## Group Iterations (2)\n\n" +
		"| ID | IID | Title | State | Start | Due |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | 10 | [Sprint 1](https://gitlab.example.com/-/iterations/1) | upcoming | 1 Mar 2026 | 14 Mar 2026 |\n" +
		"| 2 | 11 | [Sprint 2](https://gitlab.example.com/-/iterations/2) | current |  |  |\n" +
		"\n2 items total\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'issue.iteration_list_group' to page through the rest\n"
	if out != want {
		t.Errorf("FormatListMarkdown()\n got %q\nwant %q", out, want)
	}
}

// TestFormatListMarkdown_IterationWithoutTitle_LinksItsOwnAddress pins the cell
// an untitled iteration renders. GitLab lets an iteration carry no title, and a
// row whose link text is empty gives a reader nothing to click and no way to
// tell two such rows apart, so the address — the one identity such an iteration
// still has — becomes the text. A title of nothing but whitespace reads the
// same to anyone looking at the table, which is why the fallback trims before
// deciding rather than comparing against the empty string.
func TestFormatListMarkdown_IterationWithoutTitle_LinksItsOwnAddress(t *testing.T) {
	const webURL = "https://gitlab.example.com/-/iterations/7"
	tests := []struct {
		name  string
		title string
	}{
		{"no_title", ""},
		{"whitespace_only_title", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iterations := []Output{{ID: 7, IID: 3, Title: tt.title, State: 2, WebURL: webURL}}

			out := FormatListMarkdown("Iterations", toolutil.EmptyMessage("iterations"), iterations, toolutil.PaginationOutput{TotalItems: 1, TotalPages: 1})

			wantRow := "| 7 | 3 | [" + webURL + "](" + webURL + ") | current |  |  |\n"
			if !strings.Contains(out, wantRow) {
				t.Errorf("FormatListMarkdown() = %q, want it to contain the row %q", out, wantRow)
			}
		})
	}
}

// TestFormatOutputMarkdown_WithAllFields pins the whole card: the heading the
// formatter composes, one list item per field in the order written, the
// description as the card's text, and no table row anywhere.
func TestFormatOutputMarkdown_WithAllFields(t *testing.T) {
	output := Output{
		ID:          5,
		IID:         20,
		Sequence:    3,
		Title:       "Sprint 5",
		State:       3,
		GroupID:     100,
		StartDate:   "2026-03-01",
		DueDate:     "2026-03-14",
		WebURL:      "https://gitlab.example.com/-/iterations/5",
		CreatedAt:   "2026-02-01T09:00:00Z",
		UpdatedAt:   "2026-02-02T10:30:00Z",
		Description: "A description with **markdown**",
	}

	out := FormatOutputMarkdown(output)

	want := "## Iteration #20: Sprint 5\n\n" +
		"- **ID**: 5\n" +
		"- **IID**: 20\n" +
		"- **Sequence**: 3\n" +
		"- **State**: closed\n" +
		"- **Group ID**: 100\n" +
		"- **Start**: 1 Mar 2026\n" +
		"- **Due**: 14 Mar 2026\n" +
		"- **URL**: [https://gitlab.example.com/-/iterations/5](https://gitlab.example.com/-/iterations/5)\n" +
		"- **Created**: 1 Feb 2026 09:00 UTC\n" +
		"- **Updated**: 2 Feb 2026 10:30 UTC\n" +
		"- **Description**: A description with **markdown**\n"
	if out != want {
		t.Errorf("FormatOutputMarkdown()\n got %q\nwant %q", out, want)
	}
}

// TestFormatOutputMarkdown_WithHint pins the whole card of an iteration GitLab
// sent almost nothing for: every absent field writes no row, and the caller's
// hint closes the response as the guidance section.
func TestFormatOutputMarkdown_WithHint(t *testing.T) {
	out := FormatOutputMarkdown(Output{ID: 1, IID: 1, Title: "Test"}, "custom hint")

	want := "## Iteration #1: Test\n\n" +
		"- **ID**: 1\n" +
		"- **IID**: 1\n" +
		"- **State**: unknown(0)\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- custom hint\n"
	if out != want {
		t.Errorf("FormatOutputMarkdown()\n got %q\nwant %q", out, want)
	}
}

// TestStateName_AllStates verifies StateName maps GitLab's own iteration state
// enum: 1 upcoming, 2 current, 3 closed, anything else unknown. "opened" is a
// list filter meaning upcoming plus current, not a state GitLab stores, and
// naming 1 that way shifted every later value by one.
func TestStateName_AllStates(t *testing.T) {
	tests := []struct {
		name     string
		state    int64
		expected string
	}{
		{"upcoming", 1, "upcoming"},
		{"current", 2, "current"},
		{"closed", 3, "closed"},
		{"all_filter_is_not_a_state", 4, "unknown(4)"},
		{"zero_unknown", 0, "unknown(0)"},
		{"out_of_range_unknown", 99, "unknown(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StateName(tt.state)
			if got != tt.expected {
				t.Errorf("StateName(%d) = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

// TestOutputConverters_MapSharedFields verifies project and group iteration
// converters preserve all common fields and format date fields consistently.
func TestOutputConverters_MapSharedFields(t *testing.T) {
	now := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	start := gl.ISOTime(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	due := gl.ISOTime(time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))

	project := ProjectOutput(&gl.ProjectIteration{ID: 42, IID: 7, Sequence: 3, GroupID: 10, Title: "Sprint", Description: "Desc", State: 2, WebURL: "https://example.test/it/42", StartDate: &start, DueDate: &due, CreatedAt: &now, UpdatedAt: &now})
	group := GroupOutput(&gl.GroupIteration{ID: 42, IID: 7, Sequence: 3, GroupID: 10, Title: "Sprint", Description: "Desc", State: 2, WebURL: "https://example.test/it/42", StartDate: &start, DueDate: &due, CreatedAt: &now, UpdatedAt: &now})

	if project != group {
		t.Fatalf("ProjectOutput() = %+v, GroupOutput() = %+v, want equal shared fields", project, group)
	}
	if project.StartDate == "" || project.DueDate == "" || project.CreatedAt == "" || project.UpdatedAt == "" {
		t.Fatalf("converted dates = %+v, want all date fields populated", project)
	}
}

// TestOutputConverters_IterationWithoutDates_LeavesTheDateFieldsEmpty pins what
// both converters publish for an iteration GitLab dated in no way at all. An
// iteration may carry no cadence, so GitLab omits start_date and due_date and
// client-go hands over nil pointers; the four date fields must then be empty
// strings while every other field still arrives. The whole struct is compared
// rather than the four dates, because a converter that answered a dateless
// iteration with a zero Output — which is what reading one of those pointers
// the wrong way round would produce once the panic was guarded — satisfies the
// empty halves and publishes nothing a caller can use.
func TestOutputConverters_IterationWithoutDates_LeavesTheDateFieldsEmpty(t *testing.T) {
	project := ProjectOutput(&gl.ProjectIteration{ID: 42, IID: 7, Sequence: 3, GroupID: 10, Title: "Sprint", Description: "Desc", State: 2, WebURL: "https://example.test/it/42"})
	group := GroupOutput(&gl.GroupIteration{ID: 42, IID: 7, Sequence: 3, GroupID: 10, Title: "Sprint", Description: "Desc", State: 2, WebURL: "https://example.test/it/42"})

	if project != group {
		t.Fatalf("ProjectOutput() = %+v, GroupOutput() = %+v, want equal shared fields", project, group)
	}
	want := Output{ID: 42, IID: 7, Sequence: 3, GroupID: 10, Title: "Sprint", Description: "Desc", State: 2, WebURL: "https://example.test/it/42"}
	if project != want {
		t.Errorf("ProjectOutput() = %+v, want %+v", project, want)
	}
}

// TestOutputConverters_NilInput verifies converters return zero-value output
// for nil API objects, matching the previous package-local behavior.
func TestOutputConverters_NilInput(t *testing.T) {
	if got := ProjectOutput(nil); got != (Output{}) {
		t.Fatalf("ProjectOutput(nil) = %+v, want zero Output", got)
	}
	if got := GroupOutput(nil); got != (Output{}) {
		t.Fatalf("GroupOutput(nil) = %+v, want zero Output", got)
	}
}

// TestListOptions_ApplyFilters verifies shared option builders set pagination
// and optional filters for both project and group iteration list requests.
func TestListOptions_ApplyFilters(t *testing.T) {
	group := NewGroupListOptions(2, 50, "opened", "sprint", true)
	if group.Page != 2 || group.PerPage != 50 || group.State == nil || *group.State != "opened" || group.Search == nil || *group.Search != "sprint" || group.IncludeAncestors == nil || !*group.IncludeAncestors {
		t.Fatalf("NewGroupListOptions() = %+v, want pagination and filters", group)
	}

	project := NewProjectListOptions(3, 25, "current", "release", true)
	if project.Page != 3 || project.PerPage != 25 || project.State == nil || *project.State != "current" || project.Search == nil || *project.Search != "release" || project.IncludeAncestors == nil || !*project.IncludeAncestors {
		t.Fatalf("NewProjectListOptions() = %+v, want pagination and filters", project)
	}
}

// TestListOptions_NoFilters_LeavesTheOptionalFiltersUnset pins what both option
// builders put on the wire when the caller asked to filter by nothing:
// pagination, and no filter parameter at all.
//
// The distinction is not cosmetic. A pointer to a zero value is not the absence
// of a filter — client-go serializes whatever the pointer holds, so GitLab
// would receive `state=` and `include_ancestors=false` on a request that meant
// to constrain neither, and an empty state is not a state GitLab has. Only a
// nil pointer omits the parameter, which is what an unfiltered list needs.
func TestListOptions_NoFilters_LeavesTheOptionalFiltersUnset(t *testing.T) {
	t.Run("group", func(t *testing.T) {
		opts := NewGroupListOptions(1, 20, "", "", false)

		if opts.Page != 1 || opts.PerPage != 20 {
			t.Errorf("NewGroupListOptions() paged with page %d and per_page %d, want 1 and 20", opts.Page, opts.PerPage)
		}
		if opts.State != nil {
			t.Errorf("NewGroupListOptions() State = %q, want it unset for an empty state filter", *opts.State)
		}
		if opts.Search != nil {
			t.Errorf("NewGroupListOptions() Search = %q, want it unset for an empty search filter", *opts.Search)
		}
		if opts.IncludeAncestors != nil {
			t.Errorf("NewGroupListOptions() IncludeAncestors = %v, want it unset when ancestors were not asked for", *opts.IncludeAncestors)
		}
	})

	t.Run("project", func(t *testing.T) {
		opts := NewProjectListOptions(1, 20, "", "", false)

		if opts.Page != 1 || opts.PerPage != 20 {
			t.Errorf("NewProjectListOptions() paged with page %d and per_page %d, want 1 and 20", opts.Page, opts.PerPage)
		}
		if opts.State != nil {
			t.Errorf("NewProjectListOptions() State = %q, want it unset for an empty state filter", *opts.State)
		}
		if opts.Search != nil {
			t.Errorf("NewProjectListOptions() Search = %q, want it unset for an empty search filter", *opts.Search)
		}
		if opts.IncludeAncestors != nil {
			t.Errorf("NewProjectListOptions() IncludeAncestors = %v, want it unset when ancestors were not asked for", *opts.IncludeAncestors)
		}
	})
}
