package grants

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestInconsistencies_TheFixture_AgreesWithItsRecord verifies the fixture the
// other tests bend is consistent to begin with, so each finding below comes
// from the one change its case makes.
func TestInconsistencies_TheFixture_AgreesWithItsRecord(t *testing.T) {
	if found := inconsistencies(fixtureTable(), fixtureRecord(), fixtureLive(), fixtureCatalog()); len(found) != 0 {
		t.Errorf("inconsistencies = %v, want none", found)
	}
}

// TestInconsistencies_EachDisagreement_IsNamedWithTheWayOut verifies every
// check of the gate: the release, the vocabulary both ways, the public sets
// (known on one side only, and a permission on each boundary), a denial and a
// denied way naming something the record does not hold as the kind its cause
// says, a REST operation no route of the record is, one demanding other
// permissions than its route, one the record now skips, defers or declares, one
// the table skips that the record checks, and an action one artifact covers
// and the other does not, which the catalog does not build either. Each
// finding ends with the command that regenerates the artifacts, or the one
// that re-records the record when it is the record that lacks something.
func TestInconsistencies_EachDisagreement_IsNamedWithTheWayOut(t *testing.T) {
	cases := []struct {
		name  string
		bend  func(table *finegrained.Table, record *actionrequests.Record, live *apilive.Document)
		wants []string
	}{
		{
			name: "another_release",
			bend: func(table *finegrained.Table, _ *actionrequests.Record, _ *apilive.Document) {
				table.Version = "19.3.0-ee"
			},
			wants: []string{"the table was joined at GitLab 19.3.0-ee and the live record is 19.4.1-ee; run `make gen-action-grants`"},
		},
		{
			name: "no_vocabulary",
			bend: func(table *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				live.Granular = nil
				table.PublicKnown = false
			},
			wants: []string{"the live record carries no fine-grained permission vocabulary; re-record it with `make gen-api-live`"},
		},
		{
			name: "a_permission_the_record_lost",
			bend: func(_ *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				live.Granular.Assignable = live.Granular.Assignable[:3]
			},
			wants: []string{"the table names the permission read_protected_branch, which no assignable of the live record expands to; run `make gen-action-grants`"},
		},
		{
			name: "a_permission_the_table_lacks",
			bend: func(_ *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				live.Granular.Assignable = append(live.Granular.Assignable, assignable("read_epic", "epic", "read_epic"))
			},
			wants: []string{"the live record's permission read_epic is missing from the table; run `make gen-action-grants`"},
		},
		{
			name: "public_sets_known_on_one_side",
			bend: func(_ *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				live.Granular.PublicAnonymous = nil
			},
			wants: []string{"the table says the public sets are known (true) and the live record says false; run `make gen-action-grants`"},
		},
		{
			name: "public_sets_known_on_the_record_only",
			bend: func(table *finegrained.Table, _ *actionrequests.Record, _ *apilive.Document) {
				table.PublicKnown = false
			},
			wants: []string{"the table says the public sets are known (false) and the live record says true; run `make gen-action-grants`"},
		},
		{
			name: "a_public_permission_on_each_boundary",
			bend: func(_ *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				live.Granular.PublicAnonymous.Project = []string{"read_issue", "update_issue"}
				live.Granular.PublicAnonymous.Group = nil
			},
			wants: []string{
				"the table and the live record disagree on whether read_namespace is public on a group; run `make gen-action-grants`",
				"the table and the live record disagree on whether update_issue is public on a project; run `make gen-action-grants`",
			},
		},
		{
			name: "a_denial_and_a_denied_way_the_record_does_not_hold",
			bend: func(_ *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				delete(live.GraphQLAuthz.Types, "BranchRule")
				live.Routes = slices.DeleteFunc(live.Routes, func(route apilive.Route) bool { return route.Path == apilive.EndpointPrefix+"/nothing" })
			},
			wants: []string{
				"a way of running namespace.list is denied by GET /nothing, which the live record does not hold as a rest-undeclared; run `make gen-action-grants`",
				"branch.rule_list is denied by BranchRule, which the live record does not hold as a graphql-type-undeclared; run `make gen-action-grants`",
			},
		},
		{
			name: "an_operation_no_route_of_the_record_is",
			bend: func(_ *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				live.Routes = slices.DeleteFunc(live.Routes, func(route apilive.Route) bool {
					return route.Path == apilive.EndpointPrefix+"/projects/:id/protected_branches"
				})
			},
			wants: []string{"the table's operation GET /projects/:id/protected_branches is no route of the live record; run `make gen-action-grants`"},
		},
		{
			name: "a_route_demanding_other_permissions",
			bend: func(_ *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				liveRoute(live, "PATCH /projects/:id/issues").Authorization = held("project", "read_issue")
			},
			wants: []string{"the table says PATCH /projects/:id/issues demands read_issue and update_issue at project and the live record says read_issue at project; run `make gen-action-grants`"},
		},
		{
			name: "routes_the_record_now_skips_and_defers",
			bend: func(_ *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				liveRoute(live, "POST /projects/:id/things").Authorization.Skip = "public"
				liveRoute(live, "GET /namespaces").Authorization = &apilive.RouteAuthorization{Todo: "later"}
			},
			wants: []string{
				"the table says GET /namespaces demands read_namespace at group and the live record says no permission; run `make gen-action-grants`",
				"the table says POST /projects/:id/things demands read_issue at project; read_namespace at group and the live record says a skip that leaves the route to GitLab; run `make gen-action-grants`",
			},
		},
		{
			name: "a_route_the_table_skips_and_the_record_checks",
			bend: func(_ *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				liveRoute(live, "GET /topics").Authorization = held("project", "read_issue")
			},
			wants: []string{"the table says GET /topics demands a skip that leaves the route to GitLab and the live record says read_issue at project; run `make gen-action-grants`"},
		},
		{
			name: "an_action_on_one_side",
			bend: func(table *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				table.Actions = table.Actions[1:]
				record.Actions = append(record.Actions, actionrequests.RecordAction{ID: "zz.new"})
				table.Actions = append(table.Actions, finegrained.Requirement{ID: "zz.other", Classic: finegrained.ClassicAPI})
			},
			wants: []string{
				"branch.protected_list has an entry in the request record and no row in the table; run `make gen-action-grants`",
				"branch.protected_list is built by the catalog and has no row in the table; run `make gen-action-grants`",
				"zz.new has an entry in the request record and no row in the table; run `make gen-action-grants`",
				"zz.other has a row in the table and is built by no catalog; run `make gen-action-grants`",
				"zz.other has a row in the table and no entry in the request record; run `make gen-action-grants`",
			},
		},
	}
	cases = append(cases, classicCases()...)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			table, record, live := fixtureTable(), fixtureRecord(), fixtureLive()
			testCase.bend(table, &record, live)
			found := inconsistencies(table, record, live, fixtureCatalog())
			if !slices.Equal(found, testCase.wants) {
				t.Errorf("inconsistencies =\n%s\nwant\n%s", strings.Join(found, "\n"), strings.Join(testCase.wants, "\n"))
			}
		})
	}
}

// classicCase bends the fixture one way the classic half of the table can
// disagree, or one way it may depart from the rule and still agree.
type classicCase = struct {
	name  string
	bend  func(table *finegrained.Table, record *actionrequests.Record, live *apilive.Document)
	wants []string
}

// tableOp and tableRow are the fixture table's operation and row of one name,
// which a case bends in place.
func tableOp(table *finegrained.Table, name string) *finegrained.Operation {
	for i := range table.Operations {
		if table.Operations[i].Name == name {
			return &table.Operations[i]
		}
	}
	return nil
}

// tableRow is the fixture table's row of one action.
func tableRow(table *finegrained.Table, id string) *finegrained.Requirement {
	return table.Requirement(id)
}

// recordAction is the fixture record's entry of one action.
func recordAction(record *actionrequests.Record, id string) *actionrequests.RecordAction {
	for i := range record.Actions {
		if record.Actions[i].ID == id {
			return &record.Actions[i]
		}
	}
	return nil
}

// classicCases are the classic half of the gate: a GET and a HEAD said to need
// api, a mutation said to need read_api, a query said to need api with and
// without the request record naming it as selecting a field only api is
// answered, and a query the record names so that the table says needs
// read_api; a route GitLab authenticates by another credential said to need
// api, another credential said of a route with no such skip, a POST needing
// read_api and a route declaring nothing, both of which pass; a row with no
// known scope, one the record disagrees with, one needing more than the least
// of its ways and one needing less with no way denied, and one needing less
// beside a denied way, which passes.
func classicCases() []classicCase {
	const issuesQuery = "query project (issuesQuery)"
	apiOnly := func(record *actionrequests.Record) {
		recordAction(record, "issue.list").Requests[0].ClassicDeclaration = actionrequests.ClassicAPIOnlyField + " Issue.createNoteEmail"
	}
	queryNeedsAPI := func(table *finegrained.Table, record *actionrequests.Record) {
		tableOp(table, issuesQuery).Classic = finegrained.ClassicAPI
		tableRow(table, "issue.list").Classic = finegrained.ClassicAPI
		recordAction(record, "issue.list").Classic = "api"
	}
	return []classicCase{
		{
			name: "a_get_said_to_need_api",
			bend: func(table *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				tableOp(table, "GET /projects/:id/protected_branches").Classic = finegrained.ClassicAPI
				tableRow(table, "branch.protected_list").Classic = finegrained.ClassicAPI
				recordAction(record, "branch.protected_list").Classic = "api"
			},
			wants: []string{"the table says GET /projects/:id/protected_branches needs api and GitLab accepts read_api for a GET; run `make gen-action-grants`"},
		},
		{
			name: "a_head_said_to_need_api",
			bend: func(table *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				live.Routes = append(live.Routes, route("HEAD", "/raw", held("project", "read_issue")))
				table.Operations = append(table.Operations, finegrained.Operation{Name: "HEAD /raw", Classic: finegrained.ClassicAPI, Groups: []uint32{0}})
			},
			wants: []string{"the table says HEAD /raw needs api and GitLab accepts read_api for a HEAD; run `make gen-action-grants`"},
		},
		{
			name: "a_mutation_said_to_need_read_api",
			bend: func(table *finegrained.Table, _ *actionrequests.Record, _ *apilive.Document) {
				table.Operations = append(table.Operations, finegrained.Operation{Name: "mutation bulkUpdate (bulkUpdate)", Classic: finegrained.ClassicReadAPI})
			},
			wants: []string{"the table says mutation bulkUpdate (bulkUpdate) needs read_api and a mutation needs api; run `make gen-action-grants`"},
		},
		{
			name: "a_query_said_to_need_api",
			bend: func(table *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				queryNeedsAPI(table, record)
			},
			wants: []string{"the table says query project (issuesQuery) needs api and a query needs read_api; run `make gen-action-grants`"},
		},
		{
			name: "a_query_the_record_names_as_selecting_an_api_only_field",
			bend: func(table *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				queryNeedsAPI(table, record)
				apiOnly(record)
			},
		},
		{
			name: "an_api_only_query_said_to_need_read_api",
			bend: func(_ *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				apiOnly(record)
			},
			wants: []string{"the table says query project (issuesQuery) needs read_api and the request record says the query selects a field GitLab answers only to api; run `make gen-action-grants`"},
		},
		{
			name: "a_runner_route_said_to_need_api",
			bend: func(table *finegrained.Table, _ *actionrequests.Record, live *apilive.Document) {
				live.Routes = append(live.Routes, route("POST", "/runners", &apilive.RouteAuthorization{Skip: "runner_token_auth"}))
				table.Operations = append(table.Operations, finegrained.Operation{Name: "POST /runners", Classic: finegrained.ClassicAPI, Skip: true})
			},
			wants: []string{"the table says POST /runners needs api and the live record's skip reason runner_token_auth says GitLab authenticates it by another credential; run `make gen-action-grants`"},
		},
		{
			name: "another_credential_said_of_a_route_with_no_such_skip",
			bend: func(table *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				tableOp(table, "POST /projects/:id/things").Classic = finegrained.ClassicOtherCredential
				tableRow(table, "issue.thing").Classic = finegrained.ClassicOtherCredential
				recordAction(record, "issue.thing").Classic = "other-credential"
			},
			wants: []string{"the table says POST /projects/:id/things needs other-credential and a POST with the skip reason \"\" needs read_api or api; run `make gen-action-grants`"},
		},
		{
			name: "a_post_needing_read_api_and_a_route_declaring_nothing",
			bend: func(table *finegrained.Table, record *actionrequests.Record, live *apilive.Document) {
				tableOp(table, "POST /projects/:id/things").Classic = finegrained.ClassicReadAPI
				tableRow(table, "issue.thing").Classic = finegrained.ClassicReadAPI
				recordAction(record, "issue.thing").Classic = "read_api"
				live.Routes = append(live.Routes, route("DELETE", "/gone", nil))
				table.Operations = append(table.Operations, finegrained.Operation{Name: "DELETE /gone", Classic: finegrained.ClassicAPI})
			},
		},
		{
			name: "a_row_with_no_known_scope",
			bend: func(table *finegrained.Table, _ *actionrequests.Record, _ *apilive.Document) {
				tableRow(table, "later.get").Classic = finegrained.ClassicUnknown
			},
			wants: []string{"later.get needs no known classic scope in the table; run `make gen-action-grants`"},
		},
		{
			name: "a_row_the_record_disagrees_with",
			bend: func(_ *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				recordAction(record, "later.get").Classic = "api"
			},
			wants: []string{"the table says later.get needs read_api and the request record says api; run `make gen-action-grants`"},
		},
		{
			name: "a_row_needing_more_than_its_ways",
			bend: func(table *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				tableRow(table, "issue.get").Classic = finegrained.ClassicAPI
				recordAction(record, "issue.get").Classic = "api"
			},
			wants: []string{"the table says issue.get needs api and the least of its ways needs read_api; run `make gen-action-grants`"},
		},
		{
			name: "a_row_needing_less_than_its_ways",
			bend: func(table *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				tableRow(table, "issue.update").Classic = finegrained.ClassicReadAPI
				recordAction(record, "issue.update").Classic = "read_api"
			},
			wants: []string{"the table says issue.update needs read_api and the least of its ways needs api; run `make gen-action-grants`"},
		},
		{
			name: "a_row_needing_less_beside_a_denied_way",
			bend: func(table *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				tableRow(table, "namespace.list").Classic = finegrained.ClassicNoRequest
				recordAction(record, "namespace.list").Classic = "no-request"
			},
		},
	}
}

// liveRoute is the fixture record's route of one name, which a case bends in
// place. A name the fixture lacks is a mistake in the test, and the nil handed
// back for it stops the case where it is bent.
func liveRoute(live *apilive.Document, name string) *apilive.Route {
	for i := range live.Routes {
		if apilive.RouteName(&live.Routes[i]) == name {
			return &live.Routes[i]
		}
	}
	return nil
}

// TestInconsistencies_TheCatalog_IsCoveredRowForRow verifies the table is held
// to the catalog this tree builds, both ways: a row for an action the catalog
// no longer builds, and an action the catalog builds with no row, each named
// with the way out.
func TestInconsistencies_TheCatalog_IsCoveredRowForRow(t *testing.T) {
	actions := slices.DeleteFunc(fixtureCatalog(), func(action actionrequests.Action) bool { return action.ID == "issue.bulk" })
	actions = append(actions, actionrequests.Action{ID: "zz.new"})
	found := inconsistencies(fixtureTable(), fixtureRecord(), fixtureLive(), actions)
	want := []string{
		"issue.bulk has a row in the table and is built by no catalog; run `make gen-action-grants`",
		"zz.new is built by the catalog and has no row in the table; run `make gen-action-grants`",
	}
	if !slices.Equal(found, want) {
		t.Errorf("inconsistencies =\n%s\nwant\n%s", strings.Join(found, "\n"), strings.Join(want, "\n"))
	}
}

// TestBitSet_ReadsOneBitAndNothingPastTheEnd verifies a bit inside a word, a
// bit in a later word, and an index past the last word, which a table shorter
// than its permission list would otherwise read as a panic.
func TestBitSet_ReadsOneBitAndNothingPastTheEnd(t *testing.T) {
	bits := []uint64{1 << 3, 1 << 1}
	cases := []struct {
		name  string
		index int
		want  bool
	}{
		{name: "set_in_the_first_word", index: 3, want: true},
		{name: "clear_in_the_first_word", index: 4},
		{name: "set_in_a_later_word", index: 65, want: true},
		{name: "clear_in_a_later_word", index: 64},
		{name: "past_the_end", index: 128},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := bitSet(bits, testCase.index); got != testCase.want {
				t.Errorf("bitSet(%d) = %t, want %t", testCase.index, got, testCase.want)
			}
		})
	}
}
