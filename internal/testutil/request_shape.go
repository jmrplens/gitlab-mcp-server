package testutil

import (
	"maps"
	"slices"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

const (
	// restRoot is the prefix client-go puts in front of every REST call, and
	// apiRoot is what is left of an endpoint that is not under the v4 API,
	// GraphQL being the one this repository sends. Both are trimmed so a row
	// reads the way GitLab's own reference writes the endpoint.
	restRoot = "/api/v4"
	apiRoot  = "/api"

	// firstIdentifier and laterIdentifier are what an identifier segment is
	// replaced with. GitLab writes the resource a path is scoped to as :id
	// and the nested one as :issue_iid, :note_id and so on, and the shape
	// rule below cannot tell those apart, so every identifier after the
	// first is written :iid.
	firstIdentifier = ":id"
	laterIdentifier = ":iid"

	// encodedSlash is how a project or group addressed by path arrives, since
	// client-go escapes the separator: group%2Fproject is one segment.
	encodedSlash = "%2f"

	// shaMinLength and shaMaxLength bound a commit-ish segment. Seven is the
	// abbreviation GitLab itself prints and sixty-four is a full SHA-256.
	shaMinLength = 7
	shaMaxLength = 64

	// uuidLength is the length of the canonical 8-4-4-4-12 hyphenated form.
	uuidLength = 36
)

// templatePath rewrites one request path into the endpoint it stands for, so
// the inventory holds a row per endpoint rather than a row per fixture.
//
// The rule reads the shape of a segment and never a list of names. A list
// would have to be kept in step with a thousand actions and would be wrong the
// day one of them moved, while a shape stays true: a segment is an identifier
// when it is all digits, when it carries a percent-encoded slash, when it is
// hexadecimal and long enough to be a commit, or when it is shaped like a
// UUID.
//
// What the rule cannot catch is an identifier that looks like a word. A branch
// named main, a tag, a username, a label, a file with no directory in front of
// it: each stays in the path verbatim, so one endpoint reached with three
// branch names is three rows. That is a limit worth keeping visible rather
// than papering over, because the alternative, treating any trailing segment
// as an identifier, would erase the difference between the endpoint that lists
// branches and the one that reads a single branch.
//
// It also cannot tell one identifier kind from another. A numeric project id
// and a numeric issue iid are the same shape, and so are a commit SHA and a
// SHA-shaped blob id, which is why the placeholders are positional.
func templatePath(escaped string) string {
	path := strings.TrimPrefix(escaped, restRoot)
	if path == escaped {
		path = strings.TrimPrefix(escaped, apiRoot)
	}
	if path == "" {
		path = "/"
	}

	segments := strings.Split(path, "/")
	identifiers := 0
	for i, segment := range segments {
		if !isIdentifierSegment(segment) {
			continue
		}
		if identifiers == 0 {
			segments[i] = firstIdentifier
		} else {
			segments[i] = laterIdentifier
		}
		identifiers++
	}
	return strings.Join(segments, "/")
}

// isIdentifierSegment reports whether one path segment names a particular
// object rather than a fixed part of the endpoint. See [templatePath] for what
// the shapes are and what they miss.
func isIdentifierSegment(segment string) bool {
	switch {
	case isAllDigits(segment):
		return true
	case strings.Contains(strings.ToLower(segment), encodedSlash):
		return true
	case isSHAShaped(segment):
		return true
	default:
		return isUUIDShaped(segment)
	}
}

// isAllDigits reports whether every byte of segment is a decimal digit.
//
// The empty check is what keeps the empty element a split path begins with out
// of the identifier count: without it the loop would run zero times and call
// nothing an identifier, which is the wrong answer for the right reason.
func isAllDigits(segment string) bool {
	if segment == "" {
		return false
	}
	for i := range len(segment) {
		if segment[i] < '0' || segment[i] > '9' {
			return false
		}
	}
	return true
}

// isSHAShaped reports whether segment could be a commit, blob or tree object
// name: hexadecimal, long enough to be an abbreviation GitLab would print, and
// short enough to be a full SHA-256.
//
// It demands at least one digit so an ordinary word made of the letters a to f
// is not mistaken for an object name. That is a heuristic and it has a hole in
// both directions: a branch named "decade0" is read as an identifier, and the
// one abbreviated SHA in sixteen hundred that happens to be all letters is
// not.
func isSHAShaped(segment string) bool {
	if len(segment) < shaMinLength || len(segment) > shaMaxLength {
		return false
	}
	digits := 0
	for i := range len(segment) {
		switch c := segment[i]; {
		case c >= '0' && c <= '9':
			digits++
		case c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return digits > 0
}

// isUUIDShaped reports whether segment is the canonical 8-4-4-4-12 hyphenated
// form. GitLab hands one out for a runner's authentication token and for a
// few package identifiers, and none of them is anything but an identifier.
func isUUIDShaped(segment string) bool {
	if len(segment) != uuidLength {
		return false
	}
	for i := range len(segment) {
		c := segment[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

// graphQLShape is what one GraphQL request declares, which is the GraphQL
// half of what a REST path and its query keys say.
type graphQLShape struct {
	// Operation names the document by its type, its name when it has one, and
	// its root fields, because almost every document here is anonymous and the
	// root field is what tells two of them apart.
	Operation string
	// Variables are the names the document declares, sorted. The values are
	// deliberately absent: a value is a fixture, and a name is a contract.
	Variables []string
}

// summarizeGraphQL reads the shape of one document, reporting false when it
// does not parse.
//
// A document that does not parse is one of the tests that declare
// [AllowInvalidGraphQL] and send something malformed on purpose. Recording it
// would put a fixture's typo in the inventory, so it is left out; the schema
// gate is what judges documents, and this only describes them.
func summarizeGraphQL(document string) (graphQLShape, bool) {
	parsed, err := parser.ParseQuery(&ast.Source{Input: document})
	if err != nil || parsed == nil || len(parsed.Operations) == 0 {
		return graphQLShape{}, false
	}

	labels := make([]string, 0, len(parsed.Operations))
	variables := map[string]struct{}{}
	for _, operation := range parsed.Operations {
		labels = append(labels, operationLabel(operation))
		for _, variable := range operation.VariableDefinitions {
			variables[variable.Variable] = struct{}{}
		}
	}
	return graphQLShape{
		Operation: strings.Join(labels, " "),
		Variables: slices.Sorted(maps.Keys(variables)),
	}, true
}

// operationLabel renders one operation as its type, its name when it has one,
// and its root fields in the order the document declares them.
func operationLabel(operation *ast.OperationDefinition) string {
	label := string(operation.Operation)
	if operation.Name != "" {
		label += " " + operation.Name
	}
	if fields := rootFields(operation.SelectionSet); len(fields) > 0 {
		label += " " + strings.Join(fields, ",")
	}
	return label
}

// rootFields lists the top-level field names of a selection set.
//
// A fragment spread or an inline fragment at the root is skipped rather than
// followed: no document in this repository has one, and following it would
// mean resolving fragment definitions to name an operation nobody has to name
// that way.
func rootFields(selections ast.SelectionSet) []string {
	fields := make([]string, 0, len(selections))
	for _, selection := range selections {
		field, ok := selection.(*ast.Field)
		if !ok {
			continue
		}
		fields = append(fields, field.Name)
	}
	return fields
}
