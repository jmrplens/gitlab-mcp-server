package main

import (
	"strings"
	"testing"
)

// headerRows reads the column names out of every Markdown table the legend
// drew, in the order it drew them, so the test asks the rendered page what its
// columns are instead of repeating a list that could drift from it.
//
// The tables are matched by position rather than by their first cell, because
// every cell is padded to its column's width and a prefix match would break the
// day a model name grew a character.
func headerRows(t *testing.T, body string) [][]string {
	t.Helper()
	var rows [][]string
	for line := range strings.SplitSeq(body, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "| Model") {
			continue
		}
		var names []string
		for cell := range strings.SplitSeq(strings.Trim(strings.TrimSpace(line), "|"), "|") {
			if name := strings.TrimSpace(cell); name != "" && name != "Model" {
				names = append(names, name)
			}
		}
		rows = append(rows, names)
	}
	return rows
}

// TestLegend_GlossesEveryColumnItDraws is what keeps the worked example worked.
//
// The legend exists so a reader meets the definitions before the measurements.
// A column added to one of the three tables and forgotten here would appear in
// the example with no gloss beside it, which is worse than no example at all: a
// reader would take the silence for "this one is obvious". So the glosses are
// checked against the header the renderer actually drew, rather than against a
// list kept by hand.
func TestLegend_GlossesEveryColumnItDraws(t *testing.T) {
	body := legendBlock()
	drawnRows := headerRows(t, body)

	tables := []struct {
		name    string
		glosses []legendGloss
	}{
		{"published figures", figureGlosses},
		{"counted apart", apartGlosses},
		{"tokens", tokenGlosses},
	}
	if len(drawnRows) != len(tables) {
		t.Fatalf("the legend drew %d tables, want %d: a table added or removed needs its glosses", len(drawnRows), len(tables))
	}

	for i, one := range tables {
		t.Run(one.name, func(t *testing.T) {
			explained := make(map[string]bool, len(one.glosses))
			for _, gloss := range one.glosses {
				explained[gloss.Column] = true
			}
			drawn := drawnRows[i]
			if len(drawn) == 0 {
				t.Fatalf("the %s table drew no columns", one.name)
			}
			for _, column := range drawn {
				if !explained[column] {
					t.Errorf("the %s table draws %q and the legend does not say what it means", one.name, column)
				}
				delete(explained, column)
			}
			for column := range explained {
				t.Errorf("the legend explains %q, which the %s table does not draw", column, one.name)
			}
		})
	}
}

// TestLegend_SaysItsRowIsInvented keeps the example from being mistaken for a
// measurement: the model name is not one anybody can ask, and the prose says so
// before the first table.
func TestLegend_SaysItsRowIsInvented(t *testing.T) {
	body := legendBlock()
	if !strings.Contains(body, "The row here is invented") {
		t.Error("the legend does not say its row is invented")
	}
	if !strings.Contains(body, legendModel) {
		t.Errorf("the legend does not carry %q, so its row could be read as a real model", legendModel)
	}
}

// TestLegend_ExampleCaption_IsDrawnByTheComparisonRule keeps the page's account
// of comparability from drifting away from the rule that enforces it.
//
// The closing paragraph used to spell the list out by hand, and when case
// coverage was added to both keys the paragraph went on naming the old five.
// It now renders a real caption through crossVendorKey, so the two cannot
// disagree; this test holds that it really is the rendered one and that the
// dimension the hand-written list lost is in it.
func TestLegend_ExampleCaption_IsDrawnByTheComparisonRule(t *testing.T) {
	body := legendBlock()
	caption := crossVendorKey(legendRow()).String()

	if !strings.Contains(body, caption) {
		t.Fatalf("the legend does not carry the caption the comparison rule draws:\nwant %q", caption)
	}
	if !strings.Contains(caption, "cases") {
		t.Errorf("the example caption %q names no case coverage, so the example row covers the whole corpus "+
			"and cannot show a reader what a narrowed run looks like", caption)
	}
	if !strings.Contains(body, "tool schemas") {
		t.Error("the legend's caption carries no tool-schema fingerprint")
	}
}
