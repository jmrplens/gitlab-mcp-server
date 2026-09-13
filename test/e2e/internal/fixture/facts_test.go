//go:build e2e

// facts_test.go pins the GitLab 19 facts the tests rely on.

package fixture

import (
	"errors"
	"net/http"
	"testing"
)

// TestMajorVersion_Strings_ReadsTheLeadingNumber checks the version reader
// on what GET /api/v4/version answers, on the edition suffix, and on the
// strings it must read as nothing: a version that claims nothing is below
// every release, so a fact gated on a major never fires on it by accident.
func TestMajorVersion_Strings_ReadsTheLeadingNumber(t *testing.T) {
	cases := []struct {
		name    string
		version string
		want    int
	}{
		{name: "enterprise", version: "19.3.1-ee", want: 19},
		{name: "community", version: "18.11.0", want: 18},
		{name: "padded", version: " 17.0.0 ", want: 17},
		{name: "major only", version: "20", want: 20},
		{name: "empty", version: "", want: 0},
		{name: "not a number", version: "unknown", want: 0},
		{name: "negative", version: "-1.0", want: 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := MajorVersion(testCase.version); got != testCase.want {
				t.Errorf("MajorVersion(%q) = %d, want %d", testCase.version, got, testCase.want)
			}
		})
	}
}

// TestNonSQLMetricsUnserved_Answers_MatchesOnlyTheDocumented404 checks the
// predicate a test of the non-SQL metrics action asserts its error path
// with.
func TestNonSQLMetricsUnserved_Answers_MatchesOnlyTheDocumented404(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "served", err: nil, want: false},
		{name: "documented 404", err: statusError(http.StatusNotFound, "404 Not Found"), want: true},
		{name: "forbidden", err: statusError(http.StatusForbidden, "403 Forbidden"), want: false},
		{name: "network", err: errors.New("EOF"), want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := NonSQLMetricsUnserved(testCase.err); got != testCase.want {
				t.Errorf("NonSQLMetricsUnserved(%v) = %t, want %t", testCase.err, got, testCase.want)
			}
		})
	}
}
