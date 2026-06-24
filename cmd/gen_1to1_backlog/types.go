package main

import "encoding/json"

// --- Input report shapes (subsets of the three auditor JSON documents) ---

// structReport mirrors the fields consumed from audit_struct_completeness.
type structReport struct {
	Packages []struct {
		Package            string          `json:"package"`
		MissingInputCount  int             `json:"missing_input_count"`
		MissingOutputCount int             `json:"missing_output_count"`
		Gaps               json.RawMessage `json:"gaps"`
	} `json:"packages"`
}

// actionReport mirrors the fields consumed from audit_action_coverage.
type actionReport struct {
	Services []struct {
		Service        string   `json:"service"`
		Packages       []string `json:"packages"`
		APIMethods     int      `json:"api_methods"`
		CoveredMethods int      `json:"covered_methods"`
		MissingMethods []string `json:"missing_methods"`
	} `json:"services"`
}

// metadataReport mirrors the fields consumed from audit_metadata_completeness.
type metadataReport struct {
	Packages []struct {
		Package  string        `json:"package"`
		Findings []metaFinding `json:"findings"`
	} `json:"packages"`
}

// metaFinding is one action's R-META flags, passed through to the backlog.
type metaFinding struct {
	Action string   `json:"action"`
	Tool   string   `json:"tool,omitempty"`
	Usage  string   `json:"usage,omitempty"`
	Flags  []string `json:"flags"`
}

// --- Merged backlog output shapes ---

// backlog is the merged per-package 1:1 audit backlog.
type backlog struct {
	SchemaVersion int              `json:"schema_version"`
	Note          string           `json:"note"`
	Summary       backlogSummary   `json:"summary"`
	Packages      []backlogPackage `json:"packages"`
}

type backlogSummary struct {
	Packages                      int `json:"packages"`
	StructMissingInput            int `json:"struct_missing_input"`
	StructMissingOutput           int `json:"struct_missing_output"`
	ActionMissingMethods          int `json:"action_missing_methods"`
	MetaGenericUsage              int `json:"meta_generic_usage"`
	MetaAliasesOnlyToolname       int `json:"meta_aliases_only_toolname"`
	MetaEmptyRelated              int `json:"meta_empty_related"`
	MetaWeakIndividualDescription int `json:"meta_weak_individual_description"`
}

// backlogPackage carries the three gap streams for one internal/tools package.
type backlogPackage struct {
	Package  string        `json:"package"`
	Struct   *structGaps   `json:"struct,omitempty"`
	Actions  []actionGap   `json:"actions,omitempty"`
	Metadata *metadataGaps `json:"metadata,omitempty"`
}

type structGaps struct {
	MissingInput  int             `json:"missing_input"`
	MissingOutput int             `json:"missing_output"`
	Gaps          json.RawMessage `json:"gaps,omitempty"`
}

type actionGap struct {
	Service        string   `json:"service"`
	APIMethods     int      `json:"api_methods"`
	CoveredMethods int      `json:"covered_methods"`
	MissingMethods []string `json:"missing_methods"`
}

type metadataGaps struct {
	GenericUsage              int           `json:"generic_usage"`
	AliasesOnlyToolname       int           `json:"aliases_only_toolname"`
	EmptyRelated              int           `json:"empty_related"`
	WeakIndividualDescription int           `json:"weak_individual_description"`
	Findings                  []metaFinding `json:"findings"`
}
