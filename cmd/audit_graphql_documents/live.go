package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/graphqlintrospect"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// liveSchema introspects endpoint right now and returns the schema it serves,
// with one line naming what answered.
//
// This is the check the pin cannot perform. The pin says our documents were
// valid on gitlab.com on the day it was taken; this says they are valid on the
// GitLab that shipped today, which is the version a self-managed instance
// actually runs. It reuses cmd/gen_graphql_schema's introspection and
// conversion rather than a second copy, because an instance's answer has
// quirks (the canned payload, the deprecation arguments an older release
// refuses) that must be understood the same way on both sides or the two
// commands stop speaking about the same schema.
func liveSchema(ctx context.Context, endpoint, token string) (*ast.Schema, string, error) {
	target := graphqlintrospect.Target{
		Endpoint: endpoint,
		Token:    token,
		Client:   &http.Client{Timeout: graphqlintrospect.FetchTimeout},
	}

	introspected, err := graphqlintrospect.Introspect(ctx, target)
	if err != nil {
		return nil, "", err
	}

	// An instance that boots but answers with a fragment of its schema is the
	// failure this whole command must not have: every document would validate
	// against the little that arrived and the run would report success for a
	// question nobody asked. The floor is the one cmd/gen_graphql_schema
	// refuses a truncated pin with, so both sides stop believing an answer at
	// the same count.
	if graphqlintrospect.TruncatedAnswer(len(introspected.Types)) {
		return nil, "", fmt.Errorf(
			"%s answered with %d types and a GitLab schema carries more than %d: the introspection was truncated, or that instance is not the GitLab this server targets",
			endpoint, len(introspected.Types), graphqlintrospect.MinimumTypes,
		)
	}

	// The version is provenance rather than a judgement, so an instance that
	// will not name itself, which is every instance asked without a token, is
	// reported as unknown and judges the documents anyway.
	version, _ := graphqlintrospect.InstanceVersion(ctx, target)

	schema, err := graphqlschema.Load([]byte(graphqlintrospect.RenderSDL(introspected)))
	if err != nil {
		return nil, "", fmt.Errorf("%s: the converted schema does not parse: %w", endpoint, err)
	}
	return schema, fmt.Sprintf("%d types from %s (GitLab %s), fetched now, not the pinned schema",
		len(introspected.Types), endpoint, version), nil
}
