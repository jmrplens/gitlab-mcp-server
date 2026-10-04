package finegrained

import (
	"strings"
	"testing"
)

// TestAuthority_DegradedNote_NamesEachEmptyPartAndWhy verifies the next step
// a served answer gets names each part GitLab leaves empty as the selection
// that reaches it, with the type checked there and why: a type that declares
// nothing is empty for every token, a declared one unless the grant holds what
// it needs, and an answer with nothing empty gets no note.
func TestAuthority_DegradedNote_NamesEachEmptyPartAndWhy(t *testing.T) {
	authority := Unevaluated(testTable(), FallbackNone, "")
	cases := []struct {
		name     string
		decision Decision
		want     []string
	}{
		{name: "nothing empty", decision: Decision{Listed: true, Callable: true}},
		{
			name:     "a part no fine-grained token gets",
			decision: authority.Decide("vulnerability.get"),
			want: []string{
				"GitLab 19.4.1 leaves part of this answer empty for a fine-grained personal access token, with no error: " +
					"`project { issueLinks { nodes } }` (the GraphQL type VulnerabilityIssueLink, on which it declares no " +
					"fine-grained permission). Empty there does not mean there is nothing.",
			},
		},
		{
			name:     "a declared part the grant does not reach",
			decision: Decision{Degraded: []uint32{0}},
			want: []string{
				"`project` (the GraphQL type Project, which needs the project or group permissions [Issue: Read, read_role_only] this token was not granted)",
			},
		},
		{
			name:     "two parts",
			decision: Decision{Degraded: []uint32{1, 0}},
			want:     []string{"declares no fine-grained permission); `project` (the GraphQL type Project, which needs"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := authority.DegradedNote(tc.decision)
			if len(tc.want) == 0 && got != "" {
				t.Errorf("DegradedNote = %q, want none", got)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("DegradedNote =\n%s\nwant it to contain\n%s", got, want)
				}
			}
			if strings.Contains(got, "gitlab_") || strings.Contains(got, "vulnerability.") {
				t.Errorf("DegradedNote names a tool or a dotted path: %s", got)
			}
		})
	}
}

// TestAuthority_NullNote_OnlyForAnActionThatReadsGraphQL verifies the note a
// not-found answer gets is given to an action that reads GraphQL, where GitLab
// answers a denied object with null, and to no other: a REST action's refusal
// is GitLab's own 403, and an action the table has no row for is unknown.
func TestAuthority_NullNote_OnlyForAnActionThatReadsGraphQL(t *testing.T) {
	authority := Unevaluated(testTable(), FallbackNone, "")
	cases := []struct {
		id   string
		want bool
	}{
		{id: "vulnerability.get", want: true},
		{id: "merge_request.approve", want: false},
		{id: "issue.list", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			got := authority.NullNote(tc.id)
			if (got != "") != tc.want {
				t.Fatalf("NullNote(%q) = %q, want a note %v", tc.id, got, tc.want)
			}
			if tc.want && !strings.Contains(got, "not found may mean this token cannot see it rather than that it does not exist") {
				t.Errorf("NullNote(%q) = %q", tc.id, got)
			}
		})
	}
}

// TestAuthority_EmptyNote_OnlyForAnAnswerThatIsAGraphQLList verifies the note
// an empty answer gets is given to an action whose answer is a GraphQL list or
// connection, which GitLab redacts without saying so, and to no other.
func TestAuthority_EmptyNote_OnlyForAnAnswerThatIsAGraphQLList(t *testing.T) {
	table := testTable()
	table.Actions[2].Collection = true
	authority := Unevaluated(table, FallbackNone, "")
	cases := []struct {
		id   string
		want bool
	}{
		{id: "vulnerability.get", want: true},
		{id: "merge_request.approve", want: false},
		{id: "issue.list", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			got := authority.EmptyNote(tc.id)
			if (got != "") != tc.want {
				t.Fatalf("EmptyNote(%q) = %q, want a note %v", tc.id, got, tc.want)
			}
			if tc.want && !strings.Contains(got, "an empty answer may mean this token cannot see them rather than that there are none") {
				t.Errorf("EmptyNote(%q) = %q", tc.id, got)
			}
		})
	}
}
