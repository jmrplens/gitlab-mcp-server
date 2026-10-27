// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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

// TestAuthority_HeldElsewhereNote_OnlyWhenTheGrantHoldsEveryRefusedPermission
// verifies the per-session sentence for GitLab's missing-permissions refusal:
// it is written when the grant holds, at the boundary type the refusal names,
// every permission the refusal quotes by its words, and it says then that
// what is missing is this project or group in the grant, or its user's
// membership there. It is "" for a permission the grant does not hold, for
// one held at another boundary type only, for words no raw permission
// displays as, for a boundary that is not a project or a group, for no words
// at all, and for a phase A authority, which holds no grant.
func TestAuthority_HeldElsewhereNote_OnlyWhenTheGrantHoldsEveryRefusedPermission(t *testing.T) {
	granted := Evaluate(phaseBTable(), Grant{Scopes: []Scope{
		scope(AccessSelectedMemberships, NamespaceProject, "mr_approve"),
		scope(AccessSelectedMemberships, NamespaceGroup, "project_read"),
	}})
	cases := []struct {
		name      string
		authority *Authority
		boundary  string
		words     []string
		want      string
	}{
		{
			name: "a project permission held for other projects", authority: granted, boundary: "project", words: []string{"Merge Request: Approve"},
			want: "This session's token does hold [Merge Request: Approve], for other projects, so what this call is missing is this project in the grant, or a group above it, or its user's membership here.",
		},
		{
			name: "a group permission held for other groups", authority: granted, boundary: "group", words: []string{"Project: Read"},
			want: "This session's token does hold [Project: Read], for other groups, so what this call is missing is this group in the grant, or a group above it, or its user's membership here.",
		},
		{
			name: "two permissions both held", authority: granted, boundary: "project", words: []string{"Merge Request: Approve", "Project: Read"},
			want: "This session's token does hold [Merge Request: Approve, Project: Read], for other projects, so what this call is missing is this project in the grant, or a group above it, or its user's membership here.",
		},
		{name: "one of two permissions held", authority: granted, boundary: "project", words: []string{"Merge Request: Approve", "Issue Link: Read"}},
		{name: "a permission the grant does not hold", authority: granted, boundary: "project", words: []string{"Vulnerability: Read"}},
		{name: "held at a project only and refused at a group", authority: granted, boundary: "group", words: []string{"Merge Request: Approve"}},
		{name: "words no permission displays as", authority: granted, boundary: "project", words: []string{"Nothing: Here"}},
		{name: "a boundary that is not a project or a group", authority: granted, boundary: "user", words: []string{"User: Read"}},
		{name: "no words", authority: granted, boundary: "project"},
		{name: "a phase A authority", authority: Unevaluated(phaseBTable(), FallbackNone, ""), boundary: "project", words: []string{"Merge Request: Approve"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.authority.HeldElsewhereNote(tc.boundary, tc.words); got != tc.want {
				t.Errorf("HeldElsewhereNote(%q, %q) = %q, want %q", tc.boundary, tc.words, got, tc.want)
			}
		})
	}
}
