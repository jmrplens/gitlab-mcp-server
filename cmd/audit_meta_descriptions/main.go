package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// stdout is the report stream. It is a variable so a test can read what the
// command prints without redirecting the process.
var stdout io.Writer = os.Stdout

// enumerationHead matches the "- <action>: " prefix of a parameter enumeration
// line. Several actions may share one line, separated by slashes, and an action
// name may itself be a wildcard such as *_list.
var enumerationHead = regexp.MustCompile(`^- ([A-Za-z0-9_*]+(?:\s*/\s*[A-Za-z0-9_*]+)*):\s*(.*)$`)

// parameterItem matches one enumerated parameter: a lowercase name, an optional
// asterisk marking it required, and an optional parenthesised annotation. The
// annotation may not nest, which keeps a returned-shape fragment such as
// "entities* (array of {…})" out of the parsed set rather than half-read.
var parameterItem = regexp.MustCompile(`^([a-z][a-z0-9_]*)(\*?)(?:\s*\(([^()]*)\))?$`)

// bareToken matches an annotation part that can be an enum value: an identifier
// with no spaces and no punctuation. It is what separates "(ALL/NAMESPACES)",
// which names two values, from "(max 100)" or "(ID or path)", which are prose.
var bareToken = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// guidanceHead matches one line of the served "Parameter guidance:" block,
// which is generated per action from a hand-written map keyed by parameter
// name: "- <action>.<parameter>: <role>. Source: …".
var guidanceHead = regexp.MustCompile(`^- ([a-z0-9_]+)\.([a-z0-9_]+):`)

// findingKind names the three disagreements this audit reports.
const (
	kindParameter = "parameter"
	kindEnumValue = "enum value"
	kindGuidance  = "guidance"
)

// mention is one parameter a description enumerates, with the enum values its
// annotation spells when the annotation is a value list.
type mention struct {
	name   string
	values []string
}

// finding is one disagreement between a served description and the schemas of
// the actions it describes.
type finding struct {
	tool   string
	kind   string
	detail string
	line   string
}

// accepted is what the schemas on the other side of the comparison allow: every
// parameter name, and the union of published enum values per name. A name with
// no entry in values publishes no enum anywhere, and its spelled values are not
// judged.
type accepted struct {
	names  map[string]bool
	values map[string]map[string]bool
}

// schemas is one served tool's accepted parameters, both pooled and per action.
// The enumeration block names an action per line but often lists a parameter
// several actions share, so it is judged against the union; the guidance block
// names exactly one action per line and is judged against that action alone.
type schemas struct {
	union    accepted
	byAction map[string]accepted
}

func main() {
	check := flag.Bool("check", false, "exit non-zero when a served description disagrees with the schemas")
	flag.Parse()

	// os.Exit lives here, not in run: run holds the stub client's deferred
	// cleanup, and an exit inside it would skip that defer.
	os.Exit(run(*check))
}

// run performs the audit and returns the process exit code.
func run(check bool) int {
	client, cleanup := mcpsurface.NewStubClient()
	defer cleanup()

	catalog := cmdutil.Must(tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true}))
	findings, lines := audit(metaTools(client), catalog)
	return report(findings, lines, check)
}

// metaTools lists the meta surface at the widest tier over a real tools/list
// round-trip, so the descriptions judged are the ones that cross the wire.
//
// Registering a surface and listing it in memory cannot fail here: the catalog
// is the one compiled into this binary and both ends of the transport are this
// process. An audit that cannot build the surface it audits has nothing to
// report either way, so the failure aborts rather than reaching the exit code,
// which is reserved for what the -check gate found.
func metaTools(client *gitlabclient.Client) []*mcp.Tool {
	session, cleanup := mcpsurface.Session(func(server *mcp.Server) {
		cmdutil.MustDo(tools.RegisterAllMeta(server, client, edition.Ultimate))
		tools.RegisterMCPMeta(server, client)
		tools.RegisterMetaStandaloneTools(server, client)
	})
	defer cleanup()

	return cmdutil.Must(session.ListTools(context.Background(), nil)).Tools
}

// audit compares every served description with what its actions accept and
// returns the findings plus the number of enumeration lines it read. The count
// is reported because the extraction rule skips a line it cannot parse whole:
// a rule that suddenly reads nothing is a silent pass, and the number is what
// makes that visible.
func audit(served []*mcp.Tool, catalog *actioncatalog.Catalog) (findings []finding, lines int) {
	for _, tool := range served {
		allowed := acceptedFor(tool, catalog)
		for _, enumeration := range parseEnumerations(tool.Description) {
			lines++
			findings = append(findings, judge(tool.Name, enumeration, allowed.union)...)
		}
		for _, mentioned := range parseGuidance(tool.Description) {
			lines++
			findings = append(findings, judgeGuidance(tool.Name, mentioned, allowed)...)
		}
	}
	// The line is part of the key, and the sort is stable, because six lines of
	// one tool naming one wrong parameter tie on everything else: without it the
	// same corpus printed those six in a different order from run to run.
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].tool != findings[j].tool {
			return findings[i].tool < findings[j].tool
		}
		if findings[i].kind != findings[j].kind {
			return findings[i].kind < findings[j].kind
		}
		if findings[i].detail != findings[j].detail {
			return findings[i].detail < findings[j].detail
		}
		return findings[i].line < findings[j].line
	})
	return findings, lines
}

// judge reports what one enumeration line offers that the schemas do not.
func judge(toolName string, line enumeration, allowed accepted) []finding {
	var found []finding
	for _, param := range line.params {
		if !allowed.names[param.name] {
			found = append(found, finding{tool: toolName, kind: kindParameter, detail: param.name, line: line.text})
			continue
		}
		published := allowed.values[param.name]
		if len(published) == 0 {
			continue
		}
		for _, value := range param.values {
			if !published[value] {
				found = append(found, finding{
					tool:   toolName,
					kind:   kindEnumValue,
					detail: param.name + "=" + value,
					line:   line.text,
				})
			}
		}
	}
	return found
}

// judgeGuidance reports a guidance line written for a parameter its own action
// does not accept. An action the catalog does not know is not judged: the
// standalone tools carry no per-action schemas, and a group renamed out from
// under its guidance is a different failure that the enumeration side already
// reports.
func judgeGuidance(toolName string, mentioned guidance, allowed schemas) []finding {
	action, known := allowed.byAction[mentioned.action]
	if !known || action.names[mentioned.param] {
		return nil
	}
	return []finding{{
		tool:   toolName,
		kind:   kindGuidance,
		detail: mentioned.action + "." + mentioned.param,
		line:   mentioned.text,
	}}
}

// report prints the findings and returns the exit code: 1 when check is set and
// anything disagrees, 0 otherwise.
func report(findings []finding, lines int, check bool) int {
	for _, f := range findings {
		fmt.Fprintf(stdout, "%-28s %-10s %-24s %s\n", f.tool, f.kind, f.detail, f.line)
	}
	if len(findings) == 0 {
		fmt.Fprintf(stdout, "meta description audit: %d description line(s) read, every parameter and value they offer exists\n", lines)
		return 0
	}
	fmt.Fprintf(stdout, "meta description audit: %d description line(s) read, %d disagree with the schemas\n", lines, len(findings))
	if check {
		return 1
	}
	return 0
}

// acceptedFor returns what a served tool's actions accept: the routes of its
// catalog group, pooled and per action, or, for a standalone meta tool that is
// no group, the tool's own input schema as the pool.
func acceptedFor(tool *mcp.Tool, catalog *actioncatalog.Catalog) schemas {
	allowed := schemas{union: newAccepted(), byAction: map[string]accepted{}}
	group, ok := catalog.Group(tool.Name)
	if !ok {
		allowed.union.add(schemaMap(tool.InputSchema))
		return allowed
	}
	for _, action := range group.ActionsInOrder() {
		allowed.union.add(action.Route.InputSchema)
		perAction := newAccepted()
		perAction.add(action.Route.InputSchema)
		allowed.byAction[action.Name] = perAction
	}
	return allowed
}

// newAccepted returns an empty accepted set.
func newAccepted() accepted {
	return accepted{names: map[string]bool{}, values: map[string]map[string]bool{}}
}

// add folds one input schema's top-level properties into the accepted set.
// Nesting is deliberately not descended: the descriptions enumerate flat
// parameter names, and a nested field would be judged against a name no
// description spells.
func (a accepted) add(schema map[string]any) {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	for name, raw := range properties {
		a.names[name] = true
		property, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		enum, hasEnum := property["enum"].([]any)
		if !hasEnum {
			continue
		}
		for _, value := range enum {
			text, isText := value.(string)
			if !isText {
				continue
			}
			if a.values[name] == nil {
				a.values[name] = map[string]bool{}
			}
			a.values[name][text] = true
		}
	}
}

// schemaMap renders a served tool's input schema as the generic JSON map the
// rest of this audit reads. The schema is an SDK type rather than a map, and a
// tool with no schema at all is simply nothing to compare against.
func schemaMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var decoded map[string]any
	if json.Unmarshal(raw, &decoded) != nil {
		return nil
	}
	return decoded
}

// enumeration is one parsed "- <action>: <parameters>" line.
type enumeration struct {
	text   string
	params []mention
}

// parseEnumerations returns every parameter enumeration a description carries.
// The usage preamble and the action guidance are removed first, so only the
// curated body is read, and a line is returned only when it parses whole. The
// rule and its shapes are written out in this command's package comment.
func parseEnumerations(description string) []enumeration {
	var found []enumeration
	for line := range strings.SplitSeq(toolutil.StripMetaToolDescriptionPrefix(description), "\n") {
		params, ok := parseEnumerationLine(line)
		if !ok {
			continue
		}
		found = append(found, enumeration{text: strings.TrimSpace(line), params: params})
	}
	return found
}

// guidance is one parsed line of the served "Parameter guidance:" block.
type guidance struct {
	action string
	param  string
	text   string
}

// parseGuidance returns every parameter the guidance block writes about. The
// block is bounded by its own heading and the first line that is not a bullet,
// so the enumeration block further down is not read here.
func parseGuidance(description string) []guidance {
	var found []guidance
	inBlock := false
	for raw := range strings.SplitSeq(description, "\n") {
		line := strings.TrimSpace(raw)
		if line == "Parameter guidance:" {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		head := guidanceHead.FindStringSubmatch(line)
		if head == nil {
			inBlock = false
			continue
		}
		found = append(found, guidance{action: head[1], param: head[2], text: line})
	}
	return found
}

// parseEnumerationLine parses one line, reporting whether it is a parameter
// enumeration at all. A line whose head is not "- <action>: " is prose; a line
// with a single item that does not parse as a parameter is prose too, and is
// skipped whole rather than mined for the words that happen to look like names.
func parseEnumerationLine(line string) (params []mention, ok bool) {
	head := enumerationHead.FindStringSubmatch(strings.TrimSpace(line))
	if head == nil {
		return nil, false
	}
	body := stripLeadingParenthetical(sentenceHead(strings.TrimSpace(head[2])))
	if body == "" {
		// "- license_get: (no params)" and its siblings: a real
		// enumeration that enumerates nothing.
		return nil, true
	}
	for _, item := range splitTopLevel(body, ",") {
		for _, alternative := range splitTopLevel(item, " or ") {
			parsed := parameterItem.FindStringSubmatch(alternative)
			if parsed == nil {
				return nil, false
			}
			params = append(params, mention{name: parsed[1], values: annotationValues(parsed[3])})
		}
	}
	return params, true
}

// annotationValues reads an item's parenthesised annotation as a list of enum
// values, which it is only when it holds several slash-separated bare tokens.
// "(ALL/NAMESPACES)" is a value set; "(max 100)", "(bool)" and "(ID or path)"
// are prose about the parameter and name no value.
func annotationValues(annotation string) []string {
	parts := strings.Split(annotation, "/")
	if len(parts) < 2 {
		return nil
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !bareToken.MatchString(part) {
			return nil
		}
		values = append(values, part)
	}
	return values
}

// sentenceHead returns the text before the first sentence-ending period outside
// any bracket, which is where an enumeration stops and a remark about it
// begins: "project_id*, deploy_key_id*. If a workflow says …".
func sentenceHead(body string) string {
	depth := 0
	for index, char := range body {
		switch char {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '.':
			if depth == 0 && (index+1 == len(body) || body[index+1] == ' ') {
				return body[:index]
			}
		}
	}
	return body
}

// stripLeadingParenthetical removes an opening remark such as "(admin only)"
// from an enumeration body, leaving the parameters behind it. A body that is
// nothing but the remark, "(no params)", becomes empty.
func stripLeadingParenthetical(body string) string {
	if !strings.HasPrefix(body, "(") {
		return body
	}
	depth := 0
	for index, char := range body {
		switch char {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return strings.TrimSpace(body[index+1:])
			}
		}
	}
	return body
}

// splitTopLevel splits on sep outside any bracket, so an annotation that holds
// a comma stays one item and one that reads "(numeric ID or full path)" is not
// mistaken for a pair of alternative parameter names.
func splitTopLevel(body, sep string) []string {
	var parts []string
	depth, start := 0, 0
	for index := 0; index < len(body); index++ {
		switch body[index] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
		if depth != 0 || !strings.HasPrefix(body[index:], sep) {
			continue
		}
		parts = append(parts, strings.TrimSpace(body[start:index]))
		index += len(sep) - 1
		start = index + 1
	}
	return append(parts, strings.TrimSpace(body[start:]))
}
