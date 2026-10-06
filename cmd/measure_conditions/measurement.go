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
// script reads it: a total of zero is no measurement, and the script refuses
// it before it could be recorded as one.
var figurePattern = regexp.MustCompile(`^Condition coverage: (\d+)/([1-9]\d*)$`)

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
	covered    string
	total      string
	conditions []condition
	held       int
}

// parseMeasurement reads the output of a run whose status says it measured.
//
// The figure must be there, since the script refuses to pass a run without
// one. A condition the script held is printed twice, once in gobco's report
// and once indented under the gate's verdict, and is marked held. A held
// line gobco's report does not name, or a status that disagrees with whether
// anything was held, means the output is not what the script writes, and is
// refused rather than tabulated.
func parseMeasurement(output string, status int) (measurement, error) {
	var found measurement
	figure := false
	heldLines := map[string]bool{}
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if match := figurePattern.FindStringSubmatch(line); match != nil {
			found.covered, found.total, figure = match[1], match[2], true
			continue
		}
		if rest, indented := strings.CutPrefix(line, heldIndent); indented {
			if conditionPattern.MatchString(rest) {
				heldLines[rest] = true
			}
			continue
		}
		if match := conditionPattern.FindStringSubmatch(line); match != nil {
			found.conditions = append(found.conditions, condition{
				text: line,
				file: strings.ReplaceAll(match[1], `\`, "/"),
				line: match[2],
				code: unquoteCode(match[3]),
				says: match[4],
			})
		}
	}
	if !figure {
		return measurement{}, errors.New("the script's status says it measured, and its output carries no Condition coverage figure")
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
