package gitlab

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestNewUnboundClient_AnswersTheInstanceClassAndRefusesRequests verifies
// the two properties the shared catalog relies on: the client reports the
// instance class its URL names, and every request through it, whether
// through client-go or through the raw health probe, fails with
// ErrUnboundClient instead of reaching a network.
func TestNewUnboundClient_AnswersTheInstanceClassAndRefusesRequests(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		baseURL    string
		wantDotCom bool
	}{
		{name: "gitlab.com", baseURL: "https://gitlab.com", wantDotCom: true},
		{name: "self-managed", baseURL: "https://gitlab.example.com", wantDotCom: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := NewUnboundClient(tc.baseURL)
			if got := client.IsGitLabDotCom(); got != tc.wantDotCom {
				t.Errorf("IsGitLabDotCom() = %t, want %t", got, tc.wantDotCom)
			}
			if _, _, err := client.GL().Version.GetVersion(); !errors.Is(err, ErrUnboundClient) {
				t.Errorf("GetVersion() error = %v, want ErrUnboundClient", err)
			}
			if _, err := client.Ping(context.Background()); !errors.Is(err, ErrUnboundClient) {
				t.Errorf("Ping() error = %v, want ErrUnboundClient", err)
			}
		})
	}
}

// TestNewUnboundClient_UnparsableURL_Panics verifies a base URL that does
// not parse is treated as the programming error it is.
func TestNewUnboundClient_UnparsableURL_Panics(t *testing.T) {
	t.Parallel()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("NewUnboundClient(\"://bad\") did not panic")
		}
		if message, _ := recovered.(string); !strings.Contains(message, "unbound client") {
			t.Errorf("panic = %v, want it to name the unbound client", recovered)
		}
	}()
	NewUnboundClient("://bad")
}

// TestClient_IsUnbound_SeparatesTheRefusingClientFromEveryOther covers the
// question a surface asks before it runs a handler.
//
// The resource, prompt and completion surfaces resolve the caller's client
// through [Client.For] and then ask this, so that a request nothing bound is
// refused with the reason rather than run against a transport that refuses
// everything and reported as whatever GitLab error came out. Only the client
// from [NewUnboundClient] answers yes: a real client is bound, and a nil one is
// no client at all, which every caller already handles separately.
func TestClient_IsUnbound_SeparatesTheRefusingClientFromEveryOther(t *testing.T) {
	t.Parallel()

	bound, err := NewClient(&config.Config{GitLabURL: "https://gitlab.example.com", GitLabToken: "glpat-x"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	cases := map[string]struct {
		client *Client
		want   bool
	}{
		"the credential-less client": {client: NewUnboundClient("https://gitlab.invalid"), want: true},
		"a client with a credential": {client: bound},
		"no client at all":           {},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tc.client.IsUnbound(); got != tc.want {
				t.Errorf("IsUnbound() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestClient_For_ResolvesTheBoundClientBeforeUnboundIsAsked pins the order the
// surfaces rely on: the question is asked of what For returned, so a request
// carrying a credential is never mistaken for one that carries none.
func TestClient_For_ResolvesTheBoundClientBeforeUnboundIsAsked(t *testing.T) {
	t.Parallel()

	unbound := NewUnboundClient("https://gitlab.invalid")
	bound, err := NewClient(&config.Config{GitLabURL: "https://gitlab.example.com", GitLabToken: "glpat-x"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if unbound.For(WithClient(context.Background(), bound)).IsUnbound() {
		t.Error("a request carrying a credential resolved to the unbound client")
	}
	if !unbound.For(context.Background()).IsUnbound() {
		t.Error("a request carrying nothing resolved to something other than the unbound client")
	}
}
