package tenancy

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"
	"time"
)

// frozenValue is one policy value pinned: the number the register holds it to,
// with the dynamic type it takes when assigned to an interface: int for an
// untyped integer, float64 for an untyped float and time.Duration where the
// site multiplied by a time unit.
type frozenValue struct {
	name string
	got  any
	want any
}

// frozenValues is the table of current pins, and the table a deliberate change
// of value edits: the constant in values.go and its line here, two lines in
// one package (INV-021). The pins taken when the register landed are the
// values as they stood at cb6379f53, the commit the dated record
// (plan/issue-565/spec.md) is dated to, each copied from its section 3.2, with
// END-005 and HLD-007's polling cadence from its amendment G-18. A limit added
// later brings its own pin in the change that adds it.
func frozenValues() []frozenValue {
	return []frozenValue{
		{"ListenStreamsPerCredential", ListenStreamsPerCredential, 64},
		{"ListenStreamsPerProcess", ListenStreamsPerProcess, 512},
		{"WatchersPerCredential", WatchersPerCredential, 10},
		{"WatchersPerProcess", WatchersPerProcess, 512},
		{"WatchLease", WatchLease, 30 * time.Minute},
		{"WatchSlowInterval", WatchSlowInterval, 10 * time.Minute},
		{"WatchMaxLifetime", WatchMaxLifetime, 24 * time.Hour},
		{"WatchBaseInterval", WatchBaseInterval, 15 * time.Second},
		{"WatchMinInterval", WatchMinInterval, 5 * time.Second},
		{"WatchRateLimitPause", WatchRateLimitPause, 30 * time.Second},
		{"WatchRateLimitPauseMax", WatchRateLimitPauseMax, 5 * time.Minute},
		{"WatchRateLimitJitter", WatchRateLimitJitter, 0.2},
		{"ToolCallRateHTTP", ToolCallRateHTTP, 10},
		{"ToolCallRateEnvDefault", ToolCallRateEnvDefault, 0},
		{"ToolCallBurst", ToolCallBurst, 40},
		{"ToolCallRateMax", ToolCallRateMax, 1000},
		{"ToolCallBurstMax", ToolCallBurstMax, 10000},
		{"CompletionFactor", CompletionFactor, 10},
		{"CatalogDivisor", CatalogDivisor, 10},
		{"UpstreamRetries", UpstreamRetries, 2},
		{"UpstreamRetryWaitMax", UpstreamRetryWaitMax, 5 * time.Second},
		{"UpstreamRetryStep", UpstreamRetryStep, 700 * time.Millisecond},
		{"PoolSize", PoolSize, 100},
		{"PoolSizeMax", PoolSizeMax, 10000},
		{"PoolIdleTimeout", PoolIdleTimeout, 1 * time.Hour},
		{"PoolIdleTimeoutMax", PoolIdleTimeoutMax, 24 * time.Hour},
		{"PoolIdleSweepDivisor", PoolIdleSweepDivisor, 4},
		{"PoolIdleSweepFloor", PoolIdleSweepFloor, 1 * time.Minute},
		{"CredentialProbes", CredentialProbes, 16},
		{"CredentialProbeWait", CredentialProbeWait, 5 * time.Second},
		{"UpstreamRetryAfter", UpstreamRetryAfter, 30 * time.Second},
		{"OAuthCacheTTL", OAuthCacheTTL, 15 * time.Minute},
		{"OAuthCacheTTLFloor", OAuthCacheTTLFloor, 1 * time.Minute},
		{"OAuthCacheTTLMax", OAuthCacheTTLMax, 2 * time.Hour},
		{"OAuthCacheSweepDivisor", OAuthCacheSweepDivisor, 4},
		{"OAuthCacheSweepFloor", OAuthCacheSweepFloor, 30 * time.Second},
		{"RejectedTokenTTL", RejectedTokenTTL, 5 * time.Minute},
		{"RejectedTokenCapacity", RejectedTokenCapacity, 4096},
		{"CredentialMaxAge", CredentialMaxAge, 1 * time.Hour},
		{"CredentialMaxAgeCeiling", CredentialMaxAgeCeiling, 24 * time.Hour},
		{"RevalidateInterval", RevalidateInterval, 15 * time.Minute},
		{"RevalidateIntervalMax", RevalidateIntervalMax, 24 * time.Hour},
		{"UnexplainedRefusalCooldown", UnexplainedRefusalCooldown, 30 * time.Second},
		{"RequestStateTTL", RequestStateTTL, 10 * time.Minute},
		{"SessionIdleTimeout", SessionIdleTimeout, 30 * time.Minute},
		{"SessionIdleTimeoutMax", SessionIdleTimeoutMax, 24 * time.Hour},
		{"AuthFailureLimit", AuthFailureLimit, 10},
		{"AuthFailureWindow", AuthFailureWindow, 1 * time.Minute},
		{"AuthFailureLimitMax", AuthFailureLimitMax, 100000},
		{"AuthFailureWindowMax", AuthFailureWindowMax, 24 * time.Hour},
		{"TransportSourceDistinctKeys", TransportSourceDistinctKeys, 500},
		{"AuthDistinctTokenLimit", AuthDistinctTokenLimit, 50},
		{"AuthDistinctTokenWindow", AuthDistinctTokenWindow, 10 * time.Minute},
		{"AuthDistinctTokenLimitMax", AuthDistinctTokenLimitMax, 100000},
		{"AuthDistinctTokenWindowMax", AuthDistinctTokenWindowMax, 24 * time.Hour},
		{"AuthEscalationFirst", AuthEscalationFirst, 1},
		{"AuthEscalationSecond", AuthEscalationSecond, 10},
		{"AuthEscalationThird", AuthEscalationThird, 60},
		{"AuthTrackedSources", AuthTrackedSources, 4096},
		{"AuthSweepInterval", AuthSweepInterval, 5 * time.Minute},
		{"TierNamespacePageSize", TierNamespacePageSize, 100},
		{"TierNamespaceMaxPages", TierNamespaceMaxPages, 10},
	}
}

// TestValues_HoldTheirPins holds every policy value to the number, and the
// type, its pin says. A value that changes here without its own pull request
// is a change of policy riding in on something else, which is what issue 565
// exists to stop.
func TestValues_HoldTheirPins(t *testing.T) {
	for _, v := range frozenValues() {
		t.Run(v.name, func(t *testing.T) {
			if v.got != v.want {
				t.Errorf("%s = %v (%T), want %v (%T)", v.name, v.got, v.got, v.want, v.want)
			}
		})
	}
}

// TestValues_EveryRowValueIsFrozen holds three lists to one: the constants
// values.go declares, the values the rows name, and the frozen table. A
// constant no row names is a value nobody decided; a value a row names that is
// not frozen is one a change could move silently; and a frozen value no row
// names is a pin on nothing.
func TestValues_EveryRowValueIsFrozen(t *testing.T) {
	declared := declaredConstants(t, "values.go")

	var named []string
	for _, d := range Decisions() {
		named = append(named, d.Values...)
	}
	slices.Sort(named)
	if dup := slices.Compact(slices.Clone(named)); len(dup) != len(named) {
		t.Errorf("a value is named by more than one row: %v", named)
	}

	var frozen []string
	for _, v := range frozenValues() {
		frozen = append(frozen, v.name)
	}
	slices.Sort(frozen)

	if !slices.Equal(declared, named) {
		t.Errorf("values.go declares\n%v\nand the rows name\n%v", declared, named)
	}
	if !slices.Equal(declared, frozen) {
		t.Errorf("values.go declares\n%v\nand the frozen table holds\n%v", declared, frozen)
	}
}

// declaredConstants returns the exported constants a file of this package
// declares, sorted.
func declaredConstants(t *testing.T, file string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var names []string
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			for _, name := range spec.(*ast.ValueSpec).Names {
				if name.IsExported() {
					names = append(names, name.Name)
				}
			}
		}
	}
	slices.Sort(names)
	return names
}
