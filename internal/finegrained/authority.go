package finegrained

import (
	"log/slog"
	"sync"
)

// Phase is how much an [Authority] knows about the grant it stands for.
type Phase uint8

const (
	// PhaseUnknown is phase A: the grant was not evaluated, so only what no
	// fine-grained token can reach is withheld.
	PhaseUnknown Phase = iota
	// PhaseGranted is phase B: the grant was read and evaluated.
	PhaseGranted
)

// FallbackReason is why an [Authority] stayed in phase A.
type FallbackReason string

// The reasons, each with the words a withheld answer appends.
const (
	// FallbackNone is phase A with nothing to say: no grant was asked for.
	FallbackNone FallbackReason = ""
	// FallbackGrantUnreadable is a token that cannot read its own grant.
	FallbackGrantUnreadable FallbackReason = "grant-unreadable"
	// FallbackGrantUnanswered is a grant read the instance did not answer: a
	// transport failure, a timeout, a status that is no verdict on the read.
	// It is the one reason a later read can lift without anything about the
	// token changing, which the next revalidation does.
	FallbackGrantUnanswered FallbackReason = "grant-unanswered"
	// FallbackGrantTooLarge is a grant larger than this server reads.
	FallbackGrantTooLarge FallbackReason = "grant-too-large"
	// FallbackGrantShape is a grant holding something this server cannot
	// read without guessing.
	FallbackGrantShape FallbackReason = "grant-shape-unknown"
	// FallbackVersionUnreadable is an instance that answered the request for
	// its version and named none this server can read: it refused the
	// token's grant Metadata: Read, or it reported a string that does not
	// validate. Neither changes while the token and the instance stay what
	// they are, so the grant is not asked for.
	FallbackVersionUnreadable FallbackReason = "version-unreadable"
	// FallbackVersionUnanswered is a version read the instance did not
	// answer: a transport failure, a timeout, a status that is no verdict on
	// the read. Like an unanswered grant read, it says nothing about the
	// token, and a later read lifts it; the grant is not asked for, since no
	// grant is evaluated without a version.
	FallbackVersionUnanswered FallbackReason = "version-unanswered"
	// FallbackVersionOutside is an instance whose version no recorded table
	// describes.
	FallbackVersionOutside FallbackReason = "version-outside-record"
	// FallbackUnknownPermission is a grant naming a permission the recorded
	// version does not define.
	FallbackUnknownPermission FallbackReason = "unknown-permission"
)

// Authority is what one fine-grained credential may do, for the table of one
// recorded GitLab version. It is immutable: a re-read builds a new one.
//
// What it holds is sized by the table, never by the grant it was evaluated
// from, which it drops: four sets over the raw permissions and two over the
// actions, about a kilobyte, so a pool at its ceiling of entries holds about
// ten mebibytes of them. The parts of an answer a grant leaves empty are
// computed for the one action asked about rather than kept per action, which
// is what keeps it that small.
type Authority struct {
	table    *Table
	phase    Phase
	fallback FallbackReason
	// reported is the validated version the instance reported, "" when it
	// was not read.
	reported string
	// covered are, per boundary type in bit order, the raw permissions the
	// grant can cover there (phase B).
	covered [4]bitset
	// listed and callable are, per action index of the table, whether the
	// listing shows the action and whether the call guard passes it (phase
	// B).
	listed   bitset
	callable bitset
	// listingOnly is set for an instance one prerelease past the table: the
	// listing follows the grant, and every call phase A allows passes.
	listingOnly bool
	// unknownCount and unknownSample are, for a grant naming permissions the
	// table does not define, how many it named and the first few, each cut,
	// for the one log line that says why it was not evaluated.
	unknownCount  int
	unknownSample []string
}

// Unevaluated returns a phase A authority: one that withholds what no
// fine-grained token can reach at the table's version and allows the rest,
// carrying why the grant was not evaluated and the version the instance
// reported, for the words.
func Unevaluated(table *Table, reason FallbackReason, reported string) *Authority {
	return &Authority{table: table, phase: PhaseUnknown, fallback: reason, reported: reported}
}

// Table returns the table the authority decides by.
func (a *Authority) Table() *Table { return a.table }

// Phase returns how much the authority knows about the grant.
func (a *Authority) Phase() Phase { return a.phase }

// Fallback returns why the authority stayed in phase A, or [FallbackNone].
func (a *Authority) Fallback() FallbackReason { return a.fallback }

// Reported returns the validated version the instance reported, or "".
func (a *Authority) Reported() string { return a.reported }

// ListingOnly reports whether the grant decides the listing only, which is the
// one release past the table's [Judge] lets in.
func (a *Authority) ListingOnly() bool { return a.listingOnly }

// LogArgs are the key and value pairs a log line about this authority may
// carry: its phase, why it stayed in phase A, the major.minor the instance
// reported and the one the table was recorded at, and for a grant naming
// permissions the table does not define, how many and the first few, each
// cut. Nothing of the grant, the token or its id is among them.
func (a *Authority) LogArgs() []any {
	args := []any{
		"phase", a.phase.String(),
		"reason", string(a.fallback),
		"bucket", Bucket(a.reported),
		"recorded_bucket", a.table.Bucket,
	}
	if a.unknownCount > 0 {
		args = append(args, "unknown_permissions", a.unknownCount, "unknown_sample", a.unknownSample)
	}
	return args
}

// String names the phase for a log line: "A" or "B".
func (p Phase) String() string {
	if p == PhaseGranted {
		return "B"
	}
	return "A"
}

// UnknownPermissions returns how many assignable names the grant held that the
// table does not define, and at most three of them, each cut to 64 bytes: what
// the log line saying why the grant was not evaluated may name. Both are zero
// for any other authority.
func (a *Authority) UnknownPermissions() (count int, sample []string) {
	return a.unknownCount, a.unknownSample
}

// Decision is what [Authority.Decide] answers for one action ID.
type Decision struct {
	// Listed is whether tools/list, find and the manifest show the action.
	Listed bool
	// Callable is whether the call guard passes it.
	Callable bool
	// Cause is why not, when not.
	Cause Cause
	// Missing are indices into [Table.Groups]: in phase B, the groups the
	// grant fails, for the words.
	Missing []uint32
	// Degraded are indices into [Table.Elements]: the positions served empty
	// for this credential.
	Degraded []uint32
	// Known is false when the table has no row for the ID.
	Known bool
}

// Decide answers for one canonical action ID.
//
// An ID the table has no row for is allowed and reported unknown: a stale
// table on a deferred layer, or an action added since, is unknown authority,
// and unknown authority is served (INV-008, ADR-0018), since a wrong "no"
// hides a capability where a wrong "yes" surfaces as GitLab's own refusal. It
// is logged once per process at debug. A row with a [Denial] is withheld from
// listing and from calls. In phase A every other row is allowed, with the
// positions every fine-grained token gets empty.
//
// In phase B a row is listed and callable as [Evaluate] judged it. One the
// grant does not reach carries [CauseNotGranted] and the groups it fails on
// its closest path, for the words; one the call guard passes carries the
// positions served empty for this grant, its own undeclared ones and every
// declared one off a spine the grant does not cover, computed here for the one
// action asked about.
func (a *Authority) Decide(id string) Decision {
	index, row := a.table.requirementIndex(id)
	switch {
	case row == nil:
		logUnknownOnce(id)
		return Decision{Listed: true, Callable: true}
	case row.Denied != nil:
		return Decision{Cause: row.Denied.Cause, Known: true}
	case a.phase == PhaseUnknown:
		return Decision{Listed: true, Callable: true, Degraded: row.Degraded, Known: true}
	}
	decision := Decision{Listed: a.listed.has(index), Callable: a.callable.has(index), Known: true}
	if !decision.Listed {
		decision.Cause = CauseNotGranted
		decision.Missing = a.missing(row)
	}
	if decision.Callable {
		decision.Degraded = a.degraded(row)
	}
	return decision
}

// Lists reports whether a listing shows the action, which is
// [Authority.Decide]'s Listed without the rest of the decision: the groups a
// refusal names and the positions a served answer leaves empty are worded for
// the one action a caller runs, and a listing asks about every action the
// surface registers, so it asks this instead and computes neither.
func (a *Authority) Lists(id string) bool {
	index, row := a.table.requirementIndex(id)
	switch {
	case row == nil:
		logUnknownOnce(id)
		return true
	case row.Denied != nil:
		return false
	case a.phase == PhaseUnknown:
		return true
	}
	return a.listed.has(index)
}

// unknownIDs are the action IDs [Authority.Decide] has met with no row, each
// logged once per process.
var unknownIDs sync.Map

// logUnknownOnce logs at debug, the first time this process meets it, an
// action ID the table has no row for.
func logUnknownOnce(id string) {
	if _, seen := unknownIDs.LoadOrStore(id, true); !seen {
		slog.Debug("fine-grained table has no row for the action; serving it as unknown authority", "action", id)
	}
}
