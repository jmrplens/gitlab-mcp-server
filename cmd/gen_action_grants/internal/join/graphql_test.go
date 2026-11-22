// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package join

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// parentRecord is a GraphQL half holding the shapes GitLab 19.6's
// parent_authorizes check reads: a checked parent, a skipped one, one that
// declares nothing, item types skipped with and without permissions, a
// connection and an edge around one, and an interface whose possible types
// agree or disagree.
func parentRecord() *apilive.GraphQLAuthz {
	skipped := func(permissions ...string) apilive.GraphQLType {
		return apilive.GraphQLType{Enforced: true, Granular: []apilive.Directive{
			{SkipReason: apilive.SkipParentAuthorizes, ParentAuthorizes: permissions},
		}}
	}
	return &apilive.GraphQLAuthz{
		Types: map[string]apilive.GraphQLType{
			"Project": enforced(directive("", "project", "read_project")),
			"Group": enforced(
				apilive.Directive{Permissions: []string{"read_group"}, BoundaryType: "group"},
				apilive.Directive{Permissions: []string{"read_group"}, BoundaryType: "project"},
			),
			"Skipped":   enforced(apilive.Directive{SkipReason: "not_a_resource"}),
			"Unbounded": enforced(apilive.Directive{Permissions: []string{"read_unbounded"}}),
			"Bare":      enforced(),
			"Note":      skipped("read_note", "read_issue"),
			"Silent":    skipped(),
			"Award":     skipped("read_award"),
			"OtherNote": skipped("read_issue", "read_note"),
			"NoteConnection": withFields(apilive.GraphQLType{}, map[string]string{
				"nodes": "[Note]", "pageInfo": "PageInfo!",
			}),
			"NoteEdge":        withFields(apilive.GraphQLType{}, map[string]string{"node": "Note"}),
			"EmptyConnection": {},
		},
		Abstract: map[string]apilive.AbstractType{
			"Agreeing":  {Kind: "interface", PossibleTypes: []string{"Note", "OtherNote"}},
			"Differing": {Kind: "union", PossibleTypes: []string{"Note", "Award"}},
			"Partial":   {Kind: "union", PossibleTypes: []string{"Note", "Silent"}},
			"Parents":   {Kind: "interface", PossibleTypes: []string{"Project", "Bare", "Group"}},
		},
	}
}

// TestParentChecks_ReadWhatGitLab196ChecksOnAFieldReturningAParentAuthorizedType
// verifies the join holds a field without a directive of its own that
// returns a type skipped with parent_authorizes to the type's permissions at
// the boundary its parent's directives locate, as
// Types::BaseField#parent_authorizes_check does from GitLab 19.6, and to
// nothing where that check runs none.
func TestParentChecks_ReadWhatGitLab196ChecksOnAFieldReturningAParentAuthorizedType(t *testing.T) {
	t.Parallel()
	both := finegrained.BoundaryGroup | finegrained.BoundaryProject
	for _, testCase := range []struct {
		name     string
		owner    string
		returned string
		want     []requirement
	}{
		{
			name: "the type's permissions, sorted, at the parent's boundary", owner: "Project", returned: "Note",
			want: []requirement{{perms: []string{"read_issue", "read_note"}, any: finegrained.BoundaryProject}},
		},
		{
			name: "every boundary type the parent's directives name", owner: "Group", returned: "Note",
			want: []requirement{{perms: []string{"read_issue", "read_note"}, any: both}},
		},
		{
			name: "through a connection to its items", owner: "Project", returned: "NoteConnection",
			want: []requirement{{perms: []string{"read_issue", "read_note"}, any: finegrained.BoundaryProject}},
		},
		{
			name: "through an edge to its node", owner: "Project", returned: "NoteEdge",
			want: []requirement{{perms: []string{"read_issue", "read_note"}, any: finegrained.BoundaryProject}},
		},
		{
			name: "an abstract type whose possible types declare the same permissions", owner: "Project", returned: "Agreeing",
			want: []requirement{{perms: []string{"read_issue", "read_note"}, any: finegrained.BoundaryProject}},
		},
		{name: "an abstract type whose possible types disagree checks nothing", owner: "Project", returned: "Differing"},
		{name: "an abstract type one possible type of which declares none checks nothing", owner: "Project", returned: "Partial"},
		{name: "a skip that declares no permissions checks nothing", owner: "Project", returned: "Silent"},
		{name: "a type that is not skipped is not the parent's to check", owner: "Project", returned: "Project"},
		{name: "a connection without items checks nothing", owner: "Project", returned: "EmptyConnection"},
		{
			name: "a parent whose directives name no boundary type is checked at every boundary", owner: "Unbounded", returned: "Note",
			want: []requirement{{perms: []string{"read_issue", "read_note"}, any: finegrained.AllBoundaries}},
		},
		{name: "a parent that skips locates no boundary", owner: "Skipped", returned: "Note"},
		{name: "a parent that declares nothing locates no boundary", owner: "Bare", returned: "Note"},
		{
			name: "each possible parent that locates a boundary adds its own", owner: "Parents", returned: "Note",
			want: []requirement{
				{perms: []string{"read_issue", "read_note"}, any: finegrained.BoundaryProject},
				{perms: []string{"read_issue", "read_note"}, any: both},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			a := &analyzer{authz: parentRecord()}
			got := a.parentChecks(&position{owner: testCase.owner, typeName: testCase.returned, field: "f"})
			if !slices.EqualFunc(got, testCase.want, func(x, y requirement) bool { return x.key() == y.key() }) {
				t.Errorf("parentChecks(%s returning %s) = %+v, want %+v", testCase.owner, testCase.returned, got, testCase.want)
			}
		})
	}
}

// TestJudge_AFieldWithADirectiveOfItsOwn_RunsNoParentCheck verifies the
// parent check is the fallback of a field declaring nothing: a field with
// its own directive is checked by that directive alone, and one skipping by
// nothing, as GitLab reads granular_scope_authorization first.
func TestJudge_AFieldWithADirectiveOfItsOwn_RunsNoParentCheck(t *testing.T) {
	t.Parallel()
	authz := parentRecord()
	authz.Fields = map[string][]apilive.Directive{
		"Project.declared": {directive("", "project", "read_project")},
		"Project.skipping": {{SkipReason: "not_a_resource"}},
	}
	a := &analyzer{authz: authz}
	for _, testCase := range []struct {
		field string
		want  [][]string
	}{
		{field: "declared", want: [][]string{{"read_project"}}},
		{field: "skipping"},
		{field: "plain", want: [][]string{{"read_issue", "read_note"}}},
	} {
		t.Run(testCase.field, func(t *testing.T) {
			t.Parallel()
			at := &position{owner: "Project", typeName: "Note", field: testCase.field, path: testCase.field}
			element, found := a.judge(at, map[*position]bool{})
			if !found {
				t.Errorf("judge(Project.%s) found no element, want one: Note is checked, if only to skip", testCase.field)
				return
			}
			var perms [][]string
			for _, group := range element.Groups {
				perms = append(perms, group.perms)
			}
			if !slices.EqualFunc(perms, testCase.want, slices.Equal) {
				t.Errorf("judge(Project.%s) groups = %v, want %v", testCase.field, perms, testCase.want)
			}
		})
	}
}
