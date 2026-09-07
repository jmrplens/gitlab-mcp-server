package graphqlintrospect

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// ref builds a named type reference.
func ref(name string) *TypeRef { return &TypeRef{Kind: kindScalar, Name: name} }

// nonNull wraps a reference so it cannot be null.
func nonNull(inner *TypeRef) *TypeRef { return &TypeRef{Kind: kindNonNull, OfType: inner} }

// list wraps a reference in a list.
func list(inner *TypeRef) *TypeRef { return &TypeRef{Kind: kindList, OfType: inner} }

// TestRenderSDL_EveryKind_ProducesSchemaGqlparserLoads is the test that matters
// for this renderer: the output is fed back into the parser that will judge
// every document, so a dropped implements clause or a mangled default is
// caught here rather than by a document being wrongly refused months later.
func TestRenderSDL_EveryKind_ProducesSchemaGqlparserLoads(t *testing.T) {
	schema := &Schema{
		QueryType:        &TypeName{Name: "Query"},
		MutationType:     &TypeName{Name: "Mutation"},
		SubscriptionType: &TypeName{Name: "Subscription"},
		Types: []Type{
			{Kind: kindObject, Name: "Query", Fields: []Field{
				{Name: "node", Args: []InputValue{
					{Name: "id", Type: nonNull(ref("ID"))},
					{Name: "limit", Type: ref("Int"), DefaultValue: new("100")},
				}, Type: ref("Node")},
				{Name: "everything", Type: list(nonNull(ref("Thing")))},
			}},
			{Kind: kindObject, Name: "Mutation", Fields: []Field{
				{Name: "touch", Args: []InputValue{{Name: "input", Type: nonNull(ref("TouchInput"))}}, Type: ref("Boolean")},
			}},
			{Kind: kindObject, Name: "Subscription", Fields: []Field{{Name: "tick", Type: ref("Time")}}},
			{Kind: kindObject, Name: "Thing", Interfaces: []TypeName{{Name: "Node"}, {Name: "Named"}}, Fields: []Field{
				{Name: "id", Type: nonNull(ref("ID"))},
				{Name: "name", Type: ref("String")},
				{Name: "mood", Type: ref("Mood")},
			}},
			{Kind: kindInterface, Name: "Node", Fields: []Field{{Name: "id", Type: nonNull(ref("ID"))}}},
			{Kind: kindInterface, Name: "Named", Interfaces: []TypeName{{Name: "Node"}}, Fields: []Field{
				{Name: "id", Type: nonNull(ref("ID"))},
				{Name: "name", Type: ref("String")},
			}},
			{Kind: kindUnion, Name: "Anything", PossibleTypes: []TypeName{{Name: "Thing"}}},
			{Kind: kindEnum, Name: "Mood", EnumValues: []TypeName{{Name: "GOOD"}, {Name: "BAD"}}},
			{Kind: kindInputObject, Name: "TouchInput", InputFields: []InputValue{
				{Name: "id", Type: nonNull(ref("ID"))},
				{Name: "note", Type: ref("String"), DefaultValue: new(`"none"`)},
			}},
			{Kind: kindScalar, Name: "Time"},
			// Omitted: the prelude defines these, and emitting them again
			// would fail the load with a duplicate definition.
			{Kind: kindScalar, Name: "String"},
			{Kind: kindObject, Name: "__Type", Fields: []Field{{Name: "name", Type: ref("String")}}},
			{Kind: "SOMETHING_NEW", Name: "Future"},
		},
	}

	sdl := RenderSDL(schema)

	if _, err := graphqlschema.Load([]byte(sdl)); err != nil {
		t.Fatalf("the rendered SDL does not load:\n%v\n\n%s", err, sdl)
	}
	cases := []struct {
		name string
		want string
	}{
		{name: "the schema block", want: "schema {\n  query: Query\n  mutation: Mutation\n  subscription: Subscription\n}"},
		{name: "an implements clause, sorted", want: "type Thing implements Named & Node {"},
		{name: "an interface implementing another", want: "interface Named implements Node {"},
		{name: "a union", want: "union Anything = Thing"},
		{name: "an enum", want: "enum Mood {\n  BAD\n  GOOD\n}"},
		{name: "a custom scalar", want: "scalar Time"},
		{name: "arguments sorted with a default", want: "node(id: ID!, limit: Int = 100): Node"},
		{name: "an input default", want: `note: String = "none"`},
		{name: "a list of non-null", want: "everything: [Thing!]"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if !strings.Contains(sdl, testCase.want) {
				t.Errorf("the SDL does not contain %q:\n%s", testCase.want, sdl)
			}
		})
	}
	for _, absent := range []string{"scalar String", "__Type", "Future"} {
		t.Run("omits "+absent, func(t *testing.T) {
			if strings.Contains(sdl, absent) {
				t.Errorf("the SDL contains %q, which SDL must not carry:\n%s", absent, sdl)
			}
		})
	}
}

// TestRenderSDL_InstanceWithoutMutations_EmitsOnlyTheRootsItHas verifies the
// branch a self-managed instance can take: a root the introspection does not
// name must not appear in the schema block as an empty entry.
func TestRenderSDL_InstanceWithoutMutations_EmitsOnlyTheRootsItHas(t *testing.T) {
	sdl := RenderSDL(&Schema{
		QueryType:    &TypeName{Name: "Query"},
		MutationType: &TypeName{},
		Types: []Type{
			{Kind: kindObject, Name: "Query", Fields: []Field{{Name: "ok", Type: ref("Boolean")}}},
		},
	})

	if !strings.Contains(sdl, "schema {\n  query: Query\n}") {
		t.Errorf("the schema block names a root the instance did not:\n%s", sdl)
	}
	if _, err := graphqlschema.Load([]byte(sdl)); err != nil {
		t.Fatalf("the rendered SDL does not load: %v", err)
	}
}

// TestRenderTypeRef_NestedWrappers_UnwrapInOrder verifies the one piece of
// this renderer with real recursion. A type reference read in the wrong order
// turns [String!]! into something else that still parses, which would be a
// schema that silently accepts and refuses the wrong documents.
func TestRenderTypeRef_NestedWrappers_UnwrapInOrder(t *testing.T) {
	cases := []struct {
		name string
		ref  *TypeRef
		want string
	}{
		{name: "nothing", ref: nil, want: ""},
		{name: "a name", ref: ref("String"), want: "String"},
		{name: "non-null", ref: nonNull(ref("ID")), want: "ID!"},
		{name: "a list", ref: list(ref("String")), want: "[String]"},
		{name: "a non-null list of non-null", ref: nonNull(list(nonNull(ref("String")))), want: "[String!]!"},
		{name: "a list of lists", ref: list(list(ref("Int"))), want: "[[Int]]"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := renderTypeRef(testCase.ref); got != testCase.want {
				t.Errorf("renderTypeRef() = %q, want %q", got, testCase.want)
			}
		})
	}
}
