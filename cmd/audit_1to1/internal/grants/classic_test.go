package grants

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestClassicView_CountsTheScopesAndNamesWhatDeparts verifies the classic
// view counts the rows per scope and the ones read_api reaches, lists in name
// order every REST operation that is not a GET or a HEAD and that read_api
// reaches, leaving out the GraphQL ones, the reads and the ones needing api,
// and names each action whose classification departs from that reach, a
// write served and a read withheld.
func TestClassicView_CountsTheScopesAndNamesWhatDeparts(t *testing.T) {
	table := &finegrained.Table{
		Operations: []finegrained.Operation{
			{Name: "POST /runners", Classic: finegrained.ClassicOtherCredential},
			{Name: "POST /markdown", Classic: finegrained.ClassicReadAPI},
			{Name: "GET /projects/:id", Classic: finegrained.ClassicReadAPI},
			{Name: "HEAD /raw", Classic: finegrained.ClassicReadAPI},
			{Name: "POST /projects/:id/ci/lint", Classic: finegrained.ClassicAPI},
			{Name: "query project (q)", Classic: finegrained.ClassicReadAPI},
		},
		Actions: []finegrained.Requirement{
			{ID: "project.get", Classic: finegrained.ClassicReadAPI},
			{ID: "repository.archive", Classic: finegrained.ClassicNoRequest},
			{ID: "runner.register", Classic: finegrained.ClassicOtherCredential},
			{ID: "template.lint", Classic: finegrained.ClassicAPI},
			{ID: "project.delete", Classic: finegrained.ClassicAPI},
		},
	}
	actions := []actionrequests.Action{
		{ID: "project.get", ReadOnly: true},
		{ID: "repository.archive", ReadOnly: true},
		{ID: "template.lint", ReadOnly: true},
		{ID: "runner.register"},
		{ID: "project.delete"},
	}
	got := classicView(table, actions)
	want := ClassicView{
		ByScope:          map[string]int{"api": 2, "read_api": 1, "other-credential": 1, "no-request": 1},
		ReachedByReadAPI: 3,
		ServedOtherThanGet: []ClassicOperation{
			{Operation: "POST /markdown", Classic: "read_api"},
			{Operation: "POST /runners", Classic: "other-credential"},
		},
		Departures: []ClassicDeparture{
			{ID: "runner.register", Classic: "other-credential", ReadOnly: false, Served: true},
			{ID: "template.lint", Classic: "api", ReadOnly: true, Served: false},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("classicView =\n%+v\nwant\n%+v", got, want)
	}
}

// TestClassicView_NothingDeparts_ListsAreEmptyNotAbsent verifies a table with
// nothing to list writes empty lists, which a reader ranges over, rather than
// nulls.
func TestClassicView_NothingDeparts_ListsAreEmptyNotAbsent(t *testing.T) {
	got := classicView(&finegrained.Table{}, nil)
	if got.ServedOtherThanGet == nil || got.Departures == nil || got.ByScope == nil {
		t.Errorf("classicView of an empty table = %+v, want empty lists and map", got)
	}
}
