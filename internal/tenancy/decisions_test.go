package tenancy

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// specRequirementIDs is every requirement the register answers, written out
// rather than generated, so that a requirement added and forgotten in the
// register, or the reverse, fails here by name. The list taken when the
// register landed is one ID per decision and per-request bound the code held
// at cb6379f53, END-005 among them, which the register's gate surfaced as it
// landed; a limit added later adds its ID here in the change that adds its
// row.
var specRequirementIDs = []string{
	"IDN-001", "IDN-002", "IDN-003", "IDN-004", "IDN-005", "IDN-006", "IDN-007",
	"IDN-008", "IDN-009", "IDN-010", "IDN-011", "IDN-012", "IDN-013",
	"ADM-001", "ADM-002", "ADM-003", "ADM-004", "ADM-005", "ADM-006", "ADM-007",
	"ADM-008", "ADM-009", "ADM-010", "ADM-011", "ADM-012", "ADM-013", "ADM-014",
	"AUB-001", "AUB-002", "AUB-003", "AUB-004", "AUB-005",
	"RTC-001", "RTC-002", "RTC-003", "RTC-004", "RTC-005", "RTC-006", "RTC-007",
	"HLD-001", "HLD-002", "HLD-003", "HLD-004", "HLD-005", "HLD-006", "HLD-007",
	"HLD-008", "HLD-009", "HLD-010", "HLD-011",
	"POL-001", "POL-002", "POL-003", "POL-004", "POL-005", "POL-006", "POL-007",
	"POL-008", "POL-009",
	"AUT-001", "AUT-002", "AUT-003", "AUT-004", "AUT-005", "AUT-006", "AUT-007",
	"AUT-008",
	"DST-001", "DST-002", "DST-003",
	"END-001", "END-002", "END-003", "END-004", "END-005",
	"RQB-001", "RQB-002", "RQB-003", "RQB-004", "RQB-005", "RQB-006", "RQB-007",
	"RQB-008", "RQB-009", "RQB-010", "RQB-011",
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
// to Busy. ADM-014, the verification ceiling issue 950 added, is the
// twenty-ninth valued row, HLD-011, the held-request ceiling issue 951 added,
// the thirtieth, and HLD-010, the session ceiling the same issue put in place
// of the decision by absence that row used to record, the thirty-first, which
// is also why there was one ruled row fewer. AUT-007, the actions issue 952
// withholds from a fine-grained session, is the thirty-fifth ruled row, and
// AUT-008, what such a session's grant does not reach, the thirty-sixth;
// RQB-011, the bounds on reading that grant, is the eleventh request bound.
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
		{"valued", Valued, 31},
		{"ruled", Ruled, 36},
		{"promoted", Promoted, 2},
		{"mechanism", Mechanism, 6},
		{"request-bound", RequestBound, 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if counts[tc.disposition] != tc.want {
				t.Errorf("%d rows, want %d", counts[tc.disposition], tc.want)
			}
		})
	}
}

// TestDecisions_FunctionsNameThePromotedRules pins which rows name a register
// function: the five rows of the method meter name MeterFor, and so does
// HLD-011, which counts the methods it meters to an upstream; POL-003 names
// Busy, the three authentication budgets name the switch that says whether
// each is on, AUT-008 names CoverableAt, what one scope of a fine-grained
// token's grant covers, and every other row names none.
func TestDecisions_FunctionsNameThePromotedRules(t *testing.T) {
	want := map[string]string{
		"RTC-001": "MeterFor",
		"RTC-002": "MeterFor",
		"RTC-003": "MeterFor",
		"RTC-004": "MeterFor",
		"RTC-007": "MeterFor",
		"HLD-011": "MeterFor",
		"POL-003": "Busy",
		"AUB-001": "BudgetOn",
		"AUB-002": "TransportSourceBudgetOn",
		"AUB-003": "EscalationOn",
		"AUT-008": "CoverableAt",
	}
	for _, d := range Decisions() {
		t.Run(d.ID, func(t *testing.T) {
			if got := strings.Join(d.Functions, ","); got != want[d.ID] {
				t.Errorf("%s names functions %q, want %q", d.ID, got, want[d.ID])
			}
		})
	}
}

// TestDecisions_ListingBucket_HasAProcessPartnerThatFollowsIt pins what issue
// 951 decided for tools/list: the per-entry bucket has a partner keyed on the
// process that no operator can change, the partner follows the row it
// partners, so it is off where that row is, by the decision that says so, and
// it refuses with that row's own refusal, so no caller is told that other
// callers are listing (INV-019). Neither row carries F-03 any longer, since
// the departure it recorded is answered. The finding itself stays in the
// register's list, filed as issue 951's.
func TestDecisions_ListingBucket_HasAProcessPartnerThatFollowsIt(t *testing.T) {
	entry, _ := Lookup("RTC-003")
	process, _ := Lookup("RTC-007")
	if entry.Partner != "RTC-007" || entry.Carries("F-03") {
		t.Errorf("RTC-003: partner %q, carries F-03 %v; want partner RTC-007 and no F-03", entry.Partner, entry.Carries("F-03"))
	}
	if process.Key != KeyProcess || process.Source != Constant || !process.ProtectsProcess {
		t.Errorf("RTC-007: key %s, source %d, protects the process %v; want a constant on the process",
			process.Key, process.Source, process.ProtectsProcess)
	}
	if process.OffWith != "RTC-003" || process.OffWithBy != "issue 951" || !slices.Contains(process.Decided, "issue 951") ||
		process.Carries("F-03") {
		t.Errorf("RTC-007: off with %q by %q, decided %v, carries F-03 %v; want it off with RTC-003 by issue 951's decision",
			process.OffWith, process.OffWithBy, process.Decided, process.Carries("F-03"))
	}
	if refusal := process.Refusals[0]; refusal.At != entry.Refusals[0].At {
		t.Errorf("RTC-007 refuses at %v, RTC-003 at %v; want the same refusal, so a caller is not told others are listing",
			refusal.At, entry.Refusals[0].At)
	}
	if process.AtCapacity != RefuseNewcomer {
		t.Errorf("RTC-007 at capacity: %d, want it to refuse the newcomer and take nothing across keys", process.AtCapacity)
	}
	if FindingIssue("F-03") != 951 {
		t.Errorf("F-03 is filed as issue %d, want it kept as issue 951's", FindingIssue("F-03"))
	}
}

// TestDecisions_HeldRequests_AreBoundedOnTheProcessByIssue951 pins what issue
// 951 decided for the calls the process holds open: a ceiling keyed on the
// process that no operator sets, derived from the process's descriptor limit
// and nothing else, so no memory cap, and standing alone with no
// per-credential partner, refusing the newcomer on every channel its methods
// carry with words that say to retry later and charge nothing, and existing
// only over HTTP. Its gate refusal holds in both eras, since it answers a POST of
// 2026-07-28 and a POST that would open a stateful session with no slot left
// for its standalone stream, which only an earlier revision sends.
func TestDecisions_HeldRequests_AreBoundedOnTheProcessByIssue951(t *testing.T) {
	held, _ := Lookup("HLD-011")
	if !heldIsDecidedByIssue951(held) {
		t.Errorf("HLD-011: key %s, stdio %s, source %d, protects the process %v, partner %q, at capacity %d, "+
			"decided %v, findings %v, functions %v; want a derived value on the process alone, over HTTP only, "+
			"refusing the newcomer, decided by issue 951, carrying no finding and counting what MeterFor meters",
			held.Key, held.StdioKey, held.Source, held.ProtectsProcess, held.Partner, held.AtCapacity,
			held.Decided, held.Findings, held.Functions)
	}
	descriptors := []string{"HeldRequestDescriptors", "DescriptorSpareDivisor", "FallbackDescriptorLimit"}
	if !slices.Equal(held.Values, descriptors) {
		t.Errorf("HLD-011 values %v, want %v alone: issue 951 decided the server keeps no memory cap, "+
			"so the ceiling is sized from descriptors and the memory limit the process runs under bounds memory", held.Values, descriptors)
	}
	want := []struct {
		channel Channel
		code    int
		status  int
		retry   RetryAfter
	}{
		{Gate, CodeUnavailable, 503, RetryAfterFixed},
		{ToolError, 0, 0, RetryAfterNone},
		{RPC, CodeTooManyRequests, 0, RetryAfterNone},
		{EmptyCompletion, 0, 0, RetryAfterNone},
	}
	if len(held.Refusals) != len(want) {
		t.Fatalf("HLD-011 declares %d refusals, want %d", len(held.Refusals), len(want))
	}
	for i, w := range want {
		t.Run(w.channel.String(), func(t *testing.T) {
			r := held.Refusals[i]
			if r.Channel != w.channel || r.Code != w.code || r.Status != w.status || r.RetryAfter != w.retry ||
				r.Answer != RetryLater || len(r.Charged) != 0 {
				t.Errorf("HLD-011 refuses with %+v; want channel %s, code %d, status %d, retry later, charging nothing",
					r, w.channel, w.code, w.status)
			}
			if w.channel != EmptyCompletion && r.Prefix != "This server is busy." {
				t.Errorf("prefix %q, want words that name no bound", r.Prefix)
			}
		})
	}
	if gate := held.Refusals[0]; gate.Era != EraAny {
		t.Errorf("HLD-011's gate refusal holds in era %d, want both: a modern POST and a POST opening a stateful session", gate.Era)
	}
}

// TestDecisions_StatefulSessions_AreBoundedOnTheProcessByIssue951 pins what
// issue 951 decided for the stateful sessions, the half of F-31 HLD-011 left:
// HLD-010 is no longer the decision by absence that carried the finding but a
// ceiling of the same shape as HLD-011's, derived from it by the one value it
// names, refusing the newcomer in the gate with HLD-011's own refusal in the
// only era that has sessions, charging nothing: a POST of 2026-07-28 is left to
// the SDK, which closes its session with it. The owner records the sessions
// grow (IDN-010) carry the finding no longer either, since a session past the
// ceiling is refused before it is recorded, and no row carries F-31 now while
// it stays filed as issue 951's. No per-credential ceiling stands beside it,
// which issue 951 decided because a number on a credential multiplies with
// every token a caller mints, and no fixed cap stands beside its one value.
func TestDecisions_StatefulSessions_AreBoundedOnTheProcessByIssue951(t *testing.T) {
	sessions, _ := Lookup("HLD-010")
	held, _ := Lookup("HLD-011")
	if !heldIsDecidedByIssue951(withFunctions(sessions, held.Functions)) || len(sessions.Functions) != 0 {
		t.Errorf("HLD-010: key %s, stdio %s, source %d, protects the process %v, partner %q, at capacity %d, "+
			"decided %v, findings %v, functions %v; want HLD-011's shape, naming no function",
			sessions.Key, sessions.StdioKey, sessions.Source, sessions.ProtectsProcess, sessions.Partner,
			sessions.AtCapacity, sessions.Decided, sessions.Findings, sessions.Functions)
	}
	if !slices.Equal(sessions.Values, []string{"SessionHeldDivisor"}) || sessions.Disposition != Valued {
		t.Errorf("HLD-010 values %v, disposition %d; want SessionHeldDivisor alone, valued", sessions.Values, sessions.Disposition)
	}
	if len(sessions.Refusals) != 1 {
		t.Fatalf("HLD-010 declares %d refusals, want 1", len(sessions.Refusals))
	}
	refusal, gate := sessions.Refusals[0], held.Refusals[0]
	if refusal.Era != EraLegacy || refusal.Channel != gate.Channel || refusal.Code != gate.Code ||
		refusal.Status != gate.Status || refusal.RetryAfter != gate.RetryAfter || refusal.Prefix != gate.Prefix ||
		refusal.At != gate.At || len(refusal.Charged) != 0 {
		t.Errorf("HLD-010 refuses with %+v; want HLD-011's gate refusal %+v, in the era that has sessions", refusal, gate)
	}
	owners, _ := Lookup("IDN-010")
	if owners.Carries("F-31") || owners.AtCapacity != RefuseNewcomer || !slices.Contains(owners.Decided, "issue 951") {
		t.Errorf("IDN-010: findings %v, at capacity %d, decided %v; want no finding, the newcomer refused, "+
			"and issue 951 recorded", owners.Findings, owners.AtCapacity, owners.Decided)
	}
	for _, d := range Decisions() {
		t.Run(d.ID, func(t *testing.T) {
			if d.Carries("F-31") {
				t.Errorf("%s still carries F-31, which issue 951 answered", d.ID)
			}
		})
	}
	if FindingIssue("F-31") != 951 {
		t.Errorf("F-31 is filed as issue %d, want it kept as issue 951's", FindingIssue("F-31"))
	}
}

// TestDecisions_Initialize_StaysUnmeteredByIssue951 pins the other half of
// what issue 951 decided for the stateful sessions: initialize stays charged to
// no bucket (RTC-004) once HLD-010 bounds the sessions it opens, since a price
// in the opener's own rate would sit on a key a caller can mint and multiply
// with every token it mints, and the row records that decision.
func TestDecisions_Initialize_StaysUnmeteredByIssue951(t *testing.T) {
	unmetered, _ := Lookup("RTC-004")
	if !slices.Contains(unmetered.Decided, "issue 951") {
		t.Errorf("RTC-004 decided %v, want issue 951 recorded", unmetered.Decided)
	}
	if meter := MeterFor("initialize"); meter != Unmetered {
		t.Errorf("MeterFor(initialize) = %d, want Unmetered", meter)
	}
}

// withFunctions is d naming the register functions given, so a row that names
// none can be held to a predicate written for one that does.
func withFunctions(d Decision, functions []string) Decision {
	d.Functions = functions
	return d
}

// heldIsDecidedByIssue951 reports whether HLD-011 has the shape issue 951
// decided for it: a value derived on the process alone, with no per-credential
// partner (a credential is a key a caller can mint, so a number on it
// multiplies with every token the caller mints), over HTTP only, refusing the
// newcomer, carrying no finding and counting what MeterFor meters to an
// upstream.
func heldIsDecidedByIssue951(held Decision) bool {
	return held.Key == KeyProcess && held.StdioKey == KeyNone && held.Source == Derived && held.ProtectsProcess &&
		held.Partner == "" && held.AtCapacity == RefuseNewcomer && slices.Contains(held.Decided, "issue 951") &&
		len(held.Findings) == 0 && slices.Equal(held.Functions, []string{"MeterFor"})
}

// TestDecisions_TwoMCPClauses_AreDecidedByIssue959 pins what issue 959
// decided about the two clauses of MCP the server meets in part: the tool-call
// bucket stays off by default on stdio, and the response profile chosen from
// clientInfo stays as a deliberate deviation. Both are recorded as decisions
// on the rows they concern, which therefore no longer carry F-19 and F-33;
// the findings themselves stay in the register's list, filed as issue 959's,
// because a decision answers a finding rather than removing it from the
// specification. The two defaults the stdio position rests on are pinned with
// every other value, in TestValues_HoldTheirPins.
func TestDecisions_TwoMCPClauses_AreDecidedByIssue959(t *testing.T) {
	for _, tc := range []struct {
		id      string
		finding string
	}{
		{"RTC-001", "F-19"},
		{"IDN-013", "F-33"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			d, ok := Lookup(tc.id)
			if !ok {
				t.Fatalf("%s names no row", tc.id)
			}
			if !slices.Contains(d.Decided, "issue 959") || d.Carries(tc.finding) {
				t.Errorf("%s: decided %v, carries %s %v; want issue 959's decision recorded and %s no longer carried",
					tc.id, d.Decided, tc.finding, d.Carries(tc.finding), tc.finding)
			}
			if FindingIssue(tc.finding) != 959 {
				t.Errorf("%s is filed as issue %d, want it kept as issue 959's", tc.finding, FindingIssue(tc.finding))
			}
		})
	}
}

// TestDecisions_TierFallback_IsINV008sRecordedException holds issue 952's
// decision on F-09: unknown scopes resolve wide (ADM-003) and an unknown tier
// resolves narrow, to Free with an enterprise build's warning (AUT-003, issue
// 900), and the second is kept as INV-008's recorded exception. Both rows
// record the decision and carry F-09 no longer; AUT-003 keeps F-10, the tier
// narrowing that still answers as unknown, which is issue 956's.
func TestDecisions_TierFallback_IsINV008sRecordedException(t *testing.T) {
	for _, tc := range []struct {
		id      string
		decided []string
	}{
		{"ADM-003", []string{"ADR-0018", "issue 952"}},
		{"AUT-003", []string{"issue 900", "issue 952"}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			d, ok := Lookup(tc.id)
			if !ok {
				t.Fatalf("%s names no row", tc.id)
			}
			if !slices.Equal(d.Decided, tc.decided) || d.Carries("F-09") {
				t.Errorf("%s: decided %v, carries F-09 %v; want %v and F-09 no longer carried", tc.id, d.Decided, d.Carries("F-09"), tc.decided)
			}
		})
	}
	if tier, _ := Lookup("AUT-003"); !slices.Equal(tier.Findings, []string{"F-10"}) {
		t.Errorf("AUT-003 carries %v, want F-10 alone", tier.Findings)
	}
	if FindingIssue("F-09") != 952 {
		t.Errorf("F-09 is filed as issue %d, want it kept as issue 952's", FindingIssue("F-09"))
	}
}

// TestDecisions_ToolCallRefusal_AnswersF20UnderIssue961 pins what the work of
// issue 961 answered on RTC-001: the tools/call refusal a middleware makes now
// carries the resultType its revision requires, so the row records the issue
// and no longer carries F-20. F-21, which the same issue tracks, is still
// carried, since the issue stays open for it, and F-20 stays in the register's
// list, filed as issue 961's.
func TestDecisions_ToolCallRefusal_AnswersF20UnderIssue961(t *testing.T) {
	d, _ := Lookup("RTC-001")
	if !slices.Contains(d.Decided, "issue 961") || d.Carries("F-20") || !d.Carries("F-21") {
		t.Errorf("RTC-001: decided %v, findings %v; want issue 961 recorded, F-20 no longer carried and F-21 still carried",
			d.Decided, d.Findings)
	}
	if FindingIssue("F-20") != 961 {
		t.Errorf("F-20 is filed as issue %d, want it kept as issue 961's", FindingIssue("F-20"))
	}
}

// TestDecisions_OAuthVerification_IsBoundedByIssue950 pins what issue 950
// decided for the two things on the OAuth admission path that grew with what
// callers send. The identity cache (ADM-005) has a constant capacity and takes
// an expired or least recently used identity to hold a new one, which crosses
// keys and so records the decision; the verification's round trips (ADM-014)
// run under a ceiling keyed on the process that no operator can change, with
// slots of its own rather than the pool's probe slots (POL-006), and a request
// that waited in vain is told to retry and is charged nothing (its words are
// held by TestDecisions_ADM014_RefusesInADM002sWords). No row carries F-29 or
// F-30 any longer, and both stay filed as issue 950's.
func TestDecisions_OAuthVerification_IsBoundedByIssue950(t *testing.T) {
	cache, _ := Lookup("ADM-005")
	if !slices.Contains(cache.Values, "OAuthCacheCapacity") || cache.AtCapacity != EvictAcrossKeys ||
		!slices.Contains(cache.Decided, "issue 950") {
		t.Errorf("ADM-005: values %v, at capacity %d, decided %v; want OAuthCacheCapacity evicting across keys by issue 950",
			cache.Values, cache.AtCapacity, cache.Decided)
	}

	slots, _ := Lookup("ADM-014")
	if slots.Key != KeyProcess || slots.Source != Constant || !slots.ProtectsProcess || slots.AtCapacity != WaitThenRefuse ||
		!slices.Contains(slots.Decided, "issue 950") {
		t.Errorf("ADM-014: key %s, source %d, protects the process %v, at capacity %d, decided %v; "+
			"want a constant on the process that waits then refuses, by issue 950",
			slots.Key, slots.Source, slots.ProtectsProcess, slots.AtCapacity, slots.Decided)
	}
	for _, r := range slots.Refusals {
		if r.Answer != RetryLater || len(r.Charged) != 0 {
			t.Errorf("ADM-014 refuses with %s charging %v; want retry later, charging nothing", r.Answer, r.Charged)
		}
	}

	probes, _ := Lookup("POL-006")
	for _, value := range slots.Values {
		if slices.Contains(probes.Values, value) {
			t.Errorf("ADM-014 and POL-006 share %s; the verifier's slots are its own", value)
		}
	}

	for _, id := range []string{"ADM-002", "ADM-005", "ADM-014", "POL-006"} {
		t.Run(id, func(t *testing.T) {
			d, _ := Lookup(id)
			if d.Carries("F-29") || d.Carries("F-30") {
				t.Errorf("%s still carries %v", id, d.Findings)
			}
		})
	}
	for _, finding := range []string{"F-29", "F-30"} {
		t.Run(finding, func(t *testing.T) {
			if FindingIssue(finding) != 950 {
				t.Errorf("%s is filed as issue %d, want it kept as issue 950's", finding, FindingIssue(finding))
			}
		})
	}
}

// TestDecisions_ADM014_RefusesInADM002sWords pins the refusal of the
// verification ceiling to the one ADM-002 gives a verification with no verdict,
// field for field: the same function, status, code, text and Retry-After. The
// next action is the same, and a sentence of its own would tell a caller that
// others are verifying; a caller that knows the instance to be healthy can
// still infer that much from being refused at all, which is the one bit
// INV-019 accepts for a bound keyed on the process, and this is what keeps the
// wording from adding to it.
func TestDecisions_ADM014_RefusesInADM002sWords(t *testing.T) {
	slots, _ := Lookup("ADM-014")
	admission, _ := Lookup("ADM-002")
	shared := func(r Refusal) bool {
		return slices.ContainsFunc(admission.Refusals, func(a Refusal) bool {
			return a.At == r.At && a.Status == r.Status && a.Code == r.Code && a.Prefix == r.Prefix && a.RetryAfter == r.RetryAfter
		})
	}
	if len(slots.Refusals) != 1 || !shared(slots.Refusals[0]) {
		t.Errorf("ADM-014 refuses with %+v; want exactly one refusal, ADM-002's for a verification with no verdict, "+
			"so a caller is not told others are verifying", slots.Refusals)
	}
}

// findingsOfNoRow are the findings that record no row's departure, so no row
// has carried them since the register landed: F-18 is a budget GitLab.com
// keeps that nothing in the process accounts for, F-24 a message the SDK
// gives the server no way to send, and F-23 and F-27 are stale statements in
// comments and documents. Each is closed in its issue without a row changing.
var findingsOfNoRow = []string{"F-18", "F-23", "F-24", "F-27"}

// TestDecisions_AFindingNoRowCarries_IsAnsweredByItsIssue holds the rule the
// package comment states: a finding stops being carried only when its issue
// answers it, and the answer is recorded in the Decided of a row. A finding
// dropped from a row by mistake is carried by nothing and answered by nothing,
// and fails its subtest unless it is one of findingsOfNoRow, which a row must
// then not carry either. The answered set is named as well, because a row
// records the issue that decided and not the finding it answered, so a finding
// whose issue answered another one elsewhere (F-31 while issue 951 had
// answered only F-03 on RTC-007) would pass its subtest if it were dropped;
// the next finding answered joins that list in the change that answers it, as
// F-29 and F-30 did when issue 950 bounded the OAuth identity cache and
// verification, F-31 did when issue 951 bounded the stateful sessions, F-20
// did when the tool-call refusal was given its resultType under issue 961,
// F-17 did when issue 952 stopped misreading a fine-grained token, at the
// read-only narrowing and at both doors, F-09 did when issue 952 recorded
// the tier's Free fallback as INV-008's exception, and F-08 did when issue 952
// set the read_api minimum at the legacy door as well as the OAuth one.
func TestDecisions_AFindingNoRowCarries_IsAnsweredByItsIssue(t *testing.T) {
	carried := map[string]bool{}
	decided := map[string]bool{}
	for _, d := range Decisions() {
		for _, f := range d.Findings {
			carried[f] = true
		}
		for _, by := range d.Decided {
			decided[by] = true
		}
	}
	var answered []string
	for _, f := range AllFindings() {
		ofNoRow := slices.Contains(findingsOfNoRow, f.ID)
		if !carried[f.ID] && !ofNoRow {
			answered = append(answered, f.ID)
		}
		t.Run(f.ID, func(t *testing.T) {
			by := "issue " + strconv.Itoa(f.Issue)
			switch {
			case carried[f.ID] && ofNoRow:
				t.Errorf("%s is declared to record no row's departure, yet a row carries it", f.ID)
			case !carried[f.ID] && !ofNoRow && !decided[by]:
				t.Errorf("%s is carried by no row and no row records %s's decision; restore it to the rows it describes, or record the issue that answered it in their Decided", f.ID, by)
			}
		})
	}
	if got, want := strings.Join(answered, ","), "F-03,F-08,F-09,F-17,F-19,F-20,F-29,F-30,F-31,F-33"; got != want {
		t.Errorf("findings answered and carried by no row = %s, want %s", got, want)
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
	pinBusy     = "This server is busy."
	pinGrant    = "GitLab accepted this token and refused it the permission to read its own user."
	pinBelow    = "GitLab accepted this token, which carries neither the read_api nor the api scope"
	pinScope    = "GitLab rejected this token for lacking the scope"
)

// pinBelowMinimum is the 403 both doors give a credential GitLab accepted that
// carries neither read_api nor api (issue 952), the legacy gate's without a
// challenge and in its own words, the bearer guard's with one and in the words
// it gives a token GitLab refused its own user for want of a scope: uncharged,
// since the token is genuine.
func pinBelowMinimum(challenge bool) refusalPin {
	prefix := pinBelow
	if challenge {
		prefix = pinScope
	}
	return refusalPin{
		methods: "http", channel: Gate, code: -40300, status: 403, challenge: challenge,
		prefix: prefix, answer: WidenScope,
	}
}

// pinGrantRefusal is the 403 both doors give a credential GitLab accepted and
// refused the permission to read its own user, the bearer guard's with a
// challenge and the legacy gate's without: uncharged, since the token is
// genuine, and answered by widening what the credential may do.
func pinGrantRefusal(challenge bool) refusalPin {
	return refusalPin{
		methods: "http", channel: Gate, code: -40300, status: 403, challenge: challenge,
		prefix: pinGrant, answer: WidenScope,
	}
}

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

// rowPins is every row's frozen declaration: its question (spec: The
// five questions), kind, class (spec: Alignment classes), disposition, key
// and stdio key (spec: Keys), the unit its reason names and the unit of what
// it cites (spec: Alignment classes), and every refusal with its channel and
// answer (spec: Refusal channels).
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
			{methods: "completion/complete", channel: EmptyCompletion, answer: RetryLater},
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

		"ADM-001": {Admit, Rule, ClassC, Ruled, KeyEntry, KeyProcess, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -40100, status: 401, challenge: true, prefix: pinRejected,
				answer: Reauthorize, charged: pinCharged,
			},
			{
				methods: "http", channel: Gate, code: -50300, status: 503,
				prefix: "Could not initialize a GitLab session for this token.", answer: RetryLater,
			},
			pinGrantRefusal(false),
			pinBelowMinimum(false),
			{
				methods: "tools/list,tools/call,resources/list,resources/templates/list,resources/read,resources/subscribe," +
					"prompts/list,prompts/get,completion/complete,subscriptions/listen",
				era: EraStdio, channel: RPC, code: -40300,
				prefix: "GitLab accepted the token this server was started with", answer: WidenScope,
			},
		}},
		"ADM-002": {Admit, Rule, ClassC, Valued, KeyVerified, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -40100, status: 401, challenge: true, prefix: pinRejected,
				answer: Reauthorize, charged: pinCharged,
			},
			{methods: "http", channel: Gate, code: -40300, status: 403, challenge: true, answer: WidenScope},
			pinBelowMinimum(true),
			{
				methods: "http", channel: Gate, code: -50300, status: 503, retry: RetryAfterUpstreamOrFixed,
				prefix: pinRetry503 + ";", answer: RetryLater,
			},
			{
				methods: "http", channel: Gate, code: -50300, status: 503, retry: RetryAfterFixed,
				prefix: pinRetry503 + ".", answer: RetryLater,
			},
			pinGrantRefusal(true),
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
			pinGrantRefusal(false),
			pinGrantRefusal(true),
			pinBelowMinimum(false),
			pinBelowMinimum(true),
		}},
		"ADM-007": {Admit, Rule, ClassC, Ruled, KeySession, KeyNone, KeyNone, KeyNone, []refusalPin{
			{
				methods: "http", channel: Gate, code: -32600, status: 404,
				prefix: "This session does not belong to the presented credential.", answer: StartOver,
			},
		}},
		"ADM-008": {Admit, Lifetime, ClassC, Valued, KeyEntry, KeyNone, KeyNone, KeyNone, []refusalPin{
			pinListenEnd("credential_reset", StartOver),
		}},
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
		"ADM-014": {Admit, Ceiling, ClassP, Valued, KeyProcess, KeyNone, KeyNone, KeyProcess, []refusalPin{
			{
				methods: "http", channel: Gate, code: -50300, status: 503, retry: RetryAfterFixed,
				prefix: "GitLab could not verify this token right now.", answer: RetryLater,
			},
		}},
		"AUB-001": {Admit, Budget, ClassA, Valued, KeyAddress, KeyNone, KeyNone, KeyNone, pinBlockedAt()},
		"AUB-002": {Admit, Budget, ClassA, Valued, KeySource, KeyNone, KeyNone, KeyNone, pinBlockedAt()},
		"AUB-003": {Admit, Budget, ClassA, Valued, KeyAddress, KeyNone, KeyNone, KeyNone, pinBlockedAt()},
		"AUB-004": {Admit, Ceiling, ClassP, Valued, KeyProcess, KeyNone, KeyNone, KeyProcess, []refusalPin{
			{methods: "http", channel: Silent, answer: NoAnswer},
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
		"AUT-007": {Authorize, Rule, ClassC, Ruled, KeyEntry, KeyProcess, KeyNone, KeyNone, []refusalPin{
			{
				methods: "tools/call", channel: Withheld,
				prefix: "exists but is not available to a fine-grained personal access token", answer: WidenScope,
			},
			{methods: "tools/list", channel: Absent, answer: WidenScope},
		}},
		"AUT-008": {Authorize, Rule, ClassC, Ruled, KeyEntry, KeyProcess, KeyNone, KeyNone, []refusalPin{
			{
				methods: "tools/call", channel: Withheld,
				prefix: "exists but this fine-grained personal access token was not granted", answer: WidenScope,
			},
			{methods: "tools/list", channel: Absent, answer: WidenScope},
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
		"RTC-003": {Allow, Rate, ClassR, Valued, KeyEntry, KeyProcess, KeyProcess, KeyProcess, []refusalPin{
			{methods: "tools/list", channel: RPC, code: -42900, prefix: pinRate, answer: RetryLater},
		}},
		"RTC-004": {Allow, Rule, ClassP, Promoted, KeyRequest, KeyRequest, KeyNone, KeyNone, nil},
		"RTC-005": {Allow, Rate, ClassD, Valued, KeyEntry, KeyProcess, KeyTenant, KeyTenant, []refusalPin{
			{methods: pinSubMeths, channel: RPC, code: -32000, prefix: "subscriptions: rate limited", answer: RetryLater},
			{methods: "notifications/resources/updated", channel: Silent, answer: NoAnswer},
		}},
		"RTC-006": {Allow, Bound, ClassQ, Valued, KeyRequest, KeyRequest, KeyNone, KeyNone, nil},
		"RTC-007": {Allow, Rate, ClassP, Valued, KeyProcess, KeyProcess, KeyProcess, KeyProcess, []refusalPin{
			{methods: "tools/list", channel: RPC, code: -42900, prefix: pinRate, answer: RetryLater},
		}},
		"HLD-001": {Allow, Ceiling, ClassR, Valued, KeyEntry, KeyProcess, KeyEntry, KeyEntry, []refusalPin{
			{methods: "subscriptions/listen", era: EraModern, channel: RPC, code: -32000, prefix: pinListen, answer: RetryLater},
		}},
		"HLD-002": {Allow, Ceiling, ClassP, Valued, KeyProcess, KeyProcess, KeyProcess, KeyProcess, []refusalPin{
			{methods: "subscriptions/listen", era: EraModern, channel: RPC, code: -32000, prefix: pinListen, answer: RetryLater},
		}},
		"HLD-003": {Allow, Ceiling, ClassD, Valued, KeyEntry, KeyProcess, KeyTenant, KeyTenant, []refusalPin{
			{methods: pinSubMeths, channel: RPC, code: -32000, prefix: pinWatchers, answer: RetryLater},
			pinListenEnd("watcher_evicted", StartOver),
		}},
		"HLD-004": {Allow, Ceiling, ClassP, Valued, KeyProcess, KeyProcess, KeyProcess, KeyProcess, []refusalPin{
			{methods: pinSubMeths, channel: RPC, code: -32000, prefix: pinWatchers, answer: RetryLater},
		}},
		"HLD-007": {Allow, Lifetime, ClassQ, Valued, KeyRequest, KeyRequest, KeyNone, KeyNone, []refusalPin{
			pinListenEnd("lifetime_reached", StartOver),
		}},
		"HLD-010": {Allow, Ceiling, ClassP, Valued, KeyProcess, KeyNone, KeyProcess, KeyProcess, []refusalPin{
			{
				methods: "http", era: EraLegacy, channel: Gate, code: -50300, status: 503, retry: RetryAfterFixed,
				prefix: pinBusy, answer: RetryLater,
			},
		}},
		"HLD-011": {Allow, Ceiling, ClassP, Valued, KeyProcess, KeyNone, KeyProcess, KeyProcess, []refusalPin{
			{
				methods: "http", channel: Gate, code: -50300, status: 503, retry: RetryAfterFixed,
				prefix: pinBusy, answer: RetryLater,
			},
			{methods: "tools/call", channel: ToolError, prefix: pinBusy, answer: RetryLater},
			{
				methods: "resources/read,resources/subscribe,prompts/get", channel: RPC, code: -42900,
				prefix: pinBusy, answer: RetryLater,
			},
			{methods: "completion/complete", channel: EmptyCompletion, answer: RetryLater},
		}},
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
		"RQB-008", "RQB-009", "RQB-010", "RQB-011",
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
