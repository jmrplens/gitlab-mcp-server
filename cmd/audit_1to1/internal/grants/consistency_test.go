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
	if found := inconsistencies(fixtureTable(), fixtureRecord(), fixtureLive()); len(found) != 0 {
		t.Errorf("inconsistencies = %v, want none", found)
	}
}

// TestInconsistencies_EachDisagreement_IsNamedWithTheWayOut verifies every
// check of the gate: the release, the vocabulary both ways, the public sets
// (known on one side only, and a permission on each boundary), a denial and a
// denied way naming something the record does not hold as the kind its cause
// says, and an action one artifact covers and the other does not. Each
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
			name: "an_action_on_one_side",
			bend: func(table *finegrained.Table, record *actionrequests.Record, _ *apilive.Document) {
				table.Actions = table.Actions[1:]
				record.Actions = append(record.Actions, actionrequests.RecordAction{ID: "zz.new"})
				table.Actions = append(table.Actions, finegrained.Requirement{ID: "zz.other"})
			},
			wants: []string{
				"branch.protected_list has an entry in the request record and no row in the table; run `make gen-action-grants`",
				"zz.new has an entry in the request record and no row in the table; run `make gen-action-grants`",
				"zz.other has a row in the table and no entry in the request record; run `make gen-action-grants`",
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			table, record, live := fixtureTable(), fixtureRecord(), fixtureLive()
			testCase.bend(table, &record, live)
			found := inconsistencies(table, record, live)
			if !slices.Equal(found, testCase.wants) {
				t.Errorf("inconsistencies =\n%s\nwant\n%s", strings.Join(found, "\n"), strings.Join(testCase.wants, "\n"))
			}
		})
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
