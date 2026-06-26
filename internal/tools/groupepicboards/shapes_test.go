// shapes_test.go validates the documented group-epic-board sub-object mirrors
// (group, label details with the raw-superset fields, and board list label
// scope) and the time-formatting helper, covering both the fully-populated and
// nil branches of every converter per the doc-grounded reconcile.
package groupepicboards

import (
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestConvertersNil verifies that every sub-object converter returns nil (or a
// zero value) when given a nil input pointer.
func TestConvertersNil(t *testing.T) {
	if got := groupRefOutput(nil); got != nil {
		t.Errorf("groupRefOutput(nil) = %+v, want nil", got)
	}
	if got := labelDetailsOutput(nil); got != nil {
		t.Errorf("labelDetailsOutput(nil) = %+v, want nil", got)
	}
	if got := labelDetailsOutputs(nil); got != nil {
		t.Errorf("labelDetailsOutputs(nil) = %+v, want nil", got)
	}
	if got := listLabelOutput(nil); got != nil {
		t.Errorf("listLabelOutput(nil) = %+v, want nil", got)
	}
	if got := convertBoardList(nil); got != (BoardListOutput{}) {
		t.Errorf("convertBoardList(nil) = %+v, want zero", got)
	}
	if got := toOutput(nil); got.ID != 0 || got.Name != "" {
		t.Errorf("toOutput(nil) = %+v, want zero", got)
	}
}

// TestTimeHelper verifies the RFC3339 helper across nil and populated inputs.
func TestTimeHelper(t *testing.T) {
	if got := toolutil.FormatTimePtr(nil); got != "" {
		t.Errorf("toolutil.FormatTimePtr(nil) = %q, want empty", got)
	}
	tm := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := toolutil.FormatTimePtr(&tm); got != "2024-01-02T03:04:05Z" {
		t.Errorf("formatTimePtr = %q", got)
	}
}

// TestToOutputFullyPopulated verifies that toOutput surfaces every documented
// nested sub-object (group trimmed to id/name/web_url, the raw-superset label
// details including title/group_id/template/created_at, and the list label
// scope with list_type and collapsed) of a fully-populated raw-superset board,
// and that hide_*_list flags are surfaced.
func TestToOutputFullyPopulated(t *testing.T) {
	board := &groupEpicBoardAPI{
		GroupEpicBoard: gl.GroupEpicBoard{
			ID:    1,
			Name:  "Board",
			Group: &gl.Group{ID: 7, Name: "G", FullPath: "ns/g", WebURL: "https://x"},
		},
		HideBacklogList: true,
		HideClosedList:  true,
		Labels: []*labelDetailsAPI{
			{
				LabelDetails: gl.LabelDetails{ID: 10, Name: "Priority", Color: "#f00", Description: "p", DescriptionHTML: "<p>p</p>", TextColor: "#fff"},
				Title:        "Priority", GroupID: 7, ProjectID: 0, Template: false,
				CreatedAt: "2023-01-27T10:40:59.738Z", UpdatedAt: "2023-01-27T10:40:59.738Z",
			},
			nil,
		},
		Lists: []*boardListAPI{
			{
				BoardList: gl.BoardList{ID: 100, Label: &gl.Label{ID: 10, Name: "Priority", Color: "#f00", Description: "p"}, Position: 1},
				ListType:  "label",
				Collapsed: new(false),
			},
			nil,
		},
	}

	out := toOutput(board)
	assertFullGroup(t, out)
	assertFullLabels(t, out)
	assertFullList(t, out)
}

func assertFullGroup(t *testing.T, out Output) {
	t.Helper()
	if !out.HideBacklogList || !out.HideClosedList {
		t.Errorf("hide flags = %v/%v, want true/true", out.HideBacklogList, out.HideClosedList)
	}
	if out.Group == nil {
		t.Fatal("Group nil")
	}
	if out.Group.ID != 7 || out.Group.Name != "G" || out.Group.WebURL != "https://x" {
		t.Errorf("Group = %+v", out.Group)
	}
}

func assertFullLabels(t *testing.T, out Output) {
	t.Helper()
	if len(out.Labels) != 1 {
		t.Fatalf("len(Labels) = %d, want 1 (nil filtered)", len(out.Labels))
	}
	l := out.Labels[0]
	if l.Color != "#f00" || l.DescriptionHTML != "<p>p</p>" || l.TextColor != "#fff" {
		t.Errorf("LabelDetails core = %+v", l)
	}
	if l.Title != "Priority" || l.GroupID != 7 || l.CreatedAt == "" || l.UpdatedAt == "" {
		t.Errorf("LabelDetails superset = %+v", l)
	}
}

func assertFullList(t *testing.T, out Output) {
	t.Helper()
	if len(out.Lists) != 1 {
		t.Fatalf("len(Lists) = %d, want 1 (nil filtered)", len(out.Lists))
	}
	bl := out.Lists[0]
	if bl.Label == nil || bl.Label.ID != 10 || bl.Label.Name != "Priority" || bl.Label.Color != "#f00" {
		t.Errorf("Label = %+v", bl.Label)
	}
	if bl.Position != 1 || bl.ListType != "label" {
		t.Errorf("list scalars = position %d, list_type %q", bl.Position, bl.ListType)
	}
	if bl.Collapsed == nil || *bl.Collapsed {
		t.Errorf("Collapsed = %v, want non-nil false", bl.Collapsed)
	}
}

// TestMarkdownHelpersNil verifies the markdown label helpers tolerate nil and
// empty inputs.
func TestMarkdownHelpersNil(t *testing.T) {
	if got := labelNames(nil); got != nil {
		t.Errorf("labelNames(nil) = %v, want nil", got)
	}
	if got := labelNames([]*LabelDetailsOutput{nil, {Name: "a"}}); len(got) != 1 || got[0] != "a" {
		t.Errorf("labelNames filtered = %v", got)
	}
	if got := listLabelName(BoardListOutput{}); got != "" {
		t.Errorf("listLabelName(no label) = %q, want empty", got)
	}
	if got := listLabelName(BoardListOutput{Label: &ListLabelOutput{Name: "L"}}); got != "L" {
		t.Errorf("listLabelName = %q, want L", got)
	}
}
