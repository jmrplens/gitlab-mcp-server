package tenancy

import (
	"fmt"
	"testing"
)

// coverableCase is one access level and namespace kind, the boundary types
// GitLab's applicability table lets a scope of that shape cover for some
// object, and where in GitLab's source that answer is read
// (app/models/authz/granular_scope.rb and app/models/authz/boundary.rb at
// v19.4.1-ee, commit 26212baa).
type coverableCase struct {
	access    GrantAccess
	namespace NamespaceKind
	covers    Boundary
	source    string
}

// coverableTable is the whole input space of [CoverableAt] but the boundary,
// which the test walks for each row: every access level GitLab defines and the
// zero value beside them, by every namespace kind. A project or a group
// boundary has access selected_memberships (boundary.rb:68-70, :98-100), a
// standalone user or instance boundary has its own access
// (boundary.rb:13-16, :134-136), and a group boundary's namespace is the
// group (boundary.rb:76-78) while a project boundary's is its project
// namespace (boundary.rb:106-108).
func coverableTable() []coverableCase {
	const (
		standalone = "granular_scope.rb:46-47, :61-63 (standalone_access?: nil namespace and the same access)"
		selected   = "granular_scope.rb:48-49, :65-68 (namespace_access?: the boundary namespace's self and ancestors hold the scope's)"
		allMember  = "granular_scope.rb:48-49, :70-72 (all_memberships_access?: nil namespace and all_memberships)"
		noMatch    = "granular_scope.rb:44-53, :61-72 (no predicate holds for this shape)"
		unknown    = "granular_scope.rb:27-33 (no access level GitLab defines)"
	)
	return []coverableCase{
		{AccessUser, NamespaceNone, BoundaryUser, standalone},
		{AccessUser, NamespaceUser, 0, noMatch},
		{AccessUser, NamespaceGroup, 0, noMatch},
		{AccessUser, NamespaceProject, 0, noMatch},
		{AccessInstance, NamespaceNone, BoundaryInstance, standalone},
		{AccessInstance, NamespaceUser, 0, noMatch},
		{AccessInstance, NamespaceGroup, 0, noMatch},
		{AccessInstance, NamespaceProject, 0, noMatch},
		{AccessAllMemberships, NamespaceNone, BoundaryProject | BoundaryGroup, allMember},
		{AccessAllMemberships, NamespaceUser, 0, noMatch},
		{AccessAllMemberships, NamespaceGroup, 0, noMatch},
		{AccessAllMemberships, NamespaceProject, 0, noMatch},
		{AccessSelectedMemberships, NamespaceNone, 0, noMatch},
		// A user namespace is the ancestor of the projects in it and of no
		// group, which is what a personal projects scope is attached to.
		{AccessSelectedMemberships, NamespaceUser, BoundaryProject, selected},
		{AccessSelectedMemberships, NamespaceGroup, BoundaryProject | BoundaryGroup, selected},
		{AccessSelectedMemberships, NamespaceProject, BoundaryProject, selected},
		{AccessPersonalProjects, NamespaceNone, 0, noMatch},
		{AccessPersonalProjects, NamespaceUser, BoundaryProject, selected},
		{AccessPersonalProjects, NamespaceGroup, BoundaryProject | BoundaryGroup, selected},
		{AccessPersonalProjects, NamespaceProject, BoundaryProject, selected},
		{0, NamespaceNone, 0, unknown},
		{0, NamespaceUser, 0, unknown},
		{0, NamespaceGroup, 0, unknown},
		{0, NamespaceProject, 0, unknown},
	}
}

// TestCoverableAt_TranscribesGitLabsApplicabilityTable holds the promoted rule
// to GitLab's table over its whole input space: five access levels and the
// zero value, four namespace kinds, and every boundary set a byte can spell,
// so a set of several types or none is held to cover nothing as well as each
// single type to what the row says.
func TestCoverableAt_TranscribesGitLabsApplicabilityTable(t *testing.T) {
	for _, tc := range coverableTable() {
		t.Run(fmt.Sprintf("access%d_namespace%d", tc.access, tc.namespace), func(t *testing.T) {
			for boundary := range 256 {
				b := Boundary(boundary)
				want := single(b) && tc.covers&b != 0
				if got := CoverableAt(tc.access, tc.namespace, b); got != want {
					t.Errorf("CoverableAt(%d, %d, %08b) = %v, want %v (%s)", tc.access, tc.namespace, b, got, want, tc.source)
				}
			}
		})
	}
}

// single reports whether b is exactly one boundary type.
func single(b Boundary) bool { return b != 0 && b&(b-1) == 0 && b&AllBoundaries == b }

// TestCoverableAt_AllocatesNothing pins what a promoted rule may cost: it is
// called once per scope and boundary type on every evaluation of a grant, and
// allocates nothing.
func TestCoverableAt_AllocatesNothing(t *testing.T) {
	var covered bool
	allocs := testing.AllocsPerRun(100, func() {
		covered = CoverableAt(AccessSelectedMemberships, NamespaceGroup, BoundaryProject)
	})
	if allocs != 0 {
		t.Errorf("CoverableAt allocates %v times, want 0", allocs)
	}
	if !covered {
		t.Error("CoverableAt(selected_memberships, group, project) = false, want true")
	}
}

// TestBoundary_NamesAndString_SpellTheSetInBitOrder pins the spelling a refusal
// reads a requirement by: GitLab's lower-case names, in bit order, joined
// with " or ", and nothing for the empty set or a bit above the four types.
func TestBoundary_NamesAndString_SpellTheSetInBitOrder(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  Boundary
		want string
	}{
		{"project", BoundaryProject, "project"},
		{"group", BoundaryGroup, "group"},
		{"user", BoundaryUser, "user"},
		{"instance", BoundaryInstance, "instance"},
		{"all", AllBoundaries, "project or group or user or instance"},
		{"project and instance", BoundaryProject | BoundaryInstance, "project or instance"},
		{"none", 0, ""},
		{"above the four", 1 << 4, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.set.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
			if got := len(tc.set.Names()); tc.want == "" && got != 0 {
				t.Errorf("Names() has %d names, want none", got)
			}
		})
	}
}
