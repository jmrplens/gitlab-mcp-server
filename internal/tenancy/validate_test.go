package tenancy

import (
	"errors"
	"strings"
	"testing"
)

// ruleCase is one synthetic row put to one rule: the rule must accept it when
// refused is empty, and must refuse it with a detail containing refused
// otherwise.
type ruleCase struct {
	name    string
	row     Decision
	refused string
}

// row is a minimal row every rule accepts, for a case to change.
func row(change func(d *Decision)) Decision {
	d := Decision{ID: "TST-001", Question: Allow, Kind: Rule, Class: ClassC, Disposition: Ruled, Key: KeyEntry}
	if change != nil {
		change(&d)
	}
	return d
}

// ruleContext is the register a cross-row rule consults: one constant ceiling
// on the process, one configurable one, one ceiling on the entry, one budget
// on an address, one on the entry, one rule, and one rate on the entry whose
// process partner is the row a case puts to the rule.
func ruleContext() map[string]Decision {
	return map[string]Decision{
		"PROC":    {ID: "PROC", Kind: Ceiling, Key: KeyProcess, Source: Constant},
		"CONF":    {ID: "CONF", Kind: Ceiling, Key: KeyProcess, Source: Configurable},
		"ENTRY":   {ID: "ENTRY", Kind: Ceiling, Key: KeyEntry, Source: Constant},
		"BUDGET":  {ID: "BUDGET", Kind: Budget, Key: KeyAddress},
		"OWNBGT":  {ID: "OWNBGT", Kind: Budget, Key: KeyEntry},
		"RULE":    {ID: "RULE", Kind: Rule, Key: KeyAddress},
		"PAIRED":  {ID: "PAIRED", Kind: Rate, Key: KeyEntry, Partner: "TST-001"},
		"UNPAIRD": {ID: "UNPAIRD", Kind: Rate, Key: KeyEntry, Partner: "PROC"},
	}
}

// runRule puts every case to check.
func runRule(t *testing.T, check func(Decision, map[string]Decision) []string, cases []ruleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(check(tc.row, ruleContext()), "\n")
			switch {
			case tc.refused == "" && got != "":
				t.Errorf("refused an acceptable row: %s", got)
			case tc.refused != "" && !strings.Contains(got, tc.refused):
				t.Errorf("got %q, want a refusal containing %q", got, tc.refused)
			}
		})
	}
}

// TestValidate_ShareOnMintable refuses a share on any key a caller can mint,
// the entry and the tenant alike, and quotes the key's evidence when it does.
func TestValidate_ShareOnMintable(t *testing.T) {
	runRule(t, checkShareOnMintable, []ruleCase{
		{"a ceiling on the entry", row(func(d *Decision) { d.Kind = Ceiling }), ""},
		{"a share on the process", row(func(d *Decision) { d.Kind, d.Key = Share, KeyProcess }), ""},
		{"a share on the entry", row(func(d *Decision) { d.Kind = Share }), KeyEntry.Evidence()},
		{"a share on the tenant", row(func(d *Decision) { d.Kind, d.Key = Share, KeyTenant }), KeyTenant.Evidence()},
	})
}

// TestValidate_ProcessPartner holds a per-caller number protecting a process
// resource to a constant process partner, or to a finding. A caller's key
// before admission is held as one after it is: an address is as mintable as a
// credential, so a number on it multiplies with every address a caller holds,
// and RTC-008 passes only by the finding issue 1053 recorded. A bound on one
// request is no caller's holding and needs no partner.
func TestValidate_ProcessPartner(t *testing.T) {
	protects := func(d *Decision) { d.Kind, d.ProtectsProcess = Ceiling, true }
	onAnAddress := func(d *Decision) { protects(d); d.Kind, d.Class, d.Key = Rate, ClassA, KeyAddress }
	runRule(t, checkProcessPartner, []ruleCase{
		{"no partner", row(protects), "no process partner"},
		{"no partner, recorded", row(func(d *Decision) { protects(d); d.Findings = []string{"F-03"} }), ""},
		{"no partner on an address", row(onAnAddress), "no process partner"},
		{"no partner on an address, recorded", row(func(d *Decision) { onAnAddress(d); d.Findings = []string{"F-36"} }), ""},
		{"no partner on a request", row(func(d *Decision) { protects(d); d.Key = KeyRequest }), ""},
		{"a constant process partner", row(func(d *Decision) { protects(d); d.Partner = "PROC" }), ""},
		{"a partner on the entry", row(func(d *Decision) { protects(d); d.Partner = "ENTRY" }), "not a constant keyed on the process"},
		{"a configurable partner", row(func(d *Decision) { protects(d); d.Partner = "CONF" }), "not a constant keyed on the process"},
		{"protecting the process on the process", row(func(d *Decision) { protects(d); d.Key = KeyProcess }), ""},
		{"a process reason not said to protect it", row(func(d *Decision) { d.ReasonUnit = KeyProcess }), "does not say it protects one"},
	})
}

// TestValidate_ProcessAbsent holds a ceiling nothing bounds to a finding.
func TestValidate_ProcessAbsent(t *testing.T) {
	runRule(t, checkProcessAbsent, []ruleCase{
		{"a ceiling nothing bounds", row(func(d *Decision) { d.Kind = Ceiling }), "nothing bounds"},
		{"recorded", row(func(d *Decision) { d.Kind, d.Findings = Ceiling, []string{"F-31"} }), ""},
		{"a constant ceiling", row(func(d *Decision) { d.Kind, d.Source = Ceiling, Constant }), ""},
		{"a rule with no value", row(nil), ""},
	})
}

// TestValidate_AcrossKeys holds taking a holding across keys to a recorded
// decision.
func TestValidate_AcrossKeys(t *testing.T) {
	runRule(t, checkAcrossKeys, []ruleCase{
		{"undecided", row(func(d *Decision) { d.AtCapacity = EvictAcrossKeys }), "no recorded decision"},
		{"decided", row(func(d *Decision) { d.AtCapacity, d.Decided = EvictAcrossKeys, []string{"ADR-0020"} }), ""},
		{"refusing the newcomer", row(func(d *Decision) { d.AtCapacity = RefuseNewcomer }), ""},
	})
}

// TestValidate_ChannelCarried walks every channel and code INV-011 refuses.
func TestValidate_ChannelCarried(t *testing.T) {
	with := func(r Refusal, findings ...string) Decision {
		return row(func(d *Decision) { d.Refusals, d.Findings = []Refusal{r}, findings })
	}
	rpc := func(code int) Refusal {
		return Refusal{Methods: []string{"resources/read"}, Channel: RPC, Code: code, Answer: RetryLater}
	}
	gate := func(status, code int, challenge bool) Refusal {
		return Refusal{Methods: []string{MethodGate}, Channel: Gate, Status: status, Code: code, Challenge: challenge, Answer: RetryLater}
	}
	runRule(t, checkChannelCarried, []ruleCase{
		{"an in-band 429", with(rpc(CodeTooManyRequests)), ""},
		{"no method", with(Refusal{Channel: RPC, Code: CodeTooManyRequests}), "names no method"},
		{"a listing refused as a result", with(Refusal{Methods: []string{"tools/list"}, Channel: ToolError}), "not carried for tools/list"},
		{"a status inside the SDK", with(Refusal{Methods: []string{"tools/call"}, Channel: ToolError, Status: 429}), "carries none"},
		{"a Retry-After inside the SDK", with(Refusal{Methods: []string{"tools/call"}, Channel: ToolError, RetryAfter: RetryAfterFixed}), "carries none"},
		{"a challenge inside the SDK", with(Refusal{Methods: []string{"tools/call"}, Channel: ToolError, Challenge: true}), "carries none"},
		{"a code on a tool error", with(Refusal{Methods: []string{"tools/call"}, Channel: ToolError, Code: 5}), "code 5 on a channel"},
		{"a mirrored gate code", with(gate(429, CodeTooManyRequests, false)), ""},
		{"a gate code that mirrors nothing", with(gate(429, CodeUnavailable, false)), "does not mirror status 429"},
		{"a 400 with invalid request", with(gate(400, codeInvalidRequest, false)), ""},
		{"a 404 with invalid request", with(gate(404, codeInvalidRequest, false)), ""},
		{"a 403 with invalid request", with(gate(403, codeInvalidRequest, false)), "does not mirror status 403"},
		{"a 404 with another code", with(gate(404, CodeUnavailable, false)), "does not mirror status 404"},
		{"a 400 that mirrors its status", with(gate(400, -40000, false)), ""},
		{"a 401 with a challenge", with(gate(401, CodeUnauthorized, true)), ""},
		{"a 401 without a challenge", with(gate(401, CodeUnauthorized, false)), "without a WWW-Authenticate"},
		{"no code", with(rpc(0)), "code 0"},
		{"method not found", with(rpc(codeMethodNotFound)), "-32601"},
		{"a reserved code MCP does not define", with(rpc(-32042)), "reserves and does not define"},
		{"the bottom of MCP's range", with(rpc(-32099)), "reserves and does not define"},
		{"the bottom of the legacy range", with(rpc(-32019)), "legacy range"},
		{"a reserved code MCP defines", with(rpc(codeUnsupportedProtocolVersion)), ""},
		{"the legacy range", with(rpc(-32000)), "legacy range"},
		// No finding excuses it any longer: issue 956 retired -32000 from
		// every refusal, and F-07, which used to, is answered.
		{"the legacy range, even recorded", with(rpc(-32000), "F-07"), "legacy range"},
	})
}

// TestValidate_CodeRanges pins the two sub-ranges of JSON-RPC's
// implementation-defined errors at their edges: MCP reserves -32020 to -32099,
// and -32000 to -32019 is the legacy range.
func TestValidate_CodeRanges(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     int
		reserved bool
		legacy   bool
	}{
		{"just above the legacy range", -31999, false, false},
		{"the top of the legacy range", -32000, false, true},
		{"the bottom of the legacy range", -32019, false, true},
		{"the top of MCP's range", -32020, true, false},
		{"inside MCP's range", -32042, true, false},
		{"the bottom of MCP's range", -32099, true, false},
		{"just below MCP's range", -32100, false, false},
		{"a mirrored gate code", CodeTooManyRequests, false, false},
		{"a positive code", 32042, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reservedForMCP(tc.code); got != tc.reserved {
				t.Errorf("reservedForMCP(%d) = %v, want %v", tc.code, got, tc.reserved)
			}
			if got := inLegacyRange(tc.code); got != tc.legacy {
				t.Errorf("inLegacyRange(%d) = %v, want %v", tc.code, got, tc.legacy)
			}
		})
	}
}

// TestValidate_ZeroStated holds a valued row to a zero meaning, and a zero
// that is not "off" to a finding. A process partner that a recorded decision
// switches off with the row it stands beside passes without one, and nothing
// else that follows another row does: not a partner nobody decided, not one
// whose decisions are about something else, not one naming a decision it does
// not record, not a decided row following a row it does not partner, and not
// one following a row that names another partner.
//
// A zero refused at startup or replaced by the default passes without a
// finding too when the record its ZeroBy names decided it and the row records
// that decision (issue 958), whether it is the row's own zero or the zero of
// one of its settings, and not when the row records something else. A ZeroBy
// on a row whose every zero means off decides nothing, and is refused.
func TestValidate_ZeroStated(t *testing.T) {
	valued := func(d *Decision) { d.Disposition, d.Values = Valued, []string{"V"} }
	follows := func(offWith, by string, decided ...string) func(d *Decision) {
		return func(d *Decision) {
			valued(d)
			d.Key, d.Zero, d.OffWith, d.OffWithBy, d.Decided = KeyProcess, ZeroNotApplicable, offWith, by, decided
		}
	}
	decidedZero := func(zero Zero, settings []SettingZero, by string, decided ...string) func(d *Decision) {
		return func(d *Decision) {
			valued(d)
			d.Config = []string{"A", "B"}
			d.Zero, d.SettingZeros, d.ZeroBy, d.Decided = zero, settings, by, decided
		}
	}
	refusedBurst := []SettingZero{{Config: "B", Zero: ZeroRefused}}
	runRule(t, checkZeroStated, []ruleCase{
		{"a valued row with no zero meaning", row(valued), "does not say what zero means"},
		{"zero switches it off", row(func(d *Decision) { valued(d); d.Zero = ZeroOff }), ""},
		{"zero refuses startup", row(func(d *Decision) { valued(d); d.Zero = ZeroRefused }), "does not mean off"},
		{"zero selects the default", row(func(d *Decision) { valued(d); d.Zero = ZeroSelectsDefault }), "does not mean off"},
		{"another row's zero", row(func(d *Decision) { valued(d); d.Zero, d.OffWith = ZeroOff, "PROC" }), "does not mean off"},
		{"recorded", row(func(d *Decision) { valued(d); d.Zero, d.Findings = ZeroRefused, []string{"F-34"} }), ""},
		{"zero refuses startup, decided", row(decidedZero(ZeroRefused, nil, "issue 958", "issue 958")), ""},
		{"zero selects the default, decided", row(decidedZero(ZeroSelectsDefault, nil, "issue 958", "ADR-0020", "issue 958")), ""},
		{"zero refuses startup, undecided", row(decidedZero(ZeroRefused, nil, "")), "does not mean off"},
		{
			"zero refuses startup, by a decision it does not record",
			row(decidedZero(ZeroRefused, nil, "issue 958", "ADR-0020")), "does not mean off",
		},
		{"a setting's zero refuses startup", row(decidedZero(ZeroOff, refusedBurst, "")), "does not mean off"},
		{
			"a setting's zero selects the default",
			row(decidedZero(ZeroOff, []SettingZero{{Config: "B", Zero: ZeroSelectsDefault}}, "")), "does not mean off",
		},
		{"a setting's zero refuses startup, decided", row(decidedZero(ZeroOff, refusedBurst, "issue 958", "issue 958")), ""},
		{
			"zero refuses startup beside a setting it switches off, undecided",
			row(decidedZero(ZeroRefused, []SettingZero{{Config: "B", Zero: ZeroOff}}, "")), "does not mean off",
		},
		{
			"zero refuses startup beside a setting it switches off, decided",
			row(decidedZero(ZeroRefused, []SettingZero{{Config: "B", Zero: ZeroOff}}, "issue 958", "issue 958")), "",
		},
		{
			"a setting's zero refuses startup, recorded",
			row(func(d *Decision) { decidedZero(ZeroOff, refusedBurst, "")(d); d.Findings = []string{"F-34"} }), "",
		},
		{"a decision for a zero that means off", row(decidedZero(ZeroOff, nil, "issue 958", "issue 958")), "every zero it states means off"},
		{
			"a decision for a setting's zero that means off",
			row(decidedZero(ZeroOff, []SettingZero{{Config: "B", Zero: ZeroOffKeepsCount}}, "issue 958", "issue 958")),
			"every zero it states means off",
		},
		{"a ruled row", row(nil), ""},
		{"a partner following its row, decided", row(follows("PAIRED", "issue 951", "issue 951")), ""},
		{"a partner following its row, undecided", row(follows("PAIRED", "")), "does not mean off"},
		{
			"a partner following its row, decided about something else",
			row(follows("PAIRED", "", "ADR-0020")), "does not mean off",
		},
		{
			"a partner naming a decision it does not record",
			row(follows("PAIRED", "issue 951", "ADR-0020")), "does not mean off",
		},
		{"a decided row following a row it does not partner", row(follows("PROC", "issue 951", "issue 951")), "does not mean off"},
		{
			"a decided row following a row partnered elsewhere",
			row(follows("UNPAIRD", "issue 951", "issue 951")), "does not mean off",
		},
	})
}

// TestValidate_UnitsAgree holds a class D disagreement to a finding.
func TestValidate_UnitsAgree(t *testing.T) {
	disagreeing := func(d *Decision) { d.Kind, d.ReasonUnit = Ceiling, KeyTenant }
	runRule(t, checkUnitsAgree, []ruleCase{
		{"a per-user reason on the entry", row(disagreeing), "its reason is about the tenant and its key is the entry"},
		{"recorded", row(func(d *Decision) { disagreeing(d); d.Findings = []string{"F-05"} }), ""},
		{"agreeing", row(func(d *Decision) { d.Kind, d.ReasonUnit = Ceiling, KeyEntry }), ""},
	})
}

// TestValidate_StatedUnit holds a reason that misstates the unit of what it
// cites to a finding.
func TestValidate_StatedUnit(t *testing.T) {
	misstated := func(d *Decision) { d.StatedUnit, d.ReasonUnit = KeyCredential, KeyTenant }
	runRule(t, checkStatedUnit, []ruleCase{
		{"a per-token reason for a per-user budget", row(misstated), "names the credential while what it cites is kept per tenant"},
		{"recorded", row(func(d *Decision) { misstated(d); d.Findings = []string{"F-02"} }), ""},
		{"no stated unit", row(func(d *Decision) { d.ReasonUnit = KeyTenant }), ""},
		{"the same unit", row(func(d *Decision) { d.StatedUnit, d.ReasonUnit = KeyEntry, KeyCredential }), ""},
	})
}

// TestValidate_ClassConsistent holds the class to the facts it follows from.
func TestValidate_ClassConsistent(t *testing.T) {
	disagreeing := func(d *Decision) { d.Kind, d.ReasonUnit = Ceiling, KeyTenant }
	runRule(t, checkClassConsistent, []ruleCase{
		{"class D that disagrees", row(func(d *Decision) { disagreeing(d); d.Class = ClassD }), ""},
		{"class D that agrees", row(func(d *Decision) { d.Class = ClassD }), "class D and Disagrees differ"},
		{"class R that disagrees", row(func(d *Decision) { disagreeing(d); d.Class = ClassR }), "class D and Disagrees differ"},
		{"class A before admission", row(func(d *Decision) { d.Class, d.Key = ClassA, KeyAddress }), ""},
		{"class A after admission", row(func(d *Decision) { d.Class = ClassA }), "class A on the entry key"},
		{"class P on the process", row(func(d *Decision) { d.Class, d.Key = ClassP, KeyProcess }), ""},
		{"class P on a request", row(func(d *Decision) { d.Class, d.Key = ClassP, KeyRequest }), ""},
		{"class P on a requester", row(func(d *Decision) { d.Class = ClassP }), "class P on the entry key"},
	})
}

// TestValidate_ThroughConfig holds a configurable value to the configuration
// package's rule, and every other way in to a finding.
func TestValidate_ThroughConfig(t *testing.T) {
	configurable := func(d *Decision) {
		d.Source, d.Flags, d.Envs, d.Malformed = Configurable, []string{"--x"}, []string{"GITLAB_MCP_X"}, RefuseStartup
	}
	runRule(t, checkThroughConfig, []ruleCase{
		{"configurable through the package", row(configurable), ""},
		{"no flag", row(func(d *Decision) { configurable(d); d.Flags = nil }), "no flag"},
		{"no variable", row(func(d *Decision) { configurable(d); d.Envs = nil }), "no GITLAB_MCP_ variable"},
		{"no malformed policy", row(func(d *Decision) { configurable(d); d.Malformed = MalformedNone }), "no malformed-value policy"},
		{"an unprefixed variable", row(func(d *Decision) { d.Envs = []string{"MAX_THINGS"} }), "MAX_THINGS does not carry"},
		{"an environment variable alone", row(func(d *Decision) { d.Source = EnvOnly }), "outside the configuration package"},
		{"an environment variable alone, recorded", row(func(d *Decision) { d.Source, d.Findings = EnvOnly, []string{"F-13"} }), ""},
		// F-16 recorded bounds with no reason and defaults written twice, and
		// no longer the value only a Go option reached, so it excuses no value
		// only an environment variable reaches.
		{
			"an environment variable alone, carrying F-16",
			row(func(d *Decision) { d.Source, d.Findings = EnvOnly, []string{"F-16"} }), "outside the configuration package",
		},
		// A value only a Go option reaches is no operator's configuration:
		// checkFixedReasoned holds it to its reason, as it holds a constant.
		{"a Go option alone", row(func(d *Decision) { d.Source = OptionOnly }), ""},
		{"a constant", row(func(d *Decision) { d.Source = Constant }), ""},
	})
}

// TestValidate_MalformedDecided holds a malformed value that does not refuse
// startup to a recorded decision or a finding (issue 957): warning and keeping
// the default, skipping an entry and reading the value as off each pass when
// the record MalformedBy names decided it and the row records that decision,
// or when the row carries the finding that records the departure from
// INV-017, and not when the row records something else. Refusing startup, a
// switch every value of which means something and a value no operator writes
// depart from nothing, and a MalformedBy on any of them decides nothing, so it
// is refused.
func TestValidate_MalformedDecided(t *testing.T) {
	policy := func(m Malformed, by string, decided ...string) func(d *Decision) {
		return func(d *Decision) { d.Malformed, d.MalformedBy, d.Decided = m, by, decided }
	}
	runRule(t, checkMalformedDecided, []ruleCase{
		{"refusing startup", row(policy(RefuseStartup, "")), ""},
		{"accepting any value", row(policy(AcceptsAny, "")), ""},
		{"no policy", row(nil), ""},
		{"warning and keeping the default, undecided", row(policy(WarnKeepDefault, "")), "does not refuse startup, with no decision or finding"},
		{"skipping an entry, undecided", row(policy(SkipEntry, "")), "does not refuse startup, with no decision or finding"},
		{"reading as off, undecided", row(policy(ReadsAsOff, "")), "does not refuse startup, with no decision or finding"},
		{"warning and keeping the default, decided", row(policy(WarnKeepDefault, "issue 957", "issue 957")), ""},
		{"skipping an entry, decided", row(policy(SkipEntry, "issue 957", "issue 957")), ""},
		{"reading as off, decided", row(policy(ReadsAsOff, "issue 957", "ADR-0022", "issue 957")), ""},
		{
			"warning and keeping the default, by a decision it does not record",
			row(policy(WarnKeepDefault, "issue 957", "ADR-0022")), "does not refuse startup, with no decision or finding",
		},
		{
			"warning and keeping the default, decided about something else",
			row(policy(WarnKeepDefault, "", "issue 957")), "does not refuse startup, with no decision or finding",
		},
		{
			"warning and keeping the default, recorded",
			row(func(d *Decision) { d.Malformed, d.Findings = WarnKeepDefault, []string{"F-13"} }), "",
		},
		// F-34 records a departure from INV-015, which says nothing about a
		// value that does not parse.
		{
			"warning and keeping the default, carrying a finding about zero",
			row(func(d *Decision) { d.Malformed, d.Findings = WarnKeepDefault, []string{"F-34"} }),
			"does not refuse startup, with no decision or finding",
		},
		{
			"a decision for a value that refuses startup",
			row(policy(RefuseStartup, "issue 957", "issue 957")), "decided by issue 957, and a malformed value refuses startup or is none",
		},
		{
			"a decision for a switch every value of which means something",
			row(policy(AcceptsAny, "issue 957", "issue 957")), "decided by issue 957, and a malformed value refuses startup or is none",
		},
		{
			"a decision for no policy",
			row(policy(MalformedNone, "issue 957", "issue 957")), "decided by issue 957, and a malformed value refuses startup or is none",
		},
	})
}

// TestValidate_FixedReasoned holds a value no operator setting reaches, a
// constant or a Go option's, to saying why (issue 958), and refuses that
// reason on a value a setting reaches or one derived from another's.
func TestValidate_FixedReasoned(t *testing.T) {
	from := func(source Source, fixed string) func(d *Decision) {
		return func(d *Decision) { d.Source, d.Fixed = source, fixed }
	}
	runRule(t, checkFixedReasoned, []ruleCase{
		{"a constant with its reason", row(from(Constant, "why")), ""},
		{"a constant with none", row(from(Constant, "")), "no reason it is fixed"},
		{"a Go option with its reason", row(from(OptionOnly, "why")), ""},
		{"a Go option with none", row(from(OptionOnly, "")), "no reason it is fixed"},
		{"a configurable value", row(from(Configurable, "")), ""},
		{"a configurable value said to be fixed", row(from(Configurable, "why")), "is not a constant or a Go option's"},
		{"an environment variable said to be fixed", row(from(EnvOnly, "why")), "is not a constant or a Go option's"},
		{"a derived value said to be fixed", row(from(Derived, "why")), "is not a constant or a Go option's"},
		{"no value", row(nil), ""},
	})
}

// TestValidate_CapacityStated holds a ceiling with a value to saying what
// happens when it is full.
func TestValidate_CapacityStated(t *testing.T) {
	runRule(t, checkCapacityStated, []ruleCase{
		{"silent at capacity", row(func(d *Decision) { d.Kind, d.Source = Ceiling, Constant }), "does not say what happens"},
		{"refusing the newcomer", row(func(d *Decision) { d.Kind, d.Source, d.AtCapacity = Ceiling, Constant, RefuseNewcomer }), ""},
		{"a ceiling nothing bounds", row(func(d *Decision) { d.Kind = Ceiling }), ""},
		{"a rule", row(func(d *Decision) { d.Source = Constant }), ""},
	})
}

// TestValidate_CapacityFits puts each capacity behavior to the rule issue 958
// stated for when it applies: only what can fill states one; a wait is a
// ceiling's; reclaiming is from a key a caller holds; the oldest record is
// evicted only from a table kept before admission whose records have a
// lifetime or a refill; counting stops only where the newcomer it no longer
// counts is let through silently; and a holding is taken across keys only
// from a table, whose decision checkAcrossKeys asks for.
func TestValidate_CapacityFits(t *testing.T) {
	at := func(kind Kind, key Key, table bool, capacity Capacity) func(d *Decision) {
		return func(d *Decision) { d.Kind, d.Key, d.Table, d.AtCapacity = kind, key, table, capacity }
	}
	refusing := func(change func(d *Decision), channels ...Channel) func(d *Decision) {
		return func(d *Decision) {
			change(d)
			for _, c := range channels {
				d.Refusals = append(d.Refusals, Refusal{Methods: []string{MethodGate}, Channel: c, Answer: NoAnswer})
			}
		}
	}
	runRule(t, checkCapacityFits, []ruleCase{
		{"no capacity", row(nil), ""},
		{"a ceiling refusing the newcomer", row(at(Ceiling, KeyProcess, false, RefuseNewcomer)), ""},
		{"a rate refusing the newcomer", row(at(Rate, KeyProcess, false, RefuseNewcomer)), ""},
		{"a table refusing the newcomer", row(at(Rule, KeySession, true, RefuseNewcomer)), ""},
		{"a rule refusing the newcomer", row(at(Rule, KeySession, false, RefuseNewcomer)), "holds nothing that fills"},
		{"a lifetime that is no table", row(at(Lifetime, KeyEntry, false, EvictOldest)), "holds nothing that fills"},
		{"a ceiling waiting for a slot", row(at(Ceiling, KeyProcess, false, WaitThenRefuse)), ""},
		{"a rate waiting for room", row(at(Rate, KeyProcess, false, WaitThenRefuse)), "waits for room, and is not a ceiling"},
		{"reclaiming on the entry", row(at(Ceiling, KeyEntry, true, ReclaimOwnThenRefuse)), ""},
		{"reclaiming on a source", row(at(Budget, KeySource, true, ReclaimOwnThenRefuse)), ""},
		{"reclaiming on the process", row(at(Ceiling, KeyProcess, false, ReclaimOwnThenRefuse)), "on the process key, which no caller holds"},
		{"reclaiming on a request", row(at(Ceiling, KeyRequest, false, ReclaimOwnThenRefuse)), "on the request key, which no caller holds"},
		{"evicting a refused token's oldest record", row(at(Lifetime, KeyRefused, true, EvictOldest)), ""},
		{"evicting an address's oldest bucket", row(at(Rate, KeyAddress, true, EvictOldest)), ""},
		{
			"evicting the oldest verified identity",
			row(at(Lifetime, KeyVerified, true, EvictOldest)), "on the verified key, where a record is a holding",
		},
		{"evicting the oldest count of a budget", row(at(Budget, KeyAddress, true, EvictOldest)), "a lifetime or a refill"},
		{"evicting the oldest bucket of no table", row(at(Rate, KeyAddress, false, EvictOldest)), "a lifetime or a refill"},
		{"stopping counting silently", row(refusing(at(Ceiling, KeyProcess, false, StopCounting), Silent)), ""},
		{
			"stopping counting, silently among others",
			row(refusing(at(Ceiling, KeyProcess, false, StopCounting), Gate, Silent)), "",
		},
		{"stopping counting, saying nothing", row(at(Ceiling, KeyProcess, false, StopCounting)), "declares no silent refusal"},
		{
			"stopping counting aloud",
			row(refusing(at(Ceiling, KeyProcess, false, StopCounting), Gate)), "declares no silent refusal",
		},
		{"taking across keys from a table", row(at(Ceiling, KeyProcess, true, EvictAcrossKeys)), ""},
		{"taking across keys from no table", row(at(Ceiling, KeyProcess, false, EvictAcrossKeys)), "is not a table one is taken from"},
	})
}

// TestValidate_BoundedTable holds a structure keyed on a mintable value to a
// bound, or to a finding.
func TestValidate_BoundedTable(t *testing.T) {
	table := func(d *Decision) { d.Kind, d.Key, d.Table = Lifetime, KeyVerified, true }
	runRule(t, checkBoundedTable, []ruleCase{
		{"a lifetime with no capacity", row(table), "mintable verified key with no capacity"},
		{"recorded", row(func(d *Decision) { table(d); d.Findings = []string{"F-29"} }), ""},
		{"evicting the oldest", row(func(d *Decision) { table(d); d.AtCapacity = EvictOldest }), ""},
		{"keyed on the process", row(func(d *Decision) { table(d); d.Key = KeyProcess }), ""},
		{"not a table", row(func(d *Decision) { d.Kind, d.Key = Lifetime, KeyVerified }), ""},
	})
}

// TestValidate_ChargedExists holds a charge to a budget that runs before
// admission.
func TestValidate_ChargedExists(t *testing.T) {
	charged := func(to string) Decision {
		return row(func(d *Decision) {
			d.Refusals = []Refusal{{Methods: []string{MethodGate}, Channel: Gate, Charged: []string{to}}}
		})
	}
	runRule(t, checkChargedExists, []ruleCase{
		{"a budget on an address", charged("BUDGET"), ""},
		{"a row that does not exist", charged("NONE"), "charged to NONE"},
		{"a rule", charged("RULE"), "charged to RULE"},
		{"a budget on the entry", charged("OWNBGT"), "charged to OWNBGT"},
	})
}

// TestValidate_Stdio holds the stdio key to one no caller can multiply.
func TestValidate_Stdio(t *testing.T) {
	runRule(t, checkStdio, []ruleCase{
		{"absent on stdio", row(nil), ""},
		{"the process", row(func(d *Decision) { d.StdioKey = KeyProcess }), ""},
		{"a request", row(func(d *Decision) { d.StdioKey = KeyRequest }), ""},
		{"the deployment", row(func(d *Decision) { d.StdioKey = KeyDeployment }), ""},
		{"the entry", row(func(d *Decision) { d.StdioKey = KeyEntry }), "the entry key on stdio"},
		{"an address", row(func(d *Decision) { d.StdioKey = KeyAddress }), "the address key on stdio"},
	})
}

// TestValidate_WellFormed refuses a row whose fields contradict each other or
// name what does not exist.
func TestValidate_WellFormed(t *testing.T) {
	runRule(t, checkWellFormed, []ruleCase{
		{"well formed", row(func(d *Decision) { d.Partner, d.OffWith, d.Findings = "PROC", "PROC", []string{"F-01"} }), ""},
		{"no question", row(func(d *Decision) { d.Question = 0 }), "left unset"},
		{"no kind", row(func(d *Decision) { d.Kind = 0 }), "left unset"},
		{"no class", row(func(d *Decision) { d.Class = 0 }), "left unset"},
		{"no disposition", row(func(d *Decision) { d.Disposition = 0 }), "left unset"},
		{"a refusal with no channel", row(func(d *Decision) { d.Refusals = []Refusal{{Answer: RetryLater}} }), "no channel or no answer"},
		{"a refusal with no answer", row(func(d *Decision) { d.Refusals = []Refusal{{Channel: RPC}} }), "no channel or no answer"},
		{"an unknown partner", row(func(d *Decision) { d.Partner = "NONE" }), "partner NONE names no row"},
		{"an unknown zero partner", row(func(d *Decision) { d.OffWith = "NONE" }), "follows NONE"},
		{
			"a zero decided by a record it keeps",
			row(func(d *Decision) {
				d.OffWithBy, d.ZeroBy, d.Decided = "issue 951", "issue 958", []string{"issue 951", "issue 958"}
			}), "",
		},
		{
			"a partner's zero decided by a record it does not keep",
			row(func(d *Decision) { d.OffWithBy = "issue 951" }), "decided by issue 951, which it does not record",
		},
		{
			"a zero decided by a record it does not keep",
			row(func(d *Decision) { d.ZeroBy, d.Decided = "issue 958", []string{"ADR-0020"} }), "decided by issue 958, which it does not record",
		},
		{
			"a malformed-value policy decided by a record it keeps",
			row(func(d *Decision) { d.MalformedBy, d.Decided = "issue 957", []string{"ADR-0022", "issue 957"} }), "",
		},
		{
			"a malformed-value policy decided by a record it does not keep",
			row(func(d *Decision) { d.MalformedBy, d.Decided = "issue 957", []string{"ADR-0022"} }),
			"its malformed-value policy is decided by issue 957, which it does not record",
		},
		{
			"a setting's zero",
			row(func(d *Decision) {
				d.Config, d.SettingZeros = []string{"A", "B"}, []SettingZero{{Config: "B", Zero: ZeroRefused}}
			}), "",
		},
		{
			"the zero of a setting it does not have",
			row(func(d *Decision) {
				d.Config, d.SettingZeros = []string{"A", "B"}, []SettingZero{{Config: "C", Zero: ZeroRefused}}
			}),
			"the zero of C, which is not one of its settings",
		},
		{
			"a setting's zero left unsaid",
			row(func(d *Decision) { d.Config, d.SettingZeros = []string{"A", "B"}, []SettingZero{{Config: "B"}} }),
			"the zero of B and does not say what it means",
		},
		{
			"a setting's zero that agrees with the row's",
			row(func(d *Decision) {
				d.Config, d.Zero, d.SettingZeros = []string{"A", "B"}, ZeroOff, []SettingZero{{Config: "B", Zero: ZeroOff}}
			}),
			"the zero of B means what the row's own does",
		},
		{
			"a setting's zero stated twice",
			row(func(d *Decision) {
				d.Config, d.Zero = []string{"A", "B"}, ZeroOff
				d.SettingZeros = []SettingZero{{Config: "B", Zero: ZeroRefused}, {Config: "B", Zero: ZeroSelectsDefault}}
			}),
			"it states the zero of B twice",
		},
		{
			"the zero of every setting",
			row(func(d *Decision) {
				d.Config, d.Zero = []string{"A"}, ZeroRefused
				d.SettingZeros = []SettingZero{{Config: "A", Zero: ZeroOff}}
			}),
			"leaves none of its settings to the row's own zero",
		},
		{
			"the zero of every setting but one",
			row(func(d *Decision) {
				d.Config, d.Zero = []string{"A", "B", "C"}, ZeroOff
				d.SettingZeros = []SettingZero{{Config: "A", Zero: ZeroRefused}, {Config: "C", Zero: ZeroRefused}}
			}), "",
		},
		{"an unknown finding", row(func(d *Decision) { d.Findings = []string{"F-99"} }), "F-99, which is not a finding"},
		{"promoted with no function", row(func(d *Decision) { d.Disposition = Promoted }), "promoted with no function"},
		{"promoted with a function", row(func(d *Decision) { d.Disposition, d.Functions = Promoted, []string{"F"} }), ""},
		{"valued with no value", row(func(d *Decision) { d.Disposition = Valued }), "valued with no value"},
		{"a mechanism shaped as an allowance", row(func(d *Decision) { d.Disposition, d.Kind = Mechanism, Ceiling }), "shaped as an allowance"},
		{"a request bound shaped as an allowance", row(func(d *Decision) { d.Disposition, d.Kind = RequestBound, Rate }), "shaped as an allowance"},
		{"a request bound", row(func(d *Decision) { d.Disposition, d.Kind = RequestBound, Bound }), ""},
	})
}

// TestValidate_CodeMirrorsStatus holds every in-band code this server
// allocates to the status the standard assigns to what the refusal says,
// mirrored as the status times -100 (issue 956): an in-band code mirrors a
// status the gate answers the same class with somewhere in the register, so
// retry later is -42900 where the caller made too many requests and -50300
// where the server cannot serve it right now, and nothing else. JSON-RPC's
// own codes, the gate's own literals and a row that records the departure
// are left out.
func TestValidate_CodeMirrorsStatus(t *testing.T) {
	refusing := func(id string, channel Channel, code int, answer Answer, findings ...string) Decision {
		r := Refusal{Channel: channel, Code: code, Answer: answer}
		if channel == Gate {
			r.Status = code / -100
		}
		return Decision{ID: id, Findings: findings, Refusals: []Refusal{r}}
	}
	tooMany := refusing("G1", Gate, CodeTooManyRequests, RetryLater)
	unavailable := refusing("G2", Gate, CodeUnavailable, RetryLater)
	forbidden := refusing("G3", Gate, CodeForbidden, WidenScope)
	for _, tc := range []struct {
		name string
		rows []Decision
		want string
	}{
		{"too many requests", []Decision{tooMany, unavailable, refusing("A", RPC, CodeTooManyRequests, RetryLater)}, ""},
		{"cannot serve right now", []Decision{tooMany, unavailable, refusing("A", RPC, CodeUnavailable, RetryLater)}, ""},
		{"both, each saying its own", []Decision{
			tooMany, unavailable, refusing("A", RPC, CodeTooManyRequests, RetryLater), refusing("B", RPC, CodeUnavailable, RetryLater),
		}, ""},
		{
			"the legacy busy code",
			[]Decision{tooMany, unavailable, refusing("B", RPC, -32000, RetryLater)},
			`B: code-mirrors-status (INV-012): answers "retry later" in-band with code -32000, which mirrors no status the gate answers it with (429, 503)`,
		},
		{"the legacy busy code, recorded", []Decision{tooMany, unavailable, refusing("B", RPC, -32000, RetryLater, "F-07")}, ""},
		{
			"a status of another class",
			[]Decision{tooMany, unavailable, forbidden, refusing("B", RPC, CodeForbidden, RetryLater)},
			`B: code-mirrors-status (INV-012): answers "retry later" in-band with code -40300, which mirrors no status the gate answers it with (429, 503)`,
		},
		{
			"a class the gate never answers",
			[]Decision{tooMany, refusing("B", RPC, CodeForbidden, WidenScope)},
			`B: code-mirrors-status (INV-012): answers "widen the scope" in-band with code -40300, which mirrors no status the gate answers it with (none)`,
		},
		{
			"a code that mirrors no status",
			[]Decision{tooMany, refusing("B", RPC, -42901, RetryLater)},
			`B: code-mirrors-status (INV-012): answers "retry later" in-band with code -42901, which mirrors no status the gate answers it with (429)`,
		},
		{"another class, mirrored", []Decision{forbidden, refusing("B", RPC, CodeForbidden, WidenScope)}, ""},
		{"a protocol code", []Decision{tooMany, refusing("B", RPC, codeInternalError, RetryLater)}, ""},
		{"the gate alone", []Decision{tooMany, unavailable, forbidden}, ""},
		{"a tool error", []Decision{refusing("B", ToolError, 0, RetryLater)}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, v := range checkCodeMirrorsStatus(tc.rows) {
				v.Rule, v.Invariant = "code-mirrors-status", "INV-012"
				got = append(got, v.Error())
			}
			if strings.Join(got, "\n") != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestValidate_UniqueAndComplete holds the register to one row per
// requirement: a second row, a missing one and an unknown one all fail.
func TestValidate_UniqueAndComplete(t *testing.T) {
	all := Decisions()
	if v := checkUnique(all); len(v) != 0 {
		t.Errorf("the register has duplicates: %v", v)
	}
	if v := checkComplete(all); len(v) != 0 {
		t.Errorf("the register is incomplete: %v", v)
	}
	duplicated := append(append([]Decision{}, all...), all[0])
	if v := checkUnique(duplicated); len(v) != 1 || v[0].ID != all[0].ID {
		t.Errorf("a duplicate of %s: %v", all[0].ID, v)
	}
	missing := append(append([]Decision{}, all[1:]...), Decision{ID: "XYZ-001"})
	v := checkComplete(missing)
	if len(v) != 2 || v[0].ID != all[0].ID || v[1].ID != "XYZ-001" {
		t.Errorf("missing %s and extra XYZ-001: %v", all[0].ID, v)
	}
}

// TestValidate_RefusesAShareOnTheTenantWithItsEvidence is the question issue
// 540 answered by hand, put to Validate: a share granted per tenant is refused
// before review, and the refusal says why a tenant is mintable.
func TestValidate_RefusesAShareOnTheTenantWithItsEvidence(t *testing.T) {
	share := Decision{
		ID: "HLD-002", Question: Allow, Kind: Share, Class: ClassT, Disposition: Valued, Key: KeyTenant,
		Values: []string{"ListenStreamsPerProcess"}, Source: Constant, Zero: ZeroNotApplicable,
	}
	ds := Decisions()
	for i := range ds {
		if ds[i].ID == share.ID {
			ds[i] = share
		}
	}
	err := Validate(ds)
	if err == nil {
		t.Fatal("a share on the tenant was accepted")
	}
	var v *ViolationError
	if !errors.As(err, &v) || v.ID != "HLD-002" || v.Rule != "share-on-mintable" || v.Invariant != "INV-003" {
		t.Errorf("first violation = %+v", v)
	}
	if !strings.Contains(err.Error(), KeyTenant.Evidence()) {
		t.Errorf("the refusal does not quote the tenant's evidence:\n%v", err)
	}
}

// TestValidate_ReportsEverySetRule joins a set rule's violation with its name
// and invariant.
func TestValidate_ReportsEverySetRule(t *testing.T) {
	ds := append(Decisions(), Decisions()[0])
	err := Validate(ds)
	if err == nil || !strings.Contains(err.Error(), "IDN-001: unique (one row per requirement): declared twice") {
		t.Errorf("Validate with a duplicate row = %v", err)
	}
}

// TestViolationError_Error renders one line a reviewer can act on.
func TestViolationError_Error(t *testing.T) {
	v := &ViolationError{ID: "HLD-001", Rule: "process-partner", Invariant: "INV-004", Detail: "no partner"}
	if got, want := v.Error(), "HLD-001: process-partner (INV-004): no partner"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestValidateFailures_RefusesWhatINV007Forbids puts each way a failure row
// can be wrong to ValidateFailures.
func TestValidateFailures_RefusesWhatINV007Forbids(t *testing.T) {
	good := Failure{
		Kind: "missing-credential", Attributable: true, Charged: true, Decision: "IDN-001", Status: 401,
		At: Site{Pkg: pkgServer, Name: "mcpServerGate.resolve", Role: Charge, Call: "mcpServerGate.chargeFailure", Count: 1},
	}
	for _, tc := range []struct {
		name   string
		change func(f *Failure)
		want   string
	}{
		{"a charged failure the caller caused", func(*Failure) {}, ""},
		{"charged although not the caller's", func(f *Failure) { f.Attributable = false }, "charged-attributable"},
		{"a decision that is no row", func(f *Failure) { f.Decision = "XYZ-001" }, "failure-decision"},
		{"a site that charges nothing", func(f *Failure) { f.At.Role = Refuse }, "failure-site"},
		{"no charge helper", func(f *Failure) { f.At.Call = "" }, "failure-site"},
		{"no function", func(f *Failure) { f.At.Name = "" }, "failure-site"},
		{"a success status", func(f *Failure) { f.Status = 200 }, "failure-status"},
		{"the status below the refusals", func(f *Failure) { f.Status = 399 }, "failure-status"},
		{"the first refusal status", func(f *Failure) { f.Status = 400 }, ""},
		{"the last refusal status", func(f *Failure) { f.Status = 599 }, ""},
		{"a status past the refusals", func(f *Failure) { f.Status = 600 }, "failure-status"},
		{"a count that disagrees", func(f *Failure) { f.At.Count = 2 }, "declares 2 charged failures and lists 1"},
		{"an uncharged failure", func(f *Failure) { f.Charged, f.At.Count = false, 0 }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := good
			tc.change(&f)
			err := ValidateFailures([]Failure{f})
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("refused an acceptable failure: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("got %v, want a refusal containing %q", err, tc.want)
			}
		})
	}
}
