// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// minKeysetRoutes is the floor below which the keyset half of a record is not
// describing a GitLab, for the failure every other floor exists for: a walk
// that half ran returns fewer sites rather than failing, and a route it lost
// reads downstream as one GitLab pages by offset. 19.5.0-ee has 53 routes
// whose code reaches a keyset paginator, and this keeps the two-thirds margin
// the other floors keep.
const minKeysetRoutes = 35

// keysetProblems reports the ways the keyset half of a version 5 record cannot
// be rested on. Each is a shape a reader would misread, and the last is the
// one the walk itself cannot see: a route that says it pages by keyset, by
// declaring a cursor, and that the walk found no paginator for.
func keysetProblems(doc apilive.Document) []string {
	problems := paginatorProblems(doc.KeysetPaginators)
	if got := doc.KeysetRoutes(); got < minKeysetRoutes {
		problems = append(problems, fmt.Sprintf(
			"it holds %d routes reaching a keyset paginator and a GitLab has at least %d: the keyset walk did not finish, "+
				"or an introspection that does not walk it wrote the record",
			got, minKeysetRoutes,
		))
	}
	problems = append(problems, siteProblems(doc.Routes)...)
	return append(problems, declaredCursorProblems(doc)...)
}

// paginatorProblems holds the keyset_paginators block to the paginators the
// introspection probes: each answered, and nothing else answered.
//
// A record without the block is what a version 4 script writes, and labeled
// version 5 it would read as an instance where every route pages by offset,
// which is the inversion the version exists to stop.
func paginatorProblems(paginators map[string][]string) []string {
	if paginators == nil {
		return []string{"it carries no keyset_paginators block, which every version 5 introspection writes: " +
			"it reads like a version 4 record labeled 5, and every route in it would read as one GitLab pages by offset"}
	}
	probed := apilive.Probed()
	var unanswered, unknown []string
	for _, name := range probed {
		if len(paginators[name]) == 0 {
			unanswered = append(unanswered, name)
		}
	}
	for name := range paginators {
		if !slices.Contains(probed, name) {
			unknown = append(unknown, name)
		}
	}
	problems := reportSites(nil, unanswered,
		"%d keyset paginators name no parameter (%s): the probe did not run, and a route each serves would read "+
			"as followed by nothing")
	return reportSites(problems, unknown,
		"%d keyset paginators are ones nothing here reads (%s): introspect.rb probes a paginator this build does not know")
}

// siteProblems holds every keyset site to what a reader takes from it: a
// paginator it knows, a place to read, and the parameter the next page is
// asked by.
func siteProblems(routes []apilive.Route) []string {
	known := apilive.Paginators()
	var unknown, unlocated, uncursored []string
	for _, route := range routes {
		name := route.Method + " " + route.Path
		for _, site := range route.Keyset {
			if !slices.Contains(known, site.Paginator) {
				unknown = append(unknown, name+" "+quoted(site.Paginator))
			}
			if site.File == "" || site.Line <= 0 {
				unlocated = append(unlocated, name)
			}
			if len(site.Cursor) == 0 {
				uncursored = append(uncursored, name+" "+site.File+":"+strconv.Itoa(site.Line))
			}
		}
	}
	problems := reportSites(nil, unknown,
		"%d keyset sites name a paginator nothing here reads (%s): introspect.rb met a paginator this build does not know")
	problems = reportSites(problems, unlocated,
		"%d keyset sites carry no file and line (%s): nobody could read the code that serves the page")
	return reportSites(problems, uncursored,
		"%d keyset sites name no parameter their next page is asked by (%s): a reader could not say what to send back")
}

// declaredCursorProblems holds the walk to what the routes declare.
//
// The walk follows calls on the endpoint and never into a finder or a service,
// so a route paged by keyset somewhere it cannot see would read as one paged
// by offset, silently. A route that declares a parameter the record knows as
// a cursor says it pages by keyset in its own words, which is evidence the
// walk did not produce: such a route recording no keyset site, or no site
// asked by that parameter, means the walk and GitLab disagree, and only
// teaching the walk settles it.
func declaredCursorProblems(doc apilive.Document) []string {
	vocabulary := doc.CursorParameters()
	var unmarked, disagreeing []string
	for _, route := range doc.Routes {
		declared := declaredCursors(route, vocabulary)
		if len(declared) == 0 {
			continue
		}
		name := route.Method + " " + route.Path
		if !route.PagesByKeyset() {
			unmarked = append(unmarked, name+" ("+strings.Join(declared, ", ")+")")
			continue
		}
		for _, parameter := range declared {
			if !slices.Contains(route.Cursors(), parameter) {
				disagreeing = append(disagreeing, name+" "+parameter)
			}
		}
	}
	problems := reportSites(nil, unmarked,
		"%d routes declare a cursor parameter and record no keyset pagination (%s): the walk missed the code that "+
			"reads it, and each would read as a route GitLab pages by offset")
	return reportSites(problems, disagreeing,
		"%d routes declare a cursor parameter none of their keyset sites is asked by (%s): the record and the "+
			"route's own declaration disagree about what its next page is asked by")
}

// declaredCursors lists, sorted, the parameters a route declares that the
// record knows as cursors.
func declaredCursors(route apilive.Route, vocabulary []string) []string {
	var declared []string
	for name := range route.Params {
		if slices.Contains(vocabulary, name) {
			declared = append(declared, name)
		}
	}
	slices.Sort(declared)
	return declared
}

// keysetSummary is the keyset line of a report: how many routes reach a keyset
// paginator, and how many only behind a flag GitLab ships off, which an
// instance nobody configured serves by offset.
func keysetSummary(doc apilive.Document) string {
	flagged := 0
	for _, route := range doc.Routes {
		if route.PagesByKeyset() && !slices.ContainsFunc(route.Keyset, apilive.KeysetSite.OnByDefault) {
			flagged++
		}
	}
	return fmt.Sprintf("%d routes reach a keyset paginator, %d of them only behind a feature flag GitLab ships off",
		doc.Source.KeysetRoutes, flagged)
}
