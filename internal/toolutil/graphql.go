package toolutil

import (
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
)

// GraphQL pagination defaults.
const (
	GraphQLDefaultFirst = 20
	GraphQLMaxFirst     = 100
)

// GraphQLPaginationInput holds forward cursor pagination parameters for a
// GraphQL list query: first and after, and nothing else.
//
// Forward only is the safe default rather than a limitation of the helper. A
// connection paginates backwards only where GitLab's own field accepts before
// and last, and several fields this server queries refuse them: Project's
// branchRules and the work item notes widget's discussions both answer those
// arguments with argumentNotAccepted. What the two do with the backward half of
// pageInfo differs, which is why the promise is withdrawn in the output as well
// as in the input: branchRules reports no previous page and no start cursor,
// while the discussions widget is keyset-paginated and reports both from its
// second page on, offering a cursor no argument could ever spend. A domain
// whose field does accept the pair embeds [GraphQLCursorPaginationInput], which
// is the only type that can put it on the wire, and reports the whole of
// pageInfo through [GraphQLPaginationOutput]; a forward-only domain reports
// [GraphQLForwardPaginationOutput] instead.
type GraphQLPaginationInput struct {
	First *int   `json:"first,omitempty" jsonschema:"Number of items to return (default 20, max 100)"`
	After string `json:"after,omitempty" jsonschema:"Cursor for forward pagination (from previous response end_cursor)"`
}

// EffectiveFirst returns the requested page size, clamped to
// [1, GraphQLMaxFirst] with GraphQLDefaultFirst as fallback.
func (p GraphQLPaginationInput) EffectiveFirst() int {
	if p.First == nil {
		return GraphQLDefaultFirst
	}
	return clampGraphQLPageSize(*p.First)
}

// Variables returns the variable map for document, which must be the operation
// the caller is about to execute.
//
// The document is a parameter because a variable an operation does not declare
// is ignored rather than rejected: a page request carrying one is answered with
// somebody else's page and no error. Taking the document here is what makes
// that impossible, since the only way to obtain the map is to name the
// operation it is going to.
func (p GraphQLPaginationInput) Variables(document string) (map[string]any, error) {
	if err := requireGraphQLVariables(document, graphQLForwardVariables); err != nil {
		return nil, err
	}
	v := map[string]any{"first": p.EffectiveFirst()}
	if p.After != "" {
		v["after"] = p.After
	}
	return v, nil
}

// GraphQLCursorPaginationInput adds backward pagination to
// [GraphQLPaginationInput], for the connections GitLab lets a caller walk in
// both directions. Embed it only when the operation declares all four
// variables: [GraphQLCursorPaginationInput.Variables] refuses a document that
// does not.
type GraphQLCursorPaginationInput struct {
	GraphQLPaginationInput
	Last   *int   `json:"last,omitempty"   jsonschema:"Number of items to return from the end of the range (backward pagination). Cannot be combined with first"`
	Before string `json:"before,omitempty" jsonschema:"Cursor for backward pagination (from previous response start_cursor). The page size comes from last, or from first when last is omitted"`
}

// GraphQLCursor is one resolved cursor request. At most one of First and Last
// is set, which is what makes the direction the caller asked for the direction
// they get.
type GraphQLCursor struct {
	First  *int
	After  string
	Last   *int
	Before string
}

// Resolve turns the four requested parameters into the pair GitLab accepts.
//
// The cursor picks the direction and the count only sizes the page. A before
// cursor sent beside first is not backward pagination on any GitLab
// connection: the keyset ones answer it with the head of the list and the
// array-backed ones intersect first with last, so a caller following
// start_cursor back through a list would loop on the page they were already
// on. Sending both counts is refused rather than reinterpreted, because GitLab
// refuses it too and there is no reading of the pair that is not a guess.
func (p GraphQLCursorPaginationInput) Resolve() (GraphQLCursor, error) {
	if p.First != nil && p.Last != nil {
		return GraphQLCursor{}, errors.New(
			"first and last cannot be combined: GitLab answers \"Can only provide either `first` or `last`, not both\". " +
				"Page forward with first and after, backward with last and before",
		)
	}
	resolved := GraphQLCursor{After: p.After, Before: p.Before}
	if p.Before != "" || p.Last != nil {
		size := p.effectiveLast()
		resolved.Last = &size
		return resolved, nil
	}
	size := p.EffectiveFirst()
	resolved.First = &size
	return resolved, nil
}

// effectiveLast returns the backward page size, taking it from last, then from
// first, then from the default. A caller who names a size at all has named it
// once, since Resolve refuses the pair.
func (p GraphQLCursorPaginationInput) effectiveLast() int {
	switch {
	case p.Last != nil:
		return clampGraphQLPageSize(*p.Last)
	case p.First != nil:
		return clampGraphQLPageSize(*p.First)
	default:
		return GraphQLDefaultFirst
	}
}

// Variables returns the variable map for document, refusing a document that
// declares fewer variables than this input can send. See
// [GraphQLPaginationInput.Variables] for why the document is a parameter.
func (p GraphQLCursorPaginationInput) Variables(document string) (map[string]any, error) {
	if err := requireGraphQLVariables(document, graphQLCursorVariables); err != nil {
		return nil, err
	}
	resolved, err := p.Resolve()
	if err != nil {
		return nil, err
	}
	v := map[string]any{}
	if resolved.First != nil {
		v["first"] = *resolved.First
	}
	if resolved.Last != nil {
		v["last"] = *resolved.Last
	}
	if resolved.After != "" {
		v["after"] = resolved.After
	}
	if resolved.Before != "" {
		v["before"] = resolved.Before
	}
	return v, nil
}

// clampGraphQLPageSize bounds a requested page size to what GitLab accepts on
// a connection.
func clampGraphQLPageSize(n int) int {
	if n < 1 {
		return 1
	}
	if n > GraphQLMaxFirst {
		return GraphQLMaxFirst
	}
	return n
}

// graphQLVariable names one variable a pagination input can put on the wire.
type graphQLVariable struct {
	name string
	// omittable marks a variable the input sends on some requests and leaves
	// out of the map on others. Its declaration has to accept the absence,
	// which a non-null type without a default value does not: GitLab rejects
	// the whole operation, so declaring one turns the requests that omit the
	// variable into a validation error rather than a page.
	omittable bool
}

// graphQLForwardVariables and graphQLCursorVariables name every variable each
// pagination input can put on the wire. The forward input always sends a page
// size, which is why first alone is not omittable there.
var (
	graphQLForwardVariables = []graphQLVariable{
		{name: "first"},
		{name: "after", omittable: true},
	}
	graphQLCursorVariables = []graphQLVariable{
		{name: "first", omittable: true},
		{name: "after", omittable: true},
		{name: "last", omittable: true},
		{name: "before", omittable: true},
	}
)

// requireGraphQLVariables fails when document cannot carry every variable an
// input can send: one it does not declare, one it declares as a non-null type
// the input is allowed to omit, or one it declares and never passes on to a
// field.
//
// It checks the whole set the input could produce rather than the keys one
// call happens to carry, so the mismatch is reported by every request rather
// than by the rare one that trips over it. The eight domains this guard was
// written for had passed every test they have while discarding two parameters
// their schemas advertised, because no test ever paginated backwards.
//
// The last two checks exist because a declaration alone proves nothing. A
// document that declares $last and hands it to no connection is as silent as
// one that never declared it, and a document that declares $first as Int! is
// refused by GitLab on the backward requests that send last instead. Neither
// is visible to a domain test, whose mock answers whatever the document asks.
func requireGraphQLVariables(document string, variables []graphQLVariable) error {
	declared, body := graphQLDeclarations(document)
	var missing, mandatory, unused []string
	for _, variable := range variables {
		declaration, ok := declared[variable.name]
		if !ok {
			missing = append(missing, "$"+variable.name)
			continue
		}
		if variable.omittable && declaration.mustBeProvided() {
			mandatory = append(mandatory, "$"+variable.name)
		}
		if !referencesGraphQLVariable(body, variable.name) {
			unused = append(unused, "$"+variable.name)
		}
	}
	problems := make([]string, 0, 3)
	// Each problem carries its own reason and its own repair, because the
	// three have neither in common: one is a variable to add, one is a type to
	// relax, one is an argument to pass. A single trailing remedy told the
	// author of a document that declares $last and spends it nowhere to
	// declare it, which it already does.
	if len(missing) > 0 {
		problems = append(problems, "It does not declare "+strings.Join(missing, ", ")+
			". GitLab discards a variable an operation never declared rather than refusing it, "+
			"so the connection answers a page nobody asked for. Declare each on the operation.")
	}
	if len(mandatory) > 0 {
		problems = append(problems, "It declares "+strings.Join(mandatory, ", ")+
			" as non-null without a default. A variable declared that way cannot be left out, "+
			"and this input leaves out whichever half of the cursor the caller did not use. "+
			"Give each a nullable type, or a default value.")
	}
	if len(unused) > 0 {
		problems = append(problems, "It never passes "+strings.Join(unused, ", ")+
			" to a field. A variable that reaches no connection is as silent as one never declared. "+
			"Pass each to the connection field's argument of the same name.")
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf(
		"GraphQL operation cannot carry this pagination input. %s",
		strings.Join(problems, " "),
	)
}

// graphQLDeclaration is one variable definition: the type it was given and
// whether that definition supplies a default value.
type graphQLDeclaration struct {
	nonNull    bool
	hasDefault bool
}

// mustBeProvided reports whether a request that leaves this variable out is
// rejected. A non-null variable with a default is satisfied by the default.
func (d graphQLDeclaration) mustBeProvided() bool {
	return d.nonNull && !d.hasDefault
}

// graphQLDeclarations parses the operation's variable definitions and returns
// them by name alongside the rest of the document, which is where a reference
// to a variable has to appear for the variable to reach a field.
//
// Comments are removed before anything is scanned, in one place rather than in
// each scanner, because a guard that reads a comment as code answers about a
// document nobody sends: a note such as "# backward pagination would be $last"
// inside the definitions would otherwise count as declaring it, and the same
// note in the selection set would count as passing it to a field.
func graphQLDeclarations(document string) (declarations map[string]graphQLDeclaration, body string) {
	document = stripGraphQLComments(document)
	declarations = make(map[string]graphQLDeclaration)
	block, body, ok := graphQLVariableBlock(document)
	if !ok {
		return declarations, document
	}
	for i := 0; i < len(block); i++ {
		if block[i] != '$' {
			continue
		}
		end := i + 1
		for end < len(block) && isGraphQLNameByte(block[end]) {
			end++
		}
		if end > i+1 {
			name := block[i+1 : end]
			definition, next := graphQLDefinitionAfter(block, end)
			declarations[name] = definition
			end = next
		}
		i = end - 1
	}
	return declarations, body
}

// graphQLDefinitionAfter reads the type and optional default value that follow
// a variable's name in the definition block, starting at from, and returns the
// index the next definition may start at.
func graphQLDefinitionAfter(block string, from int) (declaration graphQLDeclaration, end int) {
	end = graphQLDefinitionEnd(block, from)
	_, typeText, ok := strings.Cut(block[from:end], ":")
	if !ok {
		return declaration, end
	}
	// A default value is separated from the type by an equals sign, and the
	// type itself never contains one, so the split needs no depth tracking:
	// only the default value can hold brackets or braces.
	if named, _, hasDefault := strings.Cut(typeText, "="); hasDefault {
		declaration.hasDefault = true
		typeText = named
	}
	declaration.nonNull = strings.HasSuffix(strings.TrimSpace(typeText), "!")
	return declaration, end
}

// graphQLDefinitionEnd returns the index at which the definition starting at
// from ends: the comma or the next variable that follows it, whichever comes
// first at the top level. Depth and string literals are both tracked, so
// neither a list type, an object default value nor a quoted default holding a
// bracket can end the definition early.
func graphQLDefinitionEnd(block string, from int) int {
	depth, inString := 0, false
	for i := from; i < len(block); i++ {
		c := block[i]
		if inString {
			switch c {
			case '\\':
				i++
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			depth--
		case ',', '$':
			if depth == 0 {
				return i
			}
		}
	}
	return len(block)
}

// stripGraphQLComments blanks every comment in document, keeping its length and
// its line breaks so that the scanners which follow see the same offsets they
// would have seen without them. A comment runs from an unquoted number sign to
// the end of the line, and a number sign inside a string literal is text.
func stripGraphQLComments(document string) string {
	out := []byte(document)
	inString, inComment := false, false
	for i := 0; i < len(out); i++ {
		c := out[i]
		switch {
		case inComment:
			if c == '\n' {
				inComment = false
				continue
			}
			out[i] = ' '
		case inString:
			switch c {
			case '\\':
				i++
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '#':
			inComment = true
			out[i] = ' '
		}
	}
	return string(out)
}

// referencesGraphQLVariable reports whether body mentions $name as a whole
// name, so that $first does not answer for $firstParent.
func referencesGraphQLVariable(body, name string) bool {
	needle := "$" + name
	for offset := 0; ; {
		index := strings.Index(body[offset:], needle)
		if index < 0 {
			return false
		}
		end := offset + index + len(needle)
		if end == len(body) || !isGraphQLNameByte(body[end]) {
			return true
		}
		offset = end
	}
}

// graphQLVariableBlock returns the text between the parentheses of the
// operation's variable definitions, and the whole of the document around them,
// which is where a reference to a variable has to appear for the variable to
// reach a field. A default value may itself be parenthesised or quoted, so the
// scan tracks depth and string literals rather than searching for the first
// closing parenthesis. It does not track comments, because
// [graphQLDeclarations] has already blanked them.
//
// The text before the definitions is kept because a fragment may precede the
// operation and spend one of its variables, and a fragment that does is the
// only place the reference appears.
func graphQLVariableBlock(document string) (block, rest string, ok bool) {
	from, found := graphQLOperationStart(document)
	if !found {
		return "", "", false
	}
	depth, start := 0, 0
	inString := false
	for i := from; i < len(document); i++ {
		c := document[i]
		switch {
		case inString:
			switch c {
			case '\\':
				i++
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '(':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				// The newline keeps the two halves from splicing a name
				// together across the seam the definitions left behind.
				return document[start:i], document[:start-1] + "\n" + document[i+1:], true
			}
			if depth < 0 {
				return "", "", false
			}
		case c == '{' && depth == 0:
			// The selection set began, so the operation declared no variables.
			return "", "", false
		}
	}
	return "", "", false
}

// graphQLOperationStart returns the index just past the query, mutation or
// subscription keyword that opens the document's operation definition.
//
// The scan is anchored to that keyword rather than started at the top of the
// document because a fragment definition may come first, and its selection set
// would otherwise be read as the operation's own: the caller would be told an
// operation declares nothing while it visibly declares four variables. No
// document in this repository leads with a fragment today, which is exactly why
// the guard has to be right about it in advance.
//
// A document that reaches its selection set without such a keyword is the
// shorthand form, which can declare no variables, and one that holds only
// fragments is not an operation at all; neither is found.
func graphQLOperationStart(document string) (index int, ok bool) {
	depth := 0
	inString := false
	for i := 0; i < len(document); i++ {
		c := document[i]
		switch {
		case inString:
			switch c {
			case '\\':
				i++
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '{':
			depth++
		case c == '}':
			depth--
		case depth == 0 && isGraphQLNameByte(c):
			end := i + 1
			for end < len(document) && isGraphQLNameByte(document[end]) {
				end++
			}
			switch document[i:end] {
			case "query", "mutation", "subscription":
				return end, true
			}
			i = end - 1
		}
	}
	return 0, false
}

// isGraphQLNameByte reports whether c may appear in a GraphQL name after its
// first character.
func isGraphQLNameByte(c byte) bool {
	return c == '_' ||
		(c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}

// GraphQLPageInfo holds cursor-based pagination metadata returned by
// GraphQL connection responses. It maps directly to GitLab's PageInfo type.
type GraphQLPageInfo struct {
	HasNextPage     bool   `json:"has_next_page"`
	HasPreviousPage bool   `json:"has_previous_page"`
	EndCursor       string `json:"end_cursor,omitempty"`
	StartCursor     string `json:"start_cursor,omitempty"`
}

// GraphQLPaginationOutput holds pagination metadata for GraphQL list
// tool responses, presented in a consistent format for LLM consumers.
type GraphQLPaginationOutput struct {
	HasNextPage     bool   `json:"has_next_page"`
	HasPreviousPage bool   `json:"has_previous_page"`
	EndCursor       string `json:"end_cursor,omitempty"`
	StartCursor     string `json:"start_cursor,omitempty"`
}

// GraphQLForwardPaginationOutput is the pagination metadata of a connection
// that only pages forward: whether another page follows, and the cursor that
// asks for it.
//
// It exists so that a tool taking [GraphQLPaginationInput] cannot report a
// previous page. GitLab's keyset connections fill in hasPreviousPage and
// startCursor from their second page on whether or not the field accepts
// before, so a forward-only tool reporting the whole of pageInfo hands a model
// a cursor and no parameter to spend it on, which reads as a capability the
// tool withdrew rather than as the dead end it is.
type GraphQLForwardPaginationOutput struct {
	HasNextPage bool   `json:"has_next_page"`
	EndCursor   string `json:"end_cursor,omitempty"`
}

// PageInfoToOutput converts a raw GraphQL PageInfo response struct
// (with camelCase JSON keys from the API) to the snake_case output struct.
func PageInfoToOutput(pi GraphQLRawPageInfo) GraphQLPaginationOutput {
	return GraphQLPaginationOutput(pi)
}

// PageInfoToForwardOutput converts a raw GraphQL PageInfo response struct to
// the forward-only output struct, dropping the backward half. A forward-only
// document need not select that half at all, in which case it is already zero.
func PageInfoToForwardOutput(pi GraphQLRawPageInfo) GraphQLForwardPaginationOutput {
	return GraphQLForwardPaginationOutput{HasNextPage: pi.HasNextPage, EndCursor: pi.EndCursor}
}

// GraphQLRawPageInfo matches the camelCase JSON shape returned by the
// GitLab GraphQL API before conversion to our snake_case output.
type GraphQLRawPageInfo struct {
	HasNextPage     bool   `json:"hasNextPage"`
	HasPreviousPage bool   `json:"hasPreviousPage"`
	EndCursor       string `json:"endCursor"`
	StartCursor     string `json:"startCursor"`
}

// GraphQLError is one top-level GraphQL error returned in a successful HTTP
// response body.
type GraphQLError struct {
	Message string `json:"message"`
}

// GraphQLTopLevelError formats top-level GraphQL response errors, if any.
func GraphQLTopLevelError(operation string, responseErrors []GraphQLError) error {
	if len(responseErrors) == 0 {
		return nil
	}
	messages := make([]string, 0, len(responseErrors))
	for _, graphQLError := range responseErrors {
		message := strings.TrimSpace(graphQLError.Message)
		if message != "" {
			messages = append(messages, message)
		}
	}
	if len(messages) == 0 {
		return fmt.Errorf("%s: %d GraphQL errors with empty messages", operation, len(responseErrors))
	}
	return fmt.Errorf("%s GraphQL errors: %s", operation, strings.Join(messages, "; "))
}

// GraphQLMutationError formats mutation payload errors, if any.
func GraphQLMutationError(operation string, payloadErrors []string) error {
	if len(payloadErrors) == 0 {
		return nil
	}
	messages := make([]string, 0, len(payloadErrors))
	for _, mutationError := range payloadErrors {
		message := strings.TrimSpace(mutationError)
		if message != "" {
			messages = append(messages, message)
		}
	}
	if len(messages) == 0 {
		return fmt.Errorf("%s mutation errors: %d errors with empty messages", operation, len(payloadErrors))
	}
	return fmt.Errorf("%s mutation errors: %s", operation, strings.Join(messages, "; "))
}

// FormatGraphQLPagination renders cursor-based pagination metadata as a
// Markdown summary line, suitable for appending to list tool responses.
func FormatGraphQLPagination(p GraphQLPaginationOutput, shown int) string {
	parts := []string{fmt.Sprintf("Showing %d items", shown)}
	if p.HasNextPage {
		parts = append(parts, fmt.Sprintf("next page cursor: `%s`", p.EndCursor))
	}
	if p.HasPreviousPage {
		parts = append(parts, fmt.Sprintf("prev page cursor: `%s`", p.StartCursor))
	}
	if !p.HasNextPage && !p.HasPreviousPage {
		parts = append(parts, "no more pages")
	}
	return strings.Join(parts, " | ")
}

// FormatGraphQLForwardPagination renders the pagination metadata of a
// forward-only connection, which never names a previous page.
func FormatGraphQLForwardPagination(p GraphQLForwardPaginationOutput, shown int) string {
	return FormatGraphQLPagination(
		GraphQLPaginationOutput{HasNextPage: p.HasNextPage, EndCursor: p.EndCursor},
		shown,
	)
}

// GID helpers for GitLab Global IDs (gid://gitlab/Type/123).

// FormatGID builds a GitLab Global ID string from a type name and numeric ID.
//
//	FormatGID("Vulnerability", 42) → "gid://gitlab/Vulnerability/42"
func FormatGID(typeName string, id int64) string {
	return fmt.Sprintf("gid://gitlab/%s/%d", typeName, id)
}

// ParseGID extracts the type name and numeric ID from a GitLab Global ID.
// It returns an error if the format is invalid.
//
//	ParseGID("gid://gitlab/Vulnerability/42") → ("Vulnerability", 42, nil)
func ParseGID(gid string) (typeName string, id int64, err error) {
	const prefix = "gid://gitlab/"
	if !strings.HasPrefix(gid, prefix) {
		return "", 0, fmt.Errorf("invalid GitLab GID: must start with %q, got %q", prefix, gid)
	}
	rest := strings.TrimPrefix(gid, prefix)
	slash := strings.LastIndex(rest, "/")
	if slash < 0 || slash == 0 || slash == len(rest)-1 {
		return "", 0, fmt.Errorf("invalid GitLab GID format: expected gid://gitlab/Type/ID, got %q", gid)
	}
	typeName = rest[:slash]
	id, err = strconv.ParseInt(rest[slash+1:], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid GitLab GID: ID %q is not a valid integer in %q", rest[slash+1:], gid)
	}
	return typeName, id, nil
}

// MergeVariables merges multiple variable maps into a single map.
// Later maps override earlier ones for duplicate keys.
func MergeVariables(sources ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, m := range sources {
		maps.Copy(result, m)
	}
	return result
}
