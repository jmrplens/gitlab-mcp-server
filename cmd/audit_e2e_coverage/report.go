package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// report is the machine-readable result for one runtime.
type report struct {
	// Runtime is the edition/tier key.
	Runtime string `json:"runtime"`
	// Edition and Tier are its two halves.
	Edition string `json:"edition"`
	Tier    string `json:"tier"`
	// Directory is where the shards were read from.
	Directory string `json:"directory"`
	// Runs is one row per package that started or refused.
	Runs []runRow `json:"runs"`
	// Sessions is one row per surface and mode that served something.
	Sessions []sessionRow `json:"sessions"`
	// Summary is the counts a reader looks at first.
	Summary summary `json:"summary"`
	// Levels lists the actions at each level, in the default mode.
	Levels levels `json:"levels"`
	// Actions is one row per catalog action with its default-mode state on
	// each surface.
	Actions []actionRow `json:"actions"`
	// Cells is every surface x mode x action cell that is not absent.
	Cells []cellRow `json:"cells"`
	// Capabilities is the non-tool classification.
	Capabilities map[string][]cellRow `json:"capabilities"`
	// DispatchMismatches lists every call whose dispatched action differed
	// from the one it named.
	DispatchMismatches []mismatch `json:"dispatch_mismatches"`
	// UnresolvedTools lists every tool a call named that the session never
	// served.
	UnresolvedTools []unresolvedTool `json:"unresolved_tools"`
	// UncalledTools lists, per shape, the served tools no call named.
	UncalledTools []uncalledRow `json:"uncalled_tools"`
	// Diagnostics is about the record rather than about coverage.
	Diagnostics diagnostics `json:"diagnostics"`
	// Results is the results join, when -results was given.
	Results *resultsJoin `json:"results,omitempty"`
	// Check is the -check verdict for this runtime, when asked for.
	Check *checkResult `json:"check,omitempty"`
	// Baseline is the superset comparison, when -baseline was given.
	Baseline *baselineResult `json:"baseline,omitempty"`
}

// runRow is one package's run line.
type runRow struct {
	Package     string                  `json:"package"`
	Requirement string                  `json:"requirement"`
	Status      string                  `json:"status"`
	Reason      string                  `json:"reason,omitempty"`
	Filter      string                  `json:"filter,omitempty"`
	RunID       string                  `json:"run_id"`
	Fixtures    e2ecalls.FixtureProfile `json:"fixtures"`
}

// sessionRow is one surface and mode, with what its sessions served.
type sessionRow struct {
	Surface           string `json:"surface"`
	Mode              string `json:"mode"`
	Sessions          int    `json:"sessions"`
	Tools             int    `json:"tools"`
	Resources         int    `json:"resources"`
	ResourceTemplates int    `json:"resource_templates"`
	Prompts           int    `json:"prompts"`
	DispatchObserved  bool   `json:"dispatch_observed"`
}

// summary is the headline of a runtime.
type summary struct {
	// CatalogActions is the size of the catalog the runtime serves.
	CatalogActions int `json:"catalog_actions"`
	// TestCalls is how many test-purpose calls were recorded.
	TestCalls int `json:"test_calls"`
	// L1, L2 and L3 count the actions at each level.
	L1 int `json:"l1"`
	L2 int `json:"l2"`
	L3 int `json:"l3"`
	// States is the histogram of states per surface, in the default mode.
	States map[string]map[state]int `json:"states"`
	// Capabilities is the histogram of states per capability kind.
	Capabilities map[string]map[state]int `json:"capabilities"`
}

// levels lists the actions at each level in the default mode.
type levels struct {
	// L1 is asserted on any surface.
	L1 []string `json:"l1"`
	// L2 is asserted on the dynamic surface, which is the default one.
	L2 []string `json:"l2"`
	// L3 is asserted on all three surfaces.
	L3 []string `json:"l3"`
}

// actionRow is one catalog action with its state on each surface, in the
// default mode.
type actionRow struct {
	ID     string           `json:"id"`
	Domain string           `json:"domain"`
	Tier   string           `json:"tier"`
	States map[string]state `json:"states"`
	L1     bool             `json:"l1"`
	L2     bool             `json:"l2"`
	L3     bool             `json:"l3"`
}

// cellRow is one cell as published.
type cellRow struct {
	Surface string   `json:"surface"`
	Mode    string   `json:"mode"`
	Target  string   `json:"target"`
	State   state    `json:"state"`
	Reason  string   `json:"reason,omitempty"`
	Tests   []string `json:"tests,omitempty"`
	Calls   int      `json:"calls,omitempty"`
	// Delivered is set on a subscription row when a resource-updated
	// notification for the kind reached a passing test.
	Delivered bool `json:"delivered,omitempty"`
}

// uncalledRow lists the served tools of one shape that no call named.
type uncalledRow struct {
	Surface string   `json:"surface"`
	Mode    string   `json:"mode"`
	Tools   []string `json:"tools"`
}

// buildReport publishes a classification.
func buildReport(c *classification) *report {
	rep := &report{
		Runtime:            c.rt.key,
		Edition:            c.rt.edition,
		Tier:               c.rt.tier.String(),
		Directory:          c.rt.dir,
		Runs:               runRows(c.rt),
		Sessions:           sessionRows(c),
		Capabilities:       map[string][]cellRow{},
		DispatchMismatches: c.mismatches,
		UnresolvedTools:    c.unresolved,
		UncalledTools:      uncalledRows(c),
		Diagnostics:        c.diagnostics,
	}
	rep.Actions, rep.Levels = actionRows(c)
	rep.Cells = cellRows(c.cells)
	for kind, cells := range c.capabilities {
		rep.Capabilities[kind] = cellRows(cells)
	}
	for i, row := range rep.Capabilities[capabilitySubscriptions] {
		// A subscription is accepted at subscribe time and proven at
		// delivery; the row says which it reached.
		rep.Capabilities[capabilitySubscriptions][i].Delivered = c.delivered[cellKey{
			shape: shapeKey{surface: row.Surface, mode: row.Mode}, action: row.Target,
		}]
	}
	rep.Summary = summarize(c, rep)
	return rep
}

// runRows publishes the run lines.
func runRows(rt *runtimeRecords) []runRow {
	rows := make([]runRow, 0, len(rt.runs))
	for _, run := range rt.runs {
		rows = append(rows, runRow{
			Package: run.Package, Requirement: run.Requirement, Status: run.Status, Reason: run.Reason,
			Filter: run.Filter, RunID: run.RunID, Fixtures: run.Fixtures,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Package < rows[j].Package })
	return rows
}

// sessionRows publishes the shapes.
func sessionRows(c *classification) []sessionRow {
	rows := make([]sessionRow, 0, len(c.shapes))
	for _, shape := range c.shapes {
		rows = append(rows, sessionRow{
			Surface: shape.key.surface, Mode: shape.key.mode, Sessions: shape.sessions,
			Tools: len(shape.tools), Resources: len(shape.resources), ResourceTemplates: len(shape.templates),
			Prompts: len(shape.prompts), DispatchObserved: shape.observed,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Surface != rows[j].Surface {
			return surfaceOrder(rows[i].Surface) < surfaceOrder(rows[j].Surface)
		}
		return rows[i].Mode < rows[j].Mode
	})
	return rows
}

// surfaceOrder puts the surfaces in the order the levels read them: the
// default surface first.
func surfaceOrder(surface string) int {
	switch surface {
	case config.ToolSurfaceDynamic:
		return 0
	case config.ToolSurfaceMeta:
		return 1
	case config.ToolSurfaceIndividual:
		return 2
	default:
		return 3
	}
}

// allSurfaces lists the three surfaces in level order.
var allSurfaces = []string{config.ToolSurfaceDynamic, config.ToolSurfaceMeta, config.ToolSurfaceIndividual}

// actionRows publishes one row per catalog action and settles the levels.
func actionRows(c *classification) ([]actionRow, levels) {
	rows := make([]actionRow, 0, len(c.catalog.ids))
	var lv levels
	for _, id := range c.catalog.ids {
		action := c.catalog.actions[id]
		row := actionRow{ID: id, Domain: action.domain, Tier: action.tier.String(), States: map[string]state{}}
		asserted := 0
		for _, surface := range allSurfaces {
			found, ran := c.cells[cellKey{shape: shapeKey{surface: surface, mode: modeDefault}, action: id}]
			if !ran {
				continue
			}
			row.States[surface] = found.state
			if found.state == stateAsserted {
				asserted++
			}
		}
		row.L1 = asserted > 0
		row.L2 = row.States[config.ToolSurfaceDynamic] == stateAsserted
		row.L3 = asserted == len(allSurfaces)
		if row.L1 {
			lv.L1 = append(lv.L1, id)
		}
		if row.L2 {
			lv.L2 = append(lv.L2, id)
		}
		if row.L3 {
			lv.L3 = append(lv.L3, id)
		}
		rows = append(rows, row)
	}
	return rows, lv
}

// cellRows publishes every cell that is not absent, in a stable order.
func cellRows(cells map[cellKey]*cell) []cellRow {
	rows := make([]cellRow, 0, len(cells))
	for _, found := range cells {
		if found.state == stateAbsent {
			continue
		}
		calls := 0
		for _, n := range found.counts {
			calls += n
		}
		rows = append(rows, cellRow{
			Surface: found.key.shape.surface, Mode: found.key.shape.mode, Target: found.key.action,
			State: found.state, Reason: found.reason, Tests: found.bestTests(), Calls: calls,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Surface != rows[j].Surface {
			return surfaceOrder(rows[i].Surface) < surfaceOrder(rows[j].Surface)
		}
		if rows[i].Mode != rows[j].Mode {
			return rows[i].Mode < rows[j].Mode
		}
		return rows[i].Target < rows[j].Target
	})
	return rows
}

// uncalledRows lists, per shape, the served tools nothing called.
func uncalledRows(c *classification) []uncalledRow {
	rows := make([]uncalledRow, 0, len(c.shapes))
	for _, shape := range c.shapes {
		var uncalled []string
		for tool := range shape.tools {
			if !c.called[shape.key][tool] {
				uncalled = append(uncalled, tool)
			}
		}
		sort.Strings(uncalled)
		rows = append(rows, uncalledRow{Surface: shape.key.surface, Mode: shape.key.mode, Tools: uncalled})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Surface != rows[j].Surface {
			return surfaceOrder(rows[i].Surface) < surfaceOrder(rows[j].Surface)
		}
		return rows[i].Mode < rows[j].Mode
	})
	return rows
}

// summarize counts what the rows say.
func summarize(c *classification, rep *report) summary {
	s := summary{
		CatalogActions: len(c.catalog.ids),
		L1:             len(rep.Levels.L1),
		L2:             len(rep.Levels.L2),
		L3:             len(rep.Levels.L3),
		States:         map[string]map[state]int{},
		Capabilities:   map[string]map[state]int{},
	}
	for _, call := range c.rt.calls {
		if call.Purpose == e2ecalls.PurposeTest {
			s.TestCalls++
		}
	}
	for _, found := range c.cells {
		if found.key.shape.mode != modeDefault {
			continue
		}
		if s.States[found.key.shape.surface] == nil {
			s.States[found.key.shape.surface] = map[state]int{}
		}
		s.States[found.key.shape.surface][found.state]++
	}
	for kind, cells := range c.capabilities {
		s.Capabilities[kind] = map[state]int{}
		for _, found := range cells {
			s.Capabilities[kind][found.state]++
		}
	}
	return s
}

// writeJSON writes the reports as one JSON document: a list, since a run may
// cover several runtimes.
func writeJSON(w io.Writer, reports []*report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", " ")
	if err := encoder.Encode(reports); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

// writeGapTSV writes the work list the old gap audit used to: one row per
// action not asserted on any surface, then a summary line.
func writeGapTSV(w io.Writer, rep *report) {
	for _, row := range rep.Actions {
		if row.L1 {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\tdynamic=%s\tmeta=%s\tindividual=%s\n", row.ID, row.Domain, row.Tier,
			stateOr(row.States[config.ToolSurfaceDynamic]), stateOr(row.States[config.ToolSurfaceMeta]),
			stateOr(row.States[config.ToolSurfaceIndividual]))
	}
	fmt.Fprintf(w, "e2e coverage %s: L1 %d/%d (%.1f%%), L2 %d, L3 %d, %d actions not asserted on any surface\n",
		rep.Runtime, rep.Summary.L1, rep.Summary.CatalogActions, percent(rep.Summary.L1, rep.Summary.CatalogActions),
		rep.Summary.L2, rep.Summary.L3, rep.Summary.CatalogActions-rep.Summary.L1)
}

// stateOr spells a state, and names a surface no session ran on.
func stateOr(s state) string {
	if s == "" {
		return "no-session"
	}
	return string(s)
}

// percent is a share as a percentage, and zero of nothing.
func percent(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return 100 * float64(part) / float64(whole)
}

// writeMarkdownSummary writes the summary a CI step puts in
// GITHUB_STEP_SUMMARY: one section per runtime with the headline counts,
// the state histogram per surface, the capability histogram and the verdicts.
func writeMarkdownSummary(w io.Writer, reports []*report) {
	fmt.Fprintln(w, "## E2E coverage")
	for _, rep := range reports {
		fmt.Fprintf(w, "\n### %s\n\n", rep.Runtime)
		fmt.Fprintf(w, "- Catalog actions: %d\n", rep.Summary.CatalogActions)
		fmt.Fprintf(w, "- L1 (asserted on any surface): %d (%.1f%%)\n", rep.Summary.L1, percent(rep.Summary.L1, rep.Summary.CatalogActions))
		fmt.Fprintf(w, "- L2 (asserted on dynamic): %d\n", rep.Summary.L2)
		fmt.Fprintf(w, "- L3 (asserted on all three surfaces): %d\n", rep.Summary.L3)
		fmt.Fprintf(w, "- Test calls: %d; dispatch mismatches: %d; unresolved tools: %d\n",
			rep.Summary.TestCalls, len(rep.DispatchMismatches), len(rep.UnresolvedTools))
		writeStateTable(w, "Surface", rep.Summary.States)
		writeStateTable(w, "Capability", rep.Summary.Capabilities)
		writeVerdicts(w, rep)
	}
}

// markdownStates is the column order of a state table.
var markdownStates = []state{
	stateAsserted, stateUnobserved, stateSweepOnly, stateErrorPathOnly, stateRefusedOnly, statePreviewOnly,
	stateCleanupOnly, stateUnasserted, stateUnservable, stateSkipped, stateFailed, stateAbsent,
}

// writeStateTable writes one histogram table.
func writeStateTable(w io.Writer, first string, histogram map[string]map[state]int) {
	if len(histogram) == 0 {
		return
	}
	fmt.Fprintf(w, "\n| %s |", first)
	for _, s := range markdownStates {
		fmt.Fprintf(w, " %s |", s)
	}
	fmt.Fprint(w, "\n| --- |")
	for range markdownStates {
		fmt.Fprint(w, " ---: |")
	}
	fmt.Fprintln(w)
	keys := make([]string, 0, len(histogram))
	for key := range histogram {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if surfaceOrder(keys[i]) != surfaceOrder(keys[j]) {
			return surfaceOrder(keys[i]) < surfaceOrder(keys[j])
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		fmt.Fprintf(w, "| %s |", key)
		for _, s := range markdownStates {
			fmt.Fprintf(w, " %d |", histogram[key][s])
		}
		fmt.Fprintln(w)
	}
}

// writeVerdicts writes the check and baseline verdicts when the run asked
// for them.
func writeVerdicts(w io.Writer, rep *report) {
	if rep.Check != nil {
		fmt.Fprintf(w, "\n- Check: %s\n", verdictWord(rep.Check.Passed))
		for _, finding := range rep.Check.Findings {
			fmt.Fprintf(w, "  - %s\n", finding)
		}
	}
	if rep.Baseline != nil {
		fmt.Fprintf(w, "\n- Baseline: %s (%d reached in the baseline, %d lost)\n",
			verdictWord(len(rep.Baseline.Lost) == 0), rep.Baseline.BaselineReached, len(rep.Baseline.Lost))
		for _, lost := range rep.Baseline.Lost {
			fmt.Fprintf(w, "  - %s\n", lost)
		}
	}
}

// verdictWord spells a pass or a failure.
func verdictWord(passed bool) string {
	if passed {
		return "passed"
	}
	return "FAILED"
}
