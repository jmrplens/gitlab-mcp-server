package toolutil

import (
	"strings"
	"testing"
)

// TestBoardColumnScope_EachScopeKind_NamesWhatTheColumnCollects pins the
// scope cell the project and the group boards both write for a column: the
// label, the assignee's handle, the milestone and the iteration, each escaped
// as a table cell, the first of them present winning over the rest. A scope
// object GitLab sent without the name or title it is shown by is passed over,
// so the next one present names the column, and a column with none, the
// status column a REST answer can carry, renders as an empty cell.
func TestBoardColumnScope_EachScopeKind_NamesWhatTheColumnCollects(t *testing.T) {
	label := &BoardLabelOutput{Name: "To|Do"}
	unnamedLabel := &BoardLabelOutput{Color: "#fff"}
	assignee := &BoardListAssigneeOutput{ID: 3, Username: "ada"}
	unnamedAssignee := &BoardListAssigneeOutput{ID: 4, Name: "No Handle"}
	iteration := &IterationOutput{ID: 5, Title: "Sprint|4"}
	untitledIteration := &IterationOutput{ID: 6}
	tests := []struct {
		name      string
		label     *BoardLabelOutput
		assignee  *BoardListAssigneeOutput
		milestone string
		iteration *IterationOutput
		want      string
	}{
		{name: "label", label: label, want: "To&#124;Do"},
		{name: "label wins over the rest", label: label, assignee: assignee, milestone: "v1", iteration: iteration, want: "To&#124;Do"},
		{name: "label with no name passes to the assignee", label: unnamedLabel, assignee: assignee, want: "@ada"},
		{name: "assignee", assignee: assignee, want: "@ada"},
		{name: "assignee wins over milestone and iteration", assignee: assignee, milestone: "v1", iteration: iteration, want: "@ada"},
		{name: "assignee with no username passes to the milestone", assignee: unnamedAssignee, milestone: "v1|x", want: "Milestone: v1&#124;x"},
		{name: "milestone wins over iteration", milestone: "v1", iteration: iteration, want: "Milestone: v1"},
		{name: "iteration", iteration: iteration, want: "Iteration: Sprint&#124;4"},
		{name: "iteration with no title", iteration: untitledIteration, want: ""},
		{name: "objects with nothing to show", label: unnamedLabel, assignee: unnamedAssignee, iteration: untitledIteration, want: ""},
		{name: "status column", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BoardColumnScope(tt.label, tt.assignee, tt.milestone, tt.iteration); got != tt.want {
				t.Errorf("BoardColumnScope() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBoardColumnCells_Row_WritesTheSixCellsInHeaderOrder pins one column's
// row in a table of a board's columns, under the ID, Scope, Position, Max
// Issues, Max Weight and Limit Metric headers: the IDs and position as
// numbers, the scope as given, a ceiling of zero as a dash rather than as the
// number 0, and the metric escaped like any value GitLab wrote, or a dash when
// GitLab sent none.
func TestBoardColumnCells_Row_WritesTheSixCellsInHeaderOrder(t *testing.T) {
	tests := []struct {
		name                 string
		id, position         int64
		scope, metric        string
		maxIssues, maxWeight int64
		want                 string
	}{
		{
			name: "limited by issues", id: 10, scope: "Doing", position: 2,
			maxIssues: 3, metric: "issue_count",
			want: "| 10 | Doing | 2 | 3 | - | issue_count |\n",
		},
		{
			name: "limited by weight", id: 11, scope: "@ada",
			maxWeight: 8, metric: "<b>|x",
			want: "| 11 | @ada | 0 | - | 8 | &lt;b>&#124;x |\n",
		},
		{
			name: "no limit and no metric", id: 12, position: 4,
			want: "| 12 |  | 4 | - | - | - |\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cells := BoardColumnCells(tt.id, tt.scope, tt.position, tt.maxIssues, tt.maxWeight, tt.metric)
			if got := MarkdownTableRow(cells...); got != tt.want {
				t.Errorf("BoardColumnCells() row = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBoardColumnLimits_Write_ListsTheLimitedColumns pins the section a page
// of boards closes with when one of its columns carries a work-in-progress
// limit: a blank line after the table above it, the heading, then one row per
// column with a ceiling on its issues, on their weight or on both, in the
// order the columns were added. A ceiling a column does not set and a metric
// GitLab sent none of are dashes, and a metric is escaped like any value
// GitLab wrote. A column with neither ceiling has no row, whether or not it
// names a metric, since a metric alone limits nothing.
func TestBoardColumnLimits_Write_ListsTheLimitedColumns(t *testing.T) {
	var limits BoardColumnLimits
	limits.Add(1, 10, "Doing", 3, 0, "issue_count")
	limits.Add(1, 11, "To Do", 0, 0, "")
	limits.Add(1, 12, "Review", 0, 4, "issue_weights")
	limits.Add(2, 20, "@alice", 2, 8, "all_metrics")
	limits.Add(2, 21, "Milestone: v1", 0, 0, "issue_count")
	limits.Add(2, 22, "Iteration: S1", 5, 0, "")
	limits.Add(3, 30, "", 1, 0, "<b>|x")

	var b strings.Builder
	b.WriteString("| 3 | Ops |\n")
	limits.Write(&b)

	want := "| 3 | Ops |\n\n" +
		"### Column Limits\n\n" +
		"| Board | List | Scope | Max Issues | Max Weight | Limit Metric |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | 10 | Doing | 3 | - | issue_count |\n" +
		"| 1 | 12 | Review | - | 4 | issue_weights |\n" +
		"| 2 | 20 | @alice | 2 | 8 | all_metrics |\n" +
		"| 2 | 22 | Iteration: S1 | 5 | - | - |\n" +
		"| 3 | 30 |  | 1 | - | &lt;b>&#124;x |\n"
	if got := b.String(); got != want {
		t.Errorf("BoardColumnLimits.Write()\n got %q\nwant %q", got, want)
	}
}

// TestBoardColumnLimits_Write_NoLimitedColumn_WritesOneSentence pins what a
// page whose boards limit no column closes with: one sentence after a blank
// line, with no heading and no table header standing over nothing, whether no
// column was added at all or every one added carries no ceiling.
func TestBoardColumnLimits_Write_NoLimitedColumn_WritesOneSentence(t *testing.T) {
	const want = "| 1 | Dev |\n\nNo column on these boards has a work-in-progress limit.\n"
	tests := []struct {
		name string
		add  func(*BoardColumnLimits)
	}{
		{name: "no column", add: func(*BoardColumnLimits) {}},
		{name: "unlimited columns", add: func(l *BoardColumnLimits) {
			l.Add(1, 10, "To Do", 0, 0, "")
			l.Add(1, 11, "Doing", 0, 0, "issue_count")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var limits BoardColumnLimits
			tt.add(&limits)
			var b strings.Builder
			b.WriteString("| 1 | Dev |\n")
			limits.Write(&b)
			if got := b.String(); got != want {
				t.Errorf("BoardColumnLimits.Write()\n got %q\nwant %q", got, want)
			}
		})
	}
}
