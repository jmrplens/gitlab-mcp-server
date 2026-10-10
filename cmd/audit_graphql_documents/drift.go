package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
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
//
// Written as a chain of returns rather than a tagless switch, whose case
// expressions carry no statement counter for the mutation gate to see.
func (c coordinate) String() string {
	if c.fieldName == "" {
		return c.typeName
	}
	if c.argName == "" {
		return c.typeName + "." + c.fieldName
	}
	return c.typeName + "." + c.fieldName + "(" + c.argName + ")"
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
// A document neither schema accepts contributes no coordinates, since there is
// nothing to walk: a document is walked through the schema that validated it,
// and one that validated nowhere has no fields anybody can resolve. One of
// this repository's is already reported as a refusal by the gate.
//
// The documents client-go builds are a section of their own. They reach
// GitLab through this server as much as the repository's own do, and a re-pin
// that changes something only they read is a change a reader of this report
// has to see: the 19.5 re-pin added BUSINESS_LOGIC to SecurityScanProfileType,
// the enum client-go's scan profile document selects as scanType, and no
// report said so. A client-go document that contributes no coordinate is named
// as not walked rather than passed over, since a count that silently left it
// out reads as a document that touches nothing: a shell the collector could
// not render, and a document neither schema accepts. The second is named here
// and nowhere else in this command, because this repository's own documents
// are judged by the gate and client-go's are not.
func driftReport(pinned, probed *ast.Schema, documents []graphqldocs.Document, sdk sdkRead, pin graphqlschema.Source, now time.Time) string {
	var report strings.Builder
	report.WriteString(driftSection(pinned, probed, documents, "the documents touch"))
	if sdk.err != nil {
		fmt.Fprintf(&report, "%s client-go's documents were not read, so nothing they touch is compared: %v\n", prefix, sdk.err)
	} else {
		report.WriteString(driftSection(pinned, probed, sdk.documents, "client-go's documents touch"))
		for _, document := range sdk.documents {
			if why := notWalked(pinned, probed, document); why != "" {
				fmt.Fprintf(&report, "    not walked: %s (%s:%d), %s\n", document.Label(),
					filepath.Base(document.Position.Filename), document.Position.Line, why)
			}
		}
	}
	fmt.Fprintf(&report, "    the pin: %s%s\n", pin, pinAge(pin, now))
	return report.String()
}

// notWalked says why a document contributed no coordinate to its section, or
// "" when it was walked: a shell is not text either schema can parse, and a
// document neither schema accepts has no field anybody can resolve.
func notWalked(pinned, probed *ast.Schema, document graphqldocs.Document) string {
	if document.Assembly != nil && document.Assembly.Unrendered != "" {
		return "assembled at run time: " + document.Assembly.Unrendered
	}
	if _, parsed := parseUnderEither(probed, pinned, document.Text); parsed == nil {
		return "neither schema accepts it"
	}
	return ""
}

// sdkRead is what a run read of the documents client-go builds: the documents,
// or why there are none to compare.
type sdkRead struct {
	documents []graphqldocs.Document
	err       error
}

// driftSection compares the coordinates one set of documents touches, under a
// line naming whose documents they are.
func driftSection(pinned, probed *ast.Schema, documents []graphqldocs.Document, whose string) string {
	coordinates := touchedCoordinates(probed, pinned, documents)

	var differences []string
	for _, at := range coordinates {
		if difference := difference(pinned, probed, at); difference != "" {
			differences = append(differences, fmt.Sprintf("    %s: %s\n", at, difference))
		}
	}

	var section strings.Builder
	if len(differences) == 0 {
		fmt.Fprintf(&section, "%s the pin and the live schema agree on all %d coordinate(s) %s\n",
			prefix, len(coordinates), whose)
		return section.String()
	}
	fmt.Fprintf(&section, "%s the pin and the live schema disagree on %d of %d coordinate(s) %s\n",
		prefix, len(differences), len(coordinates), whose)
	for _, line := range differences {
		section.WriteString(line)
	}
	return section.String()
}

// pinAge renders how long ago the pin was taken, or "" when its record carries
// a date nothing can subtract. An unparseable date is left to the record's own
// decoding, which has already accepted it, and to gen_graphql_schema's --check,
// which refuses it, rather than turned into a second complaint about the same
// field here.
func pinAge(pin graphqlschema.Source, now time.Time) string {
	age, err := provenance.Age(pin.RetrievedAt, now)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(", %d day(s) ago", provenance.Days(age))
}

// touchedCoordinates returns every coordinate the documents depend on, sorted.
//
// Each document is walked through whichever schema accepts it, preferring the
// one this run judged by. The fallback matters on exactly the run that matters:
// a document the pin accepts and the live schema refuses is walked through the
// pin, so the drift report can name the field that stopped existing rather than
// falling silent about the document whose refusal prompted the question.
func touchedCoordinates(preferred, fallback *ast.Schema, documents []graphqldocs.Document) []coordinate {
	found := map[coordinate]bool{}
	for _, doc := range documents {
		schema, parsed := parseUnderEither(preferred, fallback, doc.Text)
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
	// Compared rather than ordered with <: the coordinates are a map's keys, so
	// no two render alike and a < and a <= would sort them the same.
	slices.SortFunc(coordinates, func(left, right coordinate) int { return strings.Compare(left.String(), right.String()) })
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
	if w.definition(name) != nil {
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
		return typeDifference(pinned, probed, pinnedType, probedType)
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
//
// Written as a chain of returns rather than a tagless switch, whose case
// expressions carry no statement counter for the mutation gate to see.
func presence(inPin, inLive bool) string {
	if inPin && !inLive {
		return "the pin has it, the live schema does not"
	}
	if !inPin && inLive {
		return "the live schema has it, the pin does not"
	}
	return ""
}

// typeDifference compares two definitions of the same named type, each read
// in the schema it belongs to.
func typeDifference(pinnedSchema, probedSchema *ast.Schema, pinned, probed *ast.Definition) string {
	if pinned.Kind != probed.Kind {
		return fmt.Sprintf("the pin says %s, the live schema says %s", pinned.Kind, probed.Kind)
	}
	switch pinned.Kind {
	case ast.Enum:
		return enumDifference(pinned, probed)
	case ast.Union:
		return memberDifference("members", pinnedSchema.GetPossibleTypes(pinned), probedSchema.GetPossibleTypes(probed))
	case ast.Interface:
		return memberDifference("implementations", pinnedSchema.GetPossibleTypes(pinned), probedSchema.GetPossibleTypes(probed))
	default:
		return ""
	}
}

// enumDifference reports the values one schema holds and the other does not.
//
// An enum is compared by value because that is how an enum breaks a document:
// GitLab accepts exactly the spellings it lists, so a value withdrawn between
// two releases turns a request our handlers still send into a refusal.
func enumDifference(pinned, probed *ast.Definition) string {
	return setDifference("", enumValueNames(pinned), enumValueNames(probed))
}

// memberDifference reports the possible types one schema gives a union or an
// interface and the other does not.
//
// An abstract type is compared by its members because that is how it changes
// under a document without any field of it changing: an inline fragment on a
// member the live schema withdrew is refused, and a member it added is an
// object the decoder meets with no fragment for it. The second is how GitLab
// 19.5 changed WorkItemWidget under this repository's own documents, adding
// the WorkItemWidgetDecisionLog implementation, and only this comparison
// reports it. A union no document selects is not compared at all:
// ScanProfileConfiguration gained five members in the same release, and no
// document of this repository or of client-go reaches it.
func memberDifference(noun string, pinned, probed []*ast.Definition) string {
	return setDifference(noun+" ", definitionNames(pinned), definitionNames(probed))
}

// setDifference reports the names one schema holds and the other does not,
// each list in a reader's order, or "" when the two hold the same names.
func setDifference(noun string, pinned, probed []string) string {
	dropped := namesMissingFrom(probed, pinned)
	added := namesMissingFrom(pinned, probed)

	var parts []string
	if len(dropped) > 0 {
		parts = append(parts, "drops "+noun+strings.Join(dropped, ", "))
	}
	if len(added) > 0 {
		parts = append(parts, "adds "+noun+strings.Join(added, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return "the live schema " + strings.Join(parts, " and ")
}

// enumValueNames is the names an enum lists.
func enumValueNames(definition *ast.Definition) []string {
	names := make([]string, 0, len(definition.EnumValues))
	for _, value := range definition.EnumValues {
		names = append(names, value.Name)
	}
	return names
}

// definitionNames is the names of a list of definitions.
func definitionNames(definitions []*ast.Definition) []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	return names
}

// namesMissingFrom returns the names of have that lack does not hold, sorted.
func namesMissingFrom(lack, have []string) []string {
	var missing []string
	for _, name := range have {
		if !slices.Contains(lack, name) {
			missing = append(missing, name)
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
