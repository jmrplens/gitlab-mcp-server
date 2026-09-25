package main

import (
	"reflect"
	"slices"
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

// The two resources the active tool surface decides, spelled here rather than
// read from the resources package, so a test that expects them to be filed
// apart does not take its expectation from the code under test.
const (
	manifestIndex  = "gitlab://tools"
	manifestDetail = "gitlab://tools/{id}"
)

// fixtureSession builds one session line of the full capability surface, which
// lists the tool-manifest pair beside the rest of the catalog as the server
// does.
func fixtureSession(key shapeKey, observed bool) *e2ecalls.Session {
	return &e2ecalls.Session{
		Label: key.surface + "/" + key.mode, Surface: key.surface, Mode: key.mode,
		Capabilities: config.CapabilitySurfaceFull, Transport: "stdio", Tools: servedTools,
		Resources: []string{"gitlab://groups", manifestIndex},
		ResourceTemplates: []string{
			"gitlab://project/{project_id}", "gitlab://project/{project_id}/issue/{issue_iid}",
			"gitlab://project/{project_id}/file/{ref}/{+path}", manifestDetail,
		},
		Prompts:          []string{"summarize_issue"},
		DispatchObserved: observed,
	}
}

// minimalSession builds a session line of the minimal capability surface as
// the harness records one: the tool-manifest pair and nothing else, since the
// server registers no prompt, no other resource and no subscription there.
func minimalSession(key shapeKey) *e2ecalls.Session {
	session := fixtureSession(key, true)
	session.Label += "/minimal"
	session.Capabilities = config.CapabilitySurfaceMinimal
	session.Resources = []string{manifestIndex}
	session.ResourceTemplates = []string{manifestDetail}
	session.Prompts = nil
	return session
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
		{test: "TestManifest", method: methodReadResource, target: "gitlab://tools/issue.list", shape: metaDefault},
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

// mustCell reads one action cell, failing the test when the classification
// holds no such cell: reported as a missing cell rather than left to a nil
// dereference, which would abort every subtest after it.
func mustCell(t *testing.T, c *classification, shape shapeKey, action string) *cell {
	t.Helper()
	found, exists := c.cells[cellKey{shape: shape, action: action}]
	if !exists {
		t.Fatalf("no cell for %s/%s %s", shape.surface, shape.mode, action)
	}
	return found
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
			found := mustCell(t, c, tc.shape, tc.action)
			if found.reason != tc.want {
				t.Errorf("reason = %q, want %q", found.reason, tc.want)
			}
		})
	}
}

// TestClassify_UnservableReason_DynamicWithoutAMetaTool verifies the one way
// the dynamic surface's borrowed reason does not apply: it is read off the
// meta surface's listing, so an action with no meta tool to look up has no
// reason to give and is reached through the execute tool like every other.
//
// Looking up the empty name instead would find it missing from every listing
// and report the action withheld on a surface that serves it.
func TestClassify_UnservableReason_DynamicWithoutAMetaTool(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())
	shape, known := c.shapes[dynamicDefault]
	if !known {
		t.Fatal("no shape for dynamic/default")
	}

	if got := c.unservableReason(shape, catalogAction{id: "orphan.action"}); got != "" {
		t.Errorf("unservableReason() = %q, want none for an action with no meta tool", got)
	}
}

// TestClassify_SkipReason_SettledByName verifies that a cell two skipped
// tests reached, each with a reason of its own, carries the same reason on
// every run: the first test in name order. The classification is run many
// times because the defect this pins was an iteration over a map, which
// settles the reason differently from one run to the next.
func TestClassify_SkipReason_SettledByName(t *testing.T) {
	rt := fixtureRuntime()
	rt.calls = nil
	rt.skips = []*e2ecalls.Skip{
		{Test: "TestSkipB", Reason: "no bitbucket fixture"},
		{Test: "TestSkipA", Reason: "no runner"},
	}
	rt.calls = append(rt.calls,
		fixtureCall(callSpec{test: "TestSkipB", action: "project.list", dispatched: "project.list", status: e2ecalls.StatusSkipped, shape: dynamicDefault}),
		fixtureCall(callSpec{test: "TestSkipA", action: "project.list", dispatched: "project.list", status: e2ecalls.StatusSkipped, shape: dynamicDefault}),
	)
	for range 64 {
		found := mustCell(t, classify(rt, fixtureCatalog()), dynamicDefault, "project.list")
		if found.state != stateSkipped || found.reason != "no runner" {
			t.Fatalf("project.list = %s (%q), want skipped with TestSkipA's reason on every run", found.state, found.reason)
		}
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

	found := mustCell(t, c, dynamicDefault, "issue.list")
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

// TestClassify_CapabilityCell_UnlistedUntilServed verifies which capability
// cells a call created stay unlisted: those whose target no session of their
// capability surface listed, and not those a session served, which the call
// reached first and fillCapabilityCells claimed afterwards.
func TestClassify_CapabilityCell_UnlistedUntilServed(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())
	full := config.CapabilitySurfaceFull
	cases := []struct {
		name         string
		kind, target string
		shape        shapeKey
		want         bool
	}{
		{name: "a completion no session offers", kind: capabilityCompletions, target: "summarize_issue project_id", want: true},
		{name: "a read of a URI nothing listed", kind: capabilityResources, target: "gitlab://nowhere", want: true},
		{name: "a read of a listed resource", kind: capabilityResources, target: "gitlab://groups"},
		{name: "a read through a listed template", kind: capabilityResources, target: "gitlab://project/{project_id}/issue/{issue_iid}"},
		{name: "a prompt a session listed", kind: capabilityPrompts, target: "summarize_issue"},
		{name: "a manifest read on its shape", kind: capabilityToolManifest, target: manifestDetail, shape: metaDefault},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found, exists := c.capabilities[tc.kind][capabilityKey(tc.kind, tc.shape, full, tc.target)]
			if !exists {
				t.Fatalf("no %s cell for %q", tc.kind, tc.target)
			}
			if found.unlisted != tc.want {
				t.Errorf("unlisted = %t, want %t", found.unlisted, tc.want)
			}
		})
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
func capabilityState(c *classification, kind string, key cellKey) state {
	found, exists := c.capabilities[kind][key]
	if !exists {
		return "missing"
	}
	return found.state
}

// onCapabilities is the key of a cell counted once per capability surface,
// spelled out rather than built by the code under test.
func onCapabilities(capabilities, target string) cellKey {
	return cellKey{capabilities: capabilities, action: target}
}

// onShape is the key of a cell counted once per surface x mode.
func onShape(shape shapeKey, target string) cellKey {
	return cellKey{shape: shape, action: target}
}

// onShapeAndCapabilities is the key of a cell counted once per surface x mode x
// capability surface, which is the tool manifest's.
func onShapeAndCapabilities(shape shapeKey, capabilities, target string) cellKey {
	return cellKey{shape: shape, capabilities: capabilities, action: target}
}

// capabilityStates reads every cell of one kind as key and state, for a test
// that holds the whole set rather than the cells it thought to look up: a
// twin nobody named is exactly what the grain exists to rule out.
func capabilityStates(c *classification, kind string) map[cellKey]state {
	states := map[cellKey]state{}
	for key, found := range c.capabilities[kind] {
		states[key] = found.state
	}
	return states
}

// The two capability surfaces, shortened for the tables below.
const (
	full    = config.CapabilitySurfaceFull
	minimal = config.CapabilitySurfaceMinimal
)

// TestClassify_Capabilities_ClassifiedByTarget verifies the non-tool
// classification: a resource by the template it expands or its own static
// URI, the tool manifest by the same rule on the shape that read it, a prompt
// by name, a completion by reference and argument, a subscription by kind,
// and the elicitation flow by whether it elicited.
func TestClassify_Capabilities_ClassifiedByTarget(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())

	cases := []struct {
		name string
		kind string
		key  cellKey
		want state
	}{
		{name: "resource by template", kind: capabilityResources, key: onCapabilities(full, "gitlab://project/{project_id}/issue/{issue_iid}"), want: stateAsserted},
		{name: "resource by reserved-expansion template", kind: capabilityResources, key: onCapabilities(full, "gitlab://project/{project_id}/file/{ref}/{+path}"), want: stateAsserted},
		{name: "static resource by its URI", kind: capabilityResources, key: onCapabilities(full, "gitlab://groups"), want: stateAsserted},
		{name: "resource nothing read", kind: capabilityResources, key: onCapabilities(full, "gitlab://project/{project_id}"), want: stateAbsent},
		{name: "resource nothing serves keeps its URI", kind: capabilityResources, key: onCapabilities(full, "gitlab://nowhere"), want: stateErrorPathOnly},
		{name: "tool manifest detail by its template", kind: capabilityToolManifest, key: onShapeAndCapabilities(metaDefault, full, manifestDetail), want: stateAsserted},
		{name: "tool manifest on a shape nothing read it on", kind: capabilityToolManifest, key: onShapeAndCapabilities(dynamicDefault, full, manifestDetail), want: stateAbsent},
		{name: "prompt by name", kind: capabilityPrompts, key: onCapabilities(full, "summarize_issue"), want: stateAsserted},
		{name: "completion by reference and argument", kind: capabilityCompletions, key: onCapabilities(full, "summarize_issue project_id"), want: stateAsserted},
		{name: "subscription by kind", kind: capabilitySubscriptions, key: onCapabilities(full, "issue"), want: stateAsserted},
		{name: "listen by kind", kind: capabilitySubscriptions, key: onCapabilities(full, "pipeline"), want: stateAsserted},
		{name: "subscribable kind nothing watched", kind: capabilitySubscriptions, key: onCapabilities(full, "wiki"), want: stateAbsent},
		{name: "elicitation flow that elicited", kind: capabilityElicitation, key: onShape(metaDefault, "interactive.issue_create"), want: stateAsserted},
		{name: "elicitation flow that answered without eliciting", kind: capabilityElicitation, key: onShape(dynamicDefault, "interactive.issue_create"), want: stateErrorPathOnly},
		{name: "read-only reads", kind: capabilityModes, key: onShape(metaReadOnly, facetReads), want: stateAsserted},
		{name: "read-only withheld", kind: capabilityModes, key: onShape(metaReadOnly, facetWithheld), want: stateAsserted},
		{name: "safe reads", kind: capabilityModes, key: onShape(individualSafe, facetReads), want: stateAsserted},
		{name: "safe previews", kind: capabilityModes, key: onShape(individualSafe, facetPreviews), want: stateAsserted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := capabilityState(c, tc.kind, tc.key); got != tc.want {
				t.Errorf("%s %+v = %s, want %s", tc.kind, tc.key, got, tc.want)
			}
		})
	}
}

// TestClassify_Sessions_RepeatedLinesAndMinimalCapabilities verifies what
// folding the session lines settles: a shape opened twice is one shape with
// both listings unioned, each capability surface is folded apart from the
// other whatever shapes its sessions ran as, the minimal surface serves no
// prompt, and it gets no subscription cell, because the server accepts no
// subscription there and an absent cell would read as a kind nothing watched.
//
// The subscription absence is asserted by capability surface, which is what
// the keys carry now, and beside a count on the full surface: a loop over the
// subscription cells that found none at all would otherwise pass it. A prompt
// the minimal surface was asked for anyway is a cell of its own, which
// TestClassify_Prompts_OneCellPerCapabilitySurface holds.
func TestClassify_Sessions_RepeatedLinesAndMinimalCapabilities(t *testing.T) {
	leanShape := shapeKey{surface: config.ToolSurfaceDynamic, mode: modeReadOnly}
	second := fixtureSession(dynamicDefault, true)
	second.Prompts = []string{"triage_issue"}

	rt := fixtureRuntime()
	rt.calls = nil
	rt.sessions = append(rt.sessions, second, minimalSession(leanShape))
	c := classify(rt, fixtureCatalog())

	shape, known := c.shapes[dynamicDefault]
	if !known {
		t.Fatal("no shape for dynamic/default")
	}
	if shape.sessions != 2 {
		t.Errorf("sessions = %d, want the two lines folded into one shape", shape.sessions)
	}
	if !shape.prompts["summarize_issue"] || !shape.prompts["triage_issue"] {
		t.Errorf("prompts = %q, want both listings unioned", sortedKeys(shape.prompts))
	}
	fullSurface, minimalSurface := c.capabilitySurfaces[full], c.capabilitySurfaces[minimal]
	if fullSurface == nil || minimalSurface == nil {
		t.Fatalf("capability surfaces = %q, want full and minimal", sortedKeys(c.capabilitySurfaces))
	}
	if fullSurface.sessions != 6 || len(fullSurface.shapes) != 5 {
		t.Errorf("full = %d sessions on %d shapes, want the six full lines on the fixture's five shapes",
			fullSurface.sessions, len(fullSurface.shapes))
	}
	if minimalSurface.sessions != 1 || len(minimalSurface.prompts) != 0 {
		t.Errorf("minimal = %d sessions serving prompts %q, want the one lean line and no prompt",
			minimalSurface.sessions, sortedKeys(minimalSurface.prompts))
	}
	subscriptions := map[string]int{}
	for key := range c.capabilities[capabilitySubscriptions] {
		subscriptions[key.capabilities]++
	}
	if subscriptions[full] != len(subscribableKinds(nil)) || subscriptions[minimal] != 0 {
		t.Errorf("subscription cells per capability surface = %v, want every kind on full and none on minimal", subscriptions)
	}
}

// TestClassify_Sessions_IdleSessionIsNamedApartAndHoldsNoShapeUnobserved
// verifies the fold of a session that asked nothing.
//
// Every shape here has one observed session beside a second one. On the
// default dynamic shape the second is idle, the way a session started only to
// compare what it lists at a pinned tier is: it made no traced call, so its
// spans cannot have arrived, and the row must still read observed while the
// session is named among the idle ones. On the default meta shape the second
// made traced calls and saw no span of them, which is the finding the flag is
// for, and holds its row false. An idle line that also says it was observed,
// which the harness never writes, is neither idle nor unobserved. The label
// the idle session carries is one the observed session of its shape carries
// too, the collision two sessions of one process can have, and it is the line
// and not the label that decides.
func TestClassify_Sessions_IdleSessionIsNamedApartAndHoldsNoShapeUnobserved(t *testing.T) {
	idle := fixtureSession(dynamicDefault, false)
	idle.Idle = true
	unobserved := fixtureSession(metaDefault, false)
	unobserved.Label = "meta/default/unobserved"
	contradictory := fixtureSession(individualDefault, true)
	contradictory.Label = "individual/default/contradictory"
	contradictory.Idle = true

	rt := fixtureRuntime()
	rt.calls = nil
	rt.sessions = []*e2ecalls.Session{
		fixtureSession(dynamicDefault, true), idle,
		fixtureSession(metaDefault, true), unobserved,
		fixtureSession(individualDefault, true), contradictory,
	}
	c := classify(rt, fixtureCatalog())

	wantObserved := map[shapeKey]bool{dynamicDefault: true, metaDefault: false, individualDefault: true}
	for key, want := range wantObserved {
		t.Run(key.surface+"/"+key.mode, func(t *testing.T) {
			shape, known := c.shapes[key]
			if !known {
				t.Fatalf("no shape for %s/%s", key.surface, key.mode)
			}
			if shape.observed != want {
				t.Errorf("observed = %t, want %t", shape.observed, want)
			}
		})
	}
	if want := []string{"dynamic/default"}; !slices.Equal(c.diagnostics.IdleSessions, want) {
		t.Errorf("idle sessions = %q, want %q", c.diagnostics.IdleSessions, want)
	}
	if want := []string{"meta/default/unobserved"}; !slices.Equal(c.diagnostics.UnobservedSessions, want) {
		t.Errorf("unobserved sessions = %q, want %q", c.diagnostics.UnobservedSessions, want)
	}
}

// TestClassify_Sessions_IdleSessionsAreListedInOrder pins the order the idle
// list is published in, which is the label's and not the shards': two
// directories read in another order must publish the same report.
func TestClassify_Sessions_IdleSessionsAreListedInOrder(t *testing.T) {
	later := fixtureSession(metaDefault, false)
	later.Idle = true
	earlier := fixtureSession(dynamicDefault, false)
	earlier.Idle = true

	rt := fixtureRuntime()
	rt.calls = nil
	rt.sessions = []*e2ecalls.Session{later, earlier}
	c := classify(rt, fixtureCatalog())

	if want := []string{"dynamic/default", "meta/default"}; !slices.Equal(c.diagnostics.IdleSessions, want) {
		t.Errorf("idle sessions = %q, want them sorted as %q", c.diagnostics.IdleSessions, want)
	}
}

// TestClassify_Prompts_OneCellPerCapabilitySurface verifies that a prompt is
// one cell per capability surface, however many shapes served it: the
// fixture's five shapes all list summarize_issue, one of them rendered it, and
// the classification holds exactly one cell for it, asserted, with no absent
// twin on the four shapes that did not. A render on the minimal surface, which
// refuses prompts/get, is a cell of its own and leaves the full one alone.
func TestClassify_Prompts_OneCellPerCapabilitySurface(t *testing.T) {
	rt := fixtureRuntime()
	refused := fixtureCall(callSpec{
		test: "TestPromptsMinimal", method: methodGetPrompt, target: "summarize_issue",
		expectation: "protocol_error", outcome: e2ecalls.OutcomeProtocolError, shape: dynamicDefault,
	})
	refused.Capabilities = minimal
	rt.calls = append(rt.calls, refused)
	c := classify(rt, fixtureCatalog())

	want := map[cellKey]state{
		onCapabilities(full, "summarize_issue"):    stateAsserted,
		onCapabilities(minimal, "summarize_issue"): stateErrorPathOnly,
	}
	if got := capabilityStates(c, capabilityPrompts); !reflect.DeepEqual(got, want) {
		t.Errorf("prompt cells = %v, want %v", got, want)
	}
}

// TestClassify_Resources_OneCellPerCapabilitySurface verifies the same of the
// resources: one cell per static URI and per template the capability surface
// served, whatever shape read it, plus the one a read outside them made, and
// no cell for the tool-manifest pair, which is a kind of its own.
func TestClassify_Resources_OneCellPerCapabilitySurface(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())

	want := map[cellKey]state{
		onCapabilities(full, "gitlab://groups"):                                  stateAsserted,
		onCapabilities(full, "gitlab://project/{project_id}"):                    stateAbsent,
		onCapabilities(full, "gitlab://project/{project_id}/issue/{issue_iid}"):  stateAsserted,
		onCapabilities(full, "gitlab://project/{project_id}/file/{ref}/{+path}"): stateAsserted,
		onCapabilities(full, "gitlab://nowhere"):                                 stateErrorPathOnly,
	}
	if got := capabilityStates(c, capabilityResources); !reflect.DeepEqual(got, want) {
		t.Errorf("resource cells = %v, want %v", got, want)
	}
}

// TestClassify_ToolManifest_OneCellPerShapeAndCapabilitySurface verifies the
// one resource pair whose content the shape does change: gitlab://tools and
// gitlab://tools/{id} are filed as tool_manifest, one cell per surface x mode
// x capability surface, so a read on the minimal dynamic surface is not
// credited to the full one on the same shape, a read of the index is not
// credited to the detail, and a read in the default mode is not credited to
// the read-only one.
func TestClassify_ToolManifest_OneCellPerShapeAndCapabilitySurface(t *testing.T) {
	rt := fixtureRuntime()
	rt.calls = nil
	rt.sessions = append(rt.sessions, minimalSession(dynamicDefault))
	index := fixtureCall(callSpec{test: "TestManifestIndex", method: methodReadResource, target: manifestIndex, shape: dynamicDefault})
	detail := fixtureCall(callSpec{test: "TestManifestDetail", method: methodReadResource, target: "gitlab://tools/issue.list", shape: dynamicDefault})
	detail.Capabilities = minimal
	rt.calls = append(rt.calls, index, detail)
	c := classify(rt, fixtureCatalog())

	cells := capabilityStates(c, capabilityToolManifest)
	if len(cells) != 12 {
		t.Errorf("tool manifest cells = %d, want both items on each of the five full shapes and the one minimal shape (12): %v", len(cells), cells)
	}
	cases := []struct {
		name string
		key  cellKey
		want state
	}{
		{name: "the index read on full", key: onShapeAndCapabilities(dynamicDefault, full, manifestIndex), want: stateAsserted},
		{name: "the detail read on minimal", key: onShapeAndCapabilities(dynamicDefault, minimal, manifestDetail), want: stateAsserted},
		{name: "the detail on full, kept apart from minimal", key: onShapeAndCapabilities(dynamicDefault, full, manifestDetail), want: stateAbsent},
		{name: "the index on minimal, kept apart from full", key: onShapeAndCapabilities(dynamicDefault, minimal, manifestIndex), want: stateAbsent},
		{name: "the index in another mode", key: onShapeAndCapabilities(metaReadOnly, full, manifestIndex), want: stateAbsent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, held := cells[tc.key]; !held || got != tc.want {
				t.Errorf("%+v = %s (held %t), want %s", tc.key, got, held, tc.want)
			}
		})
	}
	for key := range c.capabilities[capabilityResources] {
		if key.action == manifestIndex || key.action == manifestDetail {
			t.Errorf("resource cell %+v: the tool-manifest pair is a kind of its own", key)
		}
	}
}

// TestClassify_Subscriptions_OneCellPerKindOnTheFullSurface verifies that a
// subscribable kind is one cell on the full capability surface, whatever the
// number of shapes: a sixth full shape and a minimal one leave the count at
// one per kind. The delivery a notification proved is filed under the same
// key, so the report can find it without a shape.
func TestClassify_Subscriptions_OneCellPerKindOnTheFullSurface(t *testing.T) {
	rt := fixtureRuntime()
	rt.sessions = append(rt.sessions,
		fixtureSession(shapeKey{surface: config.ToolSurfaceMeta, mode: modeSafe}, true),
		minimalSession(shapeKey{surface: config.ToolSurfaceDynamic, mode: modeReadOnly}),
	)
	c := classify(rt, fixtureCatalog())

	kinds := subscribableKinds(nil)
	cells := capabilityStates(c, capabilitySubscriptions)
	if len(cells) != len(kinds) {
		t.Errorf("subscription cells = %d, want one per kind (%d) whatever the %d shapes", len(cells), len(kinds), len(c.shapes))
	}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			want := stateAbsent
			if kind == "issue" || kind == "pipeline" {
				want = stateAsserted
			}
			if got := cells[onCapabilities(full, kind)]; got != want {
				t.Errorf("%s on full = %q, want %s", kind, got, want)
			}
		})
	}
	if want := map[cellKey]bool{onCapabilities(full, "issue"): true}; !reflect.DeepEqual(c.delivered, want) {
		t.Errorf("delivered = %v, want %v", c.delivered, want)
	}
}

// TestClassify_Completions_OneCellPerReference verifies that a completion
// reference two shapes offered is one cell, and that the reference of the
// tool manifest's own template is a completion like any other: the completion
// handler answers it the same way on every tool surface.
func TestClassify_Completions_OneCellPerReference(t *testing.T) {
	rt := fixtureRuntime()
	rt.sessions, rt.calls = nil, nil
	for _, shape := range []shapeKey{dynamicDefault, metaDefault} {
		session := fixtureSession(shape, true)
		session.Completions = []string{"summarize_issue project_id", manifestDetail + " id"}
		rt.sessions = append(rt.sessions, session)
	}
	rt.calls = append(rt.calls, fixtureCall(callSpec{
		test: "TestCompletions", method: methodComplete, target: "summarize_issue project_id", shape: metaDefault,
	}))
	c := classify(rt, fixtureCatalog())

	want := map[cellKey]state{
		onCapabilities(full, "summarize_issue project_id"): stateAsserted,
		onCapabilities(full, manifestDetail+" id"):         stateAbsent,
	}
	if got := capabilityStates(c, capabilityCompletions); !reflect.DeepEqual(got, want) {
		t.Errorf("completion cells = %v, want %v", got, want)
	}
}

// TestClassify_CallWithoutCapabilities_CountsAsTheDefaultSurface verifies how
// a line written before the harness recorded the capability surface is read:
// as the full surface the server falls back to, for the session and for the
// call alike, rather than as a third surface keyed by the empty string that
// nothing else would ever fill.
func TestClassify_CallWithoutCapabilities_CountsAsTheDefaultSurface(t *testing.T) {
	rt := fixtureRuntime()
	rt.calls = nil
	for _, session := range rt.sessions {
		session.Capabilities = ""
	}
	call := fixtureCall(callSpec{test: "TestPrompts", method: methodGetPrompt, target: "summarize_issue", shape: metaDefault})
	call.Capabilities = ""
	rt.calls = append(rt.calls, call)
	c := classify(rt, fixtureCatalog())

	if got := sortedKeys(c.capabilitySurfaces); !slices.Equal(got, []string{full}) {
		t.Errorf("capability surfaces = %q, want the lines read as full", got)
	}
	want := map[cellKey]state{onCapabilities(full, "summarize_issue"): stateAsserted}
	if got := capabilityStates(c, capabilityPrompts); !reflect.DeepEqual(got, want) {
		t.Errorf("prompt cells = %v, want %v", got, want)
	}
	if got := len(c.capabilities[capabilitySubscriptions]); got != len(subscribableKinds(nil)) {
		t.Errorf("subscription cells = %d, want the kinds of the full surface the lines are read as", got)
	}
}

// TestCapabilityGrains_EveryKind_CountedAtTheGrainItVariesAlong pins the
// model: every capability kind the fold writes is in the table, at the grain
// the server makes it vary along, and each grain keys its cells with exactly
// the coordinates it names. A kind missing from the table would be keyed at
// the zero grain without a word, and the page would not list it, so the kinds
// a classification of the fixture actually wrote are held against the table
// in both directions rather than against the list below, which a new kind
// would be missing from as well.
func TestCapabilityGrains_EveryKind_CountedAtTheGrainItVariesAlong(t *testing.T) {
	shape := shapeKey{surface: config.ToolSurfaceMeta, mode: modeReadOnly}
	cases := []struct {
		kind  string
		grain string
		key   cellKey
	}{
		{kind: capabilityResources, grain: "capability surface", key: onCapabilities(minimal, "x")},
		{kind: capabilityPrompts, grain: "capability surface", key: onCapabilities(minimal, "x")},
		{kind: capabilityCompletions, grain: "capability surface", key: onCapabilities(minimal, "x")},
		{kind: capabilitySubscriptions, grain: "capability surface", key: onCapabilities(minimal, "x")},
		{kind: capabilityToolManifest, grain: "surface x mode x capability surface", key: onShapeAndCapabilities(shape, minimal, "x")},
		{kind: capabilityElicitation, grain: "surface x mode", key: onShape(shape, "x")},
		{kind: capabilityModes, grain: "surface x mode", key: onShape(shape, "x")},
	}
	if len(capabilityGrains) != len(cases) {
		t.Errorf("capabilityGrains holds %d kinds, want the %d the fold writes: %q", len(capabilityGrains), len(cases), sortedKeys(capabilityGrains))
	}
	written := classify(fixtureRuntime(), fixtureCatalog()).capabilities
	for _, kind := range sortedKeys(written) {
		if _, held := capabilityGrains[kind]; !held {
			t.Errorf("the fold wrote the kind %q, which capabilityGrains does not hold: it is keyed at the zero grain and left off the page", kind)
		}
	}
	for _, kind := range sortedKeys(capabilityGrains) {
		if _, wrote := written[kind]; !wrote {
			t.Errorf("capabilityGrains holds the kind %q, which a classification of the fixture never wrote", kind)
		}
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			grain, held := capabilityGrains[tc.kind]
			if !held || grain.String() != tc.grain {
				t.Errorf("grain = %q (held %t), want %q", grain, held, tc.grain)
			}
			if got := capabilityKey(tc.kind, shape, minimal, "x"); got != tc.key {
				t.Errorf("capabilityKey() = %+v, want %+v", got, tc.key)
			}
		})
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
			if got := capabilityState(c, capabilityModes, onShape(tc.shape, tc.facet)); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.name, got, tc.want)
			}
		})
	}
}

// TestClassify_Modes_EvidenceFollowsWhetherTheActionMutates verifies the one
// thing a protective mode's evidence turns on: a read going through is not
// evidence that a mutation was withheld, and a mutation answering for real is
// not evidence that reads go through.
//
// Both halves are about the run you would be reading the report of. A read
// that errored in read-only mode must not be filed as a mutation the mode
// withheld, and a mutation that ran in read-only mode -- which is the failure
// the mode exists to prevent -- must not be filed as a read going through, or
// the report would say the protection works because it did not.
func TestClassify_Modes_EvidenceFollowsWhetherTheActionMutates(t *testing.T) {
	rt := fixtureRuntime()
	rt.calls = nil
	for _, spec := range []callSpec{
		// Reads, one per credit the reads facet accepts.
		{test: "TestRoAsserted", action: "issue.list", dispatched: "issue.list", shape: metaReadOnly},
		{test: "TestRoUnobserved", action: "project.get", shape: metaReadOnly},
		{test: "TestRoSweep", purpose: e2ecalls.PurposeSweep, action: "project.list", dispatched: "project.list", shape: metaReadOnly},
		// Mutations withheld, one refused and one answered as a tool error.
		{test: "TestRoRefused", expectation: "unknown_action", action: "issue.create", outcome: e2ecalls.RefusedOutcome("unknown_action"), shape: metaReadOnly},
		{test: "TestRoErrorPath", expectation: "tool_error", action: "issue.delete", dispatched: "issue.delete", outcome: e2ecalls.OutcomeToolError, shape: metaReadOnly},
		// A read that errored: withheld is about mutations, so this is neither.
		{test: "TestRoReadErrored", expectation: "tool_error", action: "environment.get", dispatched: "environment.get", outcome: e2ecalls.OutcomeToolError, shape: metaReadOnly},
		// A mutation the mode let through: not a read going through.
		{test: "TestRoMutationRan", action: "issue.create", dispatched: "issue.create", shape: metaReadOnly},
		// Safe mode: a mutation previewed is evidence, a read previewed is not.
		{test: "TestSafePreviewed", expectation: "safe_mode", action: "issue.create", dispatched: "issue.create", outcome: e2ecalls.OutcomePreview, shape: individualSafe},
		{test: "TestSafeReadPreviewed", expectation: "safe_mode", action: "issue.list", dispatched: "issue.list", outcome: e2ecalls.OutcomePreview, shape: individualSafe},
	} {
		rt.calls = append(rt.calls, fixtureCall(spec))
	}
	c := classify(rt, fixtureCatalog())

	cases := []struct {
		name     string
		shape    shapeKey
		reads    []string
		withheld []string
		previews []string
	}{
		{
			name:     "read-only",
			shape:    metaReadOnly,
			reads:    []string{"TestRoAsserted", "TestRoSweep", "TestRoUnobserved"},
			withheld: []string{"TestRoErrorPath", "TestRoRefused"},
		},
		{name: "safe", shape: individualSafe, previews: []string{"TestSafePreviewed"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := c.modes[tc.shape]
			if evidence == nil {
				t.Fatalf("no mode evidence for %s/%s", tc.shape.surface, tc.shape.mode)
			}
			for _, facet := range []struct {
				name string
				got  map[string]bool
				want []string
			}{
				{name: facetReads, got: evidence.reads, want: tc.reads},
				{name: facetWithheld, got: evidence.withheld, want: tc.withheld},
				{name: facetPreviews, got: evidence.previews, want: tc.previews},
			} {
				if got := sortedKeys(facet.got); !slices.Equal(got, facet.want) {
					t.Errorf("%s = %q, want %q", facet.name, got, facet.want)
				}
			}
		})
	}
}

// TestClassify_Diagnostics_CallWithoutAStatusIsCounted verifies the counter
// the results join exists to clear: a call line whose test carried no verdict
// is counted, and credited with the failures rather than on trust.
func TestClassify_Diagnostics_CallWithoutAStatusIsCounted(t *testing.T) {
	rt := fixtureRuntime()
	rt.calls = nil
	unjudged := fixtureCall(callSpec{test: "TestUnjudged", action: "issue.list", dispatched: "issue.list", shape: dynamicDefault})
	unjudged.TestStatus = ""
	rt.calls = append(rt.calls, unjudged)
	c := classify(rt, fixtureCatalog())

	if c.diagnostics.WithoutStatus != 1 {
		t.Errorf("WithoutStatus = %d, want 1: the call carries no verdict", c.diagnostics.WithoutStatus)
	}
	if got := stateOf(c, dynamicDefault, "issue.list"); got != stateFailed {
		t.Errorf("state = %s, want failed: a call whose test's verdict is unknown is not proven coverage", got)
	}
}

// TestClassify_ApplyStatic_UnassertedAndSkipped verifies what the source
// adds: an asserted cell whose result every site discards becomes
// unasserted, and an absent cell named by a test that skipped becomes
// skipped with the test's reason on every shape. A skipped test naming an
// action the catalog does not hold, which the static gate reports on its own
// side, has no cell to land on and makes none.
func TestClassify_ApplyStatic_UnassertedAndSkipped(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())
	c.applyStatic(&staticResult{
		unassertedIDs: map[string]bool{"issue.list": true},
		testIDs:       map[string]map[string]bool{"TestSkipped": {"project.list": true, "ghost.action": true}},
	})

	if got := stateOf(c, dynamicDefault, "issue.list"); got != stateUnasserted {
		t.Errorf("issue.list on dynamic = %s, want unasserted", got)
	}
	if got := stateOf(c, individualDefault, "issue.list"); got != stateSweepOnly {
		t.Errorf("issue.list on individual = %s, want the sweep credit left alone", got)
	}
	if got := stateOf(c, dynamicDefault, "ghost.action"); got != "missing" {
		t.Errorf("ghost.action on dynamic = %s, want no cell for an action the catalog lacks", got)
	}
	for _, shape := range []shapeKey{dynamicDefault, metaDefault, individualDefault} {
		t.Run(shape.surface, func(t *testing.T) {
			found := mustCell(t, c, shape, "project.list")
			if found.state != stateSkipped || found.reason != "no runner" {
				t.Errorf("project.list on %s/%s = %s (%q), want skipped (no runner)", shape.surface, shape.mode, found.state, found.reason)
			}
		})
	}
}

// TestClassify_ElicitationCells_OwnTheirCredit verifies that a flow cell is
// derived from its interactive action's cell and does not share its maps: a
// credit added to the action cell afterwards changes nothing on the flow,
// and applyStatic's skip attribution reaches the flow by derivation, with
// the reason, rather than by aliasing a map it never accounted for.
func TestClassify_ElicitationCells_OwnTheirCredit(t *testing.T) {
	rt := fixtureRuntime()
	rt.skips = append(rt.skips, &e2ecalls.Skip{Test: "TestInteractiveSkipped", Reason: "no elicitation client"})
	c := classify(rt, fixtureCatalog())
	key := cellKey{shape: metaDefault, action: "interactive.issue_create"}
	action, flow := c.cells[key], c.capabilities[capabilityElicitation][key]
	if action == nil || flow == nil {
		t.Fatalf("cells = action %v, flow %v; want both for the elicited flow", action, flow)
	}
	if flow.state != stateAsserted || flow.counts[creditAsserted] != 1 {
		t.Fatalf("flow = %s with %d asserted calls, want asserted with 1", flow.state, flow.counts[creditAsserted])
	}

	action.add(creditCleanup, "TestLater")
	if flow.counts[creditCleanup] != 0 || flow.tests[creditCleanup] != nil {
		t.Errorf("flow counts = %v after the action cell changed, want its own maps untouched", flow.counts)
	}

	// The individual shape has no call to the flow, so its action cell is
	// absent until the skip lands on it, and the flow must follow.
	c.applyStatic(&staticResult{
		unassertedIDs: map[string]bool{},
		testIDs:       map[string]map[string]bool{"TestInteractiveSkipped": {"interactive.issue_create": true}},
	})
	skippedKey := cellKey{shape: individualDefault, action: "interactive.issue_create"}
	skippedAction, skippedFlow := c.cells[skippedKey], c.capabilities[capabilityElicitation][skippedKey]
	if skippedAction == nil || skippedFlow == nil {
		t.Fatalf("cells = action %v, flow %v; want both on individual", skippedAction, skippedFlow)
	}
	if skippedAction.state != stateSkipped || skippedFlow.state != stateSkipped || skippedFlow.reason != "no elicitation client" {
		t.Errorf("after applyStatic: action %s, flow %s (%q); want both skipped with the reason", skippedAction.state, skippedFlow.state, skippedFlow.reason)
	}
	if skippedFlow.counts[creditSkipped] != 1 || !skippedFlow.tests[creditSkipped]["TestInteractiveSkipped"] {
		t.Errorf("flow credit = %v, want the one skipped call derived from the action cell", skippedFlow.counts)
	}
}

// TestClassify_Edges_RecordOddities verifies the shapes a shard can hold
// that the fixture above does not: a call on a shape no session line named,
// a read on a capability surface no session line named, a manifest detail
// read on such a surface, which is still the manifest, a non-tool call with
// no target, a subscription to a URI the server would not accept, a tool call
// naming no tool, a skipped subtest whose skip line names its parent, and a
// call whose method the classification has no cell for, which is counted as a
// call and credited to nothing.
func TestClassify_Edges_RecordOddities(t *testing.T) {
	rt := fixtureRuntime()
	unlisted := shapeKey{surface: config.ToolSurfaceDynamic, mode: modeSafe}
	onUnlistedSurface := fixtureCall(callSpec{test: "TestUnlistedSurface", method: methodReadResource, target: "gitlab://project/2", shape: dynamicDefault})
	onUnlistedSurface.Capabilities = minimal
	detailOnUnlistedSurface := fixtureCall(callSpec{test: "TestUnlistedSurfaceManifest", method: methodReadResource, target: "gitlab://tools/issue.list", shape: dynamicDefault})
	detailOnUnlistedSurface.Capabilities = minimal
	rt.calls = append(rt.calls,
		fixtureCall(callSpec{test: "TestUnlisted", action: "issue.list", dispatched: "issue.list", shape: unlisted}),
		fixtureCall(callSpec{test: "TestUnlisted", method: methodReadResource, target: "gitlab://project/1", shape: unlisted}),
		onUnlistedSurface,
		detailOnUnlistedSurface,
		fixtureCall(callSpec{test: "TestNoTarget", method: methodGetPrompt, shape: metaDefault}),
		fixtureCall(callSpec{test: "TestNotSubscribable", method: methodSubscribe, target: "gitlab://project/1/branches", shape: dynamicDefault}),
		fixtureCall(callSpec{test: "TestSkipped/sub", action: "project.list", dispatched: "project.list", status: e2ecalls.StatusSkipped, shape: dynamicDefault}),
		fixtureCall(callSpec{test: "TestSkippedNoLine", action: "merge_train.list", dispatched: "merge_train.list", status: e2ecalls.StatusSkipped, shape: dynamicDefault}),
		fixtureCall(callSpec{test: "TestListed", method: "tools/list", target: "gitlab_issue", shape: dynamicDefault}),
	)
	noTool := fixtureCall(callSpec{test: "TestNoTool", shape: metaDefault, outcome: e2ecalls.OutcomeProtocolError, expectation: e2ecalls.ExpectationAny})
	noTool.Tool = ""
	// A prompt the server answered and a notification it delivered, both
	// inside a test that went on to fail: neither is evidence of anything,
	// since what the test asserted about them never held.
	rt.calls = append(rt.calls, noTool,
		fixtureCall(callSpec{test: "TestFailedElicit", method: methodElicit, target: "Confirm?", expectation: e2ecalls.ExpectationAny, status: e2ecalls.StatusFailed, shape: metaDefault}),
		fixtureCall(callSpec{test: "TestFailedDelivery", method: methodResourceUpdated, target: "gitlab://project/1/pipeline/9", expectation: e2ecalls.ExpectationAny, status: e2ecalls.StatusFailed, shape: dynamicDefault}),
		// A dispatch with no action beside it: every call the harness makes
		// names the action it asked for, so a line without one asked for
		// nothing that could disagree with what ran.
		fixtureCall(callSpec{test: "TestOnlyDispatched", dispatched: "server.health_check", shape: dynamicDefault}),
	)
	c := classify(rt, fixtureCatalog())

	if c.elicited["TestFailedElicit"] {
		t.Error("a prompt answered inside a failing test was recorded as an elicitation")
	}
	if c.delivered[onCapabilities(full, "pipeline")] {
		t.Error("a notification delivered to a failing test was recorded as delivery")
	}
	if got := stateOf(c, dynamicDefault, "server.health_check"); got != stateAsserted {
		t.Errorf("a call carrying only a dispatched action = %s, want asserted against what ran", got)
	}
	if len(c.mismatches) != 1 {
		t.Errorf("mismatches = %+v, want only the fixture's rewritten route", c.mismatches)
	}

	if got := stateOf(c, unlisted, "issue.list"); got != stateAsserted {
		t.Errorf("a call on a shape without a session line = %s, want asserted", got)
	}
	if got := capabilityState(c, capabilityResources, onCapabilities(full, "gitlab://project/{project_id}")); got != stateAsserted {
		t.Errorf("a read on a shape without a session line = %s, want asserted under the template its capability surface served", got)
	}
	if got := capabilityState(c, capabilityResources, onCapabilities(minimal, "gitlab://project/2")); got != stateAsserted {
		t.Errorf("a read on a capability surface without a session line = %s, want asserted under its own URI", got)
	}
	if got := capabilityState(c, capabilityToolManifest, onShapeAndCapabilities(dynamicDefault, minimal, "gitlab://tools/{id}")); got != stateAsserted {
		t.Errorf("a manifest detail read on a capability surface without a session line = %s, want asserted under the detail template on its shape", got)
	}
	wantManifest := []string{capabilityToolManifest + " " + config.ToolSurfaceDynamic + "/" + modeDefault + "/" + minimal + " gitlab://tools/{id}"}
	if credited := testsCredited(c, "TestUnlistedSurfaceManifest"); !slices.Equal(credited, wantManifest) {
		t.Errorf("a manifest detail read on a capability surface without a session line was credited to %q, want %q and no resources cell", credited, wantManifest)
	}
	for key := range c.capabilities[capabilityPrompts] {
		if key.action == "" {
			t.Errorf("a prompt call with no target created the cell %+v", key)
		}
	}
	if got := capabilityState(c, capabilitySubscriptions, onCapabilities(full, "gitlab://project/1/branches")); got != stateAsserted {
		t.Errorf("a subscription the server would refuse = %s under its URI, want asserted as recorded", got)
	}
	skipped := mustCell(t, c, dynamicDefault, "project.list")
	if skipped.state != stateSkipped || skipped.reason != "no runner" {
		t.Errorf("a skipped subtest = %s (%q), want skipped with its parent's reason", skipped.state, skipped.reason)
	}
	if noLine := mustCell(t, c, dynamicDefault, "merge_train.list"); noLine.state != stateSkipped || noLine.reason != "" {
		t.Errorf("a skipped test without a skip line = %s (%q), want skipped with no reason", noLine.state, noLine.reason)
	}
	if len(c.unresolved) != 1 {
		t.Errorf("unresolved = %+v, want only the raw call: a call naming no tool resolves nothing", c.unresolved)
	}
	if c.diagnostics.Calls != len(rt.calls) {
		t.Errorf("Calls = %d, want every line counted (%d), the tools/list one included", c.diagnostics.Calls, len(rt.calls))
	}
	if credited := testsCredited(c, "TestListed"); len(credited) != 0 {
		t.Errorf("a tools/list call was credited to %q, want nothing: the fold has no cell for the method", credited)
	}
}

// testsCredited names every cell, action or capability, that carries a
// credit from the test.
func testsCredited(c *classification, test string) []string {
	var credited []string
	note := func(kind string, key cellKey, found *cell) {
		for _, tests := range found.tests {
			if tests[test] {
				credited = append(credited, kind+" "+key.shape.surface+"/"+key.shape.mode+"/"+key.capabilities+" "+key.action)
			}
		}
	}
	for key, found := range c.cells {
		note("action", key, found)
	}
	for kind, cells := range c.capabilities {
		for key, found := range cells {
			note(kind, key, found)
		}
	}
	sort.Strings(credited)
	return credited
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
		// A passing model call, dispatched and answered, still earns nothing:
		// what it named was decided by a provider at run time, so crediting it
		// would make this suite's coverage a function of what a language model
		// felt like trying and would move with every run.
		{name: "model credits nothing", call: &e2ecalls.Call{Method: methodCallTool, Purpose: e2ecalls.PurposeModel, Dispatched: "a.b", Outcome: e2ecalls.OutcomeOK, TestStatus: e2ecalls.StatusPassed}, want: creditNone},
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
		{name: "nothing for a model call", call: &e2ecalls.Call{Purpose: e2ecalls.PurposeModel, Dispatched: "a.c"}, want: ""},
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
// match, whichever of them is listed first. The catch-all under a merge
// request is listed after the notes template it competes with, so that the
// more specific one is found before the less specific one is weighed against
// it.
func TestMatchTemplate_SeveralMatch_MostSpecificWins(t *testing.T) {
	templates := []string{
		"gitlab://project/{project_id}",
		"gitlab://project/{project_id}/mr/{merge_request_iid}",
		"gitlab://project/{project_id}/mr/{merge_request_iid}/notes",
		"gitlab://project/{project_id}/mr/{merge_request_iid}/{+rest}",
		"gitlab://project/{project_id}/file/{ref}/{+path}",
		"gitlab://group/{group_id}/{kind}/{value}",
		// Four shapes a placeholder segment can be spelled wrongly in. The
		// templates are read off a session line, which is what a server
		// answered resources/templates/list with, so a malformed one reaches
		// the matcher without anything here having written it.
		"gitlab://opened/{unclosed",
		"gitlab://reserved/{+unclosed",
		"gitlab://closed/unopened}",
		"gitlab://middle/{+path}/tail",
	}
	cases := []struct {
		name    string
		uri     string
		want    string
		matched bool
	}{
		{name: "one segment per variable", uri: "gitlab://project/42", want: "gitlab://project/{project_id}", matched: true},
		{name: "the longer template wins", uri: "gitlab://project/42/mr/7/notes", want: "gitlab://project/{project_id}/mr/{merge_request_iid}/notes", matched: true},
		{name: "the catch-all takes what no longer template does", uri: "gitlab://project/42/mr/7/approvals/1", want: "gitlab://project/{project_id}/mr/{merge_request_iid}/{+rest}", matched: true},
		{name: "a shorter path takes the shorter template", uri: "gitlab://project/42/mr/7", want: "gitlab://project/{project_id}/mr/{merge_request_iid}", matched: true},
		{name: "reserved expansion takes the rest", uri: "gitlab://project/42/file/main/src/a/b.go", want: "gitlab://project/{project_id}/file/{ref}/{+path}", matched: true},
		{name: "reserved expansion needs one segment", uri: "gitlab://project/42/file/main/", matched: false},
		{name: "an empty simple segment does not match", uri: "gitlab://project//mr/7", matched: false},
		{name: "two variables in a row", uri: "gitlab://group/3/label/bug", want: "gitlab://group/{group_id}/{kind}/{value}", matched: true},
		{name: "a literal that differs", uri: "gitlab://project/42/issue/7", matched: false},
		{name: "extra segments do not match", uri: "gitlab://group/3/label/bug/extra", matched: false},
		{name: "an unclosed placeholder is a literal", uri: "gitlab://opened/xyz", matched: false},
		{name: "an unclosed reserved expansion is a literal", uri: "gitlab://reserved/a/b", matched: false},
		{name: "a closing brace alone is a literal", uri: "gitlab://closed/xyz", matched: false},
		{name: "a reserved expansion that is not last matches nothing", uri: "gitlab://middle/a/tail", matched: false},
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

// TestMatchTemplate_TiesAndBareVariables_Resolved verifies the two edges of
// the specificity count. Two templates a URI matches with as many literal
// segments each settle on the one listed first, which is the one the sorted
// session listing puts first, so the cell a read lands on does not move from
// one run to the next. And a template with no literal segment at all still
// matches: zero literals is a match that consumed nothing, not the absence of
// one.
func TestMatchTemplate_TiesAndBareVariables_Resolved(t *testing.T) {
	cases := []struct {
		name      string
		templates []string
		uri       string
		want      string
	}{
		{name: "a tie goes to the first listed", templates: []string{"gitlab://x/{a}/y", "gitlab://x/{b}/y"}, uri: "gitlab://x/1/y", want: "gitlab://x/{a}/y"},
		{name: "a template of one variable", templates: []string{"{whole}"}, uri: "anything", want: "{whole}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, matched := matchTemplate(tc.templates, tc.uri)
			if !matched || got != tc.want {
				t.Errorf("matchTemplate(%q) = (%q, %t), want (%q, true)", tc.uri, got, matched, tc.want)
			}
		})
	}
}

// TestClassify_UnservableReason_DynamicKeepsItsOwnReason verifies that the
// dynamic surface's own reason stands when it has one: a mutation in read-only
// mode is withheld by the mode, which the dynamic surface knows without
// borrowing anything from the meta listing, and the meta sessions of the mode
// serving the domain tool must not turn it back into a servable cell.
func TestClassify_UnservableReason_DynamicKeepsItsOwnReason(t *testing.T) {
	rt := fixtureRuntime()
	rt.calls = nil
	dynamicReadOnly := shapeKey{surface: config.ToolSurfaceDynamic, mode: modeReadOnly}
	rt.sessions = append(rt.sessions, fixtureSession(dynamicReadOnly, true))
	c := classify(rt, fixtureCatalog())

	found := mustCell(t, c, dynamicReadOnly, "issue.delete")
	if found.state != stateUnservable || found.reason != "withheld by read-only mode" {
		t.Errorf("issue.delete on dynamic/read-only = %s (%q), want unservable, withheld by read-only mode", found.state, found.reason)
	}
}

// TestTemplateKinds_UnclassifiableTemplate_LeftOut verifies the fallback
// universe's one filter: a template whose sample URI the server's classifier
// refuses names no kind, since a subscription cell under a name the server
// does not have would be one nothing could ever fill, while the templates
// beside it are named and sorted.
func TestTemplateKinds_UnclassifiableTemplate_LeftOut(t *testing.T) {
	got := templateKinds([]string{
		"gitlab://project/{project_id}/pipeline/{pipeline_id}",
		"gitlab://nowhere/{thing}",
		"gitlab://project/{project_id}/issue/{issue_iid}",
	})
	if want := []string{"issue", "pipeline"}; !slices.Equal(got, want) {
		t.Errorf("templateKinds() = %q, want %q", got, want)
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
		// A placeholder at the very start is still a placeholder: the first
		// character is not text to copy over.
		{template: "{project_id}/tail", want: "1/tail"},
	}
	for _, tc := range cases {
		t.Run(tc.template, func(t *testing.T) {
			if got := sampleURI(tc.template); got != tc.want {
				t.Errorf("sampleURI(%q) = %q, want %q", tc.template, got, tc.want)
			}
		})
	}
}
