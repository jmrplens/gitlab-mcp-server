package finegrained

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
	// FallbackGrantTooLarge is a grant larger than this server reads.
	FallbackGrantTooLarge FallbackReason = "grant-too-large"
	// FallbackGrantShape is a grant holding something this server cannot
	// read without guessing.
	FallbackGrantShape FallbackReason = "grant-shape-unknown"
	// FallbackVersionUnreadable is an instance whose version was not read or
	// did not validate.
	FallbackVersionUnreadable FallbackReason = "version-unreadable"
	// FallbackVersionOutside is an instance whose version no recorded table
	// describes.
	FallbackVersionOutside FallbackReason = "version-outside-record"
	// FallbackUnknownPermission is a grant naming a permission the recorded
	// version does not define.
	FallbackUnknownPermission FallbackReason = "unknown-permission"
)

// Authority is what one fine-grained credential may do, for the table of one
// recorded GitLab version. It is immutable: a re-read builds a new one.
type Authority struct {
	table    *Table
	phase    Phase
	fallback FallbackReason
	// reported is the validated version the instance reported, "" when it
	// was not read.
	reported string
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
// hides a capability where a wrong "yes" surfaces as GitLab's own refusal. A
// row with a [Denial] is withheld from listing and from calls. In phase A
// every other row is allowed, with the positions every fine-grained token
// gets empty.
func (a *Authority) Decide(id string) Decision {
	row := a.table.Requirement(id)
	if row == nil {
		return Decision{Listed: true, Callable: true}
	}
	if row.Denied != nil {
		return Decision{Cause: row.Denied.Cause, Known: true}
	}
	return Decision{Listed: true, Callable: true, Degraded: row.Degraded, Known: true}
}
