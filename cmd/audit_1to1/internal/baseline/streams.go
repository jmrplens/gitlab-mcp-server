// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package baseline

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// The streams, each named by its scope and the summary counter that counts
// it.
const (
	StructsMissingInputs    = "structs.missing_input_fields"
	StructsMissingOutputs   = "structs.missing_output_fields"
	StructsExtraOutputs     = "structs.extra_output_fields"
	StructsTypeMismatches   = "structs.type_mismatches"
	ActionsMissingMethods   = "actions.missing_methods"
	ParamsUnmodeled         = "params.unmodeled"
	ParamsNotOffered        = "params.not_offered"
	ParamsOfferedUnrecorded = "params.offered_unrecorded"
	ParamsUnjudged          = "params.unjudged"
	PathsUnpublished        = "paths.unpublished_fields"
	PathsNestedUnpublished  = "paths.typed_nested_unpublished_fields"
	PathsNestedUnsurfaced   = "paths.typed_nested_unsurfaced_fields"
	PathsAlwaysSent         = "paths.always_sent_but_optional"
	PathsIdentifierLeads    = "paths.identifier_leads"
	RequiredUnplaced        = "required.fields_unplaced"
	RequiredUnjudged        = "required.fields_unjudged"
	GrantsRowsNotDerived    = "grants.rows_not_derived"
	GrantsNotRecorded       = "grants.derived_not_recorded"
	EnumsUnsurfaced         = "enums.unsurfaced_output_fields"
)

// keySeparator joins the fields of a key. A route and a Go type can hold a
// space, so a space alone would not say where one field ends.
const keySeparator = " | "

// Reports are the native JSON reports the streams are read from, one per
// scope. Structs, Required and Enums are read from a full report, since
// -gaps-only drops from their lists the type mismatches of a package with no
// missing field, the unplaced fields and the enum fields with no gap; the
// other four list every finding of their streams in either mode.
type Reports struct {
	Structs  []byte
	Actions  []byte
	Params   []byte
	Paths    []byte
	Required []byte
	Grants   []byte
	Enums    []byte
}

// Build reads every stream out of reports. A stream that does not list as
// many findings as its report's summary counts is an error rather than a
// stream, since a baseline written from it would hold less than the tree.
func Build(reports Reports) (Baseline, error) {
	baseline := Baseline{SchemaVersion: SchemaVersion, Streams: map[string][]string{}}
	for _, scope := range []struct {
		name    string
		content []byte
		read    func([]byte) ([]stream, error)
	}{
		{name: "structs", content: reports.Structs, read: structsStreams},
		{name: "actions", content: reports.Actions, read: actionsStreams},
		{name: "params", content: reports.Params, read: paramsStreams},
		{name: "paths", content: reports.Paths, read: pathsStreams},
		{name: "required", content: reports.Required, read: requiredStreams},
		{name: "grants", content: reports.Grants, read: grantsStreams},
		{name: "enums", content: reports.Enums, read: enumsStreams},
	} {
		streams, err := scope.read(scope.content)
		if err != nil {
			return Baseline{}, fmt.Errorf("the %s report: %w", scope.name, err)
		}
		for _, read := range streams {
			if len(read.keys) != read.counted {
				return Baseline{}, fmt.Errorf("the %s report lists %d findings of %s where its summary counts %d: -gaps-only shortens some lists, and a field the report renamed lists none",
					scope.name, len(read.keys), read.name, read.counted)
			}
			baseline.Streams[read.name] = keySet(read.keys)
		}
	}
	return baseline, nil
}

// stream is one list a report holds: its name in the baseline, the key of
// each finding in the order the report lists them, a finding listed twice
// twice, and how many findings the report's summary counts for it.
type stream struct {
	name    string
	keys    []string
	counted int
}

// add appends the key the fields spell.
func (s *stream) add(fields ...string) {
	s.keys = append(s.keys, strings.Join(fields, keySeparator))
}

// keySet returns keys sorted and once each, an empty list rather than none,
// so a stream with no finding is written as [] and read back as one.
func keySet(keys []string) []string {
	set := append([]string{}, keys...)
	slices.Sort(set)
	return slices.Compact(set)
}

// summary is a report's summary block, read counter by counter, so a counter
// the report stops carrying is an error rather than a zero.
type summary map[string]json.RawMessage

// counts reads the named counters, failing on the first the summary does not
// carry as a count.
func (s summary) counts(names ...string) ([]int, error) {
	counted := make([]int, len(names))
	for i, name := range names {
		raw, ok := s[name]
		if !ok {
			return nil, fmt.Errorf("its summary carries no %s", name)
		}
		if err := json.Unmarshal(raw, &counted[i]); err != nil {
			return nil, fmt.Errorf("its summary's %s is not a count: %w", name, err)
		}
	}
	return counted, nil
}

// decode reads a report and the named counters of its summary, which every
// report keeps under the same key.
func decode(content []byte, report any, names ...string) ([]int, error) {
	var head struct {
		Summary summary `json:"summary"`
	}
	if err := json.Unmarshal(content, &head); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(content, report); err != nil {
		return nil, err
	}
	return head.Summary.counts(names...)
}

// tagged is a field of a struct pair, by its JSON tag.
type tagged struct {
	Tag string `json:"tag"`
}

// structsReport is what the structs scope's report holds of its streams.
type structsReport struct {
	Packages []struct {
		Package string `json:"package"`
		Gaps    []struct {
			Kind           string   `json:"kind"`
			MCPType        string   `json:"mcp_type"`
			SDKType        string   `json:"sdk_type"`
			MissingFields  []tagged `json:"missing_fields"`
			ExtraFields    []tagged `json:"extra_fields"`
			TypeMismatches []struct {
				Tag     string `json:"tag"`
				MCPType string `json:"mcp_type"`
				SDKType string `json:"sdk_type"`
			} `json:"type_mismatches"`
		} `json:"gaps"`
	} `json:"packages"`
}

// structsStreams reads the four field streams of the structs scope. A key is
// the package, the MCP struct and the client-go struct of the pair and the
// field's tag; a type mismatch adds whether the pair is an input or an output
// and the two types, since a mismatch is its two types.
func structsStreams(content []byte) ([]stream, error) {
	var report structsReport
	counted, err := decode(content, &report, "missing_input_fields", "missing_output_fields", "extra_output_fields", "type_mismatches")
	if err != nil {
		return nil, err
	}
	inputs := stream{name: StructsMissingInputs, counted: counted[0]}
	outputs := stream{name: StructsMissingOutputs, counted: counted[1]}
	extras := stream{name: StructsExtraOutputs, counted: counted[2]}
	mismatches := stream{name: StructsTypeMismatches, counted: counted[3]}
	for _, pkg := range report.Packages {
		for _, gap := range pkg.Gaps {
			missing := &outputs
			if gap.Kind == "input" {
				missing = &inputs
			}
			for _, field := range gap.MissingFields {
				missing.add(pkg.Package, gap.MCPType, gap.SDKType, field.Tag)
			}
			for _, field := range gap.ExtraFields {
				extras.add(pkg.Package, gap.MCPType, gap.SDKType, field.Tag)
			}
			for _, mismatch := range gap.TypeMismatches {
				mismatches.add(pkg.Package, gap.Kind, gap.MCPType, gap.SDKType, mismatch.Tag, mismatch.MCPType, mismatch.SDKType)
			}
		}
	}
	return []stream{inputs, outputs, extras, mismatches}, nil
}

// actionsStreams reads the methods the actions scope finds no handler
// reaching, each keyed by its client-go service and its name.
func actionsStreams(content []byte) ([]stream, error) {
	var report struct {
		Services []struct {
			Service        string   `json:"service"`
			MissingMethods []string `json:"missing_methods"`
		} `json:"services"`
	}
	counted, err := decode(content, &report, "missing_methods")
	if err != nil {
		return nil, err
	}
	methods := stream{name: ActionsMissingMethods, counted: counted[0]}
	for _, service := range report.Services {
		for _, method := range service.MissingMethods {
			methods.add(service.Service, method)
		}
	}
	return []stream{methods}, nil
}

// paramsStreams reads the four classes of the params scope's findings, each
// a stream keyed by the route and the param. A class no stream holds is an
// error, since its findings would be in no stream.
func paramsStreams(content []byte) ([]stream, error) {
	var report struct {
		Findings []struct {
			Route string `json:"route"`
			Param string `json:"param"`
			Class string `json:"class"`
		} `json:"findings"`
	}
	classes := []string{"unmodeled", "not_offered", "offered_unrecorded", "unjudged"}
	counted, err := decode(content, &report, classes...)
	if err != nil {
		return nil, err
	}
	streams := []stream{
		{name: ParamsUnmodeled, counted: counted[0]},
		{name: ParamsNotOffered, counted: counted[1]},
		{name: ParamsOfferedUnrecorded, counted: counted[2]},
		{name: ParamsUnjudged, counted: counted[3]},
	}
	for _, finding := range report.Findings {
		class := slices.Index(classes, finding.Class)
		if class < 0 {
			return nil, fmt.Errorf("it lists a finding of class %q, which no stream holds", finding.Class)
		}
		streams[class].add(finding.Route, finding.Param)
	}
	return streams, nil
}

// pathsField is an output field of the paths scope's shape comparisons.
type pathsField struct {
	Package string `json:"package"`
	Type    string `json:"type"`
	Under   string `json:"under"`
	Field   string `json:"field"`
	SDKType string `json:"sdk_type"`
}

// pathsReport is what the paths scope's report holds of its streams.
type pathsReport struct {
	Shapes struct {
		Ran         bool         `json:"ran"`
		Unpublished []pathsField `json:"unpublished"`
		Typed       struct {
			Ran               bool         `json:"ran"`
			NestedUnpublished []pathsField `json:"nested_unpublished"`
			NestedUnsurfaced  []pathsField `json:"nested_unsurfaced"`
		} `json:"typed"`
	} `json:"shapes"`
	AlwaysSent struct {
		Ran      bool `json:"ran"`
		Optional []struct {
			Package    string `json:"package"`
			Method     string `json:"method"`
			Path       string `json:"path"`
			OptionType string `json:"option_type"`
			Field      string `json:"field"`
		} `json:"optional_but_always_sent"`
	} `json:"always_sent"`
	Identifiers struct {
		Ran   bool `json:"ran"`
		Leads []struct {
			Package     string `json:"package"`
			Method      string `json:"method"`
			Path        string `json:"path"`
			Placeholder string `json:"placeholder"`
		} `json:"leads"`
	} `json:"identifier_values"`
}

// pathsStreams reads the five report-only lists of the paths scope. A
// section that did not run lists nothing and counts nothing, which would
// read as a clean stream, so it is an error. A package-grain field is keyed
// by its package, type and field, a nested one by the type it sits in, the
// key it sits under, the field and the client-go struct of its parent's
// pairing; an always-sent param by its package, request, option struct and
// field; a lead by its package, request and placeholder.
func pathsStreams(content []byte) ([]stream, error) {
	var report pathsReport
	counted, err := decode(content, &report, "unpublished_fields", "typed_nested_unpublished_fields", "typed_nested_unsurfaced_fields", "always_sent_but_optional")
	if err != nil {
		return nil, err
	}
	for _, section := range []struct {
		name string
		ran  bool
	}{
		{name: "shapes", ran: report.Shapes.Ran},
		{name: "shapes.typed", ran: report.Shapes.Typed.Ran},
		{name: "always_sent", ran: report.AlwaysSent.Ran},
		{name: "identifier_values", ran: report.Identifiers.Ran},
	} {
		if !section.ran {
			return nil, fmt.Errorf("its %s section did not run, so its findings cannot be read", section.name)
		}
	}
	unpublished := stream{name: PathsUnpublished, counted: counted[0]}
	for _, field := range report.Shapes.Unpublished {
		unpublished.add(field.Package, field.Type, field.Field)
	}
	nestedUnpublished := stream{name: PathsNestedUnpublished, counted: counted[1]}
	for _, field := range report.Shapes.Typed.NestedUnpublished {
		nestedUnpublished.add(field.Package, field.Type, field.Under, field.Field, field.SDKType)
	}
	nestedUnsurfaced := stream{name: PathsNestedUnsurfaced, counted: counted[2]}
	for _, field := range report.Shapes.Typed.NestedUnsurfaced {
		nestedUnsurfaced.add(field.Package, field.Type, field.Under, field.Field, field.SDKType)
	}
	alwaysSent := stream{name: PathsAlwaysSent, counted: counted[3]}
	for _, param := range report.AlwaysSent.Optional {
		alwaysSent.add(param.Package, param.Method+" "+param.Path, param.OptionType, param.Field)
	}
	leads := stream{name: PathsIdentifierLeads}
	for _, lead := range report.Identifiers.Leads {
		leads.add(lead.Package, lead.Method+" "+lead.Path, lead.Placeholder)
	}
	// The leads have no counter of their own; the section ran, which is what
	// can be asked of them.
	leads.counted = len(leads.keys)
	return []stream{unpublished, nestedUnpublished, nestedUnsurfaced, alwaysSent, leads}, nil
}

// actionFields are fields of one action.
type actionFields struct {
	Action string   `json:"action"`
	Fields []string `json:"fields"`
}

// requiredStreams reads the fields the required scope places on no route and
// the ones it cannot judge, each keyed by its action and field.
func requiredStreams(content []byte) ([]stream, error) {
	var report struct {
		Unplaced []actionFields `json:"unplaced"`
		Unjudged []actionFields `json:"unjudged"`
	}
	counted, err := decode(content, &report, "fields_unplaced", "fields_unjudged")
	if err != nil {
		return nil, err
	}
	unplaced := stream{name: RequiredUnplaced, counted: counted[0]}
	unjudged := stream{name: RequiredUnjudged, counted: counted[1]}
	for _, list := range []struct {
		into    *stream
		actions []actionFields
	}{{into: &unplaced, actions: report.Unplaced}, {into: &unjudged, actions: report.Unjudged}} {
		for _, action := range list.actions {
			for _, field := range action.Fields {
				list.into.add(action.Action, field)
			}
		}
	}
	return []stream{unplaced, unjudged}, nil
}

// packageRequest is a request of the grants scope's inventory cross-check.
type packageRequest struct {
	Package string `json:"package"`
	Request string `json:"request"`
}

// grantsStreams reads the two leads of the grants scope's cross-check of the
// derivation with the request inventory, each keyed by its package and
// request. Its summary counts what each side holds and how much of it the
// other reaches, so a lead count is the difference.
func grantsStreams(content []byte) ([]stream, error) {
	var report struct {
		InventoryCheck struct {
			NotDerived  []packageRequest `json:"rows_not_derived"`
			NotRecorded []packageRequest `json:"derived_not_recorded"`
		} `json:"inventory_check"`
	}
	counted, err := decode(content, &report,
		"inventory_rest_rows", "inventory_rest_rows_derived", "inventory_graphql_rows", "inventory_graphql_rows_derived",
		"derived_rest_requests", "derived_rest_requests_recorded", "derived_graphql_requests", "derived_graphql_requests_recorded")
	if err != nil {
		return nil, err
	}
	notDerived := stream{name: GrantsRowsNotDerived, counted: counted[0] - counted[1] + counted[2] - counted[3]}
	for _, row := range report.InventoryCheck.NotDerived {
		notDerived.add(row.Package, row.Request)
	}
	notRecorded := stream{name: GrantsNotRecorded, counted: counted[4] - counted[5] + counted[6] - counted[7]}
	for _, request := range report.InventoryCheck.NotRecorded {
		notRecorded.add(request.Package, request.Request)
	}
	return []stream{notDerived, notRecorded}, nil
}

// enumsStreams reads the output fields of an enum type whose schema names no
// value, each keyed by its action, MCP struct and field.
func enumsStreams(content []byte) ([]stream, error) {
	var report struct {
		Packages []struct {
			Findings []struct {
				Action  string `json:"action"`
				Kind    string `json:"kind"`
				MCPType string `json:"mcp_type"`
				Field   string `json:"field"`
				Source  string `json:"source"`
			} `json:"findings"`
		} `json:"packages"`
	}
	counted, err := decode(content, &report, "unsurfaced_output_fields")
	if err != nil {
		return nil, err
	}
	unsurfaced := stream{name: EnumsUnsurfaced, counted: counted[0]}
	for _, pkg := range report.Packages {
		for _, finding := range pkg.Findings {
			if finding.Kind == "output" && finding.Source == "none" {
				unsurfaced.add(finding.Action, finding.MCPType, finding.Field)
			}
		}
	}
	return []stream{unsurfaced}, nil
}
