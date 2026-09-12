//go:build e2e

// network_test.go pins the URL resolution that replaced three copies in the
// suite before it.

package fixture

import (
	"testing"
)

// TestResolveInternalGitLabURL_Settings_DefaultsToTheServiceName checks the
// fallback, the trimming, and that a configured address wins.
func TestResolveInternalGitLabURL_Settings_DefaultsToTheServiceName(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		want       string
	}{
		{name: "unset", configured: "", want: "http://gitlab-e2e"},
		{name: "blank", configured: "  ", want: "http://gitlab-e2e"},
		{name: "configured", configured: "http://gitlab.internal:8080", want: "http://gitlab.internal:8080"},
		{name: "trailing slash", configured: "http://gitlab.internal/", want: "http://gitlab.internal"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := resolveInternalGitLabURL(testCase.configured); got != testCase.want {
				t.Errorf("resolveInternalGitLabURL(%q) = %q, want %q", testCase.configured, got, testCase.want)
			}
		})
	}
}

// TestRepositoryURL_Settings_UsesTheInternalAddressWhenConfigured checks
// the mirror source: the internal address with the project path when there
// is one, the URL GitLab published otherwise.
func TestRepositoryURL_Settings_UsesTheInternalAddressWhenConfigured(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		path       string
		published  string
		want       string
	}{
		{name: "docker", configured: "http://gitlab-e2e/", path: "user/proj", published: "http://localhost:8929/user/proj.git", want: "http://gitlab-e2e/user/proj.git"},
		{name: "self-hosted", configured: "", path: "user/proj", published: "https://gitlab.example/user/proj.git", want: "https://gitlab.example/user/proj.git"},
		{name: "leading slash", configured: "http://gitlab-e2e", path: "/user/proj", published: "", want: "http://gitlab-e2e/user/proj.git"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := repositoryURL(testCase.configured, testCase.path, testCase.published); got != testCase.want {
				t.Errorf("repositoryURL() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestMirrorTargetURL_Token_EmbedsTheCredentialAsOAuth2 checks the push
// mirror target carries the token the way GitLab authenticates a push to
// itself, and that query and fragment never survive.
func TestMirrorTargetURL_Token_EmbedsTheCredentialAsOAuth2(t *testing.T) {
	got, err := mirrorTargetURL("http://gitlab-e2e/?x=1#frag", "glpat-secret", "/user/target")
	if err != nil {
		t.Fatalf("mirrorTargetURL() error = %v", err)
	}
	if want := "http://oauth2:glpat-secret@gitlab-e2e/user/target.git"; got != want {
		t.Errorf("mirrorTargetURL() = %q, want %q", got, want)
	}
}

// TestMirrorTargetURL_BadBase_IsRefused checks that an address with no host
// is an error rather than a URL GitLab would fail to push to later.
func TestMirrorTargetURL_BadBase_IsRefused(t *testing.T) {
	cases := []struct {
		name string
		base string
	}{
		{name: "no scheme", base: "gitlab-e2e"},
		{name: "unparseable", base: "http://[::1"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := mirrorTargetURL(testCase.base, "t", "p"); err == nil {
				t.Errorf("mirrorTargetURL(%q) error = nil, want a refusal", testCase.base)
			}
		})
	}
}

// TestFixtureServiceURL_Settings_JoinsWithoutDoubleSlashes checks the
// fixture service address and the join.
func TestFixtureServiceURL_Settings_JoinsWithoutDoubleSlashes(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		resource   string
		want       string
	}{
		{name: "default", configured: "", resource: "webhook", want: "http://e2e-fixture:8080/webhook"},
		{name: "both slashes", configured: "http://fixture:9000/", resource: "/emoji.png", want: "http://fixture:9000/emoji.png"},
		{name: "nested", configured: "http://fixture:9000", resource: "mirror/target.git", want: "http://fixture:9000/mirror/target.git"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := fixtureServiceURL(testCase.configured, testCase.resource); got != testCase.want {
				t.Errorf("fixtureServiceURL() = %q, want %q", got, testCase.want)
			}
		})
	}
}
