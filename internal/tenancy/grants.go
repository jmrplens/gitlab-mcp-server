package tenancy

import "strings"

// This file holds the one decision issue 952 promotes into the register: what
// one granular scope of a fine-grained personal access token covers when the
// object a call will reach is not known yet (register row AUT-008).
//
// A grant is minted by its caller, scope by scope, and a scope is worth what
// GitLab's applicability table says it is (app/models/authz/granular_scope.rb
// at v19.4.1-ee, applicable_to_boundary? and the three predicates it calls).
// Before a call the target is unknown, so this server reads the table
// existentially: a scope covers a boundary type when it could apply to some
// object of that type. That reading over-approximates exactly where GitLab
// would need the target, and the over-approximation is the safe direction on
// REST, where a wrong "yes" is GitLab's own 403 or 404 on that one call.
//
// The vocabulary sits beside the rule rather than in a value file, the way
// key.go keeps the key vocabulary: these constants name the inputs of a
// function, and no layer is meant to alias one as a policy value.

// Boundary is a set of GitLab's boundary types (app/models/authz/boundary.rb):
// the kinds of object a fine-grained permission is granted and checked at.
type Boundary uint8

// The four boundary types, one bit each, so a requirement that any of several
// boundaries may satisfy is one value.
const (
	BoundaryProject Boundary = 1 << iota
	BoundaryGroup
	BoundaryUser
	BoundaryInstance
)

// AllBoundaries is every boundary type, which is what a requirement whose
// boundary GitLab computes at run time with no declared type may resolve to.
const AllBoundaries = BoundaryProject | BoundaryGroup | BoundaryUser | BoundaryInstance

// Names spells the set as GitLab's enum and boundary extractor do, lower case,
// in bit order.
func (b Boundary) Names() []string {
	spellings := [...]string{"project", "group", "user", "instance"}
	var names []string
	for i, name := range spellings {
		if b&(Boundary(1)<<i) != 0 {
			names = append(names, name)
		}
	}
	return names
}

// String joins the names with " or ", which is how a refusal reads a
// requirement any of them may satisfy.
func (b Boundary) String() string { return strings.Join(b.Names(), " or ") }

// GrantAccess is the access level of one granular scope, which says what
// namespaces it reaches (GranularScope::Access).
type GrantAccess uint8

// The access levels GitLab 19.4 defines. The zero value is none of them.
const (
	AccessPersonalProjects GrantAccess = iota + 1
	AccessSelectedMemberships
	AccessAllMemberships
	AccessUser
	AccessInstance
)

// NamespaceKind is the kind of namespace a scope is attached to.
type NamespaceKind uint8

// The namespace kinds a scope can carry.
const (
	// NamespaceNone is a scope with no namespace: all memberships, the user
	// and the instance.
	NamespaceNone NamespaceKind = iota
	// NamespaceUser is the creating user's own namespace, which a personal
	// projects scope carries.
	NamespaceUser
	// NamespaceGroup is a group, which covers the group and every project
	// under it.
	NamespaceGroup
	// NamespaceProject is one project's namespace.
	NamespaceProject
)

// CoverableAt reports whether a granular scope of the given access level,
// attached to a namespace of the given kind, can cover a permission checked at
// the given boundary type, for some object of that type (AUT-008).
//
// It is GitLab's applicability table read existentially:
//
//   - a user or an instance scope with no namespace covers only its own
//     boundary type, the standalone user or instance (standalone_access?);
//   - an all memberships scope with no namespace covers a project and a group
//     (all_memberships_access?, asked only of a project or a group boundary,
//     whose access is selected memberships);
//   - a selected memberships or personal projects scope covers a boundary
//     whose namespace has the scope's among its self and ancestors
//     (namespace_access?). A group boundary's namespace is the group and a
//     project boundary's is its project namespace, so a group scope covers the
//     group and every project under it, while a scope on one project, or on
//     the creating user's namespace, covers projects only and never a group;
//   - everything else answers false, as GitLab's nil namespace checks do: a
//     selected memberships or personal projects scope with no namespace, and
//     an all memberships, user or instance scope with one.
//
// boundary is one type; a set of several, or none, is covered by nothing. It is
// a pure function of three small enumerations: it allocates nothing and takes
// no lock, and its inputs are enumerated whole by its test.
func CoverableAt(access GrantAccess, namespace NamespaceKind, boundary Boundary) bool {
	switch access {
	case AccessUser:
		return namespace == NamespaceNone && boundary == BoundaryUser
	case AccessInstance:
		return namespace == NamespaceNone && boundary == BoundaryInstance
	case AccessAllMemberships:
		return namespace == NamespaceNone && membershipBoundary(boundary)
	case AccessSelectedMemberships, AccessPersonalProjects:
		switch namespace {
		case NamespaceGroup:
			return membershipBoundary(boundary)
		case NamespaceProject, NamespaceUser:
			return boundary == BoundaryProject
		default:
			return false
		}
	default:
		return false
	}
}

// membershipBoundary reports whether boundary is one of the two types GitLab
// judges a membership scope at, a project or a group.
func membershipBoundary(boundary Boundary) bool {
	return boundary == BoundaryProject || boundary == BoundaryGroup
}
