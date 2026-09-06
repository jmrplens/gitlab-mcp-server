package main

import (
	"sort"
	"strings"
)

// The introspection kinds this renderer distinguishes.
const (
	kindScalar      = "SCALAR"
	kindObject      = "OBJECT"
	kindInterface   = "INTERFACE"
	kindUnion       = "UNION"
	kindEnum        = "ENUM"
	kindInputObject = "INPUT_OBJECT"
	kindNonNull     = "NON_NULL"
	kindList        = "LIST"
)

// builtinScalars are defined by gqlparser's own prelude, so emitting them
// again would make the schema fail to load with a duplicate definition.
var builtinScalars = map[string]bool{
	"Int": true, "Float": true, "String": true, "Boolean": true, "ID": true,
}

// renderSDL converts an introspection result into SDL.
//
// Everything nameable is sorted, so regenerating against an instance that
// reorders its answer produces a diff of what actually changed rather than a
// reshuffle. Order carries no meaning in SDL: a schema is a set of
// definitions, a type is a set of fields, and a field's arguments are matched
// by name.
func renderSDL(schema *schemaIntrospection) string {
	types := append([]schemaType(nil), schema.Types...)
	sort.Slice(types, func(i, j int) bool { return types[i].Name < types[j].Name })

	var blocks []string
	for i := range types {
		if block := renderType(&types[i]); block != "" {
			blocks = append(blocks, block)
		}
	}
	blocks = append(blocks, renderRoots(schema))
	return strings.Join(blocks, "\n\n") + "\n"
}

// renderRoots emits the schema block naming the operation roots. It is written
// out rather than left implicit because an instance is free to name its roots
// something other than Query, Mutation and Subscription.
func renderRoots(schema *schemaIntrospection) string {
	var lines []string
	for _, root := range []struct {
		operation string
		named     *typeName
	}{
		{"query", schema.QueryType},
		{"mutation", schema.MutationType},
		{"subscription", schema.SubscriptionType},
	} {
		if root.named != nil && root.named.Name != "" {
			lines = append(lines, "  "+root.operation+": "+root.named.Name)
		}
	}
	return "schema {\n" + strings.Join(lines, "\n") + "\n}"
}

// renderType emits one type definition, or "" for a type SDL must not carry:
// an introspection type, or a scalar the prelude already defines.
func renderType(definition *schemaType) string {
	if strings.HasPrefix(definition.Name, "__") {
		return ""
	}
	switch definition.Kind {
	case kindScalar:
		if builtinScalars[definition.Name] {
			return ""
		}
		return "scalar " + definition.Name
	case kindObject:
		return renderFielded("type", definition)
	case kindInterface:
		return renderFielded("interface", definition)
	case kindUnion:
		return "union " + definition.Name + " = " + strings.Join(sortedNames(definition.PossibleTypes), " | ")
	case kindEnum:
		return renderEnum(definition)
	case kindInputObject:
		return renderInputObject(definition)
	default:
		return ""
	}
}

// renderFielded emits an object or interface, which differ only in keyword.
func renderFielded(keyword string, definition *schemaType) string {
	head := keyword + " " + definition.Name
	if implemented := sortedNames(definition.Interfaces); len(implemented) > 0 {
		head += " implements " + strings.Join(implemented, " & ")
	}

	fields := append([]schemaItem(nil), definition.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })

	lines := make([]string, 0, len(fields))
	for _, field := range fields {
		lines = append(lines, "  "+field.Name+renderArgs(field.Args)+": "+renderTypeRef(field.Type))
	}
	return head + " {\n" + strings.Join(lines, "\n") + "\n}"
}

// renderEnum emits an enum with its values.
func renderEnum(definition *schemaType) string {
	values := sortedNames(definition.EnumValues)
	lines := make([]string, 0, len(values))
	for _, value := range values {
		lines = append(lines, "  "+value)
	}
	return "enum " + definition.Name + " {\n" + strings.Join(lines, "\n") + "\n}"
}

// renderInputObject emits an input object with its fields and defaults.
func renderInputObject(definition *schemaType) string {
	fields := append([]inputValue(nil), definition.InputFields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })

	lines := make([]string, 0, len(fields))
	for _, field := range fields {
		lines = append(lines, "  "+field.Name+": "+renderTypeRef(field.Type)+renderDefault(field.DefaultValue))
	}
	return "input " + definition.Name + " {\n" + strings.Join(lines, "\n") + "\n}"
}

// renderArgs emits an argument list, or "" for a field that takes none.
func renderArgs(args []inputValue) string {
	if len(args) == 0 {
		return ""
	}
	sorted := append([]inputValue(nil), args...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	rendered := make([]string, 0, len(sorted))
	for _, arg := range sorted {
		rendered = append(rendered, arg.Name+": "+renderTypeRef(arg.Type)+renderDefault(arg.DefaultValue))
	}
	return "(" + strings.Join(rendered, ", ") + ")"
}

// renderDefault emits a default value clause. Introspection reports the
// literal as it would be written in SDL, so it is emitted verbatim.
func renderDefault(value *string) string {
	if value == nil {
		return ""
	}
	return " = " + *value
}

// renderTypeRef unwraps the NON_NULL and LIST nodes introspection nests a type
// reference in.
func renderTypeRef(ref *typeRef) string {
	if ref == nil {
		return ""
	}
	switch ref.Kind {
	case kindNonNull:
		return renderTypeRef(ref.OfType) + "!"
	case kindList:
		return "[" + renderTypeRef(ref.OfType) + "]"
	default:
		return ref.Name
	}
}

// sortedNames returns the names of a reference list, sorted.
func sortedNames(refs []typeName) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	sort.Strings(names)
	return names
}
