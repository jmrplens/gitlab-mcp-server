package main

import (
	"fmt"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// oauthRefusedRoute is a REST route GitLab answers only for an access token,
// refusing an OAuth token whatever scopes it carries, with the source lines
// that refuse it.
type oauthRefusedRoute struct {
	// Route is the route as the action grants table names its operation
	// ("DELETE /personal_access_tokens/self").
	Route  string
	Reason string
}

// oauthRefusedRoutes are the routes of the action grants table GitLab 19.4.1
// refuses to an OAuth token, from a sweep of its API for every check of the
// calling token's type. Each action's classic or OAuth line names the ones it
// sends, since the scope a line names is no use to an OAuth token there. A
// route the table holds no operation for stops generation, so an entry cannot
// outlive the last action that sends it.
var oauthRefusedRoutes = []oauthRefusedRoute{
	{
		Route:  "GET /personal_access_tokens/self",
		Reason: "lib/api/personal_access_tokens/self_information.rb:19-23 at v19.4.1-ee answers 400 to a token that is not a personal access token",
	},
	{
		Route:  "DELETE /personal_access_tokens/self",
		Reason: "lib/api/personal_access_tokens/self_information.rb:19-23 at v19.4.1-ee answers 400 to a token that is not a personal access token",
	},
	{
		Route:  "POST /personal_access_tokens/self/rotate",
		Reason: "lib/api/personal_access_tokens/self_rotation.rb:37 at v19.4.1-ee answers 405 to a token that is not a personal access token",
	},
	{
		Route:  "POST /groups/:id/access_tokens/self/rotate",
		Reason: "lib/api/resource_access_tokens/self_rotation.rb:40-41 at v19.4.1-ee answers 405 to a token that is not a personal access token, or not a bot's",
	},
	{
		Route:  "POST /projects/:id/access_tokens/self/rotate",
		Reason: "lib/api/resource_access_tokens/self_rotation.rb:40-41 at v19.4.1-ee answers 405 to a token that is not a personal access token, or not a bot's",
	},
}

// oauthRefusedSet is the declared routes as a set, refusing a declaration
// whose route the action grants table holds no operation for: the table holds
// a route only while some action sends it.
func oauthRefusedSet(grants *finegrained.Table, routes []oauthRefusedRoute) (map[string]bool, error) {
	held := make(map[string]bool, len(grants.Operations))
	for _, op := range grants.Operations {
		held[op.Name] = true
	}
	set := make(map[string]bool, len(routes))
	for _, route := range routes {
		if !held[route.Route] {
			return nil, fmt.Errorf("the OAuth refusal declared for %s names no operation of the action grants table; remove it, or run make gen-action-grants", route.Route)
		}
		set[route.Route] = true
	}
	return set, nil
}

// oauthRefusal names the routes of an action's ways GitLab refuses to an
// OAuth token, sorted and each once, and reports whether every way the action
// can run sends one, which is when no input lets an OAuth token run it.
func oauthRefusal(grants *finegrained.Table, row *finegrained.Requirement, refused map[string]bool) (routes []string, everyWay bool) {
	everyWay = len(row.Paths) > 0
	for _, path := range row.Paths {
		sends := false
		for _, op := range path {
			if name := grants.Operations[op].Name; refused[name] {
				sends = true
				routes = append(routes, name)
			}
		}
		everyWay = everyWay && sends
	}
	slices.Sort(routes)
	return slices.Compact(routes), everyWay
}
