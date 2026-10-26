// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package apilive

import (
	"slices"
	"sort"
)

// This file is the keyset half of the record, added in schema version 5 for
// issue 1165: where each route's code reaches one of GitLab's keyset
// paginators and which parameter its next page is asked by, and which
// parameter each of those paginators names in the next link it writes.
//
// GitLab reads pagination=keyset without declaring it, so a route's params
// cannot answer which routes page by keyset: at 19.5.0 four of the 53 routes
// whose code reaches a keyset paginator declare `pagination`, and 22 declare
// no cursor either. The introspection therefore walks the code each route's
// block reaches through its helpers and records the places that call one, and
// asks the paginators themselves what their next link names rather than
// copying the names here.
//
// Reaching a keyset paginator is where GitLab can serve a keyset page, not a
// promise that it does. paginate_with_strategies picks its paginator by the
// model of the relation it is handed and serves a keyset page only for the
// models and orders it supports, answering 405 to the rest, and the walk
// reads code, not the relation a request builds. So a site records the
// cursor its paginator names when the model is one it supports, and a route
// whose model is not, the service account token lists at 19.5.0 among them,
// is recorded with a cursor GitLab never reads there. A reader holding a
// route to whether it really pages by keyset, R-PAGE in cmd/audit_1to1,
// declares those routes rather than trusting the site.

// The paginators a keyset site names, as introspect.rb spells them.
const (
	// PaginatorStrategies is paginate_with_strategies, the helper that pages
	// by keyset when pagination=keyset: by `cursor` when the relation's model
	// declares keyset orderings, and by `id_after` and `id_before` when the
	// call passes `use_cursor: false` or the model declares none, where
	// anything but Project ordered by id answers 405. The model is the
	// relation's, which the walk cannot see, so a site records `cursor`
	// unless the call passes `use_cursor: false`, and on a model that
	// declares no keyset orderings that cursor is one GitLab never reads.
	PaginatorStrategies = "paginate_with_strategies"
	// PaginatorGitalyKeysetPager is Gitlab::Pagination::GitalyKeysetPager,
	// asked by `page_token`, for branches, tags, the tree and commits.
	PaginatorGitalyKeysetPager = "gitaly_keyset_pager"
	// PaginatorGitalyPagination is a finder executed with
	// `gitaly_pagination: true`, which pages through Gitaly by the same
	// `page_token` without the pager.
	PaginatorGitalyPagination = "gitaly_pagination"
	// PaginatorHeaderBuilder is a route writing its own keyset next link or
	// cursor headers through Gitlab::Pagination::Keyset::HeaderBuilder.
	PaginatorHeaderBuilder = "keyset_header_builder"
	// PaginatorKeysetPaginate is a relation paged with keyset_paginate.
	PaginatorKeysetPaginate = "keyset_paginate"
)

// The keys of [Document.KeysetPaginators]: the paginators the introspection
// asks which parameter their next link names.
const (
	// ProbedCursorPager is CursorPager, which paginate_with_strategies uses
	// for a model declaring keyset orderings.
	ProbedCursorPager = "cursor_pager"
	// ProbedPager is Keyset::Pager, which it uses otherwise.
	ProbedPager = "pager"
	// ProbedGitalyKeysetPager is GitalyKeysetPager.
	ProbedGitalyKeysetPager = "gitaly_keyset_pager"
	// ProbedCursorHeaders is the parameter the X-NEXT-CURSOR header is sent
	// back as, read off the CursorPager answer that writes both.
	ProbedCursorHeaders = "cursor_headers"
)

// Paginators lists every paginator a keyset site may name, in the order they
// are documented above.
func Paginators() []string {
	return []string{
		PaginatorStrategies, PaginatorGitalyKeysetPager, PaginatorGitalyPagination,
		PaginatorHeaderBuilder, PaginatorKeysetPaginate,
	}
}

// Probed lists every paginator the keyset_paginators block must answer for.
func Probed() []string {
	return []string{ProbedCursorPager, ProbedPager, ProbedGitalyKeysetPager, ProbedCursorHeaders}
}

// KeysetSite is one place a route's code calls a keyset paginator, which is
// where GitLab can serve the route a keyset page (see the top of this file
// for why that is not the same as saying it does).
type KeysetSite struct {
	// Paginator is what would serve the page, one of [Paginators].
	Paginator string `json:"paginator"`
	// File and Line locate the call, repository relative, and Text is that
	// line read back from the image.
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text,omitempty"`
	// Via names the helpers the walk went through from the route's block to
	// the call, outermost first, and is empty when the block makes it.
	Via []string `json:"via,omitempty"`
	// Cursor is the parameter, or the pair, the next page is asked by when
	// the paginator serves one. At a [PaginatorStrategies] site it is the
	// cursor of the paginator a model with keyset orderings gets, unless the
	// call passes `use_cursor: false`.
	Cursor []string `json:"cursor,omitempty"`
	// Flags are the feature flags the call is behind, in the two shapes the
	// walk reads them in: `if Feature.enabled?(:flag)` or
	// `unless Feature.disabled?(:flag)`.
	Flags []KeysetFlag `json:"flags,omitempty"`
}

// KeysetFlag is a feature flag a keyset site is behind.
type KeysetFlag struct {
	Name string `json:"name"`
	// DefaultEnabled is whether GitLab turns the flag on unless an
	// administrator turns it off, and nil when GitLab defines no such flag.
	DefaultEnabled *bool `json:"default_enabled,omitempty"`
}

// OnByDefault reports whether the site is reached on an instance nobody has
// configured: it is behind no flag, or behind none GitLab ships off. A flag
// GitLab does not define is taken as off, since Feature.enabled? answers
// false for one.
func (s KeysetSite) OnByDefault() bool {
	for _, flag := range s.Flags {
		if flag.DefaultEnabled == nil || !*flag.DefaultEnabled {
			return false
		}
	}
	return true
}

// PagesByKeyset reports whether any code the route reaches calls a keyset
// paginator. In version 5 a route that reaches none is one GitLab answers with
// an offset page whatever pagination says. One that reaches one can still
// answer a keyset request with 405, by the relation's model or by the order
// asked for, which the record does not say.
func (r Route) PagesByKeyset() bool { return len(r.Keyset) > 0 }

// Cursors lists, sorted and once each, the parameters any of the route's
// keyset sites is asked by.
func (r Route) Cursors() []string {
	var cursors []string
	for _, site := range r.Keyset {
		cursors = append(cursors, site.Cursor...)
	}
	slices.Sort(cursors)
	return slices.Compact(cursors)
}

// KeysetRoutes counts the routes whose code reaches a keyset paginator, the
// figure the source block carries and a floor is set against.
func (d Document) KeysetRoutes() int {
	count := 0
	for _, route := range d.Routes {
		if route.PagesByKeyset() {
			count++
		}
	}
	return count
}

// CursorParameters is every parameter the record says a next page can be
// asked by: what each probed paginator names and what each site names. A
// route declaring one of them is one that says it pages by keyset.
func (d Document) CursorParameters() []string {
	seen := map[string]bool{}
	for _, names := range d.KeysetPaginators {
		for _, name := range names {
			seen[name] = true
		}
	}
	for _, route := range d.Routes {
		for _, name := range route.Cursors() {
			seen[name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
