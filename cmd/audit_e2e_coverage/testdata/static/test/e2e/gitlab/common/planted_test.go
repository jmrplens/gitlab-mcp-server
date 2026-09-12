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

// TestPlanted_HelperConstant_Resolved passes its id through a helper's
// ActionID parameter, which the gate must read as the constant it is.
func TestPlanted_HelperConstant_Resolved(t *testing.T) {
	checkRead(harness.New(t).Session(), "project.get")
}

// checkRead is the helper the constant travels through.
func checkRead(s *harness.Session, id harness.ActionID) {
	harness.DoVoid(s, id, nil)
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
