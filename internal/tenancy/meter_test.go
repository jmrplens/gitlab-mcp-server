package tenancy

import (
	"fmt"
	"strings"
	"testing"
)

// legacyMeter is the oracle [MeterFor] is held to: the method switch of
// toolutil.AttachRateLimitFunc as it stood at cb6379f53
// (internal/toolutil/rate_limit.go:302-326), before the register decided it.
// The case lists are copied verbatim with the method constants spelled as
// their literals (methodToolsCall, methodResourcesRead,
// methodResourcesSubscribe, methodSubscriptionsListen, methodPromptsGet and
// methodToolsList), and each case body is replaced by the bucket it charged,
// the fall-through being the unmetered path. It is kept apart from MeterFor on
// purpose: a later edit of the register that changes which method is charged
// to which bucket fails the tests below until this copy is edited too, which
// is what makes such an edit a visible change of policy (issue 565).
func legacyMeter(method string) Meter {
	switch method {
	case "tools/call":
		return MeterToolResult
	case "resources/read", "resources/subscribe", "subscriptions/listen", "prompts/get":
		return MeterToolRPC
	case "tools/list":
		return MeterCatalog
	case "completion/complete":
		return MeterCompletion
	}
	return Unmetered
}

// legacyRevisionMethods are the methods of MCP 2025-11-25, requests and
// notifications alike, as schema/2025-11-25/schema.ts declares them at
// modelcontextprotocol/modelcontextprotocol ab3a39c13bd23be691c2760e1c6c5c15a64582e1.
func legacyRevisionMethods() []string {
	return []string{
		"completion/complete", "elicitation/create", "initialize", "logging/setLevel",
		"notifications/cancelled", "notifications/elicitation/complete", "notifications/initialized",
		"notifications/message", "notifications/progress", "notifications/prompts/list_changed",
		"notifications/resources/list_changed", "notifications/resources/updated",
		"notifications/roots/list_changed", "notifications/tasks/status", "notifications/tools/list_changed",
		"ping", "prompts/get", "prompts/list", "resources/list", "resources/read", "resources/subscribe",
		"resources/templates/list", "resources/unsubscribe", "roots/list", "sampling/createMessage",
		"tasks/cancel", "tasks/get", "tasks/list", "tasks/result", "tools/call", "tools/list",
	}
}

// modernRevisionMethods are the methods of MCP 2026-07-28, requests and
// notifications alike, as schema/2026-07-28/schema.ts declares them at the
// same commit.
func modernRevisionMethods() []string {
	return []string{
		"completion/complete", "elicitation/create", "notifications/cancelled", "notifications/message",
		"notifications/progress", "notifications/prompts/list_changed", "notifications/resources/list_changed",
		"notifications/resources/updated", "notifications/subscriptions/acknowledged",
		"notifications/tools/list_changed", "prompts/get", "prompts/list", "resources/list", "resources/read",
		"resources/templates/list", "roots/list", "sampling/createMessage", "server/discover",
		"subscriptions/listen", "tools/call", "tools/list",
	}
}

// everyMethod is the union of both revisions' methods, each once.
func everyMethod() []string {
	var out []string
	for _, method := range append(legacyRevisionMethods(), modernRevisionMethods()...) {
		if !has(out, method) {
			out = append(out, method)
		}
	}
	return out
}

// spellings are the ways a method can arrive that differ from its schema
// spelling: in another case, and with whitespace around it. The switch that
// was replaced compared the strings exactly, so every one of them was
// unmetered, and MeterFor must agree. A spelling that repeats another (the
// empty method in upper case is still empty) is left out.
func spellings(method string) []string {
	var out []string
	for _, s := range []string{
		method, strings.ToUpper(method), strings.ToUpper(method[:min(1, len(method))]) + method[min(1, len(method)):],
		" " + method, method + " ", "\t" + method + "\n",
	} {
		if !has(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// TestMeterFor_AgreesWithTheReplacedSwitch holds MeterFor to the switch it
// replaced on every method of both protocol revisions, requests and
// notifications alike, on the empty method, and on each of them spelled in
// another case or with whitespace around it. The mapping being identical is
// the whole of why promoting it changed nothing (issue 565).
func TestMeterFor_AgreesWithTheReplacedSwitch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		methods []string
	}{
		{"2025-11-25", legacyRevisionMethods()},
		{"2026-07-28", modernRevisionMethods()},
		{"no method", []string{""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range tc.methods {
				for _, spelled := range spellings(method) {
					t.Run(fmt.Sprintf("%q", spelled), func(t *testing.T) {
						if got, want := MeterFor(spelled), legacyMeter(spelled); got != want {
							t.Errorf("MeterFor(%q) = %d, the replaced switch charged %d", spelled, got, want)
						}
					})
				}
			}
		})
	}
}

// meteredRefusal is one refusal of a row MeterFor answers part of, with the
// bucket the methods it names are charged to.
type meteredRefusal struct {
	name    string
	row     string
	channel Channel
	want    Meter
}

// meteredRefusals are the refusals of RTC-001 to RTC-003, one per bucket.
func meteredRefusals() []meteredRefusal {
	return []meteredRefusal{
		{"tool calls, refused as a result", "RTC-001", ToolError, MeterToolResult},
		{"the other doors to GitLab, refused in-band", "RTC-001", RPC, MeterToolRPC},
		{"completions, refused empty", "RTC-002", EmptyCompletion, MeterCompletion},
		{"catalog listings, refused in-band", "RTC-003", RPC, MeterCatalog},
	}
}

// refusedMethods returns the methods the row's refusals on the channel name,
// failing the test when the row does not exist, does not name MeterFor or has
// no such refusal.
func refusedMethods(t *testing.T, m meteredRefusal) []string {
	t.Helper()
	d, ok := Lookup(m.row)
	if !ok {
		t.Fatalf("no row %s", m.row)
	}
	if !has(d.Functions, "MeterFor") {
		t.Errorf("%s does not name MeterFor in Functions: %v", m.row, d.Functions)
	}
	var methods []string
	for _, r := range d.Refusals {
		if r.Channel == m.channel {
			methods = append(methods, r.Methods...)
		}
	}
	if len(methods) == 0 {
		t.Fatalf("%s has no %s refusal", m.row, m.channel)
	}
	return methods
}

// TestMeterFor_ChargesWhatTheRowsRefuse ties the function to the rows that
// name it: each method a refusal of RTC-001 to RTC-003 names is charged to the
// bucket whose refusal that is. A method moved from one bucket to another in
// MeterFor without its row, or the other way round, fails here.
func TestMeterFor_ChargesWhatTheRowsRefuse(t *testing.T) {
	for _, m := range meteredRefusals() {
		t.Run(m.name, func(t *testing.T) {
			for _, method := range refusedMethods(t, m) {
				if got := MeterFor(method); got != m.want {
					t.Errorf("MeterFor(%q) = %d, want %d, the bucket %s refuses it on", method, got, m.want, m.row)
				}
			}
		})
	}
}

// TestMeterFor_LeavesEveryMethodNoRowRefusesUnmetered is RTC-004 from the
// other side: every method of either revision that no refusal of RTC-001 to
// RTC-003 names is charged to no bucket, and RTC-004 is the promoted row that
// says so.
func TestMeterFor_LeavesEveryMethodNoRowRefusesUnmetered(t *testing.T) {
	if d, ok := Lookup("RTC-004"); !ok || d.Disposition != Promoted || !has(d.Functions, "MeterFor") {
		t.Fatalf("RTC-004 = %+v, want a promoted row naming MeterFor", d)
	}
	charged := map[string]bool{}
	for _, m := range meteredRefusals() {
		for _, method := range refusedMethods(t, m) {
			charged[method] = true
		}
	}
	for _, method := range everyMethod() {
		if charged[method] {
			continue
		}
		t.Run(method, func(t *testing.T) {
			if got := MeterFor(method); got != Unmetered {
				t.Errorf("MeterFor(%q) = %d, and no row refuses it", method, got)
			}
		})
	}
}

// FuzzMeterFor holds MeterFor to the replaced switch on inputs nobody listed.
// The seeds are every method of both revisions and the empty method; the
// committed corpus under testdata/fuzz/FuzzMeterFor adds near misses (a
// trailing slash, a NUL, a full-width slash, a prefix).
func FuzzMeterFor(f *testing.F) {
	for _, method := range append(everyMethod(), "") {
		f.Add(method)
	}
	f.Fuzz(func(t *testing.T, method string) {
		if got, want := MeterFor(method), legacyMeter(method); got != want {
			t.Errorf("MeterFor(%q) = %d, the replaced switch charged %d", method, got, want)
		}
	})
}
