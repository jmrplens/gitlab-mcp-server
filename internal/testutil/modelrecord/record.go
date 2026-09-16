// The line shapes a shard holds, and the one envelope they are written in.
// Every field name here is the contract between the runner that writes a shard
// and cmd/gen_model_results that reads it back to score and publish.

package modelrecord

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	// DirEnv names the directory the model evaluation runner records into.
	// Recording is off unless it is set, so a run that only probes a provider
	// contract leaves nothing behind.
	//
	// The path must be absolute, for the reason its sibling
	// e2ecalls.DirEnv gives: a test binary runs with its own package
	// directory as the working directory, so a relative path would drop a
	// shard under whichever package happened to write first and the merge
	// would find none of them. A relative value is refused rather than
	// resolved, because resolving it would produce exactly that scattering
	// silently.
	//
	// It is deliberately not the same variable as the coverage record's. A
	// model run writes both, and pointing them at one directory would hand
	// cmd/audit_e2e_coverage a tree of model shards to walk; the two records
	// answer different questions and are kept apart at the directory.
	DirEnv = "GITLAB_MCP_TEST_MODELEVAL_DIR"

	// SchemaVersion is the version every line carries. A reader refuses a line
	// written under another one rather than guessing which fields it holds:
	// these shards are scored by a command in the same tree against a corpus
	// in the same tree, so a mismatch means a stale artifact and not an old
	// peer to be tolerated.
	SchemaVersion = 1

	// ShardPattern is the [os.CreateTemp] pattern a shard file is named with.
	// One process writes one shard, so package binaries running side by side
	// never write to the same file and no cross-process locking is needed.
	ShardPattern = shardPrefix + "*" + shardExt

	// shardPrefix and shardExt bracket the name of a shard. [ReadShards]
	// matches them directly instead of calling [path/filepath.Match] on
	// ShardPattern, so it has no pattern error to discard.
	shardPrefix = "modeleval-"
	shardExt    = ".jsonl"
)

// MaxResultBytes bounds the structured result a [Call] line carries.
//
// A result is kept at all because an argument of a later step may be bound to a
// field of an earlier step's answer, and a scorer that runs long after the run
// can only check that binding against what was written down. It is bounded
// because a single list call can answer with megabytes, and a record whose size
// is decided by the largest project in the fixture is a record nobody keeps.
//
// 64 KiB is well above every answer a fixture-sized project produces and well
// below [maxShardLine], which leaves room for JSON escaping to multiply the
// bytes on the way out.
const MaxResultBytes = 64 << 10

// MaxTextBytes bounds the rendered answer a [Call] line carries.
//
// [Call.Text] is what the model actually read, and on this server that is
// server-rendered Markdown: a raw file, a diff, a job trace. Those are as
// unbounded as a structured result and arrive more often, since every call
// answers with text and only some answer with content. It is kept for the same
// reason the result is, so it is bounded on the same terms and at the same
// number.
const MaxTextBytes = 64 << 10

// maxShardLine bounds one recorded line. The writer refuses to write a longer
// one and the reader's scanner refuses to read one.
//
// It is a cap of its own rather than a consequence of the field caps. Two of
// the values a line carries are bounded, [Call.Result] by [CapResult] and
// [Call.Text] by [CapText]; three are not, because [Call.Arguments],
// [Block.Arguments] and [Attempt.Stimulus] are written exactly as the model and
// the runner produced them and truncating what was sent would corrupt the one
// thing the record is for. A megabyte holds both capped values at their worst
// case, where JSON escaping turns each of their 64 KiB into six characters a
// byte, and leaves room around them.
//
// Both halves are held to it, and each for its own failure. A line silently
// dropped for being long would look exactly like a call nobody made, which is
// the one thing a record of what a model did exists to be believed about. A
// line written past what the reader takes is worse: the scanner stops at it, so
// one long answer loses the whole shard, and a shard is every attempt one
// process paid for, read back months after the money was spent.
const maxShardLine = 1 << 20

// The six line types. Every [Record] carries exactly one of them, and the
// reader refuses a record whose payload is not the one its type names.
const (
	// TypeRun names the line one test package writes once: the revision, the
	// instance, the corpus and the providers the run was configured with.
	TypeRun = "run"
	// TypeSession names the line one server configuration writes when it
	// starts, holding the shape the model was talking to.
	TypeSession = "session"
	// TypeAttempt names the line one case-and-model pairing writes: the
	// stimulus as sent, and how the conversation ended.
	TypeAttempt = "attempt"
	// TypeTurn names the line one provider request and its answer write.
	TypeTurn = "turn"
	// TypeCall names the line one tools/call the model made writes, with what
	// the server dispatched for it.
	TypeCall = "call"
	// TypeVerify names the line a recipe's post-run check against GitLab
	// writes.
	TypeVerify = "verify"
)

// How an attempt ended. The five failure endings are kept apart rather than
// folded into one, because they are charged to different parties: over_budget
// and no_tool_call are the model's, malformed is the model's output, and
// provider_error and harness_error are not the model's at all.
//
// Whether an attempt that ended [EndedNoToolCall] did the right thing is a
// scoring question and is deliberately not answered here: on a read-only
// surface a model that declines in text is correct, and on every other surface
// it is not. The record says what happened; the scorer says what it was worth.
const (
	// EndedCompleted is an attempt whose conversation reached its own end
	// inside the turn cap.
	EndedCompleted = "completed"
	// EndedOverBudget is an attempt that hit the turn cap.
	EndedOverBudget = "over_budget"
	// EndedMalformed is an attempt whose model emitted a tool call that could
	// not be parsed. Nothing is repaired and nothing is retried: a repair is a
	// hint the model did not earn, and a retry that replaces the attempt hides
	// the signal.
	EndedMalformed = "malformed"
	// EndedNoToolCall is an attempt whose model answered in text when the
	// conversation was not over.
	EndedNoToolCall = "no_tool_call"
	// EndedProviderError is an attempt the provider would not answer, after
	// the retries the runner allows.
	EndedProviderError = "provider_error"
	// EndedHarnessError is an attempt this side broke: a world that would not
	// build, a session that would not start, a call that never reached the
	// server.
	EndedHarnessError = "harness_error"
	// EndedSkipped is an attempt that never ran because the runtime did not
	// meet the case's needs, with the reason in [Attempt.Reason].
	//
	// It is an ending rather than a flag of its own so that every attempt line
	// answers "how did this end" with a value: a licensed case on a community
	// instance is absent for a reason, and a report that cannot tell that from
	// a case nobody wrote is the reason the old evaluator's coverage figure
	// meant nothing.
	EndedSkipped = "skipped"
)

// What came back from one tools/call. The vocabulary is e2ecalls' on purpose:
// the harness classifies an answer once, and two classifications of one answer
// is how the coverage record and this one would come to disagree about what the
// server did.
const (
	// OutcomeOK is a call the server answered without an error.
	OutcomeOK = "ok"
	// OutcomeToolError is a call answered with a tool error result. After a
	// correct dispatch with correct arguments this is GitLab refusing, which
	// is its own class and never a model failure.
	OutcomeToolError = "tool_error"
	// OutcomeProtocolError is a call answered with a JSON-RPC error, which is
	// what naming a tool the surface does not register looks like.
	OutcomeProtocolError = "protocol_error"
	// OutcomeTransportError is a call that never got an answer.
	OutcomeTransportError = "transport_error"
	// OutcomePreview is a safe-mode preview: the server answered the call with
	// the preview of a mutation it did not make.
	OutcomePreview = "preview"
	// OutcomeRefusedPrefix starts the outcome of a call the server refused.
	// The refusal reason follows it, and [RefusedOutcome] spells one.
	OutcomeRefusedPrefix = "refused:"
)

// How a provider request ended. A request that was answered carries
// [TurnOK] whatever the model said in it; the model's own behavior is the
// attempt's ending and not the turn's status.
const (
	// TurnOK is a request the provider answered.
	TurnOK = "ok"
	// TurnRateLimited is a request the provider refused for rate, which the
	// runner retries as another try of the same turn.
	TurnRateLimited = "rate_limited"
	// TurnServerError is a request the provider failed to serve.
	TurnServerError = "server_error"
	// TurnRequestError is a request the provider rejected as malformed, which
	// is this side's fault and is never retried.
	TurnRequestError = "request_error"
	// TurnTransportError is a request that never reached the provider.
	TurnTransportError = "transport_error"
)

// What a model emitted in one turn. A thinking block is recorded because a
// provider bills for it and a reasoning model spends most of a turn there.
const (
	// BlockText is prose the model wrote.
	BlockText = "text"
	// BlockThinking is reasoning the provider returned as its own block type.
	BlockThinking = "thinking"
	// BlockToolCall is a tool call the model made.
	BlockToolCall = "tool_call"
)

// RefusedOutcome spells the outcome of a call the server refused for the given
// reason, as [OutcomeRefusedPrefix] followed by that reason.
//
// It exists so the writer and the scorer agree on one spelling: a decline on a
// read-only surface is recognized by this prefix and the reason after it, and a
// second hand-written concatenation is how the two would come to disagree.
func RefusedOutcome(reason string) string {
	return OutcomeRefusedPrefix + reason
}

// CapResult bounds a structured result to [MaxResultBytes], reporting whether
// it had to.
//
// A result that fits is returned unchanged. One that does not is replaced by a
// JSON string holding its first [MaxResultBytes] bytes, rather than by those
// bytes on their own: cutting a JSON document in half leaves something no
// reader can parse, and a shard with one unparseable line is a shard that reads
// as no run at all. The head is kept because a person reading the trace wants
// to see what the answer looked like; the flag is what the scorer reads, and a
// binding into a truncated result fails with the truncation as its reason
// rather than silently missing.
//
// The cut may land inside a UTF-8 rune. Encoding the head as a JSON string
// replaces whatever that leaves with U+FFFD, which is the right trade for a
// value that is already documented as incomplete.
func CapResult(result json.RawMessage) (json.RawMessage, bool) {
	if len(result) <= MaxResultBytes {
		return result, false
	}
	// json.Marshal of a string cannot fail: invalid UTF-8 is replaced rather
	// than refused, which is the behavior relied on directly above. There is
	// therefore no error here to report and no branch worth carrying for one.
	head, _ := json.Marshal(string(result[:MaxResultBytes])) //nolint:errchkjson // see above
	return head, true
}

// CapText bounds a rendered answer to [MaxTextBytes], reporting whether it had
// to.
//
// It keeps the head itself rather than replacing it, which is where it differs
// from [CapResult] and why: a string cut in half is still a string, while a JSON
// document cut in half is not a document at all. The cut may land inside a UTF-8
// rune, and what that leaves is written as U+FFFD when the line is marshaled,
// which is the right trade for a value already flagged as incomplete.
func CapText(text string) (string, bool) {
	if len(text) <= MaxTextBytes {
		return text, false
	}
	return text[:MaxTextBytes], true
}

// Line is one thing a shard can hold. The set is closed on purpose: [Run],
// [Session], [Attempt], [Turn], [Call] and [Verify] are the only
// implementations, because the reader dispatches on [Record.Type] and a seventh
// shape it has never heard of would be dropped or guessed at.
type Line interface {
	// record wraps the line in the envelope it is written as.
	record() Record
}

// Record is one line of a shard: the schema it was written under, which line it
// is, and exactly one payload.
//
// It is an envelope rather than one flat struct because the six lines share
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
	// Attempt is set when Type is [TypeAttempt].
	Attempt *Attempt `json:"attempt,omitempty"`
	// Turn is set when Type is [TypeTurn].
	Turn *Turn `json:"turn,omitempty"`
	// Call is set when Type is [TypeCall].
	Call *Call `json:"call,omitempty"`
	// Verify is set when Type is [TypeVerify].
	Verify *Verify `json:"verify,omitempty"`
}

// payloadPresent answers, per line type, whether the record carries the payload
// that type names.
//
// It is a table rather than a switch so that adding a line type is one entry in
// one place: the reader asks it both questions it has, whether the type is
// known and whether its payload is there.
var payloadPresent = map[string]func(Record) bool{
	TypeRun:     func(r Record) bool { return r.Run != nil },
	TypeSession: func(r Record) bool { return r.Session != nil },
	TypeAttempt: func(r Record) bool { return r.Attempt != nil },
	TypeTurn:    func(r Record) bool { return r.Turn != nil },
	TypeCall:    func(r Record) bool { return r.Call != nil },
	TypeVerify:  func(r Record) bool { return r.Verify != nil },
}

// validate reports what is wrong with a record read back from a shard.
//
// A shard is machine-written, so every one of these means the artifact is stale
// or truncated rather than that a caller made a mistake. Reporting it is what
// keeps a published model figure from being computed over lines nobody can
// read.
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
	// Exactly one payload is the envelope's contract, and the type naming one
	// says nothing about the other five: a line carrying a turn and a call
	// together is two claims under one type, and a reader that took the named
	// half would score an attempt it had only half read. Machine-written or
	// not, that is a shard to refuse.
	carried := 0
	for _, has := range payloadPresent {
		if has(r) {
			carried++
		}
	}
	if carried != 1 {
		return fmt.Errorf("line type %q carries %d payloads, want exactly one", r.Type, carried)
	}
	// The envelope is sound; what it carries still has to mean something. See
	// validate.go: an attempt with no case, model, session or ending passes
	// every check above and describes nothing, and a scorer given it counts an
	// attempt that never happened.
	return r.validatePayload()
}

// invalidRawField names the first field of the record holding JSON that cannot
// be parsed, and is empty when every one of them can.
//
// The three places a line carries JSON this package did not build are the two
// argument fields and the call result, and all three are filled from a model's
// own output. The writer asks before it encodes, so a malformed one is reported
// as the field it is, with somewhere to put the text instead, rather than as the
// encoder's own message about a type name. A fourth such field added to the
// record above belongs here too, and
// TestRecordInvalidRawField_KnowsEveryRawFieldOfTheRecord is what says so.
func (r Record) invalidRawField() string {
	if r.Turn != nil {
		for index, block := range r.Turn.Blocks {
			if !parsableRaw(block.Arguments) {
				return fmt.Sprintf("the arguments of block %d", index+1)
			}
		}
	}
	if r.Call != nil {
		if !parsableRaw(r.Call.Arguments) {
			return "the call arguments"
		}
		if !parsableRaw(r.Call.Result) {
			return "the call result"
		}
	}
	return ""
}

// parsableRaw reports whether a raw field holds something a line can be written
// with: nothing at all, or JSON that parses.
func parsableRaw(raw json.RawMessage) bool {
	return len(raw) == 0 || json.Valid(raw)
}

// Price is what one model's tokens cost, per million, as the price table
// declared them.
//
// It travels with the run rather than being looked up when a report is
// rendered, because a price changes and a run that already happened was charged
// at the price of its own day. All four numbers are carried because a cost
// folded into one figure hides the thing worth seeing: a cache read is an order
// of magnitude cheaper than the write that created it, and a single number over
// a cached conversation reads as a model being cheap when it is being repeated.
type Price struct {
	// InputPerMillionUSD is the price of a million uncached input tokens.
	InputPerMillionUSD float64 `json:"input_per_million_usd"`
	// OutputPerMillionUSD is the price of a million output tokens.
	OutputPerMillionUSD float64 `json:"output_per_million_usd"`
	// CacheWritePerMillionUSD is the price of a million tokens written to the
	// provider's prompt cache.
	CacheWritePerMillionUSD float64 `json:"cache_write_per_million_usd"`
	// CacheReadPerMillionUSD is the price of a million tokens read from it.
	CacheReadPerMillionUSD float64 `json:"cache_read_per_million_usd"`
	// RetrievedOn is the day the price table recorded these figures, so a
	// reader can judge them stale without opening the table.
	RetrievedOn string `json:"retrieved_on"`
}

// Provider is one configured model, as the run resolved it.
//
// Options holds the request options as sent rather than as configured, which is
// the difference that matters on a row: a model that refuses a temperature had
// the field omitted, and "provider default" is a different claim from "zero".
type Provider struct {
	// Spec is the provider:model;key=value string the run was configured
	// with.
	Spec string `json:"spec"`
	// Name is the adapter the spec selected.
	Name string `json:"name"`
	// Model is the model identifier sent to that adapter.
	Model string `json:"model"`
	// Options are the request options as they went on the wire, each value
	// spelled the way the row publishes it.
	Options map[string]string `json:"options,omitempty"`
	// Price is what this model's tokens cost, absent when the run was allowed
	// to start unpriced.
	Price *Price `json:"price,omitempty"`
}

// Run is what one test package was configured with, written once per package.
//
// It is the denominator of everything else in the shard. An attempt means
// nothing without the revision that produced the server, the instance it ran
// against and the corpus its key came from: the served catalog, the pruned
// schemas and the actions that exist at all follow from the edition and the
// tier, and the key a score is computed against follows from the corpus.
type Run struct {
	// Package is the test package that wrote the line.
	Package string `json:"package"`
	// RunID is the identifier every name this run created is scoped to.
	RunID string `json:"run_id"`
	// Commit is the revision under test, from E2E_COMMIT.
	Commit string `json:"commit,omitempty"`
	// StartedAt is when the package started, in UTC.
	StartedAt time.Time `json:"started_at"`
	// Edition is what the instance reported: a community or enterprise build.
	Edition string `json:"edition"`
	// GitLabVersion is the version the instance reported.
	GitLabVersion string `json:"gitlab_version"`
	// Tier is the licensing tier the catalog was built at.
	Tier string `json:"tier"`
	// TierConfirmed is whether the tier came from the instance license rather
	// than from a setting. An unconfirmed tier means the catalog may hold
	// actions the instance will refuse, which is a different reading of a
	// failed attempt.
	TierConfirmed bool `json:"tier_confirmed"`
	// Filter is the -run expression the binary was given, empty when it ran
	// everything. A partial run is not a claim about the rest, and the
	// publisher refuses a record carrying one.
	Filter string `json:"filter,omitempty"`
	// CorpusDigest is the digest of the corpus the stimuli came from. A record
	// is re-scored from the corpus at HEAD, so a digest that no longer matches
	// means the key moved under the answer.
	CorpusDigest string `json:"corpus_digest"`
	// ContractDigest is the digest of the surface contract the system message
	// carried.
	ContractDigest string `json:"contract_digest"`
	// Providers are the models this run was configured with.
	Providers []Provider `json:"providers,omitempty"`
	// BudgetUSD is the ceiling the run stops at, zero when none was set.
	BudgetUSD float64 `json:"budget_usd,omitempty"`
	// Repeat is how many times each attempt was run.
	Repeat int `json:"repeat"`
}

// Session is one server configuration, written when that session starts.
//
// It is what the model was talking to, and every field of it changes what the
// model could have done: the surface decides how a call is shaped, the mode
// decides whether a mutation runs, the meta schema mode decides whether
// parameter names are published at all, and the token scopes decide what the
// catalog held in the first place.
type Session struct {
	// Label names the session in the attempt lines that reference it.
	Label string `json:"label"`
	// Surface is dynamic, meta or individual.
	Surface string `json:"surface"`
	// Mode is default, read-only or safe.
	Mode string `json:"mode"`
	// Capabilities is the resource and prompt surface: full or minimal.
	Capabilities string `json:"capabilities"`
	// MetaParamSchema is the meta-tool input-schema mode: opaque, compact or
	// full. On an opaque meta session the parameter names are not published,
	// so argument fidelity measures name recall from descriptions and
	// refusals, which is why every meta row carries this.
	MetaParamSchema string `json:"meta_param_schema,omitempty"`
	// TierPin is the tier forced on the server, empty when it was detected.
	TierPin string `json:"tier_pin,omitempty"`
	// SliceSize is how many individual tools the model was shown, zero when
	// the whole served set was. The individual catalog does not fit a context
	// window, so an individual row is a measurement within a slice and says
	// so.
	SliceSize int `json:"slice_size,omitempty"`
	// TokenScopes are the PAT scopes the credential carried.
	TokenScopes []string `json:"token_scopes,omitempty"`
	// ServedTools is how many tools the session listed.
	ServedTools int `json:"served_tools"`
	// ToolSchemaDigests is the digest of the tool list as each provider
	// received it, keyed by [Provider.Spec].
	//
	// It is per provider and not one digest because a provider-specific
	// rewrite of the schemas is exactly the defect this catches: two of four
	// adapters used to inject parameter names into the execute schema, so two
	// columns of a published table were measuring a different surface from the
	// other two and nothing on the row said so.
	ToolSchemaDigests map[string]string `json:"tool_schema_digests,omitempty"`
}

// Attempt is one case put to one model on one surface, written once per run of
// it.
//
// The stimulus is written down as sent rather than as templated, because what
// the model was given is the thing under test: a prompt that names its own
// answer is the defect this whole record exists to make visible, and it can
// only be read out of a run if the rendered text is there.
type Attempt struct {
	// ID is what the turn, call and verify lines of this attempt name. It is
	// the runner's own identifier and is opaque here.
	ID string `json:"id"`
	// Case is the corpus case identifier.
	Case string `json:"case"`
	// Model is the [Provider.Spec] that was asked.
	Model string `json:"model"`
	// Surface is the surface the attempt ran on.
	Surface string `json:"surface"`
	// Session is the [Session.Label] that served it.
	Session string `json:"session"`
	// Repeat is which run of this attempt it is, from 1.
	Repeat int `json:"repeat"`
	// ShownTools is how many tools this attempt was shown, Overflowed whether
	// the tools it had to be shown outnumbered the budget, and ToolDigest the
	// digest of the list as this attempt's provider received it.
	//
	// They are on the attempt and not on the session because on the individual
	// surface the list is chosen per case, and the three session fields cannot
	// be read back into it: slice_size is the budget rather than a length,
	// served_tools is what the whole session listed, and rebuilding the list
	// from them would need the served tool definitions and the domain of each,
	// which nothing here persists. Without them a row reads slice_size: 128
	// for an attempt that was shown 312 tools, and two attempts under that one
	// number are not comparable. Off the individual surface they say the same
	// thing the session does, since every attempt is shown the whole list.
	ShownTools int    `json:"shown_tools,omitempty"`
	Overflowed bool   `json:"overflowed,omitempty"`
	ToolDigest string `json:"tool_digest,omitempty"`
	// Facts are the fixture values the stimulus was rendered with, which is
	// what makes a failed attempt reproducible: a prompt naming project 42
	// says nothing without the project that was 42 that day.
	Facts map[string]string `json:"facts,omitempty"`
	// Stimulus is the rendered prompt as sent.
	Stimulus string `json:"stimulus,omitempty"`
	// EndedBy is one of the Ended* constants.
	EndedBy string `json:"ended_by"`
	// Reason says why an attempt ended the way it did: the need that was not
	// met on a skip, the provider's message on a provider error, what broke on
	// a harness error. It is empty on a completed attempt.
	Reason string `json:"reason,omitempty"`
}

// Usage is what one provider request was billed for.
//
// Four numbers, never one. A single "tokens" figure over a cached conversation
// reads as a model being frugal when it is being repeated, which is how a
// published table came to show one model consuming a fraction of another's
// budget while sending the same conversation more times.
type Usage struct {
	// Input is uncached input tokens.
	Input int `json:"input"`
	// Output is output tokens, reasoning included where the provider bills it
	// there.
	Output int `json:"output"`
	// CacheCreated is input tokens written into the provider's prompt cache.
	CacheCreated int `json:"cache_created,omitempty"`
	// CacheRead is input tokens served from it.
	CacheRead int `json:"cache_read,omitempty"`
}

// Block is one piece of what a model emitted in a turn.
type Block struct {
	// Kind is one of the Block* constants.
	Kind string `json:"kind"`
	// Text is the prose or the reasoning, empty on a tool call.
	Text string `json:"text,omitempty"`
	// Tool is the tool a [BlockToolCall] named.
	Tool string `json:"tool,omitempty"`
	// Arguments are the arguments it carried, as the provider returned them,
	// and must be valid JSON: the writer refuses a line whose raw fields are
	// not, naming the field. Raw is where the text of a call that does not
	// parse belongs.
	Arguments json.RawMessage `json:"arguments,omitempty"`
	// Raw is the argument text exactly as the provider emitted it, set when
	// that text is not JSON a reader can parse and Arguments is therefore
	// empty.
	//
	// It is where a malformed tool call lives, and it exists because Arguments
	// cannot hold one: a [encoding/json.RawMessage] carrying text that does not
	// parse is a line that does not encode. A malformed call is an ending the
	// run classifies ([EndedMalformed]) and a model is charged for, and nothing
	// can be said about one whose text the record had nowhere to keep.
	Raw string `json:"raw,omitempty"`
	// CallID is the provider's own identifier for the call, which is what a
	// tool result is fed back against on the next turn.
	CallID string `json:"call_id,omitempty"`
}

// Turn is one request to a provider and the answer it gave.
//
// Index and Try are apart because they answer different questions: Index is
// where the model is in the conversation, and Try is how many times this side
// had to ask to get that far. A rate-limited request that succeeded on the
// third try is three turn lines at one index, which is what "each recorded"
// means and what keeps a latency figure honest.
type Turn struct {
	// Attempt is the [Attempt.ID] this turn belongs to.
	Attempt string `json:"attempt"`
	// Index is the position in the conversation, from 1.
	Index int `json:"index"`
	// Try is which attempt at this request it was, from 1.
	Try int `json:"try"`
	// RequestDigest is the digest of the request body as sent, so two rows can
	// be compared without the bodies being kept.
	RequestDigest string `json:"request_digest,omitempty"`
	// Blocks are what the model emitted.
	Blocks []Block `json:"blocks,omitempty"`
	// Usage is what the request was billed for.
	Usage Usage `json:"usage"`
	// Status is one of the Turn* constants.
	Status string `json:"status"`
	// Detail carries a failed request's message, empty on a served one.
	Detail string `json:"detail,omitempty"`
	// LatencyMS is how long the provider took, in milliseconds.
	LatencyMS float64 `json:"latency_ms,omitempty"`
}

// Call is one tools/call the model made, and what the server did with it.
//
// It holds both halves of the question a score asks: what the model requested,
// which the client knows, and what the server dispatched, which only the
// server's own span can say. The two differ wherever an alias is rewritten, and
// scoring the requested one would credit an action that never ran.
//
// A call the server withheld carries no dispatched action at all, on any
// surface. The attribute the span publishes is read from the served catalog, so
// a mutating action removed by read-only mode is not in it, and both dispatchers
// refuse before the span is stamped. That is why RequestedAction is recorded
// beside it rather than derived: it is the only reading of a withheld call, and
// declining correctly is the behavior a protective mode is measured on.
type Call struct {
	// Attempt is the [Attempt.ID] this call belongs to.
	Attempt string `json:"attempt"`
	// Turn is the [Turn.Index] it was made in.
	Turn int `json:"turn"`
	// Index is the position among the calls of this attempt, from 1.
	//
	// It is what makes two identical calls two lines. A model that sends the
	// same call twice is a model paying an overhead the record has to show,
	// and without a position the second line would be dropped as a duplicate
	// of the first.
	Index int `json:"index"`
	// Tool is the tool name the model named.
	Tool string `json:"tool"`
	// Arguments are the arguments it sent, values included. This is the one
	// place this record deliberately differs from the coverage record: whether
	// the value was right is the question here.
	//
	// They are valid JSON by construction, being what a client built a
	// tools/call out of; a tool call whose text never parsed made no call and is
	// recorded as a [Block] carrying [Block.Raw] instead. The writer refuses a
	// line whose raw fields are not valid JSON either way.
	Arguments json.RawMessage `json:"arguments,omitempty"`
	// RequestedAction is the canonical catalog action the call names, read
	// from the tool name and the arguments.
	RequestedAction string `json:"requested_action,omitempty"`
	// DispatchedAction is the canonical action the server's span reported
	// running. It is empty for a discovery call, for a refused call and when
	// no span arrived.
	DispatchedAction string `json:"dispatched_action,omitempty"`
	// DispatchedTool is the tool name the span reported, which a discovery
	// call carries without an action.
	DispatchedTool string `json:"dispatched_tool,omitempty"`
	// RefusalReason is the span's refusal reason, set when the server declined
	// to run the action.
	RefusalReason string `json:"refusal_reason,omitempty"`
	// Outcome is one of the Outcome* constants, or [RefusedOutcome] of the
	// reason the server gave.
	Outcome string `json:"outcome"`
	// Result is the structured content the call answered with, bounded by
	// [CapResult].
	Result json.RawMessage `json:"result,omitempty"`
	// ResultTruncated is whether the cap bit. A binding into a truncated
	// result fails with the truncation as its reason rather than missing
	// silently.
	ResultTruncated bool `json:"result_truncated,omitempty"`
	// Text is the human-readable answer the model actually read, which is not
	// the structured content and is what a model's next move was based on,
	// bounded by [CapText].
	Text string `json:"text,omitempty"`
	// TextTruncated is whether that cap bit. It is written for the same reason
	// ResultTruncated is: a rendered answer is a raw file, a diff or a job
	// trace, so it is the longest thing a call line carries, and a reader
	// comparing what the model read with what it did next has to know it is
	// holding a head rather than the answer.
	TextTruncated bool `json:"text_truncated,omitempty"`
	// DurationMS is how long the call took, in milliseconds.
	DurationMS float64 `json:"duration_ms,omitempty"`
	// TraceID is the W3C trace the harness stamped into the call, which is
	// what the server's span was joined on.
	TraceID string `json:"trace_id,omitempty"`
	// Requests is how many GitLab requests the handler made while running
	// this call, counted from the client spans of the same trace. Read it as a
	// floor: the spans travel through a batching processor that drops silently
	// when its queue overflows.
	Requests int `json:"requests,omitempty"`
	// DispatchObserved is whether the server's span for this call arrived. It
	// is false rather than absent on purpose: every verdict derived from a
	// call whose span never came is a claim about what was asked and not about
	// what ran, and the scorer marks those unobserved rather than scoring
	// them.
	DispatchObserved bool `json:"dispatch_observed"`
}

// Verify is a recipe's own check against GitLab after an attempt finished.
//
// It is the half of the answer no tool call can give: a model that created the
// right thing through the right action may still have created it wrong, and the
// recipe that built the world is what knows how to look.
type Verify struct {
	// Attempt is the [Attempt.ID] that was checked.
	Attempt string `json:"attempt"`
	// Name is the recipe check that ran.
	Name string `json:"name"`
	// Passed is whether GitLab held what the check expected.
	Passed bool `json:"passed"`
	// Detail says what it found, which is what makes a failure triageable
	// without re-running a paid attempt.
	Detail string `json:"detail,omitempty"`
}

// record wraps the run line in its envelope.
func (r *Run) record() Record { return Record{Schema: SchemaVersion, Type: TypeRun, Run: r} }

// record wraps the session line in its envelope.
func (s *Session) record() Record {
	return Record{Schema: SchemaVersion, Type: TypeSession, Session: s}
}

// record wraps the attempt line in its envelope.
func (a *Attempt) record() Record {
	return Record{Schema: SchemaVersion, Type: TypeAttempt, Attempt: a}
}

// record wraps the turn line in its envelope.
func (t *Turn) record() Record { return Record{Schema: SchemaVersion, Type: TypeTurn, Turn: t} }

// record wraps the call line in its envelope.
func (c *Call) record() Record { return Record{Schema: SchemaVersion, Type: TypeCall, Call: c} }

// record wraps the verify line in its envelope.
func (v *Verify) record() Record {
	return Record{Schema: SchemaVersion, Type: TypeVerify, Verify: v}
}
