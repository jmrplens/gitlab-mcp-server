// metadata_audit_test.go contains focused tests for the metadata audit
// helpers. Tests cover the small pure-function audits (naming, descriptions,
// annotations, schema validity) without spinning up the full MCP server.
package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestAuditNaming_FlagsMismatchedToolNames verifies auditNaming reports a
// violation per tool name that does not match the supplied pattern.
func TestAuditNaming_FlagsMismatchedToolNames(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "gitlab_project_get"},
		{Name: "InvalidName"},
		{Name: "gitlabProjectGet"},
	}

	got := auditNaming(tools, toolNameRe, "individual")
	if len(got) != 2 {
		t.Fatalf("auditNaming() returned %d violations, want 2", len(got))
	}
	for _, v := range got {
		if v.category != "naming" {
			t.Errorf("category = %q, want naming", v.category)
		}
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
// reports tools whose description is shorter than minDescLen.
func TestAuditDescriptions_FlagsShortDescription(t *testing.T) {
	short := strings.Repeat("a", minDescLen-1)
	tools := []*mcp.Tool{
		{Name: "ok", Description: strings.Repeat("a", minDescLen)},
		{Name: "short", Description: short},
	}

	got := auditDescriptions(tools, "individual")
	if len(got) != 1 {
		t.Fatalf("auditDescriptions() = %d violations, want 1", len(got))
	}
	if got[0].tool != "short" {
		t.Fatalf("tool = %q, want short", got[0].tool)
	}
}

// TestAuditAnnotations_DetectsNilAndConflictingHints verifies the annotation
// audit flags nil annotations and conflicting ReadOnly/Destructive hints.
func TestAuditAnnotations_DetectsNilAndConflictingHints(t *testing.T) {
	destr := true
	tools := []*mcp.Tool{
		{Name: "nil_ann"},
		{Name: "conflict", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &destr}},
		{Name: "ok", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destr}},
	}

	got := auditAnnotations(tools, "individual")
	if len(got) != 2 {
		t.Fatalf("auditAnnotations() = %d violations, want 2: %+v", len(got), got)
	}
}

// TestAuditAnnotationTypes_ValidatesReadAndDeleteNames verifies the audit
// reports tools whose name suffix should match their annotation hint.
func TestAuditAnnotationTypes_ValidatesReadAndDeleteNames(t *testing.T) {
	// Read name without ReadOnlyHint.
	tools := []*mcp.Tool{
		{Name: "gitlab_project_list", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false}},
		// Delete name without DestructiveHint=true.
		{Name: "gitlab_project_delete", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: nil}},
		// Compliant read tool.
		{Name: "gitlab_project_get", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
	}

	got := auditAnnotationTypes(tools)
	if len(got) < 2 {
		t.Fatalf("auditAnnotationTypes() = %d violations, want >= 2: %+v", len(got), got)
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
// reports tools whose schema is missing or not typed as object.
func TestAuditInputSchema_RequiresObjectType(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "wrong_type", InputSchema: map[string]any{"type": "string"}},
		{Name: "no_map", InputSchema: "not a map"},
		{Name: "ok", InputSchema: map[string]any{"type": "object"}},
	}

	got := auditInputSchema(tools)
	if len(got) != 2 {
		t.Fatalf("auditInputSchema() = %d, want 2: %+v", len(got), got)
	}
}

// TestAuditAdditionalProperties_RequiresFalseConstraint verifies the audit
// reports schemas without additionalProperties=false.
func TestAuditAdditionalProperties_RequiresFalseConstraint(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "missing", InputSchema: map[string]any{"type": "object"}},
		{Name: "true_value", InputSchema: map[string]any{"type": "object", "additionalProperties": true}},
		{Name: "ok", InputSchema: map[string]any{"type": "object", "additionalProperties": false}},
	}

	got := auditAdditionalProperties(tools, "individual")
	if len(got) != 2 {
		t.Fatalf("auditAdditionalProperties() = %d, want 2: %+v", len(got), got)
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
	if len(got) != 1 {
		t.Fatalf("auditDuplicates() = %d, want 1", len(got))
	}
	if got[0].tool != "gitlab_x" {
		t.Fatalf("duplicate tool = %q, want gitlab_x", got[0].tool)
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

	audit := auditResultEnvelopes()

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

// TestRenderEnvelope_Panic_IsReportedRatherThanRaised checks the recovery
// the audit relies on to finish a run whose one formatter panics.
func TestRenderEnvelope_Panic_IsReportedRatherThanRaised(t *testing.T) {
	type panicking struct{ Name string }
	toolutil.RegisterMarkdown(func(panicking) string { panic("no render") })

	result, panicked := renderEnvelope(reflect.TypeFor[panicking](), testutil.FixtureMultiPage)

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
// writes the no-violations message when there are no findings.
func TestPrintReport_EmptyViolationsWritesNoViolationsMessage(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout.
	output := captureStdout(t, func() {
		printMetadataReport(
			[]*mcp.Tool{{Name: "gitlab_x"}},
			[]*mcp.Tool{{Name: "gitlab_y"}},
			nil,
			nil,
			envelopeAudit{Formatters: 3, NilOnZero: 1},
		)
	})

	for _, want := range []string{
		"# MCP Tool Metadata Audit Report",
		"| Individual tools | 1 |",
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
