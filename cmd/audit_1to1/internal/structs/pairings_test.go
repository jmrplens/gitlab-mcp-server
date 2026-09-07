package structs

import (
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// TestCollectOutputPairings_Repository_NamesTheSDKStructBehindEachOutput
// verifies the first link of the type-grain shape join. The paths scope asks
// GitLab's document what the endpoints one output type models actually send,
// and the only thing that says which endpoints those are is the client-go
// struct a converter fills the type from. A pairing lost here reads downstream
// as a type nothing routes to, which is silence rather than a finding.
func TestCollectOutputPairings_Repository_NamesTheSDKStructBehindEachOutput(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}

	pairings, err := CollectOutputPairings(root)
	if err != nil {
		t.Fatalf("collect the pairings: %v", err)
	}

	if pairings.ClientGoDir == "" {
		t.Fatal("no client-go directory found in the import graph the handlers compile against")
	}
	if _, statErr := filepath.Glob(filepath.Join(pairings.ClientGoDir, "*.go")); statErr != nil {
		t.Errorf("client-go directory %q does not read: %v", pairings.ClientGoDir, statErr)
	}
	if len(pairings.Outputs) < 100 {
		t.Fatalf("collected %d pairing(s), want the converters across the whole tree", len(pairings.Outputs))
	}

	found := false
	for _, pairing := range pairings.Outputs {
		if strings.Contains(pairing.SDKType, ".") {
			t.Errorf("%s.%s names the SDK struct as %q, want it unqualified", pairing.Package, pairing.MCPType, pairing.SDKType)
		}
		if pairing.Package == "mrapprovals" && pairing.MCPType == "ConfigOutput" {
			found = true
			if pairing.SDKType != "MergeRequestApprovals" {
				t.Errorf("ConfigOutput pairs with %q, want MergeRequestApprovals", pairing.SDKType)
			}
		}
	}
	if !found {
		t.Error("no pairing for mrapprovals.ConfigOutput, the one confirmed phantom this join was built for")
	}
}

// TestCollectOutputPairings_ATreeThatDoesNotLoad_IsReported verifies that a
// load failure comes back as an error rather than as an empty pass. The scope
// above skips its whole type-grain join on one, and a nil error with no
// pairings would tell it there was nothing to compare.
func TestCollectOutputPairings_ATreeThatDoesNotLoad_IsReported(t *testing.T) {
	pairings, err := CollectOutputPairings(filepath.Join(t.TempDir(), "absent"))

	if err == nil {
		t.Fatalf("CollectOutputPairings() = %+v, nil; want the load failure reported", pairings)
	}
}

// clientGoPackage builds a package whose scope declares the Client struct,
// which is what identifies the client-go root.
func clientGoPackage(t *testing.T, path string, files []string) *packages.Package {
	t.Helper()
	typed := types.NewPackage(path, "gitlab")
	named := types.NewNamed(types.NewTypeName(token.NoPos, typed, "Client", nil), types.NewStruct(nil, nil), nil)
	typed.Scope().Insert(named.Obj())
	typed.MarkComplete()
	return &packages.Package{PkgPath: path, Types: typed, GoFiles: files}
}

// TestClientGoDir_FindsTheRootByItsClientStruct verifies how the SDK source is
// located. The import path carries a major-version suffix that moves with every
// major release and several client-go packages share its prefix, so the root is
// identified by the one declaration only it has. A wrong directory here parses
// no routes at all, which the join reads as an SDK nothing routes through.
func TestClientGoDir_FindsTheRootByItsClientStruct(t *testing.T) {
	const rootPath = shared.ClientGoPkgPath + "/v2"
	other := types.NewPackage(shared.ClientGoPkgPath+"/v2/testing", "testing")
	other.MarkComplete()

	cases := []struct {
		name    string
		imports map[string]*packages.Package
		want    string
	}{
		{
			name:    "the root, among its siblings",
			imports: map[string]*packages.Package{rootPath: clientGoPackage(t, rootPath, []string{"/cache/client-go/projects.go"})},
			want:    "/cache/client-go",
		},
		{name: "no imports at all"},
		{
			name:    "an import from somewhere else entirely",
			imports: map[string]*packages.Package{"net/http": {PkgPath: "net/http", GoFiles: []string{"/go/http.go"}}},
		},
		{
			name:    "a client-go package with no type information",
			imports: map[string]*packages.Package{rootPath: {PkgPath: rootPath, GoFiles: []string{"/cache/client-go/projects.go"}}},
		},
		{
			name:    "a client-go package that is not the root",
			imports: map[string]*packages.Package{other.Path(): {PkgPath: other.Path(), Types: other, GoFiles: []string{"/cache/client-go/testing/x.go"}}},
		},
		{
			name:    "the root with no files on disk",
			imports: map[string]*packages.Package{rootPath: clientGoPackage(t, rootPath, nil)},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := clientGoDir([]*packages.Package{{PkgPath: "tools", Imports: testCase.imports}})

			if got != testCase.want {
				t.Errorf("clientGoDir() = %q, want %q", got, testCase.want)
			}
		})
	}
}
