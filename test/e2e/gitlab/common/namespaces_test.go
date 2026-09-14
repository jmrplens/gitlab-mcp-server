//go:build e2e

// namespaces_test.go covers the namespace reads the user tool serves, held
// to the one namespace every account is sure to have: its own, whose path
// is the username.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/namespaces"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// userNamespaceKind is how GitLab labels a personal namespace.
const userNamespaceKind = "user"

// TestNamespaces_Reads_FindTheRunUsersOwn lists, searches, checks and reads
// namespaces on every surface, and finds the run user's personal namespace
// through each: the listing narrowed to its name holds it, the search finds
// it, the existence check reports its path taken, and the read by path
// answers with a user namespace of that path.
//
// Replaces: TestMeta_UserNamespacesNotifications
func TestNamespaces_Reads_FindTheRunUsersOwn(t *testing.T) {
	e := harness.New(t)
	rt := e.Runtime()

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		listed := harness.Do[namespaces.ListOutput](s, actionUserNamespaceList, map[string]any{"search": rt.Username})
		if !namesPath(listed.Namespaces, rt.Username) {
			e.T.Errorf("namespace_list narrowed to %q does not hold the run user's own namespace: %+v", rt.Username, listed.Namespaces)
		}

		searched := harness.Do[namespaces.ListOutput](s, actionUserNamespaceSearch, map[string]any{"query": rt.Username})
		if !namesPath(searched.Namespaces, rt.Username) {
			e.T.Errorf("namespace_search for %q does not hold the run user's own namespace: %+v", rt.Username, searched.Namespaces)
		}

		exists := harness.Do[namespaces.ExistsOutput](s, actionUserNamespaceExists, map[string]any{"id": rt.Username})
		if !exists.Exists {
			e.T.Errorf("namespace_exists reports %q free, and it is the run user's own", rt.Username)
		}

		got := harness.Do[namespaces.Output](s, actionUserNamespaceGet, map[string]any{"id": rt.Username})
		if got.ID == 0 || got.Path != rt.Username || got.Kind != userNamespaceKind {
			e.T.Errorf("namespace_get(%q) answered id %d path %q kind %q, want the user namespace of that path", rt.Username, got.ID, got.Path, got.Kind)
		}
	})
}

// namesPath reports whether a listing holds a namespace of the given path.
func namesPath(listed []namespaces.Output, path string) bool {
	return slices.ContainsFunc(listed, func(namespace namespaces.Output) bool { return namespace.Path == path })
}
