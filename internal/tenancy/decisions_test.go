package tenancy

import (
	"strings"
	"testing"
)

// specRequirementIDs is every requirement of the specification's section 3.2,
// written out rather than generated, so that a requirement added to the
// specification and forgotten in the register, or the reverse, fails here by
// name. END-005 is the specification amendment that answered the plan's
// question Q7.
var specRequirementIDs = []string{
	"IDN-001", "IDN-002", "IDN-003", "IDN-004", "IDN-005", "IDN-006", "IDN-007",
	"IDN-008", "IDN-009", "IDN-010", "IDN-011", "IDN-012", "IDN-013",
	"ADM-001", "ADM-002", "ADM-003", "ADM-004", "ADM-005", "ADM-006", "ADM-007",
	"ADM-008", "ADM-009", "ADM-010", "ADM-011", "ADM-012", "ADM-013",
	"AUB-001", "AUB-002", "AUB-003", "AUB-004", "AUB-005",
	"RTC-001", "RTC-002", "RTC-003", "RTC-004", "RTC-005", "RTC-006",
	"HLD-001", "HLD-002", "HLD-003", "HLD-004", "HLD-005", "HLD-006", "HLD-007",
	"HLD-008", "HLD-009", "HLD-010",
	"POL-001", "POL-002", "POL-003", "POL-004", "POL-005", "POL-006", "POL-007",
	"POL-008", "POL-009",
	"AUT-001", "AUT-002", "AUT-003", "AUT-004", "AUT-005", "AUT-006",
	"DST-001", "DST-002", "DST-003",
	"END-001", "END-002", "END-003", "END-004", "END-005",
	"RQB-001", "RQB-002", "RQB-003", "RQB-004", "RQB-005", "RQB-006", "RQB-007",
	"RQB-008", "RQB-009", "RQB-010",
}

// TestDecisions_EveryRequirementHasOneRow holds the register to the
// specification's requirements one for one: each has exactly one row, and
// there is no row the specification does not know.
func TestDecisions_EveryRequirementHasOneRow(t *testing.T) {
	counts := map[string]int{}
	for _, d := range Decisions() {
		counts[d.ID]++
	}
	for _, id := range specRequirementIDs {
		t.Run(id, func(t *testing.T) {
			if counts[id] != 1 {
				t.Errorf("%s has %d rows, want exactly one", id, counts[id])
			}
		})
	}
	if len(counts) != len(specRequirementIDs) {
		t.Errorf("the register declares %d requirements and the specification has %d", len(counts), len(specRequirementIDs))
	}
	if got := strings.Join(requirementIDs(), ","); got != strings.Join(specRequirementIDs, ",") {
		t.Errorf("requirementIDs() = %s\nwant %s", got, strings.Join(specRequirementIDs, ","))
	}
}

// TestDecisions_ValidateIsNil is the register's own verdict on itself: every
// row meets the invariants, or carries the finding that records why not.
func TestDecisions_ValidateIsNil(t *testing.T) {
	if err := Validate(Decisions()); err != nil {
		t.Errorf("Validate(Decisions()) =\n%v", err)
	}
}

// TestDecisions_AreGroupedByQuestion holds the order Decisions promises: the
// families by the question they answer, the request bounds last.
func TestDecisions_AreGroupedByQuestion(t *testing.T) {
	var last Question
	sawBound := false
	for _, d := range Decisions() {
		if d.Disposition == RequestBound {
			sawBound = true
			continue
		}
		if sawBound {
			t.Errorf("%s follows a request bound", d.ID)
		}
		if d.Question < last {
			t.Errorf("%s answers question %d after question %d", d.ID, d.Question, last)
		}
		last = d.Question
	}
}

// TestDecisions_DispositionCounts pins how many rows the register holds of
// each disposition in this layer: RTC-004 is promoted to MeterFor, and POL-003
// to Busy.
func TestDecisions_DispositionCounts(t *testing.T) {
	counts := map[Disposition]int{}
	for _, d := range Decisions() {
		counts[d.Disposition]++
	}
	for _, tc := range []struct {
		name        string
		disposition Disposition
		want        int
	}{
		{"valued", Valued, 27},
		{"ruled", Ruled, 35},
		{"promoted", Promoted, 2},
		{"mechanism", Mechanism, 6},
		{"request-bound", RequestBound, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if counts[tc.disposition] != tc.want {
				t.Errorf("%d rows, want %d", counts[tc.disposition], tc.want)
			}
		})
	}
}

// TestDecisions_FunctionsNameThePromotedRules pins which rows name a register
// function: the four rows of the method meter name MeterFor, POL-003 names
// Busy, the three authentication budgets name the switch that says whether
// each is on, and every other row names none.
func TestDecisions_FunctionsNameThePromotedRules(t *testing.T) {
	want := map[string]string{
		"RTC-001": "MeterFor",
		"RTC-002": "MeterFor",
		"RTC-003": "MeterFor",
		"RTC-004": "MeterFor",
		"POL-003": "Busy",
		"AUB-001": "BudgetOn",
		"AUB-002": "TransportSourceBudgetOn",
		"AUB-003": "EscalationOn",
	}
	for _, d := range Decisions() {
		t.Run(d.ID, func(t *testing.T) {
			if got := strings.Join(d.Functions, ","); got != want[d.ID] {
				t.Errorf("%s names functions %q, want %q", d.ID, got, want[d.ID])
			}
		})
	}
}

// TestLookup_FindsARowAndRefusesAnUnknownID reads one row back and asks for one
// that does not exist.
func TestLookup_FindsARowAndRefusesAnUnknownID(t *testing.T) {
	d, ok := Lookup("HLD-001")
	if !ok || d.ID != "HLD-001" || d.Partner != "HLD-002" {
		t.Errorf("Lookup(HLD-001) = %+v, %v", d, ok)
	}
	if _, found := Lookup("HLD-999"); found {
		t.Error("Lookup(HLD-999) found a row")
	}
}

// refusalPin is what a row's refusal must say, written as literals.
type refusalPin struct {
	methods   string
	era       Era
	channel   Channel
	code      int
	status    int
	retry     RetryAfter
	challenge bool
	prefix    string
	answer    Answer
	charged   string
}

// rowPin is what a row must say, written as literals from the specification.
type rowPin struct {
	question    Question
	kind        Kind
	class       Class
	disposition Disposition
	key         Key
	stdio       Key
	stated      Key
	reason      Key
	refusals    []refusalPin
}

// head is everything a pin says but its refusals, in a form that compares.
func (p rowPin) head() [8]uint8 {
	return [8]uint8{
		uint8(p.question), uint8(p.kind), uint8(p.class), uint8(p.disposition),
		uint8(p.key), uint8(p.stdio), uint8(p.stated), uint8(p.reason),
	}
}

// The refusals several rows share, as the specification states them.
const (
	pinRejected = "GitLab rejected this token."
	pinBlocked  = "Too many failed authentication attempts from this address."
	pinListen   = "too many open subscriptions/listen streams ("
	pinWatchers = "subscriptions: too many active subscriptions"
	pinRate     = "rate limit exceeded for "
	pinSeveral  = "This deployment serves several GitLab instances"
	pinSubMeths = "resources/subscribe,subscriptions/listen"
	pinCharged  = "AUB-001,AUB-002,AUB-003"
	pinRetry503 = "GitLab could not verify this token right now"
)

func pinBlockedAt() []refusalPin {
	blocked := refusalPin{
		methods: "http", channel: Gate, code: -42900, status: 429, retry: RetryAfterLongestBlock,
		prefix: pinBlocked, answer: RetryLater,
	}
	return []refusalPin{blocked, blocked}
}

func pinListenEnd(reason string, answer Answer) refusalPin {
	return refusalPin{methods: "subscriptions/listen", era: EraModern, channel: ListenEnd, prefix: reason, answer: answer}
}

func pinNarrowed(answer Answer) []refusalPin {
	return []refusalPin{
		{methods: "tools/call", channel: Withheld, answer: answer},
		{methods: "tools/list", channel: Absent, answer: answer},
	}
}

// rowPins is every row as the specification states it: its question (spec
// 4.4), kind, class (spec 2.3 and 3.2.11), disposition, key and stdio key
// (spec 3.2 and 4.1), the unit its reason names and the unit of what it
// cites (spec 2.3), and every refusal (spec 3.2 and 4.3).
//
//nolint:maintidx // one table of literals, one entry per requirement; splitting it would scatter the pins it exists to hold in one place.
func rowPins() map[string]rowPin {
	pins := map[string]rowPin{
		"IDN-001": {Identify, Rule, ClassC, Ruled, KeyRequest, KeyProcess, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -40100, status: 401, challenge: true,
				prefix: "Authentication required: send a GitLab personal access token", answer: Reauthorize, charged: "AUB-001,AUB-002",
			},
			{
				methods: "http", channel: Gate, code: -40100, status: 401, challenge: true,
				prefix: "Authentication required: send an OAuth access token", answer: Reauthorize, charged: "AUB-001,AUB-002",
			},
		}},
		"IDN-002": {Identify, Rule, ClassR, Ruled, KeyEntry, KeyNone, KeyNone, KeyNone, nil},
		"IDN-003": {Identify, Rule, ClassR, Ruled, KeyOwner, KeyNone, KeyNone, KeyNone, nil},
		"IDN-004": {Identify, Rule, ClassA, Ruled, KeyAddress, KeyNone, KeyNone, KeyNone, nil},
		"IDN-005": {Identify, Rule, ClassA, Ruled, KeySource, KeyNone, KeyNone, KeyNone, nil},
		"IDN-006": {Identify, Rule, ClassR, Mechanism, KeyEntry, KeyNone, KeyNone, KeyNone, nil},
		"IDN-007": {Identify, Rule, ClassP, Ruled, KeyUnbound, KeyNone, KeyNone, KeyNone, []refusalPin{
			{methods: "tools/call", channel: ToolError, answer: RetryLater},
			{
				methods: "resources/read,prompts/get", channel: RPC, code: -32603,
				prefix: "this request could not be attributed to a credential", answer: RetryLater,
			},
			{
				methods: pinSubMeths, channel: RPC, code: -32603,
				prefix: "this subscription could not be attributed to a credential", answer: RetryLater,
			},
		}},
		"IDN-008": {Identify, Rule, ClassT, Ruled, KeyTenant, KeyProcess, KeyNone, KeyNone, nil},
		"IDN-010": {Identify, Rule, ClassC, Mechanism, KeySession, KeyNone, KeyNone, KeyNone, nil},
		"ADM-011": {Identify, Rule, ClassQ, Ruled, KeyRequest, KeyNone, KeyNone, KeyNone, []refusalPin{
			{methods: "http", channel: Gate, code: -32600, status: 400, prefix: pinSeveral, answer: FixRequest},
			{methods: "http", channel: Gate, code: -32600, status: 400, answer: FixRequest},
			{methods: "http", channel: Gate, code: -32600, status: 400, prefix: pinSeveral, answer: FixRequest},
			{
				methods: "http", channel: Gate, code: -40300, status: 403,
				prefix: "This deployment does not serve the GitLab instance", answer: AskOperator,
			},
		}},

		"ADM-001": {Admit, Rule, ClassC, Ruled, KeyEntry, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -40100, status: 401, challenge: true, prefix: pinRejected,
				answer: Reauthorize, charged: pinCharged,
			},
			{
				methods: "http", channel: Gate, code: -50300, status: 503,
				prefix: "Could not initialize a GitLab session for this token.", answer: RetryLater,
			},
		}},
		"ADM-002": {Admit, Rule, ClassC, Valued, KeyVerified, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -40100, status: 401, challenge: true, prefix: pinRejected,
				answer: Reauthorize, charged: pinCharged,
			},
			{methods: "http", channel: Gate, code: -40300, status: 403, challenge: true, answer: WidenScope},
			{
				methods: "http", channel: Gate, code: -40300, status: 403, challenge: true,
				prefix: "GitLab rejected this token for lacking the scope", answer: WidenScope,
			},
			{
				methods: "http", channel: Gate, code: -50300, status: 503, retry: RetryAfterUpstreamOrFixed,
				prefix: pinRetry503 + ";", answer: RetryLater,
			},
			{
				methods: "http", channel: Gate, code: -50300, status: 503, retry: RetryAfterFixed,
				prefix: pinRetry503 + ".", answer: RetryLater,
			},
		}},
		"ADM-003": {Admit, Rule, ClassC, Ruled, KeyVerified, KeyNone, KeyNone, KeyNone, nil},
		"ADM-004": {Admit, Rule, ClassC, Ruled, KeyApplication, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -40100, status: 401, challenge: true,
				prefix: "This token is valid for the GitLab instance", answer: Reauthorize,
			},
			{
				methods: "http", channel: Gate, code: -50300, status: 503, retry: RetryAfterFixed,
				prefix: "This deployment admits only tokens issued to specific OAuth applications", answer: RetryLater,
			},
		}},
		"ADM-005": {Admit, Lifetime, ClassC, Valued, KeyVerified, KeyNone, KeyNone, KeyNone, nil},
		"ADM-006": {Admit, Lifetime, ClassA, Valued, KeyRefused, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -40100, status: 401, challenge: true, prefix: pinRejected,
				answer: Reauthorize, charged: pinCharged,
			},
		}},
		"ADM-007": {Admit, Rule, ClassC, Ruled, KeySession, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -32600, status: 404,
				prefix: "This session does not belong to the presented credential.", answer: StartOver,
			},
		}},
		"ADM-008": {Admit, Lifetime, ClassC, Valued, KeyEntry, KeyNone, KeyNone, KeyNone, nil},
		"ADM-009": {Admit, Lifetime, ClassC, Valued, KeyEntry, KeyNone, KeyNone, KeyNone, []refusalPin{
			pinListenEnd("credential_revoked", Reauthorize),
		}},
		"ADM-010": {Admit, Lifetime, ClassC, Valued, KeyEntry, KeyNone, KeyNone, KeyNone, []refusalPin{
			pinListenEnd("credential_revoked", Reauthorize),
		}},
		"ADM-012": {Admit, Rule, ClassQ, Ruled, KeyRequest, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -40300, status: 403, prefix: "Cross-origin request refused:",
				answer: AskOperator,
			},
		}},
		"ADM-013": {Admit, Rule, ClassQ, Ruled, KeyRequest, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -40300, status: 403,
				prefix: "Request refused: the Host header names a host", answer: AskOperator,
			},
		}},
		"AUB-001": {Admit, Budget, ClassA, Valued, KeyAddress, KeyNone, KeyNone, KeyNone, pinBlockedAt()},
		"AUB-002": {Admit, Budget, ClassA, Valued, KeySource, KeyNone, KeyNone, KeyNone, pinBlockedAt()},
		"AUB-003": {Admit, Budget, ClassA, Valued, KeyAddress, KeyNone, KeyNone, KeyNone, pinBlockedAt()},
		"AUB-004": {Admit, Ceiling, ClassP, Valued, KeyProcess, KeyNone, KeyNone, KeyProcess, []refusalPin{
			{methods: "http", channel: Silent, answer: NoAnswer},
		}},
		"AUB-005": {Admit, Lifetime, ClassP, Valued, KeyProcess, KeyNone, KeyNone, KeyNone, nil},
		"POL-006": {Admit, Ceiling, ClassP, Valued, KeyProcess, KeyNone, KeyNone, KeyProcess, []refusalPin{
			{
				methods: "http", channel: Gate, code: -50300, status: 503,
				prefix: "Could not initialize a GitLab session for this token.", answer: RetryLater,
			},
		}},
		"POL-009": {Admit, Rule, ClassC, Mechanism, KeyEntry, KeyNone, KeyNone, KeyNone, nil},
		"DST-001": {Admit, Rule, ClassQ, Ruled, KeyRequest, KeyNone, KeyNone, KeyNone, []refusalPin{
			{methods: "http", channel: Gate, code: -32600, status: 400, answer: AskOperator},
		}},
		"DST-002": {Admit, Rule, ClassP, Ruled, KeyDeployment, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "startup", channel: Startup, prefix: "--allow-any-gitlab-url names no instance and --http-addr ",
				answer: AskOperator,
			},
		}},

		"AUT-001": {Authorize, Rule, ClassC, Ruled, KeyEntry, KeyProcess, KeyNone, KeyNone, pinNarrowed(WidenScope)},
		"AUT-002": {Authorize, Rule, ClassC, Ruled, KeyEntry, KeyProcess, KeyNone, KeyNone, pinNarrowed(WidenScope)},
		"AUT-003": {Authorize, Rule, ClassE, Valued, KeyEntry, KeyProcess, KeyNone, KeyNone, []refusalPin{
			{methods: "tools/call", channel: Unknown, answer: NoAnswer},
			{methods: "tools/list", channel: Absent, answer: NoAnswer},
		}},
		"AUT-004": {Authorize, Rule, ClassP, Ruled, KeyDeployment, KeyDeployment, KeyNone, KeyNone, pinNarrowed(AskOperator)},
		"AUT-005": {Authorize, Rule, ClassP, Ruled, KeyProcess, KeyProcess, KeyNone, KeyNone, nil},
		"AUT-006": {Authorize, Rule, ClassP, Ruled, KeyProcess, KeyProcess, KeyNone, KeyNone, []refusalPin{
			{methods: "tools/call", channel: ToolError, answer: AskOperator},
		}},
		"POL-007": {Authorize, Rule, ClassC, Ruled, KeyEntry, KeyProcess, KeyNone, KeyNone, nil},
		"DST-003": {Authorize, Rule, ClassC, Ruled, KeyEntry, KeyProcess, KeyNone, KeyNone, []refusalPin{
			{
				methods: "tools/call", channel: ToolError, prefix: "this server refused to connect to that address",
				answer: AskOperator,
			},
		}},
		"IDN-012": {Authorize, Rule, ClassC, Ruled, KeyCredential, KeyProcess, KeyNone, KeyNone, nil},
		"IDN-013": {Authorize, Rule, ClassQ, Ruled, KeySession, KeyProcess, KeyNone, KeyNone, nil},
		"HLD-005": {Authorize, Rule, ClassQ, Ruled, KeyRequest, KeyRequest, KeyNone, KeyNone, []refusalPin{
			{
				methods: pinSubMeths, channel: RPC, code: -32602, prefix: "subscriptions: resource is not subscribable",
				answer: FixRequest,
			},
		}},
		"HLD-006": {Authorize, Rule, ClassC, Ruled, KeyEntry, KeyProcess, KeyNone, KeyNone, []refusalPin{
			{
				methods: pinSubMeths, channel: RPC, code: -32602, prefix: "subscriptions: resource inaccessible",
				answer: FixRequest,
			},
		}},
		"HLD-008": {Authorize, Rule, ClassQ, Ruled, KeyRequest, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "resources/subscribe", era: EraLegacy, channel: RPC, code: -32600,
				prefix: "resources/subscribe cannot be honored in stateless HTTP mode", answer: FixRequest,
			},
		}},
		"HLD-009": {Authorize, Rule, ClassP, Ruled, KeyDeployment, KeyDeployment, KeyNone, KeyNone, nil},

		"RTC-001": {Allow, Rate, ClassD, Valued, KeyEntry, KeyProcess, KeyCredential, KeyTenant, []refusalPin{
			{methods: "tools/call", channel: ToolError, prefix: pinRate, answer: RetryLater},
			{
				methods: "resources/read,resources/subscribe,subscriptions/listen,prompts/get", channel: RPC,
				code: -42900, prefix: pinRate, answer: RetryLater,
			},
		}},
		"RTC-002": {Allow, Rate, ClassU, Valued, KeyEntry, KeyProcess, KeyNone, KeyNone, []refusalPin{
			{methods: "completion/complete", channel: EmptyCompletion, answer: RetryLater},
		}},
		"RTC-003": {Allow, Rate, ClassD, Valued, KeyEntry, KeyProcess, KeyProcess, KeyProcess, []refusalPin{
			{methods: "tools/list", channel: RPC, code: -42900, prefix: pinRate, answer: RetryLater},
		}},
		"RTC-004": {Allow, Rule, ClassP, Promoted, KeyRequest, KeyRequest, KeyNone, KeyNone, nil},
		"RTC-005": {Allow, Rate, ClassD, Valued, KeyEntry, KeyProcess, KeyTenant, KeyTenant, []refusalPin{
			{methods: pinSubMeths, channel: RPC, code: -32000, prefix: "subscriptions: rate limited", answer: RetryLater},
			{methods: "notifications/resources/updated", channel: Silent, answer: NoAnswer},
		}},
		"RTC-006": {Allow, Bound, ClassQ, Valued, KeyRequest, KeyRequest, KeyNone, KeyNone, nil},
		"HLD-001": {Allow, Ceiling, ClassR, Valued, KeyEntry, KeyProcess, KeyEntry, KeyEntry, []refusalPin{
			{methods: "subscriptions/listen", era: EraModern, channel: RPC, code: -32000, prefix: pinListen, answer: RetryLater},
		}},
		"HLD-002": {Allow, Ceiling, ClassP, Valued, KeyProcess, KeyProcess, KeyProcess, KeyProcess, []refusalPin{
			{methods: "subscriptions/listen", era: EraModern, channel: RPC, code: -32000, prefix: pinListen, answer: RetryLater},
		}},
		"HLD-003": {Allow, Ceiling, ClassD, Valued, KeyEntry, KeyProcess, KeyTenant, KeyTenant, []refusalPin{
			{methods: pinSubMeths, channel: RPC, code: -32000, prefix: pinWatchers, answer: RetryLater},
		}},
		"HLD-004": {Allow, Ceiling, ClassP, Valued, KeyProcess, KeyProcess, KeyProcess, KeyProcess, []refusalPin{
			{methods: pinSubMeths, channel: RPC, code: -32000, prefix: pinWatchers, answer: RetryLater},
		}},
		"HLD-007": {Allow, Lifetime, ClassQ, Valued, KeyRequest, KeyRequest, KeyNone, KeyNone, []refusalPin{
			pinListenEnd("lifetime_reached", StartOver),
		}},
		"HLD-010": {Allow, Ceiling, ClassP, Ruled, KeyProcess, KeyNone, KeyNone, KeyProcess, nil},
		"POL-001": {Allow, Ceiling, ClassP, Valued, KeyProcess, KeyNone, KeyNone, KeyNone, nil},
		"POL-002": {Allow, Rule, ClassR, Ruled, KeyEntry, KeyNone, KeyNone, KeyNone, []refusalPin{
			pinListenEnd("credential_evicted", StartOver),
			{methods: "eviction", era: EraLegacy, channel: SessionClose, answer: StartOver},
		}},
		"POL-003": {Allow, Rule, ClassR, Promoted, KeyEntry, KeyNone, KeyNone, KeyNone, nil},
		"POL-004": {Allow, Lifetime, ClassR, Valued, KeyEntry, KeyNone, KeyNone, KeyNone, []refusalPin{
			pinListenEnd("credential_reset", StartOver),
		}},
		"POL-005": {Allow, Rule, ClassR, Mechanism, KeyEntry, KeyNone, KeyNone, KeyNone, nil},
		"IDN-009": {Allow, Rule, ClassP, Ruled, KeyProcess, KeyProcess, KeyNone, KeyNone, nil},
		"IDN-011": {Allow, Lifetime, ClassQ, Valued, KeyRequest, KeyRequest, KeyNone, KeyNone, nil},

		"END-001": {End, Rule, ClassR, Ruled, KeyOwner, KeyNone, KeyNone, KeyNone, []refusalPin{
			pinListenEnd("credential_evicted", StartOver),
			pinListenEnd("credential_reset", StartOver),
			pinListenEnd("credential_revoked", Reauthorize),
			pinListenEnd("shutdown", RetryLater),
		}},
		"END-002": {End, Rule, ClassR, Ruled, KeyOwner, KeyProcess, KeyNone, KeyNone, []refusalPin{
			pinListenEnd("resource_gone", FixRequest),
			pinListenEnd("lifetime_reached", StartOver),
			pinListenEnd("watcher_evicted", StartOver),
		}},
		"END-003": {End, Rule, ClassR, Ruled, KeyOwner, KeyNone, KeyNone, KeyNone, []refusalPin{
			{methods: "eviction", era: EraLegacy, channel: SessionClose, answer: StartOver},
		}},
		"END-004": {End, Rule, ClassR, Mechanism, KeyOwner, KeyNone, KeyNone, KeyNone, nil},
		"END-005": {End, Lifetime, ClassQ, Valued, KeySession, KeyNone, KeyNone, KeyNone, []refusalPin{
			{methods: "expiry", era: EraLegacy, channel: SessionClose, answer: StartOver},
		}},
		"POL-008": {End, Rule, ClassR, Mechanism, KeyOwner, KeyNone, KeyNone, KeyNone, nil},
	}
	for _, id := range []string{
		"RQB-001", "RQB-002", "RQB-003", "RQB-004", "RQB-005", "RQB-006", "RQB-007",
		"RQB-008", "RQB-009", "RQB-010",
	} {
		pins[id] = rowPin{Allow, Bound, ClassP, RequestBound, KeyRequest, KeyRequest, KeyNone, KeyNone, nil}
	}
	// A request body only exists over HTTP.
	rqb001 := pins["RQB-001"]
	rqb001.stdio = KeyNone
	pins["RQB-001"] = rqb001
	return pins
}

// TestDecisions_EachRowIsWhatTheSpecificationSays pins every row's answer to
// the literal values the specification states, one subtest per row, so that a
// change to any of them fails one named row. It pins the decision; the layers'
// own tests keep pinning the enforcement.
func TestDecisions_EachRowIsWhatTheSpecificationSays(t *testing.T) {
	pins := rowPins()
	if len(pins) != len(specRequirementIDs) {
		t.Fatalf("%d pins for %d requirements", len(pins), len(specRequirementIDs))
	}
	for _, id := range specRequirementIDs {
		t.Run(id, func(t *testing.T) {
			d, ok := Lookup(id)
			if !ok {
				t.Fatalf("%s has no row", id)
			}
			want := pins[id]
			got := rowPin{d.Question, d.Kind, d.Class, d.Disposition, d.Key, d.StdioKey, d.StatedUnit, d.ReasonUnit, nil}
			if got.head() != want.head() {
				t.Errorf("question, kind, class, disposition, key, stdio key, stated unit, reason unit:\ngot  %v\nwant %v",
					got.head(), want.head())
			}
			if len(d.Refusals) != len(want.refusals) {
				t.Fatalf("%d refusals, want %d", len(d.Refusals), len(want.refusals))
			}
			for i, r := range d.Refusals {
				gotRefusal := refusalPin{
					strings.Join(r.Methods, ","), r.Era, r.Channel, r.Code, r.Status, r.RetryAfter, r.Challenge,
					r.Prefix, r.Answer, strings.Join(r.Charged, ","),
				}
				if gotRefusal != want.refusals[i] {
					t.Errorf("refusal %d:\ngot  %+v\nwant %+v", i, gotRefusal, want.refusals[i])
				}
			}
		})
	}
}

// TestDecisions_EveryRefusalNamesWhereItsTextLives holds every refusal to a
// site with the refuse role, so the gate has somewhere to find its text.
func TestDecisions_EveryRefusalNamesWhereItsTextLives(t *testing.T) {
	for _, d := range Decisions() {
		for i, r := range d.Refusals {
			if r.At.Role != Refuse || r.At.Pkg == "" || r.At.Name == "" {
				t.Errorf("%s refusal %d: At = %+v", d.ID, i, r.At)
			}
			if r.Via != (Site{}) && r.Via.Role != Refuse {
				t.Errorf("%s refusal %d: Via = %+v", d.ID, i, r.Via)
			}
		}
	}
}

// TestDecisions_EverySiteIsNamed holds every site of every row to a package, a
// symbol and a role, and each alias, argument and pin to the constant it
// carries.
func TestDecisions_EverySiteIsNamed(t *testing.T) {
	for _, d := range Decisions() {
		for _, s := range append(append([]Site{}, d.Sites...), d.ReasonAt) {
			if s == (Site{}) {
				continue
			}
			if s.Pkg == "" || s.Name == "" || s.Role == 0 {
				t.Errorf("%s: site %+v", d.ID, s)
			}
			if (s.Role == Alias || s.Role == Arg || s.Role == Pin) && s.Reads == "" {
				t.Errorf("%s: %s %s carries no register constant", d.ID, s.Pkg, s.Name)
			}
		}
		if (d.Reason == "") != (d.ReasonAt == Site{}) {
			t.Errorf("%s: a reason and where it lives go together", d.ID)
		}
	}
}
