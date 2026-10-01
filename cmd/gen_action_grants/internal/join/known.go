package join

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// Known reports whether the element a denial names is one the record holds,
// of the kind its cause says it is: a route for a REST cause, a mutation for
// an undeclared mutation, an object, union or interface type for a position,
// and either a type or a mutation for a boundary that never resolves, which a
// declaration may say of a mutation's own check. A denial naming something
// the record does not hold was decided by nothing GitLab declared, which is
// what gate 2 refuses.
func Known(record *apilive.Document, denial *finegrained.Denial) bool {
	if !denial.Cause.GraphQL() {
		for i := range record.Routes {
			if liveName(&record.Routes[i]) == denial.Element {
				return true
			}
		}
		return false
	}
	authz := record.GraphQLAuthz
	if authz == nil {
		return false
	}
	_, mutation := authz.Mutations[denial.Element]
	if denial.Cause == finegrained.CauseMutationUndeclared {
		return mutation
	}
	if denial.Cause == finegrained.CauseBoundaryUnresolvable && mutation {
		return true
	}
	if _, ok := authz.Types[denial.Element]; ok {
		return true
	}
	_, ok := authz.Abstract[denial.Element]
	return ok
}
