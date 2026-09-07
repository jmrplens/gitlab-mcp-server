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

// labeledPair matches one item of a labeled value list: a bare value, an
// equals sign, and the label that says what it means. The label must open with
// a letter, so "x=5" is not read as a value named x.
var labeledPair = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*=\s*[A-Za-z]`)

// guidanceHead matches one line of the served "Parameter guidance:" block,
// which is generated per action from a hand-written map keyed by parameter
// name: "- <action>.<parameter>: <role>. Source: …".
var guidanceHead = regexp.MustCompile(`^- ([a-z0-9_]+)\.([a-z0-9_]+):`)

// findingKind names the four disagreements this audit reports.
const (
	kindParameter = "parameter"
	kindEnumValue = "enum value"
	kindDocValue  = "doc value"
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

// skipped is one line whose head reads as a parameter enumeration but whose
// body the rule refused, so nothing on it was judged. It is the coverage the
// summary counts and -uncovered names: a line outside the check is a line where
// a stale parameter can hide.
type skipped struct {
	tool string
	line string
}

// accepted is what the schemas on the other side of the comparison allow: every
// parameter name, the union of published enum values per name, and the union of
// the values the property descriptions spell where the value set is closed but
// carries no enum. A name with no entry in either map publishes no value set
// anywhere, and its spelled values are not judged.
type accepted struct {
	names      map[string]bool
	values     map[string]map[string]bool
	documented map[string]map[string]bool
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
	uncovered := flag.Bool("uncovered", false, "name the enumeration lines the extraction rule refused instead of only counting them")
	flag.Parse()

	// os.Exit lives here, not in run: run holds the stub client's deferred
	// cleanup, and an exit inside it would skip that defer.
	os.Exit(run(*check, *uncovered))
}

// run performs the audit and returns the process exit code.
func run(check, uncovered bool) int {
	client, cleanup := mcpsurface.NewStubClient()
	defer cleanup()

	catalog := cmdutil.Must(tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true}))
	findings, lines, refused := audit(metaTools(client), catalog)
	return report(findings, lines, refused, check, uncovered)
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
// returns the findings, the number of description lines it read, and the
// enumeration lines the extraction rule refused. Both counts are reported
// because the rule skips a line it cannot parse whole: a rule that suddenly
// reads nothing is a silent pass, and the numbers are what make that visible.
func audit(served []*mcp.Tool, catalog *actioncatalog.Catalog) (findings []finding, lines int, refused []skipped) {
	for _, tool := range served {
		allowed := acceptedFor(tool, catalog)
		read, unread := parseEnumerations(tool.Description)
		for _, enumeration := range read {
			lines++
			findings = append(findings, judge(tool.Name, enumeration, allowed.forLine(enumeration.actions))...)
		}
		for _, line := range unread {
			if !allowed.describesAnAction(line.actions) {
				continue
			}
			refused = append(refused, skipped{tool: tool.Name, line: line.text})
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
	return findings, lines, refused
}

// judge reports what one enumeration line offers that the schemas do not.
//
// A spelled value is judged against the property's published enum where there
// is one, and otherwise against the value set the property's own description
// spells. The second half exists because a numeric value set cannot be an enum
// of strings: gitlab_member_role's base_access_level offered 5 in the prose
// while its description said 10 to 50, and GitLab refuses 5.
func judge(toolName string, line enumeration, allowed accepted) []finding {
	var found []finding
	for _, param := range line.params {
		if !allowed.names[param.name] {
			found = append(found, finding{tool: toolName, kind: kindParameter, detail: param.name, line: line.text})
			continue
		}
		published, kind := allowed.values[param.name], kindEnumValue
		if len(published) == 0 {
			published, kind = allowed.documented[param.name], kindDocValue
		}
		if len(published) == 0 {
			continue
		}
		for _, value := range param.values {
			if !published[value] {
				found = append(found, finding{
					tool:   toolName,
					kind:   kind,
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
// anything disagrees, 0 otherwise. The refused lines are counted always and
// named under -uncovered, because coverage is the one thing a clean run cannot
// speak for on its own.
func report(findings []finding, lines int, refused []skipped, check, uncovered bool) int {
	for _, f := range findings {
		fmt.Fprintf(stdout, "%-28s %-10s %-24s %s\n", f.tool, f.kind, f.detail, f.line)
	}
	if uncovered {
		for _, s := range refused {
			fmt.Fprintf(stdout, "%-28s %-10s %s\n", s.tool, "uncovered", s.line)
		}
	}
	if len(findings) == 0 {
		fmt.Fprintf(stdout, "meta description audit: %d description line(s) read, %d refused, every parameter and value they offer exists\n", lines, len(refused))
		return 0
	}
	fmt.Fprintf(stdout, "meta description audit: %d description line(s) read, %d refused, %d disagree with the schemas\n", lines, len(refused), len(findings))
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

// forLine returns the set one enumeration line is judged against: the action
// its head names, when it names exactly one the catalog knows, and the pooled
// union otherwise. A line shared by several actions lists what each of them
// takes and annotates which is which, so only the union can hold it; a line
// about one action can be held to that action's own schema, and must be. The
// union is what let "- pages_update: project_id*, pages_https_only,
// pages_access_level" pass while pages_access_level belongs to project.update,
// and what hid bulk_import_start's flat url behind another action's url.
func (s schemas) forLine(actions []string) accepted {
	if len(actions) != 1 {
		return s.union
	}
	if action, known := s.byAction[actions[0]]; known {
		return action
	}
	return s.union
}

// describesAnAction reports whether a refused line's head names an action of
// this group, which is what separates a parameter line the rule could not read
// from a bullet that merely opens like one. "- Destructive: …" and "- HTTPS: …"
// name no action and are prose by construction; "- hook_edit: …" is a line
// about a real action's parameters, and one the check cannot see is coverage
// the report has to admit to.
func (s schemas) describesAnAction(actions []string) bool {
	for _, action := range actions {
		if _, known := s.byAction[action]; known {
			return true
		}
	}
	return false
}

// newAccepted returns an empty accepted set.
func newAccepted() accepted {
	return accepted{names: map[string]bool{}, values: map[string]map[string]bool{}, documented: map[string]map[string]bool{}}
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
		a.addEnum(name, property)
		description, _ := property["description"].(string)
		record(a.documented, name, labeledValues(description))
	}
}

// addEnum folds one property's published enum into the accepted set, keeping
// only its string values: a schema enum is a closed set of strings, and a
// number in one is not a value any description spells.
func (a accepted) addEnum(name string, property map[string]any) {
	enum, ok := property["enum"].([]any)
	if !ok {
		return
	}
	for _, value := range enum {
		if text, isText := value.(string); isText {
			record(a.values, name, []string{text})
		}
	}
}

// record folds values into the set held under name, creating it on first use so
// a name with no value set has no entry at all and is left unjudged.
func record(into map[string]map[string]bool, name string, values []string) {
	for _, value := range values {
		if into[name] == nil {
			into[name] = map[string]bool{}
		}
		into[name][value] = true
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

// enumeration is one parsed "- <action>: <parameters>" line, with the action
// names its head spells: a line about one action is held to that action alone.
type enumeration struct {
	text    string
	actions []string
	params  []mention
}

// refusal is one line that opens like a parameter enumeration and that the rule
// refused, carrying the action names its head spells so the caller can tell a
// stale parameter line from a bullet that only looks like one.
type refusal struct {
	actions []string
	text    string
}

// parseEnumerations returns every parameter enumeration a description carries,
// and beside it every line that opens like one and was refused. The usage
// preamble and the action guidance are removed first, so only the curated body
// is read; the "Returns:" block is bounded and skipped, since its bullets name
// actions and then describe what they answer with rather than what they accept.
// A line is returned as parsed only when it parses whole. The rule and its
// shapes are written out in this command's package comment.
func parseEnumerations(description string) (found []enumeration, refused []refusal) {
	inReturns := false
	for line := range strings.SplitSeq(toolutil.StripMetaToolDescriptionPrefix(description), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Returns:" {
			inReturns = true
			continue
		}
		if inReturns {
			if strings.HasPrefix(trimmed, "- ") {
				continue
			}
			inReturns = false
		}
		actions, params, ok := parseEnumerationLine(line)
		if actions == nil {
			continue
		}
		if !ok {
			refused = append(refused, refusal{actions: actions, text: trimmed})
			continue
		}
		found = append(found, enumeration{text: trimmed, actions: actions, params: params})
	}
	return found, refused
}

// headActions splits an enumeration head into the action names it spells, so a
// line shared by several actions is attributed to each of them.
func headActions(head string) []string {
	names := strings.Split(head, "/")
	for index, name := range names {
		names[index] = strings.TrimSpace(name)
	}
	return names
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

// parseEnumerationLine parses one line, reporting the actions its head names
// and whether the body is a parameter enumeration at all. A line whose head is
// not "- <action>: " is prose and returns no actions; a line with a single item
// that does not parse as a parameter is prose too, and is skipped whole rather
// than mined for the words that happen to look like names. The actions are
// returned either way, so a refused line can still be attributed to what it
// describes.
func parseEnumerationLine(line string) (actions []string, params []mention, ok bool) {
	head := enumerationHead.FindStringSubmatch(strings.TrimSpace(line))
	if head == nil {
		return nil, nil, false
	}
	actions = headActions(head[1])
	body := stripLeadingParenthetical(sentenceHead(strings.TrimSpace(head[2])))
	if body == "" {
		// "- license_get: (no params)" and its siblings: a real
		// enumeration that enumerates nothing.
		return actions, nil, true
	}
	for _, item := range splitTopLevel(body, ",") {
		for _, alternative := range splitTopLevel(item, " or ") {
			parsed := parameterItem.FindStringSubmatch(alternative)
			if parsed == nil {
				return actions, nil, false
			}
			params = append(params, mention{name: parsed[1], values: annotationValues(parsed[3])})
		}
	}
	return actions, params, true
}

// annotationValues reads an item's parenthesised annotation as a list of the
// values the parameter accepts, in the two shapes the descriptions use: several
// slash-separated bare tokens, "(ALL/NAMESPACES)", or a labeled list,
// "(10=Guest, 20=Reporter)". "(max 100)", "(bool)" and "(ID or path)" are prose
// about the parameter and name no value.
func annotationValues(annotation string) []string {
	parts := strings.Split(annotation, "/")
	if len(parts) < 2 {
		return labeledValues(annotation)
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !bareToken.MatchString(part) {
			return labeledValues(annotation)
		}
		values = append(values, part)
	}
	return values
}

// labeledValues reads a "<value>=<label>" list as the values it names: the
// longest run of comma-separated items, each naming a value and what it means,
// that the text opens with or that one of its parentheticals holds. A schema
// enum is a closed set of strings, so a numeric set such as an access level can
// only be written this way, on both sides of the comparison: it is how a
// description spells the set and how the prose offers it.
//
// The run ends at the first sentence break and at the first item that is not
// such a pair, so "10=Guest, 50=Owner. 60=Admin is not valid" names the levels
// the sentence offers and not the one it denies. Reading the denial as an
// offered value would be a false failure, which is the one thing this audit
// must not produce.
func labeledValues(text string) []string {
	head := sentenceHead(strings.TrimSpace(text))
	for _, region := range append([]string{head}, parentheticals(head)...) {
		if values := leadingPairs(region); len(values) > 1 {
			return values
		}
	}
	return nil
}

// leadingPairs returns the values of the comma-separated "<value>=<label>"
// items a region opens with, stopping at the first item that is not one.
func leadingPairs(region string) []string {
	var values []string
	for _, item := range splitTopLevel(region, ",") {
		pair := labeledPair.FindStringSubmatch(item)
		if pair == nil {
			return values
		}
		values = append(values, pair[1])
	}
	return values
}

// parentheticals returns the contents of every top-level parenthetical in text,
// which is where a description keeps its value set: "Base access level
// (10=Guest, 20=Reporter)" spells the set inside the brackets and the sentence
// around them is prose.
func parentheticals(text string) []string {
	var found []string
	depth, start := 0, 0
	for index, char := range text {
		switch char {
		case '(':
			if depth == 0 {
				start = index + 1
			}
			depth++
		case ')':
			depth--
			if depth == 0 {
				found = append(found, text[start:index])
			}
		}
	}
	return found
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
