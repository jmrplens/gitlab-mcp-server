//go:build e2e

package common

import (
	"testing"

	"example.com/e2efake/test/e2e/internal/harness"
)

// listIssues is a typed constant, which the gate must read at its use sites
// as well as here.
const listIssues harness.ActionID = "issue.list"

// TestPlanted_UnknownID_Reported names an action the catalog does not have.
func TestPlanted_UnknownID_Reported(t *testing.T) {
	s := harness.New(t).Session()
	harness.DoVoid(s, "nope.action", nil)
}

// TestPlanted_PremiumInCommon_Reported names a Premium action from the
// package that runs on every runtime.
func TestPlanted_PremiumInCommon_Reported(t *testing.T) {
	s := harness.New(t).Session()
	harness.DoVoid(s, "merge_train.list", nil)
}

// TestPlanted_DiscardedResult_Reported throws the answers of the
// result-bearing verbs away, four ways, the last through a parenthesized
// callee.
func TestPlanted_DiscardedResult_Reported(t *testing.T) {
	s := harness.New(t).Session()
	_ = harness.Do[map[string]any](s, "issue.get", nil)
	_, _ = harness.Try[map[string]any](s, "issue.get", nil)
	harness.Eventually(s, "issue.get", nil, func(map[string]any) bool { return true })
	_ = (harness.Do[map[string]any])(s, "issue.get", nil)
}

// TestPlanted_KeptResult_Clean reads its answers, so the same verbs are not
// findings here, and the error half of Try counts as reading it.
func TestPlanted_KeptResult_Clean(t *testing.T) {
	env := harness.New(t)
	s := env.Session()
	out := harness.Do[map[string]any](s, listIssues, nil)
	if out == nil {
		// Reading the field is what marks Env.T used, beside Env.Label,
		// which nothing reads.
		env.T.Log("empty")
	}
	if _, err := harness.Try[map[string]any](s, listIssues, nil); err != nil {
		t.Log(err)
	}
	_ = harness.Refused(s, listIssues, nil, harness.FailureNotFound)
	_ = harness.ExpectToolError(s, listIssues, nil, "not found")
}

// TestPlanted_KeyedLiteral_UsesTheField builds an Env by keyed literal, which
// is a use of the Env.Label field and not of the Label alias that shares its
// name: the type checker records a keyed field under the bare name, and the
// gate must put it back under its type.
func TestPlanted_KeyedLiteral_UsesTheField(t *testing.T) {
	env := harness.Env{T: t, Label: "keyed"}
	if env.Session() == nil {
		t.Fatal("no session")
	}
}

// TestPlanted_UnkeyedLiteral_UsesTheFieldsToo builds the same Env
// positionally, which names no field at all: the type checker records the
// use against the type rather than against a field name, and the gate must
// still not read it as a field nothing uses.
func TestPlanted_UnkeyedLiteral_UsesTheFieldsToo(t *testing.T) {
	env := harness.Env{t, "unkeyed"}
	if env.Session() == nil {
		t.Fatal("no session")
	}
}

// TestPlanted_HelperConstant_Resolved passes its id through a helper's
// ActionID parameter, which the gate must read as the constant it is.
func TestPlanted_HelperConstant_Resolved(t *testing.T) {
	checkRead(harness.New(t).Session(), "project.get")
}

// checkRead is the helper the constant travels through.
func checkRead(s *harness.Session, id harness.ActionID) {
	harness.DoVoid(s, id, nil)
}

// TestPlanted_TwoIDsOnOneLine_BothCollected names two ids on one line, which
// is the only way two sites share a position: a site is spelled file:line,
// with no column, so the pair is what makes the id the tiebreak of the site
// order. Both are unknown on purpose, so the pair is visible in the findings
// as well as in the site list.
func TestPlanted_TwoIDsOnOneLine_BothCollected(t *testing.T) {
	s := harness.New(t).Session()
	for _, each := range []harness.ActionID{"zeta.action", "alpha.action"} {
		harness.DoVoid(s, each, nil)
	}
}

// TestPlanted_TableConstants_Resolved names its ids in a table, so the verb
// call itself takes a non-constant id while every id is still a constant.
func TestPlanted_TableConstants_Resolved(t *testing.T) {
	s := harness.New(t).Session()
	cases := []struct {
		name string
		id   harness.ActionID
	}{
		{name: "status", id: "server.status"},
		{name: "list", id: listIssues},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(*testing.T) {
			harness.DoVoid(s, tc.id, nil)
		})
	}
}

// actionID spells the harness type's name in another case over the same
// underlying type, and is not it: a constant of this type is not a site, since
// the gate reads the type checker's identity of harness.ActionID and never a
// name.
type actionID string

// TestPlanted_LookalikeType_NotASite names an unknown action through the
// lookalike, which would be an unknown-id finding if the gate read the name.
func TestPlanted_LookalikeType_NotASite(t *testing.T) {
	const ghost actionID = "ghost.lookalike"
	harness.New(t).T.Log(ghost)
}

// TestPlanted_SameIDTwiceOnOneLine_CollectedOnce names one constant twice on
// one line, which is one site: a site is an id at a position, and the type
// checker records both uses.
func TestPlanted_SameIDTwiceOnOneLine_CollectedOnce(t *testing.T) {
	if same(listIssues, listIssues) != listIssues {
		t.Fatal("not the same")
	}
}

// same is the helper both copies of the constant travel through. It calls no
// verb, so neither copy is a non-constant site of anything.
func same(a, b harness.ActionID) harness.ActionID {
	if a == b {
		return a
	}
	return b
}

// TestPlanted_OtherShapes_WalkedPast carries the statement shapes a scenario
// body holds beside its verb calls, each of which the walk passes over without
// reading a verb, a discard or a reference into it: a verb called through a
// variable, whose constant is still a site; a result-bearing verb assigned
// into an element, which is not a discard; a function literal called in
// place; a method of the universe's error type; a receive statement; a
// two-value assignment; a helper that calls itself; and the harness's
// interface literal and anonymous struct, whose members are keyed under
// nothing.
func TestPlanted_OtherShapes_WalkedPast(t *testing.T) {
	s := harness.New(t).Session()
	run := harness.DoVoid
	run(s, listIssues, nil)
	results := make([]map[string]any, 1)
	results[0] = harness.Do[map[string]any](s, listIssues, nil)
	func() { results = nil }()
	if _, err := harness.Try[map[string]any](s, listIssues, nil); err != nil {
		t.Log(err.Error())
	}
	done := make(chan struct{}, 1)
	done <- struct{}{}
	<-done
	first, second := 1, 2
	countdown(first + second)
	harness.Stopper.Stop()
	harness.Anon.Field = first
}

// countdown reaches itself, which the reference walk must not record: a
// function is not on its own call path.
func countdown(n int) {
	if n > 0 {
		countdown(n - 1)
	}
}

// TestPlanted_DiscardedThroughHelper_Reported throws away the answer of a
// result-bearing verb whose id arrives as a parameter, which is a finding
// spelled without the id: the helper cannot say which constant it was.
func TestPlanted_DiscardedThroughHelper_Reported(t *testing.T) {
	discardVia(harness.New(t).Session(), "issue.get")
}

// discardVia is the helper that throws the answer away.
func discardVia(s *harness.Session, id harness.ActionID) {
	_ = harness.Do[map[string]any](s, id, nil)
}

// TestPlanted_MultiValueArguments_NonConstant hands two verbs every argument
// through one call, f(g()), so the id is the helper's result and no constant
// spells it at the verb: both calls are non-constant sites, and the second,
// which throws its answer away, is a discard like any other.
func TestPlanted_MultiValueArguments_NonConstant(t *testing.T) {
	s := harness.New(t).Session()
	harness.DoVoid(verbArguments(s))
	_ = harness.Do[map[string]any](verbArguments(s))
}

// verbArguments hands a verb its three arguments as one call's results.
func verbArguments(s *harness.Session) (*harness.Session, harness.ActionID, map[string]any) {
	return s, listIssues, nil
}

// TestPlanted_SharedName_FoldsBothPackages is declared here and in ce under
// one name. The map from a test to the ids it names is keyed by the name
// alone, because the skip line read through it names a test and no package,
// so the two declarations fold into one entry carrying both ids.
func TestPlanted_SharedName_FoldsBothPackages(t *testing.T) {
	harness.DoVoid(harness.New(t).Session(), "issue.get", nil)
}
