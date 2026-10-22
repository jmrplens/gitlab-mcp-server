package actioncatalog

import (
	"reflect"
	"slices"
	"testing"
)

// TestRequiredParamNames_ReadsEitherSpellingOfTheList verifies how a JSON
// Schema "required" list is read. It is a []any when the schema was decoded
// from JSON and a []string where an override wrote it in Go
// (toolutil.SchemaAnyOfRequired), and only a non-empty string names a
// parameter in either: anything else reaching a requirement would be
// published as a parameter no caller can supply and reported missing on every
// call of the action.
func TestRequiredParamNames_ReadsEitherSpellingOfTheList(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want []string
	}{
		{name: "a decoded list keeps its non-empty strings in order", raw: []any{"project_id", 42, "", map[string]any{}, "issue_iid"}, want: []string{"project_id", "issue_iid"}},
		{name: "a list written in Go keeps its non-empty strings in order", raw: []string{"title", "", "project_id"}, want: []string{"title", "project_id"}},
		{name: "a list naming nothing", raw: []any{42, ""}, want: nil},
		{name: "a string is not a list", raw: "project_id", want: nil},
		{name: "no list at all", raw: nil, want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RequiredParamNames(tc.raw); !slices.Equal(got, tc.want) {
				t.Fatalf("RequiredParamNames(%#v) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestRequiredParams_HoldsTheRootListAlone verifies the parameters every call
// needs: the schema's root "required" list, sorted and without duplicates. A
// name an anyOf or oneOf branch requires is not one of them, since a call
// needs only one branch; listing them here is what told a model that
// security_attribute.update needed a name, a description and a color at once
// (issue 1175).
func TestRequiredParams_HoldsTheRootListAlone(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
		want   []string
	}{
		{name: "no schema", schema: nil, want: nil},
		{name: "no root list", schema: map[string]any{"type": "object"}, want: nil},
		{
			name: "the root list sorted without duplicates, the alternatives left out",
			schema: map[string]any{
				"required": []any{"title", "project_id", "title"},
				"anyOf": []any{
					map[string]any{"required": []any{"file_name", "content"}},
					map[string]any{"required": []any{"files"}},
				},
			},
			want: []string{"project_id", "title"},
		},
		{name: "a root list written in Go", schema: map[string]any{"required": []string{"title", "project_id"}}, want: []string{"project_id", "title"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RequiredParams(tc.schema); !slices.Equal(got, tc.want) {
				t.Fatalf("RequiredParams() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestRequiredParams_LeavesTheSchemaAsItIs verifies that sorting the answer
// does not sort the schema it was read from. Every server of the process
// shares one input schema per action, so a list written in Go and returned
// without a copy would be reordered under every other reader the first time
// one of them asked.
func TestRequiredParams_LeavesTheSchemaAsItIs(t *testing.T) {
	root := []string{"title", "project_id"}
	branch := []string{"name", "color"}
	schema := map[string]any{"required": root, "anyOf": []any{map[string]any{"required": branch}}}

	RequiredParams(schema)[0] = "edited"
	RequiredParamAlternatives(schema)[0][0] = "edited"

	if !slices.Equal(root, []string{"title", "project_id"}) || !slices.Equal(branch, []string{"name", "color"}) {
		t.Fatalf("the schema's lists are now %v and %v, want them untouched", root, branch)
	}
}

// TestRequiredParamAlternatives_GroupsOfWhichACallNeedsOne verifies the
// alternative requirement groups: one per anyOf or oneOf branch, holding what
// that branch requires beyond the root list, in the order the branch declares
// it. A call needs every name of one group, not every group.
//
// A schema declaring both keywords at its root asks for a branch of each, so
// its groups are the combinations: every anyOf group joined with every oneOf
// group. Listing the groups of both keywords side by side would let a call
// that meets one anyOf branch and no oneOf branch pass as complete.
//
// A keyword one of whose branches the root list already satisfies asks
// nothing of a call, since every call satisfies that branch, so it contributes
// no group. Dropping only that branch, which is what the gitlab://tools
// manifest did, published the other branches as a requirement no call has to
// meet, and the execute validator, reading a branch that required nothing as
// no branch at all, refused calls the schema accepts.
func TestRequiredParamAlternatives_GroupsOfWhichACallNeedsOne(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
		want   [][]string
	}{
		{name: "no schema", schema: nil, want: nil},
		{name: "no alternatives", schema: map[string]any{"required": []any{"project_id"}}, want: nil},
		{
			name: "anyOf branches become groups and a branch that is not an object is skipped",
			schema: map[string]any{"anyOf": []any{
				"ignored",
				map[string]any{"required": []any{"file_name", "content"}},
				map[string]any{"required": []any{"files"}},
			}},
			want: [][]string{{"file_name", "content"}, {"files"}},
		},
		{
			name:   "a oneOf branch is asked for beside an anyOf branch",
			schema: map[string]any{"oneOf": []any{map[string]any{"required": []string{"b"}}}, "anyOf": []any{map[string]any{"required": []any{"a"}}}},
			want:   [][]string{{"a", "b"}},
		},
		{
			name: "both keywords give every combination of their branches, anyOf first",
			schema: map[string]any{
				"anyOf": []any{map[string]any{"required": []any{"a"}}, map[string]any{"required": []any{"b", "shared"}}},
				"oneOf": []any{map[string]any{"required": []any{"shared"}}, map[string]any{"required": []any{"c"}}},
			},
			want: [][]string{{"a", "shared"}, {"a", "c"}, {"b", "shared"}, {"b", "shared", "c"}},
		},
		{
			name: "a root name is not repeated inside a group",
			schema: map[string]any{
				"required": []any{"project_id"},
				"anyOf":    []any{map[string]any{"required": []any{"project_id", "name"}}, map[string]any{"required": []any{"color"}}},
			},
			want: [][]string{{"name"}, {"color"}},
		},
		{
			name: "a branch the root list satisfies leaves its keyword nothing to ask",
			schema: map[string]any{
				"required": []any{"project_id"},
				"anyOf": []any{
					map[string]any{"required": []any{"name"}},
					map[string]any{"required": []any{"project_id"}},
				},
			},
			want: nil,
		},
		{
			name:   "a branch requiring nothing leaves its keyword nothing to ask",
			schema: map[string]any{"anyOf": []any{map[string]any{"required": []any{"files"}}, map[string]any{"required": []any{}}}},
			want:   nil,
		},
		{
			name:   "a branch with no required list leaves its keyword nothing to ask",
			schema: map[string]any{"oneOf": []any{map[string]any{"properties": map[string]any{}}, map[string]any{"required": []any{"files"}}}},
			want:   nil,
		},
		{
			name: "a keyword with nothing to ask does not silence the other one",
			schema: map[string]any{
				"anyOf": []any{map[string]any{"required": []any{}}, map[string]any{"required": []any{"a"}}},
				"oneOf": []any{map[string]any{"required": []any{"b"}}, map[string]any{"required": []any{"c"}}},
			},
			want: [][]string{{"b"}, {"c"}},
		},
		{
			name:   "an empty anyOf leaves oneOf to answer",
			schema: map[string]any{"anyOf": []any{}, "oneOf": []any{map[string]any{"required": []any{"group_id"}}}},
			want:   [][]string{{"group_id"}},
		},
		{
			name:   "a keyword that is not a list is read as absent",
			schema: map[string]any{"anyOf": "invalid", "oneOf": []any{map[string]any{"required": []any{"content"}}}},
			want:   [][]string{{"content"}},
		},
		{
			name:   "an entry that does not name a parameter is dropped",
			schema: map[string]any{"oneOf": []any{map[string]any{"required": []any{"branch", 42, ""}}, map[string]any{"required": []any{"tag"}}}},
			want:   [][]string{{"branch"}, {"tag"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RequiredParamAlternatives(tc.schema); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("RequiredParamAlternatives() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestRequiredParamAlternativesExclusive_OnlyAOneOfAlone verifies when the
// groups [RequiredParamAlternatives] reads are exclusive, so that a call
// completes exactly one of them rather than at least one: when they come from
// a root oneOf, GitLab's exactly_one_of as toolutil.SchemaOneOfRequired writes
// it, and from no anyOf beside it.
//
// The groups were published as a choice of which a call satisfies at least
// one, which is what an anyOf asks, and a oneOf refuses a call satisfying two:
// a client reading the groups alone was told it could send a common name and
// a filter together to an LDAP link, which GitLab refuses. An anyOf beside the
// oneOf makes each group a combination of the two, and a call may complete
// two such groups through two anyOf branches, so those groups are not
// exclusive. A oneOf with nothing to ask is no group at all.
func TestRequiredParamAlternativesExclusive_OnlyAOneOfAlone(t *testing.T) {
	oneOf := []any{map[string]any{"required": []any{"cn"}}, map[string]any{"required": []any{"filter"}}}
	anyOf := []any{map[string]any{"required": []any{"name"}}, map[string]any{"required": []any{"path"}}}
	tests := []struct {
		name   string
		schema map[string]any
		want   bool
	}{
		{name: "no schema", schema: nil, want: false},
		{name: "no alternatives", schema: map[string]any{"required": []any{"group_id"}}, want: false},
		{name: "a oneOf alone", schema: map[string]any{"required": []any{"group_id"}, "oneOf": oneOf}, want: true},
		{name: "an anyOf alone", schema: map[string]any{"anyOf": anyOf}, want: false},
		{name: "an anyOf beside the oneOf", schema: map[string]any{"anyOf": anyOf, "oneOf": oneOf}, want: false},
		{name: "an anyOf with nothing to ask leaves the oneOf exclusive", schema: map[string]any{"anyOf": []any{map[string]any{"required": []any{}}}, "oneOf": oneOf}, want: true},
		{
			name: "a oneOf whose branch the root satisfies asks nothing",
			schema: map[string]any{
				"required": []any{"cn"},
				"oneOf":    oneOf,
			},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RequiredParamAlternativesExclusive(tc.schema); got != tc.want {
				t.Errorf("RequiredParamAlternativesExclusive() = %t, want %t", got, tc.want)
			}
		})
	}
}
