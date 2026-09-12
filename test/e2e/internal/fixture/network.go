//go:build e2e

// network.go answers where things are on the Docker network, which is not
// where the test sees them.
//
// The test reaches GitLab at a host-facing address such as localhost:8929,
// and GitLab cannot reach itself there: a mirror, a direct transfer or a
// release-cli call made from inside the compose network has to name the
// service instead. The suite this replaces spelled that address in three
// places with three fallbacks; this file is the one place it lives now, and
// every URL that has to be reachable from inside GitLab is built here.

package fixture

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The configuration keys the network helpers read, as setup-gitlab.sh writes
// them into .env.docker.
const (
	// SettingInternalGitLabURL is the address GitLab is reachable at from
	// inside the compose network.
	SettingInternalGitLabURL = "E2E_GITLAB_INTERNAL_URL"
	// SettingFixtureURL is the address of the fixture service, which answers
	// webhook deliveries, custom emoji images and mirror pushes.
	SettingFixtureURL = "E2E_FIXTURE_URL"
	// SettingGitLabToken is the run's own credential, which a mirror target
	// embeds so GitLab can push to itself.
	SettingGitLabToken = "GITLAB_TOKEN"
)

// The addresses the Docker stack uses when the settings say nothing, which
// are the service names docker-compose.yml declares.
const (
	DefaultInternalGitLabURL = "http://gitlab-e2e"
	DefaultFixtureURL        = "http://e2e-fixture:8080"
)

// InternalGitLabURL returns the address GitLab reaches itself at from inside
// the Docker network, with no trailing slash.
func InternalGitLabURL(e *harness.Env) string {
	return resolveInternalGitLabURL(e.Setting(SettingInternalGitLabURL))
}

// resolveInternalGitLabURL is InternalGitLabURL over a raw setting.
func resolveInternalGitLabURL(configured string) string {
	return withoutTrailingSlash(configured, DefaultInternalGitLabURL)
}

// RepositoryURL returns the clone URL of a project as GitLab reaches it from
// inside the Docker network, which is what a pull mirror names as its source.
// Outside Docker mode, where no internal address is configured, it is the
// URL GitLab itself published for the project.
func RepositoryURL(e *harness.Env, project Project) string {
	return repositoryURL(e.Setting(SettingInternalGitLabURL), project.Path, project.HTTPURLToRepo)
}

// repositoryURL is RepositoryURL over raw values.
func repositoryURL(configured, path, published string) string {
	if strings.TrimSpace(configured) == "" {
		return published
	}
	return withoutTrailingSlash(configured, "") + "/" + strings.TrimLeft(path, "/") + ".git"
}

// MirrorTargetURL returns an authenticated clone URL for target that GitLab
// can push to from inside the Docker network, for a push mirror. The run's
// own token rides in the URL as the oauth2 user, which is how GitLab
// authenticates a mirror push against itself.
func MirrorTargetURL(e *harness.Env, target Project) string {
	e.T.Helper()

	token := e.Setting(SettingGitLabToken)
	if token == "" {
		e.T.Fatalf("%s is required to configure a mirror target", SettingGitLabToken)
	}
	targetURL, err := mirrorTargetURL(InternalGitLabURL(e), token, target.Path)
	if err != nil {
		e.T.Fatalf("building the mirror target URL: %v", err)
	}
	return targetURL
}

// mirrorTargetURL is MirrorTargetURL over raw values.
func mirrorTargetURL(base, token, path string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parsing the internal GitLab URL %q: %w", base, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("the internal GitLab URL %q names no scheme or host", base)
	}
	parsed.User = url.UserPassword("oauth2", token)
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(path, "/") + ".git"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// HasFixtureService reports whether the fixture service should be reachable
// from GitLab in this run: either its address is configured, or the run is
// in Docker mode, where the stack always starts it. NeedFixtureService is the
// declaration; this is the same answer for a test that decides at runtime.
func HasFixtureService(e *harness.Env) bool {
	return e.Setting(SettingFixtureURL) != "" || e.DockerMode()
}

// ServiceURL returns the address of one resource on the fixture service, as
// GitLab reaches it: a webhook receiver, a custom emoji image, a mirror
// target the Docker stack answers deterministically.
func ServiceURL(e *harness.Env, resourcePath string) string {
	return fixtureServiceURL(e.Setting(SettingFixtureURL), resourcePath)
}

// fixtureServiceURL is ServiceURL over a raw setting.
func fixtureServiceURL(configured, resourcePath string) string {
	return withoutTrailingSlash(configured, DefaultFixtureURL) + "/" + strings.TrimLeft(resourcePath, "/")
}

// withoutTrailingSlash returns configured with its trailing slashes removed,
// or fallback when configured is blank.
func withoutTrailingSlash(configured, fallback string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(configured), "/")
	if trimmed == "" {
		return strings.TrimRight(fallback, "/")
	}
	return trimmed
}
