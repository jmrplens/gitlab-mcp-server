// metadata_audit_test.go provides automated validation of all MCP tool and
// meta-tool metadata: naming conventions, description quality, annotation
// correctness, InputSchema structure, and action enum constraints.
//
// Run with: go test ./internal/tools/ -run TestMetadataAudit -count=1 -v.
package tools

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolNameRe matches the gitlab_{domain}_{action}[_{detail}...] snake_case convention.
// Segments may start with a digit to support well-known acronyms like 2fa.
var toolNameRe = regexp.MustCompile(`^gitlab_[a-z][a-z0-9]*(_[a-z0-9][a-z0-9]*)+$`)

// metaToolNameRe matches meta-tool names like gitlab_{domain}[_{subdomain}].
var metaToolNameRe = regexp.MustCompile(`^gitlab_[a-z][a-z0-9]*(_[a-z0-9][a-z0-9]*)*$`)

const auditMinDescLen = 20

// auditHandler returns an HTTP handler that responds to all GitLab API
// requests with minimal valid JSON. Audit tests only need to register
// tools and inspect their metadata — they do not call tool handlers.
func auditHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})
}

// readSuffixes are tool name endings that indicate read-only operations.
// Suffix matching avoids false positives with compound resource names
// like "board_list" where "list" is part of the resource, not the action.
var readSuffixes = []string{
	"_list", "_lists", "_get", "_search",
	"_latest", "_blame", "_raw", "_diff", "_refs",
	"_statuses", "_signature", "_languages", "_statistics",
}

// isReadToolName returns true if the tool name ends with a suffix that
// indicates a read-only operation (list, get, search, etc.).
func isReadToolName(name string) bool {
	for _, sfx := range readSuffixes {
		if strings.HasSuffix(name, sfx) {
			return true
		}
	}
	return false
}

// isDeleteToolName returns true if the tool name ends with "_delete"
// or contains "delete" as an action word (e.g., gitlab_delete_terraform_state).
func isDeleteToolName(name string) bool {
	if strings.HasSuffix(name, "_delete") {
		return true
	}
	return slices.Contains(strings.Split(name, "_"), "delete")
}

// knownNamingExceptions lists tools whose names violate the convention
// but are tracked for remediation in a later audit phase.
var knownNamingExceptions = map[string]string{}

// ---------- Audit helper functions ----------.

// checkToolAnnotations validates that a tool's annotations are properly set:
// non-nil, OpenWorldHint=true, DestructiveHint present, no contradictory flags.
func checkToolAnnotations(t *testing.T, ann *mcp.ToolAnnotations) {
	t.Helper()
	if ann == nil {
		t.Fatal("annotations are nil")
	}
	if ann.OpenWorldHint == nil {
		t.Error("OpenWorldHint is nil (should be *bool)")
	} else if !*ann.OpenWorldHint {
		t.Error("OpenWorldHint should be true for GitLab tools")
	}
	if ann.DestructiveHint == nil {
		t.Error("DestructiveHint is nil (should be *bool)")
	}
	if ann.ReadOnlyHint && ann.DestructiveHint != nil && *ann.DestructiveHint {
		t.Error("ReadOnlyHint=true but DestructiveHint=true — contradictory")
	}
	if ann.ReadOnlyHint && !ann.IdempotentHint {
		t.Error("ReadOnlyHint=true but IdempotentHint=false — read-only tools should be idempotent")
	}
}

// checkToolOperationType validates that tool names match their annotation hints:
// read-suffix tools should be ReadOnly, delete-suffix tools should be Destructive.
func checkToolOperationType(t *testing.T, name string, ann *mcp.ToolAnnotations) {
	t.Helper()
	if isReadToolName(name) {
		if !ann.ReadOnlyHint {
			t.Errorf("name contains read keyword (list/get/search) but ReadOnlyHint=false")
		}
	}
	if isDeleteToolName(name) {
		if ann.DestructiveHint == nil || !*ann.DestructiveHint {
			t.Errorf("name contains 'delete' but DestructiveHint is not true")
		}
	}
}

// checkActionEnumValues validates that an action property has a valid enum
// constraint with non-empty string values.
func checkActionEnumValues(t *testing.T, actionProp map[string]any) {
	t.Helper()
	enumVal, hasEnum := actionProp["enum"]
	if !hasEnum {
		t.Fatal("action property missing 'enum' constraint")
	}
	enumList, ok := enumVal.([]any)
	if !ok {
		t.Fatalf("action enum is not []any, got %T", enumVal)
	}
	if len(enumList) == 0 {
		t.Error("action enum is empty")
	}
	var s string
	for i, v := range enumList {
		s, ok = v.(string)
		if !ok {
			t.Errorf("enum[%d] is not string, got %T", i, v)
		} else if s == "" {
			t.Errorf("enum[%d] is empty string", i)
		}
	}
}

// checkSchemaConstraints validates that 'action' is in required fields and
// additionalProperties is false.
func checkSchemaConstraints(t *testing.T, schema map[string]any) {
	t.Helper()
	required, _ := schema["required"].([]any)
	hasActionRequired := false
	for _, r := range required {
		if r == "action" {
			hasActionRequired = true
			break
		}
	}
	if !hasActionRequired {
		t.Error("'action' not in required fields")
	}
	if ap, hasAP := schema["additionalProperties"]; hasAP {
		if apBool, ok := ap.(bool); ok && apBool {
			t.Error("additionalProperties should be false")
		}
	}
}

// checkMetaToolActionEnum validates the action enum schema for a meta-tool.
func checkMetaToolActionEnum(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("InputSchema is not map[string]any, got %T", tool.InputSchema)
	}

	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		t.Fatal("InputSchema missing 'properties'")
	}

	actionProp, _ := props["action"].(map[string]any)
	if actionProp == nil {
		t.Skipf("tool %s has no 'action' property — not a domain meta-tool", tool.Name)
	}

	checkActionEnumValues(t, actionProp)
	checkSchemaConstraints(t, schema)
}

// auditToolMetadata returns metadata validation flags and a list of issues for a tool.
func auditToolMetadata(tool *mcp.Tool) (nameOK, descOK, annOK, schemaOK bool, issues []string) {
	nameOK = toolNameRe.MatchString(tool.Name)
	descOK = len(tool.Description) >= auditMinDescLen
	annOK = tool.Annotations != nil &&
		tool.Annotations.OpenWorldHint != nil &&
		tool.Annotations.DestructiveHint != nil
	if schema, ok := tool.InputSchema.(map[string]any); ok {
		_, hasProps := schema["properties"]
		schemaType, _ := schema["type"].(string)
		schemaOK = schemaType == "object" && hasProps
	}
	if !nameOK {
		issues = append(issues, "name")
	}
	if !descOK {
		issues = append(issues, "desc")
	}
	if !annOK {
		issues = append(issues, "annotations")
	}
	if !schemaOK {
		issues = append(issues, "schema")
	}
	return
}

// auditMetaToolMetadata returns metadata validation flags for a meta-tool.
func auditMetaToolMetadata(tool *mcp.Tool) (annOK, enumOK bool, actionCount int) {
	annOK = tool.Annotations != nil &&
		tool.Annotations.OpenWorldHint != nil &&
		tool.Annotations.DestructiveHint != nil
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		return
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	action, ok := props["action"].(map[string]any)
	if !ok {
		return
	}
	enumList, ok := action["enum"].([]any)
	if ok {
		enumOK = len(enumList) > 0
		actionCount = len(enumList)
	}
	return
}

// ---------- Individual tool metadata audit ----------.

// TestMetadataAudit_ToolNamingConvention verifies the behavior of metadata audit tool naming convention.
func TestMetadataAudit_ToolNamingConvention(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if reason, isException := knownNamingExceptions[tool.Name]; isException {
				t.Skipf("known exception: %s", reason)
			}
			if !toolNameRe.MatchString(tool.Name) {
				t.Errorf("name %q does not match gitlab_{action}_{resource} snake_case pattern", tool.Name)
			}
		})
	}
}

// TestMetadataAudit_ToolDescriptions verifies the behavior of metadata audit tool descriptions.
func TestMetadataAudit_ToolDescriptions(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Description == "" {
				t.Error("description is empty")
				return
			}
			if len(tool.Description) < auditMinDescLen {
				t.Errorf("description too short (%d chars, minimum %d): %q",
					len(tool.Description), auditMinDescLen, tool.Description)
			}
		})
	}
}

// TestMetadataAudit_ToolAnnotations verifies the behavior of metadata audit tool annotations.
func TestMetadataAudit_ToolAnnotations(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			checkToolAnnotations(t, tool.Annotations)
		})
	}
}

// TestMetadataAudit_ToolAnnotationOperationType verifies the behavior of metadata audit tool annotation operation type.
func TestMetadataAudit_ToolAnnotationOperationType(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			checkToolOperationType(t, tool.Name, tool.Annotations)
		})
	}
}

// TestMetadataAudit_ToolInputSchema verifies the behavior of metadata audit tool input schema.
func TestMetadataAudit_ToolInputSchema(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("InputSchema is not map[string]any, got %T", tool.InputSchema)
			}

			schemaType, _ := schema["type"].(string)
			if schemaType != "object" {
				t.Errorf("InputSchema type = %q, want \"object\"", schemaType)
			}

			// Tools with no parameters (e.g., gitlab_get_appearance)
			// generate schemas without 'properties' — this is valid.
			if _, hasProps := schema["properties"]; !hasProps {
				t.Logf("INFO: schema has no properties (zero-parameter tool)")
			}
		})
	}
}

// ---------- Meta-tool metadata audit ----------.

// TestMetadataAudit_MetaToolNaming verifies the behavior of metadata audit meta tool naming.
func TestMetadataAudit_MetaToolNaming(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if !metaToolNameRe.MatchString(tool.Name) {
				t.Errorf("meta-tool name %q does not match gitlab_{domain} pattern", tool.Name)
			}
		})
	}
}

// TestMetadataAudit_MetaToolDescriptions verifies the behavior of metadata audit meta tool descriptions.
func TestMetadataAudit_MetaToolDescriptions(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Description == "" {
				t.Error("description is empty")
				return
			}
			if len(tool.Description) < auditMinDescLen {
				t.Errorf("description too short (%d chars, minimum %d)",
					len(tool.Description), auditMinDescLen)
			}
		})
	}
}

// TestMetadataAudit_MetaToolAnnotations verifies the behavior of metadata audit meta tool annotations.
func TestMetadataAudit_MetaToolAnnotations(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Annotations == nil {
				t.Fatal("annotations are nil")
			}
			if tool.Annotations.OpenWorldHint == nil {
				t.Error("OpenWorldHint is nil (should be *bool)")
			} else if !*tool.Annotations.OpenWorldHint {
				t.Error("OpenWorldHint should be true for GitLab meta-tools")
			}
			if tool.Annotations.DestructiveHint == nil {
				t.Error("DestructiveHint is nil (should be *bool)")
			}
		})
	}
}

// TestMetadataAudit_MetaToolActionEnum verifies the behavior of metadata audit meta tool action enum.
func TestMetadataAudit_MetaToolActionEnum(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			checkMetaToolActionEnum(t, tool)
		})
	}
}

// ---------- Cross-validation ----------.

// TestMetadataAudit_NoDuplicateToolNames verifies the behavior of metadata audit no duplicate tool names.
func TestMetadataAudit_NoDuplicateToolNames(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	seen := make(map[string]int, len(result.Tools))
	for _, tool := range result.Tools {
		seen[tool.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("duplicate tool name %q registered %d times", name, count)
		}
	}
}

// TestMetadataAudit_NoDuplicateMetaToolNames verifies the behavior of metadata audit no duplicate meta tool names.
func TestMetadataAudit_NoDuplicateMetaToolNames(t *testing.T) {
	session := newMetaMCPSession(t, auditHandler(), true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	seen := make(map[string]int, len(result.Tools))
	for _, tool := range result.Tools {
		seen[tool.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("duplicate meta-tool name %q registered %d times", name, count)
		}
	}
}

// ---------- Report generator ----------.

// TestMetadataAudit_Report generates a summary report of all tool metadata
// for manual review. Run with -v to see the report.
func TestMetadataAudit_Report(t *testing.T) {
	session := newMCPSession(t, auditHandler())
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	var violations int
	t.Logf("\n## Individual Tool Metadata Report (%d tools)\n", len(result.Tools))
	t.Logf("| Tool | Name OK | Desc OK | Ann OK | Schema OK | Issues |")
	t.Logf("|------|---------|---------|--------|-----------|--------|")

	for _, tool := range result.Tools {
		nameOK, descOK, annOK, schemaOK, issues := auditToolMetadata(tool)
		if len(issues) > 0 {
			violations++
			t.Logf("| %s | %s | %s | %s | %s | %s |",
				tool.Name,
				boolMark(nameOK), boolMark(descOK), boolMark(annOK), boolMark(schemaOK),
				strings.Join(issues, ", "))
		}
	}

	if violations == 0 {
		t.Logf("\n✓ All %d individual tools pass basic metadata checks.", len(result.Tools))
	} else {
		t.Logf("\n✗ %d / %d tools have metadata issues.", violations, len(result.Tools))
	}

	metaSession := newMetaMCPSession(t, auditHandler(), true)
	metaResult, err := metaSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}

	t.Logf("\n## Meta-Tool Metadata Report (%d tools)\n", len(metaResult.Tools))
	t.Logf("| Meta-Tool | Actions | Ann OK | Enum OK |")
	t.Logf("|-----------|---------|--------|---------|")

	for _, tool := range metaResult.Tools {
		annOK, enumOK, actionCount := auditMetaToolMetadata(tool)
		t.Logf("| %s | %d | %s | %s |",
			tool.Name, actionCount, boolMark(annOK), boolMark(enumOK))
	}
}

// boolMark is an internal helper for the tools package.
func boolMark(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}
