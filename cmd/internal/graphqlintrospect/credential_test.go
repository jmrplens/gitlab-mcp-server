package graphqlintrospect_test

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/graphqlintrospect"
)

// TestCredentialFor_DecidesByInstance verifies the rule that keeps a GitLab
// credential from following a command-line flag.
//
// Both commands that introspect take their endpoint from a flag and read
// GITLAB_TOKEN from the environment beside it. Sending the two together
// unconditionally makes every mistyped host, and every host somebody else
// chose, a place the operator's token arrives; the token buys only the version
// string, since GitLab answers introspection to anyone. Each case below is one
// way an endpoint can fail to be the instance the token belongs to, and every
// refusal must also say why, because the version then comes back unknown and
// the two causes are not equally interesting.
func TestCredentialFor_DecidesByInstance(t *testing.T) {
	const token = "glpat-secret"

	cases := []struct {
		name     string
		endpoint string
		instance string
		token    string
		want     string
		reason   string
	}{
		{
			name:     "the endpoint is the instance",
			endpoint: "https://gitlab.com/api/graphql",
			instance: "https://gitlab.com",
			token:    token,
			want:     token,
		},
		{
			name:     "the instance carries a trailing path and a different case",
			endpoint: "https://GitLab.example.com/api/graphql",
			instance: "https://gitlab.example.com/",
			token:    token,
			want:     token,
		},
		{
			name:     "another host entirely",
			endpoint: "https://gitlab.gnome.org/api/graphql",
			instance: "https://gitlab.com",
			token:    token,
			reason:   "and this run asks https://gitlab.gnome.org",
		},
		{
			name:     "the same host on another port",
			endpoint: "https://gitlab.example.com:8443/api/graphql",
			instance: "https://gitlab.example.com",
			token:    token,
			reason:   "and this run asks https://gitlab.example.com:8443",
		},
		{
			name:     "the same host without the encryption",
			endpoint: "http://gitlab.example.com/api/graphql",
			instance: "https://gitlab.example.com",
			token:    token,
			reason:   "and this run asks http://gitlab.example.com",
		},
		{
			name:     "no instance named",
			endpoint: "https://gitlab.com/api/graphql",
			instance: "   ",
			token:    token,
			reason:   "GITLAB_URL is not",
		},
		{
			name:     "an endpoint that is not a URL",
			endpoint: "gitlab.example.com/api/graphql",
			instance: "https://gitlab.example.com",
			token:    token,
			reason:   "is not a URL, so GITLAB_TOKEN was not sent",
		},
		{
			name:     "an instance that is not a URL",
			endpoint: "https://gitlab.example.com/api/graphql",
			instance: "gitlab.example.com",
			token:    token,
			reason:   `GITLAB_URL is "gitlab.example.com", which is not a URL`,
		},
		{
			name:     "an endpoint nothing can parse",
			endpoint: "https://gitlab.example.com/\x7f",
			instance: "https://gitlab.example.com",
			token:    token,
			reason:   "is not a URL, so GITLAB_TOKEN was not sent",
		},
		{
			name:     "an instance nothing can parse",
			endpoint: "https://gitlab.example.com/api/graphql",
			instance: "https://gitlab.example.com/\x7f",
			token:    token,
			reason:   "which is not a URL",
		},
		{
			name:     "no token to withhold",
			endpoint: "https://gitlab.gnome.org/api/graphql",
			instance: "https://gitlab.com",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			credential, withheld := graphqlintrospect.CredentialFor(testCase.endpoint, testCase.instance, testCase.token)

			if credential != testCase.want {
				t.Errorf("CredentialFor() sent %q, want %q", credential, testCase.want)
			}
			switch {
			case testCase.reason == "" && withheld != "":
				t.Errorf("CredentialFor() withheld nothing but explained anyway: %q", withheld)
			case testCase.reason != "" && !strings.Contains(withheld, testCase.reason):
				t.Errorf("CredentialFor() does not say why it withheld the token, want %q in:\n%s", testCase.reason, withheld)
			}
		})
	}
}
