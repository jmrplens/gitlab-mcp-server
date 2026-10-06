package actioncatalog

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestScopeWithheld_Missing_JoinsBothHalvesSortedAndOnce verifies the scopes
// a caller is told to reauthorize with: GitLab's half and this server's half
// together, sorted and each once whichever half names it, and nothing when
// neither names anything.
func TestScopeWithheld_Missing_JoinsBothHalvesSortedAndOnce(t *testing.T) {
	cases := []struct {
		name     string
		withheld ScopeWithheld
		want     []string
	}{
		{name: "neither", withheld: ScopeWithheld{ID: "a.b"}},
		{name: "GitLab's", withheld: ScopeWithheld{ByGitLab: []string{"api"}}, want: []string{"api"}},
		{name: "this server's", withheld: ScopeWithheld{ByServer: []string{"admin_mode"}}, want: []string{"admin_mode"}},
		{name: "both, sorted", withheld: ScopeWithheld{ByGitLab: []string{"api"}, ByServer: []string{"admin_mode"}}, want: []string{"admin_mode", "api"}},
		{name: "one scope in both halves", withheld: ScopeWithheld{ByGitLab: []string{"api"}, ByServer: []string{"api"}}, want: []string{"api"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.withheld.Missing(); !slices.Equal(got, tc.want) {
				t.Errorf("Missing() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAction_ClassicNeed_ReadsTheRowAndFallsBackOnTheClassification verifies
// an action needs what its row of the generated table says, and, when it has
// no row or a row holding no known scope, what its classification gives:
// read_api for a read and api for anything else.
func TestAction_ClassicNeed_ReadsTheRowAndFallsBackOnTheClassification(t *testing.T) {
	cases := []struct {
		name   string
		action Action
		want   finegrained.ClassicScope
	}{
		{name: "a row", action: Action{FineGrained: &finegrained.Requirement{Classic: finegrained.ClassicOtherCredential}}, want: finegrained.ClassicOtherCredential},
		{name: "a row for a write GitLab serves read_api", action: Action{FineGrained: &finegrained.Requirement{Classic: finegrained.ClassicReadAPI}}, want: finegrained.ClassicReadAPI},
		{name: "a row with no known scope, a read", action: Action{FineGrained: &finegrained.Requirement{}, ReadOnly: true}, want: finegrained.ClassicReadAPI},
		{name: "no row, a read", action: Action{ReadOnly: true}, want: finegrained.ClassicReadAPI},
		{name: "no row, a write", action: Action{}, want: finegrained.ClassicAPI},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.action.ClassicNeed(); got != tc.want {
				t.Errorf("ClassicNeed() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestClassicNeedOf_ReadsTheGeneratedTableByID verifies an action known only
// by its ID is judged by the generated table's row, and one the table holds
// no row for by its classification.
func TestClassicNeedOf_ReadsTheGeneratedTableByID(t *testing.T) {
	cases := []struct {
		name     string
		id       string
		readOnly bool
		want     finegrained.ClassicScope
	}{
		{name: "a route another credential authenticates", id: "runner.verify", readOnly: true, want: finegrained.ClassicOtherCredential},
		{name: "a read GitLab refuses read_api", id: "template.lint", readOnly: true, want: finegrained.ClassicAPI},
		{name: "an action with no row", id: "no.such_action", readOnly: true, want: finegrained.ClassicReadAPI},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassicNeedOf(tc.id, tc.readOnly); got != tc.want {
				t.Errorf("ClassicNeedOf(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

// TestCatalog_FilterReachableWith_KeepsWhatTheScopeReaches verifies the reach
// filter keeps the actions a scope reaches whatever their classification,
// drops the rest and every group left empty, marks a kept group read-only only
// when every action it keeps reads, leaves the source catalog as it was, and
// answers a nil catalog with nil.
func TestCatalog_FilterReachableWith_KeepsWhatTheScopeReaches(t *testing.T) {
	catalog := NewCatalog()
	pkg := NewGroup(GroupOptions{ToolName: "gitlab_package", ReadOnly: true})
	pkg.SetAction(Action{Name: "download", Route: testRoute(false)})
	pkg.SetAction(Action{Name: "list", Route: testRoute(false), ReadOnly: true})
	pkg.SetAction(Action{Name: "delete", Route: testRoute(true)})
	tmpl := NewGroup(GroupOptions{ToolName: "gitlab_template"})
	tmpl.SetAction(Action{Name: "lint", Route: testRoute(false), ReadOnly: true})
	reads := NewGroup(GroupOptions{ToolName: "gitlab_issue"})
	reads.SetAction(Action{Name: "list", Route: testRoute(false), ReadOnly: true})
	reads.SetAction(Action{Name: "create", Route: testRoute(false)})
	// sequential: each group joins the one catalog every case below reads.
	for _, group := range []Group{pkg, tmpl, reads} {
		if err := catalog.AddGroup(group); err != nil {
			t.Fatalf("AddGroup(%s) error = %v", group.ToolName, err)
		}
	}

	filtered := catalog.FilterReachableWith(finegrained.ClassicReadAPI)

	reach := map[ActionID]bool{
		"package.download": true, "package.list": true, "issue.list": true,
		"package.delete": false, "template.lint": false, "issue.create": false,
	}
	for id, want := range reach {
		t.Run(string(id), func(t *testing.T) {
			if _, kept := filtered.Action(id); kept != want {
				t.Errorf("%s kept for read_api = %t, want %t", id, kept, want)
			}
		})
	}
	if _, kept := filtered.Group("gitlab_template"); kept {
		t.Error("a group left with no action was kept")
	}
	if group, _ := filtered.Group("gitlab_package"); group.ReadOnly {
		t.Error("gitlab_package keeps a write and is marked read-only")
	}
	if group, _ := filtered.Group("gitlab_issue"); !group.ReadOnly {
		t.Error("gitlab_issue keeps only a read and is not marked read-only")
	}
	if catalog.CountActions() != 6 {
		t.Errorf("source catalog mutated: CountActions() = %d, want 6", catalog.CountActions())
	}
	if all := catalog.FilterReachableWith(finegrained.ClassicAPI); all.CountActions() != 6 {
		t.Errorf("api reaches %d actions, want all 6", all.CountActions())
	}
	var nilCatalog *Catalog
	if nilCatalog.FilterReachableWith(finegrained.ClassicAPI) != nil {
		t.Error("nil catalog FilterReachableWith() must return nil")
	}
}
