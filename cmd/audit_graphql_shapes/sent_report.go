package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// oracleNone is what this dimension has to say about a tier and about a
// deprecation: nothing, and on purpose.
const oracleNone = "none"

// tierOracleReason says why a finding carries no tier, which is the one
// annotation a reader of the REST list is used to and will look for here.
const tierOracleReason = "The pinned schema declares no tier. GitLab gates a GraphQL field at resolve time, " +
	"so an unlicensed instance answers null or an error rather than omitting the field from the schema, and " +
	"nine of the eleven GraphQL-only domains are Premium or Ultimate surfaces: most of these findings sit on " +
	"licensed ground and nothing here can tell a field every instance sends from one only Ultimate sends. The " +
	"tier struct tags on the sibling output types are a hint and not an oracle. A tier is therefore omitted " +
	"from a finding rather than emitted empty, since an always-empty field reads as a condition that was " +
	"checked and found absent."

// deprecationOracleReason says why nothing here can exclude a field GitLab is
// removing, and names the fix rather than leaving the hole unexplained.
const deprecationOracleReason = "The pinned SDL carries no @deprecated directive and no description, because " +
	"cmd/internal/graphqlintrospect drops both on decode: validation never consults them and they would triple " +
	"the file. So a field GitLab has deprecated reads here as a gap, and a field GitLab is removing could be " +
	"added on the strength of one. The data is already on the wire at pin time, since an instance answers the " +
	"canned introspection payload with isDeprecated and deprecationReason whatever was asked for, and the fix " +
	"is a sidecar record written by gen_graphql_schema beside the SDL rather than a fatter SDL. Until it " +
	"exists, a deprecated field is answered by a declaration citing GitLab's own documentation."

// gatingReason says which sub-class of this dimension fails a build, since
// every other finding is a candidate for the surface rather than a defect in
// it.
const gatingReason = "This dimension reports and does not gate: a field GitLab offers and this server does not " +
	"surface is a candidate for the surface, GitLab adds fields weekly, and gating would break the build on " +
	"every re-pin over findings the missing tier and deprecation oracles cannot fully classify. One sub-class " +
	"gates, in the sibling shape check rather than here: a mutation payload whose errors no field of the " +
	"decoder reads, which drops GitLab's account of a refused mutation and reports success. The condition is " +
	"the decoder and not the document, since a payload that selects its errors and decodes none loses them " +
	"just as completely, while a decoder field the document never selects is already a hard failure of the " +
	"always-empty leg."

// sentOracles records what this check could not consult, once on the check
// rather than once on every finding.
type sentOracles struct {
	// Schema names the schema the walk judged by.
	Schema string `json:"schema"`
	// Grain names what a finding is read at, and so the extent of the claim
	// each one makes.
	Grain string `json:"grain"`
	// Pairings counts the document-and-decoder pairs walked, which is more
	// than the documents: one document sent by two callers is judged against
	// each decoder.
	Pairings int `json:"pairings"`
	// Coverage says what became of every object position the walk entered,
	// which is what a reader needs to tell the surface asked about from the
	// surface reached. Its counters are broken out rather than summed because
	// each skip is a different silence.
	Coverage sentCoverage `json:"coverage"`
	// Uncovered names the GraphQL this walk never saw at all.
	Uncovered sentUncovered `json:"uncovered"`
	// TierOracle and DeprecationOracle are both "none", each with the reason.
	TierOracle              string `json:"tier_oracle"`
	TierOracleReason        string `json:"tier_oracle_reason"`
	DeprecationOracle       string `json:"deprecation_oracle"`
	DeprecationOracleReason string `json:"deprecation_oracle_reason"`
	// Gating says which sub-class fails a build.
	Gating string `json:"gating"`
}

// sentSummary counts the findings the way a reader triages them.
type sentSummary struct {
	Findings    int `json:"findings"`
	Undeclared  int `json:"undeclared"`
	Leaf        int `json:"leaf"`
	Object      int `json:"object"`
	Collection  int `json:"collection"`
	Always      int `json:"always"`
	Nullable    int `json:"nullable"`
	Packages    int `json:"packages"`
	SchemaTypes int `json:"schema_types"`
	Occurrences int `json:"occurrences"`
}

// sentReport is the whole answer of this dimension, written where a reader of
// the field-by-field review can open it.
type sentReport struct {
	Check              sentOracles `json:"check"`
	Summary            sentSummary `json:"summary"`
	Sent               []sentField `json:"sent,omitempty"`
	UnusedDeclarations []string    `json:"unused_declarations,omitempty"`
}

// newSentReport assembles the report from one run's findings.
func newSentReport(schema string, pairings int, coverage sentCoverage, uncovered sentUncovered, found []sentField, unused []string) sentReport {
	return sentReport{
		Check: sentOracles{
			Schema:                  schema,
			Grain:                   sentGrain,
			Pairings:                pairings,
			Coverage:                coverage,
			Uncovered:               uncovered,
			TierOracle:              oracleNone,
			TierOracleReason:        tierOracleReason,
			DeprecationOracle:       oracleNone,
			DeprecationOracleReason: deprecationOracleReason,
			Gating:                  gatingReason,
		},
		Summary:            summarizeSent(found),
		Sent:               found,
		UnusedDeclarations: unused,
	}
}

// summarizeSent counts one run's findings by class, by nullability, and by
// how many of them a reader is asked to act on.
func summarizeSent(found []sentField) sentSummary {
	summary := sentSummary{Findings: len(found)}
	packages, schemaTypes := map[string]bool{}, map[string]bool{}
	for _, field := range found {
		packages[field.Package] = true
		schemaTypes[field.SchemaType] = true
		summary.Occurrences += field.Occurrences
		if !field.declared() {
			summary.Undeclared++
		}
		switch field.Class {
		case sentLeaf:
			summary.Leaf++
		case sentObject:
			summary.Object++
		case sentCollection:
			summary.Collection++
		}
		switch field.Sent {
		case sentAlways:
			summary.Always++
		case sentNullable:
			summary.Nullable++
		}
	}
	summary.Packages, summary.SchemaTypes = len(packages), len(schemaTypes)
	return summary
}

// writeSentReport writes the report where -report names.
func writeSentReport(path string, report sentReport) error {
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("render the sent report: %w", err)
	}
	if written := os.WriteFile(path, append(encoded, '\n'), 0o600); written != nil {
		return fmt.Errorf("write the sent report: %w", written)
	}
	return nil
}

// sentLine is what the run prints about this dimension, which says enough to
// decide whether to open the report.
//
// Two lines rather than one, because the second is the part a count of
// findings hides: a reader told what was found and not what was never asked
// reads the first line as the whole GraphQL surface.
func sentLine(path string, summary sentSummary, uncovered sentUncovered) string {
	line := fmt.Sprintf("%s %d field(s) the schema offers that no document of their package selects, %d undeclared, across %d package(s) and %d schema type(s) -> %s\n",
		prefix, summary.Findings, summary.Undeclared, summary.Packages, summary.SchemaTypes, path)
	if uncovered.Unavailable != "" {
		return line + fmt.Sprintf("%s %s\n", prefix, uncovered.Unavailable)
	}
	return line + fmt.Sprintf("%s not asked of %d GraphQL operation(s) in %d package(s) whose documents client-go builds; see uncovered in %s\n",
		prefix, uncovered.Operations, len(uncovered.Packages), path)
}
