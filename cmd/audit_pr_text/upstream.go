// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"regexp"
	"slices"
)

// upstreamURL matches a full gitlab.com URL to a merge request or an issue,
// up to its number: GitLab reads such a URL in a commit message as a
// reference to that item, whatever project it belongs to.
var upstreamURL = regexp.MustCompile(`(?i)\bhttps?://gitlab\.com/[\w.-]+(?:/[\w.-]+)*/-/(?:merge_requests|issues)/\d+`)

// upstreamPath matches a reference GitLab resolves across projects: a
// project's full path, its groups and nested groups included, followed by !N
// for a merge request or #N for an issue. The path must not continue one
// already started, so a URL or a longer path is not read from its middle;
// the reference itself is the first group.
var upstreamPath = regexp.MustCompile(`(?:^|[^\w./-])([\w.-]+(?:/[\w.-]+)+[!#]\d+)\b`)

// upstreamReferences returns the references to another project's merge
// requests and issues that line carries, in the order it carries them and
// each once. A bare #N or !N is not one: GitLab resolves it in the project
// the commit is in, which for this repository is its own mirror.
func upstreamReferences(line string) []string {
	type found struct {
		at  int
		ref string
	}
	var all []found
	for _, match := range upstreamURL.FindAllStringIndex(line, -1) {
		all = append(all, found{at: match[0], ref: line[match[0]:match[1]]})
	}
	for _, match := range upstreamPath.FindAllStringSubmatchIndex(line, -1) {
		all = append(all, found{at: match[2], ref: line[match[2]:match[3]]})
	}
	slices.SortFunc(all, func(a, b found) int { return cmp.Compare(a.at, b.at) })
	var refs []string
	for _, f := range all {
		if !slices.Contains(refs, f.ref) {
			refs = append(refs, f.ref)
		}
	}
	return refs
}
