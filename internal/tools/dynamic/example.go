package dynamic

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// The values an example fills a constrained string with. Each satisfies the
// format or pattern it stands for, which is the whole point: the example used
// to write a date as "YYYY-MM-DD" and a color as "value", and a model that
// copied either sent a call its schema refused (issue 1175). The dates lie in
// the future, so an expiry the example sets is one GitLab accepts.
//
// The same held for every string whose form GitLab checks, an email address,
// a commit SHA, a cron expression, a duration and a redirect URI among them
// (issue 1272). A SHA is the 40 hex digits of a full SHA-1, which a route
// taking any revision takes as well, the digest the 64 of a SHA-256, the
// fingerprint the SHA256 form ssh-keygen prints, the duration one GitLab
// reads as an hour, and the version a release's first semantic version.
const (
	exampleDate        = "2030-01-31"
	exampleDateTime    = "2030-01-31T00:00:00Z"
	exampleURL         = "https://example.com"
	exampleHexColor    = "#1F75CB"
	exampleEmail       = "user@example.com"
	exampleHostname    = "example.com"
	exampleSHA         = "0123456789abcdef0123456789abcdef01234567"
	exampleDigest      = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	exampleCron        = "0 1 * * *"
	exampleDuration    = "1h"
	exampleFingerprint = "SHA256:ICEiIyQlJicoKSorLC0uLzAxMjM0NTY3ODk6Ozw9Pj8"
	exampleVersion     = "1.0.0"
	exampleProjectPath = "group/project"
	// examplePathPattern starts with exampleProjectPath, as GitLab requires
	// of a container repository path pattern. It is spelled out rather than
	// joined, so that no operator sits where a test could not tell it apart.
	examplePathPattern = "group/project/*"
)

// The numbers an example uses. exampleIdentifier stands in for any ID, and
// exampleAccessLevel is the access level an example grants: Developer, a
// level every route taking one accepts, the member roles' 10 to 50 among
// them. A schema states the levels only in its description, so the number
// chosen by type alone, 1, passed the schema and is no level GitLab knows.
const (
	exampleIdentifier  = 123
	exampleAccessLevel = 30
)

// patternExamples are the values tried, in order, for a string whose pattern
// refuses its placeholder: one per pattern shape the catalog declares (the hex
// color of a security attribute, the absolute URL of a release link, the
// digest of an attestation's subject, the cron expression of a schedule or a
// deploy freeze, the fingerprint of an SSH key, the semantic version of a
// changelog).
var patternExamples = []string{exampleHexColor, exampleURL, exampleDigest, exampleCron, exampleFingerprint, exampleVersion}

// exampleFor builds the gitlab_execute_action call find and describe offer for
// an action: every parameter the schema's root requires and the first
// alternative group, none of the others, each filled with a value its own
// schema accepts. It used to fill every name of every alternative and choose
// each value by the parameter's name alone, so security_attribute.update was
// offered all three of name, description and color, the last as "value",
// which its hex pattern refuses.
func exampleFor(entry actionEntry, schema map[string]any) ActionExample {
	arguments := map[string]any{
		"action": entry.ID,
		"params": exampleObject(schema),
	}
	if entry.Destructive {
		arguments["confirm"] = true
	}
	return ActionExample{
		Tool:      executeActionToolName,
		Arguments: arguments,
	}
}

// exampleObject fills the properties an object schema asks of every value:
// the ones its required list names and the first of its alternative groups,
// none of the others. The call's params are one such object, and so is an
// element of a list whose item schema says which of its fields an element
// names, as a protected environment's deploy access level names exactly one
// of a role, a user or a group (issue 1271).
func exampleObject(schema map[string]any) map[string]any {
	names := actioncatalog.RequiredParams(schema)
	if alternatives := actioncatalog.RequiredParamAlternatives(schema); len(alternatives) > 0 {
		names = append(names, alternatives[0]...)
	}
	properties := actionSchemaProperties(schema)
	object := make(map[string]any, len(names))
	for _, name := range names {
		property, _ := properties[name].(map[string]any)
		object[name] = exampleValue(name, property)
	}
	return object
}

// exampleValue returns a value for one parameter that its own schema accepts:
// a value of its enum, otherwise a value of the type the schema declares,
// shaped by its format and pattern, an array of one item and an object
// holding its required fields and its first alternative group, as the call's
// params do. A schema that declares no type it can fill is
// answered from its first oneOf or anyOf branch, and a parameter with no
// schema at all keeps the placeholder its name suggests.
//
// A schema that carries its own example is answered with the first of them,
// ahead of all that: an action that knows what a good value looks like says
// so in its schema, as orbit.query does with a GQL statement, which no type
// or name could produce.
//
// It covers the shapes the catalog's parameters take, and
// TestFind_EveryExampleSatisfiesItsInputSchema holds every action of every
// catalog to it, so a shape it does not know fails there rather than in a
// model's first call.
func exampleValue(name string, schema map[string]any) any {
	if values := exampleValues(schema); len(values) > 0 {
		return values[0]
	}
	if value, ok := enumExample(name, schema); ok {
		return value
	}
	switch exampleType(name, schema) {
	case "string":
		return exampleString(name, schema)
	case "integer", "number":
		return exampleNumber(name)
	case "boolean":
		return true
	case "array":
		items, _ := schema["items"].(map[string]any)
		return []any{exampleValue(strings.TrimSuffix(name, "s"), items)}
	case "object":
		return exampleObject(schema)
	}
	if branch := firstSchemaBranch(schema); branch != nil {
		return exampleValue(name, branch)
	}
	return placeholderForParam(name)
}

// enumExample returns the value an example takes from a schema's enum: the
// parameter's placeholder when the enum holds it, as the enum spells it, and
// otherwise the enum's first value. An access level's enum starts at 0, No
// access, which a group link refuses, and holds Developer, which the
// placeholder names. It reports false for a schema with no enum or an empty
// one.
func enumExample(name string, schema map[string]any) (any, bool) {
	values := enumValues(schema)
	if len(values) == 0 {
		return nil, false
	}
	placeholder := fmt.Sprint(placeholderForParam(name))
	for _, value := range values {
		if fmt.Sprint(value) == placeholder {
			return value, true
		}
	}
	return values[0], true
}

// enumValues returns the values a schema's enum allows. The enum is a []any
// when the schema was decoded from JSON and a []string where an action's
// override wrote it in Go (security_attribute.bulk_update's mode), and both
// reach the example.
func enumValues(schema map[string]any) []any {
	return schemaList(schema, "enum")
}

// exampleValues returns the JSON Schema examples a schema carries, in the two
// shapes [enumValues] reads.
func exampleValues(schema map[string]any) []any {
	return schemaList(schema, "examples")
}

// schemaList returns the list a schema keyword holds, as decoded JSON writes
// it ([]any) or as an action's override writes it in Go ([]string), and nil
// for anything else.
func schemaList(schema map[string]any, keyword string) []any {
	switch values := schema[keyword].(type) {
	case []any:
		return values
	case []string:
		converted := make([]any, 0, len(values))
		for _, value := range values {
			converted = append(converted, value)
		}
		return converted
	}
	return nil
}

// exampleType chooses which of the schema's declared types the example fills:
// the one the parameter's placeholder already is, when the schema allows it,
// so a project_id that takes a string or an integer is given a path, and
// otherwise the first type declared. It is empty when the schema declares no
// type but null.
func exampleType(name string, schema map[string]any) string {
	types := schemaTypes(schema)
	if len(types) == 0 {
		return ""
	}
	preferred := "string"
	if _, isNumber := placeholderForParam(name).(int); isNumber {
		preferred = "integer"
	}
	if slices.Contains(types, preferred) {
		return preferred
	}
	return types[0]
}

// schemaTypes returns the types a schema declares, null left out: the SDK
// writes a nullable Go slice as ["null","array"], and null is never the value
// an example should show.
func schemaTypes(schema map[string]any) []string {
	switch declared := schema["type"].(type) {
	case string:
		if declared != "null" {
			return []string{declared}
		}
	case []any:
		var types []string
		for _, raw := range declared {
			if name, ok := raw.(string); ok && name != "null" {
				types = append(types, name)
			}
		}
		return types
	}
	return nil
}

// exampleString returns a string its schema accepts: a value of the format the
// schema declares, otherwise the placeholder the name suggests, replaced by a
// known shape when a pattern refuses it.
func exampleString(name string, schema map[string]any) string {
	value := exampleFormatted(name, schema)
	if pattern, ok := schema["pattern"].(string); ok {
		return matchingExample(pattern, value)
	}
	return value
}

// exampleFormatted returns a value of the schema's format, or the name's
// placeholder as a string when the schema declares none the catalog uses.
func exampleFormatted(name string, schema map[string]any) string {
	switch schema["format"] {
	case "date":
		return exampleDate
	case "date-time":
		return exampleDateTime
	case "uri":
		return exampleURL
	case "email":
		return exampleEmail
	case "hostname":
		return exampleHostname
	}
	return fmt.Sprint(placeholderForParam(name))
}

// matchingExample returns value when the pattern accepts it, and otherwise the
// first of the known shapes it does accept. A pattern none of them satisfies,
// or one that does not compile, leaves value as it is: the catalog-wide test
// is what reports it.
func matchingExample(pattern, value string) string {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return value
	}
	for _, candidate := range append([]string{value}, patternExamples...) {
		if expression.MatchString(candidate) {
			return candidate
		}
	}
	return value
}

// exampleNumber returns the parameter's placeholder when it is a number, as
// for an access level, 123 for any other parameter named like an identifier,
// a group_id whose placeholder is a path included, and 1 otherwise, which is
// above the floor of every count and serial the catalog bounds.
func exampleNumber(name string) int {
	if number, isNumber := placeholderForParam(name).(int); isNumber {
		return number
	}
	if identifierParam(name) {
		return exampleIdentifier
	}
	return 1
}

// firstSchemaBranch returns the first object branch of a schema's oneOf or
// anyOf, which is how the catalog writes a parameter that takes one of several
// types (admin.feature_set's value).
func firstSchemaBranch(schema map[string]any) map[string]any {
	for _, keyword := range []string{"oneOf", "anyOf"} {
		branches, _ := schema[keyword].([]any)
		for _, raw := range branches {
			if branch, ok := raw.(map[string]any); ok {
				return branch
			}
		}
	}
	return nil
}

// placeholderForParam returns the value an example uses for a parameter by
// its name alone: a path for a project or group, a branch name, a URL, a
// color, an access level, a SHA, a duration, an identifier or a date.
// [exampleValue] starts from it and keeps it wherever the parameter's schema
// allows. A name GitLab gives a meaning its schema does not state is here for
// that reason: a label's color is a free string to its schema, and an access
// level an integer. So is a SHA where the route also takes a ref name, and a
// duration, both published with no pattern because one would refuse values
// GitLab takes (issue 1272); and a container repository's path pattern, which
// GitLab holds to the project's own path, a rule across two parameters that
// no pattern can state. A commit is a SHA as well: it is the name
// [exampleValue] gives the element of a list of commits, which is what
// merge_request.context_commits_create and context_commits_delete send.
func placeholderForParam(name string) any {
	switch name {
	case "project_id", "target_project_id":
		return exampleProjectPath
	case "repository_path_pattern":
		return examplePathPattern
	case "duration":
		return exampleDuration
	case "sha", "commit":
		return exampleSHA
	case "group_id", "namespace_id":
		return "group/subgroup"
	case "file_path", "artifact_path":
		return "path/to/file"
	case "ref", "branch", "branch_name", "target_branch", "source_branch":
		return "main"
	case "url", "remote_url", "external_url", "web_url":
		return exampleURL
	case "color":
		return exampleHexColor
	case "access_level", "group_access", "base_access_level":
		return exampleAccessLevel
	}
	if identifierParam(name) {
		return exampleIdentifier
	}
	if strings.HasSuffix(name, "_sha") {
		return exampleSHA
	}
	if strings.Contains(name, "date") {
		return exampleDate
	}
	return "value"
}

// identifierParam reports whether a parameter is named like a numeric
// identifier.
func identifierParam(name string) bool {
	return strings.HasSuffix(name, "_id") || name == "id" || strings.HasSuffix(name, "iid")
}
