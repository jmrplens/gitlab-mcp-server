package graphqlschema

import (
	"errors"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
)

// stubSchemaLoad replaces the loader [Validate] and [ValidateDocument] go
// through, restoring it when the test ends. It is what makes the
// schema-unavailable path reachable, since the embedded schema always loads.
func stubSchemaLoad(t *testing.T, err error) {
	t.Helper()
	original := schemaFor
	schemaFor = func() (*ast.Schema, error) { return nil, err }
	t.Cleanup(func() { schemaFor = original })
}

// TestSchema_EmbeddedSDL_LoadsTheWholeGitLabSchema verifies that the committed
// artifact parses and carries the types the documents in this repository name.
// A schema that loads but is missing Vulnerability would accept every broken
// document silently, so the check is for content and not only for absence of
// error.
func TestSchema_EmbeddedSDL_LoadsTheWholeGitLabSchema(t *testing.T) {
	schema, err := Schema()
	if err != nil {
		t.Fatalf("Schema() error = %v, want nil", err)
	}
	if schema.Query == nil || schema.Mutation == nil {
		t.Fatalf("Schema() has query=%v mutation=%v, want both", schema.Query != nil, schema.Mutation != nil)
	}
	for _, name := range []string{"Vulnerability", "PipelineSecurityReportFinding", "VulnerabilitySeverity", "RelativePositionType"} {
		t.Run(name, func(t *testing.T) {
			if schema.Types[name] == nil {
				t.Errorf("the pinned schema has no type %q", name)
			}
		})
	}
}

// TestSchema_CalledTwice_ReturnsTheSameSchema verifies the sync.Once: parsing
// costs around 200 ms, so a second caller must get the first caller's schema
// rather than pay for its own.
func TestSchema_CalledTwice_ReturnsTheSameSchema(t *testing.T) {
	first, err := Schema()
	if err != nil {
		t.Fatalf("Schema() error = %v", err)
	}
	second, err := Schema()
	if err != nil {
		t.Fatalf("Schema() second call error = %v", err)
	}
	if first != second {
		t.Error("Schema() parsed the SDL twice; the sync.Once is not holding")
	}
}

// TestLoad_MalformedInput_ReportsWhichStageFailed verifies that an artifact
// which is not SDL is reported as such rather than yielding a schema that
// accepts everything, and that the message names the stage: a reader fixing a
// corrupt pin needs to know it is the SDL and not the file.
func TestLoad_MalformedInput_ReportsWhichStageFailed(t *testing.T) {
	cases := []struct {
		name string
		sdl  []byte
		want string
	}{
		{name: "prose rather than a schema", sdl: []byte("this is prose, not a schema"), want: "parse the schema"},
		{name: "a truncated type", sdl: []byte("type Query {\n  ok: Boo"), want: "parse the schema"},
		{name: "still compressed", sdl: []byte{0x1f, 0x8b, 0x08, 0x00}, want: "parse the schema"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			schema, err := Load(testCase.sdl)
			if err == nil {
				t.Fatalf("Load() error = nil, want one naming %q", testCase.want)
			}
			if schema != nil {
				t.Errorf("Load() schema = %v, want nil on failure", schema)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Load() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestLoad_ValidSDL_ParsesIt verifies the success path of the exported loader,
// which is what cmd/gen_graphql_schema calls to check an artifact on disk.
func TestLoad_ValidSDL_ParsesIt(t *testing.T) {
	schema, err := Load([]byte("type Query {\n  ok: Boolean\n}\n"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if schema.Types["Query"] == nil {
		t.Error("Load() lost the Query type")
	}
}

// TestValidateDocumentAgainst_ASchemaTheCallerLoaded_JudgesByThatSchema
// verifies the entry the live re-probe uses. The point of it is that the
// verdict follows the schema handed in and not the pin, so the case that
// matters is a document the pin accepts and a narrower schema does not.
func TestValidateDocumentAgainst_ASchemaTheCallerLoaded_JudgesByThatSchema(t *testing.T) {
	narrowed, err := Load([]byte("type Query {\n  ok: Boolean\n}\n"))
	if err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	const document = `query($id: VulnerabilityID!) { vulnerability(id: $id) { id } }`

	if pinnedErr := ValidateDocument(document); pinnedErr != nil {
		t.Fatalf("the pinned schema refuses the fixture, so the comparison says nothing: %v", pinnedErr)
	}

	err = ValidateDocumentAgainst(narrowed, document)

	if err == nil {
		t.Fatal("ValidateDocumentAgainst() error = nil, want the narrower schema's refusal")
	}
	if !strings.Contains(err.Error(), "vulnerability") {
		t.Errorf("ValidateDocumentAgainst() error = %q, want it to name the field the schema lacks", err)
	}
}

// TestValidateDocument_AgainstThePinnedSchema_AcceptsRealDocumentsAndRefusesBrokenOnes
// verifies the half a static audit can ask: a document alone, with no request
// behind it. The broken cases are the real ones this gate was built for.
func TestValidateDocument_AgainstThePinnedSchema_AcceptsRealDocumentsAndRefusesBrokenOnes(t *testing.T) {
	cases := []struct {
		name     string
		document string
		want     string
	}{
		{
			name:     "a query GitLab accepts",
			document: `query($id: VulnerabilityID!) { vulnerability(id: $id) { id title solution } }`,
		},
		{
			name:     "a field the type does not have",
			document: `query($id: VulnerabilityID!) { vulnerability(id: $id) { hasSolutions } }`,
			want:     `Cannot query field "hasSolutions"`,
		},
		{
			name:     "an argument the field does not accept",
			document: `query($path: ID!) { project(fullPath: $path) { branchRules(last: 5) { nodes { name } } } }`,
			want:     `Unknown argument "last"`,
		},
		{
			name:     "a variable typed as something the argument is not",
			document: `query($path: ID!, $severity: [String!]) { project(fullPath: $path) { vulnerabilities(severity: $severity) { nodes { id } } } }`,
			want:     `used in position expecting type "[VulnerabilitySeverity!]"`,
		},
		{
			name:     "a document assembled behind a fragment",
			document: "fragment Bits on Vulnerability { id title }\nquery($id: VulnerabilityID!) { vulnerability(id: $id) { ...Bits } }",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateDocument(testCase.document)
			if testCase.want == "" {
				if err != nil {
					t.Fatalf("ValidateDocument() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateDocument() error = nil, want one naming %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("ValidateDocument() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestValidateDocument_SchemaUnavailable_ReportsTheLoadFailure verifies that a
// pinned artifact this process could not load is reported as such, rather than
// being silently treated as a schema that accepts everything.
func TestValidateDocument_SchemaUnavailable_ReportsTheLoadFailure(t *testing.T) {
	stubSchemaLoad(t, errors.New("the pin is corrupt"))

	err := ValidateDocument("query { currentUser { id } }")

	if err == nil || !strings.Contains(err.Error(), "the pin is corrupt") {
		t.Errorf("ValidateDocument() error = %v, want the load failure", err)
	}
}

// TestValidate_SchemaUnavailable_ReportsTheLoadFailure is the same guarantee
// for the full check, which has its own entry into the loader.
func TestValidate_SchemaUnavailable_ReportsTheLoadFailure(t *testing.T) {
	stubSchemaLoad(t, errors.New("the pin is corrupt"))

	err := Validate("query { currentUser { id } }", nil)

	if err == nil || !strings.Contains(err.Error(), "the pin is corrupt") {
		t.Errorf("Validate() error = %v, want the load failure", err)
	}
}

// TestValidate_DocumentAndVariablesTogether_CatchesBothHalves verifies the
// check the test transport performs. The undeclared-variable case is the
// defect that let eight domains advertise a backward pagination no operation
// declared, and it is invisible to the document half.
func TestValidate_DocumentAndVariablesTogether_CatchesBothHalves(t *testing.T) {
	const listBranchRules = `query($path: ID!, $first: Int, $after: String) {
  project(fullPath: $path) { branchRules(first: $first, after: $after) { nodes { name } } }
}`
	const listVulnerabilities = `query($path: ID!, $severity: [VulnerabilitySeverity!]) {
  project(fullPath: $path) { vulnerabilities(severity: $severity) { nodes { id } } }
}`

	cases := []struct {
		name      string
		document  string
		variables map[string]any
		want      string
	}{
		{
			name:      "everything declared and well typed",
			document:  listBranchRules,
			variables: map[string]any{"path": "group/project", "first": float64(20)},
		},
		{
			name:      "a document the schema refuses stops before the variables",
			document:  `query($id: VulnerabilityID!) { vulnerability(id: $id) { hasSolutions } }`,
			variables: map[string]any{"id": "gid://gitlab/Vulnerability/1"},
			want:      `Cannot query field "hasSolutions"`,
		},
		{
			name:      "a variable the operation never declared",
			document:  listBranchRules,
			variables: map[string]any{"path": "group/project", "last": float64(20), "before": "cursor"},
			want:      "variable $before is sent but the operation does not declare it",
		},
		{
			name:      "a value that does not fit its declared type",
			document:  listBranchRules,
			variables: map[string]any{"path": "group/project", "first": "twenty"},
			want:      "cannot use string as Int",
		},
		{
			name:      "an enum value the schema does not have",
			document:  listVulnerabilities,
			variables: map[string]any{"path": "group/project", "severity": []any{"SEVERE"}},
			want:      "SEVERE is not a valid VulnerabilitySeverity",
		},
		{
			name:      "a required variable left out",
			document:  listBranchRules,
			variables: map[string]any{"first": float64(20)},
			want:      "must be defined",
		},
		{
			name:      "a document whose selection sits behind a fragment",
			document:  "fragment Bits on Vulnerability { id title }\nquery($id: VulnerabilityID!) { vulnerability(id: $id) { ...Bits } }",
			variables: map[string]any{"id": "gid://gitlab/Vulnerability/1"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := Validate(testCase.document, testCase.variables)
			if testCase.want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() error = nil, want one naming %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Validate() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestValidateVariables_DocumentBindingNothing_AddsNoFailure verifies the
// branch where a document defines no operation. gqlparser refuses an unused
// fragment before this point, so the branch is reached directly: it exists
// because a document with nothing to bind must not have a variable failure
// invented for it on top of the refusal it already earned.
func TestValidateVariables_DocumentBindingNothing_AddsNoFailure(t *testing.T) {
	schema, err := Schema()
	if err != nil {
		t.Fatalf("Schema() error = %v", err)
	}

	if varErr := validateVariables(schema, &ast.QueryDocument{}, map[string]any{"anything": 1}); varErr != nil {
		t.Errorf("validateVariables() error = %v, want nil for a document that binds nothing", varErr)
	}
}

// TestValidate_SeveralOperations_ReportsThatNoneIsNamed verifies the case
// client-go cannot express: it sends {query, variables} and no operationName,
// so a document defining two operations names nothing GitLab can select
// however well each one validates alone.
func TestValidate_SeveralOperations_ReportsThatNoneIsNamed(t *testing.T) {
	err := Validate("query A { currentUser { id } }\nquery B { currentUser { name } }", nil)

	if err == nil {
		t.Fatal("Validate() error = nil, want the ambiguous-operation refusal")
	}
	if !strings.Contains(err.Error(), "defines 2 operations") {
		t.Errorf("Validate() error = %q, want it to say the document defines two operations", err)
	}
}

// TestValidationError_Error_JoinsEveryReason verifies that the one-line form a
// caller gets from the error interface carries all the reasons, since a caller
// that only prints %v must not lose four of five failures.
func TestValidationError_Error_JoinsEveryReason(t *testing.T) {
	err := &ValidationError{Reasons: []string{"first", "second"}}

	if got := err.Error(); got != "first; second" {
		t.Errorf("Error() = %q, want %q", got, "first; second")
	}
}

// TestErrorsAs_ValidationError_IsRecoverable verifies that a caller can reach
// the reasons through errors.As, which is how the test transport and the
// document audit both format a refusal.
func TestErrorsAs_ValidationError_IsRecoverable(t *testing.T) {
	err := ValidateDocument(`query { vulnerability(id: "x") { hasSolutions } }`)

	var refusal *ValidationError
	if !errors.As(err, &refusal) {
		t.Fatalf("ValidateDocument() error = %T, want *ValidationError", err)
	}
	if len(refusal.Reasons) == 0 {
		t.Error("the refusal carries no reasons")
	}
}

// TestValidate_MiscasedEnumValue_IsRefusedInEveryPosition verifies the check
// gqlparser does not perform.
//
// Its coercer compares an enum value with strings.EqualFold, so "critical"
// passes for VulnerabilitySeverity while gitlab.com answers "Expected
// \"critical\" to be one of: INFO, UNKNOWN, LOW, MEDIUM, HIGH, CRITICAL" and
// executes nothing. Every enum this server sends is uppercased somewhere by a
// strings.ToUpper that a refactor can turn into strings.ToLower, so without
// this the gate would miss the defect class it is most likely to meet. The
// three positions are the three shapes our documents use: a list element, a
// bare variable, and a field inside an input object.
func TestValidate_MiscasedEnumValue_IsRefusedInEveryPosition(t *testing.T) {
	const listQuery = `query($severity: [VulnerabilitySeverity!]) {
  project(fullPath: "a/b") { vulnerabilities(severity: $severity) { nodes { id } } }
}`
	const reorder = `mutation($id: WorkItemID!, $childrenIds: [WorkItemID!]!, $adjacentWorkItemId: WorkItemID!, $relativePosition: RelativePositionType!) {
  workItemUpdate(input: {id: $id, hierarchyWidget: {childrenIds: $childrenIds, adjacentWorkItemId: $adjacentWorkItemId, relativePosition: $relativePosition}}) { errors }
}`
	const dismiss = `mutation($input: VulnerabilityDismissInput!) { vulnerabilityDismiss(input: $input) { errors } }`

	cases := []struct {
		name      string
		document  string
		variables map[string]any
		want      string
	}{
		{
			name:      "inside a list",
			document:  listQuery,
			variables: map[string]any{"severity": []any{"critical", "HIGH"}},
			want:      `$severity[0] is "critical", and VulnerabilitySeverity is case sensitive`,
		},
		{
			name:     "as a bare variable",
			document: reorder,
			variables: map[string]any{
				"id": "gid://gitlab/WorkItem/1", "childrenIds": []any{"gid://gitlab/WorkItem/2"},
				"adjacentWorkItemId": "gid://gitlab/WorkItem/3", "relativePosition": "before",
			},
			want: `$relativePosition is "before", and RelativePositionType is case sensitive`,
		},
		{
			name:     "inside an input object",
			document: dismiss,
			variables: map[string]any{"input": map[string]any{
				"id": "gid://gitlab/Vulnerability/1", "dismissalReason": "used_in_tests",
			}},
			want: `$input.dismissalReason is "used_in_tests", and VulnerabilityDismissalReason is case sensitive`,
		},
		{
			name:      "the right case passes",
			document:  listQuery,
			variables: map[string]any{"severity": []any{"CRITICAL"}},
		},
		{
			name:      "a value no spelling would fix is left to the coercer",
			document:  listQuery,
			variables: map[string]any{"severity": []any{"NOT_A_SEVERITY"}},
			want:      "is not a valid VulnerabilitySeverity",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := Validate(testCase.document, testCase.variables)

			if testCase.want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() error = nil, want one naming %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Validate() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestValidate_ShapesTheEnumWalkCannotJudge_AreLeftToTheCoercer verifies that
// the walk stays out of the way wherever it has nothing to add.
//
// It descends the declared type rather than the value, so anything that does
// not match its type is somebody else's finding: gqlparser has already reported
// it, and a second message about one mistake helps nobody. These are the shapes
// that reach the walk and must pass through it untouched.
func TestValidate_ShapesTheEnumWalkCannotJudge_AreLeftToTheCoercer(t *testing.T) {
	const listQuery = `query($severity: [VulnerabilitySeverity!]) {
  project(fullPath: "a/b") { vulnerabilities(severity: $severity) { nodes { id } } }
}`
	const scalars = `query($path: ID!, $first: Int) {
  project(fullPath: $path) { branchRules(first: $first) { nodes { name } } }
}`
	const dismiss = `mutation($input: VulnerabilityDismissInput!) { vulnerabilityDismiss(input: $input) { errors } }`

	cases := []struct {
		name      string
		document  string
		variables map[string]any
		wantOK    bool
	}{
		{name: "a null where an enum was declared", document: listQuery, variables: map[string]any{"severity": nil}, wantOK: true},
		{name: "an unset variable", document: listQuery, variables: map[string]any{}, wantOK: true},
		{name: "plain scalars, which hold no enum", document: scalars, variables: map[string]any{"path": "a/b", "first": 20}, wantOK: true},
		{
			name:      "an enum sent as a number",
			document:  listQuery,
			variables: map[string]any{"severity": []any{7}},
		},
		{
			name:      "a list sent as one value",
			document:  listQuery,
			variables: map[string]any{"severity": "CRITICAL"},
			wantOK:    true,
		},
		{
			name:      "an input object sent as a string",
			document:  dismiss,
			variables: map[string]any{"input": "not an object"},
		},
		{
			name:     "an input object with a member the type does not have",
			document: dismiss,
			variables: map[string]any{"input": map[string]any{
				"id": "gid://gitlab/Vulnerability/1", "nosuchfield": "x",
			}},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := Validate(testCase.document, testCase.variables)

			if testCase.wantOK && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if testCase.wantOK {
				return
			}
			if err == nil {
				t.Fatal("Validate() error = nil, want the coercer's own refusal")
			}
			if strings.Contains(err.Error(), "case sensitive") {
				t.Errorf("Validate() error = %q, want the coercer's message and not a case complaint", err)
			}
		})
	}
}

// TestEnumWalker_AnUnknownTypeName_IsLeftAlone verifies the walk's guard for a
// type the schema does not define.
//
// Nothing reaches it through [Validate], because the document half refuses an
// operation declaring an unknown type before the variables are ever walked. It
// is exercised directly because the alternative to the guard is a nil
// dereference, and a walk that panics would take down every test in the
// repository the first time an invariant slipped.
func TestEnumWalker_AnUnknownTypeName_IsLeftAlone(t *testing.T) {
	schema, err := Schema()
	if err != nil {
		t.Fatalf("Schema() error = %v", err)
	}
	walker := enumWalker{schema: schema}

	walker.walk(&ast.Type{NamedType: "NoSuchTypeExistsHere"}, "critical", "$probe")

	if len(walker.reasons) != 0 {
		t.Errorf("walk() reported %v for a type the schema does not define, want nothing", walker.reasons)
	}
}
