package gitlab

import (
	"context"
	"errors"
	"strings"
	"testing"
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
