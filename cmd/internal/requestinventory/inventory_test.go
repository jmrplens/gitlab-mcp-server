package requestinventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

// TestRow_EveryField_CarriesTheNameTheArtifactSpellsIt verifies each field's
// JSON name against a document written out by hand, in both directions.
//
// A round trip through [Render] and [Read] cannot see this, because it encodes
// and decodes through the same tags: any permutation of them survives it whole.
// Two of those permutations are silent and costly. `package` and `path` are
// interchangeable by shape, and `package` is the only handle an action is
// joined to the requests its owner made, so exchanging them detaches every
// action from every request while the file still parses. `query` and `body`
// are interchangeable too, and R-PATH judges the body names against the params
// GitLab marks optional on a route that takes a body, so exchanging them puts
// the wrong list to the wrong oracle.
func TestRow_EveryField_CarriesTheNameTheArtifactSpellsIt(t *testing.T) {
	const document = `{
		"package": "internal/tools/issues",
		"kind": "rest",
		"method": "GET",
		"path": "/projects/:project_id/issues",
		"query": ["state"],
		"body": ["title"],
		"operation": "query issueList",
		"variables": ["fullPath"],
		"identifiers": {":project_id": 3}
	}`
	row := Row{
		Package:     "internal/tools/issues",
		Kind:        "rest",
		Method:      "GET",
		Path:        "/projects/:project_id/issues",
		Query:       []string{"state"},
		Body:        []string{"title"},
		Operation:   "query issueList",
		Variables:   []string{"fullPath"},
		Identifiers: map[string]int{":project_id": 3},
	}

	t.Run("a document names the field it fills", func(t *testing.T) {
		var decoded Row
		if err := json.Unmarshal([]byte(document), &decoded); err != nil {
			t.Fatalf("Unmarshal error = %v", err)
		}
		if !reflect.DeepEqual(decoded, row) {
			t.Errorf("decoded %+v, want %+v", decoded, row)
		}
	})

	t.Run("a rendered row writes the value under that same name", func(t *testing.T) {
		var rendered struct {
			Requests []map[string]any `json:"requests"`
		}
		if err := json.Unmarshal(Render([]Row{row}), &rendered); err != nil {
			t.Fatalf("Unmarshal error = %v", err)
		}
		want := map[string]any{
			"package":     "internal/tools/issues",
			"kind":        "rest",
			"method":      "GET",
			"path":        "/projects/:project_id/issues",
			"query":       []any{"state"},
			"body":        []any{"title"},
			"operation":   "query issueList",
			"variables":   []any{"fullPath"},
			"identifiers": map[string]any{":project_id": float64(3)},
		}
		if len(rendered.Requests) != 1 || !reflect.DeepEqual(rendered.Requests[0], want) {
			t.Errorf("rendered %+v, want %+v", rendered.Requests, want)
		}
	})
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

// TestKinds_EachOne_ClassifiesTheRowsThatLookLikeIt verifies which of the two
// kind constants is which, against the committed artifact.
//
// The test beside this one requires every row to carry a kind one of the two
// constants names, which pins the pair of spellings and says nothing about
// which is which: exchanging the two values leaves every row matching the other
// constant and every count above zero. What separates them is what a GraphQL
// request looks like, so that is what is asserted (an operation, at /graphql)
// and a REST row is the one carrying no operation at all.
func TestKinds_EachOne_ClassifiesTheRowsThatLookLikeIt(t *testing.T) {
	inventory, err := Read(repoRoot(t))
	if err != nil {
		t.Fatalf("Read() error = %v, want the committed artifact", err)
	}

	graphql := 0
	for _, row := range inventory.Requests {
		switch row.Kind {
		case KindGraphQL:
			graphql++
			if row.Operation == "" || row.Path != "/graphql" {
				t.Errorf("row %+v is recorded as %q and is no GraphQL request", row, KindGraphQL)
			}
		case KindREST:
			if row.Operation != "" {
				t.Errorf("row %+v is recorded as %q and names a GraphQL operation", row, KindREST)
			}
		}
	}
	if graphql == 0 {
		t.Errorf("no row of the committed inventory is recorded as %q", KindGraphQL)
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
