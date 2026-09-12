package toolutil

import (
	"net/url"
	"strings"
	"testing"
)

// TestRedactURL_URLWithSecrets_DropsUserinfoQueryAndFragment verifies that the
// shared instance-URL renderer keeps only the scheme, host and path, and drops
// the userinfo, query and fragment.
//
// It matters because this value is handed to whichever MCP client called the
// tool and copied into whatever that client logs: a proxy credential written
// into GITLAB_URL would otherwise travel with every health report.
func TestRedactURL_URLWithSecrets_DropsUserinfoQueryAndFragment(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "userinfo with password",
			raw:  "https://proxyuser:s3cret@gitlab.example.com/gitlab",
			want: "https://gitlab.example.com/gitlab",
		},
		{
			name: "userinfo without password",
			raw:  "https://proxyuser@gitlab.example.com/",
			want: "https://gitlab.example.com/",
		},
		{
			name: "query and fragment",
			raw:  "https://gitlab.example.com/gitlab?private_token=glpat-abcdefghij#frag",
			want: "https://gitlab.example.com/gitlab",
		},
		{
			name: "plain url is unchanged",
			raw:  "https://gitlab.example.com",
			want: "https://gitlab.example.com",
		},
		{
			name: "port survives",
			raw:  "http://gitlab.internal:8443/sub/path",
			want: "http://gitlab.internal:8443/sub/path",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := url.Parse(tc.raw)
			if err != nil {
				t.Fatalf("url.Parse(%q) failed: %v", tc.raw, err)
			}
			got := RedactURL(parsed)
			if got != tc.want {
				t.Errorf("RedactURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			for _, secret := range []string{"s3cret", "glpat-abcdefghij", "proxyuser", "frag"} {
				if strings.Contains(got, secret) {
					t.Errorf("RedactURL(%q) = %q, must not contain %q", tc.raw, got, secret)
				}
			}
		})
	}
}

// TestRedactURL_NilURL_ReturnsEmpty verifies that the renderer answers a nil
// URL with an empty string rather than panicking, since a caller holding an
// unparsed base URL has nothing to report.
func TestRedactURL_NilURL_ReturnsEmpty(t *testing.T) {
	if got := RedactURL(nil); got != "" {
		t.Errorf("RedactURL(nil) = %q, want empty", got)
	}
}

// TestRedactURLToOrigin_CallbackURL_KeepsOnlyTheOrigin verifies that a webhook
// or integration URL is reduced to its scheme and host, with any path, query
// or fragment replaced by the elision marker.
//
// It matters because a callback URL's path is the secret: a hook endpoint is a
// public host plus an unguessable path, and the value is rendered into a
// prompt a model reads.
func TestRedactURLToOrigin_CallbackURL_KeepsOnlyTheOrigin(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "secret path",
			raw:  "https://hooks.example.com/services/T00000/B00000/XXXXXXXXXXXXXXXXXXXXXXXX",
			want: "https://hooks.example.com/...",
		},
		{
			name: "userinfo and short path",
			raw:  "https://hookuser:hookpass@hooks.example.com/a",
			want: "https://hooks.example.com/...",
		},
		{
			name: "query only",
			raw:  "https://hooks.example.com/?token=s3cret",
			want: "https://hooks.example.com/...",
		},
		{
			name: "fragment only",
			raw:  "https://hooks.example.com#s3cret",
			want: "https://hooks.example.com/...",
		},
		{
			name: "bare origin",
			raw:  "https://hooks.example.com",
			want: "https://hooks.example.com",
		},
		{
			name: "root path is not an elision",
			raw:  "https://hooks.example.com/",
			want: "https://hooks.example.com",
		},
		{
			name: "not a url",
			raw:  "not a url at all",
			want: RedactedPlaceholder,
		},
		{
			name: "scheme without host",
			raw:  "mailto:ops@example.com",
			want: RedactedPlaceholder,
		},
		{
			name: "empty",
			raw:  "",
			want: RedactedPlaceholder,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactURLToOrigin(tc.raw)
			if got != tc.want {
				t.Errorf("RedactURLToOrigin(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			for _, secret := range []string{"hookpass", "hookuser", "s3cret", "XXXXXXXXXXXXXXXXXXXXXXXX", "B00000"} {
				if strings.Contains(got, secret) {
					t.Errorf("RedactURLToOrigin(%q) = %q, must not contain %q", tc.raw, got, secret)
				}
			}
		})
	}
}

// TestRedactURLToOrigin_LongURL_DoesNotEchoItsPrefix verifies that the
// renderer never returns a leading slice of the input, which is how the
// webhook table used to hide a URL and is what leaked the userinfo of a long
// one.
func TestRedactURLToOrigin_LongURL_DoesNotEchoItsPrefix(t *testing.T) {
	raw := "https://hookuser:hookpass@hooks.example.com/services/very/long/secret/path"
	got := RedactURLToOrigin(raw)
	if strings.HasPrefix(raw, got) {
		t.Errorf("RedactURLToOrigin(%q) = %q, which is a prefix of the input", raw, got)
	}
	if got != "https://hooks.example.com/..." {
		t.Errorf("RedactURLToOrigin(%q) = %q, want %q", raw, got, "https://hooks.example.com/...")
	}
}
