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

	// identifierSuffix is what a placeholder's name ends with, so a reader
	// meets :project_id and :issue_id rather than the positional :id and :iid
	// this used to write.
	identifierSuffix = "_id"

	// unnamedIdentifier is the placeholder for an identifier whose preceding
	// segment names no collection to take a name from, which is a nested file
	// name under /packages/npm/<name>/-/<file> and little else.
	unnamedIdentifier = ":id"

	// encodedSlash is how a project or group addressed by path arrives, since
	// client-go escapes the separator: group%2Fproject is one segment.
	// doubleEncodedSlash is the same separator escaped twice, which a test
	// fixture that pre-escapes its own path produces and which does not
	// contain the single-escaped form as a substring.
	encodedSlash       = "%2f"
	doubleEncodedSlash = "%252f"

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
// named main, a tag, a username, a label, a project addressed as my-project, a
// file with no directory in front of it: each stays in the path verbatim, so
// one endpoint reached with three branch names is three rows. That is a limit
// worth keeping visible rather than papering over, because the alternative,
// treating any trailing segment as an identifier, would erase the difference
// between the endpoint that lists branches and the one that reads a single
// branch.
//
// Taking the segment after a known parent instead, so that anything following
// projects or groups is the project or group, is the same trap one step along
// and it was measured rather than guessed: on the requests this suite records,
// projects is followed by the literals import, shared and user, groups by
// import and shared, packages by generic, npm and ml_models, snippets by all
// and public, runners by all, verify and reset_registration_token, and
// personal_access_tokens by self. Every one of those is a different endpoint
// from the one with an identifier there, and the rule would fold each into the
// other and then report a request we never make.
//
// It also cannot tell one identifier kind from another: a numeric project id
// and a numeric issue iid are the same shape, and so are a commit SHA and a
// SHA-shaped blob id. What names them is therefore the segment in front, which
// is the collection the identifier is a member of, so /projects/1/issues/2
// reads /projects/:project_id/issues/:issue_id and one placeholder cannot mean
// two things in one file. The placeholders used to be positional (:id for the
// first identifier the walk recognized, :iid for every later one), and that
// made :id name the project on one row and the board on the next, whenever the
// project was spelled as a fixture word the rule cannot recognize.
func templatePath(escaped string) string {
	path := strings.TrimPrefix(escaped, restRoot)
	if path == escaped {
		path = strings.TrimPrefix(escaped, apiRoot)
	}
	if path == "" {
		path = "/"
	}

	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if !isIdentifierSegment(segment) {
			continue
		}
		// Reading the previous element after it may itself have been
		// rewritten is deliberate: two identifiers in a row means the second
		// one's collection was never spelled, and a placeholder is not a
		// collection word, so it falls back to the unnamed form.
		parent := ""
		if i > 0 {
			parent = segments[i-1]
		}
		segments[i] = placeholderFor(parent)
	}
	return strings.Join(segments, "/")
}

// placeholderFor names the identifier that follows one collection segment.
func placeholderFor(parent string) string {
	name := singular(parent)
	if name == "" {
		return unnamedIdentifier
	}
	return ":" + name + identifierSuffix
}

// singular renders a collection segment as the one member an identifier under
// it stands for, and reports the empty string for a segment that is no
// collection word at all.
//
// English plurals cannot be inverted without a dictionary, and this does not
// carry one: releases and statuses are spelled the same way and singularize
// differently, so one of them comes out wrong whichever rule is chosen. That
// costs a placeholder an awkward name and nothing else, because the name is
// never compared with anything: the endpoint comparison in
// cmd/audit_1to1/internal/paths matches a documented placeholder against
// whatever we put in that position, and every placeholder here begins with a
// colon that no path segment can.
func singular(word string) string {
	if !isCollectionWord(word) {
		return ""
	}
	switch {
	case len(word) > len("ies") && strings.HasSuffix(word, "ies"):
		return word[:len(word)-len("ies")] + "y"
	case hasAnySuffix(word, "sses", "shes", "ches", "xes", "zes"):
		return word[:len(word)-len("es")]
	case strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss"):
		return word[:len(word)-1]
	default:
		return word
	}
}

// isCollectionWord reports whether a segment reads as the name of a collection:
// a lowercase word, possibly with digits and underscores in it. Anything else
// is a placeholder already written, an identifier the rule did recognize, or
// punctuation such as the "-" separator the package registry paths carry.
func isCollectionWord(segment string) bool {
	if segment == "" || segment[0] < 'a' || segment[0] > 'z' {
		return false
	}
	for i := range len(segment) {
		c := segment[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}

// hasAnySuffix reports whether word ends with any of the suffixes and is longer
// than the one it ends with, so trimming it always leaves something.
func hasAnySuffix(word string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if len(word) > len(suffix) && strings.HasSuffix(word, suffix) {
			return true
		}
	}
	return false
}

// isIdentifierSegment reports whether one path segment names a particular
// object rather than a fixed part of the endpoint. See [templatePath] for what
// the shapes are and what they miss.
func isIdentifierSegment(segment string) bool {
	switch {
	case isAllDigits(segment):
		return true
	case carriesEncodedSlash(segment):
		return true
	case isSHAShaped(segment):
		return true
	default:
		return isUUIDShaped(segment)
	}
}

// carriesEncodedSlash reports whether a segment is a path that was escaped to
// fit in one, at either depth this suite produces: a fixture that escapes its
// own project path before handing it to client-go, which escapes it again,
// arrives as %252f, and looking for the single-escaped form would miss it.
func carriesEncodedSlash(segment string) bool {
	lower := strings.ToLower(segment)
	return strings.Contains(lower, encodedSlash) || strings.Contains(lower, doubleEncodedSlash)
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
