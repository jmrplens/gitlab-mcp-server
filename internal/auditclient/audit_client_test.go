package auditclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// TestNewMock_ReturnsPingableClient verifies the audit client helper exposes a
// local GitLab version endpoint and a cleanup function.
func TestNewMock_ReturnsPingableClient(t *testing.T) {
	client, cleanup := NewMock()
	t.Cleanup(cleanup)

	version, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping() unexpected error: %v", err)
	}
	if version != "17.0.0" {
		t.Fatalf("version = %q, want 17.0.0", version)
	}
}

// TestNewMock_ClientCreationError verifies the helper closes its local server
// and reports client construction failures.
func TestNewMock_ClientCreationError(t *testing.T) {
	original := newGitLabClient
	t.Cleanup(func() { newGitLabClient = original })

	wantErr := errors.New("boom")
	newGitLabClient = func(*config.Config) (*gitlabclient.Client, error) {
		return nil, wantErr
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("NewMock() returned normally, want a panic when the client cannot be built")
		}
		message, ok := recovered.(string)
		if !ok {
			t.Fatalf("panic value = %#v, want a string", recovered)
		}
		if !strings.Contains(message, wantErr.Error()) {
			t.Errorf("panic = %q, want it to carry %q", message, wantErr.Error())
		}
		if !strings.Contains(message, "auditclient") {
			t.Errorf("panic = %q, want it to name the package that failed", message)
		}
	}()

	NewMock()
}
