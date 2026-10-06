package grants

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// ClassicView is what a classic personal access token or an OAuth token
// needs, read from the committed table: how many actions need each scope,
// what a token carrying read_api and not api is served, and where that
// departs from what the catalog classifies as a read. It reports and never
// gates: the derivation's gates 4 and 5 hold the same answer to its
// declarations, and the inconsistencies above hold it to the record.
type ClassicView struct {
	// ByScope counts the actions per classic scope, spelled as the records
	// spell it.
	ByScope map[string]int `json:"by_scope"`
	// ReachedByReadAPI counts the actions read_api reaches.
	ReachedByReadAPI int `json:"reached_by_read_api"`
	// ServedOtherThanGet are the REST operations that are not a GET or a HEAD
	// a read_api token is served, each with the scope it was found to need:
	// the routes the derivation declares GitLab grants read_api, or none of
	// the token's scopes, for every method. The record carries no route's
	// scopes, so this list is what a reviewer holds to GitLab's source.
	ServedOtherThanGet []ClassicOperation `json:"served_other_than_get"`
	// Departures are the actions whose read or write classification departs
	// from what read_api reaches.
	Departures []ClassicDeparture `json:"departures"`
}

// ClassicOperation is one REST operation and the classic scope it needs.
type ClassicOperation struct {
	Operation string `json:"operation"`
	Classic   string `json:"classic"`
}

// ClassicDeparture is one action a read_api token is served although the
// catalog classifies it as a write, or withheld although it is a read.
type ClassicDeparture struct {
	ID       string `json:"id"`
	Classic  string `json:"classic"`
	ReadOnly bool   `json:"read_only"`
	Served   bool   `json:"served_to_read_api"`
}

// classicView reads the classic half of the table for the report.
func classicView(table *finegrained.Table, actions []actionrequests.Action) ClassicView {
	view := ClassicView{ByScope: map[string]int{}, ServedOtherThanGet: []ClassicOperation{}, Departures: []ClassicDeparture{}}
	readOnly := make(map[string]bool, len(actions))
	for _, action := range actions {
		readOnly[action.ID] = action.ReadOnly
	}
	for i := range table.Actions {
		row := &table.Actions[i]
		view.ByScope[row.Classic.String()]++
		served := row.Classic.ReachableWith(finegrained.ClassicReadAPI)
		if served {
			view.ReachedByReadAPI++
		}
		if served != readOnly[row.ID] {
			view.Departures = append(view.Departures, ClassicDeparture{
				ID: row.ID, Classic: row.Classic.String(), ReadOnly: readOnly[row.ID], Served: served,
			})
		}
	}
	for i := range table.Operations {
		op := &table.Operations[i]
		method, _, _ := strings.Cut(op.Name, " ")
		if graphQLOperation(op.Name) || method == "GET" || method == "HEAD" || !op.Classic.ReachableWith(finegrained.ClassicReadAPI) {
			continue
		}
		view.ServedOtherThanGet = append(view.ServedOtherThanGet, ClassicOperation{Operation: op.Name, Classic: op.Classic.String()})
	}
	slices.SortFunc(view.ServedOtherThanGet, func(a, b ClassicOperation) int { return strings.Compare(a.Operation, b.Operation) })
	return view
}
