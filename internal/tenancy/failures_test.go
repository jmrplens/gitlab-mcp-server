package tenancy

import (
	"strings"
	"testing"
)

// TestFailures_ValidateFailuresIsNil is the failure table's verdict on itself:
// every charged failure is one the caller caused, and each function lists as
// many charged failures as it declares.
func TestFailures_ValidateFailuresIsNil(t *testing.T) {
	if err := ValidateFailures(Failures()); err != nil {
		t.Errorf("ValidateFailures(Failures()) =\n%v", err)
	}
}

// TestFailures_ListEveryRefusalOfTheThreeFunctions pins the table to the
// twenty-one refusal returns of the gate's resolve and the bearer guard's
// check and classify, of which two, two and one are charged.
func TestFailures_ListEveryRefusalOfTheThreeFunctions(t *testing.T) {
	type tally struct{ returns, charged int }
	got := map[string]tally{}
	for _, f := range Failures() {
		c := got[f.At.Name]
		c.returns++
		if f.Charged {
			c.charged++
		}
		got[f.At.Name] = c
	}
	for _, tc := range []struct {
		fn      string
		returns int
		charged int
	}{
		{"mcpServerGate.resolve", 8, 2},
		{"bearerGuard.check", 7, 2},
		{"bearerGuard.classify", 6, 1},
	} {
		t.Run(tc.fn, func(t *testing.T) {
			if got[tc.fn] != (tally{tc.returns, tc.charged}) {
				t.Errorf("%s lists %d returns, %d charged; want %d, %d",
					tc.fn, got[tc.fn].returns, got[tc.fn].charged, tc.returns, tc.charged)
			}
		})
	}
	if len(got) != 3 || len(Failures()) != 21 {
		t.Errorf("%d functions and %d failures, want 3 and 21", len(got), len(Failures()))
	}
}

// TestFailures_ChargedAreExactlyTheCallersOwn names the five charged failures:
// a missing credential and a credential GitLab refused, at each door, and the
// cached refusal the bearer guard answers from memory.
func TestFailures_ChargedAreExactlyTheCallersOwn(t *testing.T) {
	var charged []string
	for _, f := range Failures() {
		if f.Charged {
			charged = append(charged, f.At.Name+":"+f.Kind)
		}
	}
	want := "mcpServerGate.resolve:missing-credential,mcpServerGate.resolve:gitlab-rejected," +
		"bearerGuard.check:missing-credential,bearerGuard.check:cached-rejection," +
		"bearerGuard.classify:gitlab-rejected"
	if got := strings.Join(charged, ","); got != want {
		t.Errorf("charged failures = %s\nwant %s", got, want)
	}
}

// TestFailures_InsufficientScopeReportedByGitLabIsUncharged holds the branch
// the first table left out: GitLab calling a genuine token under-scoped is a
// refusal of classify's, and it is not charged, because the token is valid.
func TestFailures_InsufficientScopeReportedByGitLabIsUncharged(t *testing.T) {
	for _, f := range Failures() {
		if f.At.Name == "bearerGuard.classify" && f.Kind == "gitlab-insufficient-scope" {
			if f.Charged || f.Attributable || f.Status != 403 {
				t.Errorf("classify's insufficient scope: %+v", f)
			}
			return
		}
	}
	t.Error("classify's GitLab-reported insufficient scope is missing from the table")
}

// TestFailures_EachNamesARowAndARefusalStatus holds each failure to a row of
// the register and a status in the refusal range.
func TestFailures_EachNamesARowAndARefusalStatus(t *testing.T) {
	for _, f := range Failures() {
		if _, ok := Lookup(f.Decision); !ok {
			t.Errorf("%s:%s names %s, which is not a row", f.At.Name, f.Kind, f.Decision)
		}
		if f.Status < 400 || f.Status > 599 {
			t.Errorf("%s:%s has status %d", f.At.Name, f.Kind, f.Status)
		}
	}
}
