// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The record a client-go run writes where -report names, for the review of
// what client-go selects of what GitLab sends.

package main

import (
	"path/filepath"
	"slices"
	"strings"
)

// sdkGatingReason says what fails a client-go run, which is not what fails
// the run over our own documents.
//
// It and sdkReachReason are functions rather than constants because the
// mutation tool cannot see a package-level declaration and reads every string
// joined there as never covered.
func sdkGatingReason() string {
	return "Every finding of this run fails it unless a declaration answers it, in sdk_declarations.go for a " +
		"disagreement or an unjudged send and in sdk_declarations_sent.go for a sent field, and so does every " +
		"declaration that answers nothing. The surface is client-go's and small, an answer to a field it leaves out " +
		"is a declaration naming the upstream-bugs row that records the gap, and the tables are the record of the " +
		"one-time triage: a client-go bump that starts selecting a declared field leaves the declaration stale, and " +
		"one that stops selecting a field, or a re-pin that adds one, leaves a finding nothing answers, except on a " +
		"type declared whole rather than field by field, whose new fields that declaration already answers. The run reads the " +
		"client-go go.mod requires, so either fails the change that brings it."
}

// sdkReachReason says what reached means, which decides what the run judges.
func sdkReachReason() string {
	return "A client-go function is reached when this repository's packages under the audited patterns name it, or " +
		"name an interface method it implements, or when a reached client-go function names it in turn. A send in a " +
		"function nothing reaches is a document this server never puts on the wire, and is listed rather than judged."
}

// sdkTierOracleReason says why a client-go finding carries no tier. It is the
// reason the report over our own documents gives, less what that one says
// about our GraphQL-only domains, which are not what this run judges.
func sdkTierOracleReason() string {
	return "The pinned schema declares no tier. GitLab gates a GraphQL field at resolve time, so an unlicensed " +
		"instance answers null or an error rather than omitting the field from the schema, and nothing here can " +
		"tell a field every instance sends from one only a licensed instance sends. A tier is therefore omitted " +
		"from a finding rather than emitted empty, since an always-empty field reads as a condition that was " +
		"checked and found absent."
}

// sdkReport is the whole answer of a client-go run.
type sdkReport struct {
	Check sdkCheck `json:"check"`
	// Summary counts as the report over our own documents does, at this
	// run's grain: the package a sent row names is the client-go service it
	// is filed under, so packages counts services.
	Summary sentSummary `json:"summary"`
	// Sent are the fields the schema offers that no document of their service
	// selects, each with the declaration that answers it.
	Sent []sentField `json:"sent,omitempty"`
	// Disagreements and Unjudged are the shape findings and the unpairable
	// sends, each with the declaration that answers it.
	Disagreements []sdkShapeFinding `json:"disagreements,omitempty"`
	Unjudged      []sdkUnjudgedSend `json:"unjudged,omitempty"`
	// UnpairedDocuments names the documents client-go writes that no send
	// carries, which the run fails on.
	UnpairedDocuments []string `json:"unpaired_documents,omitempty"`
	// UnusedDeclarations names the declarations that answered nothing.
	UnusedDeclarations []string `json:"unused_declarations,omitempty"`
}

// sdkCheck records what a client-go run judged and by what.
type sdkCheck struct {
	Schema string `json:"schema"`
	// Module is the directory of the client-go the run read.
	Module string `json:"module"`
	Grain  string `json:"grain"`
	// Pairings counts the reached sends that were judged.
	Pairings int          `json:"pairings"`
	Coverage sentCoverage `json:"coverage"`
	// Reached names every client-go function holding a send this server
	// reaches, with the packages of ours that reach it, and NotReached every
	// one holding a send nothing here calls.
	Reached     []sdkReachedFunction `json:"reached"`
	NotReached  []string             `json:"not_reached,omitempty"`
	ReachReason string               `json:"reach_reason"`
	// The oracles and the gate, as the report over our own documents states
	// them.
	TierOracle              string `json:"tier_oracle"`
	TierOracleReason        string `json:"tier_oracle_reason"`
	DeprecationOracle       string `json:"deprecation_oracle"`
	DeprecationOracleReason string `json:"deprecation_oracle_reason"`
	Gating                  string `json:"gating"`
}

// sdkReachedFunction is one client-go function holding a judged send.
type sdkReachedFunction struct {
	Method string   `json:"method"`
	From   []string `json:"from"`
}

// newSDKReport assembles the report from one client-go run.
func newSDKReport(status sdkStatus) sdkReport {
	report := sdkReport{
		Check: sdkCheck{
			Schema:                  status.provenance,
			Module:                  filepath.ToSlash(status.dir),
			Grain:                   sdkGrain,
			Pairings:                len(status.pairings),
			Coverage:                status.coverage,
			NotReached:              status.unreached,
			ReachReason:             sdkReachReason(),
			TierOracle:              oracleNone,
			TierOracleReason:        sdkTierOracleReason(),
			DeprecationOracle:       oracleNone,
			DeprecationOracleReason: deprecationOracleReason(),
			Gating:                  sdkGatingReason(),
		},
		Summary:            summarizeSent(status.sent),
		Sent:               status.sent,
		Disagreements:      status.failures,
		Unjudged:           status.unjudged,
		UnusedDeclarations: status.stale,
	}
	for _, p := range status.pairings {
		method := shortName(p.Function)
		if !slices.ContainsFunc(report.Check.Reached, func(r sdkReachedFunction) bool { return r.Method == method }) {
			report.Check.Reached = append(report.Check.Reached, sdkReachedFunction{Method: method, From: status.reached[p.Function.FullName()]})
		}
	}
	slices.SortFunc(report.Check.Reached, func(a, b sdkReachedFunction) int { return strings.Compare(a.Method, b.Method) })
	for _, trouble := range status.unpaired {
		report.UnpairedDocuments = append(report.UnpairedDocuments, relative(trouble.Position, status.dir)+": "+trouble.Message)
	}
	return report
}
