package main

import (
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/subscriptions"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// The protective modes, spelled as the harness records them on every session
// and call line.
const (
	modeDefault  = "default"
	modeReadOnly = "read-only"
	modeSafe     = "safe"
)

// The MCP methods the classification reads. They are spelled here because
// the SDK keeps its own constants unexported and the harness spells the same
// strings on its side of the record.
const (
	methodCallTool        = "tools/call"
	methodReadResource    = "resources/read"
	methodGetPrompt       = "prompts/get"
	methodComplete        = "completion/complete"
	methodSubscribe       = "resources/subscribe"
	methodListen          = "subscriptions/listen"
	methodElicit          = "elicitation/create"
	methodResourceUpdated = "notifications/resources/updated"
)

// interactiveDomain is the catalog domain of the elicitation flows.
const interactiveDomain = "interactive"

// state is what one runtime x surface x mode x action cell is.
type state string

// The states, from the credit a passing test earned down to the reasons there
// is none. Each cell gets exactly one.
const (
	// stateAsserted is a test-purpose call that expected success, got it, and
	// whose dispatched action is the one it named, in a test that passed.
	stateAsserted state = "asserted"
	// stateUnobserved is an asserted call whose dispatch no span confirmed:
	// the session ran with dispatch unobserved, or the span never arrived.
	// It is a claim about what was asked for, not about what ran.
	stateUnobserved state = "unobserved"
	// stateSweepOnly is credit from a sweep over what the session serves.
	stateSweepOnly state = "sweep-only"
	// stateErrorPathOnly is credit from a call that expected, and got, an
	// error answer.
	stateErrorPathOnly state = "error-path-only"
	// stateRefusedOnly is credit from a call the server refused before any
	// handler ran.
	stateRefusedOnly state = "refused-only"
	// statePreviewOnly is credit from a safe-mode preview.
	statePreviewOnly state = "preview-only"
	// stateCleanupOnly is credit from a cleanup, after the test body.
	stateCleanupOnly state = "cleanup-only"
	// stateUnasserted is an asserted cell whose only call sites discard the
	// result, which the static scan finds and the record cannot.
	stateUnasserted state = "unasserted"
	// stateUnservable is a cell the surface or mode cannot serve at all.
	stateUnservable state = "unservable"
	// stateSkipped is a cell whose only scenario skipped, with its reason.
	stateSkipped state = "skipped"
	// stateFailed is a cell whose only calls came from tests that failed.
	stateFailed state = "failed"
	// stateAbsent is a servable cell nothing called.
	stateAbsent state = "absent"
)

// credit is what one call contributes to a cell, ordered so that the higher
// value is the better evidence and a cell's state follows from its best.
type credit int

// The credits, lowest first.
const (
	creditNone credit = iota
	creditSkipped
	creditFailed
	creditCleanup
	creditPreview
	creditRefused
	creditErrorPath
	creditSweep
	creditUnobserved
	creditAsserted
)

// creditStates maps each credit to the state a cell with that best credit is.
var creditStates = map[credit]state{
	creditSkipped:    stateSkipped,
	creditFailed:     stateFailed,
	creditCleanup:    stateCleanupOnly,
	creditPreview:    statePreviewOnly,
	creditRefused:    stateRefusedOnly,
	creditErrorPath:  stateErrorPathOnly,
	creditSweep:      stateSweepOnly,
	creditUnobserved: stateUnobserved,
	creditAsserted:   stateAsserted,
}

// String names the credit as the baseline comparison records it.
func (c credit) String() string {
	if s, known := creditStates[c]; known {
		return string(s)
	}
	return "none"
}

// creditOf says what one call contributes, and to which action.
//
// The action credited is the one the server said it ran when a span said so,
// and the one the test named otherwise: a call rewritten to another route
// exercised that route and not the one it asked for. Whether the two differ
// is reported separately as a mismatch.
//
// Two purposes earn nothing at all. A raw call is about the envelope, so what
// the server made of it is somebody else's evidence; a model call was chosen
// by a provider at run time, so crediting it would make this suite's coverage
// a function of what a language model felt like trying.
func creditOf(call *e2ecalls.Call) (earned credit, target string) {
	target = call.Dispatched
	if target == "" {
		target = call.Action
	}
	if target == "" || call.Purpose == e2ecalls.PurposeRaw || call.Purpose == e2ecalls.PurposeModel {
		return creditNone, ""
	}
	return creditOfOutcome(call), target
}

// creditOfOutcome is [creditOf]'s answer once the target is settled, shared
// with the non-tool verbs, whose target is what they addressed.
func creditOfOutcome(call *e2ecalls.Call) credit {
	if call.TestStatus == e2ecalls.StatusSkipped {
		return creditSkipped
	}
	if call.TestStatus != e2ecalls.StatusPassed {
		// Failed, or no status at all. A call whose test's verdict is unknown
		// is not proven coverage, and is counted with the failures rather
		// than credited on trust.
		return creditFailed
	}
	if call.Purpose == e2ecalls.PurposeCleanup {
		if call.Outcome == e2ecalls.OutcomeOK {
			return creditCleanup
		}
		return creditNone
	}
	return creditOfAnswer(call)
}

// creditOfAnswer credits a call from a passing test by what came back.
func creditOfAnswer(call *e2ecalls.Call) credit {
	switch {
	case call.Outcome == e2ecalls.OutcomePreview:
		return creditPreview
	case strings.HasPrefix(call.Outcome, e2ecalls.OutcomeRefusedPrefix):
		return creditRefused
	case call.Outcome == e2ecalls.OutcomeToolError, call.Outcome == e2ecalls.OutcomeProtocolError:
		// The test passed, so the error was the answer it wanted.
		return creditErrorPath
	case call.Outcome != e2ecalls.OutcomeOK:
		// A transport error says nothing about the action.
		return creditNone
	case call.Purpose == e2ecalls.PurposeSweep:
		return creditSweep
	case call.Method == methodCallTool && call.Dispatched == "":
		return creditUnobserved
	default:
		return creditAsserted
	}
}

// shapeKey names one surface in one mode.
type shapeKey struct {
	surface string
	mode    string
}

// sessionShape is what the sessions of one surface and mode served, folded
// over every session line with that shape, whatever their capability surface.
// It is what the session rows publish and what the action cells are judged
// against; the capability cells are judged against [capabilityShape].
type sessionShape struct {
	key shapeKey
	// tools, resources, templates and prompts are the union of what those
	// sessions listed.
	tools     map[string]bool
	resources map[string]bool
	templates []string
	prompts   map[string]bool
	// observed is whether every session of the shape that made a traced call
	// had the server's span of at least one of them arrive, of whatever
	// method. An idle session, which made no traced call, does not hold it
	// false: it asked nothing a span could answer. It is true of a shape whose
	// sessions were all idle, for want of anything to hold it false, which is
	// why the row publishes [sessionShape.dispatchObserved] rather than this.
	// It decides no credit; the credit is judged per call, and a tool call's on
	// the action its own span named.
	observed bool
	// sessions counts the session lines folded in.
	sessions int
	// idle counts the ones among them that made no traced call.
	idle int
}

// dispatchObserved is what the shape's row publishes as dispatch_observed:
// at least one session of the shape made a traced call, and every one that
// did had the server's span of one of them arrive.
//
// The first half is what keeps the flag a positive claim. A shape whose
// sessions were all idle asked nothing a span could answer, so nothing about
// its telemetry was seen either way, and reading it as observed would put it
// beside a shape whose spans did arrive with nothing to tell the two apart. It
// reads false instead, and the row's idle count, equal to its session count,
// says that this false is for want of a question rather than for want of
// telemetry.
func (s *sessionShape) dispatchObserved() bool {
	return s.observed && s.idle < s.sessions
}

// capabilityShape is what the sessions of one capability surface served,
// folded over every session line with it, whatever their tool surface and
// mode.
//
// It is the denominator of every capability kind counted at the capability
// grain, which is why it exists apart from [sessionShape]: the server
// registers its resources, prompts, completions and subscribable kinds from
// the capability surface and the operator's exclusions alone, so the sessions
// that differ only in tool surface or mode served the same set, and one cell
// per item per capability surface is all there is to fill.
type capabilityShape struct {
	// key is the effective capability surface, full or minimal.
	key string
	// resources, templates, prompts, completions and kinds are the union of
	// what those sessions listed. The resources and templates keep the
	// tool-manifest pair, because a read of it resolves against them like any
	// other; the capability cells leave the pair out and count it per shape
	// and capability surface.
	resources   map[string]bool
	templates   []string
	prompts     map[string]bool
	completions map[string]bool
	kinds       map[string]bool
	// shapes is every surface x mode a session of this capability surface ran
	// as, each with the tool-manifest items it listed: the denominator of the
	// tool_manifest cells, whose content varies along all three coordinates.
	shapes map[shapeKey]map[string]bool
	// sessions counts the session lines folded in.
	sessions int
}

// cellKey names one cell: a surface x mode x action for the action cells, and
// for a capability cell the coordinates its kind varies along, the others left
// empty (see [capabilityKey]).
type cellKey struct {
	shape        shapeKey
	capabilities string
	action       string
}

// cell is one action or capability item at its key's grain (see cellKey),
// with everything the calls said.
type cell struct {
	key cellKey
	// best is the highest credit any call earned.
	best credit
	// tests names the tests behind each credit.
	tests map[credit]map[string]bool
	// counts is how many calls earned each credit.
	counts map[credit]int
	// reason explains an unservable or skipped cell.
	reason string
	// state is settled once every call and every structural rule is in.
	state state
	// unlisted marks a capability cell only a call created, for a target no
	// session of its capability surface listed. It stays in the report as
	// evidence of what was called and out of the histogram, which is counted
	// against what the sessions served.
	unlisted bool
}

// newCell returns an empty cell.
func newCell(key cellKey) *cell {
	return &cell{key: key, tests: map[credit]map[string]bool{}, counts: map[credit]int{}}
}

// add records one call's credit.
func (c *cell) add(earned credit, test string) {
	if earned == creditNone {
		return
	}
	c.counts[earned]++
	if c.tests[earned] == nil {
		c.tests[earned] = map[string]bool{}
	}
	c.tests[earned][test] = true
	c.best = max(c.best, earned)
}

// copyFrom makes this cell say what another does, with maps of its own.
//
// The maps are copied rather than shared because a cell derived from another
// is read after the source may have moved on: applyStatic adds a skipped
// credit to an action cell after the flow cell was derived from it, and a
// flow cell sharing that map would carry a credit its own state never
// accounted for.
func (c *cell) copyFrom(source *cell) {
	c.best, c.state, c.reason = source.best, source.state, source.reason
	c.tests = make(map[credit]map[string]bool, len(source.tests))
	for earned, tests := range source.tests {
		c.tests[earned] = maps.Clone(tests)
	}
	c.counts = maps.Clone(source.counts)
}

// bestTests names the tests behind the cell's best credit, sorted.
func (c *cell) bestTests() []string {
	names := make([]string, 0, len(c.tests[c.best]))
	for name := range c.tests[c.best] {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// mismatch is a call whose dispatched action is not the one it named.
type mismatch struct {
	Test       string `json:"test"`
	Surface    string `json:"surface"`
	Mode       string `json:"mode"`
	Requested  string `json:"requested"`
	Dispatched string `json:"dispatched"`
	Purpose    string `json:"purpose"`
	TestStatus string `json:"test_status"`
}

// unresolvedTool is a tool a call named that the session never served.
type unresolvedTool struct {
	Test    string `json:"test"`
	Surface string `json:"surface"`
	Mode    string `json:"mode"`
	Tool    string `json:"tool"`
	Outcome string `json:"outcome"`
}

// diagnostics counts what the fold saw that is about the record itself
// rather than about coverage.
type diagnostics struct {
	// Calls is every call line.
	Calls int `json:"calls"`
	// ToolCalls is the tools/call lines among them.
	ToolCalls int `json:"tool_calls"`
	// DispatchLines is how many dispatch lines the shards carried.
	DispatchLines int `json:"dispatch_lines"`
	// LateJoins is how many calls took their dispatched action from a
	// separate dispatch line.
	LateJoins int `json:"late_joins"`
	// WithoutDispatch is how many test-purpose tool calls that succeeded
	// carry no dispatched action even after the join.
	WithoutDispatch int `json:"without_dispatch"`
	// WithoutStatus is how many calls carry no test status, which the
	// results join is for.
	WithoutStatus int `json:"without_status"`
	// TransportErrors is how many calls never got an answer.
	TransportErrors int `json:"transport_errors"`
	// RawCalls is how many calls named a tool directly and credit nothing.
	RawCalls int `json:"raw_calls"`
	// UnknownActions names the actions calls named that the runtime's
	// catalog does not hold, sorted.
	UnknownActions []string `json:"unknown_actions,omitempty"`
	// UnobservedSessions names the sessions that made a traced call and never
	// had the server's span of one arrive, which says their telemetry did not
	// reach the harness. Each holds its shape's dispatch_observed false.
	UnobservedSessions []string `json:"unobserved_sessions,omitempty"`
	// IdleSessions names the sessions that issued no trace: they started,
	// listed what they serve, and made no call that carried one (a subscribe
	// on protocol 2026-07-28 carries none), which is what the tier-pin
	// sessions do. Their spans cannot have arrived, and that is no finding
	// about their telemetry, so they are named here and hold no shape false on
	// their own. A shape of idle sessions alone reads false all the same, with
	// its idle count equal to its session count ([sessionShape.dispatchObserved]).
	IdleSessions []string `json:"idle_sessions,omitempty"`
}

// modeEvidence is what the calls on one protective shape showed of the mode.
type modeEvidence struct {
	// reads is a passing test reading through the mode.
	reads map[string]bool
	// withheld is a passing test seeing a mutating action withheld.
	withheld map[string]bool
	// previews is a passing test seeing a mutating action previewed.
	previews map[string]bool
	// failed is a test that failed on the shape.
	failed map[string]bool
}

// classification is the whole of what one runtime's records say, judged
// against the catalog it served.
type classification struct {
	rt      *runtimeRecords
	catalog *servedCatalog
	// shapes is every surface x mode a session line named.
	shapes map[shapeKey]*sessionShape
	// capabilitySurfaces is every effective capability surface a session line
	// named.
	capabilitySurfaces map[string]*capabilityShape
	// cells is every surface x mode x action, filled for every catalog
	// action on every shape once the calls are in.
	cells map[cellKey]*cell
	// capability cells, keyed by kind, then by the kind's grain and target.
	capabilities map[string]map[cellKey]*cell
	// delivered marks the subscription cells a resource-updated notification
	// reached in a passing test, keyed as those cells are.
	delivered map[cellKey]bool
	// elicited marks the tests that saw an elicitation request.
	elicited map[string]bool
	// modes is the evidence per protective shape.
	modes map[shapeKey]*modeEvidence
	// called is the tools every call named, per shape.
	called map[shapeKey]map[string]bool
	// mismatches, unresolved and diagnostics are the lists the report
	// publishes beside the cells.
	mismatches  []mismatch
	unresolved  []unresolvedTool
	diagnostics diagnostics
	// skipReasons is the reason each skipped test gave, by test name.
	skipReasons map[string]string
}

// The capability kinds, which are the keys of [classification.capabilities].
const (
	capabilityResources     = "resources"
	capabilityPrompts       = "prompts"
	capabilityCompletions   = "completions"
	capabilitySubscriptions = "subscriptions"
	capabilityToolManifest  = "tool_manifest"
	capabilityElicitation   = "elicitation"
	capabilityModes         = "modes"
)

// cellGrain is what one capability cell is per: the coordinates of a session
// the kind's content varies along, which are the ones its key carries.
type cellGrain int

// The grains, from the coarsest a kind can be counted at.
const (
	// grainCapabilitySurface is one cell per item per capability surface.
	grainCapabilitySurface cellGrain = iota
	// grainShape is one cell per item per surface x mode.
	grainShape
	// grainShapeAndCapabilitySurface is one cell per item per surface x mode
	// x capability surface.
	grainShapeAndCapabilitySurface
)

// String spells a grain the way the committed page states it.
func (g cellGrain) String() string {
	switch g {
	case grainShape:
		return "surface x mode"
	case grainShapeAndCapabilitySurface:
		return "surface x mode x capability surface"
	default:
		return "capability surface"
	}
}

// capabilityGrains is the grain each capability kind is counted at.
//
// A cell per coordinate a kind does not vary along is a cell nothing can fill
// differently from its twin, so the key carries only the coordinates that
// change what the server serves:
//
//   - Resources, prompts, completions and subscriptions are registered from
//     the capability surface and the operator's exclusions alone. Neither the
//     tool surface nor the protective mode reaches them, and the minimal
//     capability surface serves a different set (no prompt, no subscription,
//     one resource and one template), so it is the one coordinate kept.
//   - The tool manifest, gitlab://tools and gitlab://tools/{id}, lists what the
//     session's tool surface registered after the read-only and safe passes,
//     and is served on both capability surfaces with a subscriptions section
//     only on full, so it varies along all three.
//   - The elicitation flows and the protective modes are reached through the
//     actions a surface serves in a mode, and are counted where those are.
//
// It is the one statement of the model, read by [capabilityKey], which keys
// every capability cell, and by the committed page, which prints it, so the
// page cannot describe a grain the fold does not use.
var capabilityGrains = map[string]cellGrain{
	capabilityResources:     grainCapabilitySurface,
	capabilityPrompts:       grainCapabilitySurface,
	capabilityCompletions:   grainCapabilitySurface,
	capabilitySubscriptions: grainCapabilitySurface,
	capabilityToolManifest:  grainShapeAndCapabilitySurface,
	capabilityElicitation:   grainShape,
	capabilityModes:         grainShape,
}

// capabilityKey is the cell one item of a capability kind is counted in, at
// the kind's grain: the coordinates the kind does not vary along are left
// empty, so every session that differs only in them lands on the same cell.
func capabilityKey(kind string, shape shapeKey, capabilities, target string) cellKey {
	switch capabilityGrains[kind] {
	case grainShape:
		return cellKey{shape: shape, action: target}
	case grainShapeAndCapabilitySurface:
		return cellKey{shape: shape, capabilities: capabilities, action: target}
	default:
		return cellKey{capabilities: capabilities, action: target}
	}
}

// isToolManifest reports whether a resource URI or template is one of the pair
// the active tool surface decides, which the resources package names.
func isToolManifest(uri string) bool {
	return slices.Contains(resources.ToolSurfaceResourceURIs(), uri)
}

// classify judges one runtime's records against its catalog.
func classify(rt *runtimeRecords, catalog *servedCatalog) *classification {
	c := &classification{
		rt:                 rt,
		catalog:            catalog,
		shapes:             map[shapeKey]*sessionShape{},
		capabilitySurfaces: map[string]*capabilityShape{},
		cells:              map[cellKey]*cell{},
		capabilities:       map[string]map[cellKey]*cell{},
		delivered:          map[cellKey]bool{},
		elicited:           map[string]bool{},
		modes:              map[shapeKey]*modeEvidence{},
		called:             map[shapeKey]map[string]bool{},
		skipReasons:        map[string]string{},
	}
	c.foldSessions()
	c.foldSkips()
	for _, call := range rt.calls {
		c.foldCall(call)
	}
	c.fillActionCells()
	c.fillCapabilityCells()
	c.fillModeCells()
	c.diagnostics.DispatchLines = rt.dispatches
	c.diagnostics.LateJoins = rt.lateJoins
	return c
}

// foldSessions unions the session lines per surface and mode, and again per
// capability surface.
//
// A session that was not dispatch-observed holds its shape false unless it
// was idle. The idle sessions are the ones that issued no trace. Classified
// without this rule, the 2026-09-23 shards behind the committed record hold
// unobserved default sessions on both runtimes, of two kinds. On dynamic,
// three tier-pin sessions only listed what they serve and made no call at
// all, and the subscription sweep's private session made untraced subscribes
// and one traced resource read. On every surface, the minimal
// capability-surface session's only calls were reads of the tool manifest,
// plus a completion on dynamic. Neither kind says anything about whether the
// row's telemetry worked. The record on main reads those rows observed only
// because its fold judged a session only when a call line under its label was
// a tools/call, which left every one of those sessions out, and that label
// join is what this rule replaces. Setting the idle sessions apart clears the
// first kind, and counting the server span of every method as arriving, which
// the harness now does, clears the second, so the default rows keep reading
// observed at the next record, now without the join.
//
// Idleness is the session line's own word rather than something inferred
// from the call lines beside it, because a label is not unique within a
// shard: the HTTP transport session and a read_api session narrowed to
// read-only each share theirs with another session of the same process, and
// a join on the label would lend one the other's calls.
func (c *classification) foldSessions() {
	for _, session := range c.rt.sessions {
		key := shapeKey{surface: session.Surface, mode: session.Mode}
		shape, seen := c.shapes[key]
		if !seen {
			shape = &sessionShape{
				key: key, tools: map[string]bool{}, resources: map[string]bool{}, prompts: map[string]bool{},
				observed: true,
			}
			c.shapes[key] = shape
		}
		shape.sessions++
		markAll(shape.tools, session.Tools)
		markAll(shape.resources, session.Resources)
		markAll(shape.prompts, session.Prompts)
		shape.templates = mergeSorted(shape.templates, session.ResourceTemplates)
		switch {
		case session.DispatchObserved:
			// Its telemetry arrived, which is all the flag asks.
		case session.Idle:
			shape.idle++
			c.diagnostics.IdleSessions = append(c.diagnostics.IdleSessions, session.Label)
		default:
			shape.observed = false
			c.diagnostics.UnobservedSessions = append(c.diagnostics.UnobservedSessions, session.Label)
		}
		c.foldCapabilitySession(key, session)
	}
	sort.Strings(c.diagnostics.UnobservedSessions)
	sort.Strings(c.diagnostics.IdleSessions)
}

// foldCapabilitySession unions one session line into its capability surface.
//
// The surface is the effective one, so a line written before the harness
// recorded the field is read as the default surface EffectiveCapabilitySurface
// answers, which is what the server serves when the setting is unset, rather
// than as a third surface of its own. An unknown value never starts a server:
// both transports refuse it at startup, so no session line can carry one.
func (c *classification) foldCapabilitySession(shape shapeKey, session *e2ecalls.Session) {
	key := config.EffectiveCapabilitySurface(session.Capabilities)
	surface, seen := c.capabilitySurfaces[key]
	if !seen {
		surface = &capabilityShape{
			key: key, resources: map[string]bool{}, prompts: map[string]bool{}, completions: map[string]bool{},
			kinds: map[string]bool{}, shapes: map[shapeKey]map[string]bool{},
		}
		c.capabilitySurfaces[key] = surface
	}
	surface.sessions++
	markAll(surface.resources, session.Resources)
	markAll(surface.prompts, session.Prompts)
	markAll(surface.completions, session.Completions)
	markAll(surface.kinds, session.SubscribableKinds)
	surface.templates = mergeSorted(surface.templates, session.ResourceTemplates)
	manifest := surface.shapes[shape]
	if manifest == nil {
		manifest = map[string]bool{}
		surface.shapes[shape] = manifest
	}
	for _, listed := range [][]string{session.Resources, session.ResourceTemplates} {
		for _, uri := range listed {
			if isToolManifest(uri) {
				manifest[uri] = true
			}
		}
	}
}

// foldSkips indexes the skip reasons by test.
func (c *classification) foldSkips() {
	for _, skip := range c.rt.skips {
		c.skipReasons[skip.Test] = skip.Reason
	}
}

// markAll sets every name in the set.
func markAll(set map[string]bool, names []string) {
	for _, name := range names {
		set[name] = true
	}
}

// mergeSorted unions two name lists into one sorted, deduplicated list.
func mergeSorted(existing, more []string) []string {
	seen := map[string]bool{}
	for _, name := range existing {
		seen[name] = true
	}
	for _, name := range more {
		seen[name] = true
	}
	merged := make([]string, 0, len(seen))
	for name := range seen {
		merged = append(merged, name)
	}
	sort.Strings(merged)
	return merged
}

// foldCall routes one call line to the cell it is evidence for.
func (c *classification) foldCall(call *e2ecalls.Call) {
	c.diagnostics.Calls++
	if call.TestStatus == "" {
		c.diagnostics.WithoutStatus++
	}
	if call.Outcome == e2ecalls.OutcomeTransportError {
		c.diagnostics.TransportErrors++
	}
	shape := shapeKey{surface: call.Surface, mode: call.Mode}
	// The effective surface, for the reason foldCapabilitySession reads it: a
	// call line from before the harness recorded one is a call on full.
	capabilities := config.EffectiveCapabilitySurface(call.Capabilities)
	switch call.Method {
	case methodCallTool:
		c.foldToolCall(shape, call)
	case methodReadResource:
		target := c.resourceTarget(capabilities, call.Target)
		c.foldCapability(resourceKind(target), shape, capabilities, target, call)
	case methodGetPrompt:
		c.foldCapability(capabilityPrompts, shape, capabilities, call.Target, call)
	case methodComplete:
		c.foldCapability(capabilityCompletions, shape, capabilities, call.Target, call)
	case methodSubscribe, methodListen:
		c.foldCapability(capabilitySubscriptions, shape, capabilities, subscriptionKind(call.Target), call)
	case methodElicit:
		if call.TestStatus == e2ecalls.StatusPassed {
			c.elicited[call.Test] = true
		}
	case methodResourceUpdated:
		if call.TestStatus == e2ecalls.StatusPassed {
			c.delivered[capabilityKey(capabilitySubscriptions, shape, capabilities, subscriptionKind(call.Target))] = true
		}
	}
}

// resourceKind files a read under the kind its target belongs to: the tool
// manifest when the target is one of the pair the tool surface decides, and
// the resources otherwise.
func resourceKind(target string) string {
	if isToolManifest(target) {
		return capabilityToolManifest
	}
	return capabilityResources
}

// foldToolCall routes one tools/call line.
func (c *classification) foldToolCall(shape shapeKey, call *e2ecalls.Call) {
	c.diagnostics.ToolCalls++
	c.noteTool(shape, call)
	if call.Purpose == e2ecalls.PurposeRaw {
		c.diagnostics.RawCalls++
	}
	earned, target := creditOf(call)
	if target == "" {
		return
	}
	if call.Dispatched != "" && call.Action != "" && call.Dispatched != call.Action {
		c.mismatches = append(c.mismatches, mismatch{
			Test: call.Test, Surface: call.Surface, Mode: call.Mode, Requested: call.Action,
			Dispatched: call.Dispatched, Purpose: call.Purpose, TestStatus: call.TestStatus,
		})
	}
	if earned == creditUnobserved {
		c.diagnostics.WithoutDispatch++
	}
	action, known := c.catalog.actions[target]
	if !known {
		c.diagnostics.UnknownActions = mergeSorted(c.diagnostics.UnknownActions, []string{target})
		return
	}
	c.cellFor(cellKey{shape: shape, action: target}).add(earned, call.Test)
	c.noteMode(shape, action, call, earned)
}

// noteTool records which tool a call named on its shape, and lists a tool the
// shape never served.
func (c *classification) noteTool(shape shapeKey, call *e2ecalls.Call) {
	if call.Tool == "" {
		return
	}
	if c.called[shape] == nil {
		c.called[shape] = map[string]bool{}
	}
	c.called[shape][call.Tool] = true
	served, shapeKnown := c.shapes[shape]
	if shapeKnown && !served.tools[call.Tool] {
		c.unresolved = append(c.unresolved, unresolvedTool{
			Test: call.Test, Surface: call.Surface, Mode: call.Mode, Tool: call.Tool, Outcome: call.Outcome,
		})
	}
}

// noteMode records what a call showed of a protective mode.
func (c *classification) noteMode(shape shapeKey, action catalogAction, call *e2ecalls.Call, earned credit) {
	if shape.mode == modeDefault {
		return
	}
	evidence := c.modes[shape]
	if evidence == nil {
		evidence = &modeEvidence{reads: map[string]bool{}, withheld: map[string]bool{}, previews: map[string]bool{}, failed: map[string]bool{}}
		c.modes[shape] = evidence
	}
	switch {
	case earned == creditFailed:
		evidence.failed[call.Test] = true
	case earned == creditPreview && !action.readOnly:
		evidence.previews[call.Test] = true
	case (earned == creditRefused || earned == creditErrorPath) && !action.readOnly:
		evidence.withheld[call.Test] = true
	case (earned == creditAsserted || earned == creditUnobserved || earned == creditSweep) && action.readOnly:
		evidence.reads[call.Test] = true
	}
}

// cellFor returns the cell for a key, creating it on first use.
func (c *classification) cellFor(key cellKey) *cell {
	found, exists := c.cells[key]
	if !exists {
		found = newCell(key)
		c.cells[key] = found
	}
	return found
}

// capabilityCellFor returns a capability cell, creating it on first use.
func (c *classification) capabilityCellFor(kind string, key cellKey) *cell {
	if c.capabilities[kind] == nil {
		c.capabilities[kind] = map[cellKey]*cell{}
	}
	found, exists := c.capabilities[kind][key]
	if !exists {
		found = newCell(key)
		c.capabilities[kind][key] = found
	}
	return found
}

// foldCapability credits one non-tool call to its capability cell, keyed at
// the kind's grain.
func (c *classification) foldCapability(kind string, shape shapeKey, capabilities, target string, call *e2ecalls.Call) {
	if target == "" {
		return
	}
	key := capabilityKey(kind, shape, capabilities, target)
	found, seen := c.capabilities[kind][key]
	if !seen {
		// Unlisted until fillCapabilityCells finds the target among what a
		// session served, which it does after every call is folded.
		found = c.capabilityCellFor(kind, key)
		found.unlisted = true
	}
	found.add(creditOfOutcome(call), call.Test)
}

// resourceTarget names the resource a read addressed as its template, or as
// its own URI when it is a static resource or matches no template the
// capability surface served.
//
// The capability surface and not the shape is what a read is resolved
// against, because it is what decides the set: every shape of one capability
// surface lists the same templates, the manifest pair included.
//
// A read on a capability surface no session line named has no set to resolve
// against, and is still resolved against the manifest pair, which every
// capability surface serves: without it a read of one manifest detail would
// be filed under the resources by its own URI while the index read beside it
// was filed as the manifest.
func (c *classification) resourceTarget(capabilities, uri string) string {
	served, known := c.capabilitySurfaces[capabilities]
	if !known {
		if template, matched := matchTemplate(resources.ToolSurfaceResourceURIs(), uri); matched {
			return template
		}
		return uri
	}
	if served.resources[uri] {
		return uri
	}
	if template, matched := matchTemplate(served.templates, uri); matched {
		return template
	}
	return uri
}

// subscriptionKind names the kind a subscription URI belongs to, and the URI
// itself when it is not one the server accepts.
func subscriptionKind(uri string) string {
	kind, ok := subscriptions.Classify(uri)
	if !ok {
		return uri
	}
	return kind.String()
}

// fillActionCells gives every catalog action a cell on every shape, settling
// the structural states the calls could not.
func (c *classification) fillActionCells() {
	for _, shape := range c.shapes {
		for _, id := range c.catalog.ids {
			action := c.catalog.actions[id]
			found := c.cellFor(cellKey{shape: shape.key, action: id})
			found.state = c.settle(found, c.unservableReason(shape, action))
		}
	}
	for _, found := range c.cells {
		if found.state == "" {
			// A cell a call created on a shape no session line named, which
			// is a shard whose session line was lost. The call still counts.
			found.state = c.settle(found, "")
		}
	}
}

// unservableReason says why a shape cannot serve an action, and returns the
// empty string when it can.
//
// The dynamic surface serves two tools whatever the token's scopes, so its
// own listing cannot say which actions were withheld. The meta sessions of
// the same mode can, because the same token narrowed both: an action whose
// domain tool the meta surface withheld is withheld on dynamic too. When no
// meta session ran in that mode, nothing is inferred and the dynamic surface
// is taken to serve every action its mode allows.
func (c *classification) unservableReason(shape *sessionShape, action catalogAction) string {
	reason := action.unservableReason(shape.key.surface, shape.key.mode, shape.tools)
	if reason != "" || shape.key.surface != config.ToolSurfaceDynamic {
		return reason
	}
	meta, ran := c.shapes[shapeKey{surface: config.ToolSurfaceMeta, mode: shape.key.mode}]
	if ran && action.metaTool != "" && !meta.tools[action.metaTool] {
		return "tool " + action.metaTool + " not served on meta, so the action is withheld here too"
	}
	return ""
}

// settle picks a cell's state from its best credit and, when no call earned
// any, from the structural reason.
func (c *classification) settle(found *cell, unservable string) state {
	if found.best != creditNone {
		found.reason = c.reasonFor(found)
		return creditStates[found.best]
	}
	if unservable != "" {
		found.reason = unservable
		return stateUnservable
	}
	return stateAbsent
}

// reasonFor spells the reason a skipped cell carries: the skip line's reason
// of the test that skipped, when there is one.
//
// The tests are read in name order, so that two skipped tests giving two
// reasons for one cell settle on the same one every run: the report is a CI
// artifact, and a reason that changed between two runs of one shard set
// would read as a change in the suite.
func (c *classification) reasonFor(found *cell) string {
	if found.best != creditSkipped {
		return ""
	}
	for _, test := range sortedKeys(found.tests[creditSkipped]) {
		if reason, known := c.skipReasons[test]; known {
			return reason
		}
		if reason, known := c.skipReasons[topLevelTest(test)]; known {
			return reason
		}
	}
	return ""
}

// fillCapabilityCells gives every item a capability surface served a cell at
// its kind's grain, and settles the elicitation flows from the interactive
// actions' cells.
//
// A cell a call created outside those denominators, a read of a URI nothing
// listed, a completion for an argument no prompt or template declares, or a
// subscription to a kind the server refuses, is settled too and stays
// unlisted: it is evidence of what was called, published apart from the
// histogram so that each histogram row sums to the figure it is counted
// against.
func (c *classification) fillCapabilityCells() {
	for _, surface := range c.capabilitySurfaces {
		for kind, items := range surface.served() {
			for _, item := range items {
				c.settleCapability(kind, capabilityKey(kind, shapeKey{}, surface.key, item))
			}
		}
		for shape, items := range surface.shapes {
			for item := range items {
				c.settleCapability(capabilityToolManifest, capabilityKey(capabilityToolManifest, shape, surface.key, item))
			}
		}
	}
	for _, cells := range c.capabilities {
		for _, found := range cells {
			if found.state == "" {
				found.state = c.settle(found, "")
			}
		}
	}
	c.fillElicitationCells()
}

// served lists what one capability surface serves of each kind counted at the
// capability grain: the cells the kind gets on it, and the figure its row in
// the report publishes, which are one list so the two cannot disagree.
//
// The tool-manifest pair is left out of the resources, being counted per shape
// and capability surface as a kind of its own, and the subscribable kinds are listed only on the full
// surface, which is the only one the server accepts a subscription on.
func (s *capabilityShape) served() map[string][]string {
	var items []string
	for _, uri := range sortedKeys(s.resources) {
		if !isToolManifest(uri) {
			items = append(items, uri)
		}
	}
	for _, template := range s.templates {
		if !isToolManifest(template) {
			items = append(items, template)
		}
	}
	served := map[string][]string{
		capabilityResources:   items,
		capabilityPrompts:     sortedKeys(s.prompts),
		capabilityCompletions: sortedKeys(s.completions),
	}
	if s.key == config.CapabilitySurfaceFull {
		served[capabilitySubscriptions] = subscribableKinds(s.kinds)
	}
	return served
}

// settleCapability creates a capability cell for something a session served
// and settles its state.
func (c *classification) settleCapability(kind string, key cellKey) {
	found := c.capabilityCellFor(kind, key)
	found.unlisted = false
	found.state = c.settle(found, "")
}

// subscribableKinds names the kinds a session said it accepts, and every kind
// the server accepts when the session line did not say: the harness leaves
// that list empty today, and the server's whitelist is the same on every
// instance.
func subscribableKinds(listed map[string]bool) []string {
	if len(listed) > 0 {
		return sortedKeys(listed)
	}
	return templateKinds(subscriptions.Templates())
}

// templateKinds names the kind each template's sample URI classifies as,
// sorted, and leaves out a template whose sample the server would not accept.
//
// The server's own templates all classify, which its drift guards hold it
// to, so the leaving out is for a template whose variables sampleURI fills
// in a way the classifier refuses: counting that one as a kind would give the
// histogram a subscription cell the server has no name for.
func templateKinds(templates []string) []string {
	kinds := make([]string, 0, len(templates))
	for _, template := range templates {
		kind, ok := subscriptions.Classify(sampleURI(template))
		if ok {
			kinds = append(kinds, kind.String())
		}
	}
	sort.Strings(kinds)
	return kinds
}

// sampleURI expands a subscription template with placeholder values, so the
// classifier can name the template's kind: a numeric id where the variable
// names one, and a word elsewhere.
func sampleURI(template string) string {
	var b strings.Builder
	rest := template
	for {
		before, after, opened := strings.Cut(rest, "{")
		variable, tail, closed := strings.Cut(after, "}")
		if !opened || !closed {
			// No placeholder left, or one that never closes: the rest is
			// literal text, as a URI template reader would take it.
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(before)
		if strings.HasSuffix(variable, "_id") || strings.HasSuffix(variable, "_iid") {
			b.WriteString("1")
		} else {
			b.WriteString("sample")
		}
		rest = tail
	}
}

// fillElicitationCells derives one cell per elicitation flow from the
// interactive actions' cells, marking whether the flow elicited.
//
// It derives rather than shares, and runs again after applyStatic: the flow
// is the interactive action seen as a capability, so whatever the source
// later says of the action (skipped, unasserted) the flow says too.
func (c *classification) fillElicitationCells() {
	for key, found := range c.cells {
		// Every cell names a catalog action: fillActionCells makes one per
		// catalog id and foldToolCall refuses a target the catalog lacks, so
		// there is no unknown here to test for, and one would carry the
		// empty domain and fall out of this comparison anyway.
		if c.catalog.actions[key.action].domain != interactiveDomain {
			continue
		}
		flow := c.capabilityCellFor(capabilityElicitation, capabilityKey(capabilityElicitation, key.shape, "", key.action))
		flow.copyFrom(found)
		if flow.state == stateAsserted && !c.anyElicited(found.tests[creditAsserted]) {
			// The tool answered and nothing was asked of the client: the
			// flow ran its cancelled or unsupported branch rather than
			// eliciting, which is the error path of the capability.
			flow.state = stateErrorPathOnly
			flow.reason = "the call succeeded without an elicitation request"
		}
	}
}

// anyElicited reports whether any of the tests saw an elicitation request.
func (c *classification) anyElicited(tests map[string]bool) bool {
	for test := range tests {
		if c.elicited[test] {
			return true
		}
	}
	return false
}

// The facets of a protective mode, which are the targets of its cells.
const (
	facetReads    = "reads"
	facetWithheld = "withheld"
	facetPreviews = "previews"
)

// fillModeCells settles the protective modes: on every read-only or safe
// shape, one cell for reads going through and one for mutations being
// withheld or previewed.
func (c *classification) fillModeCells() {
	for key := range c.shapes {
		if key.mode == modeDefault {
			continue
		}
		evidence := c.modes[key]
		if evidence == nil {
			evidence = &modeEvidence{}
		}
		c.settleFacet(key, facetReads, evidence.reads, evidence.failed)
		if key.mode == modeReadOnly {
			c.settleFacet(key, facetWithheld, evidence.withheld, evidence.failed)
		}
		if key.mode == modeSafe {
			c.settleFacet(key, facetPreviews, evidence.previews, evidence.failed)
		}
	}
}

// settleFacet writes one mode cell: asserted when a passing test showed the
// facet, failed when only a failing test touched the shape, absent otherwise.
func (c *classification) settleFacet(key shapeKey, facet string, shown, failed map[string]bool) {
	found := c.capabilityCellFor(capabilityModes, capabilityKey(capabilityModes, key, "", facet))
	for test := range shown {
		found.add(creditAsserted, test)
	}
	if found.best == creditNone {
		for test := range failed {
			found.add(creditFailed, test)
		}
	}
	found.state = c.settle(found, "")
}

// applyStatic folds in what only the source can say: an asserted cell whose
// every result-bearing call site discards the result is unasserted, and an
// absent cell named by a test that skipped is skipped, with the reason the
// test gave.
//
// The skip attribution is by test, not by surface: a test that skipped before
// its first call never said which surface it would have used, so the skip
// lands on every shape the run served.
func (c *classification) applyStatic(sr *staticResult) {
	for _, found := range c.cells {
		if found.state == stateAsserted && sr.unassertedIDs[found.key.action] {
			found.state = stateUnasserted
			found.reason = "every call site that receives the result discards it"
		}
	}
	for _, skip := range c.rt.skips {
		for id := range sr.testIDs[topLevelTest(skip.Test)] {
			for shape := range c.shapes {
				found, exists := c.cells[cellKey{shape: shape, action: id}]
				if exists && found.state == stateAbsent {
					found.state = stateSkipped
					found.reason = skip.Reason
					found.add(creditSkipped, skip.Test)
				}
			}
		}
	}
	// The flows were derived from the interactive actions' cells before any
	// of this ran, so they are derived again from what those cells say now.
	c.fillElicitationCells()
}

// sortedKeys returns a map's keys in order. It is generic because the same
// question is asked of a set here and of the coverage record's runtimes in
// runtime_record_check.go, and two spellings of six lines is how they drift.
func sortedKeys[V any](set map[string]V) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// matchTemplate finds the most specific served template a URI expands, where
// a {name} variable matches one path segment and a {+name} variable matches
// the rest of the path.
//
// Most specific means the most literal segments, so that a URI under
// project/{id}/mr/{iid}/notes is not credited to project/{id}/mr/{iid}.
func matchTemplate(templates []string, uri string) (string, bool) {
	best, bestLiterals := "", -1
	for _, template := range templates {
		literals, matched := templateMatch(template, uri)
		if matched && literals > bestLiterals {
			best, bestLiterals = template, literals
		}
	}
	return best, bestLiterals >= 0
}

// templateMatch reports whether uri expands template, and how many literal
// segments the match consumed.
func templateMatch(template, uri string) (int, bool) {
	tSegs := strings.Split(template, "/")
	uSegs := strings.Split(uri, "/")
	literals := 0
	for i, seg := range tSegs {
		if i >= len(uSegs) {
			return 0, false
		}
		switch {
		case strings.HasPrefix(seg, "{+") && strings.HasSuffix(seg, "}"):
			// The reserved expansion takes the rest of the path, one segment
			// at least.
			return literals, i == len(tSegs)-1 && uSegs[i] != ""
		case strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}"):
			if uSegs[i] == "" {
				return 0, false
			}
		case seg != uSegs[i]:
			return 0, false
		default:
			literals++
		}
	}
	return literals, len(tSegs) == len(uSegs)
}
