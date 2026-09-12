package main

import (
	"maps"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
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
func creditOf(call *e2ecalls.Call) (earned credit, target string) {
	target = call.Dispatched
	if target == "" {
		target = call.Action
	}
	if target == "" || call.Purpose == e2ecalls.PurposeRaw {
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
// over every session line with that shape.
type sessionShape struct {
	key shapeKey
	// tools, resources, templates, prompts, completions and kinds are the
	// union of what those sessions listed.
	tools       map[string]bool
	resources   map[string]bool
	templates   []string
	prompts     map[string]bool
	completions map[string]bool
	kinds       map[string]bool
	// full is whether any session of the shape served the full capability
	// surface, which is the one subscriptions exist on.
	full bool
	// observed is whether every session of the shape saw its probe span.
	observed bool
	// sessions counts the session lines folded in.
	sessions int
}

// cellKey names one surface x mode x action.
type cellKey struct {
	shape  shapeKey
	action string
}

// cell is one surface x mode x action with everything the calls said.
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
	if earned > c.best {
		c.best = earned
	}
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
	// UnobservedSessions names the sessions whose probe span never arrived.
	UnobservedSessions []string `json:"unobserved_sessions,omitempty"`
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
	// cells is every surface x mode x action, filled for every catalog
	// action on every shape once the calls are in.
	cells map[cellKey]*cell
	// capability cells, keyed by kind, then by surface x mode x target.
	capabilities map[string]map[cellKey]*cell
	// delivered marks (shape, kind) pairs a resource-updated notification
	// reached in a passing test.
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
	capabilityElicitation   = "elicitation"
	capabilityModes         = "modes"
)

// classify judges one runtime's records against its catalog.
func classify(rt *runtimeRecords, catalog *servedCatalog) *classification {
	c := &classification{
		rt:           rt,
		catalog:      catalog,
		shapes:       map[shapeKey]*sessionShape{},
		cells:        map[cellKey]*cell{},
		capabilities: map[string]map[cellKey]*cell{},
		delivered:    map[cellKey]bool{},
		elicited:     map[string]bool{},
		modes:        map[shapeKey]*modeEvidence{},
		called:       map[shapeKey]map[string]bool{},
		skipReasons:  map[string]string{},
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

// foldSessions unions the session lines per surface and mode.
func (c *classification) foldSessions() {
	for _, session := range c.rt.sessions {
		key := shapeKey{surface: session.Surface, mode: session.Mode}
		shape, seen := c.shapes[key]
		if !seen {
			shape = &sessionShape{
				key: key, tools: map[string]bool{}, resources: map[string]bool{},
				prompts: map[string]bool{}, completions: map[string]bool{}, kinds: map[string]bool{},
				observed: true,
			}
			c.shapes[key] = shape
		}
		shape.sessions++
		markAll(shape.tools, session.Tools)
		markAll(shape.resources, session.Resources)
		markAll(shape.prompts, session.Prompts)
		markAll(shape.completions, session.Completions)
		markAll(shape.kinds, session.SubscribableKinds)
		shape.templates = mergeSorted(shape.templates, session.ResourceTemplates)
		shape.full = shape.full || session.Capabilities == config.CapabilitySurfaceFull
		if !session.DispatchObserved {
			shape.observed = false
			c.diagnostics.UnobservedSessions = append(c.diagnostics.UnobservedSessions, session.Label)
		}
	}
	sort.Strings(c.diagnostics.UnobservedSessions)
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
	switch call.Method {
	case methodCallTool:
		c.foldToolCall(shape, call)
	case methodReadResource:
		c.foldCapability(capabilityResources, shape, c.resourceTarget(shape, call.Target), call)
	case methodGetPrompt:
		c.foldCapability(capabilityPrompts, shape, call.Target, call)
	case methodComplete:
		c.foldCapability(capabilityCompletions, shape, call.Target, call)
	case methodSubscribe, methodListen:
		c.foldCapability(capabilitySubscriptions, shape, subscriptionKind(call.Target), call)
	case methodElicit:
		if call.TestStatus == e2ecalls.StatusPassed {
			c.elicited[call.Test] = true
		}
	case methodResourceUpdated:
		if call.TestStatus == e2ecalls.StatusPassed {
			c.delivered[cellKey{shape: shape, action: subscriptionKind(call.Target)}] = true
		}
	}
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

// foldCapability credits one non-tool call to its capability cell.
func (c *classification) foldCapability(kind string, shape shapeKey, target string, call *e2ecalls.Call) {
	if target == "" {
		return
	}
	c.capabilityCellFor(kind, cellKey{shape: shape, action: target}).add(creditOfOutcome(call), call.Test)
}

// resourceTarget names the resource a read addressed as its template, or as
// its own URI when it is a static resource or matches no template the shape
// served.
func (c *classification) resourceTarget(shape shapeKey, uri string) string {
	served, known := c.shapes[shape]
	if !known {
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

// fillCapabilityCells gives every served resource, template, prompt,
// completion and subscribable kind a cell on the shapes that served it, and
// settles the elicitation flows from the interactive actions' cells.
func (c *classification) fillCapabilityCells() {
	for _, shape := range c.shapes {
		for _, uri := range sortedKeys(shape.resources) {
			c.settleCapability(capabilityResources, cellKey{shape: shape.key, action: uri})
		}
		for _, template := range shape.templates {
			c.settleCapability(capabilityResources, cellKey{shape: shape.key, action: template})
		}
		for _, name := range sortedKeys(shape.prompts) {
			c.settleCapability(capabilityPrompts, cellKey{shape: shape.key, action: name})
		}
		for _, ref := range sortedKeys(shape.completions) {
			c.settleCapability(capabilityCompletions, cellKey{shape: shape.key, action: ref})
		}
		if shape.full {
			for _, kind := range subscribableKinds(shape.kinds) {
				c.settleCapability(capabilitySubscriptions, cellKey{shape: shape.key, action: kind})
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

// settleCapability creates a capability cell for something a shape served and
// settles its state.
func (c *classification) settleCapability(kind string, key cellKey) {
	found := c.capabilityCellFor(kind, key)
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
	kinds := make([]string, 0, len(subscriptions.Templates()))
	for _, template := range subscriptions.Templates() {
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
		open := strings.Index(rest, "{")
		if open < 0 {
			b.WriteString(rest)
			return b.String()
		}
		closing := strings.Index(rest[open:], "}")
		if closing < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:open])
		variable := rest[open+1 : open+closing]
		if strings.HasSuffix(variable, "_id") || strings.HasSuffix(variable, "_iid") {
			b.WriteString("1")
		} else {
			b.WriteString("sample")
		}
		rest = rest[open+closing+1:]
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
		action, known := c.catalog.actions[key.action]
		if !known || action.domain != interactiveDomain {
			continue
		}
		flow := c.capabilityCellFor(capabilityElicitation, key)
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
	found := c.capabilityCellFor(capabilityModes, cellKey{shape: key, action: facet})
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

// sortedKeys returns a set's names in order.
func sortedKeys(set map[string]bool) []string {
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
