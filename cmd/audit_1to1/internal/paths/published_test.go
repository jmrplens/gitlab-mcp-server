package paths

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
// inside a response that does carry them. A nested type is returned all the
// same, marked inner, as is an exported struct not named as an output, since
// the sent direction counts their fields as the package's.
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

type CreateInput struct {
	Title string `+"`json:\"title\"`"+`
}

type rawOutput struct {
	Secret string `+"`json:\"secret\"`"+`
}
`)

	types := publishedTypes(root)

	want := []publishedType{
		{Package: "internal/tools/sample", Name: "NotAnOutputType", Fields: []string{"whatever"}, Inner: true},
		{Package: "internal/tools/sample", Name: "NoteOutput", Fields: []string{"body"}, Inner: true},
		{Package: "internal/tools/sample", Name: "Output", Fields: []string{"author", "id", "notes"}, Nested: map[string]nestedType{"author": {Name: "UserOutput", Fields: []string{"name"}}, "notes": {Name: "NoteOutput", Fields: []string{"body"}}}},
		{Package: "internal/tools/sample", Name: "UserOutput", Fields: []string{"name"}, Inner: true},
	}
	// An input and an unexported struct publish nothing: the first is what a
	// caller sends, the second a decode target nobody sees.
	if !reflect.DeepEqual(types, want) {
		t.Errorf("publishedTypes() = %+v, want %+v", types, want)
	}
}

// writeShared puts one Go source file under the shared shapes package of a
// tree writePackage began.
func writeShared(t *testing.T, root, source string) {
	t.Helper()
	dir := filepath.Join(root, sharedDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shapes.go"), []byte(source), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
}

// TestPublishedTypes_SharedShapes_ResolveWhereverAPackageNamesThem verifies
// the one cross-package edge the parse follows. A domain package takes a
// user, a role or a milestone from internal/toolutil, as a field of that
// type, as an embed, or as an alias under its own name, and until this was
// read none of those fields counted as published: a package whose only user
// object is the shared one was reported as failing to surface `username`.
// The shared struct is flattened within its own package, a field of one
// shared shape typed as another resolves too, an alias is published under
// the package's name, and the hints type stays out, since its next steps are
// the server's and not GitLab's.
func TestPublishedTypes_SharedShapes_ResolveWhereverAPackageNamesThem(t *testing.T) {
	root := writePackage(t, "sample", `package sample

import "example.com/toolutil"

type Output struct {
	toolutil.HintableOutput
	ID     int64                   `+"`json:\"id\"`"+`
	Author toolutil.BasicUserOutput `+"`json:\"author\"`"+`
	Role   MemberRoleOutput         `+"`json:\"role\"`"+`
	Other  elsewhere.Thing          `+"`json:\"other\"`"+`
}

type MemberRoleOutput = toolutil.MemberRoleOutput

type ActorOutput struct {
	toolutil.BasicUserOutput
	Kind string `+"`json:\"kind\"`"+`
}

type NoteOutput = toolutil.NoteOutput

type DiscussionOutput struct {
	toolutil.HintableOutput
	Notes []toolutil.NoteOutput `+"`json:\"notes\"`"+`
}

type TimeStatsOutput = toolutil.TimeStatsOutput

type DeleteOutput = toolutil.DeleteOutput

type Hints = toolutil.HintableOutput

func Remove() (DeleteOutput, error) { return DeleteOutput{}, nil }

func (o Output) Stats() TimeStatsOutput { return TimeStatsOutput{} }

func toStats() TimeStatsOutput { return TimeStatsOutput{} }
`)
	writeShared(t, root, `package toolutil

type HintableOutput struct {
	NextSteps []string `+"`json:\"next_steps,omitempty\"`"+`
}

type identity struct {
	ID       int64  `+"`json:\"id\"`"+`
	Username string `+"`json:\"username\"`"+`
}

type BasicUserOutput struct {
	identity
	Name string `+"`json:\"name\"`"+`
}

type MemberRoleOutput struct {
	ID    int64            `+"`json:\"id\"`"+`
	Owner *BasicUserOutput `+"`json:\"owner\"`"+`
}

type NoteOutput struct {
	HintableOutput
	Body string `+"`json:\"body\"`"+`
}

type TimeStatsOutput struct {
	TimeEstimate int `+"`json:\"time_estimate\"`"+`
}

type DeleteOutput struct {
	Status string `+"`json:\"status\"`"+`
}

type MergeRequestOutput struct {
	TimeStats TimeStatsOutput `+"`json:\"time_stats\"`"+`
	Deleted   DeleteOutput    `+"`json:\"deleted\"`"+`
}
`)

	types := publishedTypes(root)

	// NoteOutput is an alias the package keeps for its converters while the
	// field naming the shape is typed toolutil.NoteOutput: nested under either
	// name, and without the hints the shared shape embeds. TimeStatsOutput is
	// named by nothing in the package and by a shared shape, and returned only
	// by a method and an unexported function, so it is nested too; DeleteOutput
	// is named by that same shared shape and returned by an exported function,
	// which makes it a response of the package's own; and an alias of the
	// hints type resolves to nothing and is not published.
	want := []publishedType{
		{Package: "internal/tools/sample", Name: "ActorOutput", Fields: []string{"id", "kind", "name", "username"}},
		{Package: "internal/tools/sample", Name: "DeleteOutput", Fields: []string{"status"}},
		{Package: "internal/tools/sample", Name: "DiscussionOutput", Fields: []string{"notes"}, Nested: map[string]nestedType{
			"notes": {Name: "toolutil.NoteOutput", Fields: []string{"body"}},
		}},
		{Package: "internal/tools/sample", Name: "MemberRoleOutput", Fields: []string{"id", "owner"}, Inner: true},
		{Package: "internal/tools/sample", Name: "NoteOutput", Fields: []string{"body"}, Inner: true},
		{Package: "internal/tools/sample", Name: "Output", Fields: []string{"author", "id", "other", "role"}, Nested: map[string]nestedType{
			"author": {Name: "toolutil.BasicUserOutput", Fields: []string{"id", "name", "username"}},
			"role":   {Name: "MemberRoleOutput", Fields: []string{"id", "owner"}},
		}},
		{Package: "internal/tools/sample", Name: "TimeStatsOutput", Fields: []string{"time_estimate"}, Inner: true},
	}
	if !reflect.DeepEqual(types, want) {
		t.Errorf("publishedTypes() = %+v, want %+v", types, want)
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

	if types := publishedTypesIn(gone, "internal/tools/gone", nil, nil); len(types) != 0 {
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
// promoted, a shared shape among them under its qualified name, which is
// what flatten resolves or, for the hints type, drops. The check exists
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
	if !reflect.DeepEqual(embeds, []string{"Promoted", "PromotedToo", "toolutil.HintableOutput"}) {
		t.Errorf("jsonTags() embeds = %v, want the two local embeds and the shared one, none carrying a json name", embeds)
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

type Status string

type StatusOutput struct {
	*Status
	Note string `+"`json:\"note\"`"+`
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
	ID         int              `+"`json:\"id\"`"+`
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
	// struct and is passed over, and so is a type declared inside a function;
	// Status is not a struct either, and embedding it is a field encoding/json
	// writes under the type's name.
	want := []publishedType{
		{Package: "internal/tools/sample", Name: "DetailsOutput", Fields: []string{"connected", "group", "hollow", "id", "owner"}, Nested: map[string]nestedType{"group": {Name: "GroupOutput", Fields: []string{"path"}}}},
		{Package: "internal/tools/sample", Name: "GroupOutput", Fields: []string{"path"}, Inner: true},
		{Package: "internal/tools/sample", Name: "LeftOutput", Fields: []string{"a", "b"}},
		{Package: "internal/tools/sample", Name: "ListItem", Fields: []string{"id", "uploaded_by"}, Inner: true},
		{Package: "internal/tools/sample", Name: "Output", Fields: []string{"group", "hollow", "id", "owner"}, Nested: map[string]nestedType{"group": {Name: "GroupOutput", Fields: []string{"path"}}, "owner": {Name: "UserOutput", Fields: []string{"name"}}}},
		{Package: "internal/tools/sample", Name: "RightOutput", Fields: []string{"a", "b"}},
		{Package: "internal/tools/sample", Name: "StatusOutput", Fields: []string{"Status", "note"}},
		{Package: "internal/tools/sample", Name: "UploadedByOutput", Fields: []string{"name"}, Inner: true},
		{Package: "internal/tools/sample", Name: "UserOutput", Fields: []string{"name"}, Inner: true},
	}
	if !reflect.DeepEqual(types, want) {
		t.Errorf("publishedTypes() = %+v, want %+v", types, want)
	}
}

// TestEnvelopePayload_TellsThePackagingFromTheContent verifies the rule that
// decides whether a type named as somebody's field is a response.
//
// Both shapes exist in this repository and they look identical to a walk that
// only asks "is this named as a field". `{badge: BadgeItem}` is packaging over
// a response, so `BadgeItem` is what GitLab answered with and is what the type
// grain has to judge against the endpoint. `jobs.Output` naming a
// `ProjectObject` among thirty other fields is a reference to another
// resource, and judging it would hold a job's project reference to what
// GET /projects/:id answers with and report all eighty-five fields of a
// project as missing from it.
//
// Pagination is set aside because it is framing this server adds, not
// something GitLab sent, so a list plus its pagination is still a wrapper.
func TestEnvelopePayloads_TellsThePackagingFromTheContent(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name       string
		fields     []string
		fieldTypes map[string]string
		want       []string
	}{
		{
			name:   "a get envelope",
			fields: []string{"badge"}, fieldTypes: map[string]string{"badge": "BadgeItem"},
			want: []string{"BadgeItem"},
		},
		{
			name:   "a list envelope beside its pagination",
			fields: []string{"badges", "pagination"},
			fieldTypes: map[string]string{
				"badges": "BadgeItem", "pagination": sharedPrefix + "PaginationOutput",
			},
			want: []string{"BadgeItem"},
		},
		{
			name:       "a response carrying a reference among its own fields",
			fields:     []string{"id", "name", "project"},
			fieldTypes: map[string]string{"project": "ProjectObject"},
		},
		{
			name:       "a response carrying a scalar beside the object",
			fields:     []string{"badge", "deleted"},
			fieldTypes: map[string]string{"badge": "BadgeItem"},
		},
		{
			// Both come back as candidates; whether they are packaging is
			// resolveAlternatives' call, which refuses this pair.
			name:       "two objects are candidates, not yet an answer",
			fields:     []string{"group", "project"},
			fieldTypes: map[string]string{"group": "GroupOutput", "project": "ProjectObject"},
			want:       []string{"GroupOutput", "ProjectObject"},
		},
		{
			name:       "nothing but pagination",
			fields:     []string{"pagination"},
			fieldTypes: map[string]string{"pagination": sharedPrefix + "PaginationOutput"},
		},
		{name: "no fields at all"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := envelopePayloads(testCase.fields, testCase.fieldTypes); !slices.Equal(got, testCase.want) {
				t.Errorf("envelopePayloads() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestResolveAlternatives_OnlyShapesOfOneEntityAreTheResponse verifies the
// second half of the envelope rule. A list that keeps two shapes of one
// entity apart, the narrow one embedded in the wide one, wraps both, and both
// are judged as responses; a struct carrying two unrelated objects wraps
// neither, because each is a reference and neither is what the endpoint sent.
func TestResolveAlternatives_OnlyShapesOfOneEntityAreTheResponse(t *testing.T) {
	t.Parallel()
	parsed := parsedPackage{
		enveloped: map[string]bool{},
		structs: []declaredStruct{
			{Name: "BasicOutput", Fields: []string{"id"}},
			{Name: "Output", Fields: []string{"archived"}, Embeds: []string{"BasicOutput"}},
			{Name: "GroupOutput", Fields: []string{"id"}},
			{Name: "ProjectObject", Fields: []string{"id"}},
		},
		alternatives: [][]string{
			{"Output", "BasicOutput"},
			{"GroupOutput", "ProjectObject"},
		},
	}
	resolveAlternatives(&parsed)
	for name, want := range map[string]bool{
		"Output": true, "BasicOutput": true,
		"GroupOutput": false, "ProjectObject": false,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := parsed.enveloped[name]; got != want {
				t.Errorf("enveloped[%s] = %v, want %v: shapes of one entity are the response, unrelated objects are references", name, got, want)
			}
		})
	}
	if oneFamily(nil, func(string, string) bool { return true }) {
		t.Error("an empty set was reported as one family")
	}
}
