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
	// Path is where the field sits under the output, element markers elided.
	Path string
	// Sent is false for a field published that no answer carried, and true
	// for a key an answer carried that no field publishes.
	Sent bool
	// Category says what kind of disagreement this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it still
	// holds.
	Reason string
}

// Orbit declaration categories. What GitLab.com never sends is the new
// category the Orbit record needed: the two tables before it answer a record
// that is incomplete, and this one answers a client library that publishes
// more than the server sends.
const (
	// categoryOrbitNeverSent is a key the Orbit route never sends at the place
	// the type publishes it, because client-go copies it there from where
	// GitLab does send it, for the callers of an older answer that was flat.
	// The values are GitLab's; the place is client-go's.
	categoryOrbitNeverSent = "gitlab-never-sends-the-key-client-go-promotes"
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

// reasonStatusPromoted answers the five flat keys of the status output.
const reasonStatusPromoted = "ee/lib/api/orbit/data.rb answers GET /orbit/status with `present({ user: { available: available }, system: system })`, " +
	"and system is the hash get_cluster_health builds (ee/lib/analytics/knowledge_graph/grpc_client.rb): status, timestamp, version " +
	"and components, or formatted_text for the llm format. Nothing is sent at the top beside user and system, which the recording " +
	"confirms for both formats. client-go's OrbitStatus.UnmarshalJSON copies the system object's fields to the top for the callers " +
	"of the flat shape Orbit used to answer with, and StatusOutput publishes what client-go decoded, so the values are GitLab's and " +
	"each also appears under system, where the recording has it."

// declaredOrbitFields holds every Orbit finding that is not a defect, each
// with the evidence that settles it.
var declaredOrbitFields = []orbitDeclaration{
	{Package: orbitPkg, Output: "StatusOutput", Path: "components", Category: categoryOrbitNeverSent, Reason: reasonStatusPromoted},
	{Package: orbitPkg, Output: "StatusOutput", Path: "formatted_text", Category: categoryOrbitNeverSent, Reason: reasonStatusPromoted},
	{Package: orbitPkg, Output: "StatusOutput", Path: "status", Category: categoryOrbitNeverSent, Reason: reasonStatusPromoted},
	{Package: orbitPkg, Output: "StatusOutput", Path: "timestamp", Category: categoryOrbitNeverSent, Reason: reasonStatusPromoted},
	{Package: orbitPkg, Output: "StatusOutput", Path: "version", Category: categoryOrbitNeverSent, Reason: reasonStatusPromoted},
	{
		Package: orbitPkg, Output: "StatusOutput", Path: "system.error", Category: categoryOrbitConditional,
		Reason: "get_cluster_health (ee/lib/analytics/knowledge_graph/grpc_client.rb) rescues GRPC::BadStatus and answers " +
			"`{ status: 'unknown', error: 'Service unreachable' }`, and sends no error otherwise. The recording reached a " +
			"healthy cluster, so the key is published for the one answer it cannot record.",
	},
	{
		Package: orbitPkg, Output: "QueryOutput", Path: "raw_query_strings", Category: categoryOrbitConditional,
		Reason: "workhorse/internal/orbit/sendquery.go writes the query answer as queryResponse, whose RawQueryStrings is " +
			"`json:\"raw_query_strings,omitempty\"`, and the Knowledge Graph service fills it only when the query sets " +
			"options.include_debug_sql and the caller may see debug SQL (crates/query-engine/shared/src/stages/output.rs in " +
			"gitlab-org/orbit/knowledge-graph, `requested && can_see_debug_sql(ctx)`). The recorded query asks for no debug SQL.",
	},
	{
		Package: orbitPkg, Output: "QueryOutput", Path: "formatted_text", Category: categoryOrbitWholeBody,
		Reason: "the llm answer to POST /orbit/query is text/plain, written by writeLLMResultResponse in " +
			"workhorse/internal/orbit/sendquery.go, so it has no keys at all. Query reads it through client-go's QueryRaw and " +
			"publishes the text as formatted_text, the name the other Orbit answers give their llm text.",
	},
	{
		Package: orbitPkg, Output: "DSLOutput", Path: "content", Category: categoryOrbitWholeBody,
		Reason: "GET /orbit/schema/dsl answers with the DSL itself: a JSON Schema document for raw, a JSON string for llm " +
			"(get_query_dsl in ee/lib/analytics/knowledge_graph/grpc_client.rb). client-go's GetDsl returns the body as a " +
			"string; DSL publishes the raw document whole as content, since its keys are its own properties and not a " +
			"response shape, and the llm string's text, decoded by llmGrammar. The record keeps the raw body verbatim for " +
			"the same reason.",
	},
	{
		Package: orbitPkg, Output: "DSLOutput", Path: "response_format", Category: categoryServerDerived,
		Reason: "the response format the caller asked for, echoed by DSL (internal/tools/orbit/orbit.go) so a reader can " +
			"tell a JSON Schema body from an llm grammar; the route answers with the body alone.",
	},
}

// declaredOrbitField finds the declaration covering one finding.
func declaredOrbitField(finding OrbitField, sent bool) (orbitDeclaration, bool) {
	for _, declaration := range declaredOrbitFields {
		if declaration.covers(finding, sent) {
			return declaration, true
		}
	}
	return orbitDeclaration{}, false
}

// covers reports whether this declaration accounts for one finding.
func (d orbitDeclaration) covers(finding OrbitField, sent bool) bool {
	return d.Package == finding.Package && d.Output == finding.Output && d.Path == finding.Path && d.Sent == sent
}

// key names one declaration in a report.
func (d orbitDeclaration) key() string {
	direction := "published"
	if d.Sent {
		direction = "sent"
	}
	return d.Package + "." + d.Output + " " + d.Path + " (" + direction + ")"
}

// classifyOrbitFindings attaches the declaration that accounts for each
// finding in either direction, and names the declarations that accounted for
// none, sorted and nil when there are none.
func classifyOrbitFindings(unpublished, unsurfaced []OrbitField) (classifiedUnpublished, classifiedUnsurfaced []OrbitField, unused []string) {
	used := map[string]bool{}
	classifiedUnpublished = classifyOrbitDirection(unpublished, false, used)
	classifiedUnsurfaced = classifyOrbitDirection(unsurfaced, true, used)
	for _, declaration := range declaredOrbitFields {
		if !used[declaration.key()] {
			unused = append(unused, declaration.key())
		}
	}
	sort.Strings(unused)
	return classifiedUnpublished, classifiedUnsurfaced, unused
}

// classifyOrbitDirection annotates one direction's findings, recording in used
// the declarations that accounted for something.
func classifyOrbitDirection(found []OrbitField, sent bool, used map[string]bool) []OrbitField {
	var classified []OrbitField
	for _, finding := range found {
		if declaration, ok := declaredOrbitField(finding, sent); ok {
			finding.Category = declaration.Category
			finding.Reason = declaration.Reason
			used[declaration.key()] = true
		}
		classified = append(classified, finding)
	}
	return classified
}
