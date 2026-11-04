// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The exit statuses of scripts/coverage-conditions.sh that mean a measurement
// was taken: the gate held nothing, or the gate held at least one condition.
// Every other status means the script could not measure.
const (
	statusMeasured = 0
	statusGateHeld = 3
)

// figurePattern is the line gobco ends its report on, read exactly as the
// script reads it: opened with the words of the mode it counted in, and a
// total of zero is no measurement, which the script refuses before it could
// be recorded as one.
var figurePattern = regexp.MustCompile(`^(Condition coverage|Branch coverage): (\d+)/([1-9]\d*)$`)

// coverageMode is one GOBCO_MODE scripts/coverage-conditions.sh reads: the
// words gobco opens its figure with in that mode, and the sentence a summary
// says what the figures count in.
type coverageMode struct {
	figure string
	counts string
}

// coverageModes are the GOBCO_MODE values the script reads. condition is
// gobco's default, which counts every atomic condition, each operand of &&
// and || and each comparison wherever it is written. branch is gobco's
// -branch, which counts the whole controlling condition of each if and each
// for, and each case of a switch.
var coverageModes = map[string]coverageMode{
	"condition": {
		figure: "Condition coverage",
		counts: "gobco counts each condition twice, once per outcome, so a total is twice the number of conditions.",
	},
	"branch": {
		figure: "Branch coverage",
		counts: "gobco counts each decision twice, once per outcome: the whole condition of an `if` or a `for`, and each case of a `switch`, so a total is twice the number of decisions.",
	},
}

// conditionPattern is a condition gobco names as not evaluated both ways, in
// the three shapes its report writes one in, with the file first:
//
//	main.go:91:5: condition "x != \"\"" was 3 times true but never false
//	main.go:91:5: condition "x != \"\"" was once false but never true
//	main.go:91:5: condition "x != \"\"" was never evaluated
//
// The quoted code is matched greedily up to the last `" was `, which is where
// it ends, since what gobco says after it holds no quote.
var conditionPattern = regexp.MustCompile(`^(.+\.go):(\d+):\d+: condition (".*") was (never evaluated|.* but never (?:true|false))$`)

// heldIndent is how the script prints a condition its gate held, under the
// line that says how many there were.
const heldIndent = "  "

// condition is one condition gobco named as not evaluated both ways.
type condition struct {
	text string
	file string
	line string
	code string
	says string
	held bool
}

// measurement is what one run's output says: gobco's figure, the outcomes
// evaluated of the outcomes there are, and the conditions it named.
type measurement struct {
	covered    int
	total      int
	conditions []condition
	held       int
}

// parseMeasurement reads the output of a run whose status says it measured,
// in the mode the runs were asked for, which is a key of coverageModes.
//
// The figure must be there in the words of the mode, since the script
// refuses to pass a run without one; a figure in the other mode's words is
// not the measurement asked for, and is passed over like any other line. A
// condition the script held is printed twice, once in gobco's report and
// once indented under the gate's verdict, and is marked held. A held line
// gobco's report does not name, or a status that disagrees with whether
// anything was held, means the output is not what the script writes, and is
// refused rather than tabulated.
//
// The line measurePackage writes for a package with nothing to measure in
// the mode stands for the figure 0 of 0, and is the whole of what such a run
// leaves: beside a figure or a condition it is an output nothing writes, and
// is refused. In the other mode's words it is passed over, as that mode's
// figure is.
func parseMeasurement(output string, status int, mode string) (measurement, error) {
	label := coverageModes[mode].figure
	read := scanOutput(output, mode)
	found := measurement{conditions: read.conditions}
	figure, nothing, heldLines := read.figure, read.nothing, read.heldLines
	if nothing && (figure != "" || len(found.conditions) > 0 || len(heldLines) > 0) {
		return measurement{}, errors.New("the output says there was nothing to measure, and carries a figure or a condition as well")
	}
	if figure == "" && !nothing {
		return measurement{}, fmt.Errorf("the script's status says it measured, and its output carries no %s figure", label)
	}
	if figure != "" {
		var err error
		if found.covered, found.total, err = readFigure(figure); err != nil {
			return measurement{}, err
		}
	}
	for i := range found.conditions {
		if heldLines[found.conditions[i].text] {
			found.conditions[i].held = true
			found.held++
		}
	}
	if found.held != len(heldLines) {
		return measurement{}, fmt.Errorf("the script held %d condition(s) and gobco's report names %d of them", len(heldLines), found.held)
	}
	if (status == statusGateHeld) != (found.held > 0) {
		return measurement{}, fmt.Errorf("the script exited %d and its gate held %d condition(s)", status, found.held)
	}
	return found, nil
}

// scanned is what the lines of one run's output say, before they are held to
// each other: the last figure in the words of the mode, whether the line that
// says there was nothing to measure in the mode is there, every condition
// gobco named, and the lines the gate printed as held.
type scanned struct {
	figure     string
	nothing    bool
	conditions []condition
	heldLines  map[string]bool
}

// scanOutput reads the output of a run line by line, in the mode the runs
// were asked for, and keeps what each line says (scanned.read).
func scanOutput(output, mode string) scanned {
	read := scanned{heldLines: map[string]bool{}}
	label := coverageModes[mode].figure
	for line := range strings.SplitSeq(output, "\n") {
		read.read(strings.TrimSuffix(line, "\r"), label, mode)
	}
	return read
}

// read keeps what one line says: a figure, which counts only in the words of
// the mode, label; the line saying there was nothing to measure, which counts
// only in the mode's own name; a condition the gate held, printed indented
// under its verdict; or a condition gobco named. Any other line says nothing.
func (s *scanned) read(line, label, mode string) {
	if match := figurePattern.FindStringSubmatch(line); match != nil {
		if match[1] == label {
			s.figure = line
		}
		return
	}
	if match := nothingPattern.FindStringSubmatch(line); match != nil {
		s.nothing = s.nothing || match[1] == mode
		return
	}
	if rest, indented := strings.CutPrefix(line, heldIndent); indented {
		if conditionPattern.MatchString(rest) {
			s.heldLines[rest] = true
		}
		return
	}
	if match := conditionPattern.FindStringSubmatch(line); match != nil {
		s.conditions = append(s.conditions, condition{
			text: line,
			file: strings.ReplaceAll(match[1], `\`, "/"),
			line: match[2],
			code: unquoteCode(match[3]),
			says: match[4],
		})
	}
}

// readFigure reads the two counts of a line figurePattern matched. A count
// too large for an int is refused rather than wrapped, since a total summed
// from it would be a number nobody measured; gobco prints counts of
// conditions, so no real report comes near that.
func readFigure(line string) (covered, total int, err error) {
	match := figurePattern.FindStringSubmatch(line)
	if covered, err = strconv.Atoi(match[2]); err != nil {
		return 0, 0, fmt.Errorf("the figure %q does not fit a count: %w", line, err)
	}
	if total, err = strconv.Atoi(match[3]); err != nil {
		return 0, 0, fmt.Errorf("the figure %q does not fit a count: %w", line, err)
	}
	return covered, total, nil
}

// lineBreak is a line break in a condition's code with the spaces and tabs
// around it, which is what a condition written across lines carries where it
// was broken: the end of one line, the break, and the next line's
// indentation. CommonMark ends a line at a carriage return as well as at a
// line feed, so either is one.
var lineBreak = regexp.MustCompile(`[ \t]*[\r\n]\s*`)

// unquoteCode returns the condition's code as written in the source, on one
// line. gobco prints it Go-quoted, and a quoting it does not produce is kept
// as printed. Each line break is then written as one space, together with
// the spaces and tabs around it, since a line break inside a table cell ends
// the row there. Nothing else is touched: a code span keeps the spaces and
// tabs inside it as written, so a string literal holding two spaces or a tab
// is shown holding them.
func unquoteCode(quoted string) string {
	code, err := strconv.Unquote(quoted)
	if err != nil {
		code = quoted
	}
	return lineBreak.ReplaceAllString(code, " ")
}
