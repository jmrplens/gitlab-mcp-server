package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// The fixture's shapes.
var (
	dynamicDefault    = shapeKey{surface: config.ToolSurfaceDynamic, mode: modeDefault}
	metaDefault       = shapeKey{surface: config.ToolSurfaceMeta, mode: modeDefault}
	individualDefault = shapeKey{surface: config.ToolSurfaceIndividual, mode: modeDefault}
	metaReadOnly      = shapeKey{surface: config.ToolSurfaceMeta, mode: modeReadOnly}
	individualSafe    = shapeKey{surface: config.ToolSurfaceIndividual, mode: modeSafe}
)

// fixtureCatalog is a catalog of a few actions with every shape the
// classification distinguishes: a read, a mutation, a destructive action, an
// action shadowed on individual, one with no individual tool, one a scope
// withholds, an elicitation flow and a rewritten route.
func fixtureCatalog() *servedCatalog {
	actions := []catalogAction{
		{id: "issue.list", domain: "issue", readOnly: true, metaTool: "gitlab_issue", individualTool: "gitlab_issue_list"},
		{id: "issue.create", domain: "issue", metaTool: "gitlab_issue", individualTool: "gitlab_issue_create"},
		{id: "issue.delete", domain: "issue", destructive: true, metaTool: "gitlab_issue", individualTool: "gitlab_issue_delete"},
		{id: "project.get", domain: "project", readOnly: true, metaTool: "gitlab_project", individualTool: "gitlab_project_get"},
		{id: "project.list", domain: "project", readOnly: true, metaTool: "gitlab_project", individualTool: "gitlab_project_list"},
		{id: "repository.file_history", domain: "repository", readOnly: true, metaTool: "gitlab_repository", individualOwner: "repository.file"},
		{id: "server.health_check", domain: "server", readOnly: true, metaTool: "gitlab_server"},
		{id: "admin.list", domain: "admin", readOnly: true, metaTool: "gitlab_admin", individualTool: "gitlab_admin_list"},
		{id: "environment.get", domain: "environment", readOnly: true, metaTool: "gitlab_environment", individualTool: "gitlab_environment_get"},
		{id: "environment.protected_get", domain: "environment", readOnly: true, metaTool: "gitlab_environment", individualTool: "gitlab_environment_protected_get"},
		{id: "interactive.issue_create", domain: interactiveDomain, standalone: true, metaTool: "gitlab_interactive_issue_create", individualTool: "gitlab_interactive_issue_create"},
		{id: "merge_train.list", domain: "merge_train", tier: edition.Premium, readOnly: true, metaTool: "gitlab_merge_train", individualTool: "gitlab_merge_train_list"},
	}
	catalog := &servedCatalog{tier: edition.Free, actions: map[string]catalogAction{}}
	for _, action := range actions {
		catalog.actions[action.id] = action
		catalog.ids = append(catalog.ids, action.id)
	}
	sort.Strings(catalog.ids)
	return catalog
}

// servedTools are the tools every fixture session lists: everything but the
// admin group, which the token's scopes withhold.
var servedTools = []string{
	"gitlab_execute_action", "gitlab_find_action", "gitlab_issue", "gitlab_project", "gitlab_repository", "gitlab_server",
	"gitlab_environment", "gitlab_interactive_issue_create", "gitlab_merge_train",
	"gitlab_issue_list", "gitlab_issue_create", "gitlab_issue_delete", "gitlab_project_get", "gitlab_project_list",
	"gitlab_environment_get", "gitlab_environment_protected_get", "gitlab_merge_train_list",
}

// fixtureSession builds one session line.
func fixtureSession(key shapeKey, observed bool) *e2ecalls.Session {
	return &e2ecalls.Session{
		Label: key.surface + "/" + key.mode, Surface: key.surface, Mode: key.mode,
		Capabilities: config.CapabilitySurfaceFull, Transport: "stdio", Tools: servedTools,
		Resources:         []string{"gitlab://groups"},
		ResourceTemplates: []string{"gitlab://project/{project_id}", "gitlab://project/{project_id}/issue/{issue_iid}", "gitlab://project/{project_id}/file/{ref}/{+path}"},
		Prompts:           []string{"summarize_issue"},
		DispatchObserved:  observed,
	}
}

// callSpec is the short form a fixture call is written in.
type callSpec struct {
	test, purpose, expectation, method, action, dispatched, target, outcome, status string
	shape                                                                           shapeKey
}

// fixtureCall expands a spec into a call line.
func fixtureCall(spec callSpec) *e2ecalls.Call {
	if spec.purpose == "" {
		spec.purpose = e2ecalls.PurposeTest
	}
	if spec.expectation == "" {
		spec.expectation = e2ecalls.ExpectationOK
	}
	if spec.method == "" {
		spec.method = methodCallTool
	}
	if spec.outcome == "" {
		spec.outcome = e2ecalls.OutcomeOK
	}
	if spec.status == "" {
		spec.status = e2ecalls.StatusPassed
	}
	tool := ""
	if spec.method == methodCallTool {
		tool = toolFor(spec.shape.surface, spec.action)
	}
	return &e2ecalls.Call{
		Test: spec.test, Purpose: spec.purpose, Expectation: spec.expectation, Session: spec.shape.surface + "/" + spec.shape.mode,
		Surface: spec.shape.surface, Mode: spec.shape.mode, Capabilities: config.CapabilitySurfaceFull, Requirement: "any",
		Method: spec.method, Tool: tool, Action: spec.action, Dispatched: spec.dispatched, Target: spec.target,
		Outcome: spec.outcome, TraceID: spec.test + spec.action, TestStatus: spec.status,
	}
}

// toolFor names the tool a fixture call to an action goes to on a surface.
func toolFor(surface, action string) string {
	catalog := fixtureCatalog()
	entry, known := catalog.actions[action]
	if !known {
		return "gitlab_execute_action"
	}
	tool, _ := entry.toolOn(surface)
	return tool
}

// fixtureRuntime is the runtime every classification test reads: one call or
// two per state, on the shapes that show it.
func fixtureRuntime() *runtimeRecords {
	rt := &runtimeRecords{
		dir: "fixture", key: "community/free", edition: "community", tier: edition.Free,
		runs: []*e2ecalls.Run{{Package: "common", Requirement: "any", Edition: "community", Tier: "free", Status: e2ecalls.RunStarted}},
		sessions: []*e2ecalls.Session{
			fixtureSession(dynamicDefault, true), fixtureSession(metaDefault, false), fixtureSession(individualDefault, true),
			fixtureSession(metaReadOnly, true), fixtureSession(individualSafe, true),
		},
		skips: []*e2ecalls.Skip{{Test: "TestSkipped", Reason: "no runner"}},
	}
	specs := []callSpec{
		// asserted on dynamic, unobserved on meta, sweep-only on individual.
		{test: "TestIssueList", action: "issue.list", dispatched: "issue.list", shape: dynamicDefault},
		{test: "TestIssueList", action: "issue.list", shape: metaDefault},
		{test: "TestSweep", purpose: e2ecalls.PurposeSweep, action: "issue.list", dispatched: "issue.list", shape: individualDefault},
		// error-path-only, refused-only, cleanup-only, preview-only.
		{test: "TestIssueCreate", expectation: "tool_error", action: "issue.create", dispatched: "issue.create", outcome: e2ecalls.OutcomeToolError, shape: dynamicDefault},
		{test: "TestIssueDelete", expectation: "needs_confirmation", action: "issue.delete", dispatched: "issue.delete", outcome: e2ecalls.RefusedOutcome("needs_confirmation"), shape: dynamicDefault},
		{test: "TestIssueDelete", purpose: e2ecalls.PurposeCleanup, expectation: e2ecalls.ExpectationAny, action: "issue.delete", dispatched: "issue.delete", shape: metaDefault},
		{test: "TestSafe", expectation: "safe_mode", action: "issue.create", dispatched: "issue.create", outcome: e2ecalls.OutcomePreview, shape: individualSafe},
		{test: "TestSafe", action: "issue.list", dispatched: "issue.list", shape: individualSafe},
		// failed and skipped.
		{test: "TestFailing", action: "project.get", dispatched: "project.get", status: e2ecalls.StatusFailed, shape: dynamicDefault},
		{test: "TestSkipping", action: "project.get", dispatched: "project.get", status: e2ecalls.StatusSkipped, shape: metaDefault},
		// a call the transport lost, which credits nothing.
		{test: "TestLost", action: "project.get", outcome: e2ecalls.OutcomeTransportError, shape: individualDefault},
		// a rewritten route: credit goes to what ran.
		{test: "TestEnvironment", action: "environment.get", dispatched: "environment.protected_get", shape: metaDefault},
		// the read-only mode: a read goes through, a mutation is withheld.
		{test: "TestReadOnly", action: "issue.list", dispatched: "issue.list", shape: metaReadOnly},
		{test: "TestReadOnly", expectation: "unknown_action", action: "issue.create", outcome: e2ecalls.RefusedOutcome("unknown_action"), shape: metaReadOnly},
		// an action the catalog lacks, and a raw call to a tool nobody serves.
		{test: "TestGhost", action: "ghost.action", outcome: e2ecalls.OutcomeToolError, expectation: e2ecalls.ExpectationAny, shape: dynamicDefault},
		// the elicitation flow: elicited on meta, answered without eliciting on dynamic.
		{test: "TestInteractive", action: "interactive.issue_create", dispatched: "interactive.issue_create", shape: metaDefault},
		{test: "TestInteractive", method: methodElicit, target: "Confirm?", expectation: e2ecalls.ExpectationAny, shape: metaDefault},
		{test: "TestInteractiveCancelled", action: "interactive.issue_create", dispatched: "interactive.issue_create", shape: dynamicDefault},
		// the non-tool verbs.
		{test: "TestResources", method: methodReadResource, target: "gitlab://project/1/issue/5", shape: dynamicDefault},
		{test: "TestResources", method: methodReadResource, target: "gitlab://project/1/file/main/docs/a.md", shape: dynamicDefault},
		{test: "TestResources", method: methodReadResource, target: "gitlab://groups", shape: dynamicDefault},
		{test: "TestResources", method: methodReadResource, target: "gitlab://nowhere", outcome: e2ecalls.OutcomeProtocolError, expectation: "protocol_error", shape: dynamicDefault},
		{test: "TestPrompts", method: methodGetPrompt, target: "summarize_issue", shape: metaDefault},
		{test: "TestCompletions", method: methodComplete, target: "summarize_issue project_id", shape: metaDefault},
		{test: "TestSubscriptions", method: methodSubscribe, target: "gitlab://project/1/issue/5", shape: dynamicDefault},
		{test: "TestSubscriptions", method: methodResourceUpdated, target: "gitlab://project/1/issue/5", expectation: e2ecalls.ExpectationAny, shape: dynamicDefault},
		{test: "TestSubscriptions", method: methodListen, target: "gitlab://project/1/pipeline/9", shape: dynamicDefault},
	}
	for _, spec := range specs {
		rt.calls = append(rt.calls, fixtureCall(spec))
	}
	raw := fixtureCall(callSpec{test: "TestRaw", purpose: e2ecalls.PurposeRaw, expectation: e2ecalls.ExpectationAny, shape: metaDefault, outcome: e2ecalls.OutcomeProtocolError})
	raw.Tool = "gitlab_nope"
	rt.calls = append(rt.calls, raw)
	return rt
}

// stateOf reads one action cell's state off a classification.
func stateOf(c *classification, shape shapeKey, action string) state {
	found, exists := c.cells[cellKey{shape: shape, action: action}]
	if !exists {
		return "missing"
	}
	return found.state
}

// TestClassify_States_EachCellGetsOne walks every state the classification
// can assign and the cell in the fixture that shows it, which is the table
// the whole report is read through.
func TestClassify_States_EachCellGetsOne(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())

	cases := []struct {
		name   string
		shape  shapeKey
		action string
		want   state
	}{
		{name: "asserted", shape: dynamicDefault, action: "issue.list", want: stateAsserted},
		{name: "unobserved when no span confirmed the dispatch", shape: metaDefault, action: "issue.list", want: stateUnobserved},
		{name: "sweep-only", shape: individualDefault, action: "issue.list", want: stateSweepOnly},
		{name: "error-path-only", shape: dynamicDefault, action: "issue.create", want: stateErrorPathOnly},
		{name: "refused-only", shape: dynamicDefault, action: "issue.delete", want: stateRefusedOnly},
		{name: "cleanup-only", shape: metaDefault, action: "issue.delete", want: stateCleanupOnly},
		{name: "preview-only", shape: individualSafe, action: "issue.create", want: statePreviewOnly},
		{name: "failed", shape: dynamicDefault, action: "project.get", want: stateFailed},
		{name: "skipped from a skipped test's call", shape: metaDefault, action: "project.get", want: stateSkipped},
		{name: "a transport error credits nothing", shape: individualDefault, action: "project.get", want: stateAbsent},
		{name: "absent", shape: dynamicDefault, action: "project.list", want: stateAbsent},
		{name: "unservable when the individual name is shadowed", shape: individualDefault, action: "repository.file_history", want: stateUnservable},
		{name: "unservable with no individual tool", shape: individualDefault, action: "server.health_check", want: stateUnservable},
		{name: "unservable when the scope withholds the meta tool", shape: metaDefault, action: "admin.list", want: stateUnservable},
		{name: "unservable on dynamic when meta withheld the domain", shape: dynamicDefault, action: "admin.list", want: stateUnservable},
		{name: "unservable when read-only withholds a mutation", shape: metaReadOnly, action: "issue.delete", want: stateUnservable},
		{name: "a rewritten route credits what ran", shape: metaDefault, action: "environment.protected_get", want: stateAsserted},
		{name: "a rewritten route does not credit what was asked", shape: metaDefault, action: "environment.get", want: stateAbsent},
		{name: "a mutation refused in read-only is refused-only there", shape: metaReadOnly, action: "issue.create", want: stateRefusedOnly},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stateOf(c, tc.shape, tc.action); got != tc.want {
				t.Errorf("state(%s/%s, %s) = %s, want %s", tc.shape.surface, tc.shape.mode, tc.action, got, tc.want)
			}
		})
	}
}

// TestClassify_Reasons_Explain verifies that a cell without credit says why:
// the shadowing sibling, the missing tool, the withheld group, the mode, and
// a skipped test's reason.
func TestClassify_Reasons_Explain(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())

	cases := []struct {
		name   string
		shape  shapeKey
		action string
		want   string
	}{
		{name: "shadowed", shape: individualDefault, action: "repository.file_history", want: "individual tool name is registered for repository.file"},
		{name: "no tool", shape: individualDefault, action: "server.health_check", want: "no individual tool"},
		{name: "withheld", shape: metaDefault, action: "admin.list", want: "tool gitlab_admin not served"},
		{name: "withheld on dynamic", shape: dynamicDefault, action: "admin.list", want: "tool gitlab_admin not served on meta, so the action is withheld here too"},
		{name: "read-only", shape: metaReadOnly, action: "issue.delete", want: "withheld by read-only mode"},
		{name: "asserted has none", shape: dynamicDefault, action: "issue.list", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found := c.cells[cellKey{shape: tc.shape, action: tc.action}]
			if found == nil || found.reason != tc.want {
				t.Errorf("reason = %q, want %q", found.reason, tc.want)
			}
		})
	}
}

// TestClassify_Precedence_BestCreditWins verifies that a cell with several
// calls takes its state from the best of them, so one asserted call is not
// hidden by a later cleanup or a failed sibling.
func TestClassify_Precedence_BestCreditWins(t *testing.T) {
	rt := fixtureRuntime()
	rt.calls = append(rt.calls,
		fixtureCall(callSpec{test: "TestIssueListAgain", purpose: e2ecalls.PurposeCleanup, action: "issue.list", dispatched: "issue.list", shape: dynamicDefault}),
		fixtureCall(callSpec{test: "TestIssueListFailing", action: "issue.list", dispatched: "issue.list", status: e2ecalls.StatusFailed, shape: dynamicDefault}),
	)
	c := classify(rt, fixtureCatalog())

	found := c.cells[cellKey{shape: dynamicDefault, action: "issue.list"}]
	if found.state != stateAsserted {
		t.Errorf("state = %s, want asserted", found.state)
	}
	if want := []string{"TestIssueList"}; !reflect.DeepEqual(found.bestTests(), want) {
		t.Errorf("bestTests() = %q, want %q", found.bestTests(), want)
	}
	if found.counts[creditCleanup] != 1 || found.counts[creditFailed] != 1 {
		t.Errorf("counts = %v, want one cleanup and one failed beside the assertion", found.counts)
	}
}

// TestClassify_Mismatches_Listed verifies that a call whose dispatched action
// differs from the one it named is listed with both halves.
func TestClassify_Mismatches_Listed(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())

	want := []mismatch{{
		Test: "TestEnvironment", Surface: config.ToolSurfaceMeta, Mode: modeDefault, Requested: "environment.get",
		Dispatched: "environment.protected_get", Purpose: e2ecalls.PurposeTest, TestStatus: e2ecalls.StatusPassed,
	}}
	if !reflect.DeepEqual(c.mismatches, want) {
		t.Errorf("mismatches = %+v, want %+v", c.mismatches, want)
	}
}

// TestClassify_Diagnostics_CountTheRecord verifies the counters that are
// about the record itself: the unknown action, the raw call, the unresolved
// tool, the lost call, the unobserved session and the call without a
// dispatch.
func TestClassify_Diagnostics_CountTheRecord(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())

	want := diagnostics{
		Calls: len(fixtureRuntime().calls), ToolCalls: 18, WithoutDispatch: 1, TransportErrors: 1, RawCalls: 1,
		UnknownActions: []string{"ghost.action"}, UnobservedSessions: []string{"meta/default"},
	}
	if !reflect.DeepEqual(c.diagnostics, want) {
		t.Errorf("diagnostics = %+v, want %+v", c.diagnostics, want)
	}
	wantUnresolved := []unresolvedTool{{Test: "TestRaw", Surface: config.ToolSurfaceMeta, Mode: modeDefault, Tool: "gitlab_nope", Outcome: e2ecalls.OutcomeProtocolError}}
	if !reflect.DeepEqual(c.unresolved, wantUnresolved) {
		t.Errorf("unresolved = %+v, want %+v", c.unresolved, wantUnresolved)
	}
}

// capabilityState reads one capability cell's state.
func capabilityState(c *classification, kind string, shape shapeKey, target string) state {
	found, exists := c.capabilities[kind][cellKey{shape: shape, action: target}]
	if !exists {
		return "missing"
	}
	return found.state
}

// TestClassify_Capabilities_ClassifiedByTarget verifies the non-tool
// classification: a resource by the template it expands or its own static
// URI, a prompt by name, a completion by reference and argument, a
// subscription by kind, and the elicitation flow by whether it elicited.
func TestClassify_Capabilities_ClassifiedByTarget(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())

	cases := []struct {
		name   string
		kind   string
		shape  shapeKey
		target string
		want   state
	}{
		{name: "resource by template", kind: capabilityResources, shape: dynamicDefault, target: "gitlab://project/{project_id}/issue/{issue_iid}", want: stateAsserted},
		{name: "resource by reserved-expansion template", kind: capabilityResources, shape: dynamicDefault, target: "gitlab://project/{project_id}/file/{ref}/{+path}", want: stateAsserted},
		{name: "static resource by its URI", kind: capabilityResources, shape: dynamicDefault, target: "gitlab://groups", want: stateAsserted},
		{name: "resource nothing read", kind: capabilityResources, shape: dynamicDefault, target: "gitlab://project/{project_id}", want: stateAbsent},
		{name: "resource nothing serves keeps its URI", kind: capabilityResources, shape: dynamicDefault, target: "gitlab://nowhere", want: stateErrorPathOnly},
		{name: "prompt by name", kind: capabilityPrompts, shape: metaDefault, target: "summarize_issue", want: stateAsserted},
		{name: "prompt on a shape nothing rendered it on", kind: capabilityPrompts, shape: dynamicDefault, target: "summarize_issue", want: stateAbsent},
		{name: "completion by reference and argument", kind: capabilityCompletions, shape: metaDefault, target: "summarize_issue project_id", want: stateAsserted},
		{name: "subscription by kind", kind: capabilitySubscriptions, shape: dynamicDefault, target: "issue", want: stateAsserted},
		{name: "listen by kind", kind: capabilitySubscriptions, shape: dynamicDefault, target: "pipeline", want: stateAsserted},
		{name: "subscribable kind nothing watched", kind: capabilitySubscriptions, shape: dynamicDefault, target: "wiki", want: stateAbsent},
		{name: "elicitation flow that elicited", kind: capabilityElicitation, shape: metaDefault, target: "interactive.issue_create", want: stateAsserted},
		{name: "elicitation flow that answered without eliciting", kind: capabilityElicitation, shape: dynamicDefault, target: "interactive.issue_create", want: stateErrorPathOnly},
		{name: "read-only reads", kind: capabilityModes, shape: metaReadOnly, target: facetReads, want: stateAsserted},
		{name: "read-only withheld", kind: capabilityModes, shape: metaReadOnly, target: facetWithheld, want: stateAsserted},
		{name: "safe reads", kind: capabilityModes, shape: individualSafe, target: facetReads, want: stateAsserted},
		{name: "safe previews", kind: capabilityModes, shape: individualSafe, target: facetPreviews, want: stateAsserted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := capabilityState(c, tc.kind, tc.shape, tc.target); got != tc.want {
				t.Errorf("%s %s/%s %q = %s, want %s", tc.kind, tc.shape.surface, tc.shape.mode, tc.target, got, tc.want)
			}
		})
	}
	if !c.delivered[cellKey{shape: dynamicDefault, action: "issue"}] {
		t.Error("the resource-updated notification was not recorded as delivered for the issue kind")
	}
	if got, want := len(c.capabilities[capabilitySubscriptions]), len(subscribableKinds(nil))*len(c.shapes); got != want {
		t.Errorf("subscription cells = %d, want one per kind on each of the %d full-capability shapes (%d)", got, len(c.shapes), want)
	}
}

// TestClassify_Modes_AbsentWithoutEvidence verifies that a protective shape
// with no call on it reports both facets absent, and one whose only call
// failed reports them failed.
func TestClassify_Modes_AbsentWithoutEvidence(t *testing.T) {
	rt := fixtureRuntime()
	rt.calls = nil
	rt.calls = append(rt.calls, fixtureCall(callSpec{test: "TestSafeFailing", action: "issue.list", dispatched: "issue.list", status: e2ecalls.StatusFailed, shape: individualSafe}))
	c := classify(rt, fixtureCatalog())

	cases := []struct {
		name  string
		shape shapeKey
		facet string
		want  state
	}{
		{name: "read-only reads absent", shape: metaReadOnly, facet: facetReads, want: stateAbsent},
		{name: "read-only withheld absent", shape: metaReadOnly, facet: facetWithheld, want: stateAbsent},
		{name: "safe reads failed", shape: individualSafe, facet: facetReads, want: stateFailed},
		{name: "safe previews failed", shape: individualSafe, facet: facetPreviews, want: stateFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := capabilityState(c, capabilityModes, tc.shape, tc.facet); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.name, got, tc.want)
			}
		})
	}
}

// TestClassify_ApplyStatic_UnassertedAndSkipped verifies what the source
// adds: an asserted cell whose result every site discards becomes
// unasserted, and an absent cell named by a test that skipped becomes
// skipped with the test's reason on every shape.
func TestClassify_ApplyStatic_UnassertedAndSkipped(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())
	c.applyStatic(&staticResult{
		unassertedIDs: map[string]bool{"issue.list": true},
		testIDs:       map[string]map[string]bool{"TestSkipped": {"project.list": true}},
	})

	if got := stateOf(c, dynamicDefault, "issue.list"); got != stateUnasserted {
		t.Errorf("issue.list on dynamic = %s, want unasserted", got)
	}
	if got := stateOf(c, individualDefault, "issue.list"); got != stateSweepOnly {
		t.Errorf("issue.list on individual = %s, want the sweep credit left alone", got)
	}
	for _, shape := range []shapeKey{dynamicDefault, metaDefault, individualDefault} {
		t.Run(shape.surface, func(t *testing.T) {
			found := c.cells[cellKey{shape: shape, action: "project.list"}]
			if found.state != stateSkipped || found.reason != "no runner" {
				t.Errorf("project.list on %s/%s = %s (%q), want skipped (no runner)", shape.surface, shape.mode, found.state, found.reason)
			}
		})
	}
}

// TestClassify_Edges_RecordOddities verifies the shapes a shard can hold
// that the fixture above does not: a call on a shape no session line named,
// a non-tool call with no target, a subscription to a URI the server would
// not accept, a tool call naming no tool, and a skipped subtest whose skip
// line names its parent.
func TestClassify_Edges_RecordOddities(t *testing.T) {
	rt := fixtureRuntime()
	unlisted := shapeKey{surface: config.ToolSurfaceDynamic, mode: modeSafe}
	rt.calls = append(rt.calls,
		fixtureCall(callSpec{test: "TestUnlisted", action: "issue.list", dispatched: "issue.list", shape: unlisted}),
		fixtureCall(callSpec{test: "TestUnlisted", method: methodReadResource, target: "gitlab://project/1", shape: unlisted}),
		fixtureCall(callSpec{test: "TestNoTarget", method: methodGetPrompt, shape: metaDefault}),
		fixtureCall(callSpec{test: "TestNotSubscribable", method: methodSubscribe, target: "gitlab://project/1/branches", shape: dynamicDefault}),
		fixtureCall(callSpec{test: "TestSkipped/sub", action: "project.list", dispatched: "project.list", status: e2ecalls.StatusSkipped, shape: dynamicDefault}),
		fixtureCall(callSpec{test: "TestSkippedNoLine", action: "merge_train.list", dispatched: "merge_train.list", status: e2ecalls.StatusSkipped, shape: dynamicDefault}),
	)
	noTool := fixtureCall(callSpec{test: "TestNoTool", shape: metaDefault, outcome: e2ecalls.OutcomeProtocolError, expectation: e2ecalls.ExpectationAny})
	noTool.Tool = ""
	rt.calls = append(rt.calls, noTool)
	c := classify(rt, fixtureCatalog())

	if got := stateOf(c, unlisted, "issue.list"); got != stateAsserted {
		t.Errorf("a call on a shape without a session line = %s, want asserted", got)
	}
	if got := capabilityState(c, capabilityResources, unlisted, "gitlab://project/1"); got != stateAsserted {
		t.Errorf("a read on a shape without a session line = %s, want asserted under its own URI", got)
	}
	if _, exists := c.capabilities[capabilityPrompts][cellKey{shape: metaDefault, action: ""}]; exists {
		t.Error("a prompt call with no target created a cell")
	}
	if got := capabilityState(c, capabilitySubscriptions, dynamicDefault, "gitlab://project/1/branches"); got != stateAsserted {
		t.Errorf("a subscription the server would refuse = %s under its URI, want asserted as recorded", got)
	}
	skipped := c.cells[cellKey{shape: dynamicDefault, action: "project.list"}]
	if skipped.state != stateSkipped || skipped.reason != "no runner" {
		t.Errorf("a skipped subtest = %s (%q), want skipped with its parent's reason", skipped.state, skipped.reason)
	}
	if noLine := c.cells[cellKey{shape: dynamicDefault, action: "merge_train.list"}]; noLine.state != stateSkipped || noLine.reason != "" {
		t.Errorf("a skipped test without a skip line = %s (%q), want skipped with no reason", noLine.state, noLine.reason)
	}
	if len(c.unresolved) != 1 {
		t.Errorf("unresolved = %+v, want only the raw call: a call naming no tool resolves nothing", c.unresolved)
	}
}

// TestCreditOf_Outcomes_Credited verifies the credit table call by call, which is the
// rule every state above follows.
func TestCreditOf_Outcomes_Credited(t *testing.T) {
	cases := []struct {
		name string
		call *e2ecalls.Call
		want credit
	}{
		{name: "asserted", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeTest, Action: "a.b", Dispatched: "a.b", Outcome: e2ecalls.OutcomeOK, TestStatus: e2ecalls.StatusPassed}, want: creditAsserted},
		{name: "unobserved", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeTest, Action: "a.b", Outcome: e2ecalls.OutcomeOK, TestStatus: e2ecalls.StatusPassed}, want: creditUnobserved},
		{name: "sweep", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeSweep, Action: "a.b", Dispatched: "a.b", Outcome: e2ecalls.OutcomeOK, TestStatus: e2ecalls.StatusPassed}, want: creditSweep},
		{name: "sweep preview is preview", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeSweep, Action: "a.b", Outcome: e2ecalls.OutcomePreview, TestStatus: e2ecalls.StatusPassed}, want: creditPreview},
		{name: "error path", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeTest, Action: "a.b", Outcome: e2ecalls.OutcomeToolError, TestStatus: e2ecalls.StatusPassed}, want: creditErrorPath},
		{name: "protocol error path", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeTest, Action: "a.b", Outcome: e2ecalls.OutcomeProtocolError, TestStatus: e2ecalls.StatusPassed}, want: creditErrorPath},
		{name: "refused", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeTest, Action: "a.b", Outcome: e2ecalls.RefusedOutcome("x"), TestStatus: e2ecalls.StatusPassed}, want: creditRefused},
		{name: "cleanup", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeCleanup, Action: "a.b", Outcome: e2ecalls.OutcomeOK, TestStatus: e2ecalls.StatusPassed}, want: creditCleanup},
		{name: "cleanup that failed credits nothing", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeCleanup, Action: "a.b", Outcome: e2ecalls.OutcomeToolError, TestStatus: e2ecalls.StatusPassed}, want: creditNone},
		{name: "transport error credits nothing", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeTest, Action: "a.b", Outcome: e2ecalls.OutcomeTransportError, TestStatus: e2ecalls.StatusPassed}, want: creditNone},
		{name: "failed test", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeTest, Action: "a.b", Outcome: e2ecalls.OutcomeOK, TestStatus: e2ecalls.StatusFailed}, want: creditFailed},
		{name: "no status is not passed", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeTest, Action: "a.b", Outcome: e2ecalls.OutcomeOK}, want: creditFailed},
		{name: "skipped test", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeTest, Action: "a.b", Outcome: e2ecalls.OutcomeOK, TestStatus: e2ecalls.StatusSkipped}, want: creditSkipped},
		{name: "raw credits nothing", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeRaw, Dispatched: "a.b", Outcome: e2ecalls.OutcomeOK, TestStatus: e2ecalls.StatusPassed}, want: creditNone},
		{name: "no action credits nothing", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeTest, Outcome: e2ecalls.OutcomeOK, TestStatus: e2ecalls.StatusPassed}, want: creditNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := creditOf(tc.call); got != tc.want {
				t.Errorf("creditOf() = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestCreditOf_Target_IsWhatRan verifies that the credited action is the
// dispatched one when a span said so, and the requested one otherwise.
func TestCreditOf_Target_IsWhatRan(t *testing.T) {
	cases := []struct {
		name string
		call *e2ecalls.Call
		want string
	}{
		{name: "dispatched wins", call: &e2ecalls.Call{Action: "a.b", Dispatched: "a.c", TestStatus: e2ecalls.StatusPassed}, want: "a.c"},
		{name: "requested when no span", call: &e2ecalls.Call{Action: "a.b", TestStatus: e2ecalls.StatusPassed}, want: "a.b"},
		{name: "nothing for a raw call", call: &e2ecalls.Call{Purpose: e2ecalls.PurposeRaw, Dispatched: "a.c"}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, got := creditOf(tc.call); got != tc.want {
				t.Errorf("creditOf() target = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCredit_String_NamesEveryCredit verifies the spelling the baseline
// comparison records, including the one no state maps to.
func TestCredit_String_NamesEveryCredit(t *testing.T) {
	cases := []struct {
		credit credit
		want   string
	}{
		{credit: creditNone, want: "none"},
		{credit: creditSkipped, want: "skipped"},
		{credit: creditFailed, want: "failed"},
		{credit: creditCleanup, want: "cleanup-only"},
		{credit: creditPreview, want: "preview-only"},
		{credit: creditRefused, want: "refused-only"},
		{credit: creditErrorPath, want: "error-path-only"},
		{credit: creditSweep, want: "sweep-only"},
		{credit: creditUnobserved, want: "unobserved"},
		{credit: creditAsserted, want: "asserted"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.credit.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMatchTemplate_SeveralMatch_MostSpecificWins verifies the template matcher: one
// segment per simple variable, the rest of the path for a reserved
// expansion, and the template with the most literal segments when several
// match.
func TestMatchTemplate_SeveralMatch_MostSpecificWins(t *testing.T) {
	templates := []string{
		"gitlab://project/{project_id}",
		"gitlab://project/{project_id}/mr/{merge_request_iid}",
		"gitlab://project/{project_id}/mr/{merge_request_iid}/notes",
		"gitlab://project/{project_id}/file/{ref}/{+path}",
		"gitlab://group/{group_id}/{kind}/{value}",
	}
	cases := []struct {
		name    string
		uri     string
		want    string
		matched bool
	}{
		{name: "one segment per variable", uri: "gitlab://project/42", want: "gitlab://project/{project_id}", matched: true},
		{name: "the longer template wins", uri: "gitlab://project/42/mr/7/notes", want: "gitlab://project/{project_id}/mr/{merge_request_iid}/notes", matched: true},
		{name: "a shorter path takes the shorter template", uri: "gitlab://project/42/mr/7", want: "gitlab://project/{project_id}/mr/{merge_request_iid}", matched: true},
		{name: "reserved expansion takes the rest", uri: "gitlab://project/42/file/main/src/a/b.go", want: "gitlab://project/{project_id}/file/{ref}/{+path}", matched: true},
		{name: "reserved expansion needs one segment", uri: "gitlab://project/42/file/main/", matched: false},
		{name: "an empty simple segment does not match", uri: "gitlab://project//mr/7", matched: false},
		{name: "two variables in a row", uri: "gitlab://group/3/label/bug", want: "gitlab://group/{group_id}/{kind}/{value}", matched: true},
		{name: "a literal that differs", uri: "gitlab://project/42/issue/7", matched: false},
		{name: "extra segments do not match", uri: "gitlab://project/42/mr/7/notes/extra", matched: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, matched := matchTemplate(templates, tc.uri)
			if matched != tc.matched || got != tc.want {
				t.Errorf("matchTemplate(%q) = (%q, %t), want (%q, %t)", tc.uri, got, matched, tc.want, tc.matched)
			}
		})
	}
}

// TestSubscribableKinds_NoListing_EveryServerKindNamed verifies that the fallback
// universe names every kind the server accepts, once each, and that a
// session's own list is taken as given.
func TestSubscribableKinds_NoListing_EveryServerKindNamed(t *testing.T) {
	kinds := subscribableKinds(nil)
	if len(kinds) != 26 {
		t.Errorf("subscribableKinds(nil) = %d kinds %q, want the server's 26", len(kinds), kinds)
	}
	seen := map[string]bool{}
	for _, kind := range kinds {
		if seen[kind] || kind == "unknown" {
			t.Errorf("kind %q is repeated or unknown", kind)
		}
		seen[kind] = true
	}
	if got := subscribableKinds(map[string]bool{"issue": true}); !reflect.DeepEqual(got, []string{"issue"}) {
		t.Errorf("subscribableKinds(listed) = %q, want the listed kinds", got)
	}
}

// TestSampleURI_Variables_Expanded verifies the expansion the kind lookup
// rests on: numeric for identifiers, a word elsewhere.
func TestSampleURI_Variables_Expanded(t *testing.T) {
	cases := []struct {
		template string
		want     string
	}{
		{template: "gitlab://project/{project_id}/mr/{merge_request_iid}", want: "gitlab://project/1/mr/1"},
		{template: "gitlab://project/{project_id}/wiki/{slug}", want: "gitlab://project/1/wiki/sample"},
		{template: "gitlab://project/{project_id}/file/{ref}/{+path}", want: "gitlab://project/1/file/sample/sample"},
		{template: "gitlab://groups", want: "gitlab://groups"},
		{template: "gitlab://broken/{", want: "gitlab://broken/{"},
	}
	for _, tc := range cases {
		t.Run(tc.template, func(t *testing.T) {
			if got := sampleURI(tc.template); got != tc.want {
				t.Errorf("sampleURI(%q) = %q, want %q", tc.template, got, tc.want)
			}
		})
	}
}
