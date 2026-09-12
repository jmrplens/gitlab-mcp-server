package main

import (
	"go/ast"
	"go/types"
	"strings"
	"testing"
)

// rawFixture is the fixture the second verdict is tested against: one package
// holding one of every shape a flag or an instant reaches the page raw in,
// beside the same values rendered through their helpers, a value declared
// raw where it is written, and a declaration that excuses nothing.
var rawFixture = map[string]string{
	"mdraw/mdraw.go": mdrawSource,
}

// mdrawSource holds the raw shapes and the rendered ones side by side.
const mdrawSource = `package mdraw

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Stamp is a named string, the shape a converter gives an instant it does not
// parse.
type Stamp string

// Item is the shape a GitLab response fills.
type Item struct {
	Title      string
	Stat       string
	Active     bool
	Locked     *bool
	Count      int
	ViewedAt   int
	When       time.Time
	Expires    *time.Time
	Elapsed    time.Duration
	CreatedAt  string
	UpdatedAt  string
	StartedAt  string
	FinishedAt Stamp
	EndsAt     *string
	DueDate    string
	StartDate  string
	Render     func(string) string
}

// FormatRaw prints a flag and an instant every way the tree prints them
// without a helper.
//
//gitlab:allow-raw item.StartDate: a date GitLab sends without a time, shown as the day it names.
//gitlab:allow-raw item.Retired: nothing prints this any more.
func FormatRaw(item Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| Active | %t |\n", item.Active)
	fmt.Fprintf(&b, "| Enabled | %v |\n", item.Active)
	fmt.Fprintf(&b, "| Locked | %v |\n", item.Locked)
	fmt.Fprintf(&b, "| When | %v |\n", item.When)
	fmt.Fprintf(&b, "| Expires | %s |\n", item.Expires)
	fmt.Fprintf(&b, "| Day | %s |\n", item.When.Format("2006-01-02"))
	fmt.Fprintf(&b, "| Approved | %s |\n", yesNo(item.Active))
	fmt.Fprintf(&b, "| Created | %s |\n", toolutil.EscapeMdTableCell(item.CreatedAt))
	fmt.Fprintf(&b, "| Updated | %s |\n", strings.TrimSpace(item.UpdatedAt))
	fmt.Fprintf(&b, "| Start | %s |\n", item.StartDate)
	fmt.Fprintf(&b, "| Finished | %s |\n", string(item.FinishedAt))
	fmt.Fprintf(&b, "| Ends | %s |\n", *item.EndsAt)
	b.WriteString(toolutil.MarkdownTableRow(strconv.FormatBool(item.Active), item.DueDate))
	fmt.Fprintf(&b, "Merged: %t\n", item.Active)
	return b.String()
}

// yesNo is the hand-rolled flag helper the tree has several of.
func yesNo(on bool) string {
	if on {
		return "Yes"
	}
	return "No"
}

// FormatRendered prints the same values through their helpers, and the
// shapes that share a type or a name with a raw one and are not one.
func FormatRendered(item Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| Active | %s |\n", toolutil.BoolEmoji(item.Active))
	fmt.Fprintf(&b, "| Created | %s |\n", toolutil.FormatTime(item.CreatedAt))
	fmt.Fprintf(&b, "| When | %s |\n", toolutil.FormatTimeValue(item.When))
	fmt.Fprintf(&b, "| Count | %d |\n", item.Count)
	fmt.Fprintf(&b, "| Count | %v |\n", item.Count)
	fmt.Fprintf(&b, "| Elapsed | %v |\n", item.Elapsed)
	fmt.Fprintf(&b, "| Title | %s |\n", toolutil.EscapeMdTableCell(item.Title))
	fmt.Fprintf(&b, "| Stat | %s |\n", toolutil.EscapeMdTableCell(item.Stat))
	fmt.Fprintf(&b, "| Default | %v |\n", true)
	fmt.Fprintf(&b, "| Label | %s |\n", label(item))
	fmt.Fprintf(&b, "| Views | %v |\n", item.ViewedAt)
	fmt.Fprintf(&b, "| Started | %s |\n", strings.Replace(toolutil.EscapeMdTableCell(item.StartedAt), "T", " ", 1))
	fmt.Fprintf(&b, "| Rendered | %s |\n", item.Render(item.CreatedAt))
	fmt.Fprintf(&b, "| Named | %s |\n", named(item))
	fmt.Fprintf(&b, "` + "```" + `json\n{\"active\": %t}\n` + "```" + `\n", item.Active)
	return b.String()
}

// label returns a word for a flag beside a word that is not one, so the
// helper is not read as a flag renderer.
func label(item Item) string {
	if item.Active {
		return "Yes"
	}
	return "pending"
}

// named returns through a named result, which the flag-helper rule does not
// read as a word.
func named(item Item) (word string) {
	word = "Yes"
	if !item.Active {
		word = "No"
	}
	return
}
`

// wantRawFindings is every raw flag or instant the fixture prints.
var wantRawFindings = []string{
	"mdraw bool-time *item.EndsAt",
	"mdraw bool-time item.Active",
	"mdraw bool-time item.Active",
	"mdraw bool-time item.Active",
	"mdraw bool-time item.DueDate",
	"mdraw bool-time item.Expires",
	"mdraw bool-time item.Locked",
	"mdraw bool-time item.When",
	`mdraw bool-time item.When.Format("2006-01-02")`,
	"mdraw bool-time strconv.FormatBool(item.Active)",
	"mdraw bool-time string(item.FinishedAt)",
	"mdraw bool-time strings.TrimSpace(item.UpdatedAt)",
	"mdraw bool-time toolutil.EscapeMdTableCell(item.CreatedAt)",
	"mdraw bool-time yesNo(item.Active)",
}

// auditRawFixture runs the audit over the raw fixture with the named
// contexts.
func auditRawFixture(t *testing.T, contexts string) Report {
	t.Helper()
	prog := loadFixture(t, rawFixture)
	sel, err := parseContexts(contexts)
	if err != nil {
		t.Fatalf("parseContexts: %v", err)
	}
	return audit(prog, sel, repoRoot(t))
}

// TestAudit_RawFixture_ReportsEveryFlagAndInstantPrintedRaw pins the second
// verdict shape by shape, with the helper each finding is told to reach for.
func TestAudit_RawFixture_ReportsEveryFlagAndInstantPrintedRaw(t *testing.T) {
	report := auditRawFixture(t, "bool-time")

	missing, extra := diff(entries(report.Findings), wantRawFindings)
	for _, line := range missing {
		t.Errorf("raw value not reported: %s", line)
	}
	for _, line := range extra {
		t.Errorf("unexpected finding: %s", line)
	}
	wants := map[string]string{
		"item.Active":                                "toolutil.BoolEmoji or Card.Bool",
		"item.Locked":                                "toolutil.BoolEmoji or Card.Bool",
		"strconv.FormatBool(item.Active)":            "toolutil.BoolEmoji or Card.Bool",
		"yesNo(item.Active)":                         "toolutil.BoolEmoji or Card.Bool",
		"item.When":                                  "toolutil.FormatTime or Card.Time",
		"item.Expires":                               "toolutil.FormatTime or Card.Time",
		"item.DueDate":                               "toolutil.FormatTime or Card.Time",
		"*item.EndsAt":                               "toolutil.FormatTime or Card.Time",
		"string(item.FinishedAt)":                    "toolutil.FormatTime or Card.Time",
		`item.When.Format("2006-01-02")`:             "toolutil.FormatTime or Card.Time",
		"strings.TrimSpace(item.UpdatedAt)":          "toolutil.FormatTime or Card.Time",
		"toolutil.EscapeMdTableCell(item.CreatedAt)": "toolutil.FormatTime or Card.Time",
	}
	for _, finding := range report.Findings {
		if !strings.HasSuffix(finding.Package, "/mdraw") {
			continue
		}
		if want := wants[finding.Expression]; finding.Wants != want {
			t.Errorf("%s wants %q, want %q", finding.Expression, finding.Wants, want)
		}
		if finding.Context != "bool-time" {
			t.Errorf("%s reported in context %s, want bool-time", finding.Expression, finding.Context)
		}
	}
}

// TestAudit_RawFixture_NamesWhatEachValueIs checks the reasons, since the
// reason is what tells the reader which helper the migration reaches for
// and why the value was not already through it.
func TestAudit_RawFixture_NamesWhatEachValueIs(t *testing.T) {
	report := auditRawFixture(t, "bool-time")
	reasons := map[string]string{}
	for _, finding := range report.Findings {
		if strings.HasSuffix(finding.Package, "/mdraw") {
			reasons[finding.Verb+" "+finding.Expression] = finding.Reason
		}
	}

	cases := []struct {
		name string
		key  string
		want string
	}{
		{name: "the boolean verb", key: "%t item.Active", want: "printed as true or false by %t"},
		{name: "a boolean under a textual verb", key: "%v item.Active", want: "a boolean printed as true or false"},
		{name: "a pointer to a boolean", key: "%v item.Locked", want: "a boolean printed as true or false"},
		{name: "a time.Time", key: "%v item.When", want: "a time.Time printed in Go's default form"},
		{name: "a pointer to a time.Time", key: "%s item.Expires", want: "a time.Time printed in Go's default form"},
		{name: "a layout of the formatter's own", key: `%s item.When.Format("2006-01-02")`, want: "formatted by hand"},
		{name: "a yes-or-no helper", key: "%s yesNo(item.Active)", want: "spells a flag as a word, yesNo"},
		{name: "strconv.FormatBool", key: "cell strconv.FormatBool(item.Active)", want: "spelled by strconv.FormatBool"},
		{name: "a timestamp escaped for the cell", key: "%s toolutil.EscapeMdTableCell(item.CreatedAt)", want: "printed as it arrived"},
		{name: "a timestamp trimmed", key: "%s strings.TrimSpace(item.UpdatedAt)", want: "printed as it arrived"},
		{name: "a timestamp converted from a named string", key: "%s string(item.FinishedAt)", want: "printed as it arrived"},
		{name: "a timestamp read through a pointer", key: "%s *item.EndsAt", want: "printed as it arrived"},
		{name: "a date in a cell builder", key: "cell item.DueDate", want: "printed as it arrived"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := reasons[tc.key]
			if !ok {
				t.Fatalf("no finding for %s; got %v", tc.key, reasons)
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("reason for %s is %q, want one saying %q", tc.key, got, tc.want)
			}
		})
	}
}

// TestAudit_RawFixture_ExcusesADeclaredValueWithItsOwnDirective checks that
// the raw directive excuses the raw verdict alone, and that one excusing
// nothing is stale only once the verdict has run.
func TestAudit_RawFixture_ExcusesADeclaredValueWithItsOwnDirective(t *testing.T) {
	judged := auditRawFixture(t, "bool-time")

	if got := entries(judged.Excused); len(got) != 1 || got[0] != "mdraw bool-time item.StartDate" {
		t.Errorf("excused = %v, want the declared start date alone", got)
	}
	var stale []string
	for _, directive := range judged.StaleDirectives {
		if strings.HasPrefix(directive.Package, fixtureDir) {
			stale = append(stale, string(directive.Kind)+" "+directive.Expression)
		}
	}
	if len(stale) != 1 || stale[0] != "raw item.Retired" {
		t.Errorf("stale = %v, want the one raw directive that excuses nothing", stale)
	}

	unjudged := auditRawFixture(t, allContexts)
	for _, directive := range unjudged.StaleDirectives {
		if strings.HasPrefix(directive.Package, fixtureDir) {
			t.Errorf("a run without the raw verdict reported the raw directive %s as stale", directive.Expression)
		}
	}
	if unjudged.Summary.ByContext["bool-time"] != 0 {
		t.Errorf("the default run reported %d raw value(s), which stages the rule into the gate", unjudged.Summary.ByContext["bool-time"])
	}
}

// TestAudit_RawFixture_AsksBothQuestionsOfOneHole checks that the two
// verdicts are independent: a timestamp escaped for its cell is safe for
// escaping and raw for display, a raw date in a cell is a finding for both,
// and the raw directive on the start date excuses the display verdict alone,
// so its escaping finding stands.
func TestAudit_RawFixture_AsksBothQuestionsOfOneHole(t *testing.T) {
	report := auditRawFixture(t, "all,bool-time")

	var cells []string
	raw := 0
	for _, finding := range report.Findings {
		if !strings.HasSuffix(finding.Package, "/mdraw") {
			continue
		}
		switch finding.Context {
		case "bool-time":
			raw++
		case "table-cell":
			cells = append(cells, finding.Expression)
		default:
			t.Errorf("unexpected %s finding: %s", finding.Context, finding.Expression)
		}
	}
	if raw != len(wantRawFindings) {
		t.Errorf("all,bool-time reported %d raw value(s), want %d", raw, len(wantRawFindings))
	}
	if want := "strings.TrimSpace(item.UpdatedAt) item.StartDate string(item.FinishedAt) *item.EndsAt item.DueDate"; strings.Join(cells, " ") != want {
		t.Errorf("all,bool-time reported the cell findings %v, want %s", cells, want)
	}
	if report.Summary.Contexts != "table-cell, heading, list-item, link-label, link-destination, fence, bool-time" {
		t.Errorf("all,bool-time judged %q", report.Summary.Contexts)
	}
}

// TestRawByType_Types_AnswersForBooleansAndInstantsOnly checks the type rule
// on its own, over the shapes an output struct is built from.
func TestRawByType_Types_AnswersForBooleansAndInstantsOnly(t *testing.T) {
	prog := loadFixture(t, rawFixture)
	pkg := fixturePackage(t, prog, "mdraw")
	item := pkg.Types.Scope().Lookup("Item").Type().Underlying().(*types.Struct)
	fieldType := func(name string) types.Type {
		for field := range item.Fields() {
			if field.Name() == name {
				return field.Type()
			}
		}
		t.Fatalf("Item has no field %s", name)
		return nil
	}

	cases := []struct {
		name string
		typ  types.Type
		want rawKind
	}{
		{name: "a boolean", typ: fieldType("Active"), want: rawBool},
		{name: "a pointer to a boolean", typ: fieldType("Locked"), want: rawBool},
		{name: "a time.Time", typ: fieldType("When"), want: rawTime},
		{name: "a pointer to a time.Time", typ: fieldType("Expires"), want: rawTime},
		{name: "a duration", typ: fieldType("Elapsed"), want: rawNone},
		{name: "a string", typ: fieldType("Title"), want: rawNone},
		{name: "a number", typ: fieldType("Count"), want: rawNone},
		{name: "nothing at all", typ: nil, want: rawNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := rawByType(tc.typ); got != tc.want {
				t.Errorf("rawByType(%v) = %v, want %v", tc.typ, got, tc.want)
			}
		})
	}
}

// TestRawSelector_UntypedField_IsNotAFinding checks the guard on a selector
// the type checker recorded nothing for, which a synthetic expression is and
// which must read as nothing rather than as a timestamp.
func TestRawSelector_UntypedField_IsNotAFinding(t *testing.T) {
	prog := loadFixture(t, rawFixture)
	pkg := fixturePackage(t, prog, "mdraw")

	kind, why := rawSelector(pkg, &ast.SelectorExpr{X: ast.NewIdent("item"), Sel: ast.NewIdent("CreatedAt")})

	if kind != rawNone || why != "" {
		t.Errorf("rawSelector on an untyped field = %v (%s), want nothing", kind, why)
	}
}

// TestRawKind_Wants_NamesTheHelper checks the helper each kind sends the
// reader to, and that the kind meaning nothing names none.
func TestRawKind_Wants_NamesTheHelper(t *testing.T) {
	cases := []struct {
		name string
		kind rawKind
		want string
	}{
		{name: "a flag", kind: rawBool, want: "toolutil.BoolEmoji or Card.Bool"},
		{name: "an instant", kind: rawTime, want: "toolutil.FormatTime or Card.Time"},
		{name: "neither", kind: rawNone, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.kind.wants(); got != tc.want {
				t.Errorf("wants() = %q, want %q", got, tc.want)
			}
		})
	}
}
