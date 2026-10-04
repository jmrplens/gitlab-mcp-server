package finegrained

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// Boundary is a set of GitLab's boundary types (app/models/authz/boundary.rb):
// the kinds of object a fine-grained permission is granted and checked at. It
// is the register's type, since the one rule that says what a grant covers
// lives there ([tenancy.CoverableAt], register row AUT-008), and this package
// writes its tables in the same vocabulary rather than in a copy of it.
type Boundary = tenancy.Boundary

// The four boundary types, one bit each, so a requirement that any of several
// boundaries may satisfy is one value.
const (
	BoundaryProject  = tenancy.BoundaryProject
	BoundaryGroup    = tenancy.BoundaryGroup
	BoundaryUser     = tenancy.BoundaryUser
	BoundaryInstance = tenancy.BoundaryInstance
)

// AllBoundaries is every boundary type, which is what a requirement whose
// boundary GitLab computes at run time with no declared type may resolve to.
const AllBoundaries = tenancy.AllBoundaries

// ParseBoundary reads one boundary type as GitLab spells it, and reports false
// for anything else.
func ParseBoundary(name string) (Boundary, bool) {
	for _, boundary := range [...]Boundary{BoundaryProject, BoundaryGroup, BoundaryUser, BoundaryInstance} {
		if name == boundary.String() {
			return boundary, true
		}
	}
	return 0, false
}

// Group is one requirement GitLab checks on its own: every raw permission in
// Perms, held at one boundary type among Any. A REST route has one for its
// primary declaration and one per additional scope; a GraphQL element has one
// per requirement group of its directives, with the permissions of that
// group's first directive only, as GitLab reads them
// (lib/gitlab/graphql/authz/granular_scope_authorization.rb).
type Group struct {
	// Perms are indices into [Table.Permissions].
	Perms []uint16
	Any   Boundary
}

// Operation is one wire request: a REST route or a GraphQL operation.
type Operation struct {
	// Name is the route as the live record spells it ("POST
	// /projects/:id/merge_requests/:merge_request_iid/approve"), or a GraphQL
	// operation's kind and root field with the document it comes from.
	Name string
	// Groups are indices into [Table.Groups]: a REST route's primary group and
	// additional scopes, or a GraphQL mutation's own groups.
	Groups []uint32
	// Skip is set when the route or mutation opts out of the check for any
	// reason: the grant does not decide it, GitLab does.
	Skip bool
	// Spine are indices into [Table.Elements]: the declared positions on the
	// answer spine, and every declared position whose denial a non-null chain
	// carries onto it. Every group of each must pass.
	Spine []uint32
	// OffSpine are the declared positions whose denial stays off the spine,
	// judged per grant for the degraded note and never for the listing.
	OffSpine []uint32
}

// Element is one GraphQL object position a document selects. At an abstract
// position (a union or an interface) it stands for the member types GitLab
// may resolve there, and is judged as the worst of them.
type Element struct {
	// Path is where the position sits in the answer, root field first
	// ("vulnerability.issueLinks.nodes").
	Path string
	// Type is the object type, or the union or interface name.
	Type string
	// Members are an abstract position's possible types at the recorded
	// version.
	Members []string
	// Groups are indices into [Table.Groups]; for an abstract position, every
	// member's groups.
	Groups []uint32
	// Undeclared is set for an enforced type with no directive (for an
	// abstract position, when any member is one): no fine-grained token
	// passes it.
	Undeclared bool
	// Effect is what a denial here does to the answer.
	Effect Effect
}

// Requirement is one action's. It is reachable when some path passes, and a
// path passes when every operation in it passes.
type Requirement struct {
	// ID is the canonical action ID.
	ID string
	// Paths are sets of indices into [Table.Operations], the mandatory
	// operations of each input-selected way the action can run.
	Paths [][]uint32
	// Denied is set when no fine-grained token reaches the action at the
	// recorded version.
	Denied *Denial
	// DeniedWays are the denials of the ways no fine-grained token runs,
	// each once, when another way is reachable: the action runs, except with
	// an input that selects one of these. They are kept out of Paths, which
	// is what a grant is judged against, since no grant passes them.
	DeniedWays []Denial
	// Degraded are indices into [Table.Elements]: undeclared positions off
	// every spine, served and always empty.
	Degraded []uint32
	// GraphQL is set when some mandatory operation of any way, a denied one
	// included, is a GraphQL one, which is what a null answer is worth a hint
	// for: an input that selects a denied GraphQL way is answered null too.
	GraphQL bool
	// Collection is set when the answer spine of any way, a denied one
	// included, ends in a list or a connection, which is what an empty answer
	// is worth a hint for.
	Collection bool
}

// Denial says why no fine-grained token reaches an action.
type Denial struct {
	Cause Cause `json:"cause"`
	// Element is the type, mutation or route that decides it.
	Element string `json:"element"`
	Effect  Effect `json:"effect"`
}

// Cause is why an action is withheld from a fine-grained token.
type Cause string

// The causes, each one a way GitLab refuses a fine-grained token at the
// recorded version whatever its grant, and the one cause a grant decides.
const (
	// CauseMutationUndeclared is a GraphQL mutation that declares nothing:
	// refused, nothing runs.
	CauseMutationUndeclared Cause = "graphql-mutation-undeclared"
	// CauseTypeUndeclared is an enforced GraphQL object type with no
	// directive on the answer spine.
	CauseTypeUndeclared Cause = "graphql-type-undeclared"
	// CausePayloadUndeclared is a declared mutation whose payload object is
	// undeclared, or reaches an undeclared non-null field: the write commits
	// and the answer is null.
	CausePayloadUndeclared Cause = "graphql-payload-undeclared"
	// CauseBoundaryUnresolvable is a type declared at a boundary the object
	// the action reaches never resolves to.
	CauseBoundaryUnresolvable Cause = "graphql-boundary-unresolvable"
	// CauseRESTTodo is a REST route whose declaration GitLab deferred.
	CauseRESTTodo Cause = "rest-todo"
	// CauseRESTUndeclared is a REST route that declares nothing.
	CauseRESTUndeclared Cause = "rest-undeclared"
	// CauseNotGranted is an action the token's grant does not reach, the one
	// cause a grant decides.
	CauseNotGranted Cause = "not-granted"
)

// GraphQL reports whether the cause is one only GraphQL answers with, which
// is the set a REST route serving the same object could remove (issue 1054).
func (c Cause) GraphQL() bool { return strings.HasPrefix(string(c), "graphql-") }

// Effect is what GitLab does to the answer when a fine-grained token is
// denied a position or an operation.
type Effect string

// The effects, each derived from GitLab's rules for the position's signature
// and its type's authorization.
const (
	// EffectNull is a nullable object answered null, with no error.
	EffectNull Effect = "null"
	// EffectNullOrEmpty is an object a boundary does not resolve for: null at
	// a single position, removed from a list.
	EffectNullOrEmpty Effect = "null-or-empty"
	// EffectRemoved is a list or connection whose denied items GitLab
	// removes, with no error.
	EffectRemoved Effect = "removed-items"
	// EffectListNull is a list of non-null items that one denied item nulls
	// whole, with an error.
	EffectListNull Effect = "list-null"
	// EffectRefused is a request refused before anything runs.
	EffectRefused Effect = "refused"
	// EffectCommittedThenNull is a write that commits and answers null.
	EffectCommittedThenNull Effect = "committed-then-null"
)

// Assignable is one permission a user grants a token, as GitLab names it.
type Assignable struct {
	Name string
	// Permissions are indices into [Table.Permissions], the raw permissions
	// it expands to.
	Permissions []uint16
	// Boundaries are the boundary types it can be granted at.
	Boundaries Boundary
	// Deprecated names are kept so a grant written under an older name still
	// reads.
	Deprecated bool
	// Grantable is set when a fine-grained token may hold it, which is every
	// assignable but those GitLab reserves for roles.
	Grantable bool
}

// PublicProject and PublicGroup index [Table.PublicAnonymous].
const (
	PublicProject = 0
	PublicGroup   = 1
)

// Table is the generated join, for one recorded GitLab version.
type Table struct {
	// Version is the release the record was taken from ("19.4.1-ee") and
	// Bucket its major.minor ("19.4"), which is what an instance's version is
	// matched against.
	Version string
	Bucket  string
	// Permissions are the raw permission names, sorted.
	Permissions []string
	// Display is, per raw permission, the words of the first assignable a
	// token can be granted that expands to it ("Merge Request: Approve"),
	// which is what the token creation page offers; empty when none can.
	Display []string
	// RefusalDisplay is, per raw permission, the words GitLab's own refusal
	// prints, deprecated names included.
	RefusalDisplay []string
	Assignables    []Assignable
	// PublicAnonymous are bit sets over Permissions, indexed by
	// [PublicProject] and [PublicGroup]: the anonymous policy GitLab evaluated
	// on a public project and group.
	PublicAnonymous [2][]uint64
	// PublicKnown is false when the record carries no evaluated set, in which
	// case nothing here refuses a call on the public question.
	PublicKnown bool
	Groups      []Group
	Operations  []Operation
	Elements    []Element
	// Actions are sorted by ID.
	Actions []Requirement
}

// Requirement returns the row for one canonical action ID, or nil when the
// table has none.
func (t *Table) Requirement(id string) *Requirement {
	_, row := t.requirementIndex(id)
	return row
}

// requirementIndex returns the index and the row for one canonical action ID,
// or a nil row when the table has none.
func (t *Table) requirementIndex(id string) (int, *Requirement) {
	if t == nil {
		return 0, nil
	}
	i, found := slices.BinarySearchFunc(t.Actions, id, func(row Requirement, target string) int {
		return strings.Compare(row.ID, target)
	})
	if !found {
		return 0, nil
	}
	return i, &t.Actions[i]
}

// Assignable returns the assignable permission of one name, deprecated ones
// included, or nil for a name the recorded version does not define.
func (t *Table) Assignable(name string) *Assignable {
	if t == nil {
		return nil
	}
	for i := range t.Assignables {
		if t.Assignables[i].Name == name {
			return &t.Assignables[i]
		}
	}
	return nil
}

// Knows reports whether every assignable name in names is one the recorded
// version defines. A grant naming anything else was written for another
// release, which is the vocabulary guard: a rename or a new assignable falls
// back to phase A rather than being read as nothing granted.
func (t *Table) Knows(names []string) bool {
	for _, name := range names {
		if t.Assignable(name) == nil {
			return false
		}
	}
	return true
}

// DisplayVersion is the version as a person reads it, without the edition
// suffix the record carries ("19.4.1").
func (t *Table) DisplayVersion() string {
	version, _, _ := strings.Cut(t.Version, "-")
	return version
}

// Bucket returns the major.minor of a version an instance reports, which is
// what a recorded table is chosen by, or "" for anything that does not start
// with two numbers.
func Bucket(version string) string {
	major, rest, _ := strings.Cut(version, ".")
	minor := leadingDigits(rest)
	if major == "" || leadingDigits(major) != major || minor == "" {
		return ""
	}
	return major + "." + minor
}

// leadingDigits returns the run of ASCII digits s starts with, "" when it
// starts with anything else.
func leadingDigits(s string) string {
	return s[:len(s)-len(strings.TrimLeftFunc(s, isDigit))]
}

// isDigit reports whether r is an ASCII digit.
func isDigit(r rune) bool { return '0' <= r && r <= '9' }
