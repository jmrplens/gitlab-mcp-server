// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package graphqlintrospect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// FromSDL reads a schema written as SDL into the shape an introspection answer
// takes, so [RenderSDL] renders it exactly as it renders a schema fetched from
// an instance.
//
// It exists for the release pin. GitLab's own CI dumps the schema of every
// pipeline it runs (the graphql-schema-dump job, rake
// gitlab:graphql:schema:dump), the pipeline of a release tag included, so the
// schema a release serves can be read without booting that release. The dump
// is SDL with every description and directive GitLab declares, which is four
// times the size of the pin and orders nothing the way the pin does; reading
// it back through the introspection shape drops what validation never
// consults, as an introspection does, and sorts what the renderer sorts, so
// the two committed schemas differ only where GitLab does and a diff of one
// against the other reads as what changed between the release and gitlab.com.
//
// name labels the source in a parse error.
func FromSDL(name string, sdl []byte) (*Schema, error) {
	loaded, gqlErr := gqlparser.LoadSchema(&ast.Source{Name: name, Input: string(sdl)})
	if gqlErr != nil {
		return nil, fmt.Errorf("parse %s: %w", name, gqlErr)
	}
	return fromLoaded(loaded), nil
}

// fromLoaded converts a loaded schema. The prelude's own definitions are left
// out, as an introspection's are left out by the renderer, because the loader
// puts them back.
func fromLoaded(loaded *ast.Schema) *Schema {
	schema := &Schema{
		QueryType:        rootName(loaded.Query),
		MutationType:     rootName(loaded.Mutation),
		SubscriptionType: rootName(loaded.Subscription),
	}
	names := make([]string, 0, len(loaded.Types))
	for name, definition := range loaded.Types {
		if !definition.BuiltIn {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		schema.Types = append(schema.Types, typeFromDefinition(loaded, loaded.Types[name]))
	}
	return schema
}

// rootName names an operation root, or nil for a schema without one.
func rootName(root *ast.Definition) *TypeName {
	if root == nil {
		return nil
	}
	return &TypeName{Name: root.Name}
}

// definitionKinds maps the loader's kinds to the introspection's.
var definitionKinds = map[ast.DefinitionKind]string{
	ast.Scalar:      kindScalar,
	ast.Object:      kindObject,
	ast.Interface:   kindInterface,
	ast.Union:       kindUnion,
	ast.Enum:        kindEnum,
	ast.InputObject: kindInputObject,
}

// typeFromDefinition converts one definition.
//
// The loader keeps an input object's fields in the list it keeps an object's
// in, so the kind decides which of the two introspection lists they go to.
// The introspection fields the loader adds to the query root (__schema and
// __type) are dropped, since no introspection lists them as fields either.
func typeFromDefinition(loaded *ast.Schema, definition *ast.Definition) Type {
	converted := Type{
		Kind:          definitionKinds[definition.Kind],
		Name:          definition.Name,
		Interfaces:    typeNames(definition.Interfaces),
		PossibleTypes: typeNames(definition.Types),
	}
	for _, value := range definition.EnumValues {
		converted.EnumValues = append(converted.EnumValues, TypeName{Name: value.Name})
	}
	for _, field := range definition.Fields {
		if strings.HasPrefix(field.Name, "__") {
			continue
		}
		if definition.Kind == ast.InputObject {
			converted.InputFields = append(converted.InputFields, InputValue{
				Name: field.Name, Type: typeRef(loaded, field.Type), DefaultValue: literal(field.DefaultValue),
			})
			continue
		}
		args := make([]InputValue, 0, len(field.Arguments))
		for _, arg := range field.Arguments {
			args = append(args, InputValue{Name: arg.Name, Type: typeRef(loaded, arg.Type), DefaultValue: literal(arg.DefaultValue)})
		}
		converted.Fields = append(converted.Fields, Field{Name: field.Name, Args: args, Type: typeRef(loaded, field.Type)})
	}
	return converted
}

// typeNames wraps a list of names as references.
func typeNames(names []string) []TypeName {
	refs := make([]TypeName, 0, len(names))
	for _, name := range names {
		refs = append(refs, TypeName{Name: name})
	}
	return refs
}

// typeRef converts a type reference into the wrapper chain an introspection
// nests it in: non-null outermost, then the list, then the named type.
func typeRef(loaded *ast.Schema, typ *ast.Type) *TypeRef {
	if typ.NonNull {
		inner := *typ
		inner.NonNull = false
		return &TypeRef{Kind: kindNonNull, OfType: typeRef(loaded, &inner)}
	}
	if typ.Elem != nil {
		return &TypeRef{Kind: kindList, OfType: typeRef(loaded, typ.Elem)}
	}
	named := &TypeRef{Name: typ.NamedType}
	if definition := loaded.Types[typ.NamedType]; definition != nil {
		named.Kind = definitionKinds[definition.Kind]
	}
	return named
}

// literal renders a default value the way GitLab's introspection reports one
// (graphql-ruby's printer): a list as "[A, B]", an input object as
// "{name: value}", a string quoted. nil is no default, which is not the same
// as a default of null, and introspection keeps the two apart the same way.
func literal(value *ast.Value) *string {
	if value == nil {
		return nil
	}
	rendered := printValue(value)
	return &rendered
}

// stringEscapes are the characters a GraphQL string literal writes as a named
// escape. strconv.Quote is not used: it writes \a, \v and \x01, which GraphQL
// does not have, so a default holding one would render a schema that does not
// load.
var stringEscapes = map[rune]string{
	'"': `\"`, '\\': `\\`, '\b': `\b`, '\f': `\f`, '\n': `\n`, '\r': `\r`, '\t': `\t`,
}

// quoteString renders a string as a GraphQL string literal: the named escapes,
// any other control character as \u, and everything else as written.
func quoteString(raw string) string {
	var quoted strings.Builder
	quoted.WriteByte('"')
	for _, char := range raw {
		if escape, named := stringEscapes[char]; named {
			quoted.WriteString(escape)
			continue
		}
		if char < ' ' {
			fmt.Fprintf(&quoted, `\u%04x`, char)
			continue
		}
		quoted.WriteRune(char)
	}
	quoted.WriteByte('"')
	return quoted.String()
}

// printValue renders one value literal.
func printValue(value *ast.Value) string {
	switch value.Kind {
	case ast.StringValue, ast.BlockValue:
		return quoteString(value.Raw)
	case ast.ListValue:
		elements := make([]string, 0, len(value.Children))
		for _, child := range value.Children {
			elements = append(elements, printValue(child.Value))
		}
		return "[" + strings.Join(elements, ", ") + "]"
	case ast.ObjectValue:
		members := make([]string, 0, len(value.Children))
		for _, child := range value.Children {
			members = append(members, child.Name+": "+printValue(child.Value))
		}
		return "{" + strings.Join(members, ", ") + "}"
	default:
		return value.Raw
	}
}
