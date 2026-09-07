package paths

import (
	"slices"
	"sort"
	"strings"
)

// endpointDeclaration records why a request this server makes is spelled out
// on no page of GitLab's API documentation.
//
// It is the same shape [silentOwnerDeclaration] has, and it exists for the same
// reason: the check that finds these is worth gating on, and it can only gate
// once the entries a human has already adjudicated are written down where the
// next reader can disagree with them. Every one of these was read against the
// documentation before it was written here.
type endpointDeclaration struct {
	// Shape is the path this covers, matched segment by segment against a
	// recorded path. A "*" matches one segment, whatever it holds, and a
	// trailing "..." matches every remaining segment. It is written as the
	// recorder writes a path: no /api/v4, and a leading slash.
	Shape string
	// Methods are the methods this covers, and empty means all of them.
	Methods []string
	// Category says what kind of absence this is.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it still
	// holds.
	Reason string
}

// Declaration categories.
const (
	// categoryUndocumentedAlias is a path GitLab still serves and no longer
	// writes down, because it renamed the endpoint and kept the old spelling
	// working.
	categoryUndocumentedAlias = "undocumented-alias"

	// categoryDocumentedElsewhere is an endpoint documented outside doc/api,
	// which is the corpus this reads.
	categoryDocumentedElsewhere = "documented-elsewhere"

	// categoryDocumentedInProse is an endpoint a page describes in a sentence
	// or a curl example and never as the `METHOD /path` line this matches.
	categoryDocumentedInProse = "documented-in-prose"

	// categoryDocumentedWithoutScope is an endpoint whose page writes the
	// endpoint line without the parent it lives under, so the shape on the
	// page is one segment shorter than the one anybody can send.
	categoryDocumentedWithoutScope = "documented-without-scope"

	// categoryUndocumentedMethod is an endpoint the documentation spells out
	// for another method than the one we use.
	categoryUndocumentedMethod = "undocumented-method"

	// categoryDeprecatedRoute is a route GitLab still serves and has stopped
	// writing down because it named a replacement.
	categoryDeprecatedRoute = "deprecated-route"

	// categoryUndocumentedAPI is an endpoint GitLab publishes no reference
	// page for at all, which today is the experimental Knowledge Graph.
	categoryUndocumentedAPI = "undocumented-api"

	// categoryClientDefect is a path we send that GitLab does not have, and
	// the defect is in client-go rather than here. It is declared rather than
	// fixed because the fix belongs upstream, and it is named here so the
	// next reader meets the analysis rather than the symptom.
	categoryClientDefect = "client-defect"
)

// declaredUndocumentedEndpoints holds every recorded endpoint that GitLab's own
// API documentation does not spell out, each with the reason it does not.
//
// Anything not in here fails the check, which is what the dimension was asked
// for: a path GitLab does not have is a failure and not a report. The bar for
// adding an entry is the bar every one of these met, which is a page read and
// an explanation of why the endpoint is missing from it, never a way to make
// the check quiet.
var declaredUndocumentedEndpoints = []endpointDeclaration{
	{
		Shape:    "/projects/*/services/...",
		Category: categoryUndocumentedAlias,
		Reason: "client-go addresses the project integrations through /services/, the spelling GitLab replaced with " +
			"/integrations/ and still serves. integrations.md documents only the new one, so every integration " +
			"endpoint we reach looks undocumented. The request works; the day it stops, client-go is where it changes.",
	},
	{
		Shape:    "/projects/*/attestations/*",
		Methods:  []string{"GET"},
		Category: categoryDocumentedWithoutScope,
		Reason: "attestations.md writes its endpoints as `GET /:id/attestations/:subject_digest`, leaving the " +
			"projects scope out of the line while its own curl example spells /api/v4/projects/72356192/... " +
			"The endpoint we send is the one the example shows.",
	},
	{
		Shape:    "/projects/*/attestations/*/download",
		Methods:  []string{"GET"},
		Category: categoryDocumentedWithoutScope,
		Reason:   "the download half of the same page, written the same way.",
	},
	{
		Shape:    "/projects/*/repository/files/*/raw",
		Methods:  []string{"HEAD"},
		Category: categoryUndocumentedMethod,
		Reason: "repository_files.md documents the GET. GitLab answers HEAD on it with the same headers and no " +
			"body, which is how the file tools read a file's size and content type without downloading it, and " +
			"the documentation writes no HEAD line for any endpoint.",
	},
	{
		Shape:    "/projects/*/packages/generic/...",
		Category: categoryDocumentedElsewhere,
		Reason: "the generic package registry is documented under doc/user/packages/generic_packages, not under " +
			"doc/api, which is the corpus this compares against.",
	},
	{
		Shape:    "/groups/*/-/search",
		Category: categoryUndocumentedAlias,
		Reason: "client-go spells the scoped search with the /-/ segment (search.go's routeGroupsIDSearch). " +
			"GitLab's route makes that segment optional and search.md documents only /groups/:id/search, so the " +
			"request works and the spelling is the SDK's.",
	},
	{
		Shape:    "/projects/*/-/search",
		Category: categoryUndocumentedAlias,
		Reason:   "the project half of the same client-go spelling (routeProjectsIDSearch).",
	},
	{
		Shape:    "/projects/*/pipelines/*/bridges",
		Category: categoryDeprecatedRoute,
		Reason: "jobs.md deprecated the bridges route in GitLab 19.2 in favor of trigger_jobs and now spells only " +
			"the new one, while still serving the old. client-go's ListPipelineBridges is what we call, so the " +
			"move follows the SDK rather than leading it.",
	},
	{
		Shape:    "/projects/*/terraform/state/...",
		Category: categoryDocumentedElsewhere,
		Reason: "the Terraform state backend is documented under doc/administration/terraform_state and " +
			"doc/user/infrastructure/iac, never as an API reference page.",
	},
	{
		Shape:    "/projects/*/merge_requests/*/notes/*/award_emoji",
		Category: categoryDocumentedInProse,
		Reason: "emoji_reactions.md gives the note reactions one code block, for issue notes, and leaves the merge " +
			"request and snippet variants to a sentence saying the same endpoints exist under those parents.",
	},
	{
		Shape:    "/projects/*/merge_requests/*/notes/*/award_emoji/*",
		Category: categoryDocumentedInProse,
		Reason:   "the same sentence in emoji_reactions.md, for one reaction rather than the list.",
	},
	{
		Shape:    "/projects/*/snippets/*/notes/*/award_emoji",
		Category: categoryDocumentedInProse,
		Reason:   "the same sentence in emoji_reactions.md, for a snippet note.",
	},
	{
		Shape:    "/projects/*/snippets/*/notes/*/award_emoji/*",
		Category: categoryDocumentedInProse,
		Reason:   "the same sentence in emoji_reactions.md, for one reaction on a snippet note.",
	},
	{
		Shape:    "/usage_data/track_events",
		Category: categoryDocumentedInProse,
		Reason: "usage_data.md documents it in a sentence and a curl example and never as an endpoint line, " +
			"unlike track_event beside it, which it spells out.",
	},
	{
		Shape:    "/orbit/...",
		Category: categoryUndocumentedAPI,
		Reason: "the Knowledge Graph API is experimental, GitLab.com only, and has no reference page; the six " +
			"gitlab_orbit_* tools are covered by the orbitlive suite against the real endpoints instead.",
	},
	{
		Shape:    "//sidekiq/...",
		Category: categoryClientDefect,
		Reason: "client-go declares the four Sidekiq routes with a leading slash (sidekiq_metrics.go), against its " +
			"own NewRequest contract, so we send /api/v4//sidekiq/queue_metrics. gitlab.com answers that with a 308 " +
			"to the collapsed path and a self-managed front end may not. The fix is upstream; the double slash is " +
			"kept visible here rather than papered over by the comparison.",
	},
}

// classifyUndocumented attaches the declaration that accounts for each
// undocumented endpoint, and names the declarations that accounted for none.
func classifyUndocumented(found []Endpoint) (classified []Endpoint, unused []string) {
	used := map[string]bool{}
	classified = make([]Endpoint, 0, len(found))
	for _, endpoint := range found {
		if declaration, ok := declarationFor(endpoint); ok {
			endpoint.Category = declaration.Category
			endpoint.Reason = declaration.Reason
			used[declaration.Shape] = true
		}
		classified = append(classified, endpoint)
	}

	unused = make([]string, 0)
	for _, declaration := range declaredUndocumentedEndpoints {
		if !used[declaration.Shape] {
			unused = append(unused, declaration.Shape)
		}
	}
	sort.Strings(unused)
	return classified, unused
}

// declarationFor finds the declaration covering one endpoint.
func declarationFor(endpoint Endpoint) (endpointDeclaration, bool) {
	for _, declaration := range declaredUndocumentedEndpoints {
		if declaration.covers(endpoint) {
			return declaration, true
		}
	}
	return endpointDeclaration{}, false
}

// covers reports whether this declaration accounts for one endpoint.
func (d endpointDeclaration) covers(endpoint Endpoint) bool {
	if len(d.Methods) > 0 && !slices.Contains(d.Methods, endpoint.Method) {
		return false
	}
	return matchesDeclaredShape(strings.Split(d.Shape, "/"), strings.Split(endpoint.Path, "/"))
}

// matchesDeclaredShape compares a declared shape with a recorded path segment
// by segment, where "*" stands for one segment and a trailing "..." for the
// rest.
func matchesDeclaredShape(shape, recorded []string) bool {
	for i, want := range shape {
		if want == declaredRest {
			return i <= len(recorded)
		}
		if i >= len(recorded) {
			return false
		}
		if want == declaredSegment || want == recorded[i] {
			continue
		}
		return false
	}
	return len(shape) == len(recorded)
}

const (
	// declaredSegment stands for one segment of a declared shape, whatever it
	// holds, since a recorded path carries whichever placeholder the fixture
	// earned and a declaration is about the endpoint rather than the fixture.
	declaredSegment = "*"

	// declaredRest stands for every remaining segment, which is how a family
	// of endpoints under one prefix is declared once.
	declaredRest = "..."
)
