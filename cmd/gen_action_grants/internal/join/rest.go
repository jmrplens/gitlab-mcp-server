package join

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// requirement is one group before it is indexed: raw permission names held at
// one boundary type among any.
type requirement struct {
	perms []string
	any   finegrained.Boundary
}

// key identifies a requirement among the table's.
func (r requirement) key() string {
	return strings.Join(r.perms, ",") + "@" + r.any.String()
}

// Where a derived route is found among the record's and what the table calls
// it once found are apilive's to answer ([apilive.RouteIndex] and
// [apilive.RouteName]), since R-GRANT places the routes the unit suite
// recorded by the same rule and the two must agree on which route a request
// is.

// restRequirements reads what a route demands: its groups, whether the grant
// decides it at all, and why no fine-grained token passes it when none does.
// The reading is apilive's ([apilive.Route.Requirements]), because R-GRANT
// holds each REST operation of the committed table to the record by the same
// reading, and the two must agree on what a route demands.
func restRequirements(route *apilive.Route) (groups []requirement, skip bool, denied finegrained.Cause) {
	read, skip, denied := route.Requirements()
	for _, group := range read {
		groups = append(groups, requirement{perms: group.Permissions, any: group.Any})
	}
	return groups, skip, denied
}

// boundaryOf reads one boundary type, nothing for an empty or unknown one.
func boundaryOf(name string) finegrained.Boundary {
	boundary, _ := finegrained.ParseBoundary(name)
	return boundary
}

// sorted returns a sorted copy of names, which is the order GitLab compares a
// GraphQL requirement group's permissions in and the one a table keys by.
func sorted(names []string) []string {
	out := slices.Clone(names)
	slices.Sort(out)
	return out
}
