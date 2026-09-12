//go:build e2e

// license.go reads the Enterprise license the provisioning script cached,
// for the one test that installs a license through the server.
//
// The key is never a setting: setup-gitlab.sh exports the active license
// from the instance it just activated and writes it to a file under
// test/e2e, so a later run can reuse it without spending another activation.
// The test that adds a duplicate of it and removes that duplicate reads the
// same file, which is also why the test can only run where the script ran.

package fixture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// SettingEnterpriseLicenseFile is the setting that names another license
// cache than the default, the same one the provisioning scripts read.
const SettingEnterpriseLicenseFile = "E2E_ENTERPRISE_LICENSE_FILE"

// defaultEnterpriseLicenseFile is where setup-gitlab.sh caches the license,
// relative to the repository root.
var defaultEnterpriseLicenseFile = filepath.Join("test", "e2e", ".enterprise-license")

// errNoLicenseCached is a cache file that is missing or holds nothing.
var errNoLicenseCached = errors.New("no Enterprise license is cached")

// CachedEnterpriseLicense returns the Base64 license the provisioning script
// cached for this checkout, skipping the test when there is none: a run
// against an instance somebody else licensed has no key to add.
func CachedEnterpriseLicense(e *harness.Env) string {
	e.T.Helper()

	path := licenseCachePath(e.Setting(SettingEnterpriseLicenseFile), e.RepoRoot())
	key, err := readCachedLicense(path)
	if err != nil {
		e.Skipf("reading the cached Enterprise license: %v (provision one with test/e2e/scripts/setup-gitlab.sh, or name it with %s)", err, SettingEnterpriseLicenseFile)
	}
	return key
}

// licenseCachePath resolves the cache file: the configured path, made
// absolute against the repository root when it is relative, and the default
// location under the root otherwise.
func licenseCachePath(configured, root string) string {
	if configured == "" {
		return filepath.Join(root, defaultEnterpriseLicenseFile)
	}
	if filepath.IsAbs(configured) {
		return configured
	}
	return filepath.Join(root, configured)
}

// readCachedLicense reads the key out of one cache file, refusing an empty
// one the same way as a missing one.
func readCachedLicense(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w at %s: %w", errNoLicenseCached, path, err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("%w: %s is empty", errNoLicenseCached, path)
	}
	return key, nil
}
