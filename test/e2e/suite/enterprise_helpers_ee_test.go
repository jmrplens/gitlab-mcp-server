//go:build e2e && enterprise

// enterprise_helpers_ee_test.go contains shared assertions for Enterprise E2E
// tests.
package suite

import "testing"

// requirePremiumFeature fails the test if the error indicates the feature
// requires a premium/ultimate license or admin permissions. Enterprise tests
// are gated at skip level so they only run when the GitLab instance supports them.
func requirePremiumFeature(t *testing.T, err error, feature string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s failed: %v", feature, err)
	}
}
