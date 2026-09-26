// metadata_audit_test.go contains focused tests for the metadata audit
// helpers. Tests cover the small pure-function audits (naming, descriptions,
// annotations, schema validity) without spinning up the full MCP server.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestAuditNaming_FlagsMismatchedToolNames verifies auditNaming reports a
// violation per tool name that does not match the supplied pattern, naming
// the tool as its subject and the pattern in its detail.
func TestAuditNaming_FlagsMismatchedToolNames(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "gitlab_project_get"},
		{Name: "InvalidName"},
		{Name: "gitlabProjectGet"},
	}

	got := auditNaming(tools, toolNameRe, "individual")
	want := []violation{
		{"InvalidName", "naming", "individual tool name does not match " + toolNameRe.String()},
		{"gitlabProjectGet", "naming", "individual tool name does not match " + toolNameRe.String()},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("auditNaming() = %+v, want %+v", got, want)
	}
}

// TestAuditNaming_AllValidReturnsNoViolations verifies the audit is empty
// when every name matches the pattern.
func TestAuditNaming_AllValidReturnsNoViolations(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "gitlab_project_get"},
		{Name: "gitlab_issue_list"},
	}
	if got := auditNaming(tools, toolNameRe, "individual"); len(got) != 0 {
		t.Fatalf("auditNaming() = %d violations, want 0", len(got))
	}
}

// TestAuditDescriptions_FlagsShortDescription verifies the description audit
// reports tools whose description is shorter than minDescLen, with the
// length and the text in the detail.
func TestAuditDescriptions_FlagsShortDescription(t *testing.T) {
	short := strings.Repeat("a", minDescLen-1)
	tools := []*mcp.Tool{
		{Name: "ok", Description: strings.Repeat("a", minDescLen)},
		{Name: "short", Description: short},
	}

	got := auditDescriptions(tools, "individual")
	want := []violation{
		{"short", "description", fmt.Sprintf("individual description too short (%d chars): %q", minDescLen-1, short)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("auditDescriptions() = %+v, want %+v", got, want)
	}
}

// TestAuditAnnotations_DetectsNilAndConflictingHints verifies the annotation
// audit flags nil annotations and conflicting ReadOnly/Destructive hints, and
// leaves a read-only tool that states no destructive hint alone: nil is the
// hint's absence, not a conflict.
func TestAuditAnnotations_DetectsNilAndConflictingHints(t *testing.T) {
	destr := true
	tools := []*mcp.Tool{
		{Name: "nil_ann"},
		{Name: "conflict", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &destr}},
		{Name: "ok", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destr}},
		{Name: "read_only_unstated", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
	}

	got := auditAnnotations(tools, "individual")
	want := []violation{
		{"nil_ann", "annotations", "individual tool has nil Annotations"},
		{"conflict", "annotations", "ReadOnlyHint=true conflicts with DestructiveHint=true"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("auditAnnotations() = %+v, want %+v", got, want)
	}
}

// TestAuditAnnotationTypes_ValidatesReadAndDeleteNames verifies the audit
// reports exactly the tools whose name suffix contradicts their annotation
// hint: a read name without ReadOnlyHint, a delete name whose DestructiveHint
// is absent or false. A read name that is read-only and a name that suggests
// neither raise nothing, whatever their hints say, which is what tells the
// rule from one that fires on every tool that is not read-only.
func TestAuditAnnotationTypes_ValidatesReadAndDeleteNames(t *testing.T) {
	notDestructive := false
	tools := []*mcp.Tool{
		{Name: "gitlab_project_list", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false}},
		{Name: "gitlab_project_delete", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: nil}},
		{Name: "gitlab_branch_delete", Annotations: &mcp.ToolAnnotations{DestructiveHint: &notDestructive}},
		{Name: "gitlab_project_get", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
		{Name: "gitlab_project_create", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false}},
	}

	got := auditAnnotationTypes(tools)
	want := []violation{
		{"gitlab_project_list", "annotation-type", "name suggests read-only but ReadOnlyHint is false"},
		{"gitlab_project_delete", "annotation-type", "name suggests delete but DestructiveHint is not true"},
		{"gitlab_branch_delete", "annotation-type", "name suggests delete but DestructiveHint is not true"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("auditAnnotationTypes() = %+v, want %+v", got, want)
	}
}

// TestAuditAnnotationTypes_NilAnnotationsAreSkipped verifies a nil annotation
// pointer is silently skipped by the audit.
func TestAuditAnnotationTypes_NilAnnotationsAreSkipped(t *testing.T) {
	tools := []*mcp.Tool{{Name: "x"}}
	if got := auditAnnotationTypes(tools); len(got) != 0 {
		t.Fatalf("auditAnnotationTypes() = %d, want 0", len(got))
	}
}

// TestAuditInputSchema_RequiresObjectType verifies the input schema audit
// reports the tools whose schema is not a map or not typed as object, and
// which of the two each one is, and passes the one typed as object.
func TestAuditInputSchema_RequiresObjectType(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "wrong_type", InputSchema: map[string]any{"type": "string"}},
		{Name: "no_map", InputSchema: "not a map"},
		{Name: "ok", InputSchema: map[string]any{"type": "object"}},
	}

	got := auditInputSchema(tools)
	want := []violation{
		{"wrong_type", "input-schema", `InputSchema type="string", expected "object"`},
		{"no_map", "input-schema", "InputSchema is not a map"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("auditInputSchema() = %+v, want %+v", got, want)
	}
}

// TestAuditAdditionalProperties_RequiresFalseConstraint verifies the audit
// reports a schema that leaves additionalProperties out, sets it true, or
// sets it to something that is not a boolean at all, each with the value it
// found, and passes one that sets it false.
func TestAuditAdditionalProperties_RequiresFalseConstraint(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "missing", InputSchema: map[string]any{"type": "object"}},
		{Name: "true_value", InputSchema: map[string]any{"type": "object", "additionalProperties": true}},
		{Name: "not_a_bool", InputSchema: map[string]any{"type": "object", "additionalProperties": "no"}},
		{Name: "ok", InputSchema: map[string]any{"type": "object", "additionalProperties": false}},
	}

	got := auditAdditionalProperties(tools, "individual")
	want := []violation{
		{"missing", "additional-properties", "individual tool inputSchema missing additionalProperties:false"},
		{"true_value", "additional-properties", "individual tool inputSchema additionalProperties=true, want false"},
		{"not_a_bool", "additional-properties", "individual tool inputSchema additionalProperties=no, want false"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("auditAdditionalProperties() = %+v, want %+v", got, want)
	}
}

// TestAuditAdditionalProperties_NonObjectSchemasAreSkipped verifies a
// non-object schema short-circuits the additionalProperties check.
func TestAuditAdditionalProperties_NonObjectSchemasAreSkipped(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "string", InputSchema: map[string]any{"type": "string"}},
		{Name: "not_a_map", InputSchema: "not a map"},
		{Name: "no_type_no_properties", InputSchema: map[string]any{}},
	}
	if got := auditAdditionalProperties(tools, "individual"); len(got) != 0 {
		t.Fatalf("auditAdditionalProperties() = %d, want 0: %+v", len(got), got)
	}
}

// TestIsObjectSchema_DetectsTypeAndProperties verifies isObjectSchema returns
// true for both type=object and schemas with a properties key.
func TestIsObjectSchema_DetectsTypeAndProperties(t *testing.T) {
	tests := []struct {
		name string
		node map[string]any
		want bool
	}{
		{"type=object", map[string]any{"type": "object"}, true},
		{"properties only", map[string]any{"properties": map[string]any{}}, true},
		{"string type", map[string]any{"type": "string"}, false},
		{"empty", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isObjectSchema(tt.node); got != tt.want {
				t.Fatalf("isObjectSchema() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAuditDuplicates_ReportsDuplicatesByName verifies duplicate tool names are
// flagged as audit violations.
func TestAuditDuplicates_ReportsDuplicatesByName(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "gitlab_x"},
		{Name: "gitlab_y"},
		{Name: "gitlab_x"},
	}
	got := auditDuplicates(tools, "individual")
	want := []violation{{"gitlab_x", "duplicate", "duplicate individual tool name"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("auditDuplicates() = %+v, want %+v", got, want)
	}
}

// TestIsReadToolName_DetectsReadSuffixes verifies the read suffix check covers
// all configured suffixes.
func TestIsReadToolName_DetectsReadSuffixes(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"gitlab_project_list", true},
		{"gitlab_issue_get", true},
		{"gitlab_code_search", true},
		{"gitlab_commit_diff", true},
		{"gitlab_create_issue", false},
		{"plain", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := toolutil.IsReadToolName(tt.in); got != tt.want {
				t.Fatalf("toolutil.IsReadToolName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsDeleteToolName_DetectsDeleteForms verifies the delete check covers
// the _delete suffix and the "delete" segment of a name.
func TestIsDeleteToolName_DetectsDeleteForms(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"gitlab_project_delete", true},
		{"gitlab_delete_branch", true},
		{"delete_user", true},
		{"gitlab_project_get", false},
		{"plain", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := toolutil.IsDeleteToolName(tt.in); got != tt.want {
				t.Fatalf("toolutil.IsDeleteToolName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestPtrBool_FormatsPointer verifies the formatter handles nil, true, and
// false values.
func TestPtrBool_FormatsPointer(t *testing.T) {
	tr := true
	fa := false
	if got := ptrBool(nil); got != "nil" {
		t.Errorf("ptrBool(nil) = %q, want nil", got)
	}
	if got := ptrBool(&tr); got != "true" {
		t.Errorf("ptrBool(&true) = %q, want true", got)
	}
	if got := ptrBool(&fa); got != "false" {
		t.Errorf("ptrBool(&false) = %q, want false", got)
	}
}

// TestAuditResultEnvelopes_Registry_DrivesEveryFormatterAndCountsWhatItLacks
// runs the envelope audit over the real registry and checks its shape: every
// registered formatter is driven, a nil render of a zero value is counted
// apart from one of a populated value, nothing panics, and the registration
// record is the registry's own. What the lists hold is reported, not
// asserted: the audit reports in this layer, and the dispatcher annotates
// every text block on its way out, so an unannotated block here is a
// formatter building its own envelope rather than a block the client sees
// bare.
func TestAuditResultEnvelopes_Registry_DrivesEveryFormatterAndCountsWhatItLacks(t *testing.T) {
	// Two formatters registered here, for this process, give the audit one of
	// each shape the tree does not have: a render that is nothing for a
	// populated value, and a render that panics on a zero value.
	type silent struct{ Name string }
	type fragile struct{ Name string }
	toolutil.RegisterMarkdown(func(silent) string { return "" })
	toolutil.RegisterMarkdown(func(v fragile) string {
		if v.Name == "" {
			panic("no name")
		}
		return "## " + v.Name
	})

	audit, _ := auditResultEnvelopes()

	if audit.Formatters == 0 || audit.Formatters != toolutil.MarkdownFormatterCount() {
		t.Errorf("formatters = %d, want the %d the registry holds", audit.Formatters, toolutil.MarkdownFormatterCount())
	}
	if audit.NilOnZero == 0 {
		t.Error("no formatter rendered nothing for a zero value, and the guarded ones do")
	}
	if !hasEntryContaining(audit.NilOnPopulated, ".silent") {
		t.Errorf("nil on populated = %v, want the silent formatter listed", audit.NilOnPopulated)
	}
	if !hasEntryContaining(audit.Panicked, ".fragile (") || !hasEntryContaining(audit.Panicked, "[zero]: no name") {
		t.Errorf("panicked = %v, want the fragile formatter listed with its message", audit.Panicked)
	}
	if got := strings.Join(audit.RegistrationProblems, "\n"); got != strings.Join(toolutil.MarkdownRegistrationProblems(), "\n") {
		t.Errorf("registration problems = %q, want the registry's own record", got)
	}
	t.Logf("envelopes: %d formatter(s), %d nil on zero, %d nil on populated, %d unannotated block(s), %d registration problem(s)",
		audit.Formatters, audit.NilOnZero, len(audit.NilOnPopulated), len(audit.Unannotated), len(audit.RegistrationProblems))
	for _, entry := range append(append([]string{}, audit.NilOnPopulated...), audit.Unannotated...) {
		t.Logf("envelope: %s", entry)
	}
}

// hasEntryContaining reports whether any entry of a list carries the text.
func hasEntryContaining(entries []string, text string) bool {
	for _, entry := range entries {
		if strings.Contains(entry, text) {
			return true
		}
	}
	return false
}

// TestAuditResultEnvelopes_Counts_AreTheRegistryRenderedPlainly holds every
// count and list of the envelope audit to the registry rendered without the
// audit's own sorting: each registered type filled and rendered in both
// states, a nil render counted under the state that produced it, a panic
// under its message, a bare block under its position. The three conditions
// that sort a render share one switch, and a test asserting that a count is
// not zero cannot tell a counter that counts the wrong state, or counts
// downwards, from one that counts the right renders.
func TestAuditResultEnvelopes_Counts_AreTheRegistryRenderedPlainly(t *testing.T) {
	registerBareBlock()
	audit, _ := auditResultEnvelopes()

	want := envelopeAudit{RegistrationProblems: toolutil.MarkdownRegistrationProblems()}
	for _, typ := range toolutil.RegisteredMarkdownTypes() {
		want.Formatters++
		// A formatter registered as a nil function has no name to add.
		name := typ.String()
		if fn := toolutil.RegisteredMarkdownFormatterName(typ); fn != "" {
			name += " (" + fn + ")"
		}
		for _, state := range []testutil.FixtureState{testutil.FixtureZero, testutil.FixtureMultiPage} {
			result, panicked := renderEnvelope(testutil.FillFixture(typ, testutil.FixtureOptions{State: state, Text: fixtureText}))
			switch {
			case panicked != "":
				want.Panicked = append(want.Panicked, fmt.Sprintf("%s [%s]: %s", name, state, panicked))
			case result == nil && state == testutil.FixtureZero:
				want.NilOnZero++
			case result == nil:
				want.NilOnPopulated = append(want.NilOnPopulated, name)
			default:
				for i, block := range result.Content {
					if !blockAnnotated(block) {
						want.Unannotated = append(want.Unannotated, fmt.Sprintf("%s [%s] block %d (%T)", name, state, i, block))
					}
				}
			}
		}
	}

	if audit.Formatters != want.Formatters || audit.NilOnZero != want.NilOnZero {
		t.Errorf("formatters = %d, nil on zero = %d; want %d and %d", audit.Formatters, audit.NilOnZero, want.Formatters, want.NilOnZero)
	}
	if !reflect.DeepEqual(audit.NilOnPopulated, want.NilOnPopulated) {
		t.Errorf("nil on populated = %v, want %v", audit.NilOnPopulated, want.NilOnPopulated)
	}
	if !reflect.DeepEqual(audit.Panicked, want.Panicked) {
		t.Errorf("panicked = %v, want %v", audit.Panicked, want.Panicked)
	}
	if !reflect.DeepEqual(audit.Unannotated, want.Unannotated) {
		t.Errorf("unannotated = %v, want %v", audit.Unannotated, want.Unannotated)
	}
	if !reflect.DeepEqual(audit.RegistrationProblems, want.RegistrationProblems) {
		t.Errorf("registration problems = %v, want %v", audit.RegistrationProblems, want.RegistrationProblems)
	}
}

// bareBlock is the output type of a result formatter that builds its own
// envelope and leaves the annotations off, which a string formatter cannot
// do: the registry annotates every string it wraps.
type bareBlock struct{ Name string }

// registerBareBlock registers that formatter once for the process.
func registerBareBlock() {
	bareBlockOnce.Do(func() {
		toolutil.RegisterMarkdownResult(func(bareBlock) *mcp.CallToolResult {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "bare"}}}
		})
	})
}

// bareBlockOnce keeps the bare-block formatter to one registration.
var bareBlockOnce sync.Once

// TestAuditResultEnvelopes_BareBlock_IsListedWithItsPosition checks the one
// entry the walk writes for a block that reaches the client with no
// audience: the formatter's name, the state that rendered it, the block's
// index and its kind, in both states, since a result formatter is served
// as it is and the dispatcher annotates nothing it built itself.
func TestAuditResultEnvelopes_BareBlock_IsListedWithItsPosition(t *testing.T) {
	registerBareBlock()
	audit, _ := auditResultEnvelopes()

	typ := reflect.TypeFor[bareBlock]()
	name := typ.String() + " (" + toolutil.RegisteredMarkdownFormatterName(typ) + ")"
	for _, want := range []string{
		name + " [zero] block 0 (*mcp.TextContent)",
		name + " [multi-page] block 0 (*mcp.TextContent)",
	} {
		t.Run(want, func(t *testing.T) {
			if !slices.Contains(audit.Unannotated, want) {
				t.Errorf("unannotated = %v, want it to carry %q", audit.Unannotated, want)
			}
		})
	}
}

// TestRegisteredMarkdownTypes_EveryFormatterIsNamed pins the property the
// envelope walk's name guard rests on: the registry records a function name
// for every type the server registers, so the served report never prints a
// bare type. A formatter that lost its name would be one a finding could not
// point a reader at. The one registration with no name is a nil function,
// which this package registers on purpose to drive the guard, so the types it
// declares are left out.
func TestRegisteredMarkdownTypes_EveryFormatterIsNamed(t *testing.T) {
	types := toolutil.RegisteredMarkdownTypes()
	if len(types) == 0 {
		t.Fatal("the registry holds no formatter, so nothing was checked")
	}
	for _, typ := range types {
		if typ.PkgPath() == reflect.TypeFor[unnamedFormatter]().PkgPath() {
			continue
		}
		if toolutil.RegisteredMarkdownFormatterName(typ) == "" {
			t.Errorf("%s is registered under no function name", typ)
		}
	}
}

// TestRenderEnvelope_Panic_IsReportedRatherThanRaised checks the recovery
// the audit relies on to finish a run whose one formatter panics.
func TestRenderEnvelope_Panic_IsReportedRatherThanRaised(t *testing.T) {
	type panicking struct{ Name string }
	toolutil.RegisterMarkdown(func(panicking) string { panic("no render") })

	value := testutil.FillFixture(reflect.TypeFor[panicking](), testutil.FixtureOptions{State: testutil.FixtureMultiPage, Text: fixtureText})
	result, panicked := renderEnvelope(value)

	if result != nil || panicked != "no render" {
		t.Errorf("renderEnvelope = %v, %q, want nil and the panic's message", result, panicked)
	}
}

// TestBlockAnnotated_Blocks_ReadsTheAnnotationOfEachKind checks the block
// kinds the audit knows and the one it does not.
func TestBlockAnnotated_Blocks_ReadsTheAnnotationOfEachKind(t *testing.T) {
	cases := []struct {
		name  string
		block mcp.Content
		want  bool
	}{
		{name: "annotated text", block: &mcp.TextContent{Text: "x", Annotations: toolutil.ContentList}, want: true},
		{name: "bare text", block: &mcp.TextContent{Text: "x"}, want: false},
		{name: "annotated image", block: &mcp.ImageContent{Annotations: toolutil.ContentUser}, want: true},
		{name: "bare image", block: &mcp.ImageContent{}, want: false},
		{name: "annotated resource", block: &mcp.EmbeddedResource{Annotations: toolutil.ContentDetail}, want: true},
		{name: "bare resource", block: &mcp.EmbeddedResource{}, want: false},
		{name: "a kind the audit does not know", block: &mcp.AudioContent{}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := blockAnnotated(tc.block); got != tc.want {
				t.Errorf("blockAnnotated = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPrintReport_EmptyViolationsWritesNoViolationsMessage verifies the report
// writes the no-violations message when there are no findings, over two
// surfaces of different sizes so the two counts cannot trade places.
func TestPrintReport_EmptyViolationsWritesNoViolationsMessage(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout.
	output := captureStdout(t, func() {
		printMetadataReport(
			[]*mcp.Tool{{Name: "gitlab_x"}, {Name: "gitlab_z"}},
			[]*mcp.Tool{{Name: "gitlab_y"}},
			nil,
			nil,
			envelopeAudit{Formatters: 3, NilOnZero: 1},
		)
	})

	for _, want := range []string{
		"# MCP Tool Metadata Audit Report",
		"| Individual tools | 2 |",
		"| Meta-tools | 1 |",
		"| Total violations | 0 |",
		"**No violations found.**",
		"## Result Envelopes",
		"| Registered formatters | 3 |",
		"| Nil render of the zero value | 1 |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Fatalf("printReport() output missing %q:\n%s", want, output)
			}
		})
	}
}

// TestPrintReport_GroupsViolationsByCategory verifies findings are grouped
// and listed in the report.
func TestPrintReport_GroupsViolationsByCategory(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout.
	individual := []*mcp.Tool{
		{Name: "tool_a", Description: strings.Repeat("a", minDescLen+5)},
		{Name: "tool_b", Description: strings.Repeat("b", minDescLen+5)},
	}
	violations := []violation{
		{tool: "tool_a", category: "naming", detail: "bad name"},
		{tool: "tool_b", category: "description", detail: "too short"},
	}

	output := captureStdout(t, func() {
		printMetadataReport(individual, nil, violations, nil, envelopeAudit{
			NilOnPopulated:       []string{"x.Output (x.FormatMarkdown)"},
			Unannotated:          []string{"y.Output [multi-page] block 0 (*mcp.TextContent)"},
			Panicked:             []string{"z.Output [zero]: nil map"},
			RegistrationProblems: []string{"duplicate Markdown formatter for w.Output: the first registration is kept"},
		})
	})

	for _, want := range []string{
		"| Total violations | 2 |",
		"## Violations by Category",
		"### description (1)",
		"### naming (1)",
		"`tool_a` | bad name",
		"`tool_b` | too short",
		"### Individual Tools (2)",
		"### Nil render of the populated value (1)",
		"- `x.Output (x.FormatMarkdown)`",
		"### Content blocks without Annotations (1)",
		"### Formatters that panicked (1)",
		"### Registration problems (1)",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Fatalf("printReport() output missing %q:\n%s", want, output)
			}
		})
	}
}

// TestPrintReport_ListsAllMetaToolsAndTruncatesDescription verifies the meta
// tool section renders names with truncated descriptions and annotation string.
func TestPrintReport_ListsAllMetaToolsAndTruncatesDescription(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout.
	individual := []*mcp.Tool{{Name: "tool_a", Description: strings.Repeat("a", 200)}}
	meta := []*mcp.Tool{
		{Name: "meta_a", Description: strings.Repeat("m", 200), Title: "Meta A"},
	}
	// One non-nil violation keeps the report from short-circuiting and lets us
	// exercise the "All Tools" section, including the meta-tools table.
	vs := []violation{{tool: "tool_a", category: "naming", detail: "bad"}}

	output := captureStdout(t, func() {
		printMetadataReport(individual, meta, vs, nil, envelopeAudit{})
	})

	if !strings.Contains(output, "### Meta-Tools (1)") {
		t.Fatalf("printReport() output missing meta section header:\n%s", output)
	}
	if !strings.Contains(output, "`meta_a`") {
		t.Fatalf("printReport() output missing meta tool name:\n%s", output)
	}
	if !strings.Contains(output, "...") {
		t.Fatalf("printReport() output missing truncated description marker:\n%s", output)
	}
}

// TestPrintReport_AllTools_NumbersRowsAndTruncatesPastSixtyCharacters pins
// the two tables of the "All Tools" section: rows are numbered from one, a
// description of exactly sixty characters is printed whole, one character
// more is cut to sixty and marked, and the annotation column spells each
// hint. Both tables are checked, since each has a loop of its own.
func TestPrintReport_AllTools_NumbersRowsAndTruncatesPastSixtyCharacters(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout.
	sixty := strings.Repeat("a", 60)
	individual := []*mcp.Tool{
		{Name: "ind_whole", Description: sixty},
		{Name: "ind_cut", Description: strings.Repeat("b", 61), Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
	}
	meta := []*mcp.Tool{
		{Name: "meta_short", Description: "short"},
		{Name: "meta_whole", Description: strings.Repeat("d", 60)},
		{Name: "meta_cut", Description: strings.Repeat("c", 61)},
	}
	vs := []violation{{tool: "ind_whole", category: "naming", detail: "bad"}}

	output := captureStdout(t, func() {
		printMetadataReport(individual, meta, vs, nil, envelopeAudit{})
	})

	for _, want := range []string{
		"### Individual Tools (2)",
		"| 1 | `ind_whole` | " + sixty + " | nil |",
		"| 2 | `ind_cut` | " + strings.Repeat("b", 60) + "... | RO=true D=nil I=false OW=nil |",
		"### Meta-Tools (3)",
		"| 1 | `meta_short` | short | nil |",
		"| 2 | `meta_whole` | " + strings.Repeat("d", 60) + " | nil |",
		"| 3 | `meta_cut` | " + strings.Repeat("c", 60) + "... | nil |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Errorf("printReport() output missing %q:\n%s", want, output)
			}
		})
	}
}

// TestPrintReport_AllTools_AnnotationColumnSpellsEachHintInItsPlace pins the
// annotation column of both tables. Two of the four hints are booleans and two
// are optional, printed nil when unstated, so no single row can give each of
// the four a value the other three lack; these two rows together differ
// wherever any two hints could trade places, and each table has a loop of its
// own.
func TestPrintReport_AllTools_AnnotationColumnSpellsEachHintInItsPlace(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout.
	no, yes := false, true
	first := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: false, OpenWorldHint: &no}
	second := &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &yes, IdempotentHint: true}
	individual := []*mcp.Tool{{Name: "ind_first", Annotations: first}, {Name: "ind_second", Annotations: second}}
	meta := []*mcp.Tool{{Name: "meta_first", Annotations: first}, {Name: "meta_second", Annotations: second}}
	vs := []violation{{tool: "ind_first", category: "naming", detail: "bad"}}

	output := captureStdout(t, func() {
		printMetadataReport(individual, meta, vs, nil, envelopeAudit{})
	})

	for _, want := range []string{
		"| 1 | `ind_first` |  | RO=true D=nil I=false OW=false |\n",
		"| 2 | `ind_second` |  | RO=false D=true I=true OW=nil |\n",
		"| 1 | `meta_first` |  | RO=true D=nil I=false OW=false |\n",
		"| 2 | `meta_second` |  | RO=false D=true I=true OW=nil |\n",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Errorf("printReport() output missing %q:\n%s", want, output)
			}
		})
	}
}

// TestPrintResultEnvelopes_EachCountAndList_SitsUnderItsOwnTitle pins the
// envelope section over an audit in which no two counts agree: each count row
// carries its own figure, and each list is printed whole under its own
// heading. With every list one entry long, the four list rows could trade
// figures, and two lists could trade headings, and still print every line.
func TestPrintResultEnvelopes_EachCountAndList_SitsUnderItsOwnTitle(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout.
	audit := envelopeAudit{
		Formatters:           11,
		NilOnZero:            5,
		NilOnPopulated:       []string{"a.Output (a.Format)"},
		Unannotated:          []string{"b.Output [zero] block 0 (*mcp.TextContent)", "b.Output [multi-page] block 1 (*mcp.TextContent)"},
		Panicked:             []string{"c.One [zero]: first", "c.Two [zero]: second", "c.Three [multi-page]: third"},
		RegistrationProblems: []string{"problem one", "problem two", "problem three", "problem four"},
	}

	output := captureStdout(t, func() { printResultEnvelopes(audit) })

	for _, want := range []string{
		"| Registered formatters | 11 |\n",
		"| Nil render of the zero value | 5 |\n",
		"| Nil render of the populated value | 1 |\n",
		"| Content blocks without Annotations | 2 |\n",
		"| Formatters that panicked | 3 |\n",
		"| Registration problems | 4 |\n",
		"### Nil render of the populated value (1)\n\n- `a.Output (a.Format)`\n\n",
		"### Content blocks without Annotations (2)\n\n- `b.Output [zero] block 0 (*mcp.TextContent)`\n- `b.Output [multi-page] block 1 (*mcp.TextContent)`\n\n",
		"### Formatters that panicked (3)\n\n- `c.One [zero]: first`\n- `c.Two [zero]: second`\n- `c.Three [multi-page]: third`\n\n",
		"### Registration problems (4)\n\n- `problem one`\n- `problem two`\n- `problem three`\n- `problem four`\n\n",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(output, want) {
				t.Errorf("printResultEnvelopes() output missing %q:\n%s", want, output)
			}
		})
	}
}

// metadataJSONReport is the metadata view's JSON document with the envelope
// section, which [metadataJSON] leaves out.
type metadataJSONReport struct {
	metadataJSON
	Envelopes envelopeAudit `json:"envelopes"`
}

// TestPrintMetadataReport_JSON_CarriesEachFieldUnderItsKey decodes the JSON
// view of a report with one violation and a populated envelope audit, and
// holds every key to the value it was given. The document is built from a
// positional literal, so two counts, or a violation's tool and category,
// could trade keys and still encode.
func TestPrintMetadataReport_JSON_CarriesEachFieldUnderItsKey(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout and outputJSON is global.
	outputJSON = true
	t.Cleanup(func() { outputJSON = false })
	envelopes := envelopeAudit{
		Formatters:           5,
		NilOnZero:            2,
		NilOnPopulated:       []string{"x.Output (x.FormatMarkdown)"},
		Unannotated:          []string{"y.Output [zero] block 0 (*mcp.TextContent)"},
		Panicked:             []string{"z.Output [multi-page]: nil map"},
		RegistrationProblems: []string{"duplicate Markdown formatter for w.Output: the first registration is kept"},
	}

	out := captureStdout(t, func() {
		printMetadataReport(
			[]*mcp.Tool{{Name: "gitlab_a"}, {Name: "gitlab_b"}},
			[]*mcp.Tool{{Name: "gitlab_c"}},
			[]violation{{"gitlab_a", "naming", "does not match"}},
			nil,
			envelopes,
		)
	})

	var got metadataJSONReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode metadata report: %v\n%s", err, out)
	}
	want := metadataJSONReport{
		View:            "metadata",
		IndividualTools: 2,
		MetaTools:       1,
		Violations:      1,
		Entries:         []jsonEntry{{Tool: "gitlab_a", Category: "naming", Detail: "does not match"}},
		Envelopes:       envelopes,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("metadata JSON = %+v, want %+v", got, want)
	}
}

// TestPrintMetadataReport_JSON_UnwritableStdout_IsSaidOnStderr checks the
// one failure the JSON view can have, a stdout that cannot be written, and
// that it is said on stderr rather than swallowed; and that a stdout that can
// be written says nothing there, so a run that succeeded is not read as one
// that failed.
func TestPrintMetadataReport_JSON_UnwritableStdout_IsSaidOnStderr(t *testing.T) {
	// Not parallel: os.Stdout, os.Stderr and outputJSON are process-wide.
	outputJSON = true
	t.Cleanup(func() { outputJSON = false })
	report := func() { printMetadataReport(nil, nil, nil, nil, envelopeAudit{}) }

	t.Run("a closed stdout", func(t *testing.T) {
		stderr := withClosedStdout(t, func() string { return captureStderr(t, report) })
		if !strings.Contains(stderr, "encode json: ") {
			t.Errorf("stderr = %q, want the encoding failure reported", stderr)
		}
	})
	t.Run("a writable stdout", func(t *testing.T) {
		var stderr string
		captureStdout(t, func() { stderr = captureStderr(t, report) })
		if stderr != "" {
			t.Errorf("stderr = %q, want nothing when the report was written", stderr)
		}
	})
}

// withClosedStdout runs fn with os.Stdout bound to a pipe whose both ends
// are closed, so every write to it fails, and returns what fn returned.
func withClosedStdout(t *testing.T, fn func() string) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	if closeErr := reader.Close(); closeErr != nil {
		t.Fatalf("Close() reader error = %v", closeErr)
	}
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("Close() writer error = %v", closeErr)
	}
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	return fn()
}

// TestRunMetadataAudit_RegisterMetaAudit_ReadsTheTreeUnderTheWorkingDirectory
// drives the metadata view from three working directories and checks what the
// register-meta half does with each: no module root above it is a skip said
// on stderr, a tree it cannot parse is a skip said on stderr, and a tree
// carrying a package-level RegisterMeta is a violation in the report. The
// first two used to be reachable only from a process, so a skip that said
// nothing would have failed no test.
func TestRunMetadataAudit_RegisterMetaAudit_ReadsTheTreeUnderTheWorkingDirectory(t *testing.T) {
	// Not parallel: t.Chdir, captureStdout, captureStderr and outputJSON are
	// process-wide.
	outputJSON = true
	t.Cleanup(func() { outputJSON = false })

	const hub = "package tools\n"
	cases := []struct {
		name       string
		files      map[string]string
		wantStderr string
		wantEntry  *jsonEntry
	}{
		{
			name:       "no module root above the working directory",
			wantStderr: "register meta audit skipped: go.mod not found",
		},
		{
			name: "a tree the audit cannot parse",
			files: map[string]string{
				"go.mod":                          "module example.com/fixture\n",
				"internal/tools/register_meta.go": hub,
				"internal/tools/broken/broken.go": "package broken\n\nfunc (\n",
			},
			wantStderr: "register meta audit skipped: parse ",
		},
		{
			name: "a package-level RegisterMeta under the root",
			files: map[string]string{
				"go.mod":                            "module example.com/fixture\n",
				"internal/tools/register_meta.go":   hub,
				"internal/tools/legacy/register.go": "package legacy\n\nfunc RegisterMeta() {}\n",
			},
			wantEntry: &jsonEntry{
				Tool:     "legacy",
				Category: "register-meta",
				Detail:   "package-level RegisterMeta is not an approved catalog-first runtime pattern (internal/tools/legacy/register.go)",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range tc.files {
				writeTestFile(t, root, name, content)
			}
			t.Chdir(root)
			client := stubClient(t)

			var stderr string
			out := captureStdout(t, func() { stderr = captureStderr(t, func() { runMetadataAudit(client) }) })

			var got metadataJSON
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("decode metadata report: %v\n%s", err, truncate(out))
			}
			if tc.wantStderr != "" && !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr = %q, want it to carry %q", stderr, tc.wantStderr)
			}
			if tc.wantStderr == "" && stderr != "" {
				t.Errorf("stderr = %q, want nothing", stderr)
			}
			var wantEntries []jsonEntry
			if tc.wantEntry != nil {
				wantEntries = []jsonEntry{*tc.wantEntry}
			}
			if got := entriesOfCategory(got.Entries, "register-meta"); !reflect.DeepEqual(got, wantEntries) {
				t.Errorf("register-meta entries = %+v, want %+v", got, wantEntries)
			}
		})
	}
}

// entriesOfCategory returns the entries of one category, in report order.
func entriesOfCategory(entries []jsonEntry, category string) []jsonEntry {
	var matched []jsonEntry
	for _, entry := range entries {
		if entry.Category == category {
			matched = append(matched, entry)
		}
	}
	return matched
}

// spoiled returns [cleanTool] with one respect of it changed.
func spoiled(name string, change func(tool *mcp.Tool)) *mcp.Tool {
	tool := cleanTool(name)
	change(tool)
	return tool
}

// TestRunMetadataAudit_EveryRule_ReportsItsOwnSurfaceUnderItsOwnLabel drives
// the metadata view over listings of its own, carrying one violation of every
// rule on each surface the rule reads, and holds the report to the whole list
// in the order the view composes it. The served surface carries none, so this
// is the one place a rule dropped from the view, or handed the other surface,
// the other surface's label or the other surface's naming pattern, is seen.
func TestRunMetadataAudit_EveryRule_ReportsItsOwnSurfaceUnderItsOwnLabel(t *testing.T) {
	// Not parallel: listSurface, t.Chdir, captureStdout and outputJSON are
	// process-wide.
	outputJSON = true
	t.Cleanup(func() { outputJSON = false })
	registerGatingViolation()

	licensed := spoiled("gitlab_widget_licensed", func(tool *mcp.Tool) {
		tool.Description = "Toggles a licensed widget (Ultimate). Returns: the widget."
	})
	individual := []*mcp.Tool{
		cleanTool("gitlab_widget_create"),
		// One word is the meta pattern's shape, and not the individual one's.
		cleanTool("gitlab_widget"),
		spoiled("gitlab_widget_describe", func(tool *mcp.Tool) { tool.Description = "short" }),
		spoiled("gitlab_widget_bare", func(tool *mcp.Tool) { tool.Annotations = nil }),
		cleanTool("gitlab_widget_list"),
		spoiled("gitlab_widget_typed", func(tool *mcp.Tool) { tool.InputSchema = map[string]any{"type": "string"} }),
		spoiled("gitlab_widget_open", func(tool *mcp.Tool) { tool.InputSchema = map[string]any{"type": "object"} }),
		cleanTool("gitlab_widget_twice"),
		cleanTool("gitlab_widget_twice"),
		licensed,
	}
	// The meta listing is the longer one, which the served surface's is not, so
	// the view is held to listings of any relative size.
	meta := []*mcp.Tool{
		cleanTool("gitlab_gadget_create"),
		cleanTool("gitlab_gadget_update"),
		// One word passes here, and would not under the individual pattern.
		cleanTool("gitlab_gadget"),
		cleanTool("Gitlab-Gadget"),
		spoiled("gitlab_gadget_describe", func(tool *mcp.Tool) { tool.Description = "tiny" }),
		spoiled("gitlab_gadget_bare", func(tool *mcp.Tool) { tool.Annotations = nil }),
		// The two rules that read the individual surface alone would report
		// these two if they were handed this one.
		cleanTool("gitlab_gadget_list"),
		spoiled("gitlab_gadget_typed", func(tool *mcp.Tool) { tool.InputSchema = map[string]any{"type": "array"} }),
		spoiled("gitlab_gadget_open", func(tool *mcp.Tool) {
			tool.InputSchema = map[string]any{"type": "object", "additionalProperties": true}
		}),
		cleanTool("gitlab_gadget_twice"),
		cleanTool("gitlab_gadget_twice"),
	}
	withSurface(t, func(tier edition.Tier, isMeta bool) []*mcp.Tool {
		switch {
		case isMeta:
			return meta
		case tier == edition.Free:
			// Served from Premium up, below the Ultimate its description states.
			return slices.DeleteFunc(slices.Clone(individual), func(tool *mcp.Tool) bool { return tool == licensed })
		default:
			return individual
		}
	})
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/fixture\n")
	writeTestFile(t, root, "internal/tools/register_meta.go", "package tools\n")
	writeTestFile(t, root, "internal/tools/legacy/register.go", "package legacy\n\nfunc RegisterMeta() {}\n")
	t.Chdir(root)

	var returned int
	out := captureStdout(t, func() { returned = runMetadataAudit(nil) })

	var got metadataJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode metadata report: %v\n%s", err, truncate(out))
	}
	if got.IndividualTools != len(individual) || got.MetaTools != len(meta) {
		t.Errorf("individual_tools = %d, meta_tools = %d; want %d and %d", got.IndividualTools, got.MetaTools, len(individual), len(meta))
	}
	if returned != got.Violations || got.Violations != len(got.Entries) {
		t.Errorf("runMetadataAudit() = %d, violations = %d, entries = %d; want all three equal", returned, got.Violations, len(got.Entries))
	}
	want := []jsonEntry{
		{"gitlab_widget", "naming", "individual tool name does not match " + toolNameRe.String()},
		{"Gitlab-Gadget", "naming", "meta tool name does not match " + metaToolNameRe.String()},
		{"gitlab_widget_describe", "description", `individual description too short (5 chars): "short"`},
		{"gitlab_gadget_describe", "description", `meta description too short (4 chars): "tiny"`},
		{"gitlab_widget_bare", "annotations", "individual tool has nil Annotations"},
		{"gitlab_gadget_bare", "annotations", "meta tool has nil Annotations"},
		{"gitlab_widget_list", "annotation-type", "name suggests read-only but ReadOnlyHint is false"},
		{"gitlab_widget_typed", "input-schema", `InputSchema type="string", expected "object"`},
		{"gitlab_widget_open", "additional-properties", "individual tool inputSchema missing additionalProperties:false"},
		{"gitlab_gadget_open", "additional-properties", "meta tool inputSchema additionalProperties=true, want false"},
		{"gitlab_widget_twice", "duplicate", "duplicate individual tool name"},
		{"gitlab_gadget_twice", "duplicate", "duplicate meta tool name"},
		{"gitlab_widget_licensed", editionTierCategory, `the description states "Ultimate" and the surface serves the tool from premium`},
		{"legacy", "register-meta", "package-level RegisterMeta is not an approved catalog-first runtime pattern (internal/tools/legacy/register.go)"},
	}
	if len(got.Entries) < len(want) {
		t.Fatalf("entries = %+v, want at least the %d this listing carries", got.Entries, len(want))
	}
	if !reflect.DeepEqual(got.Entries[:len(want)], want) {
		t.Errorf("entries = %+v\nwant   %+v", got.Entries[:len(want)], want)
	}
	// The constant-index rule reads the global registry, which holds whatever
	// formatters this package's tests have registered by now, so the tail is
	// held to its rule, its place last and the one formatter this test owns.
	var gating bool
	for _, entry := range got.Entries[len(want):] {
		if entry.Category != constantIndexCategory || !strings.HasPrefix(entry.Tool, "main.") {
			t.Errorf("entry %+v follows the listing's own, where only this package's constant-index findings belong", entry)
		}
		gating = gating || strings.HasPrefix(entry.Tool, "main.gatingList ")
	}
	if !gating {
		t.Errorf("entries = %+v, want the constant-index violation this test registered last", got.Entries)
	}
}

// TestRunMetadataAudit_ServedSurface_ReportsOnlyThisPackagesOwnFormatters is
// the metadata view over the surface the server registers and the tree the
// package sits in: nothing it reports may name anything but a formatter this
// package's tests register to prove the constant-index rule fires. Every other
// rule is otherwise asserted over hand-made listings only, so a rule handed
// the other surface's naming pattern, which reports every one-word meta tool,
// was seen by nothing short of the gate's own run.
func TestRunMetadataAudit_ServedSurface_ReportsOnlyThisPackagesOwnFormatters(t *testing.T) {
	// Not parallel: captureStdout and outputJSON are process-wide.
	outputJSON = true
	t.Cleanup(func() { outputJSON = false })
	client := stubClient(t)

	out := captureStdout(t, func() { runMetadataAudit(client) })

	var got metadataJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode metadata report: %v\n%s", err, truncate(out))
	}
	for _, entry := range got.Entries {
		if entry.Category == constantIndexCategory && strings.HasPrefix(entry.Tool, "main.") {
			continue
		}
		t.Errorf("%s [%s]: %s", entry.Tool, entry.Category, entry.Detail)
	}
}

// unnamedFormatter is the output type of a formatter registered as a nil
// function, which is the one registration the registry records no name for.
type unnamedFormatter struct{ Name string }

// registerUnnamedFormatter registers that formatter once for the process.
func registerUnnamedFormatter() {
	unnamedFormatterOnce.Do(func() {
		var render func(unnamedFormatter) string
		toolutil.RegisterMarkdown(render)
	})
}

// unnamedFormatterOnce keeps the unnamed formatter to one registration.
var unnamedFormatterOnce sync.Once

// TestAuditResultEnvelopes_FormatterWithNoName_IsNamedByItsTypeAlone checks
// the envelope walk's name for a formatter the registry has no function name
// for: the type alone, never the type followed by an empty pair of
// parentheses, and its render's panic still listed in both states under it.
func TestAuditResultEnvelopes_FormatterWithNoName_IsNamedByItsTypeAlone(t *testing.T) {
	registerUnnamedFormatter()
	audit, _ := auditResultEnvelopes()

	typ := reflect.TypeFor[unnamedFormatter]().String()
	for _, state := range []string{"zero", "multi-page"} {
		t.Run(state, func(t *testing.T) {
			if !hasEntryContaining(audit.Panicked, typ+" ["+state+"]: ") {
				t.Errorf("panicked = %v, want %s listed under its type alone", audit.Panicked, typ)
			}
		})
	}
	if hasEntryContaining(audit.Panicked, typ+" (") {
		t.Errorf("panicked = %v, want no function name after %s", audit.Panicked, typ)
	}
}
