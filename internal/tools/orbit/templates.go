// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// orbitQueryTemplatesOp is the operation a [QueryTemplates] failure is
// reported under.
const orbitQueryTemplatesOp = "orbit_query_templates"

// queryQueryTemplates asks for the Orbit query templates. queryTemplates takes
// no arguments and can be resolved once per request, which this document does.
const queryQueryTemplates = `
query OrbitQueryTemplates {
  orbit {
    queryTemplates {
      name
      description
      query
    }
  }
}
`

// QueryTemplatesInput takes nothing: GitLab answers every template it renders
// for the user.
type QueryTemplatesInput struct{}

// QueryTemplate is one Orbit query template.
type QueryTemplate struct {
	// Name is the template's name, which orbit.named_query runs.
	Name string `json:"name"`
	// Description says what the template asks.
	Description string `json:"description,omitempty"`
	// Query is the template's GQL text, rendered for the user who asked: an
	// identity value such as the user's own id is bound in it.
	Query string `json:"query"`
}

// QueryTemplatesOutput is the list of Orbit query templates rendered for the
// user who asked.
type QueryTemplatesOutput struct {
	toolutil.HintableOutput
	// Templates are the query templates.
	Templates []QueryTemplate `json:"templates"`
}

// gqlOrbitQueryTemplates is the orbit object of an answer to
// [queryQueryTemplates], with queryTemplates null when the user cannot use
// Orbit.
type gqlOrbitQueryTemplates struct {
	QueryTemplates *[]gqlQueryTemplate `json:"queryTemplates"`
}

// gqlQueryTemplate is one OrbitQueryTemplate as [queryQueryTemplates] selects
// it.
type gqlQueryTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Query       string `json:"query"`
}

// QueryTemplates retrieves the Orbit query templates, rendered for the user
// the token belongs to.
//
// Source: GitLab's GraphQL API, Query.orbit.queryTemplates, which replaced GET
// /api/v4/orbit/query/templates when gitlab-org/gitlab!265633 removed the route
// on 2026-10-27. Each template's query is GQL rendered for the user who asked
// (my_neighbors binds the user's own id), so the answer belongs to that user
// and is never cached or shown for another. The list leaves out every named
// query that takes parameters; the description of orbit.named_query's
// parameters lists those.
// GitLab answers queryTemplates null when the user cannot use Orbit, which is
// reported as an error naming what Orbit needs, not as an empty list.
func QueryTemplates(ctx context.Context, client *gitlabclient.Client, _ QueryTemplatesInput) (QueryTemplatesOutput, error) {
	if err := ctx.Err(); err != nil {
		return QueryTemplatesOutput{}, err
	}
	var resp struct {
		Data struct {
			Orbit *gqlOrbitQueryTemplates `json:"orbit"`
		} `json:"data"`
		Errors []toolutil.GraphQLError `json:"errors"`
	}
	if _, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: queryQueryTemplates}, &resp, gl.WithContext(ctx)); err != nil {
		return QueryTemplatesOutput{}, wrapOrbitErr(orbitQueryTemplatesOp, err)
	}
	if err := toolutil.GraphQLTopLevelError(orbitQueryTemplatesOp, resp.Errors); err != nil {
		return QueryTemplatesOutput{}, err
	}
	if resp.Data.Orbit == nil || resp.Data.Orbit.QueryTemplates == nil {
		return QueryTemplatesOutput{}, errOrbitUnavailable(orbitQueryTemplatesOp, "the Orbit query templates", orbitUnavailableHint)
	}
	out := QueryTemplatesOutput{Templates: make([]QueryTemplate, 0, len(*resp.Data.Orbit.QueryTemplates))}
	for _, template := range *resp.Data.Orbit.QueryTemplates {
		out.Templates = append(out.Templates, QueryTemplate(template))
	}
	return out, nil
}
