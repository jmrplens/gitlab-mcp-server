package main

import "strings"

// docAreaForPackage maps an owner package name to the GitLab API doc area
// (the doc/api/<area>.md basename). Returns ok=false when no mapping exists so
// the domain is reported as needing manual triage.
//
// The map is intentionally explicit rather than heuristic: GitLab doc filenames
// do not follow a single rule from the Go package name, and a wrong mapping
// would silently grade an action against the wrong tier.
func docAreaForPackage(pkg string) (area string, ok bool) {
	area, ok = docAreaByPackage[pkg]
	return area, ok
}

// docAreaByPackage is populated in doc_map_data.go.
var docAreaByPackage = map[string]string{}

// docRef identifies one documentation page. Area is relative to doc/api/ by
// default; when userDoc is set it is relative to doc/, for endpoint families
// whose only tier badge lives on a user-facing page.
type docRef struct {
	area    string
	userDoc bool
}

// docPath returns the repo-relative markdown path, for report notes.
func (r docRef) docPath() string {
	if r.userDoc {
		return "doc/" + r.area + ".md"
	}
	return "doc/api/" + r.area + ".md"
}

// actionDocOverrides maps canonical action-ID prefixes to the page that
// actually documents that endpoint family when the owner package's page does
// not. Two documentation gaps require this indirection:
//
//   - cross-page families: the owner package's page omits the endpoints
//     entirely (group webhooks are absent from groups.md and live on
//     group_webhooks.md);
//   - badge-less API sections: the API page documents the endpoints but the
//     tier badge exists only on a user-facing page (merge request dependencies
//     have no badge in merge_requests.md; the tier is stated in
//     doc/user/project/merge_requests/dependencies.md).
//
// Unlike acceptedTierExceptions, no tier is hardcoded here: the expected tier
// is parsed from the referenced page's own badge, keeping the audit
// doc-grounded.
var actionDocOverrides = []struct {
	prefix string
	ref    docRef
}{
	{prefix: "group.hook_", ref: docRef{area: "group_webhooks"}},
	{prefix: "merge_request.dependencies_", ref: docRef{area: "user/project/merge_requests/dependencies", userDoc: true}},
	{prefix: "merge_request.dependency_", ref: docRef{area: "user/project/merge_requests/dependencies", userDoc: true}},
}

// docOverrideForAction returns the doc page override for a canonical action ID,
// if one is declared.
func docOverrideForAction(id string) (docRef, bool) {
	for _, o := range actionDocOverrides {
		if strings.HasPrefix(id, o.prefix) {
			return o.ref, true
		}
	}
	return docRef{}, false
}
