package required

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// Directions a finding takes. Each names what GitLab says, since GitLab is the
// side the schema is held to.
const (
	// DirectionGitLabRequires is a field GitLab requires on every way the
	// action runs and the schema leaves optional: a call the surface accepts
	// and GitLab refuses.
	DirectionGitLabRequires = "gitlab-requires"
	// DirectionGitLabOptional is a field GitLab leaves optional on some way
	// the action runs and the schema requires: a call GitLab would serve
	// that the surface refuses before sending it.
	DirectionGitLabOptional = "gitlab-optional"
)

// Report is the output of this scope.
type Report struct {
	SchemaVersion int `json:"schema_version"`
	// Requests and Record name the two committed files the join reads.
	Requests string `json:"requests"`
	Record   string `json:"record"`
	// Findings are the disagreements no declaration answers, and Declared
	// the ones a declaration does, each with its category and reason.
	Findings []Finding `json:"findings"`
	Declared []Finding `json:"declared,omitempty"`
	// Stale are declarations that answer no disagreement.
	Stale []string `json:"stale_declarations"`
	// Unplaced lists, per action, the fields no parameter of any route the
	// action sends was found under: this server's own controls, a GraphQL
	// argument, or a path parameter an input names differently from GitLab.
	// Nothing is said about them, which is why they are listed.
	Unplaced []ActionFields `json:"unplaced,omitempty"`
	// Unjudged lists, per action, the fields placed on some way of the action
	// and not on another that sends a request this scope cannot read the
	// parameters of (GraphQL, or a request the derivation left unresolved).
	Unjudged []ActionFields `json:"unjudged,omitempty"`
	Summary  Summary        `json:"summary"`
}

// Finding is one field whose requiredness the schema and GitLab disagree on.
type Finding struct {
	Action    string `json:"action"`
	Field     string `json:"field"`
	Direction string `json:"direction"`
	// Routes are the routes of the action the field was placed on, as the
	// live record names them, so a reader can look the parameter up.
	Routes []string `json:"routes"`
	// Category and Reason are the declaration's, when one answers it.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// ActionFields is a list of fields of one action.
type ActionFields struct {
	Action string   `json:"action"`
	Fields []string `json:"fields"`
}

// Summary counts what the parts hold.
type Summary struct {
	// Actions is every action the catalog publishes, and Judged the ones at
	// least one field of which was compared.
	Actions int `json:"actions"`
	Judged  int `json:"actions_judged"`
	// NoRESTRequest are the actions the record has no REST request for, whose
	// fields there is no route to place on.
	NoRESTRequest int `json:"actions_without_rest_request"`
	// FieldsJudged, FieldsUnplaced and FieldsUnjudged partition the fields of
	// the actions that send a REST request.
	FieldsJudged   int `json:"fields_judged"`
	FieldsUnplaced int `json:"fields_unplaced"`
	FieldsUnjudged int `json:"fields_unjudged"`
	Findings       int `json:"findings"`
	Declared       int `json:"declared"`
	Stale          int `json:"stale_declarations"`
}

// clean reports whether the gate passes.
func (s Summary) clean() bool { return s.Findings == 0 && s.Stale == 0 }

// Options is what a run of this scope needs beyond the tree.
type Options struct {
	// GapsOnly drops the context lists, keeping the findings, the declared
	// ones and the stale declarations.
	GapsOnly bool
}

// Seams for the inputs and for the JSON encoder that never fails on a report
// of strings and ints. Each is a variable a test restores.
var (
	marshalIndent = json.MarshalIndent
	readRecord    = actionrequests.ReadRecord
	catalogs      = actionids.Catalogs
	readLive      = func(root string) (apilive.Document, error) {
		return apilive.Read(filepath.Join(root, apilive.DefaultDir))
	}
	declarations = declaredRequiredness
	aliases      = declaredAliases
)

// Run builds the report for the repository at root and returns it as indented
// JSON with a trailing newline, together with the gate's outcome.
func Run(root string, opts Options) (content []byte, clean bool, err error) {
	report, err := buildReport(root, opts)
	if err != nil {
		return nil, false, err
	}
	content, err = marshalIndent(report, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), report.Summary.clean(), nil
}

// buildReport reads the inputs and judges every action.
func buildReport(root string, opts Options) (Report, error) {
	record, err := readRecord(root)
	if err != nil {
		return Report{}, err
	}
	live, err := readLive(root)
	if err != nil {
		return Report{}, fmt.Errorf("read the live record: %w", err)
	}
	built, err := catalogs()
	if err != nil {
		return Report{}, fmt.Errorf("build the action catalog: %w", err)
	}
	report := judgeAll(schemasByAction(built), record, routesByName(live.Routes), declarations(), aliases())
	report.Requests = actionrequests.RecordPath
	report.Record = filepath.ToSlash(apilive.Path(apilive.DefaultDir))
	if opts.GapsOnly {
		report.Unplaced = nil
		report.Unjudged = nil
	}
	return report, nil
}

// schemasByAction returns the input schema of every action the catalogs hold,
// each action once, from the first catalog that holds it: the self-managed
// build before the GitLab.com one, which agree on every action both hold.
func schemasByAction(built []*actioncatalog.Catalog) map[string]map[string]any {
	schemas := map[string]map[string]any{}
	for _, catalog := range built {
		for _, action := range catalog.Actions() {
			id := string(action.ID)
			if _, seen := schemas[id]; !seen {
				schemas[id] = action.Route.InputSchema
			}
		}
	}
	return schemas
}

// routesByName indexes the record's routes by the name the request record
// spells them with.
func routesByName(routes []apilive.Route) map[string]*apilive.Route {
	byName := make(map[string]*apilive.Route, len(routes))
	for index := range routes {
		byName[apilive.RouteName(&routes[index])] = &routes[index]
	}
	return byName
}

// judgeAll judges every action and assembles the report.
func judgeAll(schemas map[string]map[string]any, record actionrequests.Record, routes map[string]*apilive.Route, declared map[declarationKey]declaration, aliased map[aliasKey]pathAlias) Report {
	report := Report{SchemaVersion: shared.SchemaVersion, Findings: []Finding{}}
	byAction := aliasesByAction(aliased)
	byID := make(map[string]actionrequests.RecordAction, len(record.Actions))
	for _, action := range record.Actions {
		byID[action.ID] = action
	}
	ids := make([]string, 0, len(schemas))
	for id := range schemas {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	used := map[declarationKey]bool{}
	aliasUsed := map[aliasKey]bool{}
	report.Summary.Actions = len(ids)
	for _, id := range ids {
		verdict := judgeAction(id, schemas[id], byID[id], routes, byAction[id])
		for param := range verdict.aliased {
			if _, isField := schemaProperties(schemas[id])[byAction[id][param]]; isField {
				aliasUsed[aliasKey{action: id, param: param}] = true
			}
		}
		report.add(id, verdict, declared, used)
	}
	report.Stale = staleEntries(declared, used, aliased, aliasUsed)
	report.Summary.Findings = len(report.Findings)
	report.Summary.Declared = len(report.Declared)
	report.Summary.Stale = len(report.Stale)
	return report
}

// aliasesByAction indexes the declared aliases by action, then by the GitLab
// parameter each places.
func aliasesByAction(aliased map[aliasKey]pathAlias) map[string]map[string]string {
	byAction := map[string]map[string]string{}
	for key, alias := range aliased {
		if byAction[key.action] == nil {
			byAction[key.action] = map[string]string{}
		}
		byAction[key.action][key.param] = alias.Field
	}
	return byAction
}

// add files one action's verdict into the report: its counts, its context
// lists, and each disagreement as declared when a declaration answers it in
// its direction, which it marks used, or as a finding otherwise.
func (r *Report) add(id string, verdict actionVerdict, declared map[declarationKey]declaration, used map[declarationKey]bool) {
	if !verdict.rest {
		r.Summary.NoRESTRequest++
		return
	}
	if verdict.judged > 0 {
		r.Summary.Judged++
	}
	r.Summary.FieldsJudged += verdict.judged
	r.Summary.FieldsUnplaced += len(verdict.unplaced)
	r.Summary.FieldsUnjudged += len(verdict.unjudged)
	if len(verdict.unplaced) > 0 {
		r.Unplaced = append(r.Unplaced, ActionFields{Action: id, Fields: verdict.unplaced})
	}
	if len(verdict.unjudged) > 0 {
		r.Unjudged = append(r.Unjudged, ActionFields{Action: id, Fields: verdict.unjudged})
	}
	for _, finding := range verdict.findings {
		key := declarationKey{action: finding.Action, field: finding.Field}
		if answer, ok := declared[key]; ok && answer.Direction == finding.Direction {
			used[key] = true
			finding.Category, finding.Reason = answer.Category, answer.Reason
			r.Declared = append(r.Declared, finding)
			continue
		}
		r.Findings = append(r.Findings, finding)
	}
}

// staleEntries names, sorted, every declaration no disagreement used and
// every alias that placed no parameter onto a field the schema has.
func staleEntries(declared map[declarationKey]declaration, used map[declarationKey]bool, aliased map[aliasKey]pathAlias, aliasUsed map[aliasKey]bool) []string {
	stale := []string{}
	for key, answer := range declared {
		if !used[key] {
			stale = append(stale, fmt.Sprintf("%s %s (%s, %s): answers no disagreement", key.action, key.field, answer.Direction, answer.Category))
		}
	}
	for key, alias := range aliased {
		if !aliasUsed[key] {
			stale = append(stale, fmt.Sprintf("%s %s: no route the action sends declares the parameter, or the schema has no field %s", key.action, key.param, alias.Field))
		}
	}
	sort.Strings(stale)
	return stale
}

// actionVerdict is what judging one action found.
type actionVerdict struct {
	// rest is whether the action sends any REST request at all.
	rest     bool
	judged   int
	findings []Finding
	unplaced []string
	unjudged []string
	// aliased are the GitLab parameters a declared alias placed.
	aliased map[string]bool
}

// wayState is what one way of running an action says of one field.
type wayState int

const (
	// wayOptional is a way that runs without the field: every request on it
	// was read and none requires it.
	wayOptional wayState = iota
	// wayRequires is a way on which a request requires the field.
	wayRequires
	// wayUnknown is a way that does not require the field among the requests
	// it could read and sends one whose parameters it cannot.
	wayUnknown
)

// judgeAction compares each field of one action's schema with what GitLab
// declares of the parameter it is found under.
//
// GitLab requires a field when every way the action runs sends a request that
// requires it, since a way that does not is a call GitLab serves without it.
// A field found on no route is unplaced and a field some way could not be read
// for is unjudged; neither is a finding.
func judgeAction(id string, schema map[string]any, action actionrequests.RecordAction, routes map[string]*apilive.Route, aliases map[string]string) actionVerdict {
	placed := make([]map[string]bool, len(action.Requests))
	routeNames := make([]string, len(action.Requests))
	verdict := actionVerdict{aliased: map[string]bool{}}
	for index, request := range action.Requests {
		if request.Kind != actionrequests.KindREST {
			continue
		}
		route, ok := routes[request.Route]
		if !ok {
			continue
		}
		verdict.rest = true
		placed[index] = placements(route, aliases, verdict.aliased)
		routeNames[index] = request.Route
	}
	if !verdict.rest {
		return verdict
	}
	required := schemaRequired(schema)
	for _, field := range schemaFields(schema) {
		var on []string
		for index := range placed {
			if _, ok := placed[index][field]; ok {
				on = append(on, routeNames[index])
			}
		}
		if len(on) == 0 {
			verdict.unplaced = append(verdict.unplaced, field)
			continue
		}
		gitlab, judged := gitlabRequires(field, action, placed)
		if !judged {
			verdict.unjudged = append(verdict.unjudged, field)
			continue
		}
		verdict.judged++
		ours := required[field]
		if ours == gitlab {
			continue
		}
		direction := DirectionGitLabOptional
		if gitlab {
			direction = DirectionGitLabRequires
		}
		verdict.findings = append(verdict.findings, Finding{Action: id, Field: field, Direction: direction, Routes: uniqueSorted(on)})
	}
	return verdict
}

// gitlabRequires folds the ways an action runs into GitLab's answer for one
// field: required when every way requires it, optional when some way runs
// without it, and unjudged otherwise.
func gitlabRequires(field string, action actionrequests.RecordAction, placed []map[string]bool) (required, judged bool) {
	if len(action.Paths) == 0 {
		return false, false
	}
	unknown := false
	// Two ifs rather than a switch on the three states: the third case would
	// be a condition no input can make false, and the mutation tool cannot
	// see a case expression.
	for _, way := range action.Paths {
		state := wayFor(field, way, placed)
		if state == wayOptional {
			return false, true
		}
		if state == wayUnknown {
			unknown = true
		}
	}
	return true, !unknown
}

// wayFor says what one way of running the action says of a field. A request
// with no placements is one whose parameters cannot be read: a GraphQL
// operation, a request the derivation left unresolved, or a REST route the
// record does not hold.
func wayFor(field string, way []int, placed []map[string]bool) wayState {
	unreadable := false
	for _, index := range way {
		if placed[index] == nil {
			unreadable = true
			continue
		}
		if placed[index][field] {
			return wayRequires
		}
	}
	if unreadable {
		return wayUnknown
	}
	return wayOptional
}

// schemaProperties returns the top-level properties of an input schema.
func schemaProperties(schema map[string]any) map[string]any {
	properties, _ := schema["properties"].(map[string]any)
	return properties
}

// schemaFields returns the top-level properties of an input schema, sorted.
func schemaFields(schema map[string]any) []string {
	properties := schemaProperties(schema)
	fields := make([]string, 0, len(properties))
	for name := range properties {
		fields = append(fields, name)
	}
	sort.Strings(fields)
	return fields
}

// schemaRequired returns the set an input schema's top-level required list
// names, whichever of the two slice types it is held in.
func schemaRequired(schema map[string]any) map[string]bool {
	names := map[string]bool{}
	switch required := schema["required"].(type) {
	case []string:
		for _, name := range required {
			names[name] = true
		}
	case []any:
		for _, raw := range required {
			if name, ok := raw.(string); ok {
				names[name] = true
			}
		}
	}
	return names
}

// uniqueSorted returns names sorted with repeats removed.
func uniqueSorted(names []string) []string {
	out := slices.Clone(names)
	sort.Strings(out)
	return slices.Compact(out)
}

// declarationKey names the field of an action a declaration answers.
type declarationKey struct {
	action string
	field  string
}
