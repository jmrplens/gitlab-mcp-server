// exporters_test.go verifies what the startup summary is allowed to say about
// the collector it exports to.
package telemetry

import "testing"

// TestRedactEndpointUserinfo_CredentialsNeverReachTheSummary covers the one
// transform applied to an endpoint before anything displays it.
//
// The value is display-only — the exporters read the variable themselves — but
// the startup line that carries it is itself exported through the log bridge,
// so a password written into the endpoint URL would travel to the very
// collector it authenticates to, and then sit in whatever stores that.
// Everything else about the URL is kept, because an operator reading the line
// needs to recognize their own deployment in it.
func TestRedactEndpointUserinfo_CredentialsNeverReachTheSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "user and password are replaced together",
			endpoint: "https://user:hunter2@collector.example.com:4318/v1/traces",
			want:     "https://redacted@collector.example.com:4318/v1/traces",
		},
		{
			name:     "a bare username is still userinfo",
			endpoint: "https://user@collector.example.com",
			want:     "https://redacted@collector.example.com",
		},
		{
			name:     "an endpoint without credentials is untouched",
			endpoint: "https://collector.example.com:4318",
			want:     "https://collector.example.com:4318",
		},
		{
			name:     "an unparseable endpoint is returned as it was written",
			endpoint: "://not-a-url",
			want:     "://not-a-url",
		},
		{
			name:     "nothing configured stays nothing",
			endpoint: "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := redactEndpointUserinfo(tt.endpoint); got != tt.want {
				t.Errorf("redactEndpointUserinfo(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}
