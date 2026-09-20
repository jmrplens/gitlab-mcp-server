package testutil

import (
	"slices"
	"strings"
	"testing"
)

// TestTemplatePath_Identifiers_CollapseToPlaceholders verifies the rule the
// inventory depends on: two fixtures reaching the same endpoint have to become
// the same row, or the artifact counts fixtures instead of endpoints.
//
// The cases that are expected to stay verbatim are as much the contract as the
// ones that collapse. A branch name is an identifier and no shape rule can
// know it, so it is left where a reader can see it rather than guessed at.
//
// The placeholder's name comes from the collection in front of it, which is
// what stops one placeholder meaning two things in one file: the two boards
// cases are the same endpoint reached with the project spelled two ways, and
// under the positional naming this replaced the first of them called the board
// :id and the second called the project :id.
func TestTemplatePath_Identifiers_CollapseToPlaceholders(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"a numeric id and a nested iid", "/api/v4/projects/42/issues/7/notes", "/projects/:project_id/issues/:issue_id/notes"},
		{"a project addressed by encoded path", "/api/v4/projects/group%2Fproject/issues", "/projects/:project_id/issues"},
		{"an encoded path in upper case", "/api/v4/projects/group%2Fsub%2Fproject", "/projects/:project_id"},
		{"a path escaped twice over", "/api/v4/projects/group%252Fproject/issues", "/projects/:project_id/issues"},
		{"a full commit sha", "/api/v4/projects/1/repository/commits/0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b", "/projects/:project_id/repository/commits/:commit_id"},
		{"an abbreviated sha", "/api/v4/projects/1/repository/commits/0a1b2c3", "/projects/:project_id/repository/commits/:commit_id"},
		{"a uuid", "/api/v4/runners/550e8400-e29b-41d4-a716-446655440000/jobs", "/runners/:runner_id/jobs"},
		{"a board reached with a numeric project", "/api/v4/projects/7/boards/3", "/projects/:project_id/boards/:board_id"},
		{"the same board reached with a project fixture word", "/api/v4/projects/my-project/boards/3", "/projects/my-project/boards/:board_id"},
		{"a plural in ies", "/api/v4/projects/1/repositories/2", "/projects/:project_id/repositories/:repository_id"},
		{"a plural in ches", "/api/v4/projects/1/protected_branches/2", "/projects/:project_id/protected_branches/:protected_branch_id"},
		{"a singular collection segment", "/api/v4/projects/user/7", "/projects/user/:user_id"},
		{"an identifier no collection word precedes", "/api/v4/projects/1/packages/npm/pkg/-/2", "/projects/:project_id/packages/npm/pkg/-/:id"},
		{"a parent that is no word at all", "/api/v4/x/my-things/1", "/x/my-things/:id"},
		{"the graphql endpoint", "/api/graphql", "/graphql"},
		{"a path outside the api", "/health", "/health"},
		{"the api root alone", "/api/v4", "/"},
		{"a branch name, which no shape rule can catch", "/api/v4/projects/1/repository/branches/main", "/projects/:project_id/repository/branches/main"},
		{"a hex word with no digit, which is left alone", "/api/v4/projects/1/repository/branches/deadbeef", "/projects/:project_id/repository/branches/deadbeef"},
		{"a hex word with a digit, which is not", "/api/v4/projects/1/repository/branches/decade0", "/projects/:project_id/repository/branches/:branch_id"},
		{"a segment too short to be a sha", "/api/v4/templates/gitignores/Go", "/templates/gitignores/Go"},
		{"a segment too long to be a sha", "/api/v4/x/" + strings.Repeat("a1", 33), "/x/" + strings.Repeat("a1", 33)},
		{"a segment of exactly the longest sha", "/api/v4/projects/1/repository/commits/" + strings.Repeat("a1", shaMaxLength/2), "/projects/:project_id/repository/commits/:commit_id"},
		{"a uuid with a letter where a dash belongs", "/api/v4/x/550e8400ce29b-41d4-a716-446655440000", "/x/550e8400ce29b-41d4-a716-446655440000"},
		{"a uuid with a non-hex character", "/api/v4/x/550e8400-e29b-41d4-a716-44665544000g", "/x/550e8400-e29b-41d4-a716-44665544000g"},
		// A hex digit is bounded below as well as above: a character under the
		// digits and one between the digits and the letters are both outside
		// every range, and a check that only looked upwards would read either
		// as hexadecimal.
		{"a uuid with a character below the digits", "/api/v4/x/0123456f-abcd-ABCF-ef01-23456789ab!d", "/x/0123456f-abcd-ABCF-ef01-23456789ab!d"},
		{"a uuid with a character between the digits and the letters", "/api/v4/x/0123456f-abcd-ABCF-ef01-23456789ab:d", "/x/0123456f-abcd-ABCF-ef01-23456789ab:d"},
		// The digits and the hex letters are recognized to their ends, in both
		// cases, or a perfectly ordinary identifier is left in the path and
		// counted as an endpoint of its own. 0 and 9 bound the digits, a and f
		// the lowercase letters, A and F the uppercase ones, and the three
		// cases below are the first in this table to carry each end.
		{"an id carrying both ends of the digits", "/api/v4/projects/90/issues", "/projects/:project_id/issues"},
		{"a sha in upper case", "/api/v4/projects/1/repository/commits/0A1B2C3D4E5F6A7B8C9D0E1F2A3B4C5D6E7F8A9F", "/projects/:project_id/repository/commits/:commit_id"},
		{"a uuid spanning the hex letters in both cases", "/api/v4/runners/0123456f-abcd-ABCF-ef01-23456789abcd/jobs", "/runners/:runner_id/jobs"},
		// Hexadecimal until it is not: every character but one is a hex digit,
		// and that one is what keeps a branch name out of the identifier
		// count. A rule that accepted any letter here would collapse every
		// branch, tag and username in the inventory onto one row.
		{"a branch name that is hexadecimal until it is not", "/api/v4/projects/1/repository/branches/feature1", "/projects/:project_id/repository/branches/feature1"},
		// The prefix this trims is a string rather than a path, so it can end
		// inside a segment and leave the first segment holding an identifier
		// with no collection in front of it.
		{"an identifier that opens the path", "/api/v41", ":id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := templatePath(tt.path); got != tt.want {
				t.Errorf("templatePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestTemplatePath_RawValues_ComeBackUnderTheirPlaceholder verifies the half of
// the templating that the inventory's identifier counts rest on: a value it
// replaces is reported under the name it was replaced with.
//
// It is not the same question as the path above. The path says which endpoint
// was reached and the values say what stood in it, and with one fixture value a
// handler that reads the caller's project and one that has a project id written
// into it produce the same path. The repeated placeholder is the case that
// decides the shape of the answer: both values have to come back, because a map
// keeping one of them would report one distinct value where there are two,
// which is the direction that invents a lead nobody can act on.
func TestTemplatePath_RawValues_ComeBackUnderTheirPlaceholder(t *testing.T) {
	tests := []struct {
		name string
		path string
		want map[string][]string
	}{
		{
			name: "one identifier per collection",
			path: "/api/v4/projects/42/issues/7/notes",
			want: map[string][]string{":project_id": {"42"}, ":issue_id": {"7"}},
		},
		{
			name: "a project addressed by encoded path",
			path: "/api/v4/projects/group%2Fproject/issues",
			want: map[string][]string{":project_id": {"group%2Fproject"}},
		},
		{
			name: "two identifiers under one placeholder name",
			path: "/api/v4/x/my-things/1/my-things/2",
			want: map[string][]string{":id": {"1", "2"}},
		},
		{
			name: "a path with nothing to template",
			path: "/api/v4/projects",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := templatePath(tt.path)
			if len(got) != len(tt.want) {
				t.Fatalf("templatePath(%q) identifiers = %v, want %v", tt.path, got, tt.want)
			}
			for placeholder, want := range tt.want {
				if !slices.Equal(got[placeholder], want) {
					t.Errorf("templatePath(%q)[%q] = %v, want %v", tt.path, placeholder, got[placeholder], want)
				}
			}
		})
	}
}

// TestSingular_Segments_NameOneMemberOrNothing verifies the rule that names a
// placeholder, at the grain of the word rather than of a path.
//
// Two decisions live here and both are load-bearing for the inventory. What
// counts as a collection word decides whether a segment can name the
// identifier after it at all, and it is deliberately narrow: a lowercase word
// with digits and underscores in it, so the "-" of a package path, an
// identifier the shape rules already caught and a capitalised fixture word all
// fall through to the unnamed placeholder rather than lending it a name. And
// every trim leaves something behind, which is what the length guards are for:
// a word that is only a suffix is not a plural of anything, and "ies" naming
// the member "y" would put a placeholder in the file that no segment ever
// produced.
//
// The awkward answers are asserted rather than avoided. English plurals cannot
// be inverted without a dictionary and this carries none, so "status" comes out
// "statu"; the name is never compared with anything, and pinning what the rule
// really does is what keeps a later reader from taking the wrong half of that
// trade.
func TestSingular_Segments_NameOneMemberOrNothing(t *testing.T) {
	cases := []struct {
		name string
		word string
		want string
	}{
		{name: "a plain plural", word: "issues", want: "issue"},
		{name: "a plural in ies", word: "repositories", want: "repository"},
		{name: "the word ies itself, which is a plural of nothing", word: "ies", want: "ie"},
		{name: "a plural in ches", word: "branches", want: "branch"},
		{name: "a plural in sses", word: "classes", want: "class"},
		{name: "a plural in xes", word: "boxes", want: "box"},
		{name: "the word xes itself, which is a plural of nothing", word: "xes", want: "xe"},
		{name: "a singular already ending in ss", word: "access", want: "access"},
		{name: "a singular the rule gets wrong, and is allowed to", word: "status", want: "statu"},
		{name: "a singular word", word: "user", want: "user"},
		{name: "a word spanning every class the letters allow", word: "a_z09s", want: "a_z09"},
		{name: "a word opening on the first letter", word: "az", want: "az"},
		{name: "a word opening on the last letter", word: "zones", want: "zone"},
		{name: "nothing at all", word: "", want: ""},
		{name: "the separator a package path carries", word: "-", want: ""},
		{name: "a word with a hyphen in it", word: "my-things", want: ""},
		{name: "a capitalised word", word: "Go", want: ""},
		{name: "a word opening on an underscore", word: "_hidden", want: ""},
		{name: "a word opening on a digit", word: "2fa", want: ""},
		{name: "a word opening above the letters", word: "~x", want: ""},
		{name: "a word carrying a character above the letters", word: "a~b", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := singular(tc.word); got != tc.want {
				t.Errorf("singular(%q) = %q, want %q", tc.word, got, tc.want)
			}
		})
	}
}

// TestSummarizeGraphQL_Document_NamesOperationAndVariables verifies that a
// GraphQL request is described the way a REST one is: by what it asks for and
// which variables it declares, never by the values a fixture happened to send.
func TestSummarizeGraphQL_Document_NamesOperationAndVariables(t *testing.T) {
	tests := []struct {
		name      string
		document  string
		operation string
		variables []string
	}{
		{
			name:      "an anonymous query is named by its root field",
			document:  `query($projectPath: ID!, $first: Int) { project(fullPath: $projectPath) { id } }`,
			operation: "query project",
			variables: []string{"first", "projectPath"},
		},
		{
			name:      "a named mutation keeps its name",
			document:  `mutation DismissIt($id: VulnerabilityID!) { vulnerabilityDismiss(input: {id: $id}) { errors } }`,
			operation: "mutation DismissIt vulnerabilityDismiss",
			variables: []string{"id"},
		},
		{
			name:      "two root fields are both named",
			document:  `{ project { id } group { id } }`,
			operation: "query project,group",
		},
		{
			name:      "an inline fragment at the root is skipped",
			document:  `query { ... on Query { echo } }`,
			operation: "query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shape, ok := summarizeGraphQL(tt.document)
			if !ok {
				t.Fatalf("summarizeGraphQL(%q) reported no shape", tt.document)
			}
			if shape.Operation != tt.operation {
				t.Errorf("operation = %q, want %q", shape.Operation, tt.operation)
			}
			if !slices.Equal(shape.Variables, tt.variables) {
				t.Errorf("variables = %v, want %v", shape.Variables, tt.variables)
			}
		})
	}
}

// TestSummarizeGraphQL_NoOperation_IsNotRecorded verifies that a document with
// no operation to describe is left out rather than recorded as an empty one.
//
// A document that does not parse belongs to a test that declared
// [AllowInvalidGraphQL] and sent something malformed on purpose, and a fixture's
// typo has no place in an inventory of what this server sends.
func TestSummarizeGraphQL_NoOperation_IsNotRecorded(t *testing.T) {
	tests := []struct {
		name     string
		document string
	}{
		{"a document that does not parse", "query { unclosed"},
		{"a document that declares only a fragment", "fragment Fields on Project { id }"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if shape, ok := summarizeGraphQL(tt.document); ok {
				t.Errorf("summarizeGraphQL(%q) = %+v, want no shape", tt.document, shape)
			}
		})
	}
}
