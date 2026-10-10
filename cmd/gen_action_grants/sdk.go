package main

import (
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
)

// sdkSource answers the derivation's question of client-go from one reading
// of its root package and the documents it builds.
type sdkSource struct {
	sdk       *sdkroutes.SDK
	documents []graphqldocs.Document
}

// Requests reads what one client-go method sends: each of its routes as an
// alternative, a legacy request it cannot fold as an unresolved alternative,
// and every document it posts, one graphqldocs rendered from its shell as the
// rendering, and one still a shell (a text/template or a format string whose
// text depends on what the handler hands the method) as an unresolved request
// a declaration answers.
func (s *sdkSource) Requests(key string) (routes, documents []derive.Request, known bool) {
	method, ok := s.sdk.Method(key)
	if !ok {
		return nil, nil, false
	}
	for _, route := range method.Routes {
		routes = append(routes, derive.Request{Kind: derive.KindREST, Method: route.Method, Path: route.Path})
	}
	if len(method.Unresolved) > 0 {
		routes = append(routes, derive.Request{Kind: derive.KindUnresolved, Reason: "sdk-path " + key})
	}
	if !method.GraphQL {
		return routes, nil, true
	}
	for _, document := range postingOrder(s.sdk.Documents(key, s.documents)) {
		if graphqldocs.IsTemplate(document) {
			documents = append(documents, derive.Request{Kind: derive.KindUnresolved, Reason: templateClass(document) + " " + key})
			continue
		}
		documents = append(documents, derive.Request{Kind: derive.KindGraphQL, Document: document.Text, Name: document.Name})
	}
	return routes, documents, true
}

// postingOrder puts a client-go method's documents in the order it posts
// them, which the reading of client-go does not keep (it lists them in the
// order they are declared): every query before every mutation, each kind in
// the order it came. A client-go method that posts both looks up what the
// write needs first; at v3.15.0 those are WorkItems.UpdateWorkItem and
// DeleteWorkItem, each of which reads the item's global ID with
// getWorkItemIDQuery and then writes. The order decides which refusal a caller
// meets: a fine-grained token that cannot pass the lookup stops the action
// before the write is sent, so nothing commits.
func postingOrder(documents []graphqldocs.Document) []graphqldocs.Document {
	var queries, mutations []graphqldocs.Document
	for _, document := range documents {
		if graphqldocs.DefinesMutation(document.Text) {
			mutations = append(mutations, document)
			continue
		}
		queries = append(queries, document)
	}
	return append(queries, mutations...)
}

// templateClass names how a client-go document is assembled at run time: a
// text/template shell, or a printf format string.
func templateClass(document graphqldocs.Document) string {
	if strings.Contains(document.Text, "{{") {
		return "sdk-graphql-template"
	}
	return "sdk-graphql-format"
}

// clientGoDir finds the directory of client-go's root package among the
// imports of the loaded packages, which is the module the handlers compile
// against, so the reading is of that version and no other.
func clientGoDir(pkgs []*packages.Package) string {
	for _, pkg := range pkgs {
		if imported, ok := pkg.Imports[actionrequests.ClientGoPath]; ok && len(imported.GoFiles) > 0 {
			return filepath.Dir(imported.GoFiles[0])
		}
	}
	return ""
}
