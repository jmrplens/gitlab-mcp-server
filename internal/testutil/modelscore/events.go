// events.go reads a shard's lines into attempts, and one attempt's calls into
// the events a step is matched against.
//
// An event is one tools/call with both readings of what it named beside each
// other: the action the model requested, resolved here against the whole
// catalog at the row's tier, and the action the server's own span said it
// dispatched. The catalog is read at the row's tier and not at the tier the
// session served, because the calls that matter most on a protective row are
// exactly the ones the served catalog no longer has: a mutating action removed
// by read-only mode is absent from the identifier the server built, so a
// session-side reading would report the one call a read-only row is measured on
// as naming nothing at all.

package modelscore

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
)

// The protective modes, spelled as [modelrecord.Session.Mode] carries them.
//
// They are constants of this package rather than of the harness because the
// harness is built behind the e2e build tag and this package is not. The
// runner writes the harness's own spelling into the record and the two are held
// together by a test there.
const (
	// ModeDefault is a server with neither protection on.
	ModeDefault = "default"
	// ModeReadOnly is a server with every mutating operation removed, where
	// what a deployment wants from a model is a text answer saying so.
	ModeReadOnly = "read-only"
	// ModeSafe is a server that answers a mutating operation with a preview of
	// the mutation it did not make.
	ModeSafe = "safe"
)

// paramsArgument is the field both dispatching surfaces carry an action's own
// arguments in, and confirmArgument is the field every surface reads an
// explicit approval of a destructive action from.
const (
	paramsArgument  = "params"
	confirmArgument = "confirm"
)

// Attempt is one attempt as a shard holds it: the run and the session it
// belongs to, its own line, and every line that names it.
//
// It is assembled by [Attempts] rather than by a caller, because the run line
// is the one join in this record that is not by name: no attempt, turn, call or
// verify line names its run, so the run a line belongs to is the run line in
// the same shard and nothing else says so.
type Attempt struct {
	// Run is the run line of the shard this attempt was written in.
	Run modelrecord.Run
	// Session is the server configuration this attempt talked to.
	Session modelrecord.Session
	// Line is the attempt's own line: the case, the model, the stimulus as
	// sent, the facts it was rendered with, and how it ended.
	Line modelrecord.Attempt
	// Turns are its provider requests, in the order they were written.
	Turns []modelrecord.Turn
	// Calls are the tools/call lines it made, in the order they were written.
	Calls []modelrecord.Call
	// Verifies are the recipe's own checks against GitLab afterwards.
	Verifies []modelrecord.Verify
}

// Attempts groups one shard's lines into the attempts they describe.
//
// One shard, not a merged directory: the run line is per shard, and a merged
// read has lost which run each attempt belongs to along with the commit, the
// instance and the tier a published row has to carry. A caller holding several
// shards calls this once per shard, which is the shape [modelrecord.ReadShards]
// hands back.
//
// The pass is two passes on purpose. A run writes its attempt lines as each
// subtest ends and its run and session lines from an exit hook, so a session
// line arrives after every attempt that named it and a one-pass join would find
// none of them.
func Attempts(records []modelrecord.Record) ([]Attempt, error) {
	run, err := runLine(records)
	if err != nil {
		return nil, err
	}
	sessions := map[string]modelrecord.Session{}
	for _, record := range records {
		if record.Session != nil {
			sessions[record.Session.Label] = *record.Session
		}
	}

	var order []string
	byID := map[string]*Attempt{}
	for _, record := range records {
		if record.Attempt == nil {
			continue
		}
		if _, seen := byID[record.Attempt.ID]; seen {
			return nil, fmt.Errorf("two attempt lines carry the id %q", record.Attempt.ID)
		}
		order = append(order, record.Attempt.ID)
		byID[record.Attempt.ID] = &Attempt{Run: run, Line: *record.Attempt}
	}

	if attachErr := attachLines(records, byID); attachErr != nil {
		return nil, attachErr
	}

	attempts := make([]Attempt, 0, len(order))
	for _, id := range order {
		one := byID[id]
		if one.Line.Session != "" {
			session, known := sessions[one.Line.Session]
			if !known {
				return nil, fmt.Errorf("attempt %q names session %q, which the shard has no line for",
					id, one.Line.Session)
			}
			one.Session = session
		}
		attempts = append(attempts, *one)
	}
	return attempts, nil
}

// runLine returns the shard's run line.
//
// A shard with none is an error rather than a zero run, because the tier is
// read from it and scoring at the wrong tier silently changes which actions
// exist. A second run line naming another run is an error for the same reason:
// the join would have to pick one, and picking is what a reader would never see.
func runLine(records []modelrecord.Record) (modelrecord.Run, error) {
	var found *modelrecord.Run
	for _, record := range records {
		if record.Run == nil {
			continue
		}
		if found != nil && found.RunID != record.Run.RunID {
			return modelrecord.Run{}, fmt.Errorf("the shard carries two run lines, %q and %q",
				found.RunID, record.Run.RunID)
		}
		found = record.Run
	}
	if found == nil {
		return modelrecord.Run{}, fmt.Errorf("the shard carries no %s line, so the tier its attempts "+
			"ran at is unknown", modelrecord.TypeRun)
	}
	return *found, nil
}

// attachLines files every turn, call and verify line under the attempt it
// names. A line naming an attempt the shard has no line for is an error: it is
// evidence of a truncated shard, and dropping it would score a conversation
// with a hole in it as a shorter conversation.
func attachLines(records []modelrecord.Record, byID map[string]*Attempt) error {
	for _, record := range records {
		var owner string
		switch {
		case record.Turn != nil:
			owner = record.Turn.Attempt
		case record.Call != nil:
			owner = record.Call.Attempt
		case record.Verify != nil:
			owner = record.Verify.Attempt
		default:
			continue
		}
		attempt, known := byID[owner]
		if !known {
			return fmt.Errorf("a %s line names attempt %q, which the shard has no line for",
				record.Type, owner)
		}
		switch {
		case record.Turn != nil:
			attempt.Turns = append(attempt.Turns, *record.Turn)
		case record.Call != nil:
			attempt.Calls = append(attempt.Calls, *record.Call)
		default:
			attempt.Verifies = append(attempt.Verifies, *record.Verify)
		}
	}
	return nil
}

// surface returns the tool surface this attempt ran on. The attempt's own
// field is the one that names it; the session's is consulted only when the
// attempt line left it out, since one session serves one surface.
func (a Attempt) surface() string {
	if a.Line.Surface != "" {
		return a.Line.Surface
	}
	return a.Session.Surface
}

// mode returns the protective mode this attempt ran under. An empty mode reads
// as [ModeDefault], which is what a server with nothing set runs in.
func (a Attempt) mode() string {
	if a.Session.Mode == "" {
		return ModeDefault
	}
	return a.Session.Mode
}

// tier returns the licensing tier the catalog is read at.
//
// A pinned tier wins over the detected one because it is what the session
// served: a row pinned to Free is a row about the Free catalog whatever license
// the instance holds.
func (a Attempt) tier() string {
	if a.Session.TierPin != "" {
		return a.Session.TierPin
	}
	return a.Run.Tier
}

// Event is one tools/call the model made, read into what a step is matched
// against.
type Event struct {
	// Index is the call's position among the calls of the attempt, from 1.
	Index int
	// Turn is the provider turn it was made in.
	Turn int
	// Tool is the tool name the model named.
	Tool string
	// Arguments are the arguments it sent, as the model sent them.
	Arguments json.RawMessage
	// Requested is the canonical action this call names, read here against the
	// whole catalog at the row's tier. It is empty when the call names no
	// catalog action, which is what a discovery call, a standalone tool and an
	// invented tool name all look like.
	Requested string
	// Dispatched is the canonical action the server's span said it ran, after
	// every alias rewrite. It is empty for a discovery call, for a call the
	// server refused before dispatching, and on a surface where nothing
	// records a dispatch.
	Dispatched string
	// DispatchedTool is the tool the span reported, which a discovery call
	// carries without an action.
	DispatchedTool string
	// RefusalReason is the server's own reason for declining, in the server's
	// vocabulary: unknown_action, invalid_params, needs_confirmation,
	// safe_mode.
	RefusalReason string
	// Outcome is the class the answer fell into, in the record's vocabulary.
	Outcome string
	// Result is the structured content the call answered with, which is what a
	// later step's Produced argument is bound against.
	Result json.RawMessage
	// ResultTruncated is whether the record's cap bit on that result. A
	// binding into a truncated result fails naming the truncation rather than
	// missing silently.
	ResultTruncated bool
	// Text is the rendered answer the model read, which is where every refusal
	// this server makes puts its reason.
	Text string
	// Discovery is whether this call was a catalog search rather than an
	// action. It is counted as overhead and never matched against a step.
	Discovery bool
	// Observed is whether the server's span for this call arrived at all.
	Observed bool
}

// events reads one attempt's calls into events, in call order.
//
// The order is the call index rather than the order the lines happen to sit in
// the shard, because step matching is in order and a writer that buffered one
// line would otherwise change which step a call reached.
func (a Attempt) events(facts catalogFacts) []Event {
	calls := slices.Clone(a.Calls)
	slices.SortStableFunc(calls, func(left, right modelrecord.Call) int { return left.Index - right.Index })

	events := make([]Event, 0, len(calls))
	for _, call := range calls {
		event := Event{
			Index:           call.Index,
			Turn:            call.Turn,
			Tool:            call.Tool,
			Arguments:       call.Arguments,
			Requested:       facts.requested(call),
			Dispatched:      call.DispatchedAction,
			DispatchedTool:  call.DispatchedTool,
			RefusalReason:   call.RefusalReason,
			Outcome:         call.Outcome,
			Result:          call.Result,
			ResultTruncated: call.ResultTruncated,
			Text:            call.Text,
			Observed:        call.DispatchObserved,
		}
		event.Discovery = isDiscovery(event)
		events = append(events, event)
	}
	return events
}

// isDiscovery reports whether an event was a catalog search.
//
// The server's own word decides it: a span naming the find tool and no action
// is a discovery call whatever the model typed. The tool the model named is
// consulted only when no span arrived, where the alternative is to count a
// find call as an attempt at the step it was searching for.
func isDiscovery(event Event) bool {
	if event.Dispatched != "" {
		return false
	}
	if event.DispatchedTool != "" {
		return event.DispatchedTool == dynamictools.FindActionToolName
	}
	return event.Tool == dynamictools.FindActionToolName
}

// actionArguments returns the object holding the action's own arguments, which
// is not the argument object on two of the three surfaces.
//
// The individual surface registers one tool per action, so its arguments are
// the call's own. Both dispatching surfaces carry the operation in an action
// field and the operation's arguments in params, so what a step's arguments
// are compared against is that sub-object. A tool registered outside the
// catalog is named directly on every surface and takes its arguments at the top
// level, which is why the dispatcher is recognized rather than assumed from the
// surface alone.
func (e Event) actionArguments(facts catalogFacts) map[string]json.RawMessage {
	top := decodeObject(e.Arguments)
	if !facts.dispatcher(e.Tool) {
		return top
	}
	return decodeObject(top[paramsArgument])
}

// confirmed reports whether this call carried an explicit approval of a
// destructive action, at the position the surface it ran on reads one.
//
// Dynamic reads it at the top level beside the action and its params, because
// the execute tool's own schema says so and a confirm inside params is read as
// a parameter of the action rather than as an approval. The other two read it
// with the action's own arguments.
func (e Event) confirmed(facts catalogFacts) bool {
	if facts.surface == config.ToolSurfaceDynamic {
		return isTrue(decodeObject(e.Arguments)[confirmArgument])
	}
	return isTrue(e.actionArguments(facts)[confirmArgument])
}

// isTrue reads a confirmation flag.
//
// The string spelling is accepted beside the boolean one because what is being
// measured is whether the model approved the action, and a model that wrote
// "true" approved it. Whether the server then coerces that spelling is the
// server's business and is visible in the outcome of the same call.
func isTrue(raw json.RawMessage) bool {
	var flag bool
	if json.Unmarshal(raw, &flag) == nil {
		return flag
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.EqualFold(strings.TrimSpace(text), "true")
	}
	return false
}

// decodeObject reads a JSON object into its fields, or into nothing when the
// value is absent or is not an object. A model that sent a string where an
// object belongs sent no arguments at that position, which is what a missing
// required argument means and is reported as one.
func decodeObject(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	return fields
}

// actionFacts is what the catalog says about one action that a verdict depends
// on. Both flags are read from the catalog rather than declared on a case, so a
// step that becomes destructive upstream is scored as destructive here without
// the corpus being touched.
type actionFacts struct {
	destructive bool
	readOnly    bool
}

// catalogFacts is the catalog at one tier, read the way one surface reads it,
// with the tools registered outside it beside it.
//
// The standalone tools are here because a step may name one: project discovery
// and the interactive creation flows belong to no catalog action, and whether
// one of them mutates decides what a read-only row expects of it. Their own
// specs carry the two flags, so nothing here has to guess from a name.
type catalogFacts struct {
	surface    string
	identify   mcpotel.CallIdentifier
	actions    map[string]actionFacts
	standalone map[string]actionFacts
	metaTools  map[string]bool
}

// stepFacts returns what the catalog says about one step of a key, and whether
// it names anything the server registers at all.
func (f catalogFacts) stepFacts(step modelcorpus.Step) (actionFacts, bool) {
	if step.Standalone != "" {
		facts, known := f.standalone[step.Standalone]
		return facts, known
	}
	facts, known := f.actions[string(step.Action)]
	return facts, known
}

// mutating reports whether an action changes anything, which is what a
// protective mode withholds and what a decline is about. An action the catalog
// does not have is not mutating: nothing can be said about a name no catalog
// carries, and reading it as a mutation would fail an attempt for a call the
// server answered with "no such action".
func (f catalogFacts) mutating(action string) bool {
	facts, found := f.actions[action]
	return found && !facts.readOnly
}

// dispatcher reports whether a tool name is one that carries an operation in an
// action field: the dynamic execute tool, or one of the meta group tools.
func (f catalogFacts) dispatcher(tool string) bool {
	switch f.surface {
	case config.ToolSurfaceDynamic:
		return tool == dynamictools.ExecuteActionToolName
	case config.ToolSurfaceMeta:
		return f.metaTools[tool]
	default:
		return false
	}
}

// requested resolves the canonical action one call names.
//
// The reading is this package's own, over the whole catalog, and the action the
// runner wrote down is the fallback rather than the answer: the runner resolved
// it against whatever it could reach at the time, and a call withheld by a
// protective mode is exactly the one a served catalog cannot name. The fallback
// is kept because a record written against a catalog this tree no longer builds
// is still a record, and reporting the call as naming nothing would lose the
// one reading a withheld call has.
func (f catalogFacts) requested(call modelrecord.Call) string {
	if identity, named := f.identify.Identify(call.Tool, call.Arguments); named && identity.ActionID != "" {
		return identity.ActionID
	}
	return call.RequestedAction
}

// catalogKey names one reading of the catalog: the tier it is built at and the
// surface whose spelling of a call it resolves.
type catalogKey struct {
	tier    edition.Tier
	surface string
}

var (
	catalogsMu sync.Mutex
	catalogs   = map[catalogKey]catalogFacts{}
)

// catalogFor reads the catalog at one tier the way one surface reads it.
//
// An unrecognized tier is an error rather than a fallback to Free. Scoring at
// the wrong tier silently changes which actions exist, so a licensed case
// scored at Free would report every step of it as naming nothing the server
// registers, which reads as a model failure and is a record defect.
//
// The instance class is self-managed, which is the class the corpus gate builds
// at too: a GitLab.com-only action is absent on a self-managed instance at
// every tier, so a corpus that passes its own gate resolves here.
func catalogFor(surface, tier string) (catalogFacts, error) {
	if !slices.Contains(surfaces(), surface) {
		return catalogFacts{}, fmt.Errorf("the record names surface %q, which is not one of %s",
			surface, strings.Join(surfaces(), ", "))
	}
	parsed, known := edition.ParseTier(tier)
	if !known {
		return catalogFacts{}, fmt.Errorf("the record names tier %q, which is not one of free, ce, "+
			"premium or ultimate, so the catalog it ran against cannot be read", tier)
	}

	key := catalogKey{tier: parsed, surface: surface}
	catalogsMu.Lock()
	defer catalogsMu.Unlock()
	if cached, found := catalogs[key]; found {
		return cached, nil
	}
	facts, err := readCatalog(key)
	if err != nil {
		return catalogFacts{}, err
	}
	catalogs[key] = facts
	return facts, nil
}

// buildCatalog is how the catalog is obtained, as a variable so that a test can
// see what a scorer does when it cannot be built.
//
// That is not a hypothetical branch to have covered for its own sake: this is
// the one thing in the package that can fail for a reason outside the record,
// and if it failed silently every action would read as one the catalog does not
// have, which this package reports as a corpus defect. The seam is what lets
// the real message be asserted rather than assumed.
var buildCatalog = gitlabtools.SharedBaseCatalog

// readCatalog builds one reading of the catalog.
//
// IncludeMCP is set because the binary sets it, so an attempt naming
// server.status is judged against a catalog that has it.
func readCatalog(key catalogKey) (catalogFacts, error) {
	built, err := buildCatalog(false, gitlabtools.ActionCatalogOptions{
		Tier:       key.tier,
		IncludeMCP: true,
	})
	if err != nil {
		return catalogFacts{}, fmt.Errorf("build the action catalog at tier %s: %w", key.tier, err)
	}
	facts := catalogFacts{
		surface:    key.surface,
		identify:   gitlabtools.NewCallIdentifier(built, key.surface),
		actions:    make(map[string]actionFacts, built.CountActions()),
		standalone: map[string]actionFacts{},
		metaTools:  map[string]bool{},
	}
	// Every catalog action carries the meta tool it belongs to, which the
	// catalog's own validation requires, so the name is taken as it comes. A
	// guard against an empty one would be a branch nothing in this repository
	// can reach, and what it would buy is that a call with no tool name at all
	// reads its arguments at the top level rather than in params, which is a
	// record nobody can score either way.
	for _, action := range built.Actions() {
		facts.actions[string(action.ID)] = actionFacts{
			destructive: action.Destructive,
			readOnly:    action.ReadOnly,
		}
		facts.metaTools[action.ToolName] = true
	}
	for _, spec := range gitlabtools.StandaloneSurfaceToolSpecs(nil) {
		facts.standalone[spec.Name] = actionFacts{destructive: spec.Destructive, readOnly: spec.ReadOnly}
	}
	return facts, nil
}

// surfaces returns the three surfaces a record may name, spelled as the
// server's own configuration spells them.
func surfaces() []string {
	return []string{config.ToolSurfaceDynamic, config.ToolSurfaceMeta, config.ToolSurfaceIndividual}
}
