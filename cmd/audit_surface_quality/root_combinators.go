// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The rule that no served tool input schema carries a combinator or a
// conditional at its root.

package main

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// rootCombinatorCategory is the category of a tool whose served input schema
// carries a combinator or a conditional at its root.
const rootCombinatorCategory = "input-schema-root"

// surfaceListing is one served listing the root-combinator rule reads, with
// the label its violations are reported under.
type surfaceListing struct {
	kind  string
	tools []*mcp.Tool
}

// listOtherSurfaces lists what the root-combinator rule reads beyond the two
// listings every metadata rule reads: the meta surface under the compact and
// full parameter-schema modes, which serve each action's params inline, on a
// self-managed instance and on GitLab.com, the dynamic pair, and the
// individual and default meta surfaces of GitLab.com. GitLab.com alone serves
// the Orbit tools, so its meta surface is read under every mode. Each comes
// from [mcpsurface], so the rule judges what a client is served.
//
// It is a variable so a test can hand the rule listings that carry a
// violation, which the served surfaces, carrying none, cannot.
var listOtherSurfaces = func(client *gitlabclient.Client) []surfaceListing {
	dotcom := mcpsurface.NewGitLabComClient()
	var listings []surfaceListing
	for _, mode := range []string{config.MetaParamSchemaCompact, config.MetaParamSchemaFull} {
		restore := tools.SetMetaParamSchemaScoped(mode)
		listings = append(listings,
			surfaceListing{kind: "meta " + mode, tools: mcpsurface.MetaTools(client, edition.Ultimate)},
			surfaceListing{kind: "meta " + mode + " gitlab.com", tools: mcpsurface.MetaTools(dotcom, edition.Ultimate)},
		)
		restore()
	}
	return append(listings,
		surfaceListing{kind: "dynamic", tools: mcpsurface.DynamicTools(client)},
		surfaceListing{kind: "individual gitlab.com", tools: mcpsurface.IndividualTools(dotcom, edition.Ultimate)},
		surfaceListing{kind: "meta gitlab.com", tools: mcpsurface.MetaTools(dotcom, edition.Ultimate)},
	)
}

// auditRootCombinators flags every tool whose served input schema carries at
// its root one of the combinators or conditionals no tool may serve there
// ([toolutil.RootCombinators]). The Anthropic Messages API refuses a tool
// whose input_schema has oneOf, allOf or anyOf at the top level, and fails
// the whole request over it, so every request that carries one such tool
// fails, from any client that sends the tools it lists to that API.
func auditRootCombinators(tls []*mcp.Tool, kind string) []violation {
	var vs []violation
	for _, t := range tls {
		if found := toolutil.RootCombinators(t.InputSchema); len(found) > 0 {
			vs = append(vs, violation{
				t.Name, rootCombinatorCategory,
				fmt.Sprintf("%s tool inputSchema carries %s at its root, which the Anthropic Messages API refuses for the whole tool list", kind, strings.Join(found, ", ")),
			})
		}
	}
	return vs
}

// auditOtherSurfacesRootCombinators applies [auditRootCombinators] to every
// listing [listOtherSurfaces] returns, in its order.
func auditOtherSurfacesRootCombinators(client *gitlabclient.Client) []violation {
	var vs []violation
	for _, listing := range listOtherSurfaces(client) {
		vs = append(vs, auditRootCombinators(listing.tools, listing.kind)...)
	}
	return vs
}
