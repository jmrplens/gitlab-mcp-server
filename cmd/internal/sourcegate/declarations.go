// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sourcegate

import (
	"slices"
	"sort"
	"strings"
)

// Declaration is one entry of a gate's declaration table: why the function it
// names may do what the gate otherwise reports, in a category and in words.
type Declaration struct {
	Category string
	Reason   string
}

// Table is a gate's declaration table with the categories an entry may give,
// each with what it means.
//
// An entry is keyed by the package the repository names, a colon, and the
// function as its file spells it ([FuncDeclName]), which is also how a finding
// names where it sits, so an entry survives an edit above the line it excuses.
type Table struct {
	Declared   map[string]Declaration
	Categories map[string]string
}

// UnknownCategories names the declarations whose category is not one of the
// table's, in a stable order. A category nobody defined is an excuse nobody
// reviewed, so such an entry is reported rather than trusted, whatever it
// excuses.
func (t Table) UnknownCategories() []string {
	var unknown []string
	for key, entry := range t.Declared {
		if _, known := t.Categories[entry.Category]; !known {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// staleDeclarations names the entries that excused nothing this run, out of
// the ones this run was in a position to judge, in a stable order: a
// declaration that has stopped describing the tree is itself a finding.
//
// An entry naming a package the run never loaded is passed over rather than
// reported: a run narrowed to one package would otherwise condemn every
// declaration outside it, which would teach a reader to ignore the line.
func (t Table) staleDeclarations(excused, scanned map[string]struct{}) []string {
	var stale []string
	for key := range t.Declared {
		if _, used := excused[key]; used {
			continue
		}
		pkg, _, _ := strings.Cut(key, ":")
		if _, looked := scanned[pkg]; !looked {
			continue
		}
		stale = append(stale, key)
	}
	sort.Strings(stale)
	return stale
}

// Verdict is the part of a gate's report its declaration table and its load
// decide: the findings the table does not excuse and the ones it does, the
// declarations that excused nothing or give a category nobody defined, and the
// files the load left out that could hold what the gate looks for.
//
// A gate's report embeds it beside a summary of its own, so the fields read as
// the report's, and [Verdict.OK] is the report's verdict.
type Verdict[F any] struct {
	// Findings are the findings no declaration excuses, which fail the gate.
	Findings []F `json:"findings"`
	// Excused are the findings a declaration answers, kept so a verbose run
	// can show what the table is standing in front of.
	Excused []F `json:"excused,omitempty"`
	// Stale are the declarations that excused nothing this run.
	Stale []string `json:"stale_declarations,omitempty"`
	// Unknown are the declarations naming a category nobody defined.
	Unknown []string `json:"unknown_categories,omitempty"`
	// Unjudged are the files the load left out that could hold what the gate
	// judges, whose contents this run could not see.
	Unjudged []string `json:"unjudged_files,omitempty"`
}

// Judge holds a run's findings against a declaration table: each one whose key
// the table declares is excused, every other one is reported, both in the
// order given, and the table is then held to what this run scanned. The
// unjudged files are reported sorted.
func Judge[F any](findings []F, key func(F) string, table Table, scanned map[string]struct{}, unjudged []string) Verdict[F] {
	var verdict Verdict[F]
	excused := map[string]struct{}{}
	for _, finding := range findings {
		k := key(finding)
		if _, allowed := table.Declared[k]; allowed {
			excused[k] = struct{}{}
			verdict.Excused = append(verdict.Excused, finding)
			continue
		}
		verdict.Findings = append(verdict.Findings, finding)
	}
	verdict.Stale = table.staleDeclarations(excused, scanned)
	verdict.Unknown = table.UnknownCategories()
	verdict.Unjudged = slices.Sorted(slices.Values(unjudged))
	return verdict
}

// OK reports whether the run found nothing to answer for: no finding the table
// does not excuse, no declaration that excused nothing or gives an undefined
// category, and no file the load hid.
func (v Verdict[F]) OK() bool {
	return len(v.Findings) == 0 && len(v.Stale) == 0 && len(v.Unknown) == 0 && len(v.Unjudged) == 0
}

// Counts are a verdict's figures, which a gate's summary embeds so a clean
// report says what it was clean over and a failing one how much it found.
type Counts struct {
	Findings int `json:"findings"`
	Excused  int `json:"excused"`
	Stale    int `json:"stale_declarations"`
	Unknown  int `json:"unknown_categories"`
	Unjudged int `json:"unjudged_files"`
}

// Counts returns the verdict's figures.
func (v Verdict[F]) Counts() Counts {
	return Counts{
		Findings: len(v.Findings),
		Excused:  len(v.Excused),
		Stale:    len(v.Stale),
		Unknown:  len(v.Unknown),
		Unjudged: len(v.Unjudged),
	}
}
