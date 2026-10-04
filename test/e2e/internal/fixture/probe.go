//go:build e2e

// probe.go asks GitLab directly what it answers a credential, bypassing the
// server under test (issue 952).
//
// The fine-grained table the server carries rests on what GitLab answers a
// fine-grained token for each request: a REST route refused with the
// permission it needs, a HEAD answered from its GET's endpoint, a GraphQL
// type answered with null, a list that loses its items, a write that commits
// and answers null. A scenario holds the server to the table; a probe holds
// the table to GitLab, so a release that changes one of those answers fails
// on the probe that names it rather than as a scenario reading wrongly.
//
// A probe is a read of GitLab's answer and not a builder: it changes nothing
// a later probe could trip over except where its document is a write, and
// the scenario that sends one owns what it changed.

package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// ProbeAnswer is what GitLab answered one REST probe: its status, and for a
// refusal the body, which is where GitLab names what it refused for.
type ProbeAnswer struct {
	Status int
	Body   string
}

// GraphQLAnswer is a GraphQL answer read as GitLab sends it: the data by
// root field, and the messages of its errors.
type GraphQLAnswer struct {
	Data   map[string]json.RawMessage
	Errors []string
}

// ProbeREST sends one REST request as the token, through a client built for
// it, and answers GitLab's status and, for a refusal, its body. The query is
// encoded for a request without a body and sent as the body otherwise, as
// client-go does for every call.
func ProbeREST(e *harness.Env, token Token, method, path string, query any) ProbeAnswer {
	e.T.Helper()
	client, err := e.ClientFor(token.Value)
	if err != nil {
		e.T.Fatalf("building a client for the probe: %v", err)
		return ProbeAnswer{}
	}
	answer, err := probeREST(e.Ctx, client, method, path, query)
	if err != nil {
		e.T.Fatalf("probing %s %s: %v", method, path, err)
	}
	return answer
}

// probeREST is ProbeREST's half that takes a client.
func probeREST(ctx context.Context, client *gitlabclient.Client, method, path string, query any) (ProbeAnswer, error) {
	req, err := client.GL().NewRequest(method, path, query, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return ProbeAnswer{}, fmt.Errorf("building the request: %w", err)
	}
	resp, err := client.GL().Do(req, nil)
	if resp == nil {
		return ProbeAnswer{}, fmt.Errorf("no answer: %w", err)
	}
	answer := ProbeAnswer{Status: resp.StatusCode}
	if refusal, isRefusal := errors.AsType[*gl.ErrorResponse](err); isRefusal {
		answer.Body = string(refusal.Body)
	}
	return answer, nil
}

// ProbeGraphQL posts one document as the token, or as the run's own token
// when token is the zero Token, which is how a probe shows what the classic
// credential is answered beside the fine-grained one.
func ProbeGraphQL(e *harness.Env, token Token, document string, variables map[string]any) GraphQLAnswer {
	e.T.Helper()
	client := e.Client()
	if token.Value != "" {
		var err error
		if client, err = e.ClientFor(token.Value); err != nil {
			e.T.Fatalf("building a client for the probe: %v", err)
			return GraphQLAnswer{}
		}
	}
	answer, err := probeGraphQL(e.Ctx, client, document, variables)
	if err != nil {
		e.T.Fatalf("the GraphQL probe failed: %v", err)
	}
	return answer
}

// probeGraphQL is ProbeGraphQL's half that takes a client. Unlike the
// builders' runner it keeps the errors as an answer rather than a failure,
// since an error GitLab gives a fine-grained token is often what a probe asks
// about.
func probeGraphQL(ctx context.Context, client *gitlabclient.Client, document string, variables map[string]any) (GraphQLAnswer, error) {
	var envelope struct {
		Data   map[string]json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if _, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: document, Variables: variables}, &envelope, gl.WithContext(ctx)); err != nil {
		return GraphQLAnswer{}, err
	}
	answer := GraphQLAnswer{Data: envelope.Data}
	for _, refusal := range envelope.Errors {
		answer.Errors = append(answer.Errors, refusal.Message)
	}
	return answer, nil
}
