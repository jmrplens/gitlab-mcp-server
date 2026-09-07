package paths

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePackage puts one Go source file under a tools package directory and
// returns the tree root, which is what publishedTypes is pointed at.
func writePackage(t *testing.T, name, source string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, toolsDir, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".go"), []byte(source), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	return root
}

// TestPublishedTypes_NestedOutputs_AreNotCompared verifies the filter that made
// this check usable. GitLab's document lists the properties of the object an
// endpoint returns and not of the objects inside it, so comparing a nested type
// against that list reports every one of its fields: the first run produced
// 1418 findings, almost all of them the fields of a user or a group sitting
// inside a response that does carry them.
func TestPublishedTypes_NestedOutputs_AreNotCompared(t *testing.T) {
	root := writePackage(t, "sample", `package sample

type Output struct {
	ID     int64        `+"`json:\"id\"`"+`
	Author *UserOutput  `+"`json:\"author\"`"+`
	Notes  []NoteOutput `+"`json:\"notes\"`"+`
	Hidden string       `+"`json:\"-\"`"+`
	NoTag  string
}

type UserOutput struct {
	Name string `+"`json:\"name\"`"+`
}

type NoteOutput struct {
	Body string `+"`json:\"body\"`"+`
}

type NotAnOutputType struct {
	Whatever string `+"`json:\"whatever\"`"+`
}
`)

	types := publishedTypes(root)

	if len(types) != 1 {
		t.Fatalf("read %+v, want only the top-level output type", types)
	}
	if types[0].Name != "Output" || types[0].Package != "internal/tools/sample" {
		t.Errorf("type = %+v, want internal/tools/sample.Output", types[0])
	}
	if strings.Join(types[0].Fields, ",") != "author,id,notes" {
		t.Errorf("fields = %v, want the tagged ones only, sorted", types[0].Fields)
	}
}

// TestPublishedTypes_ATreeWithoutTools_ReadsNothing verifies the shape a caller
// pointed at the wrong root meets: nothing, rather than a panic or a finding.
func TestPublishedTypes_ATreeWithoutTools_ReadsNothing(t *testing.T) {
	if types := publishedTypes(t.TempDir()); len(types) != 0 {
		t.Errorf("publishedTypes() = %+v, want nothing", types)
	}
}

// TestPublishedTypes_AFileThatDoesNotParse_ContributesNothing verifies that a
// file the parser refuses is passed over rather than taking the whole scope
// down. The audit reads a tree it does not own the state of, and a half-written
// file during an edit must not turn every other package's comparison into a
// failure about that file.
func TestPublishedTypes_AFileThatDoesNotParse_ContributesNothing(t *testing.T) {
	root := writePackage(t, "broken", "package broken\n\ntype Output struct {\n")

	if types := publishedTypes(root); len(types) != 0 {
		t.Errorf("publishedTypes() = %+v, want nothing from a package that does not parse", types)
	}
}

// TestPublishedTypesIn_ADirectoryThatCannotBeRead_ContributesNothing covers the
// window between the two listings. publishedTypes lists internal/tools and then
// reads each entry, so a directory removed between those two moments reaches
// this function as a name with nothing behind it. Reached directly because that
// race cannot be staged through the caller, and it must answer nothing rather
// than fail: the audit is a reader of the tree, not its owner.
func TestPublishedTypesIn_ADirectoryThatCannotBeRead_ContributesNothing(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "removed-between-the-two-listings")

	if types := publishedTypesIn(gone, "internal/tools/gone"); len(types) != 0 {
		t.Errorf("publishedTypesIn() = %+v, want nothing", types)
	}
}

// TestJSONTags_ATagThatIsNotAQuotedString_PublishesNothing verifies the guard on
// the one value in this file that comes from outside the parser's own grammar.
// Go's parser only ever produces a quoted tag, so this branch is unreachable
// from a source tree and reachable from a synthesized node, which is exactly why
// it is written as a skip rather than a panic: the audit reports on other
// people's code and must not be the thing that crashes.
func TestJSONTags_ATagThatIsNotAQuotedString_PublishesNothing(t *testing.T) {
	structType := &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
		{
			Names: []*ast.Ident{{Name: "Unquoted"}},
			Type:  &ast.Ident{Name: "string"},
			Tag:   &ast.BasicLit{Kind: token.STRING, Value: `json:"never_read"`},
		},
		{
			Names: []*ast.Ident{{Name: "Quoted"}},
			Type:  &ast.Ident{Name: "string"},
			Tag:   &ast.BasicLit{Kind: token.STRING, Value: "`" + `json:"read"` + "`"},
		},
	}}}

	if tags := jsonTags(structType); strings.Join(tags, ",") != "read" {
		t.Errorf("jsonTags() = %v, want only the field whose tag is a quoted string", tags)
	}
}
