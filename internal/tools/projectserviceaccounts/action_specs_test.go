// action_specs_test.go contains tests for the projectServiceAccountDescription function.
package projectserviceaccounts

import (
	"strings"
	"testing"
)

// TestProjectServiceAccountDescription_DefaultBranch verifies that
// projectServiceAccountDescription returns the generic fallback description
// when called with an unknown or empty action name.
func TestProjectServiceAccountDescription_DefaultBranch(t *testing.T) {
	for _, actionName := range []string{"", "unknown_action", "service_account_foo"} {
		t.Run(actionName, func(t *testing.T) {
			desc := projectServiceAccountDescription(actionName)
			if !strings.Contains(desc, "Manage GitLab project service accounts") {
				t.Errorf("expected fallback description for action %q, got %q", actionName, desc)
			}
		})
	}
}
