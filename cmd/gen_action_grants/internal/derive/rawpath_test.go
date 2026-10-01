package derive

import (
	"go/token"
	"go/types"
	"testing"
)

// TestFold_NoExpression_IsUnknown verifies a missing argument folds to the
// unknown piece, which makes the whole path unreadable rather than wrong.
func TestFold_NoExpression_IsUnknown(t *testing.T) {
	d := &deriver{}
	if got := d.fold(nil, &frame{}, 0); len(got) != 1 || got[0] != unknownPiece {
		t.Errorf("fold(nil) = %q, want the unknown piece", got)
	}
}

// TestAssignmentsTo_NoBody_IsNone verifies a frame with no body names no
// assignment.
func TestAssignmentsTo_NoBody_IsNone(t *testing.T) {
	variable := types.NewVar(token.NoPos, nil, "path", types.Typ[types.String])
	if got := assignmentsTo(nil, nil, variable); got != nil {
		t.Errorf("assignmentsTo(nil body) = %v, want none", got)
	}
}

// TestNormalizePath_SpellsARouteTheWayClientGoDoes verifies the route
// spelling a folded path is compared in: the query and fragment dropped, the
// API prefix and the slashes around it trimmed, every segment holding a
// placeholder collapsed to one, and a path holding an unknown piece refused.
func TestNormalizePath_SpellsARouteTheWayClientGoDoes(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "projects/:/issues", want: "/projects/:/issues", ok: true},
		{raw: "/api/v4/projects/:/issues/", want: "/projects/:/issues", ok: true},
		{raw: "groups/x:y/members?page=1", want: "/groups/:/members", ok: true},
		{raw: "users#fragment", want: "/users", ok: true},
		{raw: "?page=1", want: "/", ok: true},
		{raw: "projects/" + unknownPiece, ok: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.raw, func(t *testing.T) {
			got, ok := normalizePath(testCase.raw)
			if got != testCase.want || ok != testCase.ok {
				t.Errorf("normalizePath(%q) = %q, %t; want %q, %t", testCase.raw, got, ok, testCase.want, testCase.ok)
			}
		})
	}
}
