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
		{"a uuid with a letter where a dash belongs", "/api/v4/x/550e8400ce29b-41d4-a716-446655440000", "/x/550e8400ce29b-41d4-a716-446655440000"},
		{"a uuid with a non-hex character", "/api/v4/x/550e8400-e29b-41d4-a716-44665544000g", "/x/550e8400-e29b-41d4-a716-44665544000g"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := templatePath(tt.path); got != tt.want {
				t.Errorf("templatePath(%q) = %q, want %q", tt.path, got, tt.want)
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
