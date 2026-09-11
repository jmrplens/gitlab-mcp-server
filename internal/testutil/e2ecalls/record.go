// The line shapes a shard holds, and the one envelope they are written in.
// Every field name here is the contract between the harness that writes a
// shard and cmd/audit_e2e_coverage that reads it.

package e2ecalls

import "fmt"

const (
	// DirEnv names the directory the end-to-end harness records its calls
	// into. Recording is off unless it is set, so an ordinary suite run pays
	// nothing for it.
	//
	// The path must be absolute. A test binary runs with its own package
	// directory as the working directory, so a relative path would drop one
	// shard under each of the suite's package directories and the merge would
	// find none of them. A relative value is refused rather than resolved,
	// because resolving it would produce exactly that scattering silently. The
	// sibling GITLAB_MCP_TEST_INVENTORY_DIR is refused on the same terms.
	//
	// It carries the project's prefix and is not one of the settings
	// internal/config resolves: it configures the test harness, cmd/server
	// never reads it, and there is no legacy spelling of it to warn anybody
	// about. That is the standing the developer-only EVAL_SURFACE_* variables
	// have.
	DirEnv = "GITLAB_MCP_TEST_E2E_CALLS_DIR"

	// SchemaVersion is the version every line carries. A reader refuses a line
	// written under another one rather than guessing which fields it holds:
	// these shards are read by a command in the same tree, so a mismatch means
	// a stale artifact and not an old peer to be tolerated.
	SchemaVersion = 1

	// ShardPattern is the [os.CreateTemp] pattern a shard file is named with.
	// One process writes one shard, so package binaries running side by side
	// never write to the same file and no cross-process locking is needed.
	ShardPattern = shardPrefix + "*" + shardExt

	// shardPrefix and shardExt bracket the name of a shard. [Read] matches
	// them directly instead of calling [path/filepath.Match] on ShardPattern,
	// so it has no pattern error to discard.
	shardPrefix = "calls-"
	shardExt    = ".jsonl"
)

// The five line types. Every [Record] carries exactly one of them, and the
// reader refuses a record whose payload is not the one its type names.
const (
	// TypeRun names the line a test package writes once: which runtime it
	// found, what it required, and whether it went on to run.
	TypeRun = "run"
	// TypeSession names the line one server configuration writes when it
	// starts, holding what that session serves.
	TypeSession = "session"
	// TypeCall names the line one MCP call writes: what a test asked for and
	// what came back.
	TypeCall = "call"
	// TypeDispatch names the line the server's own span writes: what it
	// decided to run for a call the harness made.
	TypeDispatch = "dispatch"
	// TypeSkip names the line a skipped test writes, with its reason.
	TypeSkip = "skip"
)

// What a call was made for. A call made to build or tear down fixture state is
// worth less as coverage than one a test asserted on, so the purpose is
// recorded rather than inferred from the test name.
const (
	// PurposeTest is a call the test body made and asserted on.
	PurposeTest = "test"
	// PurposeCleanup is a call made from a cleanup, after the test body.
	PurposeCleanup = "cleanup"
	// PurposeSweep is a call made by a sweep over what a session serves.
	PurposeSweep = "sweep"
	// PurposeRaw is a call made through the harness's raw escape hatch, where
	// the protocol rather than the action is what the test is about.
	PurposeRaw = "raw"
)

// What the caller expected. Anything other than these two names a harness
// failure class, which is why the field is a string here: the classes belong
// to the harness, and this package is the record rather than the vocabulary.
const (
	// ExpectationOK is a call the test expected to succeed.
	ExpectationOK = "ok"
	// ExpectationAny is a call whose outcome the test did not constrain.
	ExpectationAny = "any"
)

// What came back. A refusal carries its reason, which is why it is a prefix
// and not a value: [RefusedOutcome] spells one.
const (
	// OutcomeOK is a call the server answered without an error.
	OutcomeOK = "ok"
	// OutcomeToolError is a call answered with a tool error result.
	OutcomeToolError = "tool_error"
	// OutcomeProtocolError is a call answered with a JSON-RPC error.
	OutcomeProtocolError = "protocol_error"
	// OutcomeTransportError is a call that never got an answer.
	OutcomeTransportError = "transport_error"
	// OutcomePreview is a safe-mode preview: the server answered the call with
	// the preview of a mutation it did not make.
	OutcomePreview = "preview"
	// OutcomeRefusedPrefix starts the outcome of a call the server refused.
	// The refusal reason follows it.
	OutcomeRefusedPrefix = "refused:"
)

// Whether a package ran at all. A refused run is recorded rather than left
// silent, because a missing run line and a run that refused to start are
// different answers to "was this runtime exercised".
const (
	// RunStarted is a package whose runtime met its requirement.
	RunStarted = "started"
	// RunRefused is a package that found a runtime it cannot run on.
	RunRefused = "refused"
)

// How the test a call belongs to ended. Records are buffered per test and
// written from its first-registered cleanup, so every call knows this by the
// time it is written.
const (
	// StatusPassed is a test that ended without a failure.
	StatusPassed = "passed"
	// StatusFailed is a test that failed.
	StatusFailed = "failed"
	// StatusSkipped is a test that skipped.
	StatusSkipped = "skipped"
)

// RefusedOutcome spells the outcome of a call the server refused for the given
// reason, as [OutcomeRefusedPrefix] followed by that reason.
//
// It exists so the writer and the reader agree on one spelling: the coverage
// audit classifies a refusal by this prefix, and a second hand-written
// concatenation is how the two would come to disagree.
func RefusedOutcome(reason string) string {
	return OutcomeRefusedPrefix + reason
}

// Line is one thing a shard can hold. The set is closed on purpose: [Run],
// [Session], [Call], [Dispatch] and [Skip] are the only implementations,
// because the reader dispatches on [Record.Type] and a sixth shape it has
// never heard of would be dropped or guessed at.
type Line interface {
	// record wraps the line in the envelope it is written as.
	record() Record
}

// Record is one line of a shard: the schema it was written under, which line
// it is, and exactly one payload.
//
// It is an envelope rather than one flat struct because the five lines share
// almost no fields, and a flat struct would answer "which of these fields
// apply" with a convention instead of with the type.
type Record struct {
	// Schema is [SchemaVersion] as of the run that wrote the line.
	Schema int `json:"schema"`
	// Type names which payload is set: one of the Type* constants.
	Type string `json:"type"`
	// Run is set when Type is [TypeRun].
	Run *Run `json:"run,omitempty"`
	// Session is set when Type is [TypeSession].
	Session *Session `json:"session,omitempty"`
	// Call is set when Type is [TypeCall].
	Call *Call `json:"call,omitempty"`
	// Dispatch is set when Type is [TypeDispatch].
	Dispatch *Dispatch `json:"dispatch,omitempty"`
	// Skip is set when Type is [TypeSkip].
	Skip *Skip `json:"skip,omitempty"`
}

// payloadPresent answers, per line type, whether the record carries the
// payload that type names.
//
// It is a table rather than a switch so that adding a line type is one entry
// in one place: the reader asks it both questions it has, whether the type is
// known and whether its payload is there.
var payloadPresent = map[string]func(Record) bool{
	TypeRun:      func(r Record) bool { return r.Run != nil },
	TypeSession:  func(r Record) bool { return r.Session != nil },
	TypeCall:     func(r Record) bool { return r.Call != nil },
	TypeDispatch: func(r Record) bool { return r.Dispatch != nil },
	TypeSkip:     func(r Record) bool { return r.Skip != nil },
}

// validate reports what is wrong with a record read back from a shard.
//
// A shard is machine-written, so every one of these means the artifact is
// stale or truncated rather than that a caller made a mistake. Reporting it is
// what keeps a coverage figure from being computed over lines nobody can read.
func (r Record) validate() error {
	if r.Schema != SchemaVersion {
		return fmt.Errorf("schema %d is not %d: the shard was written by another version of this package", r.Schema, SchemaVersion)
	}
	present, known := payloadPresent[r.Type]
	if !known {
		return fmt.Errorf("unknown line type %q", r.Type)
	}
	if !present(r) {
		return fmt.Errorf("line type %q carries no payload", r.Type)
	}
	return nil
}

// FixtureProfile is what a runtime had available to the test package that ran
// on it.
//
// It travels with the run line because a scenario that needs a CI runner is
// absent rather than failing when there is none, and a coverage figure that
// cannot tell those apart is not worth reading.
type FixtureProfile struct {
	// Runner is whether a CI runner was registered.
	Runner bool `json:"runner"`
	// FixtureService is whether the compose fixture service was up.
	FixtureService bool `json:"fixture_service"`
	// Bitbucket is whether the Bitbucket import source was up.
	Bitbucket bool `json:"bitbucket"`
	// GHToken is whether a GitHub token was configured for import scenarios.
	GHToken bool `json:"gh_token"`
	// Seeds names the per-consumer seeds the setup script created.
	Seeds []string `json:"seeds,omitempty"`
}

// Run is what one test package found when it started, written once per
// package.
//
// It is the denominator of everything else in the shard: a call means nothing
// without the runtime it was made against, since the served catalog, the
// pruned schemas and the actions that exist at all follow from the edition and
// the tier.
type Run struct {
	// Package is the test package that wrote the line.
	Package string `json:"package"`
	// Requirement is the runtime the package asked for.
	Requirement string `json:"requirement"`
	// Edition is what the instance reported: a community or enterprise build.
	Edition string `json:"edition"`
	// Tier is the licensing tier the catalog was built at.
	Tier string `json:"tier"`
	// TierConfirmed is whether the tier came from the instance license rather
	// than from a setting. An unconfirmed tier means the catalog may hold
	// actions the instance will refuse.
	TierConfirmed bool `json:"tier_confirmed"`
	// GitLabVersion is the version the instance reported.
	GitLabVersion string `json:"gitlab_version"`
	// RunID is the identifier every name this run created is scoped to, which
	// is also what the orphan sweep deletes by.
	RunID string `json:"run_id"`
	// Commit is the revision under test, from E2E_COMMIT.
	Commit string `json:"commit,omitempty"`
	// Filter is the -run expression the binary was given, empty when it ran
	// everything. A partial run is not a coverage claim about the rest.
	Filter string `json:"filter,omitempty"`
	// Fixtures is what the runtime had available.
	Fixtures FixtureProfile `json:"fixtures"`
	// Status is [RunStarted] or [RunRefused].
	Status string `json:"status"`
	// Reason says why a refused run refused. It is empty on a started run.
	Reason string `json:"reason,omitempty"`
}

// Session is one server configuration, written when that session starts.
//
// What a session serves is the denominator the coverage audit divides by: an
// action no session served is absent rather than untested, and the two are
// different findings.
type Session struct {
	// Label names the session in the call lines that reference it.
	Label string `json:"label"`
	// Surface is dynamic, meta or individual.
	Surface string `json:"surface"`
	// Mode is default, read-only or safe.
	Mode string `json:"mode"`
	// Capabilities is the resource and prompt surface: full or minimal.
	Capabilities string `json:"capabilities"`
	// Transport is how the harness reached the binary.
	Transport string `json:"transport"`
	// Tools are the tool names the session listed.
	Tools []string `json:"tools,omitempty"`
	// Resources are the static resource URIs the session listed.
	Resources []string `json:"resources,omitempty"`
	// ResourceTemplates are the URI templates the session listed.
	ResourceTemplates []string `json:"resource_templates,omitempty"`
	// Prompts are the prompt names the session listed.
	Prompts []string `json:"prompts,omitempty"`
	// Completions are the completion references the session offers.
	Completions []string `json:"completions,omitempty"`
	// SubscribableKinds are the resource kinds the session accepts a
	// subscription for.
	SubscribableKinds []string `json:"subscribable_kinds,omitempty"`
	// DispatchObserved is whether the harness ever saw a span from this
	// session. When it is false every call of the session is a claim about
	// what was asked for and not about what ran.
	DispatchObserved bool `json:"dispatch_observed"`
}

// Call is one MCP call a test made.
//
// It holds both halves of the question coverage asks: what the test requested,
// which the client knows, and what the server dispatched, which only the
// server's own span can say. The two differ wherever an alias is rewritten,
// and crediting the requested one would credit an action that never ran.
type Call struct {
	// Test names the top-level test the call is attributed to.
	Test string `json:"test"`
	// Purpose is one of the Purpose* constants.
	Purpose string `json:"purpose"`
	// Expectation is [ExpectationOK], [ExpectationAny] or a harness failure
	// class the test declared.
	Expectation string `json:"expectation"`
	// Session is the label of the session the call went to.
	Session string `json:"session"`
	// Surface repeats the session's surface, as Mode and Capabilities repeat
	// the rest of its shape, so one call line can be classified on its own
	// rather than only after a join back to the session that served it.
	Surface string `json:"surface"`
	// Mode is the protective mode the session runs in.
	Mode string `json:"mode"`
	// Capabilities is the resource and prompt surface of the session.
	Capabilities string `json:"capabilities"`
	// Requirement is the runtime requirement of the package that made the
	// call.
	Requirement string `json:"requirement"`
	// Method is the MCP method, so the non-tool verbs are recorded on the same
	// terms as tools/call.
	Method string `json:"method"`
	// Tool is the tool name the call named, empty for a method that names
	// none.
	Tool string `json:"tool,omitempty"`
	// Action is the canonical catalog ID the test asked for.
	Action string `json:"action,omitempty"`
	// Dispatched is the canonical ID the server reported running, joined from
	// the span on [Call.TraceID]. It is empty when no span arrived.
	Dispatched string `json:"dispatched,omitempty"`
	// Target names what a non-tool call addressed: a resource URI, a prompt or
	// a completion reference.
	Target string `json:"target,omitempty"`
	// Arguments are the argument names the call carried, sorted. The values
	// are left out: a value is a fixture, a name is part of the call.
	Arguments []string `json:"arguments,omitempty"`
	// Outcome is one of the Outcome* constants, or [RefusedOutcome] of the
	// reason the server gave.
	Outcome string `json:"outcome"`
	// DurationMS is how long the call took, in milliseconds.
	DurationMS float64 `json:"duration_ms,omitempty"`
	// TraceID is the W3C trace the harness stamped into the call, which is
	// what a [Dispatch] line joins on.
	TraceID string `json:"trace_id,omitempty"`
	// TestStatus is how the test ended: one of the Status* constants. A call
	// made by a test that failed is not coverage.
	TestStatus string `json:"test_status"`
}

// Dispatch is what the server said it ran, read from its own OTLP span.
//
// It is a line of its own rather than only a field of [Call] because the span
// arrives over the network after the answer does. A call flushed before its
// span lands carries no dispatched action, and this line is what the audit
// joins to it afterwards.
type Dispatch struct {
	// TraceID is the trace the harness stamped into the call this describes.
	TraceID string `json:"trace_id"`
	// Tool is the span's gen_ai.tool.name.
	Tool string `json:"tool,omitempty"`
	// Action is the span's gitlab_mcp.action, which names the route that ran
	// rather than the one the call asked for.
	Action string `json:"action,omitempty"`
	// Domain is the span's gitlab_mcp.domain.
	Domain string `json:"domain,omitempty"`
	// RefusalReason is the span's gitlab_mcp.refusal_reason, set when the
	// server declined to run the action.
	RefusalReason string `json:"refusal_reason,omitempty"`
	// ErrorType is the span's error.type.
	ErrorType string `json:"error_type,omitempty"`
	// Status is the span status.
	Status string `json:"status,omitempty"`
}

// Skip is a test that did not run, with the reason it gave.
//
// A skip is recorded because it is the honest third answer beside covered and
// absent: an action whose only scenario skips for want of a runner is not
// covered, and saying so is more useful than leaving the cell empty.
type Skip struct {
	// Test names the test that skipped.
	Test string `json:"test"`
	// Reason is what it said when it skipped.
	Reason string `json:"reason"`
}

// record wraps the run line in its envelope.
func (r *Run) record() Record { return Record{Schema: SchemaVersion, Type: TypeRun, Run: r} }

// record wraps the session line in its envelope.
func (s *Session) record() Record {
	return Record{Schema: SchemaVersion, Type: TypeSession, Session: s}
}

// record wraps the call line in its envelope.
func (c *Call) record() Record { return Record{Schema: SchemaVersion, Type: TypeCall, Call: c} }

// record wraps the dispatch line in its envelope.
func (d *Dispatch) record() Record {
	return Record{Schema: SchemaVersion, Type: TypeDispatch, Dispatch: d}
}

// record wraps the skip line in its envelope.
func (s *Skip) record() Record { return Record{Schema: SchemaVersion, Type: TypeSkip, Skip: s} }
