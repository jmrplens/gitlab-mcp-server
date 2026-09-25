package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Finding is one place where the register and the code disagree.
type Finding struct {
	// Rule is the rule that found it, "G8".
	Rule string `json:"rule"`
	// Subject is what the finding is about: a row ("ADM-002"), a declaration
	// ("cmd/server:readinessGate.abandoned"), a failure of the table
	// ("cmd/server:bearerGuard.check missing-credential") or a register value.
	Subject string `json:"subject"`
	// Position is where in the source it is, "cmd/server/auth_gate.go:506", or
	// empty when the finding is about a declaration that matches nothing.
	Position string `json:"position,omitempty"`
	// Message says what is wrong, in terms a reviewer can act on.
	Message string `json:"message"`
}

// String renders a finding as one line.
func (f Finding) String() string {
	if f.Position == "" {
		return fmt.Sprintf("%s %s: %s", f.Rule, f.Subject, f.Message)
	}
	return fmt.Sprintf("%s %s: %s: %s", f.Rule, f.Subject, f.Position, f.Message)
}

// Summary is what a run looked at, so that a clean report says what it was
// clean over rather than only that it was clean.
type Summary struct {
	// Rows are the register's decisions, and Failures the rows of its
	// authentication failure table.
	Rows     int `json:"rows"`
	Failures int `json:"failures"`
	// Sites are the distinct declarations the register names, and Packages
	// the packages the run loaded after the test-support ones were left out.
	Sites    int `json:"sites"`
	Packages int `json:"packages"`
	// Returns, Refusals, Reasons and Settings are what the rules that read
	// behavior out of the code actually read: the refusal returns G7 matched
	// to the failure table, the refusals G8 held to their text and literal,
	// the reasons G9 found in their comments, and the variables G14 checked.
	// A rule that reads nothing passes by matching nothing, and these say it
	// did not.
	Returns  int `json:"returns"`
	Refusals int `json:"refusals"`
	Reasons  int `json:"reasons"`
	Settings int `json:"settings"`
	// Findings, Pending and Exempted are the three things a reader needs:
	// what is wrong, which rows' binding checks are deferred, and how many
	// judgements the exemption table is standing in for.
	Findings int `json:"findings"`
	Pending  int `json:"pending"`
	Exempted int `json:"exempted"`
}

// Excuse is one declaration the exemption table answered, and why.
type Excuse struct {
	// Key is the exempted declaration, "cmd/server:corsMaxAge".
	Key string `json:"key"`
	// Part is which part of G10 it answered: "names" or "literals".
	Part string `json:"part"`
	// Category and Reason are the exemption's own.
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

// Report is the whole answer.
type Report struct {
	Summary  Summary   `json:"summary"`
	Findings []Finding `json:"findings"`
	// Pending are the rows whose alias, argument and orphan checks (G2, G3,
	// G6) are deferred until the layer that moves their values lands.
	Pending []string `json:"pending"`
	// Excused are the declarations the exemption table answered this run.
	Excused []Excuse `json:"excused,omitempty"`
}

// ok reports whether the run found nothing to answer for.
func (r Report) ok() bool {
	return len(r.Findings) == 0
}

// write renders the report: every finding on its own line, the pending rows,
// what the exemption table answered when verbose, and one summary line.
func (r Report) write(out io.Writer, verbose bool) {
	for _, finding := range r.Findings {
		fmt.Fprintln(out, finding.String())
	}
	if len(r.Pending) > 0 {
		fmt.Fprintf(out, "pending (G2, G3 and G6 deferred until the layer that moves their values): %s\n",
			strings.Join(r.Pending, ", "))
	}
	if verbose {
		for _, excuse := range r.Excused {
			fmt.Fprintf(out, "%s: not a decision (%s, %s): %s\n", excuse.Key, excuse.Part, excuse.Category, excuse.Reason)
		}
	}
	fmt.Fprintf(out, "%s: %d rows, %d failures and %d declared sites over %d packages "+
		"(%d refusal returns, %d refusals, %d reasons and %d settings read); "+
		"%d findings, %d rows pending, %d declarations exempted\n",
		toolName, r.Summary.Rows, r.Summary.Failures, r.Summary.Sites, r.Summary.Packages,
		r.Summary.Returns, r.Summary.Refusals, r.Summary.Reasons, r.Summary.Settings,
		r.Summary.Findings, r.Summary.Pending, r.Summary.Exempted)
}

// sortFindings puts findings in a stable order, by rule number and then by
// subject, position and message, so two runs over one tree print one report,
// and drops exact repeats: a site several rows name is judged once per row
// that names it, and one defect is one line.
func sortFindings(found []Finding) []Finding {
	slices.SortFunc(found, func(a, b Finding) int {
		fileA, lineA := splitPosition(a.Position)
		fileB, lineB := splitPosition(b.Position)
		return cmp.Or(
			cmp.Compare(ruleNumber(a.Rule), ruleNumber(b.Rule)),
			strings.Compare(a.Subject, b.Subject),
			strings.Compare(fileA, fileB),
			cmp.Compare(lineA, lineB),
			strings.Compare(a.Message, b.Message),
		)
	})
	return slices.Compact(found)
}

// splitPosition is a position's file and line, so line 100 sorts after line
// 99. A position with no line is its file alone.
func splitPosition(position string) (file string, line int) {
	i := strings.LastIndexByte(position, ':')
	if i < 0 {
		return position, 0
	}
	line, _ = strconv.Atoi(position[i+1:])
	return position[:i], line
}

// ruleNumber is the number of a rule name, "G10" is 10, so G10 sorts after G9.
func ruleNumber(rule string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(rule, "G"))
	return n
}
