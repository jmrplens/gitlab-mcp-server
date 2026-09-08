package requestinventory

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// makeToolsPackage creates internal/tools/<name> under root, which is how the
// classification tells an owner that recorded nothing from an owner that is no
// package at all.
func makeToolsPackage(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(ToolsDir), name), 0o750); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", name, err)
	}
}

// TestClassify_Owners_AreSplitThreeWays verifies the three counts stay
// disjoint and mean what they say: an owner that recorded something, one that
// exists and recorded nothing, and one that is not a package under
// internal/tools at all.
func TestClassify_Owners_AreSplitThreeWays(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	makeToolsPackage(t, root, "adminspecs")
	rows := []Row{
		{Package: "internal/tools/issues", Path: "/projects/:id/issues"},
		{Package: "internal/completions", Path: "/groups"},
	}
	actions := []Action{
		{ID: "issue.list", Owner: "issues"},
		{ID: "issue.get", Owner: "issues"},
		{ID: "topic.list", Owner: "adminspecs"},
		{ID: "ghost.list", Owner: "nowhere"},
	}

	coverage := Classify(root, rows, actions)

	if coverage.Total != 4 || coverage.Covered != 2 || coverage.Silent != 1 || coverage.Unmapped != 1 {
		t.Fatalf("coverage = %+v, want 4 actions split 2/1/1", coverage)
	}
	if !slices.Equal(Packages(coverage.SilentOwners), []string{"adminspecs"}) {
		t.Errorf("silent owners = %v, want [adminspecs]", Packages(coverage.SilentOwners))
	}
	if !slices.Equal(Packages(coverage.UnmappedOwners), []string{"nowhere"}) {
		t.Errorf("unmapped owners = %v, want [nowhere]", Packages(coverage.UnmappedOwners))
	}
	t.Run("an owner names the actions behind its count", func(t *testing.T) {
		if !slices.Equal(coverage.SilentOwners[0].Actions, []string{"topic.list"}) {
			t.Errorf("silent owner actions = %v, want [topic.list]", coverage.SilentOwners[0].Actions)
		}
	})
}

// TestClassify_TheRootOwner_ResolvesToTheOrchestrationPackage verifies the one
// owner that is not a directory under internal/tools.
//
// The catalog gives a spec group that declares none the owner "tools", and
// TestCollectedActionSpecs_DeclareCatalogOwnership admits it beside the domain
// names, so it has to resolve to internal/tools itself. Calling it unmapped
// would report a catalog-metadata defect that is not one, and the gate now
// fails on unmapped owners.
func TestClassify_TheRootOwner_ResolvesToTheOrchestrationPackage(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")

	t.Run("covered when the root package recorded something", func(t *testing.T) {
		coverage := Classify(root,
			[]Row{{Package: ToolsDir, Path: "/projects/:project_id"}},
			[]Action{{ID: "discover.project", Owner: RootOwner}})

		if coverage.Covered != 1 || coverage.Unmapped != 0 {
			t.Errorf("coverage = %+v, want the root owner covered", coverage)
		}
	})

	t.Run("silent, not unmapped, when it recorded nothing", func(t *testing.T) {
		coverage := Classify(root,
			[]Row{{Package: "internal/tools/issues", Path: "/projects/:project_id/issues"}},
			[]Action{{ID: "discover.project", Owner: RootOwner}})

		if coverage.Silent != 1 || coverage.Unmapped != 0 {
			t.Errorf("coverage = %+v, want the root owner silent rather than unmapped", coverage)
		}
	})
}

// TestClassify_SeveralOwnersAndActions_AreSortedForAStableReport verifies the
// order, since a report that reshuffles between runs turns every audit into a
// diff nobody can read.
func TestClassify_SeveralOwnersAndActions_AreSortedForAStableReport(t *testing.T) {
	root := t.TempDir()

	coverage := Classify(root, nil, []Action{
		{ID: "zeta.get", Owner: "second"},
		{ID: "alpha.get", Owner: "second"},
		{ID: "beta.get", Owner: "first"},
	})

	if !slices.Equal(Packages(coverage.UnmappedOwners), []string{"first", "second"}) {
		t.Fatalf("owners = %v, want them sorted", Packages(coverage.UnmappedOwners))
	}
	if !slices.Equal(coverage.UnmappedOwners[1].Actions, []string{"alpha.get", "zeta.get"}) {
		t.Errorf("actions = %v, want them sorted", coverage.UnmappedOwners[1].Actions)
	}
}

// TestActions_TheRealCatalog_NamesAnOwnerForEveryAction verifies the default
// implementation against the catalog this binary carries: every action has an
// owning package, or every count built on it is counting something else.
func TestActions_TheRealCatalog_NamesAnOwnerForEveryAction(t *testing.T) {
	actions, err := Actions()
	if err != nil {
		t.Fatalf("Actions() error = %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("Actions() returned nothing")
	}
	for _, action := range actions {
		if action.Owner == "" {
			t.Fatalf("action %q of the %d in the catalog has no owning package", action.ID, len(actions))
		}
	}
}

// TestActions_ACatalogThatWillNotBuild_IsReported verifies that a caller is
// told rather than handed an empty action list, which would score every
// package as covering nothing it owns.
func TestActions_ACatalogThatWillNotBuild_IsReported(t *testing.T) {
	original := buildCatalog
	buildCatalog = func(*gitlabclient.Client, tools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		return nil, errors.New("catalog is broken")
	}
	t.Cleanup(func() { buildCatalog = original })

	actions, err := Actions()

	if err == nil {
		t.Fatal("Actions() error = nil, want the catalog failure")
	}
	if actions != nil {
		t.Errorf("Actions() = %v, want nothing on failure", actions)
	}
}

// TestDirectoryExists_Path_IsADirectoryOrNot verifies the check that tells an
// owner package that recorded nothing from a name that is no package.
func TestDirectoryExists_Path_IsADirectoryOrNot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"a directory", root, true},
		{"a regular file", file, false},
		{"nothing at all", filepath.Join(root, "absent"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := directoryExists(tt.path); got != tt.want {
				t.Errorf("directoryExists(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}
