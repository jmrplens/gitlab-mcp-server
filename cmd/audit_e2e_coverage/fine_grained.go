package main

import (
	"cmp"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// A fine-grained session is folded apart from every other (issue 952).
//
// The cells, the levels and the histograms above are a statement about what a
// classic credential is served, and the committed record and its floors are
// read against them. A session on a fine-grained token is served a narrower
// surface on purpose, and its calls are refused by design where its grant
// does not reach: folded in, its listing would sit in the union of what a
// shape served without saying so, and a refusal it was meant to get would
// stand in a cell beside the classic call of the same action. So its lines
// are kept out of all of that and counted here, per surface x mode x action,
// with what came back, and -check holds a run to the floors recorded for
// them.

// fineGrainedReport is what the fine-grained sessions of one runtime served
// and what their calls answered.
type fineGrainedReport struct {
	// Sessions counts the session lines of a fine-grained credential.
	Sessions int `json:"sessions"`
	// Shapes is one row per surface and mode those sessions ran as, with the
	// tools they were listed.
	Shapes []fineGrainedShapeRow `json:"shapes"`
	// Calls counts the tools/call lines of those sessions.
	Calls int `json:"calls"`
	// Asserted counts the cells a passing test saw run, and Refused the ones
	// a passing test saw refused for the credential, with the reason the
	// server counts that refusal under.
	Asserted int `json:"asserted"`
	Refused  int `json:"refused"`
	// Cells is one row per surface x mode x action a fine-grained session
	// called, sorted.
	Cells []fineGrainedCellRow `json:"cells"`
}

// fineGrainedShapeRow is one surface and mode a fine-grained session ran as.
type fineGrainedShapeRow struct {
	Surface  string `json:"surface"`
	Mode     string `json:"mode"`
	Sessions int    `json:"sessions"`
	// Tools is the union of what those sessions were listed, which on the
	// individual surface is narrower than a classic session's of the same
	// shape by every action the credential cannot reach.
	Tools int `json:"tools"`
}

// fineGrainedCellRow is one surface x mode x action a fine-grained session
// called, with how its calls ended. A refusal is counted under the reason the
// server gave it, so the fine-grained refusal a test was written to see is
// told apart from one for another cause.
type fineGrainedCellRow struct {
	Surface string `json:"surface"`
	Mode    string `json:"mode"`
	Action  string `json:"action"`
	// Asserted counts the calls a passing test saw run.
	Asserted int `json:"asserted,omitempty"`
	// Refused counts the refusals a passing test saw, by the class the call
	// was recorded under: the server's own withholding as fine_grained, and
	// GitLab's 403 or 404 for a call the server let through, which is what a
	// scenario asserts for a grant that does not reach the object, as
	// forbidden or not_found.
	Refused map[string]int `json:"refused,omitempty"`
	// ErrorPath counts the errors a passing test wanted that carry no class:
	// a tool error the harness names no refusal for, or a JSON-RPC error.
	ErrorPath int `json:"error_path,omitempty"`
	// Unjudged counts the calls of a test that did not pass, one whose span
	// never arrived, or one that came back in a way that says nothing about
	// the action.
	Unjudged int      `json:"unjudged,omitempty"`
	Tests    []string `json:"tests"`
}

// fineGrainedFold collects the fine-grained lines while the classic fold runs.
type fineGrainedFold struct {
	sessions int
	shapes   map[shapeKey]*fineGrainedShape
	calls    int
	cells    map[shapeKeyAction]*fineGrainedCellRow
}

// fineGrainedShape is what the fine-grained sessions of one surface and mode
// listed.
type fineGrainedShape struct {
	sessions int
	tools    map[string]bool
}

// shapeKeyAction names one fine-grained cell.
type shapeKeyAction struct {
	shape  shapeKey
	action string
}

// newFineGrainedFold returns an empty fold.
func newFineGrainedFold() *fineGrainedFold {
	return &fineGrainedFold{shapes: map[shapeKey]*fineGrainedShape{}, cells: map[shapeKeyAction]*fineGrainedCellRow{}}
}

// isFineGrained reports whether a line was written by a session on a
// fine-grained credential. A line with no kind is classic, which is every
// line written before the harness recorded one.
func isFineGrained(kind string) bool {
	return e2ecalls.CredentialKind(kind) == e2ecalls.CredentialFineGrained
}

// foldSession counts one fine-grained session line.
func (f *fineGrainedFold) foldSession(session *e2ecalls.Session) {
	f.sessions++
	key := shapeKey{surface: session.Surface, mode: session.Mode}
	shape := f.shapes[key]
	if shape == nil {
		shape = &fineGrainedShape{tools: map[string]bool{}}
		f.shapes[key] = shape
	}
	shape.sessions++
	markAll(shape.tools, session.Tools)
}

// foldCall counts one fine-grained call line. Only a tools/call names an
// action; the other methods a fine-grained session makes are not what the
// grant decides, and are counted nowhere.
func (f *fineGrainedFold) foldCall(call *e2ecalls.Call) {
	if call.Method != methodCallTool {
		return
	}
	f.calls++
	earned, target := creditOf(call)
	if target == "" {
		return
	}
	key := shapeKeyAction{shape: shapeKey{surface: call.Surface, mode: call.Mode}, action: target}
	row := f.cells[key]
	if row == nil {
		row = &fineGrainedCellRow{Surface: call.Surface, Mode: call.Mode, Action: target}
		f.cells[key] = row
	}
	if !slices.Contains(row.Tests, call.Test) {
		row.Tests = append(row.Tests, call.Test)
	}
	// A call whose span never named the action it ran is unjudged here, as a
	// classic cell calls it unobserved: the action is the test's word for it,
	// not the server's.
	switch earned {
	case creditAsserted:
		row.Asserted++
	case creditRefused:
		if row.Refused == nil {
			row.Refused = map[string]int{}
		}
		row.Refused[strings.TrimPrefix(call.Outcome, e2ecalls.OutcomeRefusedPrefix)]++
	case creditErrorPath:
		row.ErrorPath++
	default:
		row.Unjudged++
	}
}

// report publishes the fold, nil when no fine-grained session ran, so a
// report of a run that started none says nothing about fine-grained tokens
// rather than reporting zeros a reader would take for a measurement.
func (f *fineGrainedFold) report() *fineGrainedReport {
	if f.sessions == 0 && len(f.cells) == 0 {
		return nil
	}
	rep := &fineGrainedReport{Sessions: f.sessions, Calls: f.calls, Shapes: []fineGrainedShapeRow{}, Cells: []fineGrainedCellRow{}}
	for key, shape := range f.shapes {
		rep.Shapes = append(rep.Shapes, fineGrainedShapeRow{
			Surface: key.surface, Mode: key.mode, Sessions: shape.sessions, Tools: len(shape.tools),
		})
	}
	slices.SortFunc(rep.Shapes, func(a, b fineGrainedShapeRow) int {
		return compareShapes(a.Surface, a.Mode, b.Surface, b.Mode)
	})
	for _, row := range f.cells {
		slices.Sort(row.Tests)
		rep.Cells = append(rep.Cells, *row)
		if row.Asserted > 0 {
			rep.Asserted++
		}
		if row.Refused[toolutil.RefusalFineGrained] > 0 {
			rep.Refused++
		}
	}
	slices.SortFunc(rep.Cells, func(a, b fineGrainedCellRow) int {
		if order := compareShapes(a.Surface, a.Mode, b.Surface, b.Mode); order != 0 {
			return order
		}
		return cmp.Compare(a.Action, b.Action)
	})
	return rep
}

// fineGrainedFloor is the least a complete run of a runtime must show of its
// fine-grained sessions for -check to pass.
type fineGrainedFloor struct {
	// Asserted is the fewest cells a passing test saw run.
	Asserted int
	// Refused is the fewest cells a passing test saw refused for the
	// credential, under the fine_grained reason.
	Refused int
}

// checkFineGrained applies the fine-grained floors recorded for the runtime,
// under every selector it matches, to a live run's report. It is not applied
// to the committed record, which carries no fine-grained section: the floors
// are a statement about what a run did, and the record's own figures are the
// classic ones.
func checkFineGrained(rep *report, selectors []string, result *checkResult) {
	for _, selector := range floorSelectors(rep.Runtime, selectors) {
		floor, declared := fineGrainedFloors[selector]
		if !declared {
			continue
		}
		if rep.FineGrained == nil {
			result.failf("no session on a fine-grained token ran on %s, and %s records a floor for them", rep.Runtime, selector)
			continue
		}
		if rep.FineGrained.Asserted < floor.Asserted {
			result.failf("%d actions run by a fine-grained session on %s, below the floor of %d recorded for %s",
				rep.FineGrained.Asserted, rep.Runtime, floor.Asserted, selector)
		}
		if rep.FineGrained.Refused < floor.Refused {
			result.failf("%d actions refused to a fine-grained session for its credential on %s, below the floor of %d recorded for %s",
				rep.FineGrained.Refused, rep.Runtime, floor.Refused, selector)
		}
	}
}
