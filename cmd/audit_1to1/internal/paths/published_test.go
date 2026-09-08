package paths

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
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

	tags, fieldTypes, embeds := jsonTags(structType)
	if strings.Join(tags, ",") != "read" || len(embeds) != 0 {
		t.Errorf("jsonTags() = %v, embeds %v, want only the field whose tag is a quoted string", tags, embeds)
	}
	// The field type is recorded for the same field and no other. What it names
	// here is a predeclared type, which the parser writes as the same Ident a
	// local type is written as; nestedTypes is what tells them apart, by finding
	// no output type of that name.
	if !reflect.DeepEqual(fieldTypes, map[string]string{"read": "string"}) {
		t.Errorf("jsonTags() field types = %v, want the tagged field alone", fieldTypes)
	}
}

// TestJSONTags_FollowsEncodingJSON verifies that the names a struct is said to
// publish are the names encoding/json would write: an unexported field is not
// marshaled however it is tagged, a tag naming no key keeps the Go field name,
// "-" publishes nothing, an embed tagged with a name is keyed by that name,
// and an embed tagged with none or not tagged at all is one whose fields are
// promoted, while an embed from another package is neither. The check exists
// because a name this reads and GitLab never receives is a phantom finding,
// and a name it drops that GitLab does receive is a missed one.
func TestJSONTags_FollowsEncodingJSON(t *testing.T) {
	quoted := func(tag string) *ast.BasicLit {
		return &ast.BasicLit{Kind: token.STRING, Value: "`" + tag + "`"}
	}
	structType := &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
		{Names: []*ast.Ident{{Name: "hidden"}}, Type: &ast.Ident{Name: "string"}, Tag: quoted(`json:"hidden"`)},
		{Names: []*ast.Ident{{Name: "Kept"}}, Type: &ast.Ident{Name: "string"}, Tag: quoted(`json:",omitempty"`)},
		{Names: []*ast.Ident{{Name: "Renamed"}}, Type: &ast.Ident{Name: "string"}, Tag: quoted(`json:"renamed"`)},
		{Names: []*ast.Ident{{Name: "Dropped"}}, Type: &ast.Ident{Name: "string"}, Tag: quoted(`json:"-"`)},
		{Names: []*ast.Ident{{Name: "Untagged"}}, Type: &ast.Ident{Name: "string"}},
		{Type: &ast.Ident{Name: "Embedded"}, Tag: quoted(`json:"embedded"`)},
		{Type: &ast.Ident{Name: "Promoted"}, Tag: quoted(`json:",omitempty"`)},
		{Type: &ast.StarExpr{X: &ast.Ident{Name: "PromotedToo"}}},
		{Type: &ast.SelectorExpr{X: &ast.Ident{Name: "toolutil"}, Sel: &ast.Ident{Name: "HintableOutput"}}},
	}}}

	tags, fieldTypes, embeds := jsonTags(structType)
	if strings.Join(tags, ",") != "Kept,embedded,renamed" {
		t.Errorf("jsonTags() = %v, want Kept,embedded,renamed", tags)
	}
	// The type is recorded under the key the name resolved to, so a field kept
	// under its Go name and an embed kept under its tag both stay comparable.
	want := map[string]string{"Kept": "string", "embedded": "Embedded", "renamed": "string"}
	if !reflect.DeepEqual(fieldTypes, want) {
		t.Errorf("jsonTags() field types = %v, want %v", fieldTypes, want)
	}
	if !reflect.DeepEqual(embeds, []string{"Promoted", "PromotedToo"}) {
		t.Errorf("jsonTags() embeds = %v, want the two local embeds carrying no json name", embeds)
	}
}

// TestPublishedTypes_EmbedsArePromotedAndNestingCountsThroughAnyStruct verifies
// two rules of the walk that the first real run got wrong. A details type
// embedding the row type publishes the row's fields as its own, through a
// pointer as well, with its own field winning over a promoted one of the same
// name, and the row type stays comparable, since the list endpoint answers
// with it; before this the details type was reported missing every field it
// promoted. And a type reached through a plain struct, the user under the row
// of a list of uploads, is nested all the same and is not held to the
// endpoints answering with a whole user, which is what it was held to while
// only output types could make one nested. Two types embedding each other
// through pointers are read once each.
func TestPublishedTypes_EmbedsArePromotedAndNestingCountsThroughAnyStruct(t *testing.T) {
	root := writePackage(t, "sample", `package sample

import "example.com/toolutil"

type Kind string

type Output struct {
	ID     int64         `+"`json:\"id\"`"+`
	Owner  *UserOutput   `+"`json:\"owner\"`"+`
	Group  *GroupOutput  `+"`json:\"group\"`"+`
	Hollow *HollowOutput `+"`json:\"hollow\"`"+`
}

type DetailsOutput struct {
	toolutil.HintableOutput
	*Output
	Connected bool   `+"`json:\"connected\"`"+`
	Owner     string `+"`json:\"owner\"`"+`
}

type HollowOutput struct {
	*Elsewhere
}

type BareOutput struct {
	*Elsewhere
}

func helper() {
	type LocalOutput struct {
		X int `+"`json:\"x\"`"+`
	}
	_ = LocalOutput{}
}

type UserOutput struct {
	Name string `+"`json:\"name\"`"+`
}

type GroupOutput struct {
	Path string `+"`json:\"path\"`"+`
}

type ListItem struct {
	UploadedBy UploadedByOutput `+"`json:\"uploaded_by\"`"+`
}

type UploadedByOutput struct {
	Name string `+"`json:\"name\"`"+`
}

type LeftOutput struct {
	*RightOutput
	A string `+"`json:\"a\"`"+`
}

type RightOutput struct {
	*LeftOutput
	B string `+"`json:\"b\"`"+`
}
`)

	types := publishedTypes(root)

	// HollowOutput and BareOutput embed a type the package does not declare
	// and so publish nothing: the first is not nested under the field naming
	// it and the second, which nothing names, is not compared. Kind is not a
	// struct and is passed over, and so is a type declared inside a function.
	want := []publishedType{
		{Package: "internal/tools/sample", Name: "DetailsOutput", Fields: []string{"connected", "group", "hollow", "id", "owner"}, Nested: map[string]nestedType{"group": {Name: "GroupOutput", Fields: []string{"path"}}}},
		{Package: "internal/tools/sample", Name: "LeftOutput", Fields: []string{"a", "b"}},
		{Package: "internal/tools/sample", Name: "Output", Fields: []string{"group", "hollow", "id", "owner"}, Nested: map[string]nestedType{"group": {Name: "GroupOutput", Fields: []string{"path"}}, "owner": {Name: "UserOutput", Fields: []string{"name"}}}},
		{Package: "internal/tools/sample", Name: "RightOutput", Fields: []string{"a", "b"}},
	}
	if !reflect.DeepEqual(types, want) {
		t.Errorf("publishedTypes() = %+v, want %+v", types, want)
	}
}
