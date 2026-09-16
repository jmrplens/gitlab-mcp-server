// render.go draws the eight managed blocks from the committed record.
//
// Eight, in two files: four in the README, which a reader meets first and which
// carry the seven columns and nothing else, and four in the reference page,
// which carry the counts the columns leave out, what the row cost and what
// produced it. Both are generated from the same document, so a figure cannot
// stand in one place and not the other, which is the state the withdrawn tables
// were in for two releases.
//
// A block that has no row says so in a sentence naming where the withdrawn
// tables can still be read. That sentence is generated too, rather than left in
// the file by hand, because a hand-written block between generated markers is a
// block the gate cannot tell from a stale one.

package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
)

// block is one managed section: where it is, what it publishes and what it says
// when it publishes nothing.
type block struct {
	// Path is the file, relative to the repository root, and Start and End the
	// markers.
	Path       string
	Start, End string
	// Surface is the tool surface whose rows land here.
	Surface string
	// Licensed selects rows measured on a Premium or Ultimate instance, and
	// its negation the Free and Community ones. It is the split the two files
	// already had, and it is a real one: a licensed catalog is half again as
	// large, so a completion rate on it is not a reading of the same task.
	Licensed bool
	// Detailed is whether this block carries the counts, the tokens and the
	// provenance beside the published columns.
	Detailed bool
	// Legend is whether this block is the worked example rather than a
	// measurement. It draws one made-up row of each table shape with a gloss
	// per column, so the tables below it can be read without a reader holding
	// the definitions in their head. It carries no rows and is drawn whether or
	// not anything is published.
	Legend bool
	// Empty is what the block says when no row belongs to it. It names the
	// commit the withdrawn table can be read at, because a reader who followed
	// a link to a number is owed where it went.
	Empty string
}

// withdrawn is the commit the tables this rebuild withdrew can still be read
// at, and unsound the reason the sentences give for not reproducing them.
// Both are spelled once, so the eight sentences below cannot come to name
// different commits or to give different accounts of one withdrawal.
const (
	withdrawn = "4587cbfb3"
	unsound   = "` and is not reproduced because the measurement behind it was unsound."
)

// blocks are the nine managed sections, in the order a reader meets them:
// the four README summaries, the worked example, and the four detailed tables.
var blocks = []block{
	{
		Path: readmeRelPath, Start: "<!-- START MODEL EVAL DYNAMIC SUMMARY -->", End: "<!-- END MODEL EVAL DYNAMIC SUMMARY -->",
		Surface: surfaceDynamic,
		Empty: "Withdrawn. The CE dynamic table published here, last refreshed from a Docker run dated 20260627-232303, is readable at commit `" +
			withdrawn + unsound,
	},
	{
		Path: readmeRelPath, Start: "<!-- START MODEL EVAL META SUMMARY -->", End: "<!-- END MODEL EVAL META SUMMARY -->",
		Surface: surfaceMeta,
		Empty:   "Withdrawn. No CE meta-tools table was ever published here, and none will be until the rebuilt harness produces one.",
	},
	{
		Path: readmeRelPath, Start: "<!-- START MODEL EVAL ENTERPRISE META SUMMARY -->", End: "<!-- END MODEL EVAL ENTERPRISE META SUMMARY -->",
		Surface: surfaceMeta, Licensed: true,
		Empty: "Withdrawn. The Enterprise meta table published here, last refreshed from a Docker run dated 20260527, is readable at commit `" +
			withdrawn + unsound,
	},
	{
		Path: readmeRelPath, Start: "<!-- START MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->", End: "<!-- END MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->",
		Surface: surfaceDynamic, Licensed: true,
		Empty: "Withdrawn. The Enterprise dynamic table published here, last refreshed from a Docker run dated 20260628-015421, is readable at commit `" +
			withdrawn + unsound,
	},
	{
		Path: pageRelPath, Start: "<!-- START MODEL EVAL LEGEND -->", End: "<!-- END MODEL EVAL LEGEND -->",
		Legend: true,
	},
	{
		Path: pageRelPath, Start: "<!-- START MODEL EVAL DYNAMIC RESULTS -->", End: "<!-- END MODEL EVAL DYNAMIC RESULTS -->",
		Surface: surfaceDynamic, Detailed: true,
		Empty: "Withdrawn. The CE dynamic run published here, dated 20260627-232303, is readable at commit `" + withdrawn + "`.",
	},
	{
		Path: pageRelPath, Start: "<!-- START MODEL EVAL META RESULTS -->", End: "<!-- END MODEL EVAL META RESULTS -->",
		Surface: surfaceMeta, Detailed: true,
		Empty: "No CE meta-tools run has been published here. The rebuilt harness has not put the corpus to a model.",
	},
	{
		Path: pageRelPath, Start: "<!-- START MODEL EVAL ENTERPRISE META RESULTS -->", End: "<!-- END MODEL EVAL ENTERPRISE META RESULTS -->",
		Surface: surfaceMeta, Licensed: true, Detailed: true,
		Empty: "Withdrawn. The Enterprise meta-tools run published here, dated 20260527, is readable at commit `" + withdrawn + "`.",
	},
	{
		Path: pageRelPath, Start: "<!-- START MODEL EVAL ENTERPRISE DYNAMIC RESULTS -->", End: "<!-- END MODEL EVAL ENTERPRISE DYNAMIC RESULTS -->",
		Surface: surfaceDynamic, Licensed: true, Detailed: true,
		Empty: "Withdrawn. The Enterprise dynamic run published here, dated 20260628-015421, is readable at commit `" + withdrawn + "`.",
	},
}

// The surfaces a block publishes. The individual surface has no block of its
// own yet, and a row measured on it is reported rather than dropped: a renderer
// that silently discarded a measurement is the fold this whole rebuild exists
// to stop.
const (
	surfaceDynamic = "dynamic"
	surfaceMeta    = "meta"
)

// opaqueSchema is the meta surface's default input-schema mode, which withholds
// parameter names. A row measured in it carries a sentence saying so.
const opaqueSchema = "opaque"

// metaOpaqueNote is what an opaque meta row is not comparable with, and why.
//
// It is published beside the figures rather than in a footnote at the bottom of
// the page, because the comparison a reader is about to make is with the row
// immediately above.
const metaOpaqueNote = "These rows ran in the default `opaque` meta schema mode, which withholds parameter " +
	"names: a model learned them from the tool description and from `invalid_params` refusals and from nothing " +
	"else. Their argument fidelity is therefore not comparable with a dynamic row, where `gitlab_find_action` " +
	"returns the schema, or with an individual row, where the tool carries it. What the mode costs is visible " +
	"in the `invalid_params` half of the overhead column."

// selects reports whether a row belongs in this block.
func (b block) selects(one row) bool {
	return one.Key.Surface == b.Surface && licensed(one) == b.Licensed
}

// licensed reports whether a row was measured on a Premium or Ultimate
// instance, which is the split the two files publish.
func licensed(one row) bool {
	return one.Key.Tier == "premium" || one.Key.Tier == "ultimate"
}

// renderBlock draws one block's body from the whole record, which it needs
// rather than only its own rows so that a cross-surface note can name a row in
// another block.
func renderBlock(b block, all []row) string {
	if b.Legend {
		return legendBlock()
	}
	var mine []row
	for _, one := range all {
		if b.selects(one) {
			mine = append(mine, one)
		}
	}
	if len(mine) == 0 {
		return b.Empty
	}

	var body strings.Builder
	for i, one := range groupByVendor(mine) {
		if i > 0 {
			body.WriteString("\n")
		}
		body.WriteString(renderGroup(b, one, all))
	}
	return strings.TrimRight(body.String(), "\n")
}

// renderGroup draws one comparison table and whatever the block carries beside
// it.
func renderGroup(b block, one group, all []row) string {
	var body strings.Builder
	body.WriteString("Compared as one table because these rows agree on " + one.Caption + ".\n\n")
	body.WriteString(columnsTable(one.Rows))
	if b.Surface == surfaceMeta && one.Rows[0].Key.MetaParamSchema == opaqueSchema {
		body.WriteString("\n" + metaOpaqueNote + "\n")
	}
	if !b.Detailed {
		return body.String()
	}
	body.WriteString("\n" + apartTable(one.Rows))
	body.WriteString("\n" + tokensTable(one.Rows))
	body.WriteString("\n" + provenanceTable(one.Rows))
	if note := crossSurfaceNotes(one.Rows, all); note != "" {
		body.WriteString("\n" + note)
	}
	return body.String()
}

// columnsTable is the seven published figures, one row per model.
func columnsTable(rows []row) string {
	cells := make([][]string, 0, len(rows))
	for _, one := range rows {
		cells = append(cells, []string{
			"`" + one.Key.Model + "`",
			one.Columns.Clean.String(),
			one.Columns.Reached.String(),
			one.Columns.AcceptedFirstTime.String(),
			one.Columns.ArgumentFidelity.String(),
			one.Columns.Confirmation.String(),
			one.Columns.Unaided.String(),
			one.Columns.Completion.String(),
			overheadCell(one.Columns.Overhead),
		})
	}
	return docgen.RenderMarkdownTable(
		[]string{"Model", "Clean", "Reached", "Accepted first time", "Argument fidelity", "Confirmation", "Unaided", "Completion", "Overhead"},
		[]docgen.Alignment{
			docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight,
			docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight,
		},
		cells,
	)
}

// overheadCell prints the overhead column with its two halves visible, because
// a discovery call and an argument refusal mean different things and one rate
// over both would hide each inside the other.
func overheadCell(o overhead) string {
	if o.Steps == 0 {
		return "-"
	}
	return fmt.Sprintf("%s (%d find, %d invalid_params)", o.Ratio(), o.Discovery, o.InvalidParams)
}

// apartTable is what was counted and then left out of every column, which is
// the half a rate cannot carry.
func apartTable(rows []row) string {
	cells := make([][]string, 0, len(rows))
	for _, one := range rows {
		cells = append(cells, []string{
			"`" + one.Key.Model + "`",
			strconv.Itoa(one.Counts.Attempts),
			strconv.Itoa(one.Counts.Turns),
			strconv.Itoa(one.Counts.Skipped),
			strconv.Itoa(one.Counts.Unobserved),
			strconv.Itoa(one.Counts.ProviderErrors),
			strconv.Itoa(one.Counts.HarnessErrors),
			strconv.Itoa(one.Counts.GitLabRefused),
		})
	}
	return "What the columns leave out: an attempt the instance could not offer, one the server's span never described, " +
		"one the provider would not answer, one this side broke, and one GitLab refused after a correct dispatch. " +
		"None of the five is the model's, so none is in any denominator.\n\n" +
		docgen.RenderMarkdownTable(
			[]string{"Model", "Attempts", "Turns", "Skipped", "Unobserved", "Provider errors", "Harness errors", "GitLab refused"},
			[]docgen.Alignment{
				docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight,
				docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight,
			},
			cells,
		)
}

// tokensTable is what the row cost, as four numbers.
func tokensTable(rows []row) string {
	cells := make([][]string, 0, len(rows))
	for _, one := range rows {
		cells = append(cells, []string{
			"`" + one.Key.Model + "`",
			strconv.Itoa(one.Tokens.Input),
			strconv.Itoa(one.Tokens.Output),
			strconv.Itoa(one.Tokens.CacheCreated),
			strconv.Itoa(one.Tokens.CacheRead),
		})
	}
	return "Tokens, never folded into one figure: a cache read is not an input token, and a table that added them " +
		"together is how sixty thousand tokens came to be published against five million.\n\n" +
		docgen.RenderMarkdownTable(
			[]string{"Model", "Input", "Output", "Cache created", "Cache read"},
			[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight},
			cells,
		)
}

// provenanceTable is what produced each row, the part of it the caption does
// not already hold fixed.
func provenanceTable(rows []row) string {
	cells := make([][]string, 0, len(rows))
	for _, one := range rows {
		cells = append(cells, []string{
			"`" + one.Key.Model + "`",
			"`" + one.Provenance.Provider + "`",
			"`" + shortCommit(one.Provenance.Commit) + "`",
			one.Provenance.Date,
			one.Provenance.GitLabVersion + " " + one.Provenance.Edition,
			strconv.Itoa(one.Provenance.ServedTools),
			strings.Join(one.Provenance.TokenScopes, ", "),
			requestOptions(one.Provenance.RequestOptions),
		})
	}
	return docgen.RenderMarkdownTable(
		[]string{"Model", "Provider", "Commit", "Date", "GitLab", "Tools served", "Token scopes", "Request options"},
		[]docgen.Alignment{
			docgen.AlignLeft, docgen.AlignLeft, docgen.AlignLeft, docgen.AlignLeft,
			docgen.AlignLeft, docgen.AlignRight, docgen.AlignLeft, docgen.AlignLeft,
		},
		cells,
	)
}

// shortCommit is how a page spells a revision: enough to find it, short enough
// to read in a table cell.
func shortCommit(commit string) string {
	if len(commit) <= 9 {
		return commit
	}
	return commit[:9]
}

// requestOptions renders how a model was asked, sorted so a re-render of one
// record produces the same bytes.
func requestOptions(options map[string]string) string {
	if len(options) == 0 {
		return "-"
	}
	names := make([]string, 0, len(options))
	for name := range options {
		names = append(names, name)
	}
	sort.Strings(names)
	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, "`"+name+"="+options[name]+"`")
	}
	return strings.Join(pairs, ", ")
}

// crossSurfaceNotes says, for each row that has one, which row of another
// surface it may be read beside.
//
// It is the second comparison key made visible. Without it a reader holding two
// tables has to work out by hand whether the rows in them were measured under
// the same conditions, which is the question the withdrawn tables answered
// wrongly by putting everything in one table and letting the caption carry
// nothing.
func crossSurfaceNotes(rows, all []row) string {
	var notes []string
	for _, one := range rows {
		key := crossSurfaceKey(one)
		var peers []string
		for _, other := range all {
			if other.Key.Surface != one.Key.Surface && crossSurfaceKey(other) == key {
				peers = append(peers, "its `"+other.Key.Surface+"` row"+bracketed(surfaceLabel(other)))
			}
		}
		if len(peers) == 0 {
			continue
		}
		sort.Strings(peers)
		notes = append(notes, fmt.Sprintf("- `%s` may be read beside %s: they agree on %s.",
			one.Key.Model, strings.Join(slicesCompact(peers), " and "), key))
	}
	if len(notes) == 0 {
		return ""
	}
	return "Cross-surface comparisons this table takes part in:\n\n" + strings.Join(notes, "\n") + "\n"
}

// bracketed puts what a peer row decided for itself in brackets after its
// surface, and nothing at all when it decided neither.
//
// The bracket is what makes the comparison honest rather than merely possible:
// an individual peer was shown a slice of the catalog and the row reading it is
// owed that fact in the same sentence, not two tables away.
func bracketed(label string) string {
	if label == "" {
		return ""
	}
	return " (" + label + ")"
}

// slicesCompact removes the repeats a row measured twice on one other surface
// would produce, keeping the order.
func slicesCompact(values []string) []string {
	seen := map[string]bool{}
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		kept = append(kept, value)
	}
	return kept
}

// unpublishedRows names the rows of a record that no block publishes.
//
// Today that is every row measured on the individual surface. The eight blocks
// are the dynamic and meta pairs of the two pages, and an individual row is
// neither: it is measured on a slice of the catalog rather than on the whole of
// it, so it is a comparison class of its own and giving it a column beside
// those would be the fold this record exists to stop. Until a page carries a
// block for that class, such a row would be measured, scored, committed and
// shown to nobody, which is why it is named here rather than dropped: the same
// reason the record carries both halves of every ratio.
func unpublishedRows(rows []row) []string {
	var orphans []string
	for _, one := range rows {
		published := false
		for _, b := range blocks {
			if b.selects(one) {
				published = true
				break
			}
		}
		if !published {
			orphans = append(orphans, one.Key.String())
		}
	}
	return orphans
}
