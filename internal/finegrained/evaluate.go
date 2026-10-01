package finegrained

import (
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// The boundary types in bit order, which is the order the covered sets of an
// [Authority] are indexed in.
var boundaryOrder = [...]Boundary{BoundaryProject, BoundaryGroup, BoundaryUser, BoundaryInstance}

// publicBoundaries are the boundary types a public object answers anonymous
// callers at, each with the index of its set in [Table.PublicAnonymous] and in
// the covered sets.
var publicBoundaries = [...]struct {
	boundary Boundary
	public   int
	covered  int
}{
	{BoundaryProject, PublicProject, 0},
	{BoundaryGroup, PublicGroup, 1},
}

// bitset is a set of small indices, one bit each.
type bitset []uint64

// newBitset returns a set able to hold the indices below n.
func newBitset(n int) bitset { return make(bitset, (n+63)/64) }

// set adds index i.
func (b bitset) set(i int) { b[i/64] |= 1 << (uint(i) % 64) }

// has reports whether index i is in the set; an index past its end is not.
func (b bitset) has(i int) bool {
	word := i / 64
	return word < len(b) && b[word]&(1<<(uint(i)%64)) != 0
}

// Reading is what was read about a fine-grained token for its authority: the
// grant it was created with, or why the grant cannot be evaluated, and the
// version the instance reported.
type Reading struct {
	// Grant is the token's grant, as [DecodeGrant] read it.
	Grant Grant
	// Fallback is why the grant cannot be evaluated, or [FallbackNone] when it
	// can.
	Fallback FallbackReason
	// Version is the version the instance reported, validated by its reader,
	// and "" when it was not read or was not one.
	Version string
}

// Judge returns the authority one fine-grained token's reading is worth
// against a recorded table: phase B, the grant evaluated, when every guard
// holds, and phase A with the reason when one does not (issue 952).
//
// The guards, in the order a reason is chosen: the grant was read and decoded
// within its bounds; the instance reported a version; the version's
// major.minor is the table's; and every assignable name the grant holds is one
// the table defines, deprecated ones included, so a rename or a new
// assignable falls back rather than being read as nothing granted. Any of
// them failing is phase A with the newest record's verdict, naming the version
// the verdict comes from, since evaluating a grant against a table recorded
// for another release reads permission names that may mean something else
// there.
//
// One release outside the record is let in for the listing only: the
// prerelease of the minor right after the table's, which is what GitLab.com
// reports while releases stop at the table's minor (19.5.0-pre against
// 19.4). Such a session is listed what its grant reaches as the table
// declares it and may call everything phase A allows, so a requirement that
// changed in that one milestone is GitLab's own answer to the call rather than
// a refusal here with a stale name.
//
// It is pure: it reads nothing but its arguments, and returns a new authority.
func Judge(t *Table, r Reading) *Authority {
	switch {
	case r.Fallback != FallbackNone:
		return Unevaluated(t, r.Fallback, r.Version)
	case r.Version == "":
		return Unevaluated(t, FallbackVersionUnreadable, "")
	}
	listingOnly := Bucket(r.Version) != t.Bucket
	if listingOnly && !nextPrerelease(r.Version, t.Bucket) {
		return Unevaluated(t, FallbackVersionOutside, r.Version)
	}
	if unknown := t.unknownAssignables(r.Grant); len(unknown) > 0 {
		authority := Unevaluated(t, FallbackUnknownPermission, r.Version)
		authority.unknownCount = len(unknown)
		authority.unknownSample = sampleOf(unknown)
		return authority
	}
	authority := Evaluate(t, r.Grant)
	authority.reported = r.Version
	if listingOnly {
		authority.listingOnly = true
		authority.callable = newBitset(len(t.Actions))
		for i := range t.Actions {
			if t.Actions[i].Denied == nil {
				authority.callable.set(i)
			}
		}
	}
	return authority
}

// Rejudge returns what a fine-grained token's authority becomes after a
// re-read of its grant and of the instance version, and whether that replaces
// current.
//
// The authority is replaced only by one computed from reads that answered: a
// grant decoded within its bounds with a validated version, or a validated
// version whose major.minor differs from the one current was judged at (the
// instance was upgraded, which moves the token to that release's verdict,
// phase A when no table records it). Anything else keeps current: a version
// that was not read, and a grant that was not answered, was too large, could
// not be decoded or was refused. Falling back to phase A on a transient
// failure would make what a session is listed change with upstream
// availability rather than with its authorization (INV-009), and the grant
// cannot have changed in the meantime, since nothing edits one after the token
// is created.
func Rejudge(t *Table, current *Authority, r Reading) (*Authority, bool) {
	if r.Version == "" {
		return current, false
	}
	if r.Fallback != FallbackNone && current != nil && Bucket(r.Version) == Bucket(current.reported) {
		return current, false
	}
	return Judge(t, r), true
}

// nextPrerelease reports whether version is the prerelease of the minor right
// after bucket's, in the same major ("19.5.0-pre" after "19.4").
func nextPrerelease(version, bucket string) bool {
	if !strings.HasSuffix(version, "-pre") {
		return false
	}
	major, minor, _ := strings.Cut(bucket, ".")
	next, err := strconv.Atoi(minor)
	if err != nil {
		return false
	}
	return Bucket(version) == major+"."+strconv.Itoa(next+1)
}

// unknownSampleSize and unknownNameBytes bound what an authority keeps of the
// assignable names its grant held that the table does not define: the log line
// that says why the grant was not evaluated names at most this many, each cut
// to this many bytes, since the names are the minter's to choose.
const (
	unknownSampleSize = 3
	unknownNameBytes  = 64
)

// sampleOf keeps the first few names, each cut to its bound.
func sampleOf(names []string) []string {
	sample := make([]string, 0, min(len(names), unknownSampleSize))
	for _, name := range names[:min(len(names), unknownSampleSize)] {
		sample = append(sample, name[:min(len(name), unknownNameBytes)])
	}
	return sample
}

// unknownAssignables returns the assignable names a grant holds that the table
// does not define, each once, in the order the grant names them.
func (t *Table) unknownAssignables(g Grant) []string {
	index := t.assignableIndex()
	var unknown []string
	seen := map[string]bool{}
	for _, scope := range g.Scopes {
		for _, name := range scope.Permissions {
			if _, known := index[name]; !known && !seen[name] {
				seen[name] = true
				unknown = append(unknown, name)
			}
		}
	}
	return unknown
}

// assignableIndex maps each assignable name the table defines to its index.
// It is built per call rather than kept, because the table is generated data
// the binary builds nothing for, and a grant is evaluated once per entry
// build or revalidation, never per request.
func (t *Table) assignableIndex() map[string]int {
	index := make(map[string]int, len(t.Assignables))
	for i := range t.Assignables {
		index[t.Assignables[i].Name] = i
	}
	return index
}

// Evaluate returns the phase B authority of a grant against a table: which
// raw permissions the grant can cover at each boundary type, which actions it
// is listed and which it may call.
//
// What one scope covers is the register's rule, [tenancy.CoverableAt] (row
// AUT-008), applied for each boundary type; a scope's permissions are the raw
// permissions its assignable names expand to, a name the table does not define
// expanding to nothing. Then each action is judged: it is reachable when some
// path passes, a path when every operation in it passes, an operation when it
// opts out of the check or when every one of its groups and every group of
// every position on its answer spine passes, and a group when, at some boundary
// type it may be held at, every one of its permissions is covered. That is
// GitLab's own any boundary, all permissions, read existentially, since the
// target of a call is not known before it is made.
//
// An action is listed when the grant alone reaches it. It is callable when it
// is listed, or when GitLab would serve the call on a public project or group:
// for some boundary among those two, every permission of the group is covered
// or public to anonymous callers there, permission by permission, as GitLab's
// anonymous bypass enables each on its own. While the table carries no
// evaluated public set, a REST group held at a project or a group passes the
// call guard whatever the grant, so this server never refuses on the public
// question with a set it knows nothing about and GitLab judges the call. A
// denied action is neither listed nor callable.
//
// It is pure, keeps nothing of the grant, and allocates only the sets it
// returns and the transient index of assignable names.
func Evaluate(t *Table, g Grant) *Authority {
	authority := &Authority{table: t, phase: PhaseGranted}
	for i := range authority.covered {
		authority.covered[i] = newBitset(len(t.Permissions))
	}
	index := t.assignableIndex()
	for _, scope := range g.Scopes {
		for b, boundary := range boundaryOrder {
			if !tenancy.CoverableAt(scope.Access, scope.Namespace, boundary) {
				continue
			}
			for _, name := range scope.Permissions {
				if assignable, known := index[name]; known {
					for _, perm := range t.Assignables[assignable].Permissions {
						authority.covered[b].set(int(perm))
					}
				}
			}
		}
	}
	authority.listed = newBitset(len(t.Actions))
	authority.callable = newBitset(len(t.Actions))
	for i := range t.Actions {
		row := &t.Actions[i]
		switch {
		case row.Denied != nil:
		case authority.reaches(row, authority.groupListed):
			authority.listed.set(i)
			authority.callable.set(i)
		case authority.reaches(row, authority.groupCallable):
			authority.callable.set(i)
		}
	}
	return authority
}

// groupPass judges one group of an operation; rest says whether the operation
// is a REST route, which the call guard treats apart while no public set is
// recorded.
type groupPass func(group uint32, rest bool) bool

// reaches reports whether some path of row passes under pass.
func (a *Authority) reaches(row *Requirement, pass groupPass) bool {
	for _, path := range row.Paths {
		if a.pathPasses(path, pass) {
			return true
		}
	}
	return false
}

// pathPasses reports whether every operation of a path passes under pass.
func (a *Authority) pathPasses(path []uint32, pass groupPass) bool {
	for _, op := range path {
		if !a.operationPasses(&a.table.Operations[op], pass) {
			return false
		}
	}
	return true
}

// operationPasses reports whether one operation passes under pass: it opts out
// of the check, or every group of its own and of every position on its spine
// passes.
func (a *Authority) operationPasses(op *Operation, pass groupPass) bool {
	if op.Skip {
		return true
	}
	rest := !op.graphQL()
	for _, group := range op.Groups {
		if !pass(group, rest) {
			return false
		}
	}
	for _, element := range op.Spine {
		if !a.elementPasses(element, func(group uint32) bool { return pass(group, false) }) {
			return false
		}
	}
	return true
}

// elementPasses reports whether a GraphQL position passes: it is declared,
// and every one of its groups passes.
func (a *Authority) elementPasses(index uint32, pass func(uint32) bool) bool {
	element := &a.table.Elements[index]
	if element.Undeclared {
		return false
	}
	for _, group := range element.Groups {
		if !pass(group) {
			return false
		}
	}
	return true
}

// groupListed reports whether the grant alone covers a group: at some
// boundary type the group may be held at, every one of its permissions.
func (a *Authority) groupListed(index uint32, _ bool) bool {
	group := &a.table.Groups[index]
	for b, boundary := range boundaryOrder {
		if group.Any&boundary != 0 && a.coversAll(group.Perms, a.covered[b], nil) {
			return true
		}
	}
	return false
}

// groupCallable reports whether a call needing a group passes the call guard:
// the grant covers it, or at a project or a group boundary every permission is
// covered or public to anonymous callers, or no public set is recorded and the
// group is a REST route's held at a project or a group.
func (a *Authority) groupCallable(index uint32, rest bool) bool {
	if a.groupListed(index, rest) {
		return true
	}
	group := &a.table.Groups[index]
	for _, public := range publicBoundaries {
		if group.Any&public.boundary == 0 {
			continue
		}
		if !a.table.PublicKnown {
			if rest {
				return true
			}
			continue
		}
		if a.coversAll(group.Perms, a.covered[public.covered], a.table.PublicAnonymous[public.public]) {
			return true
		}
	}
	return false
}

// coversAll reports whether every permission is in covered or in public.
func (a *Authority) coversAll(perms []uint16, covered, public bitset) bool {
	for _, perm := range perms {
		if !covered.has(int(perm)) && !public.has(int(perm)) {
			return false
		}
	}
	return true
}

// graphQL reports whether the operation is a GraphQL one, which the generator
// names by its kind ("query project (vulnerabilityQuery)"); a REST route is
// named by its method and path.
func (op *Operation) graphQL() bool {
	return strings.HasPrefix(op.Name, "query ") || strings.HasPrefix(op.Name, "mutation ")
}

// missing returns the groups the grant fails on the path it comes closest to
// passing, in the order the path checks them, for the words a refusal names:
// the path failing the fewest groups, the first of those on a tie.
func (a *Authority) missing(row *Requirement) []uint32 {
	var best []uint32
	for i, path := range row.Paths {
		failed := a.failedGroups(path)
		if i == 0 || len(failed) < len(best) {
			best = failed
		}
	}
	return best
}

// failedGroups returns, each once, the groups of a path the grant alone does
// not cover: the operations' own and those of the positions on their spines.
func (a *Authority) failedGroups(path []uint32) []uint32 {
	var failed []uint32
	seen := map[uint32]bool{}
	add := func(group uint32) {
		if !seen[group] && !a.groupListed(group, false) {
			seen[group] = true
			failed = append(failed, group)
		}
	}
	for _, index := range path {
		op := &a.table.Operations[index]
		if op.Skip {
			continue
		}
		for _, group := range op.Groups {
			add(group)
		}
		for _, element := range op.Spine {
			for _, group := range a.table.Elements[element].Groups {
				add(group)
			}
		}
	}
	return failed
}

// degraded returns the positions served empty for this grant: the row's own,
// which no fine-grained token passes, then every declared position off a
// spine of its operations whose groups the grant does not cover, each once.
func (a *Authority) degraded(row *Requirement) []uint32 {
	out := row.Degraded
	seen := map[uint32]bool{}
	for _, element := range row.Degraded {
		seen[element] = true
	}
	for _, path := range row.Paths {
		for _, index := range path {
			for _, element := range a.table.Operations[index].OffSpine {
				if seen[element] || a.elementPasses(element, func(group uint32) bool { return a.groupListed(group, false) }) {
					continue
				}
				seen[element] = true
				out = append(slices.Clip(out), element)
			}
		}
	}
	return out
}
