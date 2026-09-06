package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// writeReport prints the human report: the findings grouped by the package
// that has to act on them, then what the sweep saw.
//
// Verbose adds the two buckets that are not failures. The excused one is
// printed so an exemption stays reviewable, and the unresolved one because a
// value the audit could not follow is the audit's own blind spot rather than a
// clean answer.
func writeReport(out io.Writer, report Report, verbose bool) {
	writeGroups(out, "", report.Findings)
	if verbose {
		writeGroups(out, "excused by directive: ", report.Excused)
		writeGroups(out, "unresolved: ", report.Unresolved)
	}
	writeStale(out, report.StaleDirectives)
	writeSummary(out, report.Summary)
}

// writeGroups prints findings under one `=== package ===` heading each.
func writeGroups(out io.Writer, prefix string, findings []Finding) {
	current := ""
	for _, finding := range findings {
		if finding.Package != current {
			current = finding.Package
			fmt.Fprintf(out, "=== %s%s ===\n", prefix, current)
		}
		fmt.Fprintf(out, "  %s:%d %s %s %s %s\n", finding.File, finding.Line, orDash(finding.Func),
			finding.Context, finding.Verb, finding.Expression)
		fmt.Fprintf(out, "      wants %s: %s\n", finding.Wants, finding.Reason)
	}
}

// writeStale prints the exemptions that excused nothing.
func writeStale(out io.Writer, stale []Directive) {
	if len(stale) == 0 {
		return
	}
	fmt.Fprintf(out, "=== directives that excuse nothing ===\n")
	for _, directive := range stale {
		fmt.Fprintf(out, "  %s:%d %s %s\n", directive.File, directive.Line, directive.Package, directive.Expression)
		fmt.Fprintf(out, "      %s no longer reaches a Markdown construct unescaped. Remove the directive.\n", directive.Expression)
	}
}

// writeSummary prints what the sweep saw, with the two breakdowns the walk
// through the findings is organized by.
func writeSummary(out io.Writer, summary Summary) {
	fmt.Fprintf(out, "%s: %d value(s) unescaped in %d package(s); %d excused; %d unresolved; %d stale directive(s)\n",
		toolName, summary.Findings, summary.Packages, summary.Excused, summary.Unresolved, summary.Stale)
	fmt.Fprintf(out, "  judged %d value(s) in %d Markdown sink(s), %d already safe; contexts: %s\n",
		summary.Holes, summary.Sinks, summary.Safe, summary.Contexts)
	if len(summary.ByContext) > 0 {
		fmt.Fprintf(out, "  by context: %s\n", byCount(summary.ByContext))
	}
}

// byCount renders a breakdown, biggest first, so the report opens with where
// the work is.
func byCount(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}

// orDash renders an empty field as a dash, so a column never disappears.
func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// writeJSON writes the machine-readable work list.
//
// It takes the report as any so that the encoding failure is a path a test can
// reach: a gate that cannot write its work list has to say so rather than
// report a clean run.
func writeJSON(path string, report any) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if writeErr := os.WriteFile(path, append(data, '\n'), 0o600); writeErr != nil {
		return fmt.Errorf("write %s: %w", path, writeErr)
	}
	return nil
}

// relativePath renders a file below the repository root, so a finding reads
// the same wherever the audit was run from.
func relativePath(path, root string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}
