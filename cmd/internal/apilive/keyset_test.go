// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package apilive

import (
	"slices"
	"testing"
)

// TestPaginatorNames_SpellWhatTheScriptWrites holds the names a keyset site
// and the probe carry to introspect.rb's spelling, which is what a record is
// written in: a constant renamed on one side only would make the gate refuse
// every site, or pass one nothing reads.
func TestPaginatorNames_SpellWhatTheScriptWrites(t *testing.T) {
	t.Parallel()
	t.Run("the paginators a site names", func(t *testing.T) {
		t.Parallel()
		want := []string{
			"paginate_with_strategies", "gitaly_keyset_pager", "gitaly_pagination",
			"keyset_header_builder", "keyset_paginate",
		}
		if got := Paginators(); !slices.Equal(got, want) {
			t.Errorf("Paginators() = %q, want %q", got, want)
		}
	})
	t.Run("the paginators the probe answers for", func(t *testing.T) {
		t.Parallel()
		want := []string{"cursor_pager", "pager", "gitaly_keyset_pager", "cursor_headers"}
		if got := Probed(); !slices.Equal(got, want) {
			t.Errorf("Probed() = %q, want %q", got, want)
		}
	})
}

// TestKeysetSite_OnByDefault_IsWhetherNoFlagGitLabShipsOffStandsInTheWay
// verifies what an instance nobody configured serves: a site behind no flag,
// or behind flags all on by default, pages by keyset there; one flag shipped
// off, or one GitLab does not define, is enough to turn it off.
func TestKeysetSite_OnByDefault_IsWhetherNoFlagGitLabShipsOffStandsInTheWay(t *testing.T) {
	t.Parallel()
	on := KeysetFlag{Name: "on", DefaultEnabled: new(true)}
	off := KeysetFlag{Name: "off", DefaultEnabled: new(false)}
	undefined := KeysetFlag{Name: "undefined"}
	for _, testCase := range []struct {
		name  string
		flags []KeysetFlag
		want  bool
	}{
		{name: "behind no flag", want: true},
		{name: "behind a flag on by default", flags: []KeysetFlag{on}, want: true},
		{name: "behind a flag shipped off", flags: []KeysetFlag{off}},
		{name: "behind a flag GitLab does not define", flags: []KeysetFlag{undefined}},
		{name: "behind one on and one off", flags: []KeysetFlag{on, off}},
		{name: "behind two on", flags: []KeysetFlag{on, on}, want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := (KeysetSite{Flags: testCase.flags}).OnByDefault(); got != testCase.want {
				t.Errorf("OnByDefault() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestRoute_KeysetReaders_AnswerFromItsSites verifies the two questions a
// reader asks of one route: whether it pages by keyset at all, and by which
// parameters, sorted and once each across its sites.
func TestRoute_KeysetReaders_AnswerFromItsSites(t *testing.T) {
	t.Parallel()
	route := Route{Keyset: []KeysetSite{
		{Cursor: []string{"id_before", "id_after"}},
		{Cursor: []string{"cursor", "id_after"}},
		{},
	}}

	t.Run("a route with sites pages by keyset", func(t *testing.T) {
		t.Parallel()
		if !route.PagesByKeyset() {
			t.Error("PagesByKeyset() = false, want true")
		}
	})
	t.Run("a route with none does not", func(t *testing.T) {
		t.Parallel()
		if (Route{}).PagesByKeyset() {
			t.Error("PagesByKeyset() = true, want false")
		}
	})
	t.Run("its cursors are every site's, sorted and once each", func(t *testing.T) {
		t.Parallel()
		if got, want := route.Cursors(), []string{"cursor", "id_after", "id_before"}; !slices.Equal(got, want) {
			t.Errorf("Cursors() = %q, want %q", got, want)
		}
	})
	t.Run("a route with none has no cursors", func(t *testing.T) {
		t.Parallel()
		if got := (Route{}).Cursors(); len(got) != 0 {
			t.Errorf("Cursors() = %q, want none", got)
		}
	})
}

// TestDocument_KeysetReaders_SpanTheWholeRecord verifies the two figures a
// gate reads off a record: how many routes page by keyset, and every
// parameter the record knows as a cursor, from the probe and from the sites
// alike, sorted and once each.
func TestDocument_KeysetReaders_SpanTheWholeRecord(t *testing.T) {
	t.Parallel()
	doc := Document{
		KeysetPaginators: map[string][]string{
			ProbedPager:             {"id_before", "id_after"},
			ProbedGitalyKeysetPager: {"page_token"},
			ProbedCursorPager:       {"cursor"},
		},
		Routes: []Route{
			{Keyset: []KeysetSite{{Cursor: []string{"last"}}}},
			{},
			{Keyset: []KeysetSite{{Cursor: []string{"cursor"}}}},
			{Keyset: []KeysetSite{{}}},
		},
	}

	t.Run("the routes paging by keyset are counted", func(t *testing.T) {
		t.Parallel()
		if got := doc.KeysetRoutes(); got != 3 {
			t.Errorf("KeysetRoutes() = %d, want 3", got)
		}
	})
	t.Run("the cursors span the probe and the sites", func(t *testing.T) {
		t.Parallel()
		want := []string{"cursor", "id_after", "id_before", "last", "page_token"}
		if got := doc.CursorParameters(); !slices.Equal(got, want) {
			t.Errorf("CursorParameters() = %q, want %q", got, want)
		}
	})
	t.Run("a record with neither knows no cursor", func(t *testing.T) {
		t.Parallel()
		if got := (Document{}).CursorParameters(); len(got) != 0 {
			t.Errorf("CursorParameters() = %q, want none", got)
		}
	})
}
