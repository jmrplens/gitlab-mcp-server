// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// oauthDeclaration is a REST route GitLab answers only for a personal access
// token (or a job token), refusing an OAuth access token whatever scopes it
// carries, with the source lines that refuse it.
type oauthDeclaration struct {
	// Route is the route as the table names its operation
	// ("DELETE /personal_access_tokens/self").
	Route  string
	Reason string
}

// oauthRefusedRoutes are the routes of the table GitLab 19.5.0 refuses to an
// OAuth access token, from a sweep of its API for every check of the calling
// token's type. The generator marks each one's operation
// ([finegrained.Operation.OAuthRefused]), the server withholds an action every
// way of which sends one from a session it knows to be authenticated by an
// OAuth token (issue 1238, register row AUT-009), and the tool reference names
// them on the action's token lines. An entry naming a route the table holds no
// operation for is a finding, so an entry cannot outlive the last action that
// sends its route.
//
// GET /personal_access_tokens/self/associations is refused by the same class,
// and is not declared here: no action sends it yet (issue 1613), and the
// layer that starts sending it declares it.
var oauthRefusedRoutes = []oauthDeclaration{
	{
		Route:  "GET /personal_access_tokens/self",
		Reason: "lib/api/personal_access_tokens/self_information.rb:19-23 at v19.5.0-ee answers 400 to a token that is not a personal access token, for every route of the class",
	},
	{
		Route:  "DELETE /personal_access_tokens/self",
		Reason: "lib/api/personal_access_tokens/self_information.rb:19-23 at v19.5.0-ee answers 400 to a token that is not a personal access token, for every route of the class",
	},
	{
		Route:  "POST /personal_access_tokens/self/rotate",
		Reason: "lib/api/personal_access_tokens/self_rotation.rb:40 at v19.5.0-ee answers 405 to a token that is not a personal access token",
	},
	{
		Route:  "POST /groups/:id/access_tokens/self/rotate",
		Reason: "lib/api/resource_access_tokens/self_rotation.rb:43-44 at v19.5.0-ee answers 405 to a token that is not a personal access token, or not a bot's",
	},
	{
		Route:  "POST /projects/:id/access_tokens/self/rotate",
		Reason: "lib/api/resource_access_tokens/self_rotation.rb:43-44 at v19.5.0-ee answers 405 to a token that is not a personal access token, or not a bot's",
	},
	{
		Route:  "GET /projects/:id/packages/ml_models/:model_version_id/files/(*path/):file_name",
		Reason: "lib/api/ml_model_packages.rb:33-36 at v19.5.0-ee authenticates the class with a personal access token or a job token sent as the Bearer token and no other, so an OAuth access token is answered 401",
	},
}

// markOAuthRefused sets [finegrained.Operation.OAuthRefused] on the table's
// operation for each declared route, and returns a finding for each
// declaration whose route the table holds no operation for: the table holds a
// route only while some action sends it.
func markOAuthRefused(table *finegrained.Table, declarations []oauthDeclaration) []string {
	index := make(map[string]int, len(table.Operations))
	for i, op := range table.Operations {
		index[op.Name] = i
	}
	var findings []string
	for _, declaration := range declarations {
		i, held := index[declaration.Route]
		if !held {
			findings = append(findings, fmt.Sprintf("the OAuth refusal declared for %s names no operation of the table; remove it, or derive the action that sends it", declaration.Route))
			continue
		}
		table.Operations[i].OAuthRefused = true
	}
	return findings
}
