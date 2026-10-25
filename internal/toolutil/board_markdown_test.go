package toolutil

import (
	"strings"
	"testing"
)

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
