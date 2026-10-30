// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package graphqlintrospect

import (
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// gitlabStyleDump is a schema written the way GitLab's graphql-schema-dump job
// writes one: descriptions on everything, directive definitions and their
// uses, deprecations, an interface implementing another, and a default of
// every literal kind GitLab declares.
const gitlabStyleDump = `"""
Marks a field as introduced in a specific version.
"""
directive @gl_introduced(version: String!) on FIELD | INLINE_FRAGMENT

directive @granularScope(permissions: [String!]) on FIELD_DEFINITION | OBJECT

schema {
  query: Query
  mutation: Mutation
}

"""
The root query.
"""
type Query {
  "Find a thing."
  thing(
    "Its ID."
    id: ID!
    first: Int = 20
    states: [Mood!] = [GOOD, BAD]
    filter: Filter = {name: "x", moods: [GOOD]}
    label: String = "name_asc"
    since: Time = null
  ): Thing @granularScope(permissions: ["read_thing"])
  old: String @deprecated(reason: "Use thing.")
}

type Mutation {
  touch(input: TouchInput!): Thing
}

interface Node {
  id: ID!
}

interface Named implements Node {
  id: ID!
  name: String
}

type Thing implements Named & Node @granularScope(permissions: ["read_thing"]) {
  id: ID!
  name: String
  mood: Mood
  everything: [[Thing!]]!
}

union Anything = Thing

enum Mood {
  "Good."
  GOOD
  BAD @deprecated(reason: "Gone.")
}

input Filter {
  name: String
  moods: [Mood!]
}

input TouchInput {
  id: ID!
  note: String = "none"
  dry: Boolean = false
}

scalar Time
`

// TestFromSDL_AGitLabDump_RendersAsTheIntrospectedPinIs is the test that
// matters for the reader: GitLab's dump read back through the introspection
// shape renders the way RenderSDL renders an introspection, so the release pin
// and the gitlab.com pin beside it differ only where GitLab does, and the
// result loads in the parser that judges every document.
func TestFromSDL_AGitLabDump_RendersAsTheIntrospectedPinIs(t *testing.T) {
	schema, err := FromSDL("dump.graphql", []byte(gitlabStyleDump))
	if err != nil {
		t.Fatalf("FromSDL() error = %v", err)
	}
	sdl := RenderSDL(schema)
	if _, err = graphqlschema.Load([]byte(sdl)); err != nil {
		t.Fatalf("the rendered dump does not load:\n%v\n\n%s", err, sdl)
	}
	for _, testCase := range []struct{ name, want string }{
		{name: "the schema block, with no subscription root", want: "schema {\n  query: Query\n  mutation: Mutation\n}"},
		{
			name: "arguments sorted, every default as introspection prints it",
			want: `thing(filter: Filter = {name: "x", moods: [GOOD]}, first: Int = 20, id: ID!, label: String = "name_asc", since: Time = null, states: [Mood!] = [GOOD, BAD]): Thing`,
		},
		{name: "a deprecated field kept", want: "  old: String\n"},
		{name: "an implements clause, sorted", want: "type Thing implements Named & Node {"},
		{name: "an interface implementing another", want: "interface Named implements Node {"},
		{name: "nested wrappers", want: "everything: [[Thing!]]!"},
		{name: "a union", want: "union Anything = Thing"},
		{name: "an enum with its deprecated value", want: "enum Mood {\n  BAD\n  GOOD\n}"},
		{name: "input defaults", want: "input TouchInput {\n  dry: Boolean = false\n  id: ID!\n  note: String = \"none\"\n}"},
		{name: "a custom scalar", want: "scalar Time"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if !strings.Contains(sdl, testCase.want) {
				t.Errorf("the rendered dump does not contain %q:\n%s", testCase.want, sdl)
			}
		})
	}
	for _, absent := range []string{"directive", "@", `"""`, "Find a thing", "__schema", "__type", "__Type", "scalar String"} {
		t.Run("drops "+absent, func(t *testing.T) {
			if strings.Contains(sdl, absent) {
				t.Errorf("the rendered dump carries %q, which the pin does not:\n%s", absent, sdl)
			}
		})
	}
}

// TestFromSDL_TheSameSchemaIntrospected_RendersByteForByteAlike holds the
// reader to the renderer's other input: a schema introspected and the same
// schema read from SDL render identically, which is what makes a diff of the
// release pin against the gitlab.com pin a list of GitLab's changes rather
// than of the two readers' differences.
func TestFromSDL_TheSameSchemaIntrospected_RendersByteForByteAlike(t *testing.T) {
	introspected := &Schema{
		QueryType: &TypeName{Name: "Query"},
		Types: []Type{
			{Kind: kindObject, Name: "Query", Fields: []Field{
				{Name: "things", Args: []InputValue{
					{Name: "first", Type: ref("Int"), DefaultValue: new("20")},
					{Name: "moods", Type: list(nonNull(ref("Mood"))), DefaultValue: new("[GOOD, BAD]")},
				}, Type: nonNull(list(ref("Thing")))},
			}},
			{Kind: kindObject, Name: "Thing", Fields: []Field{{Name: "id", Type: nonNull(ref("ID"))}}},
			{Kind: kindEnum, Name: "Mood", EnumValues: []TypeName{{Name: "GOOD"}, {Name: "BAD"}}},
		},
	}
	want := RenderSDL(introspected)

	read, err := FromSDL("again.graphql", []byte(want))
	if err != nil {
		t.Fatalf("FromSDL() error = %v", err)
	}
	if got := RenderSDL(read); got != want {
		t.Errorf("the schema read back from SDL renders as\n%s\nwant the introspected rendering\n%s", got, want)
	}
}

// TestFromSDL_AStringDefaultWithEscapes_RoundTripsThroughTheRendering verifies
// the one literal whose printing is this file's own: a string default holding
// every character GraphQL writes as an escape, and a control character it
// writes as \u, renders as a literal the loader reads back to the same value.
// Go's own quoting writes \x01 and \a, which GraphQL refuses, so a dump with
// such a default would render a release pin that does not load.
func TestFromSDL_AStringDefaultWithEscapes_RoundTripsThroughTheRendering(t *testing.T) {
	const want = "say \"hi\"\\ \b\f\n\r\t\x01 é"
	dump := "type Query {\n  ok(note: String = " + `"say \"hi\"\\ \b\f\n\r\t\u0001 é"` + "): Boolean\n}\n"
	schema, err := FromSDL("escapes.graphql", []byte(dump))
	if err != nil {
		t.Fatalf("FromSDL() error = %v", err)
	}
	sdl := RenderSDL(schema)
	if !strings.Contains(sdl, `note: String = "say \"hi\"\\ \b\f\n\r\t\u0001 é"`) {
		t.Errorf("the default does not render with GraphQL's escapes:\n%s", sdl)
	}
	loaded, err := graphqlschema.Load([]byte(sdl))
	if err != nil {
		t.Fatalf("the rendering does not load: %v\n%s", err, sdl)
	}
	if got := loaded.Query.Fields.ForName("ok").Arguments.ForName("note").DefaultValue.Raw; got != want {
		t.Errorf("the default reads back as %q, want %q", got, want)
	}
}

// TestFromSDL_NotSDL_NamesTheSource verifies that a dump that does not parse
// says which file it was, since the release generator reads either a download
// or a file it was handed.
func TestFromSDL_NotSDL_NamesTheSource(t *testing.T) {
	schema, err := FromSDL("broken-dump.graphql", []byte("type Query {"))
	if err == nil {
		t.Fatalf("FromSDL() = %+v, want an error", schema)
	}
	if !strings.Contains(err.Error(), "broken-dump.graphql") {
		t.Errorf("FromSDL() error = %q, want it to name the source", err)
	}
}

// TestTypeRef_ANameTheSchemaDoesNotDefine_KeepsTheName verifies the one
// reference the loader never produces from a valid schema, a name nothing
// defines, so the conversion keeps the name it was given rather than failing,
// beside a name the prelude defines, which takes its kind. The schema those
// are read from converts to its one type of its own: the loader's prelude is
// left out, since loading the rendering puts it back.
func TestTypeRef_ANameTheSchemaDoesNotDefine_KeepsTheName(t *testing.T) {
	schema, err := FromSDL("small.graphql", []byte("type Query {\n  ok: Boolean\n}\n"))
	if err != nil {
		t.Fatalf("FromSDL() error = %v", err)
	}
	if len(schema.Types) != 1 || schema.Types[0].Name != "Query" {
		t.Fatalf("FromSDL() types = %+v, want Query alone, the prelude left out", schema.Types)
	}
	loaded, err := graphqlschema.Load([]byte("type Query {\n  ok: Boolean\n}\n"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := typeRef(loaded, loaded.Query.Fields.ForName("ok").Type); got.Name != "Boolean" || got.Kind != kindScalar {
		t.Errorf("typeRef() = %+v, want the scalar Boolean", got)
	}
	if got := typeRef(loaded, ast.NonNullNamedType("Missing", nil)); renderTypeRef(got) != "Missing!" || got.OfType.Kind != "" {
		t.Errorf("typeRef() = %+v, want Missing! with no kind for the name nothing defines", got)
	}
}
