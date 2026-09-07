package graphqlschema

import (
	_ "embed" // registers the go:embed directive that carries the pinned schema
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
)

// SDLFileName is the pinned schema's name on disk. The generator writes it and
// the package embeds it, so both sides name it once here.
//
// The words are separated by a hyphen rather than an underscore because
// cmd/audit_doc_tool_names reads every gitlab_-prefixed identifier in the
// documentation as a tool name the server must register, and any page naming
// this file would otherwise fail that gate.
const SDLFileName = "gitlab-schema.graphql"

//go:embed gitlab-schema.graphql
var pinnedSDL []byte

// schemaOnce guards the one parse this process pays for. Parsing costs around
// 200 ms, which is affordable once and not affordable per validated document.
// The outcome is one value rather than two package variables so the error is
// plainly the result of that parse and not a sentinel anybody may compare to.
var (
	schemaOnce sync.Once
	pinned     struct {
		schema *ast.Schema
		err    error
	}
)

// Schema returns the pinned GitLab schema, parsing the embedded SDL on first
// call and returning the same schema to every later caller.
//
// The returned schema is read-only. gqlparser's validator never mutates it,
// and callers must not either, because every goroutine in the process shares
// this one value.
func Schema() (*ast.Schema, error) {
	schemaOnce.Do(func() { pinned.schema, pinned.err = Load(pinnedSDL) })
	return pinned.schema, pinned.err
}

// schemaFor is the loader [Validate] and [ValidateDocument] go through. It is
// a variable so the load-failure path stays reachable from a test, since the
// embedded schema cannot be made to fail from outside the package.
var schemaFor = Schema

// Load parses an SDL document. It is exported so cmd/gen_graphql_schema can
// check a file it just wrote, or a committed one it did not, without going
// through the embedded copy.
func Load(sdl []byte) (*ast.Schema, error) {
	schema, gqlErr := gqlparser.LoadSchema(&ast.Source{Name: SDLFileName, Input: string(sdl)})
	if gqlErr != nil {
		return nil, fmt.Errorf("parse the schema: %w", gqlErr)
	}
	return schema, nil
}

// ValidationError carries every reason the pinned schema refused a document.
//
// The reasons are kept apart rather than pre-joined because the two callers
// present them differently: the test transport prints one per line under the
// operation it came from, and the document audit indents them under the
// constant that declares the document.
type ValidationError struct {
	// Reasons is one message per validation failure, in the order gqlparser
	// reported them, followed by the variable failures this package adds.
	Reasons []string
}

// Error joins the reasons into one line.
func (e *ValidationError) Error() string {
	return strings.Join(e.Reasons, "; ")
}

// ValidateDocument reports whether the pinned schema accepts document,
// ignoring variable values. This is what a static audit can ask, since a
// document read out of the source has no request behind it.
func ValidateDocument(document string) error {
	schema, err := schemaFor()
	if err != nil {
		return err
	}
	_, err = parseDocument(schema, document)
	return err
}

// ValidateDocumentAgainst is [ValidateDocument] against a schema the caller
// loaded rather than the pinned one.
//
// It exists for the live re-probe. The pin can only say a document was already
// broken on the day it was taken, never that GitLab has narrowed something
// since, which is how every defect this gate was built for arose:
// securityReportFindings accepted confidence once, and
// Project.vulnerabilities.severity was [String!] once. Handing the same check a
// schema fetched today turns that into a failure on the day it happens.
func ValidateDocumentAgainst(schema *ast.Schema, document string) error {
	_, err := ParseAgainst(schema, document)
	return err
}

// ParseAgainst is [ValidateDocumentAgainst] returning what it parsed, so a
// caller can walk the fields, arguments and types a document actually depends
// on rather than the text it is written as.
//
// The live re-probe compares two schemas that way. Whole-schema drift between
// two GitLab releases is thousands of lines and says nothing; the drift under
// our own selection sets is a handful of coordinates and says everything, and
// resolving those needs the validated document, where every field carries the
// definition it resolved to.
func ParseAgainst(schema *ast.Schema, document string) (*ast.QueryDocument, error) {
	return parseDocument(schema, document)
}

// Validate reports whether GitLab would accept document sent with variables.
//
// It answers both halves of that question. The document half catches a field
// that does not exist, an argument the field does not accept and a variable
// used where its type does not fit. The variables half catches a variable sent
// that the operation never declared, which is the defect that let eight
// domains advertise backward pagination, and a value that does not fit the
// type it was declared as.
func Validate(document string, variables map[string]any) error {
	schema, err := schemaFor()
	if err != nil {
		return err
	}
	parsed, err := parseDocument(schema, document)
	if err != nil {
		return err
	}
	return validateVariables(schema, parsed, variables)
}

// parseDocument parses and validates one document against schema.
func parseDocument(schema *ast.Schema, document string) (*ast.QueryDocument, error) {
	parsed, errs := gqlparser.LoadQuery(schema, document)
	if len(errs) == 0 {
		return parsed, nil
	}
	reasons := make([]string, 0, len(errs))
	for _, err := range errs {
		reasons = append(reasons, err.Message)
	}
	return nil, &ValidationError{Reasons: reasons}
}

// validateVariables checks the variables a request carries against the
// operation that has to bind them.
func validateVariables(schema *ast.Schema, parsed *ast.QueryDocument, variables map[string]any) error {
	if len(parsed.Operations) == 0 {
		// A document defining only fragments binds nothing. GitLab refuses to
		// execute it, which the document half already reported.
		return nil
	}
	// client-go sends {query, variables} and no operationName, so a document
	// defining more than one operation names nothing GitLab can select and is
	// a defect however well each operation validates on its own.
	if len(parsed.Operations) > 1 {
		return &ValidationError{Reasons: []string{fmt.Sprintf(
			"the document defines %d operations and the request names none, so GitLab cannot choose one",
			len(parsed.Operations),
		)}}
	}

	operation := parsed.Operations[0]
	reasons := undeclaredVariables(operation, variables)
	if _, coerceErr := validator.VariableValues(schema, operation, variables); coerceErr != nil {
		reasons = append(reasons, coerceErr.Error())
	}
	reasons = append(reasons, miscasedEnums(schema, operation, variables)...)
	if len(reasons) > 0 {
		return &ValidationError{Reasons: reasons}
	}
	return nil
}

// miscasedEnums reports every enum value the request sends in the wrong case.
//
// gqlparser compares an enum value with strings.EqualFold (validator/vars.go),
// so it accepts "critical" for VulnerabilitySeverity. GitLab does not: it
// answers "Expected \"critical\" to be one of: INFO, UNKNOWN, LOW, MEDIUM,
// HIGH, CRITICAL" and executes nothing. Without this walk the gate misses a
// whole defect class it is meant to catch, and it is the class our own code is
// most likely to produce, since every enum this server sends is normalised by
// a strings.ToUpper somewhere that a refactor can turn into strings.ToLower.
//
// Only variable values need this. An enum written as a literal in the document
// is matched exactly by gqlparser's ValuesOfCorrectType rule, so the document
// half already refuses a miscased literal.
func miscasedEnums(schema *ast.Schema, operation *ast.OperationDefinition, variables map[string]any) []string {
	walker := enumWalker{schema: schema}
	for _, definition := range operation.VariableDefinitions {
		value, sent := variables[definition.Variable]
		if !sent {
			continue
		}
		walker.walk(definition.Type, value, "$"+definition.Variable)
	}
	sort.Strings(walker.reasons)
	return walker.reasons
}

// enumWalker collects the miscased enum values found under one operation's
// variables. It carries the schema and the findings so the recursion can stay a
// method with a path argument rather than a function threading three values.
type enumWalker struct {
	schema  *ast.Schema
	reasons []string
}

// walk descends one value against the type it was declared as, reporting every
// enum position whose value differs from a schema value only in case.
//
// It mirrors the descent gqlparser performs, through list elements and input
// object fields, because an enum reaches GitLab inside an input object as
// readily as it does at the top level: the security attribute and vulnerability
// mutations pass every enum that way. Anything the walk cannot resolve, an
// unknown type name or a value shaped unlike its type, is left alone, since
// gqlparser's own coercion has already reported it.
func (w *enumWalker) walk(typ *ast.Type, value any, path string) {
	if value == nil || typ == nil {
		return
	}
	if typ.Elem != nil {
		elements, ok := value.([]any)
		if !ok {
			return
		}
		for i, element := range elements {
			w.walk(typ.Elem, element, fmt.Sprintf("%s[%d]", path, i))
		}
		return
	}

	definition := w.schema.Types[typ.NamedType]
	if definition == nil {
		return
	}
	switch definition.Kind {
	case ast.Enum:
		w.checkEnum(definition, value, path)
	case ast.InputObject:
		fields, ok := value.(map[string]any)
		if !ok {
			return
		}
		for name, field := range fields {
			fieldDef := definition.Fields.ForName(name)
			if fieldDef == nil {
				continue
			}
			w.walk(fieldDef.Type, field, path+"."+name)
		}
	default:
	}
}

// checkEnum reports value when the enum holds it in a different case only. A
// value the enum does not hold at all in any case is left to gqlparser, which
// reports it with its own message rather than having two for one mistake.
func (w *enumWalker) checkEnum(definition *ast.Definition, value any, path string) {
	text, ok := value.(string)
	if !ok {
		return
	}
	for _, enumValue := range definition.EnumValues {
		if text == enumValue.Name {
			return
		}
	}
	for _, enumValue := range definition.EnumValues {
		if strings.EqualFold(text, enumValue.Name) {
			w.reasons = append(w.reasons, fmt.Sprintf(
				"%s is %q, and %s is case sensitive: GitLab accepts %q and refuses every other spelling",
				path, text, definition.Name, enumValue.Name,
			))
			return
		}
	}
}

// undeclaredVariables reports every variable the request sends that the
// operation does not declare.
//
// gqlparser's VariableValues walks the operation's declarations and ignores
// anything else in the map, so an undeclared variable is silently dropped by
// the validator exactly as it is silently dropped by GitLab. That silence is
// the whole defect: a handler that sends "last" and "before" to an operation
// declaring neither paginates forward while reporting that it paginated back.
func undeclaredVariables(operation *ast.OperationDefinition, variables map[string]any) []string {
	declared := make(map[string]bool, len(operation.VariableDefinitions))
	for _, definition := range operation.VariableDefinitions {
		declared[definition.Variable] = true
	}

	names := make([]string, 0, len(variables))
	for name := range variables {
		if !declared[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	reasons := make([]string, 0, len(names))
	for _, name := range names {
		reasons = append(reasons, fmt.Sprintf(
			"variable $%s is sent but the operation does not declare it, so GitLab discards it", name,
		))
	}
	return reasons
}
