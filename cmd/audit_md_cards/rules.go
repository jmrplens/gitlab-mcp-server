package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mdshape"
)

// allRules is the value of -rules that judges every shape rule.
const allRules = "all"

// selection is the set of rules one run judges.
//
// Staging the sweep by rule is a flag rather than a branch, for the reason
// -contexts is one in cmd/audit_md_escaping: the rules carry genuinely
// different strengths of claim and different sizes of backlog. A card field
// written inside a table costs the table its body and is a defect wherever it
// appears; a field written "**Label**: value" with no list marker costs the
// card its line breaks, is just as real, and is written that way in 24
// formatters, which is a sweep rather than a fix. A gate that shipped failing
// on all of them is a gate that gets turned off.
type selection struct {
	chosen map[mdshape.Kind]bool
	label  string
}

// parseRules turns the -rules value into the set of rules to judge.
//
// An unknown name is an error rather than an empty selection, because a
// misspelled rule would otherwise read as a gate that passed.
func parseRules(value string) (selection, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == allRules {
		chosen := make(map[mdshape.Kind]bool, len(mdshape.Kinds()))
		labels := make([]string, 0, len(mdshape.Kinds()))
		for _, kind := range mdshape.Kinds() {
			chosen[kind] = true
			labels = append(labels, string(kind))
		}
		return selection{chosen: chosen, label: strings.Join(labels, ", ")}, nil
	}

	chosen := map[mdshape.Kind]bool{}
	var labels []string
	for name := range strings.SplitSeq(value, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		kind, ok := ruleNamed(name)
		if !ok {
			return selection{}, fmt.Errorf("unknown shape rule %q: expected %s, or %s", name, allRules, ruleNames())
		}
		if !chosen[kind] {
			labels = append(labels, string(kind))
		}
		chosen[kind] = true
	}
	if len(chosen) == 0 {
		return selection{}, fmt.Errorf("no shape rule selected: expected %s, or %s", allRules, ruleNames())
	}
	return selection{chosen: chosen, label: strings.Join(labels, ", ")}, nil
}

// apply drops the findings this run does not judge, and records what it did
// judge so a report and a verdict can both say so.
func (s selection) apply(report Report) Report {
	kept := make([]Finding, 0, len(report.Findings))
	for _, finding := range report.Findings {
		if s.chosen[mdshape.Kind(finding.Kind)] {
			kept = append(kept, finding)
		}
	}
	report.Findings = kept
	report.Rules = s.label
	return report
}

// ruleNamed resolves a rule by the name a report prints for it.
func ruleNamed(name string) (mdshape.Kind, bool) {
	for _, kind := range mdshape.Kinds() {
		if string(kind) == name {
			return kind, true
		}
	}
	return "", false
}

// ruleNames lists the accepted -rules values, for the flag's own help and for
// the error a wrong one produces.
func ruleNames() string {
	names := make([]string, 0, len(mdshape.Kinds()))
	for _, kind := range mdshape.Kinds() {
		names = append(names, string(kind))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
