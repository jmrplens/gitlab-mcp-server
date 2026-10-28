// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package params

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/required"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdk"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdkoptions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// The classes of a finding, each naming who can send the param first.
const (
	// ClassUnmodeled is a param no option struct the route's requests carry
	// models and no input of the actions reaching it offers: client-go has
	// to model it before an input can offer it.
	ClassUnmodeled = "unmodeled"
	// ClassNotOffered is a param an option struct the route's requests
	// already carry models and no input offers: an input can offer it
	// today.
	ClassNotOffered = "not_offered"
	// ClassOfferedUnrecorded is a param an input offers that no recorded
	// request of the route carries: a handler that drops it, or a test that
	// never sends it.
	ClassOfferedUnrecorded = "offered_unrecorded"
	// ClassUnjudged is a param no input offers on a route one of whose
	// requests carries options the walk cannot read, so whether anything
	// can send it is not known.
	ClassUnjudged = "unjudged"
)

// Report is the output of this scope.
type Report struct {
	SchemaVersion int `json:"schema_version"`
	// Requests, Record and Inventory name the three committed files the join
	// reads.
	Requests  string  `json:"requests"`
	Record    string  `json:"record"`
	Inventory string  `json:"inventory"`
	Summary   Summary `json:"summary"`
	// Findings are the params nothing recorded sends and no declaration
	// answers, and Declared the ones a declaration does, with its category
	// and reason.
	Findings []Finding `json:"findings"`
	Declared []Finding `json:"declared,omitempty"`
	// Stale are declarations that answer no finding, which fail the gate.
	Stale []string `json:"stale_declarations"`
	// Unread are the requests whose options the walk could not read, which
	// is why a param of their route can be unjudged.
	Unread []UnreadRequest `json:"unread_requests,omitempty"`
}

// Finding is one param of one route nothing recorded sends.
type Finding struct {
	// Route is the route as the request record names it.
	Route string `json:"route"`
	// Param is the param as the live record spells it.
	Param string `json:"param"`
	Class string `json:"class"`
	// SDKModels says whether an option struct the route's requests carry can
	// send it, and Offered whether an input of an action reaching the route
	// offers it.
	SDKModels bool `json:"sdk_models"`
	Offered   bool `json:"offered"`
	// Type and Required are what the record declares of the param.
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
	// Children are the members of a container that stands for them, each
	// of its class.
	Children []string `json:"children,omitempty"`
	// Actions are the actions reaching the route, and OptionTypes the
	// client-go structs their requests carry there.
	Actions     []string `json:"actions"`
	OptionTypes []string `json:"option_types,omitempty"`
	// Category, Reason and Row are the declaration's, when one answers it.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Row      int    `json:"row,omitempty"`
}

// UnreadRequest is one request whose options the walk could not read.
type UnreadRequest struct {
	Route   string `json:"route"`
	Action  string `json:"action"`
	Site    string `json:"site,omitempty"`
	Options string `json:"options,omitempty"`
	Reason  string `json:"reason"`
}

// Summary counts what the parts hold.
type Summary struct {
	// Routes is every REST route the request record names that the live
	// record holds, and RoutesWithFindings the ones a finding or a
	// declaration is on.
	Routes             int `json:"routes_reached"`
	RoutesWithFindings int `json:"routes_with_findings"`
	// Params counts the params judged, path params and the attributes
	// Workhorse fills left out, and Covered the ones a recorded request
	// carries or a request sends on every call.
	Params  int `json:"params_judged"`
	Covered int `json:"params_covered"`
	// The four classes count the findings no declaration answers, and
	// Declared the ones a declaration does: a container that stands for its
	// members is one finding, so they count findings rather than params.
	Unmodeled         int `json:"unmodeled"`
	NotOffered        int `json:"not_offered"`
	OfferedUnrecorded int `json:"offered_unrecorded"`
	Unjudged          int `json:"unjudged"`
	Declared          int `json:"declared"`
	Stale             int `json:"stale_declarations"`
	UnreadRequests    int `json:"unread_requests"`
}

// clean reports whether the gate passes: no declaration that answers
// nothing. A finding is a report, not a failure.
func (s *Summary) clean() bool { return s.Stale == 0 }

// Options is what a run of this scope needs beyond the tree.
type Options struct {
	// GapsOnly drops the declared findings and the unread requests, keeping
	// the findings and the stale declarations.
	GapsOnly bool
}

// Seams for the inputs and for the JSON encoder that never fails on a report
// of strings, ints and bools. Each is a variable a test restores.
var (
	marshalIndent = json.MarshalIndent
	readRecord    = actionrequests.ReadRecord
	readLive      = func(root string) (apilive.Document, error) {
		return apilive.Read(filepath.Join(root, apilive.DefaultDir))
	}
	readInventory = requestinventory.Read
	inputSchemas  = catalogSchemas
	catalogs      = actionids.Catalogs
	rawRequests   = sdk.RawRequests
	readOptions   = sdkoptions.Read
	readRows      = func(root string) (map[int]string, error) {
		return waits.ReadRows(filepath.Join(root, filepath.FromSlash(waits.RegisterPath)))
	}
	declarations = paramDeclarations
	aliases      = required.FieldAliases
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

// buildReport reads the inputs and judges every route.
func buildReport(root string, opts Options) (Report, error) {
	record, err := readRecord(root)
	if err != nil {
		return Report{}, err
	}
	live, err := readLive(root)
	if err != nil {
		return Report{}, fmt.Errorf("read the live record: %w", err)
	}
	inventory, err := readInventory(root)
	if err != nil {
		return Report{}, fmt.Errorf("read the request inventory: %w", err)
	}
	schemas, err := inputSchemas()
	if err != nil {
		return Report{}, fmt.Errorf("build the action catalog: %w", err)
	}
	raw, clientGoDir, err := rawRequests(root)
	if err != nil {
		return Report{}, fmt.Errorf("walk the raw requests: %w", err)
	}
	rows, err := readRows(root)
	if err != nil {
		return Report{}, err
	}
	report := judgeAll(judgeInput{
		record:       record,
		live:         live,
		inventory:    inventory.Requests,
		schemas:      schemas,
		aliases:      aliases(),
		sources:      newSources(readOptions(clientGoDir), raw),
		declarations: declarations(),
		rows:         rows,
	})
	report.Requests = actionrequests.RecordPath
	report.Record = filepath.ToSlash(apilive.Path(apilive.DefaultDir))
	report.Inventory = requestinventory.Path
	if opts.GapsOnly {
		report.Declared = nil
		report.Unread = nil
	}
	return report, nil
}

// catalogSchemas returns the input schema of every action the catalogs hold,
// each action once, from the first catalog that holds it: the self-managed
// build before the GitLab.com one, which agree on every action both hold.
func catalogSchemas() (map[string]map[string]any, error) {
	built, err := catalogs()
	if err != nil {
		return nil, err
	}
	schemas := map[string]map[string]any{}
	for _, catalog := range built {
		for _, action := range catalog.Actions() {
			id := string(action.ID)
			if _, seen := schemas[id]; !seen {
				schemas[id] = action.Route.InputSchema
			}
		}
	}
	return schemas, nil
}

// judgeInput is everything a judgement of the tree reads.
type judgeInput struct {
	record       actionrequests.Record
	live         apilive.Document
	inventory    []requestinventory.Row
	schemas      map[string]map[string]any
	aliases      map[string]map[string]string
	sources      sources
	declarations []paramDeclaration
	rows         map[int]string
}

// judgeAll judges every route the request record names and assembles the
// report.
func judgeAll(in judgeInput) Report {
	report := Report{SchemaVersion: shared.SchemaVersion, Findings: []Finding{}, Stale: []string{}}
	routes := routesByName(in.live.Routes)
	uses, unread := collectUses(in.record, routes, in.sources)
	recorded := recordedByRoute(in.inventory, apilive.NewRouteIndex(in.live.Routes))
	report.Unread = unread
	report.Summary.Routes = len(uses)
	report.Summary.UnreadRequests = len(unread)
	var all []Finding
	for _, name := range sortedKeys(uses) {
		use := uses[name]
		offer := offerer{schemas: in.schemas, aliases: in.aliases, actions: sortedKeys(use.actions)}
		verdict := judgeRoute(use, recorded[name], offer.offers)
		report.Summary.Params += verdict.judged
		report.Summary.Covered += verdict.covered
		if len(verdict.findings) > 0 {
			report.Summary.RoutesWithFindings++
		}
		all = append(all, verdict.findings...)
	}
	used := make([]bool, len(in.declarations))
	for _, finding := range all {
		index, answered := answer(in.declarations, finding)
		if !answered {
			report.Findings = append(report.Findings, finding)
			report.Summary.count(finding.Class)
			continue
		}
		used[index] = true
		declaration := in.declarations[index]
		finding.Category, finding.Reason, finding.Row = declaration.Category, declaration.Reason, declaration.Row
		report.Declared = append(report.Declared, finding)
	}
	report.Stale = staleDeclarations(in.declarations, used, in.rows)
	report.Summary.Declared = len(report.Declared)
	report.Summary.Stale = len(report.Stale)
	return report
}

// count adds one undeclared finding of class to the summary.
func (s *Summary) count(class string) {
	switch class {
	case ClassUnmodeled:
		s.Unmodeled++
	case ClassNotOffered:
		s.NotOffered++
	case ClassOfferedUnrecorded:
		s.OfferedUnrecorded++
	default:
		s.Unjudged++
	}
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

// recordedByRoute returns, by route name, every param a recorded request of
// the route carried, spelled the way the record declares it, whichever
// package's test drove it.
func recordedByRoute(rows []requestinventory.Row, index *apilive.RouteIndex) map[string]map[string]bool {
	recorded := map[string]map[string]bool{}
	for _, row := range rows {
		if !strings.EqualFold(row.Kind, requestinventory.KindREST) {
			continue
		}
		route, found := index.Match(strings.ToUpper(row.Method), row.Path)
		if !found {
			continue
		}
		name := apilive.RouteName(route)
		if recorded[name] == nil {
			recorded[name] = map[string]bool{}
		}
		for _, param := range slices.Concat(row.Query, row.Body) {
			recorded[name][sdkoptions.Canonical(param)] = true
		}
	}
	return recorded
}

// sortedKeys returns a map's keys, sorted.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
