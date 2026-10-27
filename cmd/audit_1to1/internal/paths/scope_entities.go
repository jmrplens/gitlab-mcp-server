// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"fmt"
	"slices"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
)

// GitLab's search routes are annotated with no response entity, because the
// entity is chosen by the scope a caller sends: lib/api/search.rb disables
// the API/DescriptionSuccessResponse cop for the file ("search api responds
// with multiple entity types"), maps each scope to its entity in
// SCOPE_ENTITY, and presents the results with SCOPE_ENTITY[params[:scope]].
// The live record therefore says nothing about what any search answers, and
// every output type built from a search answer read as one the record gives no
// schema for, which is how the code search result came to drop the blob id
// GitLab sends and the wiki search to publish a wiki page nothing sends.
//
// A client-go search method sends one scope, so the method names the entity.
// scopedEntities says which, method by method, and the type grain holds a type
// built from a method's answer to that entity on every route the method
// reaches. The scope itself is not read from client-go: the table carries it so
// a reader can hold each entry to SCOPE_ENTITY, and
// [scopeDeclarationProblems] holds each to the record and to the methods
// client-go has.

// scopedEntity is the entity one client-go method's answer is presented with
// on a route GitLab annotates with none.
type scopedEntity struct {
	// Method is the client-go method, "Service.Method".
	Method string
	// Scope is the scope the method sends.
	Scope string
	// Entity is the entity SCOPE_ENTITY maps that scope to, as the record
	// names it.
	Entity string
	// Misdecoded marks a method whose answer client-go decodes into a struct
	// that is not that entity. Its routes are not credited to that struct,
	// which would hold every type modeling the struct to an answer it never
	// decodes, and the output type read off the captured answer instead is
	// paired with the method in capturedPairings.
	Misdecoded bool
}

// The entities a search scope selects that no other declaration here names.
// The rest (CommitDetail, IssueBasic, MergeRequestBasic, UserBasic) are the
// constants the sent declarations already spell.
const (
	blobEntity                = "API::Entities::Blob"
	basicProjectDetailsEntity = "API::Entities::BasicProjectDetails"
	milestoneEntity           = "API::Entities::Milestone"
	noteEntity                = "API::Entities::Note"
	snippetEntity             = "API::Entities::Snippet"
)

// scopedEntities is SCOPE_ENTITY (lib/api/search.rb) for every search method
// client-go has. groups and work_items are scopes client-go sends no method
// for.
var scopedEntities = slices.Concat(
	scopeMethods("blobs", blobEntity, false, "Search.Blobs", "Search.BlobsByGroup", "Search.BlobsByProject"),
	scopeMethods("commits", commitDetailEntity, false, "Search.Commits", "Search.CommitsByGroup", "Search.CommitsByProject"),
	scopeMethods("issues", issueBasicEntity, false, "Search.Issues", "Search.IssuesByGroup", "Search.IssuesByProject"),
	scopeMethods("merge_requests", mrBasicEntity, false,
		"Search.MergeRequests", "Search.MergeRequestsByGroup", "Search.MergeRequestsByProject"),
	scopeMethods("milestones", milestoneEntity, false, "Search.Milestones", "Search.MilestonesByGroup", "Search.MilestonesByProject"),
	scopeMethods("notes", noteEntity, false, "Search.NotesByProject"),
	scopeMethods("projects", basicProjectDetailsEntity, false, "Search.Projects", "Search.ProjectsByGroup"),
	scopeMethods("snippet_titles", snippetEntity, false, "Search.SnippetTitles"),
	scopeMethods("users", userBasicEntity, false, "Search.Users", "Search.UsersByGroup", "Search.UsersByProject"),
	// client-go decodes the wiki_blobs scope into its Wiki page struct, none
	// of whose keys GitLab sends (docs/development/upstream-bugs.md row 130).
	scopeMethods("wiki_blobs", blobEntity, true, "Search.WikiBlobs", "Search.WikiBlobsByGroup", "Search.WikiBlobsByProject"),
)

// scopeMethods declares the methods that send one scope, each with the entity
// the scope selects.
func scopeMethods(scope, entity string, misdecoded bool, methods ...string) []scopedEntity {
	declarations := make([]scopedEntity, 0, len(methods))
	for _, method := range methods {
		declarations = append(declarations, scopedEntity{Method: method, Scope: scope, Entity: entity, Misdecoded: misdecoded})
	}
	return declarations
}

// capturedPairing pairs an output type read off the captured response
// (ADR-0021) with the client-go methods whose answer it reads. No converter
// fills such a type, so nothing else can say which endpoints it models.
type capturedPairing struct {
	// Package is the internal/tools domain name.
	Package string
	// Type is the output struct's Go name.
	Type string
	// Methods are the client-go methods, "Service.Method", whose answer the
	// type is decoded from.
	Methods []string
}

// capturedPairings lists the captured types the type grain judges.
var capturedPairings = []capturedPairing{
	{Package: "search", Type: "WikiBlobOutput", Methods: []string{"Search.WikiBlobs", "Search.WikiBlobsByGroup", "Search.WikiBlobsByProject"}},
}

// capturedSDKType is what a captured type's findings name in place of the
// client-go struct a converter pairing would name: there is none, and every key
// the type is held to is one no client-go struct models for it.
const capturedSDKType = "captured response"

// scopedEntityOf returns the entity declared for a client-go method, and ""
// for a method no declaration names.
func scopedEntityOf(method string) string {
	for _, declaration := range scopedEntities {
		if declaration.Method == method {
			return declaration.Entity
		}
	}
	return ""
}

// misdecoded reports whether a client-go method is declared to decode its
// answer into a struct that is not the entity GitLab presents.
func misdecoded(method string) bool {
	for _, declaration := range scopedEntities {
		if declaration.Method == method {
			return declaration.Misdecoded
		}
	}
	return false
}

// withCaptured adds every captured pairing to the projections a type is judged
// through, in the shape a projection has: the methods whose routes are
// searched, and capturedSDKType in place of a struct, which no SDK field index
// holds. A type a converter pairs keeps its converter, as it does over a
// projection.
func withCaptured(projected map[[2]string][]structs.ProjectionPairing, sdkTypes map[[2]string][]string) map[[2]string][]structs.ProjectionPairing {
	for _, pairing := range capturedPairings {
		key := [2]string{pairing.Package, pairing.Type}
		if len(sdkTypes[key]) > 0 {
			continue
		}
		projected[key] = []structs.ProjectionPairing{{Package: pairing.Package, MCPType: pairing.Type, SDKType: capturedSDKType, Methods: pairing.Methods}}
	}
	return projected
}

// isCaptured reports whether a type's projections are a captured pairing.
func isCaptured(projections []structs.ProjectionPairing) bool {
	return len(projections) > 0 && projections[0].SDKType == capturedSDKType
}

// scopeDeclarationProblems holds both tables to the tree, as every declaration
// table here is held: a scope declaration must name a method client-go has a
// route for, an entity the record holds, and routes the record mounts with no
// entity of their own and a scope parameter; a captured pairing must name a
// type the package still publishes and methods client-go has a route for.
// Each problem is a sentence, and the list is sorted.
func scopeDeclarationProblems(scoped []scopedEntity, captured []capturedPairing, methodRoutes map[string][]sdkRoute, index *operationIndex, published []publishedType) []string {
	var problems []string
	for _, declaration := range scoped {
		problems = append(problems, scopeProblems(declaration, methodRoutes, index)...)
	}
	publishedNames := make(map[[2]string]bool, len(published))
	for _, candidate := range published {
		publishedNames[[2]string{shortPackage(candidate.Package), candidate.Name}] = true
	}
	for _, pairing := range captured {
		named := pairing.Package + "." + pairing.Type
		if !publishedNames[[2]string{pairing.Package, pairing.Type}] {
			problems = append(problems, fmt.Sprintf("captured pairing %s names a type the package no longer publishes", named))
		}
		for _, method := range pairing.Methods {
			if len(methodRoutes[method]) == 0 {
				problems = append(problems, fmt.Sprintf("captured pairing %s names %s, which client-go has no route for", named, method))
			}
		}
	}
	sort.Strings(problems)
	return slices.Compact(problems)
}

// scopeProblems is what no longer holds of one scope declaration.
func scopeProblems(declaration scopedEntity, methodRoutes map[string][]sdkRoute, index *operationIndex) []string {
	prefix := "scope declaration " + declaration.Method
	routes := methodRoutes[declaration.Method]
	if len(routes) == 0 {
		return []string{prefix + " names a method client-go has no route for"}
	}
	var problems []string
	if _, ok := index.record.Fields(declaration.Entity); !ok {
		problems = append(problems, fmt.Sprintf("%s names %s, which the record holds no entity for", prefix, declaration.Entity))
	}
	// A sequence of ifs rather than a switch, so the condition coverage tool
	// can measure each condition; it cannot see a case expression.
	for _, route := range routes {
		spelled := route.Method + " " + route.Path
		found, quality, _ := index.lookup(route.Method, route.Path)
		if quality != matchExact {
			problems = append(problems, fmt.Sprintf("%s reaches %s, which the record does not mount", prefix, spelled))
			continue
		}
		if found.Entity != "" {
			problems = append(problems, fmt.Sprintf("%s reaches %s, which the record now annotates with %s", prefix, spelled, found.Entity))
			continue
		}
		if !hasParam(found, "scope") {
			problems = append(problems, fmt.Sprintf("%s reaches %s, which takes no scope", prefix, spelled))
		}
	}
	return problems
}

// hasParam reports whether an operation declares a parameter.
func hasParam(found operation, name string) bool {
	_, ok := found.Params[name]
	return ok
}
