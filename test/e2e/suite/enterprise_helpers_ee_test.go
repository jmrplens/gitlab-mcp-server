//go:build e2e && enterprise

// enterprise_helpers_ee_test.go contains shared assertions for Enterprise E2E
// tests.
package suite

import "testing"

// requirePremiumFeature fails the test if err is non-nil. Enterprise tests
// already skip themselves on Community Edition, so any error here is an
// unexpected failure and must surface as a test failure (not a skip).
func requirePremiumFeature(t *testing.T, err error, feature string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s failed: %v", feature, err)
	}
}
