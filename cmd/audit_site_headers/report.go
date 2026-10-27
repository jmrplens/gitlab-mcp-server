// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// The levels a report line carries. Only levelFail fails the run.
const (
	levelOK      = "ok"
	levelFail    = "FAIL"
	levelPending = "pending"
	levelNotice  = "notice"
)

// page is what one fetch found: the URL, the status it answered or the error
// that kept it from answering, and the verdicts of a page that was judged.
type page struct {
	url      string
	status   int
	err      error
	verdicts []verdict
}

// judged reports whether the page answered 2xx, the only answer whose
// headers are judged.
func (p page) judged() bool {
	return p.err == nil && p.status >= http.StatusOK && p.status < http.StatusMultipleChoices
}

// line is one line of the report: its level, the title of the workflow
// command it becomes (the header, or the page when the page was not judged),
// and its text.
type line struct{ level, title, text string }

// pageLines reports a page: one line per verdict of a page that was judged,
// and a single failing line for one that was not.
func pageLines(p page) []line {
	if p.err != nil {
		return []line{{level: levelFail, title: p.url, text: fmt.Sprintf("%s could not be fetched: %v", p.url, p.err)}}
	}
	if !p.judged() {
		return []line{{level: levelFail, title: p.url, text: fmt.Sprintf("%s answered %d, so its headers were not judged", p.url, p.status)}}
	}
	lines := make([]line, 0, len(p.verdicts))
	for _, each := range p.verdicts {
		lines = append(lines, verdictLine(each))
	}
	return lines
}

// verdictLine words one verdict: the value of a header that holds, what to set
// for one that does not, what a pending one waits for, and, for a pending one
// already served, that its declaration should now say it is required.
func verdictLine(v verdict) line {
	name, served, pending := v.rule.name, strings.Join(v.served, ", "), v.rule.pending != ""
	if v.holds() {
		if pending {
			const noticeFormat = "%s: %s. It is served, and declared as waiting for %s: declare it required in cmd/audit_site_headers/headers.go and in the tests that pin the declarations, so that losing it fails this check"
			return line{level: levelNotice, title: name, text: fmt.Sprintf(noticeFormat, name, served, v.rule.pending)}
		}
		return line{level: levelOK, title: name, text: fmt.Sprintf("%s: %s", name, served)}
	}
	level, waits := levelFail, ""
	if pending {
		level = levelPending
	}
	if v.missing() {
		if pending {
			waits = "; it waits for " + v.rule.pending
		}
		return line{level: level, title: name, text: fmt.Sprintf("%s is not sent%s. Set: %s", name, waits, v.rule.recommended)}
	}
	if pending {
		waits = ", and it waits for " + v.rule.pending
	}
	return line{level: level, title: name, text: fmt.Sprintf("%s is permissive: it %s%s. Served: %s. Set: %s", name, v.reason, waits, served, v.rule.recommended)}
}

// workflowCommands maps a level to the GitHub Actions command it is annotated
// with; an ok line is not annotated.
var workflowCommands = map[string]string{levelFail: "error", levelPending: "warning", levelNotice: "notice"}

// writeReport prints each judged page's status and its verdicts indented
// under it, each page not judged on a line at the margin, and the count, and
// then, when annotate is set, one workflow command per line that is not ok.
func writeReport(w io.Writer, pages []page, annotate bool) {
	var commands []string
	for _, each := range pages {
		indent := ""
		if each.judged() {
			_, _ = fmt.Fprintf(w, "%s answered %d\n", each.url, each.status)
			indent = "  "
		}
		for _, reported := range pageLines(each) {
			_, _ = fmt.Fprintf(w, "%s%-8s %s\n", indent, reported.level, reported.text)
			command, annotated := workflowCommands[reported.level]
			if !annotated {
				continue
			}
			message := reported.text
			if each.judged() {
				message = each.url + ": " + message
			}
			commands = append(commands, fmt.Sprintf("::%s title=%s::%s", command, escapeProperty(reported.title), escapeData(message)))
		}
	}
	_, _ = fmt.Fprintf(w, "%s: %s\n", toolName, countLine(pages))
	if annotate {
		for _, command := range commands {
			_, _ = fmt.Fprintln(w, command)
		}
	}
}

// countLine counts the pages, the headers that hold, what fails (a required
// header that does not hold, and a page not judged) and what waits for the
// maintainer's rule.
func countLine(pages []page) string {
	held, failures, waiting := 0, 0, 0
	for _, each := range pages {
		if !each.judged() {
			failures++
		}
		for _, judged := range each.verdicts {
			switch {
			case judged.holds():
				held++
			case judged.fails():
				failures++
			default:
				waiting++
			}
		}
	}
	return fmt.Sprintf("%d page(s), %d header(s) held, %d failed, %d waiting for the maintainer's rule", len(pages), held, failures, waiting)
}

// failed reports whether the run fails: a page not judged, or a required
// header that does not hold.
func failed(pages []page) bool {
	for _, each := range pages {
		if !each.judged() {
			return true
		}
		for _, judged := range each.verdicts {
			if judged.fails() {
				return true
			}
		}
	}
	return false
}

// writeSummary writes the report as Markdown for a job summary: a table with
// a row per line, and the count beneath it.
func writeSummary(w io.Writer, pages []page) {
	_, _ = fmt.Fprint(w, "## Documentation site security headers\n\n| Page | Header | State | Detail |\n| --- | --- | --- | --- |\n")
	for _, each := range pages {
		for _, reported := range pageLines(each) {
			header := ""
			if each.judged() {
				header = reported.title
			}
			_, _ = fmt.Fprintf(w, "| %s | %s | %s | %s |\n", tableCell(each.url), tableCell(header), reported.level, tableCell(reported.text))
		}
	}
	_, _ = fmt.Fprintf(w, "\n%s\n", countLine(pages))
}

// tableCell escapes the pipes of a value put in a Markdown table cell.
func tableCell(value string) string { return strings.ReplaceAll(value, "|", `\|`) }

// dataEscaper and propertyEscaper encode what the GitHub Actions runner would
// read as part of a workflow command: in its data, the percent sign and line
// breaks, and in a property also the colon and the comma that separate
// properties.
var (
	dataEscaper     = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	propertyEscaper = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
)

// escapeData escapes the data of a workflow command.
func escapeData(value string) string { return dataEscaper.Replace(value) }

// escapeProperty escapes a property of a workflow command.
func escapeProperty(value string) string { return propertyEscaper.Replace(value) }
