package main

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// cardFixture is the fixture the card rule is tested against: one package
// holding one of every shape a hand-written card row takes and the shapes
// that look like one and are not, and one written through Card, which the
// rule must leave alone.
//
// It is a fixture set of its own rather than a package added to caseFixture,
// so the want lists the escaping contexts are pinned by do not move whenever
// a card shape is added.
var cardFixture = map[string]string{
	"mdcard/mdcard.go":               mdcardSource,
	"mdcardsafe/mdcardsafe.go":       mdcardSafeSource,
	"mdcardclaimed/mdcardclaimed.go": mdcardClaimedSource,
	"mdcardstale/mdcardstale.go":     mdcardStaleSource,
}

// mdcardSource holds the rows a formatter writes by hand, each in the shape
// the tree writes it: through a template, a builder write, a print call, a
// shared header constant and the two cell builders; and beside them the
// collection rows, author items and metric tables the rule must not read as a
// card.
const mdcardSource = `package mdcard

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Item is the shape a GitLab response fills.
type Item struct {
	ID     int64
	Title  string
	State  string
	Counts map[string]int
}

// hints are the server's own guidance sentences.
var hints = []string{"Use action 'get' to fetch it"}

// FormatByHand writes a one-object card the way the tree wrote them before
// Card existed: one of every shape.
func FormatByHand(item Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- **ID**: %d\n", item.ID)
	fmt.Fprintf(&b, "- %s **Archived**\n", toolutil.EmojiWarning)
	b.WriteString("- **Locked**\n")
	fmt.Fprintf(&b, "**Message**: %s\n", toolutil.EscapeMdTableCell(item.Title))
	fmt.Fprintf(&b, "| Name | %s |\n", toolutil.EscapeMdTableCell(item.Title))
	b.WriteString(toolutil.TblFieldValue)
	fmt.Fprint(&b, "| Property | Value |\n| --- | --- |\n")
	b.WriteString(toolutil.MarkdownTableHeader("Setting", "Value"))
	b.WriteString(toolutil.MarkdownTableRow("State", toolutil.EscapeMdTableCell(item.State)))
	b.WriteString(fmt.Sprintf("- **Title**: %s | **State**: %s\n", toolutil.EscapeMdTableCell(item.Title), toolutil.EscapeMdTableCell(item.State)))
	fmt.Fprintf(&b, "  - **Nested**: %s\n", toolutil.EscapeMdTableCell(item.State))
	return b.String()
}

// FormatCollections writes the shapes that share a glyph with a card row and
// are not one: a row of a table of objects, an author item, a metrics table
// keyed by a map key, a plain hint bullet, a row whose label is a value, a
// header of a collection, and emphasis at the start of a sentence.
func FormatCollections(item Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| %d | %s | %s |\n", item.ID, toolutil.EscapeMdTableCell(item.Title), toolutil.EscapeMdTableCell(item.State))
	fmt.Fprintf(&b, "- **@%s** (%s, note %d):\n", toolutil.EscapeMdTableCell(item.Title), toolutil.EscapeMdTableCell(item.State), item.ID)
	b.WriteString("| Metric | Value |\n| --- | --- |\n")
	for key, count := range item.Counts {
		fmt.Fprintf(&b, "| %s | %d |\n", toolutil.EscapeMdTableCell(key), count)
		fmt.Fprintf(&b, "- **%s**: %d\n", toolutil.EscapeMdTableCell(key), count)
	}
	for _, hint := range hints {
		fmt.Fprintf(&b, "- %s\n", hint)
	}
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Value"))
	b.WriteString(toolutil.MarkdownTableRow(toolutil.EscapeMdTableCell(item.Title), toolutil.EscapeMdTableCell(item.State)))
	b.WriteString("- **Bold** at the start of a sentence\n")
	return b.String()
}

// FormatThroughCard passes a raw value to the two Card writes the caller
// renders for, which the escaping contexts report at this call site.
func FormatThroughCard(item Item) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Item")
	c.Markdown("Owner", item.Title)
	t := c.Table("Rows", "Name", "State")
	t.Row(item.Title, toolutil.EscapeMdTableCell(item.State))
	c.End()
	return b.String()
}
`

// mdcardSafeSource holds a formatter written through Card, whose every write
// escapes for itself, so neither the card rule nor the escaping contexts have
// anything to say about it.
const mdcardSafeSource = `package mdcardsafe

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Item is the shape a GitLab response fills.
type Item struct {
	ID        int64
	Title     string
	State     string
	URL       string
	CreatedAt string
	Locked    bool
}

// FormatCard renders the item through Card.
func FormatCard(item Item) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Item: "+item.Title)
	c.Int("ID", item.ID)
	c.Field("State", item.State)
	c.Bool("Locked", item.Locked)
	c.Time("Created", item.CreatedAt)
	c.URL(item.URL)
	c.Markdown("Link", toolutil.MdTitleLink(item.Title, item.URL))
	t := c.Table("Rows", "Name", "State")
	t.Row(toolutil.EscapeMdTableCell(item.Title), toolutil.EscapeMdTableCell(item.State))
	cells := []string{toolutil.EscapeMdTableCell(item.Title), toolutil.EscapeMdTableCell(item.State)}
	t.Row(cells...)
	t.Row()
	c.End("Use action 'update' to change it")
	return b.String()
}
`

// wantCardFindings is every hand-written row the card fixture holds, named by
// the line as the source wrote it.
var wantCardFindings = []string{
	"mdcard card **Message**: %s",
	"mdcard card - **ID**: %d",
	"mdcard card - **Locked**",
	"mdcard card - **Nested**: %s",
	"mdcard card - **Title**: %s | **State**: %s",
	"mdcard card - %s **Archived**",
	"mdcard card | Field | Value |",
	"mdcard card | Name | %s |",
	"mdcard card | Property | Value |",
	`mdcard card toolutil.MarkdownTableHeader("Setting", "Value")`,
	`mdcard card toolutil.MarkdownTableRow("State", toolutil.EscapeMdTableCell(item.State))`,
}

// wantCardCallFindings is what the escaping contexts report at the Card
// writes the caller renders for.
var wantCardCallFindings = []string{
	"mdcard list-item item.Title",
	"mdcard table-cell item.Title",
}

// auditCardFixture runs the audit over the card fixture with the named
// contexts.
func auditCardFixture(t *testing.T, contexts string) Report {
	t.Helper()
	prog := loadFixture(t, cardFixture)
	sel, err := parseContexts(contexts)
	if err != nil {
		t.Fatalf("parseContexts: %v", err)
	}
	return audit(prog, sel, repoRoot(t))
}

// TestAudit_CardFixture_ReportsEveryHandWrittenRow pins what the card rule
// reports, shape by shape, and that a card written through Card is left
// alone.
func TestAudit_CardFixture_ReportsEveryHandWrittenRow(t *testing.T) {
	report := auditCardFixture(t, "card")

	missing, extra := diff(entries(report.Findings), wantCardFindings)
	for _, line := range missing {
		t.Errorf("card row not reported: %s", line)
	}
	for _, line := range extra {
		t.Errorf("unexpected finding: %s", line)
	}
	for _, finding := range report.Findings {
		if finding.Wants != "toolutil.Card" || finding.Context != "card" {
			t.Errorf("finding %q wants %q in context %s, want toolutil.Card in the card context", finding.Expression, finding.Wants, finding.Context)
		}
		if finding.Verb != "row" && finding.Verb != "header" {
			t.Errorf("finding %q has verb %q, want row or header", finding.Expression, finding.Verb)
		}
	}
	if len(report.Unresolved) != 0 {
		t.Errorf("the card rule left %d value(s) unresolved, and it judges no value", len(report.Unresolved))
	}
}

// TestAudit_CardFixture_JudgesTheCardWritesAtTheCallSite checks the other
// half of what the card contract says: Markdown and Row take a value the
// caller rendered, so a raw value passed to them is the caller's finding, in
// the list item or the cell the method writes, and never a finding inside
// card.go.
func TestAudit_CardFixture_JudgesTheCardWritesAtTheCallSite(t *testing.T) {
	report := auditCardFixture(t, allContexts)

	missing, extra := diff(entries(report.Findings), wantCardCallFindings)
	for _, line := range missing {
		t.Errorf("raw value passed to a Card write not reported: %s", line)
	}
	for _, line := range extra {
		t.Errorf("unexpected finding: %s", line)
	}
	for _, finding := range report.Findings {
		if finding.Func != "FormatThroughCard" {
			t.Errorf("finding %s %s is attributed to %s, want the formatter that passed the value", finding.Context, finding.Expression, finding.Func)
		}
	}
	for _, finding := range append(append([]Finding{}, report.Findings...), report.Unresolved...) {
		if strings.HasSuffix(finding.Package, "/mdcardsafe") {
			t.Errorf("reported a value a Card write escapes for itself: %s %s (%s)", finding.Context, finding.Expression, finding.Reason)
		}
	}
}

// TestAudit_CardFixture_IsStagedOutOfTheGate checks that "all" does not judge
// the card shape, which is what keeps the gate's exit status while the rule
// reports, and that naming the rule beside "all" judges both.
func TestAudit_CardFixture_IsStagedOutOfTheGate(t *testing.T) {
	gating := auditCardFixture(t, allContexts)
	if gating.Summary.ByContext["card"] != 0 {
		t.Errorf("the default run reported %d card row(s), which stages the rule into the gate", gating.Summary.ByContext["card"])
	}

	both := auditCardFixture(t, "all,card")
	missing, extra := diff(entries(both.Findings), append(append([]string{}, wantCardFindings...), wantCardCallFindings...))
	for _, line := range missing {
		t.Errorf("all,card did not report: %s", line)
	}
	for _, line := range extra {
		t.Errorf("all,card reported an unexpected finding: %s", line)
	}
	if both.Summary.Contexts != "table-cell, heading, list-item, link-label, link-destination, fence, card" {
		t.Errorf("all,card judged %q, want the gating set and then the card rule", both.Summary.Contexts)
	}
}

// TestCardRows_Lines_ReadsTheFourShapesAndNothingElse pins the line grammar
// itself, since a pattern one glyph too wide would report every hint bullet
// in the tree and one too narrow would miss the rows the migration exists
// for.
func TestCardRows_Lines_ReadsTheFourShapesAndNothingElse(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "a labeled list row", text: "- **State**: %s\n", want: "row"},
		{name: "a star bullet", text: "* **State**: %s", want: "row"},
		{name: "a flag row", text: "- **Archived**\n", want: "row"},
		{name: "a flag row with an emoji verb", text: "- %s **Archived**\n", want: "row"},
		{name: "a flag row with a literal emoji", text: "- ⚠️ **Revoked**\n", want: "row"},
		{name: "a row indented under an item", text: "    - **Nested**: %v\n", want: "row"},
		{name: "a row written in two pieces", text: "- **Name**: ", want: "row"},
		{name: "a bullet-less label", text: "**Message**: %s\n", want: "row"},
		{name: "a two-cell row with a constant label", text: "| Name | %s |\n", want: "row"},
		{name: "a two-cell row with a bold label", text: "| **Name** | %d |", want: "row"},
		{name: "a field header", text: "| Field | Value |\n| --- | --- |\n", want: "header"},
		{name: "a property header", text: "| Property | Value |", want: "header"},
		{name: "a setting header", text: "|Setting|Value|", want: "header"},
		{name: "an attribute header", text: "| Attribute | Value |", want: "header"},
		{name: "a collection row", text: "| %d | %s | %s |\n"},
		{name: "a two-cell collection row", text: "| %s | %s |\n"},
		{name: "a row whose label is a value", text: "- **%s**: %s\n"},
		{name: "an author item", text: "- **@%s** (%s, note %d):\n"},
		{name: "a metrics header", text: "| Metric | Value |\n"},
		{name: "a key-value header", text: "| Key | Value |\n"},
		{name: "a hint bullet", text: "- Use action 'get' to fetch it\n"},
		{name: "emphasis opening a sentence", text: "- **Bold** at the start\n"},
		{name: "a word before the label is not an emoji", text: "- Use **action**: %s\n"},
		{name: "a heading", text: "## %s\n"},
		{name: "a delimiter row alone", text: "| --- | --- |\n"},
		{name: "a three-cell row with a constant label", text: "| Name | %s | %s |\n"},
		{name: "prose", text: "%s wrote it\n"},
		{name: "nothing", text: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := cardRows(tc.text)
			if tc.want == "" {
				if len(rows) != 0 {
					t.Errorf("cardRows(%q) read %v, want no row", tc.text, rows)
				}
				return
			}
			if len(rows) != 1 || rows[0].verb != tc.want {
				t.Errorf("cardRows(%q) = %v, want one %s", tc.text, rows, tc.want)
			}
		})
	}
}

// TestCardHeaderCells_CellCounts_OnlyAPairCanBeAHeader checks the guard in
// front of the two cells the header pattern is built from.
//
// A field table has exactly two columns, so anything else is a table of
// objects rather than a card; the count is also what keeps the two cells from
// being read out of a slice that does not hold them.
func TestCardHeaderCells_CellCounts_OnlyAPairCanBeAHeader(t *testing.T) {
	cases := []struct {
		name  string
		cells []string
		want  bool
	}{
		{name: "the header of a field table", cells: []string{"Field", "Value"}, want: true},
		{name: "one cell", cells: []string{"Field"}, want: false},
		{name: "no cells at all", cells: nil, want: false},
		{name: "three cells", cells: []string{"Field", "Value", "Source"}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cardHeaderCells(tc.cells); got != tc.want {
				t.Errorf("cardHeaderCells(%v) = %v, want %v", tc.cells, got, tc.want)
			}
		})
	}
}

// TestCardScoped_Files_LeavesThePromptsAndCardItselfAlone checks the two
// exclusions the rule carries: a prompt's lists are the prompt's own layout,
// and card.go writes the rows every other formatter is asked to use.
func TestCardScoped_Files_LeavesThePromptsAndCardItselfAlone(t *testing.T) {
	cases := []struct {
		name string
		pkg  string
		file string
		want bool
	}{
		{name: "a domain formatter", pkg: "internal/tools/issues", file: "/repo/internal/tools/issues/markdown.go", want: true},
		{name: "toolutil's shared renderers", pkg: "internal/toolutil", file: "/repo/internal/toolutil/markdown.go", want: true},
		{name: "card.go itself", pkg: "internal/toolutil", file: "/repo/internal/toolutil/card.go", want: false},
		{name: "a card.go elsewhere", pkg: "internal/tools/issues", file: "/repo/internal/tools/issues/card.go", want: true},
		{name: "the prompts package", pkg: "internal/prompts", file: "/repo/internal/prompts/prompts.go", want: false},
		{name: "a package under prompts", pkg: "internal/prompts/reports", file: "/repo/internal/prompts/reports/reports.go", want: false},
		{name: "a package whose name merely begins with prompts", pkg: "internal/promptsmith", file: "/repo/internal/promptsmith/x.go", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkg := &packages.Package{PkgPath: modulePath + "/" + tc.pkg}
			if got := cardScoped(pkg, tc.file); got != tc.want {
				t.Errorf("cardScoped(%s, %s) = %v, want %v", tc.pkg, tc.file, got, tc.want)
			}
		})
	}
}

// mdcardClaimedSource writes a hand card row in a function that declares it is
// not a card, which is the shape the interactive consent prompts have: a
// question a caller approves, whose values are escaped by a stronger rule than
// the card's.
const mdcardClaimedSource = `package mdcardclaimed

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Consent is what an interactive flow asks a caller to approve.
type Consent struct {
	Title string
}

// FormatConsent writes the question and the field it would act on.
//
//gitlab:allow-card FormatConsent: a consent prompt rather than a card, escaped by a rule of its own
func FormatConsent(c Consent) string {
	var b strings.Builder
	b.WriteString("Create this?\n\n")
	fmt.Fprintf(&b, "- **Title**: %s\n", toolutil.EscapeMdTableCell(c.Title))
	return b.String()
}
`

// mdcardStaleSource declares a function that writes no card row at all, so the
// declaration excuses nothing and is reported stale once the rule has run.
const mdcardStaleSource = `package mdcardstale

// FormatNothing writes no Markdown at all.
//
//gitlab:allow-card FormatNothing: a declaration that excuses nothing, so that a run can say so
func FormatNothing() string {
	return "nothing"
}
`

// TestAudit_CardFixture_ADeclaredFunction_IsExcusedRatherThanReported holds the
// one way out of a card finding, and the one this rule did not have when it was
// staged: a function that writes something which is not a card.
//
// The subject is the function rather than the row, which is what the directive
// can key on: a hand-written row has no value of its own to name, and its text
// carries a colon the directive grammar cuts a reason at. The claim is made at
// that grain too, since what is declared is that this function writes something
// that is not a card, which is true of its rows together or of none of them.
func TestAudit_CardFixture_ADeclaredFunction_IsExcusedRatherThanReported(t *testing.T) {
	report := auditCardFixture(t, "card")

	for _, finding := range report.Findings {
		if strings.HasSuffix(finding.Package, "/mdcardclaimed") {
			t.Errorf("a declared function was reported: %s %s", finding.Func, finding.Expression)
		}
	}
	var excused int
	for _, finding := range report.Excused {
		if strings.HasSuffix(finding.Package, "/mdcardclaimed") {
			excused++
			if finding.Func != "FormatConsent" {
				t.Errorf("the excused row is attributed to %q, want the function that declared it", finding.Func)
			}
		}
	}
	if excused != 1 {
		t.Errorf("excused %d row(s) of the declaring package, want its one row", excused)
	}
}

// TestAudit_CardFixture_ADeclarationThatExcusesNothing_IsReportedStale holds
// the other half of the mechanism, which is what stops a declaration outliving
// the code it was written for: the rule that excuses has to say when it excused
// nothing. It is asked of the card rule only once that rule has run, since a
// run that never put the question cannot say the answer was not needed.
func TestAudit_CardFixture_ADeclarationThatExcusesNothing_IsReportedStale(t *testing.T) {
	judged := auditCardFixture(t, "card")
	var stale int
	for _, directive := range judged.StaleDirectives {
		if strings.HasSuffix(directive.Package, "/mdcardstale") {
			stale++
			if directive.Kind != kindCard {
				t.Errorf("the stale directive is of kind %q, want the card kind", directive.Kind)
			}
		}
	}
	if stale != 1 {
		t.Errorf("reported %d stale card directive(s), want the one that excuses nothing", stale)
	}

	notJudged := auditCardFixture(t, allContexts)
	for _, directive := range notJudged.StaleDirectives {
		if directive.Kind == kindCard {
			t.Errorf("a card directive was called stale by a run that never judged the card shape: %s %s", directive.Package, directive.Expression)
		}
	}
}
