package auditclient

import (
	"context"
	"errors"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// TestNewMock_ReturnsPingableClient verifies the audit client helper exposes a
// local GitLab version endpoint and a cleanup function.
func TestNewMock_ReturnsPingableClient(t *testing.T) {
	client, cleanup, err := NewMock()
	if err != nil {
		t.Fatalf("NewMock() unexpected error: %v", err)
	}
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

	client, cleanup, err := NewMock()
	if err == nil {
		t.Fatal("NewMock() expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("NewMock() error = %v, want wrapping %v", err, wantErr)
	}
	if client != nil {
		t.Fatalf("client = %#v, want nil", client)
	}
	if cleanup != nil {
		t.Fatal("cleanup is non-nil, want nil")
	}
}
