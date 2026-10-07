package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestRenderTable_KeyedLiteralsOfEveryField verifies the table is written as
// keyed composite literals, every field the table carries spelled once and a
// zero one left out, a boundary, a cause and an effect as the constants that
// name them, a bit set in hexadecimal, the displays once each and a
// permission's display as its index among them, and the whole formatted the
// way gofmt writes it.
func TestRenderTable_KeyedLiteralsOfEveryField(t *testing.T) {
	table := &finegrained.Table{
		Version:     "19.4.1-ee",
		Bucket:      "19.4",
		Permissions: []string{"read_a", "read_b", "read_c", "read_d"},
		Displays:    []string{"", "A read", "A: Read"},
		Display:     []uint16{2, 0, 2, 1},
		Assignables: []finegrained.Assignable{
			{Name: "read_a", Permissions: []uint16{0, 2}, Boundaries: finegrained.BoundaryProject | finegrained.BoundaryGroup, Grantable: true},
			{Name: "read_old", Permissions: []uint16{0, 1}, Deprecated: true},
		},
		PublicAnonymous: [2][]uint64{{0x1}, {}},
		PublicKnown:     true,
		Groups: []finegrained.Group{
			{Perms: []uint16{0}, Any: finegrained.AllBoundaries},
		},
		Operations: []finegrained.Operation{
			{Name: "GET /a", Classic: finegrained.ClassicReadAPI, Groups: []uint32{0}},
			{Name: "mutation m (x.Y)", Classic: finegrained.ClassicAPI, Skip: true, Spine: []uint32{0}, OffSpine: []uint32{1}},
		},
		Elements: []finegrained.Element{
			{Path: "m.thing", Type: "Thing", Groups: []uint32{0}, Effect: finegrained.EffectNull},
			{Path: "m.either", Type: "Either", Members: []string{"A", "B"}, Undeclared: true, Effect: finegrained.EffectRemoved},
		},
		Actions: []finegrained.Requirement{
			{ID: "a.denied", Classic: finegrained.ClassicOtherCredential, Denied: &finegrained.Denial{Cause: finegrained.CauseRESTTodo, Element: "GET /a", Effect: finegrained.EffectRefused}},
			{ID: "a.read", Classic: finegrained.ClassicNoRequest, Paths: [][]uint32{{0}, {0, 1}}, Degraded: []uint32{1}, GraphQL: true, Collection: true},
			{ID: "a.some", Paths: [][]uint32{{0}}, DeniedWays: []finegrained.Denial{
				{Cause: finegrained.CauseTypeUndeclared, Element: "Namespace", Effect: finegrained.EffectNull},
				{Cause: finegrained.CauseRESTUndeclared, Element: "GET /b", Effect: finegrained.EffectRefused},
			}},
		},
	}
	want := tableHeader + `	Version: "19.4.1-ee",
	Bucket:  "19.4",
	Permissions: []string{
		"read_a",
		"read_b",
		"read_c",
		"read_d",
	},
	Displays: []string{
		"",
		"A read",
		"A: Read",
	},
	Display: []uint16{2, 0, 2, 1},
	Assignables: []finegrained.Assignable{
		{Name: "read_a", Permissions: []uint16{0, 2}, Boundaries: finegrained.BoundaryProject | finegrained.BoundaryGroup, Grantable: true},
		{Name: "read_old", Permissions: []uint16{0, 1}, Boundaries: 0, Deprecated: true},
	},
	PublicAnonymous: [2][]uint64{
		{0x1},
		{},
	},
	PublicKnown: true,
	Groups: []finegrained.Group{
		{Perms: []uint16{0}, Any: finegrained.BoundaryProject | finegrained.BoundaryGroup | finegrained.BoundaryUser | finegrained.BoundaryInstance},
	},
	Operations: []finegrained.Operation{
		{Name: "GET /a", Classic: finegrained.ClassicReadAPI, Groups: []uint32{0}},
		{Name: "mutation m (x.Y)", Classic: finegrained.ClassicAPI, Skip: true, Spine: []uint32{0}, OffSpine: []uint32{1}},
	},
	Elements: []finegrained.Element{
		{Path: "m.thing", Type: "Thing", Groups: []uint32{0}, Effect: finegrained.EffectNull},
		{Path: "m.either", Type: "Either", Members: []string{"A", "B"}, Undeclared: true, Effect: finegrained.EffectRemoved},
	},
	Actions: []finegrained.Requirement{
		{ID: "a.denied", Classic: finegrained.ClassicOtherCredential, Denied: &finegrained.Denial{Cause: finegrained.CauseRESTTodo, Element: "GET /a", Effect: finegrained.EffectRefused}},
		{ID: "a.read", Classic: finegrained.ClassicNoRequest, Paths: [][]uint32{{0}, {0, 1}}, Degraded: []uint32{1}, GraphQL: true, Collection: true},
		{ID: "a.some", Classic: finegrained.ClassicUnknown, Paths: [][]uint32{{0}}, DeniedWays: []finegrained.Denial{{Cause: finegrained.CauseTypeUndeclared, Element: "Namespace", Effect: finegrained.EffectNull}, {Cause: finegrained.CauseRESTUndeclared, Element: "GET /b", Effect: finegrained.EffectRefused}}},
	},
}
`
	if got := string(renderTable(table)); got != want {
		t.Errorf("renderTable =\n%s\nwant\n%s", got, want)
	}
}

// TestConstantOf_AValueWithNoConstant_Panics verifies a cause or an effect the
// writer does not name stops the run rather than being written as a value no
// constant spells.
func TestConstantOf_AValueWithNoConstant_Panics(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil || !strings.Contains(recovered.(string), `finegrained.Effect "vanished" has no constant`) {
			t.Errorf("recovered %v, want the panic naming the effect", recovered)
		}
	}()
	constantOf(effectConstants, finegrained.Effect("vanished"))
}

// TestClassicConstants_NameEveryClassicScope verifies the writer spells each
// classic scope by the constant that names it, and stops on a value past the
// last one, naming it.
func TestClassicConstants_NameEveryClassicScope(t *testing.T) {
	want := []struct {
		scope finegrained.ClassicScope
		name  string
	}{
		{finegrained.ClassicUnknown, "finegrained.ClassicUnknown"},
		{finegrained.ClassicNoRequest, "finegrained.ClassicNoRequest"},
		{finegrained.ClassicOtherCredential, "finegrained.ClassicOtherCredential"},
		{finegrained.ClassicReadAPI, "finegrained.ClassicReadAPI"},
		{finegrained.ClassicAPI, "finegrained.ClassicAPI"},
	}
	if len(classicConstants) != len(want) {
		t.Errorf("the writer names %d classic scopes, want %d", len(classicConstants), len(want))
	}
	for _, tc := range want {
		t.Run(tc.name, func(t *testing.T) {
			if got := constantOf(classicConstants, tc.scope); got != tc.name {
				t.Errorf("ClassicScope(%d) is written as %q, want %q", tc.scope, got, tc.name)
			}
		})
	}
	defer func() {
		if recovered := recover(); recovered == nil || !strings.Contains(recovered.(string), `finegrained.ClassicScope "unknown" has no constant`) {
			t.Errorf("recovered %v, want the panic naming the scope", recovered)
		}
	}()
	constantOf(classicConstants, finegrained.ClassicAPI+1)
}

// readTypedConstants files every constant of one source file whose declared
// type is a key of declared under that type, its value mapped to the name the
// generated table would spell it by.
func readTypedConstants(t *testing.T, fset *token.FileSet, file string, declared map[string]map[string]string) {
	t.Helper()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		if gen, isGen := decl.(*ast.GenDecl); isGen && gen.Tok == token.CONST {
			for _, spec := range gen.Specs {
				fileTypedConstant(t, spec.(*ast.ValueSpec), declared)
			}
		}
	}
}

// fileTypedConstant files one constant spec when its type is one declared
// collects.
func fileTypedConstant(t *testing.T, value *ast.ValueSpec, declared map[string]map[string]string) {
	t.Helper()
	typ, isIdent := value.Type.(*ast.Ident)
	if !isIdent || declared[typ.Name] == nil {
		return
	}
	literal, err := strconv.Unquote(value.Values[0].(*ast.BasicLit).Value)
	if err != nil {
		t.Fatalf("read %s: %v", value.Names[0].Name, err)
	}
	declared[typ.Name][literal] = "finegrained." + value.Names[0].Name
}

// TestCauseAndEffectConstants_NameEveryValueFinegrainedDeclares verifies the
// writer names every Cause and Effect constant finegrained declares, each by
// its own name, read from the package's source so a value added there fails
// here rather than at the first table that carries it.
func TestCauseAndEffectConstants_NameEveryValueFinegrainedDeclares(t *testing.T) {
	declared := map[string]map[string]string{"Cause": {}, "Effect": {}}
	// Found from the module root, so a copy of this package staged elsewhere
	// in the module reads the same sources.
	files, err := filepath.Glob(filepath.Join(moduleRoot(t), "internal", "finegrained", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("list the finegrained sources: %v, %d files", err, len(files))
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if !strings.HasSuffix(file, "_test.go") {
			readTypedConstants(t, fset, file, declared)
		}
	}
	causes := map[string]string{}
	for cause, name := range causeConstants {
		causes[string(cause)] = name
	}
	effects := map[string]string{}
	for effect, name := range effectConstants {
		effects[string(effect)] = name
	}
	for kind, written := range map[string]map[string]string{"Cause": causes, "Effect": effects} {
		t.Run(kind, func(t *testing.T) {
			if len(declared[kind]) == 0 {
				t.Fatalf("finegrained declares no %s constant", kind)
			}
			if len(written) != len(declared[kind]) {
				t.Errorf("the writer names %d %s values and finegrained declares %d", len(written), kind, len(declared[kind]))
			}
			for value, name := range declared[kind] {
				if written[value] != name {
					t.Errorf("%s %q is written as %q, want %q", kind, value, written[value], name)
				}
			}
		})
	}
}
