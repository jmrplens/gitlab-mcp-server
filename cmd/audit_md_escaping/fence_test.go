package main

import (
	"fmt"
	"go/ast"
	"slices"
	"sort"
	"strings"
	"testing"
)

// fenceFixture is the fixture the fence rule is tested against: one package
// holding one of every shape a fenced block is written in, and one holding the
// shapes that must stay quiet.
//
// It is a fixture set of its own rather than a package added to caseFixture, so
// the want lists the other contexts are pinned by do not move whenever a fence
// shape is added.
var fenceFixture = map[string]string{
	"mdfence/mdfence.go":  mdfenceSource,
	"mdfence/limits.go":   mdfenceLimitsSource,
	"mdfence/writers.go":  mdfenceWritersSource,
	"mdfencesafe/safe.go": mdfenceSafeSource,
}

// mdfenceSource holds the shapes that must be reported: a block written from
// one template, a block written in three writes, a block whose body is written
// in a loop, a fence longer than three, an indented fence, the fmt calls with
// no template, and a value declared safe where it is written.
const mdfenceSource = `package mdfence

import (
	"fmt"
	"strings"
)

// Item is the shape a GitLab response fills.
type Item struct {
	Title    string
	Content  string
	Language string
	Kind     string
	Rows     []string
}

// FormatTemplateBlock writes the whole block from one template, so the info
// string and the body are both holes of it.
func FormatTemplateBlock(item Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "` + "```" + `%s\n%s\n` + "```" + `\n", item.Language, item.Content)
	return b.String()
}

// FormatWrittenBlock writes the fence, the body and the closing fence as three
// writes of its own, which no template carries.
func FormatWrittenBlock(item Item) string {
	var b strings.Builder
	b.WriteString("` + "```" + `json\n")
	b.WriteString(item.Content)
	b.WriteString("\n` + "```" + `\n")
	return b.String()
}

// FormatChart opens the block at the top and writes its body inside a loop,
// which is the shape a nested block has to inherit the state for.
func FormatChart(item Item) string {
	var b strings.Builder
	b.WriteString("` + "```" + `mermaid\n")
	for range item.Rows {
		b.WriteString(item.Title)
		b.WriteString("\n")
	}
	b.WriteString("` + "```" + `\n")
	return b.String()
}

// FormatLongFence opens with more backticks than three, so a shorter run inside
// the block does not close it.
func FormatLongFence(item Item) string {
	var b strings.Builder
	b.WriteString("` + "`````" + `\n")
	b.WriteString("` + "```" + `\n")
	b.WriteString(item.Content)
	b.WriteString("` + "`````" + `\n")
	return b.String()
}

// FormatIndentedFence indents the fence by the three spaces CommonMark allows
// in front of one.
func FormatIndentedFence(item Item) string {
	var b strings.Builder
	b.WriteString("   ` + "```" + `\n")
	b.WriteString(item.Content)
	b.WriteString("   ` + "```" + `\n")
	return b.String()
}

// FormatPrintBlock writes the block with the fmt calls that carry no template
// at all.
func FormatPrintBlock(item Item) string {
	var b strings.Builder
	fmt.Fprintln(&b, "` + "```" + `")
	fmt.Fprint(&b, item.Content)
	fmt.Fprintln(&b, "\n` + "```" + `")
	return b.String()
}

// FormatTwoBuilders assembles two documents in one function, one of them
// fenced, so the value written to the other must not be judged as though the
// fence were around it.
func FormatTwoBuilders(item Item) string {
	var fenced strings.Builder
	var plain strings.Builder
	fenced.WriteString("` + "```" + `\n")
	plain.WriteString(item.Title)
	fenced.WriteString(item.Content)
	fenced.WriteString("` + "```" + `\n")
	return fenced.String() + plain.String()
}

// FormatDeclared writes a value inside a block that is declared safe where it
// is written.
func FormatDeclared(item Item) string {
	var b strings.Builder
	b.WriteString("` + "```" + `\n")
	//gitlab:allow-unescaped item.Kind: a kind this package chose, not a value GitLab sent.
	b.WriteString(item.Kind)
	b.WriteString("\n` + "```" + `\n")
	return b.String()
}
`

// mdfenceLimitsSource holds the shapes the rule deliberately does not catch,
// each of which would otherwise be a finding about a document that may never
// render that way.
const mdfenceLimitsSource = `package mdfence

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatBranchedFence opens the block in one branch and closes it in another,
// so the state the outer block is left with says no block is open.
func FormatBranchedFence(item Item, fenced bool) string {
	var b strings.Builder
	if fenced {
		b.WriteString("` + "```" + `\n")
	}
	b.WriteString(item.Content)
	if fenced {
		b.WriteString("` + "```" + `\n")
	}
	return b.String()
}

// FormatThroughHelper hands the builder to a helper between the fences, and
// that helper may well be what writes the closing one.
func FormatThroughHelper(item Item) string {
	var b strings.Builder
	b.WriteString("` + "```" + `\n")
	writeBody(&b, item)
	b.WriteString(item.Title)
	b.WriteString("\n` + "```" + `\n")
	return b.String()
}

func writeBody(b *strings.Builder, item Item) {
	b.WriteString(item.Content)
}

// FormatDynamicFence builds a template at run time between the fences, so the
// pass cannot read what that call wrote and gives the destination up rather
// than assuming the block is still open.
func FormatDynamicFence(item Item, width int) string {
	var b strings.Builder
	b.WriteString("` + "```" + `\n")
	fmt.Fprintf(&b, "%-"+strconv.Itoa(width)+"s\n", item.Title)
	b.WriteString(item.Content)
	b.WriteString("` + "```" + `\n")
	return b.String()
}

// FormatClosedBefore writes a value after the block has been closed, where a
// backtick run of its own ends nothing this server wrote.
func FormatClosedBefore(item Item) string {
	var b strings.Builder
	b.WriteString("` + "```" + `\n")
	b.WriteString("server text\n")
	b.WriteString("` + "```" + `\n")
	b.WriteString(item.Content)
	return b.String()
}
`

// mdfenceWritersSource holds the destinations a write can name besides a local
// builder: a dereferenced pointer, which is followed, and the two that are not.
const mdfenceWritersSource = `package mdfence

import (
	"fmt"
	"os"
	"strings"
)

// Holder keeps its document in a struct field, which is not a variable this
// pass follows.
type Holder struct {
	body strings.Builder
}

// FormatHeld writes the block into that field.
func (h *Holder) FormatHeld(item Item) string {
	h.body.WriteString("` + "```" + `\n")
	h.body.WriteString(item.Content)
	h.body.WriteString("` + "```" + `\n")
	return h.body.String()
}

// FormatThroughPointer writes through a dereferenced pointer to the builder.
func FormatThroughPointer(b *strings.Builder, item Item) {
	(*b).WriteString("` + "```" + `\n")
	(*b).WriteString(item.Content)
	(*b).WriteString("` + "```" + `\n")
}

// FormatToStdout writes to a destination that is no variable of the function,
// so the block is read out of the template alone.
func FormatToStdout(item Item) {
	fmt.Fprintf(os.Stdout, "` + "```" + `\n%s\n` + "```" + `\n", item.Content)
	fmt.Fprintln(os.Stdout, item.Title)
}

// FormatSprint renders with the fmt call that takes no writer at all.
func FormatSprint(item Item) string {
	return fmt.Sprint(item.Title)
}

// FormatPrintProse writes a value with no template outside any block, where it
// changes nothing this server wrote.
func FormatPrintProse(item Item) string {
	var b strings.Builder
	fmt.Fprint(&b, item.Title)
	return b.String()
}
`

// mdfenceSafeSource holds the shapes written the way the rule asks: the two
// helpers that measure the body, and a value that has already been through an
// escaper, which cannot begin a line and so cannot close a fence.
const mdfenceSafeSource = `package mdfencesafe

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Item is the shape a GitLab response fills.
type Item struct {
	Content string
}

// FormatMeasuredBlock builds the whole block with the shared helper, which
// leaves no literal backtick run in the source at all.
func FormatMeasuredBlock(item Item) string {
	var b strings.Builder
	b.WriteString(toolutil.MarkdownFencedBlock("json", item.Content))
	return b.String()
}

// FormatMeasuredFence sizes the fence itself and writes the block around it.
func FormatMeasuredFence(item Item) string {
	var b strings.Builder
	fence := toolutil.MarkdownCodeFence(item.Content)
	fmt.Fprintf(&b, "%s\n%s\n%s\n", fence, item.Content, fence)
	return b.String()
}

// FormatEscapedInBlock writes a value that has been through an escaper. Every
// escaper either collapses the newlines or puts a quote marker in front of each
// line, so the value cannot begin a line, and a closing fence has to.
func FormatEscapedInBlock(item Item) string {
	var b strings.Builder
	b.WriteString("` + "```" + `\n")
	b.WriteString(toolutil.EscapeMdTableCell(item.Content))
	b.WriteString("\n` + "```" + `\n")
	return b.String()
}

// FormatInlineSpan writes a value between single backticks, which opens a code
// span rather than a block.
func FormatInlineSpan(item Item) string {
	return fmt.Sprintf("Content ` + "`" + `%s` + "`" + ` in prose\n", item.Content)
}
`

// wantFenceFindings is every value the fence fixture writes into a block the
// formatter fenced by hand.
var wantFenceFindings = []string{
	"mdfence fence item.Content",  // written in three writes
	"mdfence fence item.Content",  // the body of the one template
	"mdfence fence item.Content",  // inside a fence longer than three
	"mdfence fence item.Content",  // inside an indented fence
	"mdfence fence item.Content",  // written by fmt.Fprint
	"mdfence fence item.Content",  // the fenced one of two builders
	"mdfence fence item.Content",  // written through a dereferenced pointer
	"mdfence fence item.Content",  // a template of its own, to a writer no variable names
	"mdfence fence item.Language", // the info string of the opening fence
	"mdfence fence item.Title",    // written in a loop inside the block
}

// TestAudit_FenceFixture_ReportsEveryValueInsideAHandWrittenFence pins what the
// fence rule reports, shape by shape.
func TestAudit_FenceFixture_ReportsEveryValueInsideAHandWrittenFence(t *testing.T) {
	report := auditFenceFixture(t)

	missing, extra := diff(entries(report.Findings), wantFenceFindings)
	for _, line := range missing {
		t.Errorf("finding not reported: %s", line)
	}
	for _, line := range extra {
		t.Errorf("unexpected finding: %s", line)
	}
}

// TestAudit_FenceFixture_LeavesAMeasuredBlockAlone checks the half that makes
// the rule self-enforcing: a block built with either shared helper leaves no
// literal fence behind, so nothing is judged, and an escaped value inside a
// hand-written fence is safe because it cannot begin a line.
func TestAudit_FenceFixture_LeavesAMeasuredBlockAlone(t *testing.T) {
	report := auditFenceFixture(t)

	for _, finding := range append(append([]Finding{}, report.Findings...), report.Unresolved...) {
		if strings.HasSuffix(finding.Package, "/mdfencesafe") {
			t.Errorf("reported a measured block: %s %s (%s)", finding.Context, finding.Expression, finding.Reason)
		}
	}
}

// TestAudit_FenceFixture_DoesNotFollowTheShapesItDeclinesTo checks the two
// limits that keep the rule from inventing findings, and does it by naming
// them: a fence opened in one branch and closed in another, and a helper handed
// the builder between the fences. Both are false negatives on purpose, and a
// later change that turns either into a finding has to move this test.
func TestAudit_FenceFixture_DoesNotFollowTheShapesItDeclinesTo(t *testing.T) {
	report := auditFenceFixture(t)

	declined := map[string]bool{
		"FormatBranchedFence": true,
		"FormatThroughHelper": true,
		"FormatClosedBefore":  true,
		"FormatDynamicFence":  true,
		"FormatHeld":          true,
	}
	for _, finding := range append(append([]Finding{}, report.Findings...), report.Unresolved...) {
		if declined[finding.Func] {
			t.Errorf("%s reported %s %s, which the rule declines to follow",
				finding.Func, finding.Context, finding.Expression)
		}
	}
}

// TestAudit_FenceFixture_ExcusesADeclaredValueInsideAFence checks that the
// exemption mechanism reaches the fence context too, so a body that needs no
// measuring is declared where it is written rather than in a list this command
// carries.
func TestAudit_FenceFixture_ExcusesADeclaredValueInsideAFence(t *testing.T) {
	report := auditFenceFixture(t)

	if got := entries(report.Excused); len(got) != 1 || got[0] != "mdfence fence item.Kind" {
		t.Errorf("excused = %v, want the declared kind alone", got)
	}
	// toolutil is loaded beside the fixture and declares exemptions for the
	// contexts this run does not judge, which are stale by construction here
	// and belong to the sweep rather than to this test.
	for _, stale := range report.StaleDirectives {
		if strings.HasPrefix(stale.Package, fixtureDir) {
			t.Errorf("the fixture declares an exemption that excuses nothing: %s %s", stale.Package, stale.Expression)
		}
	}
}

// auditFenceFixture runs the audit over the fence fixture with the fence
// context alone, so the other five contexts' findings in toolutil do not have
// to be filtered out of every comparison.
func auditFenceFixture(t *testing.T) Report {
	t.Helper()
	prog := loadFixture(t, fenceFixture)
	sel, err := parseContexts("fence")
	if err != nil {
		t.Fatalf("parseContexts: %v", err)
	}
	return audit(prog, sel, repoRoot(t))
}

// TestFenceMarker_Lines_ReadsTheRunThatOpensABlock checks which lines carry a
// fence marker, which is the one thing the whole rule rests on.
func TestFenceMarker_Lines_ReadsTheRunThatOpensABlock(t *testing.T) {
	cases := []struct {
		name string
		line string
		want int
	}{
		{name: "three backticks", line: "```", want: 3},
		{name: "with an info string", line: "```json", want: 3},
		{name: "five backticks", line: "`````", want: 5},
		{name: "indented three", line: "   ```", want: 3},
		{name: "indented four is code, not a fence", line: "    ```", want: 0},
		{name: "two backticks open a span", line: "``code``", want: 0},
		{name: "one backtick", line: "`code`", want: 0},
		{name: "not at the start", line: "text ```", want: 0},
		{name: "empty", line: "", want: 0},
		{name: "tildes are not read as a fence", line: "~~~", want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run, ok := fenceMarker(tc.line)
			if tc.want == 0 {
				if ok {
					t.Errorf("fenceMarker(%q) read a run of %d, want no marker", tc.line, run)
				}
				return
			}
			if !ok || run != tc.want {
				t.Errorf("fenceMarker(%q) = %d, %v, want %d, true", tc.line, run, ok, tc.want)
			}
		})
	}
}

// TestFenceCursor_Writes_TracksWhetherABlockIsOpen walks the state machine over
// the write sequences a formatter produces, including the ones that span
// several writes, which is why the cursor exists rather than a scan of one
// template.
func TestFenceCursor_Writes_TracksWhetherABlockIsOpen(t *testing.T) {
	cases := []struct {
		name  string
		texts []string
		want  bool
	}{
		{name: "nothing written", texts: nil, want: false},
		{name: "prose", texts: []string{"## Title\n\n"}, want: false},
		{name: "one write opens a block", texts: []string{"```json\n"}, want: true},
		{name: "the info string is inside it", texts: []string{"```"}, want: true},
		{name: "a closed block", texts: []string{"```\n", "body\n", "```\n"}, want: false},
		{name: "closed across two writes", texts: []string{"```\n", "body\n", "\n```\n"}, want: false},
		{name: "a shorter run does not close it", texts: []string{"`````\n", "```\n"}, want: true},
		{name: "a longer run does close it", texts: []string{"```\n", "`````\n"}, want: false},
		{name: "a run mid-line closes nothing", texts: []string{"```\n", "text ```\n"}, want: true},
		{name: "reopened after closing", texts: []string{"```\n```\n", "```\n"}, want: true},
		{name: "a value cannot open one", texts: []string{"prose "}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cursor := newFenceCursor()
			for _, text := range tc.texts {
				cursor = cursor.writeText(text)
			}
			if got := cursor.inside(); got != tc.want {
				t.Errorf("after %q, inside() = %v, want %v", tc.texts, got, tc.want)
			}
		})
	}
}

// TestFenceCursor_WriteValue_LeavesTheLineOpen checks that a value written into
// the document is taken to change nothing but the position: the block it sits in
// stays as it was, and the next literal text is no longer at the start of a
// line, so a run written after it on that line closes nothing.
func TestFenceCursor_WriteValue_LeavesTheLineOpen(t *testing.T) {
	cursor := newFenceCursor().writeText("```\n").writeValue()

	if !cursor.inside() {
		t.Error("a value written inside a block left the cursor outside it")
	}
	if !cursor.writeText("```\n").inside() {
		t.Error("a run written after a value on the same line closed the block, which CommonMark does not")
	}
	if cursor.writeText("\n```\n").inside() {
		t.Error("a run at the start of the next line did not close the block")
	}
}

// TestFenceCursor_Closed_GivesUpRatherThanStayingOpen checks the state a
// destination is left in when something the audit cannot read writes into it.
func TestFenceCursor_Closed_GivesUpRatherThanStayingOpen(t *testing.T) {
	if newFenceCursor().writeText("```\n").closed().inside() {
		t.Error("giving up on a destination left the block open, which would report every later value")
	}
}

// TestFenceHoles_Template_AnswersPerHole checks the per-hole answer a template
// carrying a whole block produces, which is what turns one Fprintf into two
// findings and no more.
func TestFenceHoles_Template_AnswersPerHole(t *testing.T) {
	template := "## %s\n\n```%s\n%s\n```\n%s\n"
	holes := parseVerbs(template)
	if len(holes) != 4 {
		t.Fatalf("parseVerbs found %d hole(s) in %q, want 4", len(holes), template)
	}

	inside := fenceHoles(template, newFenceCursor(), holes)

	// The heading first, then the info string and the body of the block, then
	// the prose after it closed.
	if want := []bool{false, true, true, false}; !slices.Equal(inside, want) {
		t.Errorf("fenceHoles answered %v, want %v", inside, want)
	}
}

// TestCollectFences_Fixture_RecordsTheCursorEveryWriteStartsFrom checks the
// index the sink collection reads: a call inside a block is recorded as inside
// one, and the destinations of one function do not run together.
func TestCollectFences_Fixture_RecordsTheCursorEveryWriteStartsFrom(t *testing.T) {
	prog := loadFixture(t, fenceFixture)

	fences := collectFences(prog)

	if len(fences.at) == 0 {
		t.Fatal("the pass recorded no write at all")
	}
	inside := 0
	for _, cursor := range fences.at {
		if cursor.inside() {
			inside++
		}
	}
	if inside == 0 {
		t.Error("no recorded write starts inside a block, so nothing would ever be judged")
	}
	if len(fences.writes) == 0 {
		t.Error("no value written with no template was collected as a sink")
	}
}

// TestReceiverOf_CallShapes_AnswersOnlyForASelector checks the one shape that
// cannot appear in this repository's source and would panic a type assertion:
// a call of a plain function reaching the method walk. It names nothing rather
// than being read as a write to something.
func TestReceiverOf_CallShapes_AnswersOnlyForASelector(t *testing.T) {
	receiver := &ast.Ident{Name: "b"}
	selector := &ast.CallExpr{Fun: &ast.SelectorExpr{X: receiver, Sel: &ast.Ident{Name: "WriteString"}}}
	if got := receiverOf(selector); got != ast.Expr(receiver) {
		t.Errorf("receiverOf(b.WriteString(...)) = %v, want the receiver", got)
	}
	if got := receiverOf(&ast.CallExpr{Fun: &ast.Ident{Name: "WriteString"}}); got != nil {
		t.Errorf("receiverOf(WriteString(...)) = %v, want nothing", got)
	}
}

// TestFenceIndex_CursorFor_AnswersForACallItNeverSaw checks the default a sink
// outside every function body gets, since a Sprintf has no destination to
// inherit a block from.
func TestFenceIndex_CursorFor_AnswersForACallItNeverSaw(t *testing.T) {
	var absent *fenceIndex
	if absent.cursorFor(nil).inside() {
		t.Error("a call with no recorded cursor was answered as inside a block")
	}
	empty := &fenceIndex{at: map[*ast.CallExpr]fenceCursor{}}
	if empty.cursorFor(&ast.CallExpr{}).inside() {
		t.Error("an unrecorded call was answered as inside a block")
	}
}

// TestAudit_FenceFixture_IsDeterministic checks what the fixture comparisons
// rely on: two runs list the same findings in the same order.
func TestAudit_FenceFixture_IsDeterministic(t *testing.T) {
	first := entries(auditFenceFixture(t).Findings)
	second := entries(auditFenceFixture(t).Findings)

	if !sort.StringsAreSorted(first) {
		t.Errorf("findings are not in a stable order: %v", first)
	}
	if got, want := fmt.Sprint(first), fmt.Sprint(second); got != want {
		t.Errorf("two runs disagree:\n%s\n---\n%s", got, want)
	}
}
