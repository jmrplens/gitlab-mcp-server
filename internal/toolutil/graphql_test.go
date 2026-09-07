// graphql_test.go contains unit tests for GraphQL shared utilities:
// FormatGID and ParseGID round-tripping, the two pagination inputs
// (EffectiveFirst, Resolve, Variables and the declaration scan that keeps a
// document and the variables sent to it in step), PageInfoToOutput conversion,
// and FormatGraphQLPagination Markdown rendering.
package toolutil

import (
	"strings"
	"testing"
)

// TestFormatGID verifies that FormatGID produces correctly formatted
// GitLab Global IDs for a variety of type names and numeric IDs.
// The test uses table-driven subtests covering simple types, nested
// type paths (e.g. "WorkItems::Type"), and zero IDs, and asserts that
// the returned string matches the expected "gid://gitlab/{type}/{id}" format.
func TestFormatGID(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		id       int64
		want     string
	}{
		{"vulnerability", "Vulnerability", 42, "gid://gitlab/Vulnerability/42"},
		{"project", "Project", 1, "gid://gitlab/Project/1"},
		{"work item", "WorkItem", 999, "gid://gitlab/WorkItem/999"},
		{"nested type", "WorkItems::Type", 5, "gid://gitlab/WorkItems::Type/5"},
		{"zero id", "Issue", 0, "gid://gitlab/Issue/0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatGID(tt.typeName, tt.id)
			if got != tt.want {
				t.Errorf("FormatGID(%q, %d) = %q, want %q", tt.typeName, tt.id, got, tt.want)
			}
		})
	}
}

// TestParseGID_Valid verifies that ParseGID correctly extracts the type name
// and numeric ID from well-formed GitLab GID strings. Table-driven subtests
// cover simple types, large IDs, and nested type paths such as
// "WorkItems::Type". Each subtest asserts that the returned type name and ID
// match the expected values and that no error is returned.
func TestParseGID_Valid(t *testing.T) {
	tests := []struct {
		name     string
		gid      string
		wantType string
		wantID   int64
	}{
		{"simple", "gid://gitlab/Vulnerability/42", "Vulnerability", 42},
		{"large id", "gid://gitlab/Project/123456", "Project", 123456},
		{"nested type", "gid://gitlab/WorkItems::Type/5", "WorkItems::Type", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typeName, id, err := ParseGID(tt.gid)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if typeName != tt.wantType {
				t.Errorf("typeName = %q, want %q", typeName, tt.wantType)
			}
			if id != tt.wantID {
				t.Errorf("id = %d, want %d", id, tt.wantID)
			}
		})
	}
}

// TestParseGID_Invalid verifies that ParseGID returns a non-nil error for
// every malformed GID variant. Table-driven subtests cover: empty string,
// missing "gid://gitlab/" prefix, wrong namespace, missing ID segment,
// missing type segment, missing slash separator, and non-numeric ID.
// Each subtest asserts that the returned error is non-nil.
func TestParseGID_Invalid(t *testing.T) {
	tests := []struct {
		name string
		gid  string
	}{
		{"empty", ""},
		{"no prefix", "Vulnerability/42"},
		{"wrong prefix", "gid://github/Issue/1"},
		{"missing id", "gid://gitlab/Vulnerability/"},
		{"missing type", "gid://gitlab//42"},
		{"no slash separator", "gid://gitlab/Vulnerability"},
		{"non-numeric id", "gid://gitlab/Vulnerability/abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseGID(tt.gid)
			if err == nil {
				t.Fatalf("expected error for GID %q, got nil", tt.gid)
			}
		})
	}
}

// TestParseGID_Roundtrip verifies that FormatGID followed by ParseGID
// preserves both the type name and the numeric ID without loss. It asserts
// that no error is returned and that the recovered type name and ID are
// identical to the original inputs.
func TestParseGID_Roundtrip(t *testing.T) {
	typeName := "Vulnerability"
	id := int64(42)
	gid := FormatGID(typeName, id)
	gotType, gotID, err := ParseGID(gid)
	if err != nil {
		t.Fatalf("roundtrip error: %v", err)
	}
	if gotType != typeName {
		t.Errorf("roundtrip typeName = %q, want %q", gotType, typeName)
	}
	if gotID != id {
		t.Errorf("roundtrip id = %d, want %d", gotID, id)
	}
}

// TestGraphQLTopLevelError verifies top-level GraphQL errors are joined into an
// operation-specific error message.
//
// The test covers nil errors, multiple trimmed messages, and blank messages. It
// expects nil for no errors and descriptive fallback text when GitLab returns an
// errors array without useful messages.
func TestGraphQLTopLevelError(t *testing.T) {
	if err := GraphQLTopLevelError("securityAttributeCreate", nil); err != nil {
		t.Fatalf("GraphQLTopLevelError(nil) = %v, want nil", err)
	}

	err := GraphQLTopLevelError("securityAttributeCreate", []GraphQLError{{Message: "first"}, {Message: " second "}})
	if err == nil || !strings.Contains(err.Error(), "securityAttributeCreate GraphQL errors: first; second") {
		t.Fatalf("GraphQLTopLevelError() = %v, want joined messages", err)
	}

	err = GraphQLTopLevelError("securityAttributeCreate", []GraphQLError{{Message: " "}})
	if err == nil || !strings.Contains(err.Error(), "securityAttributeCreate: 1 GraphQL errors with empty messages") {
		t.Fatalf("GraphQLTopLevelError(blank) = %v, want generic error", err)
	}
}

// TestGraphQLMutationError verifies mutation-level GraphQL error arrays are
// joined into an operation-specific error message.
//
// The test covers nil mutation errors, multiple trimmed messages, and blank
// messages. This keeps GraphQL mutation handlers from dropping GitLab-provided
// diagnostics or returning empty error text.
func TestGraphQLMutationError(t *testing.T) {
	if err := GraphQLMutationError("securityAttributeCreate", nil); err != nil {
		t.Fatalf("GraphQLMutationError(nil) = %v, want nil", err)
	}

	err := GraphQLMutationError("securityAttributeCreate", []string{"first", " second "})
	if err == nil || !strings.Contains(err.Error(), "securityAttributeCreate mutation errors: first; second") {
		t.Fatalf("GraphQLMutationError() = %v, want joined messages", err)
	}

	err = GraphQLMutationError("securityAttributeCreate", []string{" "})
	if err == nil || !strings.Contains(err.Error(), "securityAttributeCreate mutation errors: 1 errors with empty messages") {
		t.Fatalf("GraphQLMutationError(blank) = %v, want generic error", err)
	}
}

// TestGraphQLPaginationInput_EffectiveFirst verifies that EffectiveFirst applies
// the correct default, minimum, and maximum bounds to the page size. Table-driven
// subtests cover: nil First (returns GraphQLDefaultFirst), explicit value within
// range, value exceeding GraphQLMaxFirst (clamped), zero (clamped to 1), and
// negative (clamped to 1).
func TestGraphQLPaginationInput_EffectiveFirst(t *testing.T) {
	intPtr := func(n int) *int { return new(n) }
	tests := []struct {
		name  string
		input GraphQLPaginationInput
		want  int
	}{
		{"nil defaults to 20", GraphQLPaginationInput{}, GraphQLDefaultFirst},
		{"explicit 50", GraphQLPaginationInput{First: intPtr(50)}, 50},
		{"clamped to max", GraphQLPaginationInput{First: intPtr(200)}, GraphQLMaxFirst},
		{"clamped to min", GraphQLPaginationInput{First: intPtr(0)}, 1},
		{"negative clamped", GraphQLPaginationInput{First: intPtr(-5)}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.EffectiveFirst()
			if got != tt.want {
				t.Errorf("EffectiveFirst() = %d, want %d", got, tt.want)
			}
		})
	}
}

// forwardDocument and cursorDocument are the two operation shapes the
// pagination inputs are allowed to run against. Each passes every pagination
// variable it declares to a connection field, which is what the guard demands
// of a real domain document.
const (
	forwardDocument = `query($path: ID!, $first: Int, $after: String) {
  group(fullPath: $path) { labels(first: $first, after: $after) { nodes { id } } }
}`
	cursorDocument = `query($path: ID!, $first: Int, $after: String, $last: Int, $before: String) {
  group(fullPath: $path) {
    labels(first: $first, after: $after, last: $last, before: $before) { nodes { id } }
  }
}`
)

// TestGraphQLPaginationInput_Variables verifies that the forward-only input
// sends first always and after only when the caller supplied one, and that it
// refuses an operation which does not declare both of them. The refusal is the
// point of the signature: a variable an operation omits is discarded by GitLab
// rather than rejected, so the page a caller asked for would arrive as somebody
// else's page with no error anywhere.
func TestGraphQLPaginationInput_Variables(t *testing.T) {
	intPtr := func(n int) *int { return new(n) }

	t.Run("defaults only", func(t *testing.T) {
		v, err := GraphQLPaginationInput{}.Variables(forwardDocument)
		if err != nil {
			t.Fatalf("Variables() error = %v, want nil", err)
		}
		if v["first"] != GraphQLDefaultFirst {
			t.Errorf("first = %v, want %d", v["first"], GraphQLDefaultFirst)
		}
		if _, ok := v["after"]; ok {
			t.Error("after should not be present")
		}
	})

	t.Run("with cursor", func(t *testing.T) {
		v, err := GraphQLPaginationInput{First: intPtr(10), After: "cursor123"}.Variables(forwardDocument)
		if err != nil {
			t.Fatalf("Variables() error = %v, want nil", err)
		}
		if v["first"] != 10 {
			t.Errorf("first = %v, want 10", v["first"])
		}
		if v["after"] != "cursor123" {
			t.Errorf("after = %v, want cursor123", v["after"])
		}
	})

	t.Run("never sends backward variables", func(t *testing.T) {
		v, err := GraphQLPaginationInput{First: intPtr(10), After: "cursor123"}.Variables(cursorDocument)
		if err != nil {
			t.Fatalf("Variables() error = %v, want nil", err)
		}
		assertVariableMap(t, v, map[string]any{"first": 10, "after": "cursor123"})
	})

	t.Run("undeclared after is refused", func(t *testing.T) {
		v, err := GraphQLPaginationInput{}.Variables(`query($first: Int) { group { id } }`)
		if err == nil {
			t.Fatalf("Variables() = %v, want an error naming $after", v)
		}
		if !strings.Contains(err.Error(), "$after") {
			t.Errorf("Variables() error = %v, want it to name $after", err)
		}
	})

	// A page size is on every forward request, so a document may demand it.
	t.Run("non-null first is accepted", func(t *testing.T) {
		document := `query($first: Int!, $after: String) {
  group { labels(first: $first, after: $after) { nodes { id } } }
}`
		if _, err := (GraphQLPaginationInput{}).Variables(document); err != nil {
			t.Errorf("Variables() error = %v, want nil", err)
		}
	})

	t.Run("non-null after is refused", func(t *testing.T) {
		document := `query($first: Int, $after: String!) {
  group { labels(first: $first, after: $after) { nodes { id } } }
}`
		v, err := GraphQLPaginationInput{}.Variables(document)
		if err == nil {
			t.Fatalf("Variables() = %v, want an error naming $after", v)
		}
		if !strings.Contains(err.Error(), "$after") || !strings.Contains(err.Error(), "non-null") {
			t.Errorf("Variables() error = %v, want it to name $after as non-null", err)
		}
	})
}

// TestGraphQLCursorPaginationInput_Resolve verifies the one rule that makes a
// backward request the page the caller asked for: the cursor picks the
// direction, the count only sizes the page, and exactly one of first and last
// reaches GitLab. The pair together is refused rather than reinterpreted,
// because GitLab's keyset connections refuse it too.
func TestGraphQLCursorPaginationInput_Resolve(t *testing.T) {
	intPtr := func(n int) *int { return new(n) }
	tests := []struct {
		name       string
		input      GraphQLCursorPaginationInput
		wantFirst  *int
		wantLast   *int
		wantAfter  string
		wantBefore string
		wantErr    bool
	}{
		{
			name:      "no parameters pages forward at the default size",
			input:     GraphQLCursorPaginationInput{},
			wantFirst: intPtr(GraphQLDefaultFirst),
		},
		{
			name:      "first and after page forward",
			input:     GraphQLCursorPaginationInput{First: intPtr(10), After: "end"},
			wantFirst: intPtr(10),
			wantAfter: "end",
		},
		{
			name:       "last and before page backward",
			input:      GraphQLCursorPaginationInput{Last: intPtr(5), Before: "start"},
			wantLast:   intPtr(5),
			wantBefore: "start",
		},
		{
			name:       "before alone pages backward at the default size",
			input:      GraphQLCursorPaginationInput{Before: "start"},
			wantLast:   intPtr(GraphQLDefaultFirst),
			wantBefore: "start",
		},
		{
			name:       "before sizes the backward page from first",
			input:      GraphQLCursorPaginationInput{First: intPtr(7), Before: "start"},
			wantLast:   intPtr(7),
			wantBefore: "start",
		},
		{
			name:     "last alone pages backward without a cursor",
			input:    GraphQLCursorPaginationInput{Last: intPtr(5)},
			wantLast: intPtr(5),
		},
		{
			name:      "forward size is clamped to the maximum",
			input:     GraphQLCursorPaginationInput{First: intPtr(500)},
			wantFirst: intPtr(GraphQLMaxFirst),
		},
		{
			name:     "backward size is clamped to the maximum",
			input:    GraphQLCursorPaginationInput{Last: intPtr(500)},
			wantLast: intPtr(GraphQLMaxFirst),
		},
		{
			name:     "backward size is clamped to the minimum",
			input:    GraphQLCursorPaginationInput{Last: intPtr(0)},
			wantLast: intPtr(1),
		},
		{
			name:       "backward size taken from first is clamped",
			input:      GraphQLCursorPaginationInput{First: intPtr(-3), Before: "start"},
			wantLast:   intPtr(1),
			wantBefore: "start",
		},
		{
			name:    "first and last together are refused",
			input:   GraphQLCursorPaginationInput{First: intPtr(10), Last: intPtr(5)},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.Resolve()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Resolve() = %+v, want an error", got)
				}
				if !strings.Contains(err.Error(), "first and last cannot be combined") {
					t.Errorf("Resolve() error = %v, want it to name the conflict", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v, want nil", err)
			}
			assertPageSize(t, "First", got.First, tt.wantFirst)
			assertPageSize(t, "Last", got.Last, tt.wantLast)
			if got.After != tt.wantAfter {
				t.Errorf("Resolve().After = %q, want %q", got.After, tt.wantAfter)
			}
			if got.Before != tt.wantBefore {
				t.Errorf("Resolve().Before = %q, want %q", got.Before, tt.wantBefore)
			}
		})
	}
}

// assertPageSize compares one resolved page size against what the case wants,
// reporting a nil mismatch in either direction.
func assertPageSize(t *testing.T, field string, got, want *int) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil:
		t.Errorf("Resolve().%s = nil, want %d", field, *want)
	case want == nil:
		t.Errorf("Resolve().%s = %d, want it unset so the other direction is the one GitLab sees", field, *got)
	case *got != *want:
		t.Errorf("Resolve().%s = %d, want %d", field, *got, *want)
	}
}

// TestGraphQLCursorPaginationInput_Variables verifies that the bidirectional
// input puts exactly one page size on the wire, and that it refuses a document
// which does not declare every variable it can send. That refusal is the guard
// this type exists for: eight domains published last and before for months
// while their operations declared neither, and every test they had passed,
// because none of them ever paginated backwards.
func TestGraphQLCursorPaginationInput_Variables(t *testing.T) {
	intPtr := func(n int) *int { return new(n) }
	tests := []struct {
		name         string
		input        GraphQLCursorPaginationInput
		document     string
		want         map[string]any
		wantErrParts []string
	}{
		{
			name:     "backward request omits first",
			input:    GraphQLCursorPaginationInput{Last: intPtr(5), Before: "start"},
			document: cursorDocument,
			want:     map[string]any{"last": 5, "before": "start"},
		},
		{
			name:     "forward request omits last",
			input:    GraphQLCursorPaginationInput{First: intPtr(10), After: "end"},
			document: cursorDocument,
			want:     map[string]any{"first": 10, "after": "end"},
		},
		{
			name:     "no parameters page forward at the default size",
			input:    GraphQLCursorPaginationInput{},
			document: cursorDocument,
			want:     map[string]any{"first": GraphQLDefaultFirst},
		},
		{
			name:         "forward-only document is refused",
			input:        GraphQLCursorPaginationInput{},
			document:     forwardDocument,
			wantErrParts: []string{"$last", "$before"},
		},
		{
			name:         "contradictory counts are refused",
			input:        GraphQLCursorPaginationInput{First: intPtr(10), Last: intPtr(5)},
			document:     cursorDocument,
			wantErrParts: []string{"first and last cannot be combined"},
		},
		{
			name:  "non-null declaration of an omitted variable is refused",
			input: GraphQLCursorPaginationInput{},
			document: `query($first: Int!, $after: String, $last: Int, $before: String) {
  group { labels(first: $first, after: $after, last: $last, before: $before) { nodes { id } } }
}`,
			wantErrParts: []string{"$first", "non-null"},
		},
		{
			name:  "non-null declaration with a default is accepted",
			input: GraphQLCursorPaginationInput{Last: intPtr(5), Before: "start"},
			document: `query($first: Int! = 20, $after: String, $last: Int, $before: String) {
  group { labels(first: $first, after: $after, last: $last, before: $before) { nodes { id } } }
}`,
			want: map[string]any{"last": 5, "before": "start"},
		},
		{
			name:  "declared but never passed to a field is refused",
			input: GraphQLCursorPaginationInput{},
			document: `query($first: Int, $after: String, $last: Int, $before: String) {
  group { labels(first: $first, after: $after) { nodes { id } } }
}`,
			wantErrParts: []string{"never passes", "$last", "$before"},
		},
		{
			name:  "a longer variable name does not stand in for a shorter one",
			input: GraphQLCursorPaginationInput{},
			document: `query($first: Int, $after: String, $last: Int, $before: String, $firstOnly: Boolean) {
  group(onlyFirst: $firstOnly) {
    labels(first: $first, after: $after, last: $last, before: $before) { nodes { id } }
  }
}`,
			want: map[string]any{"first": GraphQLDefaultFirst},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.Variables(tt.document)
			if len(tt.wantErrParts) > 0 {
				assertErrorNames(t, err, got, tt.wantErrParts)
				return
			}
			if err != nil {
				t.Fatalf("Variables() error = %v, want nil", err)
			}
			assertVariableMap(t, got, tt.want)
		})
	}
}

// assertErrorNames checks that a refused request produced an error naming every
// part the case expects.
func assertErrorNames(t *testing.T, err error, got map[string]any, parts []string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Variables() = %v, want an error", got)
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("Variables() error = %v, want it to name %s", err, part)
		}
	}
}

// assertVariableMap compares a variable map against the exact set the case
// expects. An extra key is reported as loudly as a wrong value, because the
// defect this guards against is a page request carrying one variable too many.
func assertVariableMap(t *testing.T, got, want map[string]any) {
	t.Helper()
	for key, value := range want {
		if got[key] != value {
			t.Errorf("Variables()[%q] = %v, want %v", key, got[key], value)
		}
	}
	for key, value := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("Variables()[%q] = %v, want it unset for a request in the other direction", key, value)
		}
	}
}

// TestGraphQLDeclarations verifies that the declaration scan reads the
// operation signature and nothing else. It covers the shapes a hand-written
// document in this repository takes, plus the ones that would fool a scan
// looking only for the first closing parenthesis: a parenthesised default
// value, a quoted one containing a parenthesis, and a comment. A leading
// fragment is covered because its selection set would otherwise be read as the
// operation's own, which reports an operation as declaring nothing while it
// visibly declares two variables.
func TestGraphQLDeclarations(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     []string
	}{
		{
			name:     "anonymous operation",
			document: `query($first: Int, $after: String) { group { id } }`,
			want:     []string{"first", "after"},
		},
		{
			name:     "named operation",
			document: `query ListEmoji($groupPath: ID!, $first: Int) { group(fullPath: $groupPath) { id } }`,
			want:     []string{"groupPath", "first"},
		},
		{
			name:     "no variables",
			document: `query { currentUser { id } }`,
			want:     nil,
		},
		{
			name:     "shorthand selection set",
			document: `{ currentUser { id } }`,
			want:     nil,
		},
		{
			name:     "mutation",
			document: `mutation($input: CreateInput!) { create(input: $input) { id } }`,
			want:     []string{"input"},
		},
		{
			name:     "parenthesised default value",
			document: `query($filter: Filter = {names: ["a"]}, $first: Int = 20) { group(filter: $filter) { id } }`,
			want:     []string{"filter", "first"},
		},
		{
			name:     "quoted default containing a parenthesis",
			document: `query($search: String = "a)b", $first: Int) { group(search: $search) { id } }`,
			want:     []string{"search", "first"},
		},
		{
			name:     "quoted default containing an escaped quote",
			document: `query($search: String = "a\")b", $first: Int) { group(search: $search) { id } }`,
			want:     []string{"search", "first"},
		},
		{
			name:     "comment before the signature",
			document: "# lists (everything)\nquery($first: Int) { group { id } }",
			want:     []string{"first"},
		},
		{
			name:     "unbalanced parentheses declare nothing",
			document: `query($first: Int { group { id } }`,
			want:     nil,
		},
		{
			name:     "stray closing parenthesis declares nothing",
			document: `query) { group { id } }`,
			want:     nil,
		},
		{
			name:     "bare dollar sign is not a declaration",
			document: `query($ , $first: Int) { group { id } }`,
			want:     []string{"first"},
		},
		{
			name:     "variable with no type is still a declaration",
			document: `query($first, $after: String) { group(after: $after) { id } }`,
			want:     []string{"first", "after"},
		},
		{
			name: "fragment before the operation",
			document: `fragment Severity on Vulnerability { id severity }
query($first: Int, $after: String) {
  project { vulnerabilities(first: $first, after: $after) { nodes { ...Severity } } }
}`,
			want: []string{"first", "after"},
		},
		{
			name: "fragment holding a brace-laden default and a comment",
			document: `# a fragment first (deliberately)
fragment Names on Group { name(format: "a{b}") }
mutation($input: CreateInput! = {names: ["a"]}) { create(input: $input) { id } }`,
			want: []string{"input"},
		},
		{
			name:     "fragments alone declare nothing",
			document: `fragment Severity on Vulnerability { id severity }`,
			want:     nil,
		},
		{
			name:     "comment between the keyword and the signature",
			document: "query # which page (of many)\n($first: Int, $after: String) { group(after: $after) { id } }",
			want:     []string{"first", "after"},
		},
		{
			name: "fragment holding an escaped quote before the operation",
			document: `fragment Names on Group { name(format: "a\")b") }
query($first: Int, $after: String) { group(after: $after) { ...Names } }`,
			want: []string{"first", "after"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := graphQLDeclarations(tt.document)
			if len(got) != len(tt.want) {
				t.Fatalf("graphQLDeclarations() = %v, want %v", got, tt.want)
			}
			for _, name := range tt.want {
				if _, ok := got[name]; !ok {
					t.Errorf("graphQLDeclarations() = %v, want it to contain %q", got, name)
				}
			}
		})
	}
}

// TestRequireGraphQLVariables_FragmentReference verifies that a variable the
// operation declares and only a fragment spends counts as passed to a field.
//
// The body the reference is looked for in is the whole document around the
// definitions rather than the text after them, because a fragment placed
// before the operation is the only place such a reference appears.
func TestRequireGraphQLVariables_FragmentReference(t *testing.T) {
	document := `fragment Page on LabelConnection { nodes { id } }
query($first: Int, $after: String, $last: Int, $before: String) {
  group {
    labels(first: $first, after: $after, last: $last, before: $before) { ...Page }
  }
}`
	if _, err := (GraphQLCursorPaginationInput{}).Variables(document); err != nil {
		t.Errorf("Variables() error = %v, want nil for a fragment-prefixed document", err)
	}
}

// TestRequireGraphQLVariables_CommentsAreNotCode verifies that a variable named
// only in a comment counts neither as declared nor as passed to a field.
//
// A note about pagination somebody has not written yet is the likeliest comment
// to appear in one of these documents, and reading it as code would have the
// guard approve exactly the shape it exists to refuse. The forward set stays
// satisfied by the same document, which is what proves the comment was removed
// rather than the whole definition block discarded.
func TestRequireGraphQLVariables_CommentsAreNotCode(t *testing.T) {
	document := `query($projectPath: ID!, $first: Int, $after: String
  # backward pagination would be $last: Int, $before: String
) {
  project(fullPath: $projectPath) {
    # someday pass $last and $before to this connection
    labels(first: $first, after: $after) { nodes { id } }
  }
}`
	if _, err := (GraphQLCursorPaginationInput{}).Variables(document); err == nil {
		t.Error("Variables() error = nil, want a refusal for variables named only in comments")
	}
	if _, err := (GraphQLPaginationInput{}).Variables(document); err != nil {
		t.Errorf("Variables() error = %v, want nil for the forward set the document really declares", err)
	}
}

// TestStripGraphQLComments verifies that a comment is blanked without moving
// anything after it, and that a number sign inside a string literal is text.
func TestStripGraphQLComments(t *testing.T) {
	for _, tc := range []struct {
		name     string
		document string
		want     string
	}{
		{name: "comment to end of line", document: "a # b\nc", want: "a    \nc"},
		{name: "number sign in a string", document: `x(s: "a # b") y`, want: `x(s: "a # b") y`},
		{name: "escaped quote before a comment", document: `x(s: "a\"b") # c`, want: `x(s: "a\"b")    `},
		{name: "no comment", document: "query($a: Int) { b }", want: "query($a: Int) { b }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := stripGraphQLComments(tc.document)
			if got != tc.want {
				t.Errorf("stripGraphQLComments() = %q, want %q", got, tc.want)
			}
			if len(got) != len(tc.document) {
				t.Errorf("length changed: got %d, want %d", len(got), len(tc.document))
			}
		})
	}
}

// TestPageInfoToOutput verifies that PageInfoToOutput maps all fields from a
// GraphQLRawPageInfo to a GraphQLPaginationOutput correctly. It asserts that
// HasNextPage, HasPreviousPage, EndCursor, and StartCursor are transferred
// without modification.
func TestPageInfoToOutput(t *testing.T) {
	raw := GraphQLRawPageInfo{
		HasNextPage:     true,
		HasPreviousPage: false,
		EndCursor:       "abc123",
		StartCursor:     "xyz789",
	}
	out := PageInfoToOutput(raw)
	if !out.HasNextPage {
		t.Error("HasNextPage should be true")
	}
	if out.HasPreviousPage {
		t.Error("HasPreviousPage should be false")
	}
	if out.EndCursor != "abc123" {
		t.Errorf("EndCursor = %q, want %q", out.EndCursor, "abc123")
	}
	if out.StartCursor != "xyz789" {
		t.Errorf("StartCursor = %q, want %q", out.StartCursor, "xyz789")
	}
}

// TestFormatGraphQLPagination verifies that FormatGraphQLPagination produces
// correct Markdown pagination summaries. Table-driven subtests cover: has-next
// page (EndCursor shown), has-previous page (StartCursor shown), last page
// (no cursors), and empty pagination. Each subtest asserts that the output
// contains the expected substring.
func TestFormatGraphQLPagination(t *testing.T) {
	tests := []struct {
		name    string
		p       GraphQLPaginationOutput
		shown   int
		wantSub string
	}{
		{
			"has next page",
			GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cur1"},
			10,
			"next page cursor: `cur1`",
		},
		{
			"has previous page",
			GraphQLPaginationOutput{HasPreviousPage: true, StartCursor: "cur0"},
			5,
			"prev page cursor: `cur0`",
		},
		{
			"no more pages",
			GraphQLPaginationOutput{},
			3,
			"no more pages",
		},
		{
			"both directions",
			GraphQLPaginationOutput{HasNextPage: true, HasPreviousPage: true, EndCursor: "e", StartCursor: "s"},
			20,
			"next page cursor: `e`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatGraphQLPagination(tt.p, tt.shown)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("FormatGraphQLPagination() = %q, want substring %q", got, tt.wantSub)
			}
		})
	}
}

// TestPageInfoToForwardOutput verifies that the forward-only conversion keeps
// the next-page half and drops the previous-page half, which is the whole
// reason the type exists: GitLab's keyset connections report a previous page
// even where the field refuses before and last, and a tool with no before
// parameter must not pass that cursor on.
func TestPageInfoToForwardOutput(t *testing.T) {
	got := PageInfoToForwardOutput(GraphQLRawPageInfo{
		HasNextPage:     true,
		HasPreviousPage: true,
		EndCursor:       "end",
		StartCursor:     "start",
	})
	want := GraphQLForwardPaginationOutput{HasNextPage: true, EndCursor: "end"}
	if got != want {
		t.Errorf("PageInfoToForwardOutput() = %+v, want %+v", got, want)
	}
}

// TestFormatGraphQLForwardPagination verifies that the forward-only summary
// line names the next page when there is one and says there are no more pages
// otherwise, never a previous page.
func TestFormatGraphQLForwardPagination(t *testing.T) {
	tests := []struct {
		name    string
		p       GraphQLForwardPaginationOutput
		shown   int
		wantSub string
	}{
		{"has next page", GraphQLForwardPaginationOutput{HasNextPage: true, EndCursor: "cur1"}, 10, "next page cursor: `cur1`"},
		{"no more pages", GraphQLForwardPaginationOutput{}, 3, "no more pages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatGraphQLForwardPagination(tt.p, tt.shown)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("FormatGraphQLForwardPagination() = %q, want substring %q", got, tt.wantSub)
			}
			if strings.Contains(got, "prev page cursor") {
				t.Errorf("FormatGraphQLForwardPagination() = %q, want no previous page", got)
			}
		})
	}
}

// TestMergeVariables verifies map merging with override behavior.
func TestMergeVariables(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := MergeVariables()
		if len(result) != 0 {
			t.Errorf("expected empty map, got %v", result)
		}
	})

	t.Run("single map", func(t *testing.T) {
		result := MergeVariables(map[string]any{"a": 1})
		if result["a"] != 1 {
			t.Errorf("a = %v, want 1", result["a"])
		}
	})

	t.Run("override", func(t *testing.T) {
		result := MergeVariables(
			map[string]any{"a": 1, "b": 2},
			map[string]any{"b": 3, "c": 4},
		)
		if result["a"] != 1 {
			t.Errorf("a = %v, want 1", result["a"])
		}
		if result["b"] != 3 {
			t.Errorf("b = %v, want 3 (override)", result["b"])
		}
		if result["c"] != 4 {
			t.Errorf("c = %v, want 4", result["c"])
		}
	})
}
