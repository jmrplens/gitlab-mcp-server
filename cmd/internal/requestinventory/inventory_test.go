package requestinventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeInventory lays out a repository root holding the committed artifact
// with the given content.
func writeInventory(t *testing.T, content []byte) string {
	t.Helper()
	root := t.TempDir()
	target := filepath.Join(root, filepath.FromSlash(Path))
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	return root
}

// TestRead_TheCommittedArtifact_ComesBackAsRows verifies the round trip both
// callers depend on, through the renderer that writes the file.
func TestRead_TheCommittedArtifact_ComesBackAsRows(t *testing.T) {
	root := writeInventory(t, Render([]Row{
		{Package: "internal/tools/issues", Kind: KindREST, Method: "GET", Path: "/projects/:id/issues", Query: []string{"state"}},
		{Package: "internal/tools/epics", Kind: KindGraphQL, Method: "POST", Path: "/graphql", Operation: "query epicList", Variables: []string{"path"}},
	}))

	inventory, err := Read(root)
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if len(inventory.Requests) != 2 {
		t.Fatalf("Read() returned %d row(s), want 2", len(inventory.Requests))
	}
	if inventory.Requests[0].Query[0] != "state" || inventory.Requests[1].Operation != "query epicList" {
		t.Errorf("Read() returned %+v, want both rows whole", inventory.Requests)
	}
	if inventory.Note == "" {
		t.Error("Read() returned an inventory with no note saying where it comes from")
	}
}

// TestRead_AnArtifactItCannotUse_Fails verifies the three ways the file is no
// answer at all. An empty one is the dangerous case: every caller asks "which
// of these did the suite never issue", and an empty file answers all of them
// with "none of them".
func TestRead_AnArtifactItCannotUse_Fails(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
		absent  bool
		want    string
	}{
		{name: "no artifact at all", absent: true, want: "read the request inventory"},
		{name: "an artifact that is not JSON", content: []byte("not json"), want: "parse "},
		{name: "an artifact with no rows", content: []byte(`{"note":"x","requests":[]}`), want: "holds no request"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			if !testCase.absent {
				root = writeInventory(t, testCase.content)
			}

			_, err := Read(root)

			if err == nil {
				t.Fatalf("Read() error = nil, want one naming %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Read() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestRender_Inventory_IsIndentedJSONEndingInANewline verifies the artifact is
// shaped like every other text file here, since a diff is the whole point of
// committing it.
func TestRender_Inventory_IsIndentedJSONEndingInANewline(t *testing.T) {
	content := Render([]Row{{Package: "internal/tools/issues", Kind: KindREST, Method: "GET", Path: "/projects/:id/issues"}})

	if !strings.HasSuffix(string(content), "\n") {
		t.Error("the rendered inventory does not end in a newline")
	}
	if !strings.Contains(string(content), "\n  \"requests\": [") {
		t.Errorf("the rendered inventory is not indented:\n%s", content)
	}
	var decoded Inventory
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if decoded.Note != Note {
		t.Errorf("note = %q, want the generated-file header", decoded.Note)
	}
}

// TestRead_TheRepositorysOwnInventory_IsUsable verifies the committed artifact
// against the reader the audit uses, since a shape change that only the
// generator knew about would leave the audit reading an empty file and
// reporting every endpoint as never issued.
func TestRead_TheRepositorysOwnInventory_IsUsable(t *testing.T) {
	inventory, err := Read(repoRoot(t))
	if err != nil {
		t.Fatalf("Read() error = %v, want the committed artifact", err)
	}
	rest, graphql := 0, 0
	for _, row := range inventory.Requests {
		switch row.Kind {
		case KindREST:
			rest++
		case KindGraphQL:
			graphql++
		default:
			t.Fatalf("row %+v carries a kind neither caller knows", row)
		}
		if row.Package == "" || row.Method == "" || row.Path == "" {
			t.Fatalf("row %+v is missing what every row must have", row)
		}
	}
	if rest == 0 || graphql == 0 {
		t.Errorf("the committed inventory holds %d REST and %d GraphQL rows, want both kinds", rest, graphql)
	}
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}
