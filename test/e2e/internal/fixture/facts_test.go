//go:build e2e

// facts_test.go pins the GitLab 19 facts the tests rely on.

package fixture

import (
	"errors"
	"net/http"
	"testing"
)

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
