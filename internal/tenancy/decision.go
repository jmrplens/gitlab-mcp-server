package tenancy

import "fmt"

// Question is which of the five questions of spec section 4.4 a decision
// answers. Every requirement answers exactly one.
type Question uint8

// The five questions.
const (
	// Identify is "who is this request?".
	Identify Question = iota + 1
	// Admit is "may this credential enter, and for how long?".
	Admit
	// Authorize is "what may it do?".
	Authorize
	// Allow is "what may it hold or spend?".
	Allow
	// End is "what happens when it exceeds, or when the server ends what it
	// held?".
	End
)

// Class says whether a decision's key agrees with the tenant definition, given
// the reason the code states for it (spec section 2.3). It is the class of the
// HTTP key: on stdio every per-caller key is the process (TEN-005).
type Class uint8

// The alignment classes of spec section 2.3.
const (
	// ClassT is keyed on the tenant itself.
	ClassT Class = iota + 1
	// ClassE is a property of the tenant or instance, resolved with the
	// entry's own credential.
	ClassE
	// ClassC is a property of the credential: validity, authority, admission.
	ClassC
	// ClassR bounds or ends something the entry itself holds.
	ClassR
	// ClassQ is scoped to a request, a session or a URI.
	ClassQ
	// ClassA is before admission, where no tenant exists yet.
	ClassA
	// ClassP is the process or the deployment.
	ClassP
	// ClassU is an allowance keyed on the entry whose stated reason names no
	// unit.
	ClassU
	// ClassD is an allowance keyed on the entry whose reason is per user or
	// per process, so one tenant with N credentials holds N units of it. A
	// class D row always carries a finding.
	ClassD
)

// Disposition is what the register does with a decision.
type Disposition uint8

// The dispositions.
const (
	// Valued decisions have their constants in values.go, and the sites alias
	// them.
	Valued Disposition = iota + 1
	// Ruled decisions are a predicate or a mapping, declared by symbol; nothing
	// moves.
	Ruled
	// Promoted decisions are rules whose function lives in the register.
	Promoted
	// Mechanism rows say how a decision is carried out. They choose no key and
	// no value.
	Mechanism
	// RequestBound rows are per-request bounds of spec section 3.2.10: declared
	// so that one list holds every requirement, and not owned.
	RequestBound
)

// Kind is the shape of a decision.
type Kind uint8

// The kinds of decision.
const (
	// Rule is a predicate or a mapping.
	Rule Kind = iota + 1
	// Ceiling is at most N held at once per key.
	Ceiling
	// Rate is a refill rate and a burst per key, or a pause keyed like one.
	Rate
	// Budget is events counted per key inside a window, then a block.
	Budget
	// Lifetime is how long an admission, a record, a build or a holding lasts.
	Lifetime
	// Bound is what one request may cost.
	Bound
	// Share is a portion promised to every key. [Validate] refuses it on any
	// key a caller can mint.
	Share
)

// IsAllowance reports whether the kind grants a key something to hold or
// spend: a ceiling, a rate, a budget or a share. Only an allowance can
// disagree with the tenant definition (see [Decision.Disagrees]).
func (k Kind) IsAllowance() bool {
	return k == Ceiling || k == Rate || k == Budget || k == Share
}

// Source is where a decision's value comes from.
type Source uint8

// The value sources.
const (
	// SourceNone is a decision with no value, or one nothing bounds (HLD-010).
	SourceNone Source = iota
	// Constant is a value no operator can change.
	Constant
	// Configurable is a value with a flag and a GITLAB_MCP_ variable.
	Configurable
	// EnvOnly is a value only an environment variable changes.
	EnvOnly
	// OptionOnly is a value only a Go option changes, which no operator can
	// reach.
	OptionOnly
	// Derived is a value computed from another decision's.
	Derived
)

// Zero is what a value of zero means for a decision (INV-015).
type Zero uint8

// The zero meanings.
const (
	// ZeroUnset has not been declared. [Validate] refuses it on a valued row.
	ZeroUnset Zero = iota
	// ZeroNotApplicable is a value no operator can set to zero.
	ZeroNotApplicable
	// ZeroRefused refuses startup.
	ZeroRefused
	// ZeroOff switches the decision off.
	ZeroOff
	// ZeroOffKeepsCount switches the ceiling off and keeps counting, because
	// another decision reads the count.
	ZeroOffKeepsCount
	// ZeroSelectsDefault replaces a zero with the default.
	ZeroSelectsDefault
)

// Capacity is what a structure or a ceiling does when it is full (VAL-006).
type Capacity uint8

// The capacity behaviors.
const (
	// CapacityNone states none: a structure with no capacity at all.
	CapacityNone Capacity = iota
	// RefuseNewcomer refuses what arrives.
	RefuseNewcomer
	// ReclaimOwnThenRefuse takes back something the same key holds, and
	// refuses when there is nothing to take.
	ReclaimOwnThenRefuse
	// EvictAcrossKeys takes a holding from another key to admit this one.
	// [Validate] requires a recorded decision for it (INV-005).
	EvictAcrossKeys
	// StopCounting stops tracking new keys rather than evicting a record.
	StopCounting
	// EvictOldest evicts the record nearest its expiry.
	EvictOldest
	// WaitThenRefuse queues for a bounded time, then refuses.
	WaitThenRefuse
)

// Malformed is what happens to a value that does not parse (INV-017).
type Malformed uint8

// The malformed-value policies.
const (
	// MalformedNone states none: a value no operator writes.
	MalformedNone Malformed = iota
	// RefuseStartup refuses to start.
	RefuseStartup
	// WarnKeepDefault warns and keeps the default.
	WarnKeepDefault
	// AcceptsAny is a switch every value of which means something (IDN-013:
	// anything but "off" enables it).
	AcceptsAny
)

// Era is a protocol revision a refusal applies in.
type Era uint8

// The eras.
const (
	// EraAny is every era the refusal's methods exist in.
	EraAny Era = iota
	// EraLegacy is protocol 2025-11-25, with initialize and optional sessions.
	EraLegacy
	// EraModern is protocol 2026-07-28, stateless.
	EraModern
	// EraStdio is the stdio transport, where no HTTP status exists.
	EraStdio
)

// Channel is how a refusal or an ending reaches the caller (spec 4.2.3).
type Channel uint8

// The channels.
const (
	// Gate is an HTTP status from the gate, before the SDK, with a JSON-RPC
	// body carrying the request id.
	Gate Channel = iota + 1
	// RPC is an in-band JSON-RPC error.
	RPC
	// ToolError is a tools/call result with isError set.
	ToolError
	// EmptyCompletion is a completion with no values.
	EmptyCompletion
	// Withheld is a narrowed surface whose answer names the cause.
	Withheld
	// Absent is a narrowed surface where the action or tool is missing.
	Absent
	// Unknown is a narrowed surface whose answer calls the action unknown.
	Unknown
	// ListenEnd is a subscriptions/listen ended with a completion result
	// carrying a watch-end reason.
	ListenEnd
	// SessionClose is a stateful session closed; later requests get 404.
	SessionClose
	// Silent is no visible refusal: uncounted, delayed or rebuilt.
	Silent
	// Startup is the process refusing to start.
	Startup
)

// String names the channel as spec section 4.2.3 does.
func (c Channel) String() string {
	switch c {
	case Gate:
		return "gate"
	case RPC:
		return "rpc"
	case ToolError:
		return "tool-error"
	case EmptyCompletion:
		return "empty-completion"
	case Withheld:
		return "withheld"
	case Absent:
		return "absent"
	case Unknown:
		return "unknown"
	case ListenEnd:
		return "listen-end"
	case SessionClose:
		return "session-close"
	case Silent:
		return "silent"
	case Startup:
		return "startup"
	default:
		return fmt.Sprintf("Channel(%d)", uint8(c))
	}
}

// Answer is the one class of next action a refusal tells its caller to take
// (INV-012, spec 4.3).
type Answer uint8

// The answer classes.
const (
	// RetryLater is come back later.
	RetryLater Answer = iota + 1
	// Reauthorize is the credential is bad.
	Reauthorize
	// WidenScope is the credential's scope is too narrow.
	WidenScope
	// AskOperator is the deployment's configuration decides.
	AskOperator
	// FixRequest is the request itself is wrong.
	FixRequest
	// StartOver is begin a new session or listen.
	StartOver
	// NoAnswer is none given.
	NoAnswer
)

// String names the answer class as spec section 4.3 does.
func (a Answer) String() string {
	switch a {
	case RetryLater:
		return "retry later"
	case Reauthorize:
		return "reauthorize"
	case WidenScope:
		return "widen the scope"
	case AskOperator:
		return "ask the operator"
	case FixRequest:
		return "fix the request"
	case StartOver:
		return "start over"
	case NoAnswer:
		return "none given"
	default:
		return fmt.Sprintf("Answer(%d)", uint8(a))
	}
}

// RetryAfter is where a gate refusal's Retry-After header takes its value.
type RetryAfter uint8

// The Retry-After sources.
const (
	// RetryAfterNone carries no Retry-After header.
	RetryAfterNone RetryAfter = iota
	// RetryAfterFixed is a register constant ([UpstreamRetryAfter]).
	RetryAfterFixed
	// RetryAfterUpstreamOrFixed is GitLab's own Retry-After, else the
	// constant (ADM-002).
	RetryAfterUpstreamOrFixed
	// RetryAfterLongestBlock is the longest block holding the request, at
	// least one second (AUB-001 to AUB-003).
	RetryAfterLongestBlock
)

// Role is what a [Site] is to its decision.
type Role uint8

// The site roles.
const (
	// Alias is a package-level const or var whose initializer is the register
	// constant [Site.Reads].
	Alias Role = iota + 1
	// Arg is a call argument: inside the function [Site.Name], the calls to
	// [Site.Call] pass the register constant at index [Site.Arg].
	Arg
	// Pin is a site that keeps its own literal, held equal to the register
	// constant [Site.Reads] (the second statements of issue 958's duplicated
	// defaults).
	Pin
	// Enforce is a function or var that applies the decision.
	Enforce
	// Refuse is a function, const or var that holds a refusal's text, or
	// builds its literal.
	Refuse
	// Reason is the declaration whose comment states the decision's reason.
	Reason
	// Charge is a function that charges an authentication budget.
	Charge
	// Derive is a symbol that computes a value of a key.
	Derive
)

// Site names a Go symbol, never a line, so that it survives edits above it.
type Site struct {
	// Pkg is the module-relative package directory, "cmd/server".
	Pkg string
	// Name is the symbol: "maxListenStreamsPerServer" for a package-level
	// const or var or a function, "listenLimits.middleware" for a method.
	Name string
	// Role is what the symbol is to the decision.
	Role Role
	// Reads is, for an Alias, Arg or Pin, the register constant it carries;
	// for an Enforce, a symbol its body must reference.
	Reads string
	// Call is, for an Arg, the callee; for a Charge, the helper counted.
	Call string
	// Arg is, for an Arg, the index of the argument; for an Alias whose
	// initializer is a composite literal, the index of the element.
	Arg int
	// Count is, for an Arg, the calls expected in the function; for a Charge,
	// the charged failures refused in the function.
	Count int
}

// Refusal is one way a decision declines a request or ends a holding, for a
// set of methods in one era.
type Refusal struct {
	// Methods are MCP methods, or "http" for the gate, "startup" for the
	// process refusing to start, and "eviction" or "expiry" for the server
	// ending what the caller held (see [Carriages]).
	Methods []string
	// Era is the protocol revision the refusal applies in.
	Era Era
	// Channel is how it reaches the caller.
	Channel Channel
	// Code is the JSON-RPC code, zero when the channel carries none.
	Code int
	// Status is the HTTP status, on the Gate channel only.
	Status int
	// RetryAfter is where a gate refusal's Retry-After comes from.
	RetryAfter RetryAfter
	// Challenge says a gate refusal carries a WWW-Authenticate challenge. A
	// 401 always does (RFC 9110 section 15.5.2); a 403 does when it is about
	// the credential's scope, and not when it is about the Origin, the Host or
	// the instance a request named.
	Challenge bool
	// Prefix is the stable leading text clients match on (CON-003), empty
	// when no text is carried or its start does not fold to a constant.
	Prefix string
	// Answer is the one class of next action (INV-012).
	Answer Answer
	// Charged are the IDs of the Budget rows it is charged to; empty charges
	// nothing (INV-007).
	Charged []string
	// At is the code that holds its text.
	At Site
	// Via is the constructor whose literal carries status, code and headers
	// when At passes it the text; zero when the literal is in At.
	Via Site
}

// Decision is one requirement of the specification, declared once.
//
// A row names its values by constant name, never by number: the numbers live
// in values.go alone, so a deliberate value change edits the constant and its
// frozen pin in the tests, two lines in one package (INV-021).
type Decision struct {
	// ID is the requirement id, "HLD-001".
	ID string
	// Question is the question of spec 4.4 it answers.
	Question Question
	// Kind is its shape.
	Kind Kind
	// Class is the alignment class of its HTTP key.
	Class Class
	// Disposition is what the register does with it.
	Disposition Disposition
	// Resource is what is held, spent or counted, in a few words.
	Resource string
	// Key is the HTTP key.
	Key Key
	// StdioKey is the key on stdio, KeyNone when the decision does not exist
	// there (VAL-011).
	StdioKey Key
	// StatedUnit is the unit the quoted reason names in its own words, KeyNone
	// when it names none.
	StatedUnit Key
	// ReasonUnit is the unit of what the reason protects or cites, as the
	// specification established it. [Decision.Disagrees] compares it with the
	// key.
	ReasonUnit Key
	// Reason is a short quotation of the code's own stated reason, verbatim.
	Reason string
	// ReasonAt is where that quotation lives.
	ReasonAt Site
	// ProtectsProcess says the reason is a process-wide resource, which is
	// what makes INV-004 ask for a process partner.
	ProtectsProcess bool
	// Table says it governs a structure holding one record per value of Key
	// (INV-010).
	Table bool
	// Partner is the ID of the process ceiling beside it (INV-004).
	Partner string
	// Values are the names of the register constants that are its values.
	Values []string
	// Functions are the names of the register functions that answer part of
	// it.
	Functions []string
	// Source is where its value comes from.
	Source Source
	// Flags are the command-line flags that set it.
	Flags []string
	// Envs are the environment variables that set it.
	Envs []string
	// Config are the internal/config fields that carry the configured value.
	Config []string
	// Malformed is what a value that does not parse does.
	Malformed Malformed
	// Zero is what a value of zero means.
	Zero Zero
	// OffWith is the ID of another row whose zero also switches this one off
	// (AUB-003's step is AUB-001's window, F-34, issue 958).
	OffWith string
	// AtCapacity is what happens when it is full.
	AtCapacity Capacity
	// Refusals are how it declines or ends, per method and era.
	Refusals []Refusal
	// Decided are the records that decided it, "ADR-0020", "issue 561".
	Decided []string
	// Findings are the findings it carries, "F-04"; [FindingIssue] names the
	// issue each is filed as.
	Findings []string
	// Sites are the symbols that decide, enforce, refuse or state it.
	Sites []Site
}

// Disagrees reports whether the decision is class D: an allowance keyed on a
// requester whose reason is about a different unit than its key, so a tenant
// holding N keys holds N units of something its own reason says is one.
//
// Only allowances can disagree: a rule or a lifetime keyed on the entry is
// the credential's own property. Units are compared as equivalence classes, so
// a reason about a credential agrees with a key on the entry. A reason about
// the process is met by a process partner beside the ceiling (INV-004).
func (d Decision) Disagrees() bool {
	if !d.Kind.IsAllowance() || d.Key.Axis() != AxisRequester || d.ReasonUnit == KeyNone {
		return false
	}
	if d.ReasonUnit.Unit() == d.Key.Unit() {
		return false
	}
	return d.ReasonUnit.Unit() != UnitProcess || d.Partner == ""
}

// Carries reports whether the decision carries the finding id.
func (d Decision) Carries(finding string) bool {
	return has(d.Findings, finding)
}

// RecordsDeparture reports whether one of the decision's findings records a
// departure from invariant, "INV-004". It is what lets a row pass a rule it
// breaks today: the finding says so, and names the issue that will decide it.
func (d Decision) RecordsDeparture(invariant string) bool {
	for _, id := range d.Findings {
		if f, ok := findingByID(id); ok && f.Records(invariant) {
			return true
		}
	}
	return false
}

// has reports whether list holds v. It is written out rather than taken from
// slices, because the leaf imports errors, fmt, strings and time and nothing
// else (see doc.go).
func has[T comparable](list []T, v T) bool {
	for _, item := range list { //nolint:modernize // slices.Contains would add an import the leaf is held not to have (TestPackage_ImportsTheStandardLibraryOnly)
		if item == v {
			return true
		}
	}
	return false
}

// Finding is one entry of the specification's findings register: a place where
// today's key disagrees with the tenant definition, or the answers disagree
// with each other, their reason or the protocol. None is fixed by the register;
// each is filed as an issue of its own.
type Finding struct {
	// ID is the finding's id in the specification, "F-04".
	ID string
	// Issue is the number of the issue it is filed as, in
	// https://github.com/jmrplens/gitlab-mcp-server/issues.
	Issue int
	// Invariants are the invariants of spec section 3.3 it records a departure
	// from. A rule of [Validate] accepts a row that breaks an invariant only
	// through a finding listed for that invariant here.
	Invariants []string
}

// Records reports whether the finding records a departure from invariant.
func (f Finding) Records(invariant string) bool {
	return has(f.Invariants, invariant)
}

// AllFindings returns the specification's thirty-five findings, in order, each
// with the issue it is filed as.
func AllFindings() []Finding {
	return []Finding{
		{ID: "F-01", Issue: 955},
		{ID: "F-02", Issue: 955, Invariants: []string{"INV-016"}},
		{ID: "F-03", Issue: 951, Invariants: []string{"INV-004", "INV-016"}},
		{ID: "F-04", Issue: 960, Invariants: []string{"INV-003"}},
		{ID: "F-05", Issue: 955, Invariants: []string{"INV-016"}},
		{ID: "F-06", Issue: 955, Invariants: []string{"INV-016"}},
		{ID: "F-07", Issue: 956, Invariants: []string{"INV-011", "INV-012"}},
		{ID: "F-08", Issue: 952, Invariants: []string{"INV-008"}},
		{ID: "F-09", Issue: 952, Invariants: []string{"INV-008"}},
		{ID: "F-10", Issue: 956, Invariants: []string{"INV-012"}},
		{ID: "F-11", Issue: 958},
		{ID: "F-12", Issue: 953},
		{ID: "F-13", Issue: 957, Invariants: []string{"INV-017"}},
		{ID: "F-14", Issue: 954},
		{ID: "F-15", Issue: 953},
		{ID: "F-16", Issue: 958, Invariants: []string{"INV-017"}},
		{ID: "F-17", Issue: 952},
		{ID: "F-18", Issue: 955, Invariants: []string{"INV-018"}},
		{ID: "F-19", Issue: 959},
		{ID: "F-20", Issue: 961},
		{ID: "F-21", Issue: 961},
		{ID: "F-22", Issue: 961},
		{ID: "F-23", Issue: 960},
		{ID: "F-24", Issue: 961},
		{ID: "F-25", Issue: 954},
		{ID: "F-26", Issue: 960},
		{ID: "F-27", Issue: 960},
		{ID: "F-28", Issue: 953},
		{ID: "F-29", Issue: 950, Invariants: []string{"INV-010"}},
		{ID: "F-30", Issue: 950, Invariants: []string{"INV-018"}},
		{ID: "F-31", Issue: 951, Invariants: []string{"INV-010", "INV-018"}},
		{ID: "F-32", Issue: 955},
		{ID: "F-33", Issue: 959},
		{ID: "F-34", Issue: 958, Invariants: []string{"INV-015"}},
		{ID: "F-35", Issue: 982, Invariants: []string{"INV-010"}},
	}
}

// findingByID looks a finding up by its id.
func findingByID(id string) (Finding, bool) {
	for _, f := range AllFindings() {
		if f.ID == id {
			return f, true
		}
	}
	return Finding{}, false
}

// FindingIssue returns the number of the issue the finding id is filed as, or
// zero for an id that names no finding.
func FindingIssue(id string) int {
	f, _ := findingByID(id)
	return f.Issue
}
