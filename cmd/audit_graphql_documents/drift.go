package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// coordinate names one place in a schema our documents depend on: a type, one
// field of a type, or one argument of that field.
//
// The whole drift between two GitLab releases is thousands of lines and tells
// nobody anything. The drift that matters is the drift under our own selection
// sets, which is what a coordinate is: every field a document selects, every
// argument it passes, and every type it names.
type coordinate struct {
	typeName  string
	fieldName string
	argName   string
}

// String renders a coordinate the way a reader would write it: Vulnerability,
// Vulnerability.severity, or Project.vulnerabilities(severity).
func (c coordinate) String() string {
	switch {
	case c.fieldName == "":
		return c.typeName
	case c.argName == "":
		return c.typeName + "." + c.fieldName
	default:
		return c.typeName + "." + c.fieldName + "(" + c.argName + ")"
	}
}

// driftReport says where the pinned schema and the one this run judged by
// disagree about something our documents touch.
//
// It exists so the pin's age is a number somebody sees rather than an
// assumption. A pin is a photograph of gitlab.com on one day, and every defect
// this gate was built for was GitLab narrowing a field after that day, so a run
// against a live instance is the only thing that can say how far the photograph
// has drifted from what an instance serves now.
//
// A document neither schema accepts contributes no coordinates. It is already
// reported as a refusal, and there is nothing to walk: a document is walked
// through the schema that validated it, and one that validated nowhere has no
// fields anybody can resolve.
func driftReport(pinned, probed *ast.Schema, documents []document, pin graphqlschema.Source, now time.Time) string {
	coordinates := touchedCoordinates(probed, pinned, documents)

	var differences []string
	for _, at := range coordinates {
		if difference := difference(pinned, probed, at); difference != "" {
			differences = append(differences, fmt.Sprintf("    %s: %s\n", at, difference))
		}
	}

	var report strings.Builder
	if len(differences) == 0 {
		fmt.Fprintf(&report, "%s the pin and the live schema agree on all %d coordinate(s) the documents touch\n",
			prefix, len(coordinates))
	} else {
		fmt.Fprintf(&report, "%s the pin and the live schema disagree on %d of %d coordinate(s) the documents touch\n",
			prefix, len(differences), len(coordinates))
		for _, line := range differences {
			report.WriteString(line)
		}
	}
	fmt.Fprintf(&report, "    the pin: %s%s\n", pin, pinAge(pin, now))
	return report.String()
}

// pinAge renders how long ago the pin was taken, or "" when its record carries
// a date nothing can subtract. An unparseable date is left to the record's own
// decoding, which has already accepted it, rather than turned into a second
// complaint about the same field.
func pinAge(pin graphqlschema.Source, now time.Time) string {
	retrieved, err := time.Parse(time.DateOnly, pin.RetrievedAt)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(", %d day(s) ago", int(now.UTC().Sub(retrieved).Hours()/24))
}

// touchedCoordinates returns every coordinate the documents depend on, sorted.
//
// Each document is walked through whichever schema accepts it, preferring the
// one this run judged by. The fallback matters on exactly the run that matters:
// a document the pin accepts and the live schema refuses is walked through the
// pin, so the drift report can name the field that stopped existing rather than
// falling silent about the document whose refusal prompted the question.
func touchedCoordinates(preferred, fallback *ast.Schema, documents []document) []coordinate {
	found := map[coordinate]bool{}
	for _, doc := range documents {
		schema, parsed := parseUnderEither(preferred, fallback, doc.text)
		if parsed == nil {
			continue
		}
		walker := &coordinateWalker{schema: schema, found: found, visited: map[string]bool{}}
		for _, operation := range parsed.Operations {
			walker.operation(operation)
		}
	}

	coordinates := make([]coordinate, 0, len(found))
	for at := range found {
		coordinates = append(coordinates, at)
	}
	sort.Slice(coordinates, func(i, j int) bool { return coordinates[i].String() < coordinates[j].String() })
	return coordinates
}

// parseUnderEither returns the first of the two schemas that accepts document,
// with the validated document it produced.
func parseUnderEither(preferred, fallback *ast.Schema, text string) (*ast.Schema, *ast.QueryDocument) {
	for _, schema := range []*ast.Schema{preferred, fallback} {
		if parsed, err := graphqlschema.ParseAgainst(schema, text); err == nil {
			return schema, parsed
		}
	}
	return nil, nil
}

// coordinateWalker collects the coordinates one document depends on.
type coordinateWalker struct {
	schema *ast.Schema
	found  map[coordinate]bool
	// visited holds the input types already descended into, so a self
	// referential input object ends the walk instead of the process.
	visited map[string]bool
}

// operation records what one operation depends on: the types of the variables
// it declares, and everything its selection set reaches.
func (w *coordinateWalker) operation(operation *ast.OperationDefinition) {
	for _, variable := range operation.VariableDefinitions {
		w.inputType(variable.Type.Name())
	}
	w.selections(operation.SelectionSet)
}

// selections walks one selection set. A fragment is walked through its
// definition rather than its name, since the fields it selects are the ones
// GitLab has to serve; a cycle is impossible, because a document with one does
// not validate and only validated documents are walked.
func (w *coordinateWalker) selections(set ast.SelectionSet) {
	for _, selection := range set {
		switch node := selection.(type) {
		case *ast.Field:
			w.field(node)
		case *ast.InlineFragment:
			w.namedType(node.TypeCondition)
			w.selections(node.SelectionSet)
		case *ast.FragmentSpread:
			if node.Definition != nil {
				w.namedType(node.Definition.TypeCondition)
				w.selections(node.Definition.SelectionSet)
			}
		}
	}
}

// field records one selected field, the arguments the document passes to it,
// and the type it returns.
//
// A field whose name begins with "__" is skipped: __typename and the
// introspection roots are the specification's, not GitLab's, and no GitLab
// release can narrow them. A field carrying neither the definition it resolved
// to nor the type it was selected on cannot be placed in any schema, so it is
// skipped rather than guessed at.
func (w *coordinateWalker) field(node *ast.Field) {
	if node.Definition == nil || node.ObjectDefinition == nil || strings.HasPrefix(node.Name, "__") {
		return
	}
	w.namedType(node.ObjectDefinition.Name)
	w.record(coordinate{typeName: node.ObjectDefinition.Name, fieldName: node.Name})
	for _, argument := range node.Arguments {
		w.record(coordinate{typeName: node.ObjectDefinition.Name, fieldName: node.Name, argName: argument.Name})
		if declared := node.Definition.Arguments.ForName(argument.Name); declared != nil {
			w.inputType(declared.Type.Name())
		}
	}
	w.namedType(node.Definition.Type.Name())
	w.selections(node.SelectionSet)
}

// inputType records an input type and descends into it.
//
// An input object is recorded whole, field by field, while an output object is
// recorded only for the fields a document selects. The asymmetry is the
// difference between the two: a document names every output field it wants, and
// hands an input object one value whose fields it never names, so any of them
// may be sent and all of them are in play.
func (w *coordinateWalker) inputType(name string) {
	if w.visited[name] {
		return
	}
	w.visited[name] = true

	definition := w.definition(name)
	if definition == nil {
		return
	}
	w.record(coordinate{typeName: name})
	for _, field := range definition.Fields {
		w.record(coordinate{typeName: name, fieldName: field.Name})
		w.inputType(field.Type.Name())
	}
}

// namedType records a type by name, without descending into it.
func (w *coordinateWalker) namedType(name string) {
	if definition := w.definition(name); definition != nil {
		w.record(coordinate{typeName: name})
	}
}

// definition resolves a type name, ignoring the ones gqlparser's own prelude
// defines: Int, String and their neighbors are the specification's and cannot
// drift between two GitLab releases, so counting them would inflate the number
// this report is judged by.
func (w *coordinateWalker) definition(name string) *ast.Definition {
	definition := w.schema.Types[name]
	if definition == nil || definition.BuiltIn {
		return nil
	}
	return definition
}

// record marks one coordinate as touched.
func (w *coordinateWalker) record(at coordinate) { w.found[at] = true }

// difference reports how the two schemas disagree about one coordinate, or ""
// when they agree about it.
func difference(pinned, probed *ast.Schema, at coordinate) string {
	pinnedType, probedType := pinned.Types[at.typeName], probed.Types[at.typeName]
	if pinnedType == nil || probedType == nil {
		return presence(pinnedType != nil, probedType != nil)
	}
	if at.fieldName == "" {
		return typeDifference(pinnedType, probedType)
	}

	pinnedField, probedField := pinnedType.Fields.ForName(at.fieldName), probedType.Fields.ForName(at.fieldName)
	if pinnedField == nil || probedField == nil {
		return presence(pinnedField != nil, probedField != nil)
	}
	if at.argName == "" {
		return typeNameDifference(pinnedField.Type, probedField.Type)
	}

	pinnedArg, probedArg := pinnedField.Arguments.ForName(at.argName), probedField.Arguments.ForName(at.argName)
	if pinnedArg == nil || probedArg == nil {
		return presence(pinnedArg != nil, probedArg != nil)
	}
	return typeNameDifference(pinnedArg.Type, probedArg.Type)
}

// presence reports a coordinate one schema has and the other does not. Two
// schemas that both lack it agree, which happens when a coordinate found under
// one document's schema is not reachable in the other at all.
func presence(inPin, inLive bool) string {
	switch {
	case inPin && !inLive:
		return "the pin has it, the live schema does not"
	case !inPin && inLive:
		return "the live schema has it, the pin does not"
	default:
		return ""
	}
}

// typeDifference compares two definitions of the same named type.
func typeDifference(pinned, probed *ast.Definition) string {
	if pinned.Kind != probed.Kind {
		return fmt.Sprintf("the pin says %s, the live schema says %s", pinned.Kind, probed.Kind)
	}
	if pinned.Kind == ast.Enum {
		return enumDifference(pinned, probed)
	}
	return ""
}

// enumDifference reports the values one schema holds and the other does not.
//
// An enum is compared by value because that is how an enum breaks a document:
// GitLab accepts exactly the spellings it lists, so a value withdrawn between
// two releases turns a request our handlers still send into a refusal.
func enumDifference(pinned, probed *ast.Definition) string {
	dropped := enumValuesMissingFrom(probed, pinned)
	added := enumValuesMissingFrom(pinned, probed)

	var parts []string
	if len(dropped) > 0 {
		parts = append(parts, "drops "+strings.Join(dropped, ", "))
	}
	if len(added) > 0 {
		parts = append(parts, "adds "+strings.Join(added, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return "the live schema " + strings.Join(parts, " and ")
}

// enumValuesMissingFrom returns the values of have that lack does not hold.
func enumValuesMissingFrom(lack, have *ast.Definition) []string {
	var missing []string
	for _, value := range have.EnumValues {
		if lack.EnumValues.ForName(value.Name) == nil {
			missing = append(missing, value.Name)
		}
	}
	sort.Strings(missing)
	return missing
}

// typeNameDifference compares two type references, which is the shape the
// defects this gate was built for take: a field or an argument that used to be
// [String!] and became [VulnerabilitySeverity!].
func typeNameDifference(pinned, probed *ast.Type) string {
	if pinned.String() == probed.String() {
		return ""
	}
	return fmt.Sprintf("the pin says %s, the live schema says %s", pinned, probed)
}
