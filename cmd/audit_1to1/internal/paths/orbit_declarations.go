// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import "sort"

// orbitDeclaration records why a disagreement between an Orbit output type and
// its recorded answers is not a defect.
//
// It is [shapeDeclaration]'s counterpart for the Orbit record, kept in a table
// of its own because its findings are keyed by where they sit in the answer
// rather than by the type declaring them: StatusComponent is published both at
// the top of the status output, where GitLab never sends it, and under system,
// where it does, and one declaration keyed on the type would excuse both.
//
// The bar is the one every declaration table here holds: the handler in
// GitLab's source that builds the answer, the code in the Orbit service or in
// Workhorse that decides the key, or, for a field this server fills itself,
// the code here that fills it. A declaration that matches no finding is
// reported stale, so the excuse cannot outlive the thing it excused.
type orbitDeclaration struct {
	// Package owns the output type, repository relative.
	Package string
	// Output is the type the recorded calls returned.
	Output string
	// Path is where the field sits under the output, element markers elided,
	// except for a kind recorded on the elements of a list, whose path ends in
	// one marker per depth, the way the finding spells it.
	Path string
	// Direction is the question the finding answers.
	Direction orbitDirection
	// Category says what kind of disagreement this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it still
	// holds.
	Reason string
}

// orbitDirection names the question an Orbit finding answers, and so which
// list of the report it sits in.
type orbitDirection string

const (
	// orbitPublished is a field a type publishes that no recorded answer
	// carries.
	orbitPublished orbitDirection = "published"
	// orbitSent is a key a recorded answer carries that no field publishes.
	orbitSent orbitDirection = "sent"
	// orbitKind is a kind a recorded answer carries at a field that the
	// published field's Go type does not carry as it came.
	orbitKind orbitDirection = "kind"
	// orbitDecoderKind is a kind a recorded answer carries at a field of the
	// client-go struct the handler decodes it into, that the struct's Go type
	// does not decode.
	orbitDecoderKind orbitDirection = "decoder-kind"
)

// Orbit declaration categories. The category for a key client-go promoted to
// where GitLab never sent it retired with GET /orbit/status, whose flat shape
// client-go's OrbitStatus promoted; the status is read from GraphQL since
// 2026-10-29.
const (
	// categoryOrbitConditional is a key GitLab sends only under a condition the
	// recording did not meet and could not, from the Orbit service's or
	// GitLab's own code.
	categoryOrbitConditional = "sent-only-under-a-condition-the-recording-did-not-meet"
	// categoryOrbitWholeBody is a field that carries a body GitLab sends
	// without keys of its own: a verbatim document, or text.
	categoryOrbitWholeBody = "this-server-carries-a-body-gitlab-sends-without-keys"
)

// orbitPkg is the package every Orbit declaration is about.
const orbitPkg = toolsDir + "/orbit"

// reasonToolText answers the llm text of an Orbit MCP command.
const reasonToolText = "the llm format of an Orbit MCP command is the text block of the tools/call result (ee/lib/api/orbit/mcp.rb " +
	"dispatches tools/call to the command, whose llm answer is TOON text), recorded at result.content[].text as text with no keys. " +
	"The handler (callMCPTool in internal/tools/orbit/mcp.go) publishes the text as formatted_text, the name the other Orbit answers give their llm text."

// reasonEndpointNames answers the pair of node kinds of a schema edge.
const reasonEndpointNames = "GitLab's GraphQL OrbitSchemaEdgeEndpoint answers each end of an edge as an OrbitSchemaNode, of which querySchema " +
	"(internal/tools/orbit/graphql.go) asks the name alone, as source { name } and target { name }; convertSchema publishes the two names " +
	"as source_type and target_type, the names SchemaEdgeVariant has carried since the REST schema route answered them flat."

// reasonInputSchema answers the input schema of an Orbit MCP tool.
const reasonInputSchema = "MCP's tools/list answers each tool's input JSON Schema as inputSchema, the key the MCP specification names; Tools " +
	"(internal/tools/orbit/mcp.go) publishes it as parameters, the name the REST manifest GET /orbit/tools gave it until " +
	"gitlab-org/gitlab!265998 removed the route, so the output keeps its field."

// declaredOrbitFields holds every Orbit finding that is not a defect, each
// with the evidence that settles it.
var declaredOrbitFields = []orbitDeclaration{
	{Package: orbitPkg, Output: "DSLOutput", Path: "formatted_text", Direction: orbitPublished, Category: categoryOrbitWholeBody, Reason: reasonToolText},
	{Package: orbitPkg, Output: "SchemaOutput", Path: "formatted_text", Direction: orbitPublished, Category: categoryOrbitWholeBody, Reason: reasonToolText},
	{
		Package: orbitPkg, Output: "SchemaOutput", Path: "domains", Direction: orbitPublished, Category: categoryServerDerived,
		Reason: "GitLab's GraphQL OrbitSchema lists no domains, and each OrbitSchemaNode names its domain (domain { name }); convertSchema " +
			"(internal/tools/orbit/graphql.go) groups the node names under each domain in the order GitLab lists the nodes, which is the " +
			"list the REST schema route answered as domains[].node_names until gitlab-org/gitlab!265633 removed it.",
	},
	{Package: orbitPkg, Output: "SchemaOutput", Path: "edges.variants.source_type", Direction: orbitPublished, Category: categoryServerDerived, Reason: reasonEndpointNames},
	{Package: orbitPkg, Output: "SchemaOutput", Path: "edges.variants.target_type", Direction: orbitPublished, Category: categoryServerDerived, Reason: reasonEndpointNames},
	{Package: orbitPkg, Output: "SchemaOutput", Path: "edges.variants.source", Direction: orbitSent, Category: categoryServerDerived, Reason: reasonEndpointNames},
	{Package: orbitPkg, Output: "SchemaOutput", Path: "edges.variants.target", Direction: orbitSent, Category: categoryServerDerived, Reason: reasonEndpointNames},
	{
		Package: orbitPkg, Output: "SchemaOutput", Path: "nodes.domain", Direction: orbitKind, Category: categoryServerDerived,
		Reason: "GitLab's GraphQL OrbitSchemaNode.domain is an OrbitSchemaDomain object whose one field is its name; convertSchema " +
			"(internal/tools/orbit/graphql.go) publishes the name as the string SchemaNode.domain.",
	},
	{Package: orbitPkg, Output: "ToolsOutput", Path: "tools.parameters", Direction: orbitPublished, Category: categoryServerDerived, Reason: reasonInputSchema},
	{Package: orbitPkg, Output: "ToolsOutput", Path: "tools.inputSchema", Direction: orbitSent, Category: categoryServerDerived, Reason: reasonInputSchema},
	{
		Package: orbitPkg, Output: "QueryOutput", Path: "raw_query_strings", Direction: orbitPublished, Category: categoryOrbitConditional,
		Reason: "workhorse/internal/orbit/sendquery.go writes the query answer as queryResponse, whose RawQueryStrings is " +
			"`json:\"raw_query_strings,omitempty\"`, and the Knowledge Graph service fills it only when the query sets " +
			"options.include_debug_sql and the caller may see debug SQL (crates/query-engine/shared/src/stages/output.rs in " +
			"gitlab-org/orbit/knowledge-graph, `requested && can_see_debug_sql(ctx)`). The recorded query asks for no debug SQL.",
	},
	{
		Package: orbitPkg, Output: "QueryOutput", Path: "formatted_text", Direction: orbitPublished, Category: categoryOrbitWholeBody,
		Reason: "the llm and gql answers to POST /orbit/query are text/plain, both written by writeLLMResultResponse in " +
			"workhorse/internal/orbit/sendquery.go, so they have no keys at all. Query reads either through client-go's QueryRaw and " +
			"publishes the text as formatted_text, the name the other Orbit answers give their llm text.",
	},
	{
		Package: orbitPkg, Output: "DSLOutput", Path: "response_format", Direction: orbitPublished, Category: categoryServerDerived,
		Reason: "the response format the caller asked for, echoed by DSL (internal/tools/orbit/mcp.go) so a reader can " +
			"tell the query command read as JSON from its compact llm text; list_commands of the Orbit MCP endpoint answers " +
			"with the commands alone.",
	},
}

// declaredOrbitField finds the declaration covering one finding.
func declaredOrbitField(finding OrbitField, direction orbitDirection) (orbitDeclaration, bool) {
	for _, declaration := range declaredOrbitFields {
		if declaration.covers(finding, direction) {
			return declaration, true
		}
	}
	return orbitDeclaration{}, false
}

// covers reports whether this declaration accounts for one finding.
func (d orbitDeclaration) covers(finding OrbitField, direction orbitDirection) bool {
	return d.Package == finding.Package && d.Output == finding.Output && d.Path == finding.Path && d.Direction == direction
}

// key names one declaration in a report.
func (d orbitDeclaration) key() string {
	return d.Package + "." + d.Output + " " + d.Path + " (" + string(d.Direction) + ")"
}

// classified returns the check with the declaration that accounts for each
// finding of the three questions, both halves of the third, attached, and the
// declarations that accounted for none named, sorted and nil when there are
// none. The fields whose type could not be judged are no finding, so nothing
// declares them.
func (c OrbitCheck) classified() OrbitCheck {
	used := map[string]bool{}
	c.Unpublished = classifyOrbitDirection(c.Unpublished, orbitPublished, used)
	c.Unsurfaced = classifyOrbitDirection(c.Unsurfaced, orbitSent, used)
	c.Mismatched = classifyOrbitDirection(c.Mismatched, orbitKind, used)
	c.DecoderMismatched = classifyOrbitDirection(c.DecoderMismatched, orbitDecoderKind, used)
	c.UnusedDeclarations = nil
	for _, declaration := range declaredOrbitFields {
		if !used[declaration.key()] {
			c.UnusedDeclarations = append(c.UnusedDeclarations, declaration.key())
		}
	}
	sort.Strings(c.UnusedDeclarations)
	return c
}

// classifyOrbitDirection annotates one direction's findings, recording in used
// the declarations that accounted for something.
func classifyOrbitDirection(found []OrbitField, direction orbitDirection, used map[string]bool) []OrbitField {
	var classified []OrbitField
	for _, finding := range found {
		if declaration, ok := declaredOrbitField(finding, direction); ok {
			finding.Category = declaration.Category
			finding.Reason = declaration.Reason
			used[declaration.key()] = true
		}
		classified = append(classified, finding)
	}
	return classified
}
