//go:build e2e

// seeds.go reads the state setup-gitlab.sh provisioned before the run: the
// objects a test consumes and cannot make for itself.
//
// A seed is provisioned per consumer, one copy for each package and surface
// that eats it, because a destructive scenario consumes its seed: a registry
// tag deleted on the individual surface is not there for the meta run, and
// whichever ran second would have to skip. The consumer slot is therefore
// part of the variable name, and a test reads its own copy by naming the
// surface it runs on. The one seed that cannot be multiplied, the pending
// schema migration, has no slot and is used on one surface only.

package fixture

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// Seed names a per-consumer seed, as the variable name it is provisioned
// under before the consumer suffix.
type Seed string

// The per-consumer seeds setup-gitlab.sh provisions.
const (
	// SeedRegistryProject is the path of a project whose container registry
	// holds the tags seed-a and seed-b.
	SeedRegistryProject Seed = "E2E_REGISTRY_PROJECT"
	// SeedPendingApproveUserID is the ID of a user in the
	// blocked_pending_approval state, for the approve scenario to approve.
	SeedPendingApproveUserID Seed = "E2E_PENDING_APPROVE_USER_ID"
	// SeedPendingRejectUserID is its sibling, for the reject scenario.
	SeedPendingRejectUserID Seed = "E2E_PENDING_REJECT_USER_ID"
)

// The seeds provisioned once per instance rather than per consumer.
const (
	// SettingDBMigrationVersion is the version of a schema migration the
	// setup script made pending, for the db_migration_mark happy path. An
	// instance has one schema_migrations table, so there is one of these,
	// and its scenario runs on one surface with OnSurfaces and this reason.
	SettingDBMigrationVersion = "E2E_DB_MIGRATION_VERSION"
	// SettingRootToken is the root user's token the setup script created,
	// for a scenario that needs an administrator other than the run's own.
	SettingRootToken = "E2E_ROOT_TOKEN" //nolint:gosec // G101: the name of the setting that holds a token, not a token
)

// SeedValue returns the value provisioned for seed, for this package on the
// given surface, and whether it was provisioned at all.
func SeedValue(e *harness.Env, seed Seed, surface harness.Surface) (string, bool) {
	value := e.Setting(seedKey(seed, e.Package(), surface))
	return value, value != ""
}

// RequireSeed returns the value provisioned for seed on the given surface,
// skipping the test with the variable's name when there is none: a seed the
// provisioning script could not create is reported as absent rather than as
// a failure of the action that would have consumed it.
func RequireSeed(e *harness.Env, seed Seed, surface harness.Surface) string {
	e.T.Helper()

	value, provisioned := SeedValue(e, seed, surface)
	if !provisioned {
		e.Skipf("seed %s is not provisioned for this run; setup-gitlab.sh writes it into test/e2e/.env.docker", seedKey(seed, e.Package(), surface))
	}
	return value
}

// seedKey builds the variable name one consumer slot reads a seed from:
// the seed, the package and the surface, uppercased and joined with
// underscores, which is what setup-gitlab.sh's seed_slot_suffix produces
// for a "package:surface" slot.
func seedKey(seed Seed, pkg string, surface harness.Surface) string {
	return string(seed) + "_" + slotSuffix(pkg) + "_" + slotSuffix(surface.String())
}

// slotSuffix spells one half of a consumer slot the way the shell does: in
// upper case, with every separator turned into an underscore.
func slotSuffix(part string) string {
	replacer := strings.NewReplacer(":", "_", "-", "_")
	return strings.ToUpper(replacer.Replace(part))
}

// RegistryProject returns the path of the registry seed project provisioned
// for this package on surface, skipping the test when there is none.
func RegistryProject(e *harness.Env, surface harness.Surface) string {
	e.T.Helper()
	return RequireSeed(e, SeedRegistryProject, surface)
}

// PendingApprovalUsers returns the IDs of the two users seeded in the
// pending-approval state for this package on surface: one to approve, one to
// reject. The test is skipped when either is missing, since the pair is
// provisioned together.
func PendingApprovalUsers(e *harness.Env, surface harness.Surface) (approveID, rejectID int64) {
	e.T.Helper()

	approveID = seedID(e, SeedPendingApproveUserID, surface)
	rejectID = seedID(e, SeedPendingRejectUserID, surface)
	return approveID, rejectID
}

// seedID reads a seed that must be a numeric identifier.
func seedID(e *harness.Env, seed Seed, surface harness.Surface) int64 {
	e.T.Helper()

	value := RequireSeed(e, seed, surface)
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		e.T.Fatalf("seed %s is %q, and the provisioning script writes a numeric user ID there", seedKey(seed, e.Package(), surface), value)
	}
	return id
}

// DBMigrationVersion returns the version of the schema migration the setup
// script made pending, skipping the test when it could not.
func DBMigrationVersion(e *harness.Env) string {
	e.T.Helper()

	version := e.Setting(SettingDBMigrationVersion)
	if version == "" {
		e.Skipf("%s is not set: the setup script could not seed a pending schema migration", SettingDBMigrationVersion)
	}
	return version
}

// RootToken returns the root user's token the setup script recorded,
// skipping the test when the run has none.
func RootToken(e *harness.Env) string {
	e.T.Helper()

	token := e.Setting(SettingRootToken)
	if token == "" {
		e.Skipf("%s is not set: only the Docker setup script records one", SettingRootToken)
	}
	return token
}
