package paths

import (
	"errors"
	"go/ast"
	"go/parser"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
)

// parseType reads a Go type expression the way the parser hands a field's
// type to [shapeOf].
func parseType(t *testing.T, source string) ast.Expr {
	t.Helper()
	expr, err := parser.ParseExpr(source)
	if err != nil {
		t.Fatalf("parse %q: %v", source, err)
	}
	return expr
}

// TestShapeOf_ReadsTheWrappersTheCoreAndTheTagOptions verifies the reading a
// field's Go type is held to: pointers, slices and arrays in the order they
// wrap, the core as the source names it or as the kind of type written in
// place, and the two options that change what a scalar takes.
func TestShapeOf_ReadsTheWrappersTheCoreAndTheTagOptions(t *testing.T) {
	cases := []struct {
		source, options string
		want            goShape
	}{
		{"string", "", goShape{Core: "string", Spelled: "string"}},
		{"*int64", "string,omitempty", goShape{Layers: "*", Core: "int64", Quoted: true, OmitEmpty: true, Spelled: "*int64"}},
		{"[]*Item", "omitempty", goShape{Layers: "[*", Core: "Item", OmitEmpty: true, Spelled: "[]*Item"}},
		{"*[3][]int", "", goShape{Layers: "*#[", Core: "int", Spelled: "*[3][]int"}},
		{"time.Time", "", goShape{Core: "time.Time", Spelled: "time.Time"}},
		{"map[string]any", "", goShape{Core: coreMap, Spelled: "map[string]any"}},
		{"interface{}", "", goShape{Core: coreInterface, Spelled: "interface{}"}},
		{"struct{ A int }", "", goShape{Core: coreStruct, Spelled: "struct{A int}"}},
		// A selector on a selector names no package this repository writes,
		// and a channel is no type encoding/json decodes: neither has a core.
		{"a.b.C", "", goShape{Spelled: "a.b.C"}},
		{"chan int", "", goShape{Spelled: "chan int"}},
		// An option is read only as a whole word of the list.
		{"int", "strings,omitemptyish", goShape{Core: "int", Spelled: "int"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.source+" "+testCase.options, func(t *testing.T) {
			if got := shapeOf(parseType(t, testCase.source), testCase.options); !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("shapeOf() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// kindsSource is a package as [typeSource.kinds] sees it: a struct of its own
// and one internal/toolutil shares, types declared as a scalar, a list, a
// pointer and an alias of the shared struct, a type that decodes itself, one
// that writes itself and nothing else, and two types declared as each other,
// which no compiler accepts and a parse still reads.
func kindsSource() *typeSource {
	return &typeSource{
		structs: map[string]declaredStruct{"Item": {Name: "Item"}, "toolutil.Person": {Name: "toolutil.Person"}},
		named: map[string]goShape{
			"State": {Core: "string"}, "Names": {Layers: "[", Core: "string"}, "Ref": {Layers: "*", Core: "Item"},
			"Alias": {Core: "toolutil.Person"}, "Custom": {Core: "string"}, "Written": {Core: "string"},
			"Ping": {Core: "Pong"}, "Pong": {Core: "Ping"},
		},
		decoders: map[string]bool{"Custom": true},
		encoders: map[string]bool{"Written": true},
	}
}

// TestTypeSourceKinds_HoldsEachGoTypeToTheKindsEachReadingTakes verifies the
// table both halves of the kinds question read, depth by depth, under each
// reading. The decoder's reading takes a null everywhere, since encoding/json
// reads one into any value without an error, reads a byte slice from an array
// as well as from a base64 string and json.Number from a string, and follows a
// type that only writes itself to what it is declared as. The published
// type's reading carries a null only where it is written back as a null or as
// the key left out (a pointer, a nil list, an interface, an omitempty scalar)
// and not where it would be written as a zero GitLab did not send (a scalar),
// as a whole struct value, or as an element of a list; it carries neither the
// array a byte slice reads nor the string json.Number reads, since both are
// written as the other kind; and a type that writes itself is unknown to it.
// Under both, a quoted scalar travels as a string, a declared type is held to
// what it is declared as, and a type that decodes itself, one from a package
// this does not model and one it cannot read are unknown rather than guessed
// at.
func TestTypeSourceKinds_HoldsEachGoTypeToTheKindsEachReadingTakes(t *testing.T) {
	const (
		str, num, boo = orbitrecord.KindString, orbitrecord.KindNumber, orbitrecord.KindBoolean
		obj, arr, nul = orbitrecord.KindObject, orbitrecord.KindArray, orbitrecord.KindNull
	)
	every, unknown := kindSet{every: true}, kindSet{unknown: true}
	kinds := func(list ...string) kindSet { return kindSet{kinds: list} }
	cases := []struct {
		source, options  string
		depth            int
		carried, decoded kindSet
	}{
		{"string", "", 0, kinds(str), kinds(str, nul)},
		{"string", "omitempty", 0, kinds(str, nul), kinds(str, nul)},
		{"*string", "", 0, kinds(str, nul), kinds(str, nul)},
		{"**string", "", 0, kinds(str, nul), kinds(str, nul)},
		{"bool", "", 0, kinds(boo), kinds(boo, nul)},
		{"int64", "", 0, kinds(num), kinds(num, nul)},
		{"uint8", "omitempty", 0, kinds(num, nul), kinds(num, nul)},
		{"int64", "string", 0, kinds(str), kinds(str, nul)},
		{"*int64", "string", 0, kinds(str, nul), kinds(str, nul)},
		{"json.Number", "", 0, kinds(num), kinds(num, str, nul)},
		{"time.Time", "omitempty", 0, kinds(str), kinds(str, nul)},
		{"*time.Time", "", 0, kinds(str, nul), kinds(str, nul)},
		{"any", "", 0, every, every},
		{"interface{}", "", 0, every, every},
		{"map[string]int", "", 0, every, every},
		{"json.RawMessage", "", 0, every, every},
		{"Item", "omitempty", 0, kinds(obj), kinds(obj, nul)},
		{"*Item", "", 0, kinds(obj, nul), kinds(obj, nul)},
		{"struct{ A int }", "", 0, kinds(obj), kinds(obj, nul)},
		{"toolutil.Person", "", 0, kinds(obj), kinds(obj, nul)},
		{"[]string", "", 0, kinds(arr, nul), kinds(arr, nul)},
		{"*[]string", "", 0, kinds(arr, nul), kinds(arr, nul)},
		{"[]string", "omitempty", 1, kinds(str), kinds(str, nul)},
		{"[]string", "string", 1, kinds(str), kinds(str, nul)},
		{"[]int", "string", 1, kinds(num), kinds(num, nul)},
		{"[]string", "", 2, every, every},
		{"[]*Item", "", 0, kinds(arr, nul), kinds(arr, nul)},
		{"[]*Item", "", 1, kinds(obj, nul), kinds(obj, nul)},
		{"[]Item", "", 1, kinds(obj), kinds(obj, nul)},
		{"[][]int", "", 1, kinds(arr, nul), kinds(arr, nul)},
		{"[][]int", "", 2, kinds(num), kinds(num, nul)},
		{"[3]int", "", 0, kinds(arr), kinds(arr, nul)},
		{"[3]int", "", 1, kinds(num), kinds(num, nul)},
		{"*[3]int", "", 0, kinds(arr, nul), kinds(arr, nul)},
		{"[]byte", "", 0, kinds(str, nul), kinds(arr, str, nul)},
		{"[]uint8", "", 0, kinds(str, nul), kinds(arr, str, nul)},
		{"[]uint16", "", 0, kinds(arr, nul), kinds(arr, nul)},
		// A list of byte slices is a list, whose elements are the strings.
		{"[][]byte", "", 0, kinds(arr, nul), kinds(arr, nul)},
		{"[][]byte", "", 1, kinds(str, nul), kinds(arr, str, nul)},
		{"State", "omitempty", 0, kinds(str, nul), kinds(str, nul)},
		{"State", "string", 0, kinds(str), kinds(str, nul)},
		{"Names", "", 0, kinds(arr, nul), kinds(arr, nul)},
		{"Names", "", 1, kinds(str), kinds(str, nul)},
		{"Ref", "", 0, kinds(obj, nul), kinds(obj, nul)},
		{"Alias", "", 0, kinds(obj), kinds(obj, nul)},
		{"Custom", "", 0, unknown, unknown},
		{"Custom", "", 1, every, every},
		{"[]Custom", "", 0, kinds(arr, nul), kinds(arr, nul)},
		{"[]Custom", "", 1, unknown, unknown},
		{"Written", "", 0, unknown, kinds(str, nul)},
		{"[]Written", "", 1, unknown, kinds(str, nul)},
		{"Ping", "", 0, unknown, unknown},
		{"gl.Thing", "", 0, unknown, unknown},
		{"complex128", "", 0, unknown, unknown},
		{"chan int", "", 0, unknown, unknown},
		{"string", "", 1, every, every},
	}
	source := kindsSource()
	for _, testCase := range cases {
		t.Run(testCase.source+" "+testCase.options+" "+strconv.Itoa(testCase.depth), func(t *testing.T) {
			shape := shapeOf(parseType(t, testCase.source), testCase.options)
			if got := source.kinds(shape, testCase.depth, false); !reflect.DeepEqual(got, testCase.carried) {
				t.Errorf("kinds(carried) = %+v, want %+v", got, testCase.carried)
			}
			if got := source.kinds(shape, testCase.depth, true); !reflect.DeepEqual(got, testCase.decoded) {
				t.Errorf("kinds(decoded) = %+v, want %+v", got, testCase.decoded)
			}
		})
	}
}

// TestKindSetRefuses_NamesTheRecordedKindsNotTaken verifies the difference a
// kind finding reports: sorted, only what the type does not take, and nothing
// for a type that takes every kind.
func TestKindSetRefuses_NamesTheRecordedKindsNotTaken(t *testing.T) {
	recorded := map[string]bool{orbitrecord.KindString: true, orbitrecord.KindNull: true, orbitrecord.KindNumber: true}
	if got := (kindSet{kinds: []string{orbitrecord.KindNumber}}).refuses(recorded); !slices.Equal(got, []string{orbitrecord.KindNull, orbitrecord.KindString}) {
		t.Errorf("refuses() = %q, want null and string in order", got)
	}
	if got := (kindSet{kinds: []string{orbitrecord.KindNumber, orbitrecord.KindString, orbitrecord.KindNull}}).refuses(recorded); got != nil {
		t.Errorf("refuses() = %q, want nothing refused", got)
	}
	if got := (kindSet{every: true}).refuses(recorded); got != nil {
		t.Errorf("refuses() = %q, want nothing refused by a type that takes every kind", got)
	}
}

// TestTrailingElements_CountsTheMarkersAPathEndsIn verifies the depth a
// recorded kind is filed under: the markers at the end of a path, never the
// ones inside it.
func TestTrailingElements_CountsTheMarkersAPathEndsIn(t *testing.T) {
	cases := map[string]int{
		orbitrecord.Root: 0, "[]": 1, "a": 0, "a[]": 1, "a[][]": 2, "a[].b": 0, "a[].b[]": 1,
	}
	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			if got := trailingElements(path); got != want {
				t.Errorf("trailingElements(%q) = %d, want %d", path, got, want)
			}
		})
	}
}

// orbitKindsFixture is a package whose output types carry every kind of
// field the kinds question tells apart: a number, a pointer and an omitempty
// scalar GitLab sends as null, a scalar it sends null that is written as zero,
// a list whose elements are struct values, a list of strings, a scalar type the
// package declares (beside a method whose name decides nothing), a type that
// decodes itself through UnmarshalJSON, one through UnmarshalText and one
// that only writes itself through MarshalJSON, a shared struct, and a value
// passed through as any. The package declares its own Level, which a shared
// shape's field of the shared Level must never resolve to.
const orbitKindsFixture = `package orbitk

import "example.com/toolutil"

type GraphOutput struct {
	toolutil.HintableOutput
	Count    int64           ` + "`json:\"count\"`" + `
	Indexing *Indexing       ` + "`json:\"indexing,omitempty\"`" + `
	Items    []Item          ` + "`json:\"items,omitempty\"`" + `
	Owner    toolutil.Person ` + "`json:\"owner\"`" + `
	Priority Level           ` + "`json:\"priority\"`" + `
	Clock    Clock           ` + "`json:\"clock\"`" + `
	Stamp    Stamp           ` + "`json:\"stamp\"`" + `
	Tag      Tag             ` + "`json:\"tag\"`" + `
	Result   any             ` + "`json:\"result,omitempty\"`" + `
}

type Indexing struct {
	LastError string  ` + "`json:\"last_error,omitempty\"`" + `
	LastRun   *string ` + "`json:\"last_run\"`" + `
	Total     int64   ` + "`json:\"total\"`" + `
}

type Item struct {
	Name string   ` + "`json:\"name\"`" + `
	Tags []string ` + "`json:\"tags\"`" + `
}

type Level int

func (l Level) String() string { return "" }

type Clock string

func (c *Clock) UnmarshalJSON(data []byte) error { return nil }

type Stamp string

func (s Stamp) UnmarshalText(text []byte) error { return nil }

func (s Stamp) MarshalText() ([]byte, error) { return nil, nil }

type Tag string

func (t Tag) MarshalJSON() ([]byte, error) { return nil, nil }
`

// orbitKindsShared is the shared shapes package of that tree: a person whose
// rank is a scalar type the shared package declares and whose boss is another
// person.
const orbitKindsShared = `package toolutil

type HintableOutput struct {
	NextSteps []string ` + "`json:\"next_steps,omitempty\"`" + `
}

type Level string

type Person struct {
	Login string  ` + "`json:\"login\"`" + `
	Rank  Level   ` + "`json:\"rank,omitempty\"`" + `
	Boss  *Person ` + "`json:\"boss,omitempty\"`" + `
}
`

// graphCalls are the two recorded calls of the kinds fixture's graph output: a
// structured answer carrying every kind of field it tells apart, and one
// answering the count as a string.
func graphCalls() []orbitrecord.Call {
	call := func(variant string, keyTree []orbitrecord.Key) orbitrecord.Call {
		return orbitrecord.Call{Action: "orbit.graph", Variant: variant, Output: "internal/tools/orbitk.GraphOutput", Response: orbitrecord.Response{Status: 200, Keys: keyTree}}
	}
	return []orbitrecord.Call{
		call("raw", keys(
			orbitrecord.Root, "count=number",
			"indexing", "indexing.last_error=null", "indexing.last_run=null", "indexing.total=null",
			"items=array", "items[]=object|null", "items[].name=string", "items[].tags=array|null", "items[].tags[]=string|number",
			"owner", "owner.login=string", "owner.rank=string", "owner.boss=null",
			"priority=number", "clock=string", "stamp=string", "tag=string", "result", "result.anything=string",
		)),
		call("text", keys(orbitrecord.Root, "count=string")),
	}
}

// TestOrbitCheck_TheKindsRecorded_AreHeldToEachFieldsGoType verifies the
// published half of the third question end to end, the two cases the issue
// asks for among them: a number field GitLab sends as a string is reported
// with the kind it does not carry, while a pointer field GitLab sends as null
// is not. Kinds are unioned across calls and judged at the depth they were
// recorded at, so the elements of a list are held to its element type and
// reported under a path ending in a marker; a shared shape's field is read as
// the shared package declares it, which leaves a shared scalar type unjudged
// rather than resolved against the domain's Level; and a type that decodes
// itself, by either method, or that writes itself is listed apart as
// unjudged. With no converter pairing to be had, the decoder half says it did
// not run and asks nothing.
func TestOrbitCheck_TheKindsRecorded_AreHeldToEachFieldsGoType(t *testing.T) {
	root := writePackage(t, "orbitk", orbitKindsFixture)
	writeShared(t, root, orbitKindsShared)
	stubOrbitRecord(t, orbitrecord.Document{Calls: graphCalls()}, nil)

	check := orbitCheck(root)

	calls := []string{"orbit.graph (raw)", "orbit.graph (text)"}
	pkg := "internal/tools/orbitk"
	wantMismatched := []OrbitField{
		{Package: pkg, Output: "GraphOutput", Path: "count", Type: "GraphOutput", Field: "count", GoType: "int64", Kinds: []string{orbitrecord.KindString}, Calls: calls},
		{Package: pkg, Output: "GraphOutput", Path: "indexing.total", Type: "Indexing", Field: "total", GoType: "int64", Kinds: []string{orbitrecord.KindNull}, Calls: calls},
		{Package: pkg, Output: "GraphOutput", Path: "items.tags[]", Type: "Item", Field: "tags", GoType: "[]string", Kinds: []string{orbitrecord.KindNumber}, Calls: calls},
		{Package: pkg, Output: "GraphOutput", Path: "items[]", Type: "GraphOutput", Field: "items", GoType: "[]Item", Kinds: []string{orbitrecord.KindNull}, Calls: calls},
	}
	if !reflect.DeepEqual(check.Mismatched, wantMismatched) {
		t.Errorf("mismatched =\n%+v\nwant\n%+v", check.Mismatched, wantMismatched)
	}
	wantUnjudged := []OrbitField{
		{Package: pkg, Output: "GraphOutput", Path: "clock", Type: "GraphOutput", Field: "clock", GoType: "Clock", Kinds: []string{orbitrecord.KindString}, Calls: calls},
		{Package: pkg, Output: "GraphOutput", Path: "owner.rank", Type: "toolutil.Person", Field: "rank", GoType: "Level", Kinds: []string{orbitrecord.KindString}, Calls: calls},
		{Package: pkg, Output: "GraphOutput", Path: "stamp", Type: "GraphOutput", Field: "stamp", GoType: "Stamp", Kinds: []string{orbitrecord.KindString}, Calls: calls},
		{Package: pkg, Output: "GraphOutput", Path: "tag", Type: "GraphOutput", Field: "tag", GoType: "Tag", Kinds: []string{orbitrecord.KindString}, Calls: calls},
	}
	if !reflect.DeepEqual(check.KindsUnjudged, wantUnjudged) {
		t.Errorf("unjudged =\n%+v\nwant\n%+v", check.KindsUnjudged, wantUnjudged)
	}
	if check.Unpublished != nil || check.Unsurfaced != nil || check.undeclared() != 4 {
		t.Errorf("unpublished = %+v, unsurfaced = %+v, undeclared() = %d, want the four kind findings alone", check.Unpublished, check.Unsurfaced, check.undeclared())
	}
	// Thirteen fields are judged, two of them at their elements as well; the
	// four fields no kind could be read for (clock, owner.rank, stamp and tag)
	// are not counted as judged.
	if check.KindsCompared != 15 {
		t.Errorf("KindsCompared = %d, want 15", check.KindsCompared)
	}
	if check.DecodersRead || check.Decoders != nil || check.Undecoded != nil || check.DecoderKindsCompared != 0 ||
		check.DecoderMismatched != nil || check.DecoderKindsUnjudged != nil {
		t.Errorf("decoder half = %t %q %q %d %+v %+v, want it skipped", check.DecodersRead, check.Decoders, check.Undecoded,
			check.DecoderKindsCompared, check.DecoderMismatched, check.DecoderKindsUnjudged)
	}
}

// orbitDecodersPackage is a second tools package of the kinds tree: an output
// wrapping a bare array answer, whose handler decodes into a struct that
// decodes the whole body itself, and one carrying text, which no converter
// fills from a client-go struct.
const orbitDecodersPackage = `package orbitd

type ToolsOutput struct {
	Tools []Tool ` + "`json:\"tools\"`" + `
}

type Tool struct {
	Name string ` + "`json:\"name\"`" + `
}

type TextOutput struct {
	Content string ` + "`json:\"content\"`" + `
}
`

// orbitClientGo is the client-go package of the kinds tree: the struct the
// graph output is filled from, typed the way client-go types it (a pointer
// where a value may be null, a raw message where it is passed on), with a
// count still an int64, a list of strings, a type that decodes itself and one
// that only writes itself, and the tools struct that decodes its bare array
// body itself.
const orbitClientGo = `package gitlab

import (
	"encoding/json"
	"time"
)

type GraphStatus struct {
	Count    int64           ` + "`json:\"count\"`" + `
	Indexing *GraphIndexing  ` + "`json:\"indexing,omitempty\"`" + `
	Items    []*GraphItem    ` + "`json:\"items,omitempty\"`" + `
	Owner    Person          ` + "`json:\"owner\"`" + `
	Priority Level           ` + "`json:\"priority\"`" + `
	Clock    Clock           ` + "`json:\"clock\"`" + `
	Stamp    Stamp           ` + "`json:\"stamp\"`" + `
	Result   json.RawMessage ` + "`json:\"result,omitempty\"`" + `
}

type GraphIndexing struct {
	LastError *string    ` + "`json:\"last_error,omitempty\"`" + `
	LastRun   *time.Time ` + "`json:\"last_run\"`" + `
	Total     int64      ` + "`json:\"total\"`" + `
}

type GraphItem struct {
	Name string   ` + "`json:\"name\"`" + `
	Tags []string ` + "`json:\"tags\"`" + `
}

type Person struct {
	Login string  ` + "`json:\"login\"`" + `
	Rank  string  ` + "`json:\"rank,omitempty\"`" + `
	Boss  *Person ` + "`json:\"boss,omitempty\"`" + `
}

type Level int

type Clock string

func (c *Clock) UnmarshalJSON(data []byte) error { return nil }

type Stamp string

func (s Stamp) MarshalText() ([]byte, error) { return nil, nil }

type Tools struct {
	Tools []*GraphItem ` + "`json:\"tools\"`" + `
}

func (t *Tools) UnmarshalJSON(data []byte) error { return nil }
`

// stubDecoders hands the Orbit check a client-go directory and the converter
// pairings naming its structs, without loading the tool packages.
func stubDecoders(t *testing.T, dir string, outputs ...structs.OutputPairing) {
	t.Helper()
	previous := collectPairings
	collectPairings = func(string) (structs.Pairings, error) {
		return structs.Pairings{ClientGoDir: dir, Outputs: outputs}, nil
	}
	t.Cleanup(func() { collectPairings = previous })
}

// TestOrbitCheck_TheDecoderHalf_HoldsEachClientGoStructToTheKindsRecorded
// verifies the half of the kinds question that asks what fails at run time:
// the struct a converter fills the output type from is held to the same
// recorded kinds under the decoder's reading. A count GitLab sends as a string
// and a list of strings holding a number are refused there too, while the
// nulls the published half refused (a plain int64, a struct value's element)
// are read without an error and pass. A pairing named twice is judged once; a
// struct that decodes its whole body itself, or that the parse did not find,
// is named at the body's path with the kinds the body came as; a field whose
// type decodes itself is unjudged, one that only writes itself is not; and an
// output no converter pairs is named apart.
func TestOrbitCheck_TheDecoderHalf_HoldsEachClientGoStructToTheKindsRecorded(t *testing.T) {
	root := writePackage(t, "orbitk", orbitKindsFixture)
	writeShared(t, root, orbitKindsShared)
	writeToolsPackage(t, root, "orbitd", orbitDecodersPackage)
	clientGo := filepath.Join(t.TempDir(), "client-go")
	if err := os.MkdirAll(clientGo, 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	writeSourceFile(t, clientGo, "orbit.go", orbitClientGo)
	toolsRoot := append([]orbitrecord.Key{{Path: orbitrecord.Root, Kinds: []string{orbitrecord.KindArray}}}, keys("[]", "[].name=string")...)
	calls := append(graphCalls(),
		orbitrecord.Call{Action: "orbit.tools", Variant: "default", Output: "internal/tools/orbitd.ToolsOutput", Response: orbitrecord.Response{Status: 200, Keys: toolsRoot}},
		orbitrecord.Call{Action: "orbit.text", Variant: "llm", Output: "internal/tools/orbitd.TextOutput", Response: orbitrecord.Response{Status: 200, Keys: keys(orbitrecord.Root, "content=string")}},
	)
	stubOrbitRecord(t, orbitrecord.Document{Calls: calls}, nil)
	stubDecoders(t, clientGo,
		structs.OutputPairing{Package: "orbitk", MCPType: "GraphOutput", SDKType: "GraphStatus"},
		structs.OutputPairing{Package: "orbitk", MCPType: "GraphOutput", SDKType: "Missing"},
		structs.OutputPairing{Package: "orbitk", MCPType: "GraphOutput", SDKType: "GraphStatus"},
		structs.OutputPairing{Package: "orbitd", MCPType: "ToolsOutput", SDKType: "Tools"},
	)

	check := orbitCheck(root)

	if !check.DecodersRead {
		t.Fatal("DecodersRead = false, want the decoder half run")
	}
	wantDecoders := []string{"orbitd.ToolsOutput from gl.Tools", "orbitk.GraphOutput from gl.GraphStatus", "orbitk.GraphOutput from gl.Missing"}
	if !slices.Equal(check.Decoders, wantDecoders) || !slices.Equal(check.Undecoded, []string{"orbitd.TextOutput"}) {
		t.Errorf("decoders = %q, undecoded = %q", check.Decoders, check.Undecoded)
	}
	graph := []string{"orbit.graph (raw)", "orbit.graph (text)"}
	pkg := "internal/tools/orbitk"
	wantMismatched := []OrbitField{
		{Package: pkg, Output: "GraphOutput", Path: "count", Type: "gl.GraphStatus", Field: "count", GoType: "int64", Kinds: []string{orbitrecord.KindString}, Calls: graph},
		{Package: pkg, Output: "GraphOutput", Path: "items.tags[]", Type: "gl.GraphItem", Field: "tags", GoType: "[]string", Kinds: []string{orbitrecord.KindNumber}, Calls: graph},
	}
	if !reflect.DeepEqual(check.DecoderMismatched, wantMismatched) {
		t.Errorf("decoder mismatched =\n%+v\nwant\n%+v", check.DecoderMismatched, wantMismatched)
	}
	wantUnjudged := []OrbitField{
		{Package: "internal/tools/orbitd", Output: "ToolsOutput", Path: orbitrecord.Root, Type: "gl.Tools", GoType: "Tools", Kinds: []string{orbitrecord.KindArray}, Calls: []string{"orbit.tools (default)"}},
		{Package: pkg, Output: "GraphOutput", Path: orbitrecord.Root, Type: "gl.Missing", GoType: "Missing", Kinds: []string{orbitrecord.KindObject}, Calls: graph},
		{Package: pkg, Output: "GraphOutput", Path: "clock", Type: "gl.GraphStatus", Field: "clock", GoType: "Clock", Kinds: []string{orbitrecord.KindString}, Calls: graph},
	}
	if !reflect.DeepEqual(check.DecoderKindsUnjudged, wantUnjudged) {
		t.Errorf("decoder unjudged =\n%+v\nwant\n%+v", check.DecoderKindsUnjudged, wantUnjudged)
	}
	// Fifteen fields of the struct are judged, two of them at their elements as
	// well; clock is not, and nothing below the raw result is.
	if check.DecoderKindsCompared != 17 {
		t.Errorf("DecoderKindsCompared = %d, want 17", check.DecoderKindsCompared)
	}
	// The published half's four findings and the decoder half's two are all
	// undeclared.
	if check.undeclared() != 6 {
		t.Errorf("undeclared() = %d, want 6", check.undeclared())
	}
}

// TestReadDecoderSource_WithoutAPairing_ReadsNothing verifies the two ways the
// decoder half is skipped: the pairing load failed, or it named no client-go
// directory to parse.
func TestReadDecoderSource_WithoutAPairing_ReadsNothing(t *testing.T) {
	cases := map[string]func(string) (structs.Pairings, error){
		"load failed":  func(string) (structs.Pairings, error) { return structs.Pairings{}, errors.New("no packages") },
		"no client-go": func(string) (structs.Pairings, error) { return structs.Pairings{}, nil },
		"failed and dir": func(string) (structs.Pairings, error) {
			return structs.Pairings{ClientGoDir: t.TempDir()}, errors.New("partial")
		},
	}
	for name, load := range cases {
		t.Run(name, func(t *testing.T) {
			previous := collectPairings
			collectPairings = load
			t.Cleanup(func() { collectPairings = previous })
			if got := readDecoderSource(t.TempDir()); got != nil {
				t.Errorf("readDecoderSource() = %+v, want nil", got)
			}
		})
	}
}
