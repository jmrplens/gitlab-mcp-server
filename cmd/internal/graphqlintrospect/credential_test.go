package graphqlintrospect_test

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqlintrospect"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
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
// the two causes are not equally interesting. A value that is not a URL is
// quoted as the operator wrote it, and it is that value rather than the other
// one beside it, since it is the one they have to correct.
//
// A GITLAB_URL padded with whitespace is still the instance it names: the
// padding is what an env file or a quoted shell line leaves behind, and
// refusing it would withhold the token from the one instance it belongs to.
//
// The last two cases hold the two halves of "names no scheme and host" apart.
// A string missing both reads the same whichever way that test is written, so
// a value with one and not the other is the only thing that tells a demand for
// both from a demand for either: a bare host would otherwise be reduced to an
// origin of "://host" and compared as if it were one, which turns a refusal
// into a mismatch and quietly changes what the operator is told.
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
			name:     "an instance written with the whitespace a dotenv file leaves around it",
			endpoint: "https://gitlab.example.com/api/graphql",
			instance: " https://gitlab.example.com\n",
			token:    token,
			want:     token,
		},
		{
			name:     "an endpoint that is not a URL",
			endpoint: "gitlab.example.com/api/graphql",
			instance: "https://gitlab.example.com",
			token:    token,
			reason:   `the endpoint "gitlab.example.com/api/graphql" is not a URL, so GITLAB_TOKEN was not sent`,
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
		{
			name:     "a scheme with no host behind it",
			endpoint: "https:///api/graphql",
			instance: "https://gitlab.example.com",
			token:    token,
			reason:   "is not a URL, so GITLAB_TOKEN was not sent",
		},
		{
			name:     "a host with no scheme in front of it",
			endpoint: "//gitlab.example.com/api/graphql",
			instance: "https://gitlab.example.com",
			token:    token,
			reason:   "is not a URL, so GITLAB_TOKEN was not sent",
		},
		// net/http sends a request for this host to xn--gitlab-qyd.example.com,
		// which is not the instance whatever strings.ToLower makes of the
		// spelling: only ASCII letters may be folded.
		{
			name:     "a lookalike written with a dotted capital I",
			endpoint: "https://gİtlab.example.com/api/graphql",
			instance: "https://gitlab.example.com",
			token:    token,
			reason:   "and this run asks https://gİtlab.example.com",
		},
		{
			name:     "a lookalike percent-encoded",
			endpoint: "https://G%C4%B0TLAB.example.com/api/graphql",
			instance: "https://gitlab.example.com",
			token:    token,
			reason:   "and this run asks https://gİtlab.example.com",
		},
		{
			name:     "an instance written with a dotted capital I and asked as written",
			endpoint: "https://g%C4%B0tlab.example.com/api/graphql",
			instance: "https://GİTLAB.example.com",
			token:    token,
			want:     token,
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

// TestFoldASCIICase_IsTheServersFold holds the commands' copy of the host fold
// to the server's own, gitlab.FoldHostCase.
//
// The copy exists so that the two commands that introspect do not link the
// server's GitLab client package for ten lines; this test is what keeps it a
// copy rather than a second rule. Both folds work byte by byte, so every
// single byte is compared on its own, and then whole hosts carrying the two
// runes strings.ToLower would have folded into ASCII and a byte that is not
// UTF-8.
func TestFoldASCIICase_IsTheServersFold(t *testing.T) {
	for b := range 256 {
		host := string([]byte{byte(b)})
		if got, want := graphqlintrospect.FoldASCIICaseForTest(host), gitlabclient.FoldHostCase(host); got != want {
			t.Errorf("byte %#02x: the commands fold it to %q, the server to %q", b, got, want)
		}
	}
	for _, host := range []string{
		"",
		"GitLab.Example.COM:8443",
		"GİTLAB.example.com",
		"KUBE.example.com",
		"G\xc4TLAB",
		"[FD00:EC2::254]:443",
	} {
		t.Run(host, func(t *testing.T) {
			if got, want := graphqlintrospect.FoldASCIICaseForTest(host), gitlabclient.FoldHostCase(host); got != want {
				t.Errorf("the commands fold %q to %q, the server to %q", host, got, want)
			}
		})
	}
}
