//go:build e2e

// seeds_test.go pins the variable name a consumer slot reads its seed from,
// which has to agree with what setup-gitlab.sh writes.

package fixture

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestSeedKey_Slots_SpellTheShellSuffix checks that the key for a package
// and surface is what the script's seed_slot_suffix produces for the slot
// "package:surface": upper case, separators to underscores.
func TestSeedKey_Slots_SpellTheShellSuffix(t *testing.T) {
	cases := []struct {
		name    string
		seed    Seed
		pkg     string
		surface harness.Surface
		want    string
	}{
		{name: "registry on common individual", seed: SeedRegistryProject, pkg: "common", surface: harness.SurfaceIndividual, want: "E2E_REGISTRY_PROJECT_COMMON_INDIVIDUAL"},
		{name: "approve on common meta", seed: SeedPendingApproveUserID, pkg: "common", surface: harness.SurfaceMeta, want: "E2E_PENDING_APPROVE_USER_ID_COMMON_META"},
		{name: "reject on common dynamic", seed: SeedPendingRejectUserID, pkg: "common", surface: harness.SurfaceDynamic, want: "E2E_PENDING_REJECT_USER_ID_COMMON_DYNAMIC"},
		{name: "a package with a dash", seed: SeedRegistryProject, pkg: "ee-extra", surface: harness.SurfaceMeta, want: "E2E_REGISTRY_PROJECT_EE_EXTRA_META"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := seedKey(testCase.seed, testCase.pkg, testCase.surface); got != testCase.want {
				t.Errorf("seedKey() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestSlotSuffix_Separators_BecomeUnderscores checks the shell's tr on its
// own: colons and dashes both become underscores, and the case is upper.
func TestSlotSuffix_Separators_BecomeUnderscores(t *testing.T) {
	if got, want := slotSuffix("common:individual"), "COMMON_INDIVIDUAL"; got != want {
		t.Errorf("slotSuffix(common:individual) = %q, want %q", got, want)
	}
	if got, want := slotSuffix("read-only"), "READ_ONLY"; got != want {
		t.Errorf("slotSuffix(read-only) = %q, want %q", got, want)
	}
}
