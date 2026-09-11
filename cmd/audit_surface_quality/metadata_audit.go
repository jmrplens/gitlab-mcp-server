// Metadata-quality audit functions (formerly audit_tools).

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Naming patterns for MCP tool name validation.
var (
	// toolNameRe matches individual tool names: gitlab_{word}_{word}[_{word}...].
	toolNameRe = regexp.MustCompile(`^gitlab_[a-z][a-z0-9]*(_[a-z0-9]+)+$`)
	// metaToolNameRe matches meta-tool names: gitlab_{word}[_{word}...].
	metaToolNameRe = regexp.MustCompile(`^gitlab_[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
)

// minDescLen is the minimum acceptable description length for an MCP tool.
const minDescLen = 20

const markdownFourColumnSeparator = "| --- | --- | --- | --- |\n"

// violation records a single metadata rule infraction for a tool.
//
// tool is the MCP tool name that violated the rule. category is the rule
// family used to group findings in the report (naming, description,
// annotations, etc.). detail is a human-readable explanation rendered
// verbatim into the Markdown table.
type violation struct {
	tool     string // MCP tool name that violated the rule.
	category string // Rule category (naming, description, annotations, etc.).
	detail   string // Human-readable explanation of the violation.
}

// runMetadataAudit runs the metadata-quality checks (naming, descriptions,
// annotations, schema shape, duplicates, register-meta inventory) and prints
// the report to stdout.
func runMetadataAudit(client *gitlabclient.Client) {
	individualTools := listTools(client, false)
	metaTools := listTools(client, true)

	violations := make([]violation, 0, len(individualTools)+len(metaTools))

	violations = append(violations, auditNaming(individualTools, toolNameRe, "individual")...)
	violations = append(violations, auditNaming(metaTools, metaToolNameRe, "meta")...)
	violations = append(violations, auditDescriptions(individualTools, "individual")...)
	violations = append(violations, auditDescriptions(metaTools, "meta")...)
	violations = append(violations, auditAnnotations(individualTools, "individual")...)
	violations = append(violations, auditAnnotations(metaTools, "meta")...)
	violations = append(violations, auditAnnotationTypes(individualTools)...)
	violations = append(violations, auditInputSchema(individualTools)...)
	violations = append(violations, auditAdditionalProperties(individualTools, "individual")...)
	violations = append(violations, auditAdditionalProperties(metaTools, "meta")...)
	violations = append(violations, auditDuplicates(individualTools, "individual")...)
	violations = append(violations, auditDuplicates(metaTools, "meta")...)

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "register meta audit skipped: %v\n", err)
	}
	var registerMetaDefinitions []registerMetaDefinition
	if root != "" {
		registerMetaDefinitions, err = auditRegisterMetaDefinitions(root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "register meta audit skipped: %v\n", err)
		}
		violations = append(violations, auditRegisterMetaDefinitionViolations(registerMetaDefinitions)...)
	}

	printMetadataReport(individualTools, metaTools, violations, registerMetaDefinitions, auditResultEnvelopes())
}

// envelopeAudit is what driving every registered Markdown formatter with a
// zero and a populated fixture found about the envelope its result travels
// in: a nil result, which reaches the dispatcher as no content block at all,
// and a content block with no Annotations, which reaches the client with no
// audience. It reports and does not gate in this layer: a formatter that
// renders nothing for a zero value is a guard, not a defect, and is counted
// apart from one that renders nothing for a populated value.
type envelopeAudit struct {
	Formatters           int      `json:"formatters"`
	NilOnZero            int      `json:"nil_on_zero"`
	NilOnPopulated       []string `json:"nil_on_populated"`
	Unannotated          []string `json:"unannotated"`
	Panicked             []string `json:"panicked"`
	RegistrationProblems []string `json:"registration_problems"`
}

// auditResultEnvelopes drives every registered formatter through
// MarkdownForResult, the way the dispatchers do, with the zero value and the
// populated fixture the runtime gate uses, and records what the envelope
// lacked.
func auditResultEnvelopes() envelopeAudit {
	audit := envelopeAudit{RegistrationProblems: toolutil.MarkdownRegistrationProblems()}
	for _, typ := range toolutil.RegisteredMarkdownTypes() {
		audit.Formatters++
		name := typ.String()
		if fn := toolutil.RegisteredMarkdownFormatterName(typ); fn != "" {
			name += " (" + fn + ")"
		}
		for _, state := range []testutil.FixtureState{testutil.FixtureZero, testutil.FixtureMultiPage} {
			result, panicked := renderEnvelope(typ, state)
			switch {
			case panicked != "":
				audit.Panicked = append(audit.Panicked, name+" ["+state.String()+"]: "+panicked)
			case result == nil && state == testutil.FixtureZero:
				audit.NilOnZero++
			case result == nil:
				audit.NilOnPopulated = append(audit.NilOnPopulated, name)
			default:
				for i, block := range result.Content {
					if !blockAnnotated(block) {
						audit.Unannotated = append(audit.Unannotated, fmt.Sprintf("%s [%s] block %d (%T)", name, state, i, block))
					}
				}
			}
		}
	}
	return audit
}

// renderEnvelope renders one type in one state, reporting a panic rather
// than ending the audit on it.
func renderEnvelope(typ reflect.Type, state testutil.FixtureState) (result *mcp.CallToolResult, panicked string) {
	defer func() {
		if r := recover(); r != nil {
			panicked = fmt.Sprint(r)
		}
	}()
	return toolutil.MarkdownForResult(testutil.FillFixture(typ, testutil.FixtureOptions{State: state}).Interface()), ""
}

// blockAnnotated reports whether a content block carries Annotations, for
// the block kinds a formatter writes.
func blockAnnotated(block mcp.Content) bool {
	switch b := block.(type) {
	case *mcp.TextContent:
		return b.Annotations != nil
	case *mcp.ImageContent:
		return b.Annotations != nil
	case *mcp.EmbeddedResource:
		return b.Annotations != nil
	default:
		return false
	}
}

// printResultEnvelopes writes the envelope section of the Markdown report:
// the counts, and the formatters whose populated render is nil, unannotated
// or a panic, so the list is the work list. The JSON view carries the same
// audit under the metadata report's "envelopes" key.
func printResultEnvelopes(audit envelopeAudit) {
	fmt.Printf("\n## Result Envelopes\n\n")
	fmt.Printf("Every registered Markdown formatter driven with a zero and a populated fixture. This section reports and does not gate: a nil render of a zero value is a guard, and is counted apart.\n\n")
	fmt.Printf("| Metric | Count |\n")
	fmt.Printf("| --- | ---: |\n")
	fmt.Printf("| Registered formatters | %d |\n", audit.Formatters)
	fmt.Printf("| Nil render of the zero value | %d |\n", audit.NilOnZero)
	fmt.Printf("| Nil render of the populated value | %d |\n", len(audit.NilOnPopulated))
	fmt.Printf("| Content blocks without Annotations | %d |\n", len(audit.Unannotated))
	fmt.Printf("| Formatters that panicked | %d |\n", len(audit.Panicked))
	fmt.Printf("| Registration problems | %d |\n\n", len(audit.RegistrationProblems))
	printEnvelopeList("Nil render of the populated value", audit.NilOnPopulated)
	printEnvelopeList("Content blocks without Annotations", audit.Unannotated)
	printEnvelopeList("Formatters that panicked", audit.Panicked)
	printEnvelopeList("Registration problems", audit.RegistrationProblems)
}

// printEnvelopeList writes one list of the envelope section, when it has
// entries.
func printEnvelopeList(title string, entries []string) {
	if len(entries) == 0 {
		return
	}
	fmt.Printf("### %s (%d)\n\n", title, len(entries))
	for _, entry := range entries {
		fmt.Printf("- `%s`\n", entry)
	}
	fmt.Println()
}

// auditNaming checks that every tool name matches the given regex pattern.
// kind is a label ("individual" or "meta") used in violation messages.
func auditNaming(tls []*mcp.Tool, re *regexp.Regexp, kind string) []violation {
	var vs []violation
	for _, t := range tls {
		if !re.MatchString(t.Name) {
			vs = append(vs, violation{t.Name, "naming", fmt.Sprintf("%s tool name does not match %s", kind, re.String())})
		}
	}
	return vs
}

// auditDescriptions flags tools whose description is shorter than minDescLen.
func auditDescriptions(tls []*mcp.Tool, kind string) []violation {
	var vs []violation
	for _, t := range tls {
		if len(t.Description) < minDescLen {
			vs = append(vs, violation{t.Name, "description", fmt.Sprintf("%s description too short (%d chars): %q", kind, len(t.Description), t.Description)})
		}
	}
	return vs
}

// auditAnnotations checks that every tool has non-nil Annotations and
// that ReadOnlyHint and DestructiveHint are not both true simultaneously.
func auditAnnotations(tls []*mcp.Tool, kind string) []violation {
	var vs []violation
	for _, t := range tls {
		if t.Annotations == nil {
			vs = append(vs, violation{t.Name, "annotations", kind + " tool has nil Annotations"})
			continue
		}
		if t.Annotations.ReadOnlyHint && t.Annotations.DestructiveHint != nil && *t.Annotations.DestructiveHint {
			vs = append(vs, violation{t.Name, "annotations", "ReadOnlyHint=true conflicts with DestructiveHint=true"})
		}
	}
	return vs
}

// auditAnnotationTypes verifies consistency between tool name suffixes and
// their annotation hints: read-like names should have ReadOnlyHint=true,
// delete-like names should have DestructiveHint=true.
func auditAnnotationTypes(tls []*mcp.Tool) []violation {
	var vs []violation
	for _, t := range tls {
		if t.Annotations == nil {
			continue
		}
		isRead := toolutil.IsReadToolName(t.Name)
		isDelete := toolutil.IsDeleteToolName(t.Name)

		if isRead && !t.Annotations.ReadOnlyHint {
			vs = append(vs, violation{t.Name, "annotation-type", "name suggests read-only but ReadOnlyHint is false"})
		}
		if isDelete {
			if t.Annotations.DestructiveHint == nil || !*t.Annotations.DestructiveHint {
				vs = append(vs, violation{t.Name, "annotation-type", "name suggests delete but DestructiveHint is not true"})
			}
		}
	}
	return vs
}

// auditInputSchema validates that each tool's InputSchema is a valid JSON
// Schema object with type "object" and at least one property defined.
func auditInputSchema(tls []*mcp.Tool) []violation {
	var vs []violation
	for _, t := range tls {
		schema, ok := t.InputSchema.(map[string]any)
		if !ok {
			vs = append(vs, violation{t.Name, "input-schema", "InputSchema is not a map"})
			continue
		}
		typ, _ := schema["type"].(string)
		if typ != "object" {
			vs = append(vs, violation{t.Name, "input-schema", fmt.Sprintf("InputSchema type=%q, expected \"object\"", typ)})
		}
	}
	return vs
}

// auditAdditionalProperties flags root tool input schemas that do not declare
// `additionalProperties: false`. Without this constraint an LLM that mistypes
// an argument name receives a confusing "missing parameter" error instead of
// the actionable "unknown property" diagnostic JSON Schema validation would
// produce. The lockdown middleware in toolutil.LockdownInputSchemas should
// keep this audit at zero violations.
func auditAdditionalProperties(tls []*mcp.Tool, kind string) []violation {
	var vs []violation
	for _, t := range tls {
		schema, ok := t.InputSchema.(map[string]any)
		if !ok {
			continue
		}
		if !isObjectSchema(schema) {
			continue
		}
		raw, present := schema["additionalProperties"]
		if !present {
			vs = append(vs, violation{
				t.Name, "additional-properties",
				kind + " tool inputSchema missing additionalProperties:false",
			})
			continue
		}
		if v, isBool := raw.(bool); !isBool || v {
			vs = append(vs, violation{
				t.Name, "additional-properties",
				fmt.Sprintf("%s tool inputSchema additionalProperties=%v, want false", kind, raw),
			})
		}
	}
	return vs
}

// isObjectSchema reports whether a JSON Schema node represents an object.
// A schema is considered object-shaped when its type is "object" or it
// declares a "properties" map; both shapes are valid for MCP tool inputs.
func isObjectSchema(node map[string]any) bool {
	if t, ok := node["type"].(string); ok {
		return t == "object"
	}
	_, hasProps := node["properties"]
	return hasProps
}

// auditDuplicates detects tools with the same name registered more than once.
func auditDuplicates(tls []*mcp.Tool, kind string) []violation {
	var vs []violation
	seen := make(map[string]bool, len(tls))
	for _, t := range tls {
		if seen[t.Name] {
			vs = append(vs, violation{t.Name, "duplicate", fmt.Sprintf("duplicate %s tool name", kind)})
		}
		seen[t.Name] = true
	}
	return vs
}

// printMetadataReport writes the full markdown audit report to stdout,
// including summary counts, violations grouped by category, a complete
// listing of all individual and meta-tools with their annotations, and the
// result-envelope section.
func printMetadataReport(individual, meta []*mcp.Tool, vs []violation, registerMetaDefinitions []registerMetaDefinition, envelopes envelopeAudit) {
	if outputJSON {
		report := struct {
			View            string        `json:"view"`
			IndividualTools int           `json:"individual_tools"`
			MetaTools       int           `json:"meta_tools"`
			Violations      int           `json:"violations"`
			Entries         []jsonEntry   `json:"entries"`
			Envelopes       envelopeAudit `json:"envelopes"`
		}{"metadata", len(individual), len(meta), len(vs), toEntries(vs), envelopes}
		if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "encode json: %v\n", err)
		}
		return
	}
	defer printResultEnvelopes(envelopes)
	now := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("# MCP Tool Metadata Audit Report\n\n")
	fmt.Printf("Generated: %s\n\n", now)
	fmt.Printf("## Summary\n\n")
	fmt.Printf("| Metric | Count |\n")
	fmt.Printf("| --- | --- |\n")
	fmt.Printf("| Individual tools | %d |\n", len(individual))
	fmt.Printf("| Meta-tools | %d |\n", len(meta))
	fmt.Printf("| Total violations | %d |\n\n", len(vs))
	printRegisterMetaDefinitions(registerMetaDefinitions)

	if len(vs) == 0 {
		fmt.Println("**No violations found.**")
		return
	}

	// Group by category
	categories := make(map[string][]violation)
	for _, v := range vs {
		categories[v.category] = append(categories[v.category], v)
	}

	fmt.Printf("## Violations by Category\n\n")
	for cat, catVs := range categories {
		fmt.Printf("### %s (%d)\n\n", cat, len(catVs))
		fmt.Printf("| Tool | Detail |\n")
		fmt.Printf("| --- | --- |\n")
		for _, v := range catVs {
			fmt.Printf("| `%s` | %s |\n", v.tool, v.detail)
		}
		fmt.Println()
	}

	fmt.Printf("## All Tools\n\n")
	fmt.Printf("### Individual Tools (%d)\n\n", len(individual))
	fmt.Printf("| # | Name | Description (first 60 chars) | Annotations |\n")
	fmt.Print(markdownFourColumnSeparator)
	for i, t := range individual {
		desc := t.Description
		if len(desc) > 60 {
			desc = desc[:60] + "..."
		}
		ann := "nil"
		if t.Annotations != nil {
			ann = fmt.Sprintf("RO=%v D=%v I=%v OW=%v",
				t.Annotations.ReadOnlyHint,
				ptrBool(t.Annotations.DestructiveHint),
				t.Annotations.IdempotentHint,
				ptrBool(t.Annotations.OpenWorldHint))
		}
		fmt.Printf("| %d | `%s` | %s | %s |\n", i+1, t.Name, desc, ann)
	}

	fmt.Printf("\n### Meta-Tools (%d)\n\n", len(meta))
	fmt.Printf("| # | Name | Description (first 60 chars) | Annotations |\n")
	fmt.Print(markdownFourColumnSeparator)
	for i, t := range meta {
		desc := t.Description
		if len(desc) > 60 {
			desc = desc[:60] + "..."
		}
		ann := "nil"
		if t.Annotations != nil {
			ann = fmt.Sprintf("RO=%v D=%v I=%v OW=%v",
				t.Annotations.ReadOnlyHint,
				ptrBool(t.Annotations.DestructiveHint),
				t.Annotations.IdempotentHint,
				ptrBool(t.Annotations.OpenWorldHint))
		}
		fmt.Printf("| %d | `%s` | %s | %s |\n", i+1, t.Name, desc, ann)
	}
}

func printRegisterMetaDefinitions(definitions []registerMetaDefinition) {
	if len(definitions) == 0 {
		return
	}
	referenced := 0
	delegated := 0
	unexpected := unexpectedRegisterMetaDefinitions(definitions)
	for _, definition := range definitions {
		if definition.Referenced {
			referenced++
		}
		if isDelegatedRegisterMetaDefinition(definition) {
			delegated++
		}
	}
	fmt.Printf("## RegisterMeta Definition Inventory\n\n")
	fmt.Printf("This section is enforced. Package-level `RegisterMeta` definitions are no longer an approved catalog-first runtime pattern.\n\n")
	fmt.Printf("| Metric | Count |\n")
	fmt.Printf("| --- | ---: |\n")
	fmt.Printf("| Package-level RegisterMeta definitions | %d |\n", len(definitions))
	fmt.Printf("| Referenced from central meta hub | %d |\n", referenced)
	fmt.Printf("| Approved delegated definitions | %d |\n", delegated)
	fmt.Printf("| Unexpected definitions | %d |\n\n", len(unexpected))
	fmt.Printf("| Status | Package | File | Meta tool names |\n")
	fmt.Print(markdownFourColumnSeparator)
	for _, definition := range definitions {
		status := "unexpected"
		if isDelegatedRegisterMetaDefinition(definition) {
			status = "delegated"
		}
		toolNames := strings.Join(definition.ToolNames, ", ")
		if toolNames == "" {
			toolNames = "-"
		}
		fmt.Printf("| %s | `%s` | `%s` | `%s` |\n", status, definition.Package, definition.File, toolNames)
	}
	fmt.Println()
}

// ptrBool formats a *bool as "true", "false", or "nil".
func ptrBool(p *bool) string {
	if p == nil {
		return "nil"
	}
	if *p {
		return "true"
	}
	return "false"
}
