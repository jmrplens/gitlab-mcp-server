// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package merge

import "encoding/json"

// --- Input report shapes (subsets of the three auditor JSON documents) ---

// structReport mirrors the fields consumed from the structs scope.
type structReport struct {
	// Summary is the stream's own count of the tree, which -gaps-only leaves
	// as it is while it drops the clean packages from Packages.
	Summary structSummary `json:"summary"`
	// StaleDeclarations is the struct stream's other kind of finding: a key of
	// one of its declaration tables that answered no candidate. It belongs to
	// no package the report lists, since the package a key names may be the
	// very thing that is gone, so it is carried beside the packages.
	StaleDeclarations []json.RawMessage `json:"stale_declarations"`
	Packages          []struct {
		Package            string          `json:"package"`
		MissingInputCount  int             `json:"missing_input_count"`
		MissingOutputCount int             `json:"missing_output_count"`
		ExtraOutputCount   int             `json:"extra_output_count"`
		Gaps               json.RawMessage `json:"gaps"`
	} `json:"packages"`
}

// structSummary is the part of the structs scope's summary the backlog
// repeats.
type structSummary struct {
	Packages            int `json:"packages"`
	MissingInputFields  int `json:"missing_input_fields"`
	MissingOutputFields int `json:"missing_output_fields"`
	ExtraOutputFields   int `json:"extra_output_fields"`
	TypeMismatches      int `json:"type_mismatches"`
}

// actionReport mirrors the fields consumed from the actions scope.
type actionReport struct {
	Services []struct {
		Service        string   `json:"service"`
		Packages       []string `json:"packages"`
		APIMethods     int      `json:"api_methods"`
		CoveredMethods int      `json:"covered_methods"`
		MissingMethods []string `json:"missing_methods"`
	} `json:"services"`
}

// enumReport mirrors the fields consumed from the enums scope.
type enumReport struct {
	// Summary is the stream's own count of the tree, which -gaps-only leaves
	// as it is.
	Summary  enumSummary `json:"summary"`
	Packages []struct {
		Package       string          `json:"package"`
		MissingValues int             `json:"missing_values"`
		ExtraValues   int             `json:"extra_values"`
		Findings      json.RawMessage `json:"findings"`
	} `json:"packages"`
}

// enumSummary is the part of the enums scope's summary the backlog repeats.
type enumSummary struct {
	MissingValues int `json:"missing_values"`
	ExtraValues   int `json:"extra_values"`
}

// --- Merged backlog output shapes ---

// backlog is the merged per-package 1:1 audit backlog.
type backlog struct {
	SchemaVersion int            `json:"schema_version"`
	Note          string         `json:"note"`
	Summary       backlogSummary `json:"summary"`
	// StructStaleDeclarations passes the struct stream's stale declarations
	// through verbatim, absent when there are none.
	StructStaleDeclarations []json.RawMessage `json:"struct_stale_declarations,omitempty"`
	Packages                []backlogPackage  `json:"packages"`
}

type backlogSummary struct {
	Packages                int `json:"packages"`
	PackagesWithGaps        int `json:"packages_with_gaps"`
	StructMissingInput      int `json:"struct_missing_input"`
	StructMissingOutput     int `json:"struct_missing_output"`
	StructExtraOutput       int `json:"struct_extra_output"`
	StructTypeMismatches    int `json:"struct_type_mismatches"`
	StructStaleDeclarations int `json:"struct_stale_declarations"`
	ActionMissingMethods    int `json:"action_missing_methods"`
	EnumMissingValues       int `json:"enum_missing_values"`
	EnumExtraValues         int `json:"enum_extra_values"`
}

// backlogPackage carries the three gap streams for one internal/tools package.
type backlogPackage struct {
	Package string      `json:"package"`
	Struct  *structGaps `json:"struct,omitempty"`
	Actions []actionGap `json:"actions,omitempty"`
	Enums   *enumGaps   `json:"enums,omitempty"`
}

// hasFindings reports whether the package carries a finding of any stream: a
// missing or extra field, a missing method or a value gap. A struct section
// whose counts are all zero is the census entry of a clean package, and the
// action and enum sections exist only for a finding.
func (p backlogPackage) hasFindings() bool {
	return p.Struct.hasFindings() || len(p.Actions) > 0 || p.Enums != nil
}

// enumGaps is the R-ENUM slice of one package: the value counts and the
// findings passed through verbatim.
type enumGaps struct {
	MissingValues int             `json:"missing_values"`
	ExtraValues   int             `json:"extra_values"`
	Findings      json.RawMessage `json:"findings,omitempty"`
}

type structGaps struct {
	MissingInput  int             `json:"missing_input"`
	MissingOutput int             `json:"missing_output"`
	ExtraOutput   int             `json:"extra_output"`
	Gaps          json.RawMessage `json:"gaps,omitempty"`
}

// hasFindings reports whether the section carries a missing or an extra
// field, which is what the struct stream itself counts a package with gaps
// by. A nil section carries none.
func (g *structGaps) hasFindings() bool {
	return g != nil && (g.MissingInput != 0 || g.MissingOutput != 0 || g.ExtraOutput != 0)
}

type actionGap struct {
	Service        string   `json:"service"`
	APIMethods     int      `json:"api_methods"`
	CoveredMethods int      `json:"covered_methods"`
	MissingMethods []string `json:"missing_methods"`
}
