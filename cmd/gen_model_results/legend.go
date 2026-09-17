// The worked example: one made-up row of each table on this page, with what
// every column means beside it.
//
// It exists because the tables below are dense and their columns are not
// self-explanatory: "unaided" and "clean" both sound like success, "overhead"
// is two different costs printed together, and every figure is a fraction
// rather than a rate on purpose. A reader meeting those for the first time
// halfway down a page of real numbers has to hold the definitions in their head
// while reading the measurements. Here the definitions come first, over numbers
// that are obviously invented, so the real tables can be read as tables.
//
// The example row is drawn by the same functions the real tables are drawn by,
// which is the property that keeps this honest: a column added to a table and
// forgotten here would appear in the example with no gloss rather than not
// appearing at all, and the gloss list is checked against the rendered header
// by TestLegend_GlossesEveryColumnItDraws.

package main

import (
	"strings"
)

// legendModel is the name the example row carries. It is not a model anybody
// can ask, which is the point: nobody should be able to mistake the example for
// a measurement.
const legendModel = "example:not-a-real-model"

// legendGloss is one column and what it means.
type legendGloss struct {
	Column string
	Means  string
}

// figureGlosses explain the published figures, in the order the table draws
// them.
var figureGlosses = []legendGloss{
	{"Clean", "The headline. Attempts that went right end to end **with no help at all**: the task finished, every step was reached, every argument the case gives a truth for matched, every destructive step carried its approval, nothing of ours had to refuse anything, and what it did to GitLab checked out afterwards. It is a conjunction of the columns after it, never an average of them: there is no defensible weighting between them, so a weighted score would be a reading of whoever chose the weights."},
	{"Reached", "Steps the model got to, or correctly declined, over the steps the case declares. A step is reached when a call named it and the server dispatched it."},
	{"Accepted first time", "Of the steps reached, how many were reached by the **first** call about them. A second call means the first was refused and repaired."},
	{"Argument fidelity", "Argument **values** compared against the truth the case declares, not argument names. A model that places `title` correctly and writes the wrong title fails here, which is the whole reason values are compared."},
	{"Confirmation", "Destructive steps whose reaching call carried the approval, over destructive steps declared. A deletion that ran without one is a finding, not a faster model."},
	{"Unaided", "Attempts that finished with no refusal of ours anywhere. Weaker than Clean: an attempt can be unaided and still have written a wrong value."},
	{"Completion", "Attempts that finished the task at all. Weakest of the three: it says the conversation ended correctly and nothing about how."},
	{"Overhead", "What getting there cost, per step reached, with the two halves apart. A `find` is the dynamic surface's declared cost, since the catalog is not in the tool list and has to be searched. An `invalid_params` is the model learning a parameter name from a rejection, which is what the opaque meta schema leaves it to do. One rate over both would hide each inside the other."},
}

// apartGlosses explain the counts that are published and then left out of every
// column.
var apartGlosses = []legendGloss{
	{"Attempts", "How many attempts are behind the figures above. The five columns after Turns are **not** in this number."},
	{"Turns", "Provider requests the row paid for, which is not the same as calls: one turn can carry several tool calls, and a refused request is a turn of its own."},
	{"Skipped", "The instance did not meet the case's needs, so it never ran. Counting it would rank a model by the license of the instance it was measured on."},
	{"Unobserved", "It ran and the server's span never described it, so nothing can be said about it either way."},
	{"Provider errors", "The provider would not answer."},
	{"Harness errors", "This side broke."},
	{"GitLab refused", "GitLab refused a call the model dispatched correctly, with the arguments the case declares. That reports the instance or the fixture, not the model."},
}

// tokenGlosses explain why the cost is four numbers rather than one.
var tokenGlosses = []legendGloss{
	{"Input", "Uncached input tokens."},
	{"Output", "Output tokens, reasoning included where the provider bills it there."},
	{"Cache created", "Input tokens written into the provider's prompt cache."},
	{"Cache read", "Input tokens served from it, billed at a fraction of the others."},
}

// legendBlock draws the whole worked example.
func legendBlock() string {
	// Assembled as sections joined by one blank line rather than by newlines
	// appended as it goes, because a table renderer that already ends in a
	// newline and a gloss list that also does add up to a blank line nobody
	// asked for, which markdownlint reports and a reader sees as a gap.
	example := legendRow()
	sections := []string{
		"**How to read the tables below.** Every figure is a numerator over a denominator " +
			"rather than a percentage, because a column reading 100% over one attempt and one reading 100% " +
			"over ninety are not the same claim and a rate cannot tell them apart. The row here is invented; " +
			"the columns are the real ones, drawn by the same code.",
		strings.TrimRight(columnsTable([]row{example}), "\n"),
		strings.TrimRight(glossList(figureGlosses), "\n"),
		strings.TrimRight(apartTable([]row{example}), "\n"),
		strings.TrimRight(glossList(apartGlosses), "\n"),
		strings.TrimRight(tokensTable([]row{example}), "\n"),
		strings.TrimRight(glossList(tokenGlosses), "\n"),
		"And one thing no column carries: a row is only comparable with another row that agreed with it " +
			"on everything a caption lists. That list is not written out here, it is drawn by the same " +
			"code that captions the real tables, so it cannot come to say less than the rule enforces:",
		"> Compared as one table because these rows agree on " + crossVendorKey(example).String() + ".",
		"Two tables with different captions are two measurements rather than two readings of one. Case " +
			"coverage is in there for a reason worth stating: the corpus fingerprint says which corpus was " +
			"asked, never how much of it, so a run narrowed to a handful of cases would otherwise sit beside " +
			"a full one under a caption claiming they agree.",
	}
	return strings.Join(sections, "\n\n")
}

// legendRow is the invented row the example is drawn from.
//
// The numbers are chosen so that every column says something different and the
// relationship between the three attempt-grained columns is visible: 7 of 10
// finished, 6 of those needed nothing from us, and only 5 were clean, so the
// gap between Completion and Clean is a real gap a reader can see rather than a
// sentence they have to take on trust.
func legendRow() row {
	return row{
		// A whole key, not only the model, because the caption below is drawn
		// from it and a key with holes in it renders as empty backticks rather
		// than as the sentence a reader is meant to learn to read.
		Key: rowKey{
			Model:            legendModel,
			Surface:          "dynamic",
			Mode:             "default",
			Tier:             "free",
			CorpusDigest:     "example-corpus",
			ContractDigest:   "example-contract",
			ToolSchemaDigest: "example-tools",
			Repeat:           1,
		},
		Counts: counts{
			Attempts: 10, Turns: 23, Skipped: 2, Unobserved: 1,
			ProviderErrors: 1, HarnessErrors: 0, GitLabRefused: 1,
		},
		Columns: columns{
			Clean:             ratio{Numerator: 5, Denominator: 10},
			Reached:           ratio{Numerator: 17, Denominator: 19},
			AcceptedFirstTime: ratio{Numerator: 14, Denominator: 17},
			ArgumentFidelity:  ratio{Numerator: 24, Denominator: 26},
			Confirmation:      ratio{Numerator: 3, Denominator: 4},
			Unaided:           ratio{Numerator: 6, Denominator: 10},
			Completion:        ratio{Numerator: 7, Denominator: 10},
			Overhead:          overhead{Discovery: 9, InvalidParams: 3, Steps: 17},
		},
		Tokens: tokens{Input: 120000, Output: 4200, CacheCreated: 30000, CacheRead: 88000},
		// Ten invented cases, so the example caption shows a coverage that is
		// part of the corpus rather than all of it. That is the shape a reader
		// most needs to recognize: a row narrowed with MODELEVAL_CASES looks
		// exactly like a full one except here.
		Cases: legendCases(),
	}
}

// legendCases are the invented case identifiers the example row covers. They
// are spelled out rather than generated so the example's caption is the same
// string on every run, which is what lets the page be gated.
func legendCases() map[string]caseFigures {
	names := []string{
		"XX-001", "XX-002", "XX-003", "XX-004", "XX-005",
		"XX-006", "XX-007", "XX-008", "XX-009", "XX-010",
	}
	cases := make(map[string]caseFigures, len(names))
	for _, name := range names {
		cases[name] = caseFigures{Run: "example-run", Date: "2026-09-17", Commit: "0000000000"}
	}
	return cases
}

// glossList renders the definitions as a list.
func glossList(glosses []legendGloss) string {
	var body strings.Builder
	for _, one := range glosses {
		body.WriteString("- **" + one.Column + "** — " + one.Means + "\n")
	}
	return body.String()
}
