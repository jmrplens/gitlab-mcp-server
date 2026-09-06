package main

import (
	"strings"
	"testing"
)

// TestContextAt_Templates_ClassifiesTheConstructTheHoleSitsIn covers each
// construct the audit judges and the prose it skips, including the shapes that
// look like a construct and are not.
func TestContextAt_Templates_ClassifiesTheConstructTheHoleSitsIn(t *testing.T) {
	cases := []struct {
		name     string
		template string
		want     mdContext
	}{
		{name: "table cell", template: "| %s |", want: ctxCell},
		{name: "indented table cell", template: "  | %s |", want: ctxCell},
		{name: "cell on the second line", template: "## Title\n| %s |", want: ctxCell},
		{name: "heading", template: "## %s", want: ctxHeading},
		{name: "deepest heading", template: "###### %s", want: ctxHeading},
		{name: "seven hashes is not a heading", template: "####### %s", want: ctxProse},
		{name: "hash without a space is not a heading", template: "##%s", want: ctxProse},
		{name: "hash alone is not a heading", template: "%s", want: ctxProse},
		{name: "bullet list item", template: "- %s", want: ctxListItem},
		{name: "star list item", template: "* %s", want: ctxListItem},
		{name: "ordered list item", template: "1. %s", want: ctxListItem},
		{name: "parenthesised ordered item", template: "12) %s", want: ctxListItem},
		{name: "a number alone is not a list", template: "12%s", want: ctxProse},
		{name: "a number and a dot at the end is not a list", template: "12.%s", want: ctxProse},
		{name: "a dash without a space is not a list", template: "-%s", want: ctxProse},
		{name: "link label", template: "[%s](https://example.invalid)", want: ctxLinkLabel},
		{name: "link destination", template: "[title](%s)", want: ctxLinkDest},
		{name: "link label inside a cell", template: "| [%s](x) |", want: ctxLinkLabel},
		{name: "a closed link is prose again", template: "[a](b) %s", want: ctxProse},
		{name: "a closed bracket is not a label", template: "[a] %s", want: ctxProse},
		{name: "prose", template: "%s wrote it", want: ctxProse},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			offset := strings.Index(tc.template, "%s")
			if got := contextAt(tc.template, offset); got != tc.want {
				t.Errorf("contextAt(%q) = %s, want %s", tc.template, got, tc.want)
			}
		})
	}
}

// TestMdContext_Values_NameTheHelperTheyNeed checks that every context reports
// the helper a finding tells its reader to reach for, and that prose is the
// one that needs none.
func TestMdContext_Values_NameTheHelperTheyNeed(t *testing.T) {
	cases := []struct {
		name       string
		ctx        mdContext
		label      string
		wants      string
		structural bool
	}{
		{name: "cell", ctx: ctxCell, label: "table-cell", wants: "toolutil.EscapeMdTableCell", structural: true},
		{name: "list item", ctx: ctxListItem, label: "list-item", wants: "toolutil.EscapeMdTableCell", structural: true},
		{name: "heading", ctx: ctxHeading, label: "heading", wants: "toolutil.EscapeMdHeading", structural: true},
		{name: "link label", ctx: ctxLinkLabel, label: "link-label", wants: "toolutil.MdTitleLink", structural: true},
		{name: "link destination", ctx: ctxLinkDest, label: "link-destination", wants: "toolutil.MdTitleLink", structural: true},
		{name: "prose", ctx: ctxProse, label: "prose", wants: "", structural: false},
		{name: "a value that names nothing", ctx: mdContext(99), label: "prose", wants: "", structural: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ctx.String(); got != tc.label {
				t.Errorf("String() = %q, want %q", got, tc.label)
			}
			if got := tc.ctx.wants(); got != tc.wants {
				t.Errorf("wants() = %q, want %q", got, tc.wants)
			}
			if got := tc.ctx.structural(); got != tc.structural {
				t.Errorf("structural() = %v, want %v", got, tc.structural)
			}
		})
	}
}

// TestParseContexts_Values_SelectsWhatToJudge covers the flag's accepted
// spellings: the default, the empty value that means the same, and a list with
// the spacing and repetition a command line really carries.
func TestParseContexts_Values_SelectsWhatToJudge(t *testing.T) {
	everything := "table-cell, heading, list-item, link-label, link-destination"
	cases := []struct {
		name      string
		value     string
		wantLabel string
		judges    []mdContext
		refuses   []mdContext
	}{
		{
			name:      "all",
			value:     allContexts,
			wantLabel: everything,
			judges:    []mdContext{ctxCell, ctxHeading, ctxListItem, ctxLinkLabel, ctxLinkDest},
			refuses:   []mdContext{ctxProse},
		},
		{name: "empty means all", value: "   ", wantLabel: everything, judges: []mdContext{ctxCell}},
		{
			name:      "a list",
			value:     " table-cell , heading ,, table-cell ",
			wantLabel: "table-cell, heading",
			judges:    []mdContext{ctxCell, ctxHeading},
			refuses:   []mdContext{ctxListItem, ctxLinkLabel},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sel, err := parseContexts(tc.value)
			if err != nil {
				t.Fatalf("parseContexts(%q): %v", tc.value, err)
			}
			if sel.label != tc.wantLabel {
				t.Errorf("label = %q, want %q", sel.label, tc.wantLabel)
			}
			assertJudges(t, sel, tc.judges, tc.refuses)
		})
	}
}

// assertJudges checks what a selection judges and what it leaves alone.
func assertJudges(t *testing.T, sel selection, judges, refuses []mdContext) {
	t.Helper()
	for _, ctx := range judges {
		if !sel.judges(ctx) {
			t.Errorf("selection does not judge %s", ctx)
		}
	}
	for _, ctx := range refuses {
		if sel.judges(ctx) {
			t.Errorf("selection judges %s, which it was not asked to", ctx)
		}
	}
}

// TestParseContexts_WrongValues_AreRefused checks that a value naming no
// context is an error rather than a run with nothing to judge, which would
// read as a gate that passed.
func TestParseContexts_WrongValues_AreRefused(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{name: "an unknown name", value: "cells", want: "unknown Markdown context"},
		{name: "nothing but separators", value: ",,", want: "no Markdown context selected"},
		{name: "prose is not selectable", value: "prose", want: "unknown Markdown context"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseContexts(tc.value)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("parseContexts(%q) error = %v, want one saying %q", tc.value, err, tc.want)
			}
		})
	}
}

// TestContextNames_Default_ListsEverySelectableContext checks the help text
// the flag prints, since a wrong value is answered with it.
func TestContextNames_Default_ListsEverySelectableContext(t *testing.T) {
	names := contextNames()

	for _, ctx := range structuralContexts {
		if !strings.Contains(names, ctx.String()) {
			t.Errorf("contextNames() = %q, missing %s", names, ctx)
		}
	}
	if strings.Contains(names, "prose") {
		t.Errorf("contextNames() = %q, which offers a context the audit never judges", names)
	}
}
