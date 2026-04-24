// roots_test.go contains unit and integration tests for the roots package.
// Unit tests verify [Manager] CRUD operations, [HasRoot] lookups, [FindGitRoot]
// heuristics, and [isGitRoot] pattern matching.
// Integration tests use in-memory MCP transports to verify [Manager.Refresh]
// and [ListClientRoots] against real client sessions.

package roots

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestNewManager verifies that [NewManager] returns a non-nil [Manager] with
// no initial roots.
func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if got := m.GetRoots(); got != nil {
		t.Errorf("expected nil roots on new manager, got %d", len(got))
	}
}

// TestSetAnd_GetRoots verifies that [Manager.setRoots] stores roots correctly
// and [Manager.GetRoots] returns an independent copy of the stored slice.
func TestSetAnd_GetRoots(t *testing.T) {
	m := NewManager()
	testRoots := []*mcp.Root{
		{URI: "file:///home/user/project1", Name: "project1"},
		{URI: "file:///home/user/project2", Name: "project2"},
	}
	m.setRoots(testRoots)

	got := m.GetRoots()
	if len(got) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(got))
	}
	if got[0].URI != "file:///home/user/project1" {
		t.Errorf("unexpected root[0]: %s", got[0].URI)
	}

	// Verify GetRoots returns a copy, not the original slice
	got[0] = nil
	original := m.GetRoots()
	if original[0] == nil {
		t.Error("GetRoots should return a copy, not the original slice")
	}
}

// TestSetRoots_ClearsOnNil verifies that setting nil roots clears previously
// stored roots.
func TestSetRoots_ClearsOnNil(t *testing.T) {
	m := NewManager()
	m.setRoots([]*mcp.Root{{URI: "file:///tmp"}})

	m.setRoots(nil)
	if got := m.GetRoots(); got != nil {
		t.Errorf("expected nil after clearing, got %d roots", len(got))
	}
}

// TestGetRoots_Empty verifies that setting an empty slice results in nil
// from [Manager.GetRoots].
func TestGetRoots_Empty(t *testing.T) {
	m := NewManager()
	m.setRoots([]*mcp.Root{})
	got := m.GetRoots()
	if got != nil {
		t.Errorf("expected nil for empty slice, got %d", len(got))
	}
}

// TestHasRoot uses table-driven subtests to verify that [Manager.HasRoot]
// correctly identifies existing and non-existing root URIs.
func TestHasRoot(t *testing.T) {
	m := NewManager()
	m.setRoots([]*mcp.Root{
		{URI: "file:///home/user/project"},
		{URI: "file:///home/user/other"},
	})

	tests := []struct {
		name string
		uri  string
		want bool
	}{
		{"existing root", "file:///home/user/project", true},
		{"another existing", "file:///home/user/other", true},
		{"non-existing", "file:///home/user/missing", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.HasRoot(tt.uri); got != tt.want {
				t.Errorf("HasRoot(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

// TestHasRoot_EmptyManager verifies that [Manager.HasRoot] returns false
// when no roots have been set.
func TestHasRoot_EmptyManager(t *testing.T) {
	m := NewManager()
	if m.HasRoot("file:///anything") {
		t.Error("empty manager should return false for any URI")
	}
}

// TestFindGitRoot uses table-driven subtests to verify that [Manager.FindGitRoot]
// identifies Git repository roots via path heuristics (.git suffix, /repos/,
// /repositories/, /git/ segments) and returns the first match.
func TestFindGitRoot(t *testing.T) {
	tests := []struct {
		name      string
		roots     []*mcp.Root
		wantURI   string
		wantFound bool
	}{
		{
			name:      "empty roots",
			roots:     nil,
			wantURI:   "",
			wantFound: false,
		},
		{
			name: "no git root",
			roots: []*mcp.Root{
				{URI: "file:///home/user/documents"},
				{URI: "file:///tmp/data"},
			},
			wantURI:   "",
			wantFound: false,
		},
		{
			name: ".git suffix",
			roots: []*mcp.Root{
				{URI: "file:///home/user/project/.git"},
			},
			wantURI:   "file:///home/user/project/.git",
			wantFound: true,
		},
		{
			name: "repos path indicator",
			roots: []*mcp.Root{
				{URI: "file:///home/user/repos/my-project"},
			},
			wantURI:   "file:///home/user/repos/my-project",
			wantFound: true,
		},
		{
			name: "repositories path indicator",
			roots: []*mcp.Root{
				{URI: "file:///opt/repositories/backend"},
			},
			wantURI:   "file:///opt/repositories/backend",
			wantFound: true,
		},
		{
			name: "git path indicator",
			roots: []*mcp.Root{
				{URI: "file:///home/user/git/my-repo"},
			},
			wantURI:   "file:///home/user/git/my-repo",
			wantFound: true,
		},
		{
			name: "first match wins",
			roots: []*mcp.Root{
				{URI: "file:///tmp/data"},
				{URI: "file:///home/user/repos/first"},
				{URI: "file:///home/user/repos/second"},
			},
			wantURI:   "file:///home/user/repos/first",
			wantFound: true,
		},
		{
			name: "windows style path with .git",
			roots: []*mcp.Root{
				{URI: "file:///C:/Users/dev/project/.git"},
			},
			wantURI:   "file:///C:/Users/dev/project/.git",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager()
			m.setRoots(tt.roots)

			gotURI, gotFound := m.FindGitRoot()
			if gotFound != tt.wantFound {
				t.Errorf("FindGitRoot() found = %v, want %v", gotFound, tt.wantFound)
			}
			if gotURI != tt.wantURI {
				t.Errorf("FindGitRoot() uri = %q, want %q", gotURI, tt.wantURI)
			}
		})
	}
}

// TestIsGitRoot uses table-driven subtests to verify that [isGitRoot] correctly
// identifies root URIs that indicate a Git repository, including .git paths,
// /repos/, /repositories/, and /git/ segments with case-insensitive matching.
func TestIsGitRoot(t *testing.T) {
	tests := []struct {
		name string
		root *mcp.Root
		want bool
	}{
		{"nil root", nil, false},
		{"empty URI", &mcp.Root{URI: ""}, false},
		{"invalid URI", &mcp.Root{URI: "://bad"}, false},
		{"plain directory", &mcp.Root{URI: "file:///home/user/docs"}, false},
		{".git directory", &mcp.Root{URI: "file:///project/.git"}, true},
		{"ends with .git", &mcp.Root{URI: "file:///code/.git"}, true},
		{"/repos/ path", &mcp.Root{URI: "file:///home/repos/project"}, true},
		{"/repositories/ path", &mcp.Root{URI: "file:///opt/repositories/api"}, true},
		{"/git/ path", &mcp.Root{URI: "file:///home/git/backend"}, true},
		{"case insensitive repos", &mcp.Root{URI: "file:///home/Repos/project"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGitRoot(tt.root); got != tt.want {
				t.Errorf("isGitRoot(%v) = %v, want %v", tt.root, got, tt.want)
			}
		})
	}
}

// TestRefresh_NilSession verifies that [Manager.Refresh] clears roots and
// returns no error when given a nil session.
func TestRefresh_NilSession(t *testing.T) {
	m := NewManager()
	m.setRoots([]*mcp.Root{{URI: "file:///old"}})

	err := m.Refresh(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error for nil session, got: %v", err)
	}
	if got := m.GetRoots(); got != nil {
		t.Errorf("expected roots cleared on nil session, got %d", len(got))
	}
}

// setupInMemorySession creates a connected client+server pair via in-memory
// transports. The client declares the given roots. Returns the ServerSession
// that can call ListRoots on the client.
func setupInMemorySession(t *testing.T, clientRoots []*mcp.Root) *mcp.ServerSession {
	t.Helper()
	ctx := context.Background()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	client.AddRoots(clientRoots...)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})
	return serverSession
}

// TestRefresh_WithSession verifies that [Manager.Refresh] fetches and stores
// roots from a live in-memory MCP client session.
func TestRefresh_WithSession(t *testing.T) {
	testRoots := []*mcp.Root{
		{URI: "file:///home/user/project", Name: "project"},
		{URI: "file:///home/user/repos/backend", Name: "backend"},
	}
	ss := setupInMemorySession(t, testRoots)

	m := NewManager()
	err := m.Refresh(context.Background(), ss)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got := m.GetRoots()
	if len(got) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(got))
	}
	uris := map[string]bool{}
	for _, r := range got {
		uris[r.URI] = true
	}
	for _, want := range testRoots {
		if !uris[want.URI] {
			t.Errorf("missing root URI: %s", want.URI)
		}
	}
}

// TestRefresh_WithCancelledContext verifies that [Manager.Refresh] returns an
// error when the context is already canceled.
func TestRefresh_WithCancelledContext(t *testing.T) {
	ss := setupInMemorySession(t, []*mcp.Root{{URI: "file:///tmp"}})

	m := NewManager()
	ctx := testutil.CancelledCtx(t) // cancel immediately

	err := m.Refresh(ctx, ss)
	if err == nil {
		t.Error("expected error with canceled context")
	}
}

// TestListClientRoots_NilSession verifies that [ListClientRoots] returns nil
// roots and no error when given a nil session.
func TestListClientRoots_NilSession(t *testing.T) {
	roots, err := ListClientRoots(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error for nil session, got: %v", err)
	}
	if roots != nil {
		t.Errorf("expected nil roots for nil session, got %d", len(roots))
	}
}

// TestListClientRoots_WithSession verifies that [ListClientRoots] fetches
// roots from a live in-memory MCP client session.
func TestListClientRoots_WithSession(t *testing.T) {
	testRoots := []*mcp.Root{
		{URI: "file:///workspace/app", Name: "app"},
	}
	ss := setupInMemorySession(t, testRoots)

	got, err := ListClientRoots(context.Background(), ss)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 root, got %d", len(got))
	}
	if got[0].URI != "file:///workspace/app" {
		t.Errorf("unexpected URI: %s", got[0].URI)
	}
}

// TestListClientRoots_CancelledContext verifies that [ListClientRoots] returns
// an error when the context is already canceled.
func TestListClientRoots_CancelledContext(t *testing.T) {
	ss := setupInMemorySession(t, []*mcp.Root{{URI: "file:///tmp"}})

	ctx := testutil.CancelledCtx(t)

	_, err := ListClientRoots(ctx, ss)
	if err == nil {
		t.Error("expected error with canceled context")
	}
}

// TestManager_ConcurrentAccess exercises concurrent reads and writes on
// [Manager] to verify thread-safety of the internal mutex.
func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager()
	testRoots := []*mcp.Root{
		{URI: "file:///home/user/project", Name: "project"},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			m.setRoots(testRoots)
		}
	}()

	for range 100 {
		_ = m.GetRoots()
		_ = m.HasRoot("file:///home/user/project")
		_, _ = m.FindGitRoot()
	}
	<-done
}

// TestSetRootsForTest verifies that the exported [Manager.SetRootsForTest]
// helper correctly delegates to the internal setRoots method.
func TestSetRootsForTest(t *testing.T) {
	m := NewManager()
	testRoots := []*mcp.Root{
		{URI: "file:///test/project", Name: "test"},
	}
	m.SetRootsForTest(testRoots)

	got := m.GetRoots()
	if len(got) != 1 {
		t.Fatalf("expected 1 root, got %d", len(got))
	}
	if got[0].URI != "file:///test/project" {
		t.Errorf("root URI = %q, want %q", got[0].URI, "file:///test/project")
	}
}
