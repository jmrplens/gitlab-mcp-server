// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// orbitNamedQueryOp is the operation a [NamedQuery] failure is reported under.
const orbitNamedQueryOp = "orbit_named_query"

// namedQueryPath is where GitLab runs a named query, below /api/v4; the name
// is the one segment after it. client-go has no method for the route.
const namedQueryPath = "orbit/query"

// namedQueryPattern is how every one of the seventeen named queries GitLab.com
// listed on 2026-10-29 is spelled, checked so a name is always one path
// segment and a typo is refused before the request.
var namedQueryPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// NamedQueryInput names the query, its parameters and the format.
type NamedQueryInput struct {
	// Name is the named query to run: a template orbit.query_templates lists,
	// or one of the queries that take parameters.
	Name string `json:"name" jsonschema:"Named query to run: a template orbit.query_templates lists (recent_merges) or a query that takes parameters (context_for_job),required"`
	// Parameters are the named query's parameters, by the name GitLab gives
	// each; a template takes none. The description lists the others' as the
	// guidance does ([namedQueryGuidance]), since the individual surface serves
	// the description alone. See [namedQueryParameters] for the ids in them.
	Parameters map[string]any `json:"parameters,omitempty" jsonschema:"The named query's parameters by name, an object. A template takes none (empty or omitted). The others: context_for_issue, context_for_job, context_for_review, expand_merge_request and expand_work_item take node_ids (a list of ids) and limit, both required; expand_neighbors takes entity, node_ids and limit; definition_callees and definition_references take node_id, a string; file_definitions and file_definition_callers take project_id and file_path; list_nodes takes entity; search_nodes takes entity, field and text. Give an id of 2^53 or beyond, such as a definition's or a file's, as a decimal string: as a number it is refused."`
	// ResponseFormat is raw, llm or gql, as orbit.query takes it.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Response format: raw (structured JSON, the default), llm (compact text) or gql (a graph pattern table). json is read as raw."`
}

// namedQueryAnswer is the raw answer of a named query, the envelope POST
// /orbit/query answers with.
type namedQueryAnswer struct {
	Result          json.RawMessage `json:"result"`
	QueryType       string          `json:"query_type"`
	RawQueryStrings []string        `json:"raw_query_strings"`
	RowCount        int64           `json:"row_count"`
}

// NamedQuery runs one of GitLab's named Orbit queries and returns its answer,
// the envelope orbit.query returns.
//
// Source: POST /api/v4/orbit/query/:name, which client-go has no method for.
// GitLab takes a JSON body alone, {"parameters": {...}, "response_format":
// ...}, and answers form encoding 400 (measured 2026-10-29). The parameters
// are sent as an empty object when the caller gives none, which GitLab
// answered for a template, and with every graph id in them exact or not at all
// ([namedQueryParameters]). A refusal is 400 invalid_request with GitLab's
// account of the fault: an unknown name lists the named queries, and an
// unknown, missing or mistyped parameter names the valid ones.
func NamedQuery(ctx context.Context, client *gitlabclient.Client, input NamedQueryInput) (QueryOutput, error) {
	if err := ctx.Err(); err != nil {
		return QueryOutput{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return QueryOutput{}, toolutil.ErrRequiredString(orbitNamedQueryOp, "name")
	}
	if !namedQueryPattern.MatchString(name) {
		return QueryOutput{}, errNamedQueryName
	}
	format, err := queryResponseFormat(input.ResponseFormat)
	if err != nil {
		return QueryOutput{}, err
	}
	parameters, err := namedQueryParameters(input.Parameters)
	if err != nil {
		return QueryOutput{}, err
	}
	body := map[string]any{"parameters": parameters, "response_format": string(*format)}
	path := namedQueryPath + "/" + toolutil.EscapePathSegment(name)
	if *format != gl.OrbitResponseFormatRaw {
		var text bytes.Buffer
		if err = restRequest(ctx, client, http.MethodPost, path, body, &text); err != nil {
			return QueryOutput{}, wrapNamedQueryErr(err)
		}
		return QueryOutput{FormattedText: text.String()}, nil
	}
	var answer namedQueryAnswer
	if err = restRequest(ctx, client, http.MethodPost, path, body, &answer); err != nil {
		return QueryOutput{}, wrapNamedQueryErr(err)
	}
	return QueryOutput{
		Result:          decodeRaw(answer.Result),
		QueryType:       answer.QueryType,
		RawQueryStrings: answer.RawQueryStrings,
		RowCount:        answer.RowCount,
	}, nil
}

// nodeIDsParameter is the parameter whose value is a list of graph ids, which
// GitLab takes as integers alone: it refused "14831871432" in it with 400 on
// 2026-10-29.
const nodeIDsParameter = "node_ids"

// firstInexactNumber is 2^53, the first whole number a JSON number is no
// longer told from its neighbor by once it is decoded as a float64:
// 9007199254740993 decodes to it.
const firstInexactNumber = 1 << 53

// namedQueryParameters returns the parameters as they are sent. The ids of
// definitions and files have up to 63 bits (662267755689139244 in the
// 2026-10-29 capture), and a number that large was decoded as another one
// before the handler reads it, which GitLab would run the query on. So a
// number from [firstInexactNumber] on is refused, naming the parameter that
// holds it, and a decimal string in node_ids is sent as the integer it spells,
// the one form of such an id that arrives whole. A string that spells no
// integer of 64 bits is left for GitLab to refuse.
func namedQueryParameters(parameters map[string]any) (map[string]any, error) {
	sent := make(map[string]any, len(parameters))
	for _, name := range slices.Sorted(maps.Keys(parameters)) {
		if holdsInexactNumber(parameters[name]) {
			return nil, fmt.Errorf("%s: parameters.%s holds a number of 2^53 (%d) or beyond, which is read with its last digits changed and would name another id; send the id as a decimal string, \"662267755689139244\"", orbitNamedQueryOp, name, int64(firstInexactNumber))
		}
		sent[name] = parameters[name]
	}
	if ids, ok := sent[nodeIDsParameter].([]any); ok {
		sent[nodeIDsParameter] = exactNodeIDs(ids)
	}
	return sent, nil
}

// holdsInexactNumber reports whether a decoded JSON value is, or holds
// anywhere in it, a number that may have been another one in the document.
func holdsInexactNumber(value any) bool {
	switch typed := value.(type) {
	case float64:
		return math.Abs(typed) >= firstInexactNumber
	case []any:
		return slices.ContainsFunc(typed, holdsInexactNumber)
	case map[string]any:
		for _, member := range typed {
			if holdsInexactNumber(member) {
				return true
			}
		}
	}
	return false
}

// exactNodeIDs returns the ids of node_ids with each decimal string as the
// integer it spells.
func exactNodeIDs(ids []any) []any {
	exact := slices.Clone(ids)
	for i, id := range exact {
		text, isText := id.(string)
		if !isText {
			continue
		}
		if number, err := strconv.ParseInt(text, 10, 64); err == nil {
			exact[i] = number
		}
	}
	return exact
}

// errNamedQueryName is what [NamedQuery] answers for a name no named query
// is spelled like.
var errNamedQueryName = errors.New("orbit_named_query: name is lowercase letters, digits and underscores, starting with a letter, as a named query is spelled (recent_merges, context_for_job)")

// namedQueryRefusedHint follows a named query GitLab refused with 400: its
// message names what it refused, and the description of the parameters lists
// the ones each named query takes, which nothing GitLab serves lists.
const namedQueryRefusedHint = "GitLab's message names the fault: an unknown name lists the named queries, and a parameter refusal names the valid ones. orbit.query_templates lists the templates, which take no parameters, and the description of parameters lists what the others take"

// namedQueryRefusalCodes are the codes GitLab refuses a named query with, 400,
// for a fault in the request itself (doc/api/orbit.md): its name or a
// parameter, or the query it makes of them, which did not compile or
// validate. Its other codes describe the service.
var namedQueryRefusalCodes = map[string]bool{"invalid_request": true, "compile_error": true, "validation_error": true}

// wrapNamedQueryErr classifies a failed [NamedQuery]: a 400 is GitLab's
// refusal of the name or the parameters, followed by [namedQueryRefusedHint],
// and anything else is classified the way every Orbit failure is, by
// [wrapOrbitErr].
func wrapNamedQueryErr(err error) error {
	if !toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return wrapOrbitErr(orbitNamedQueryOp, err)
	}
	hint := namedQueryRefusedHint
	if refusal := namedQueryRefusal(err); refusal != "" {
		hint = "GitLab answered " + refusal + ". " + hint
	}
	return toolutil.WrapErrWithHint(orbitNamedQueryOp, err, hint)
}

// namedQueryRefusal returns the code and the message of GitLab's refusal of a
// named query, bounded as every reflected message is, or "" when err is not
// one. internal/toolutil reflects a GitLab body only in GitLab's own error
// shape, and the code-and-message shape only for the Workhorse refusal of POST
// /orbit/query, so the {"code":"invalid_request", "message":...} GitLab.com
// answered a named query's unknown name or parameters with on 2026-10-29 would
// reach the caller as a bare 400. It is read here, for this route's refusal
// alone and only in exactly that shape: the two keys, a string code naming the
// request as the fault ([namedQueryRefusalCodes]), and a string message, which
// is GitLab's account of what to correct.
func namedQueryRefusal(err error) string {
	var glErr *gl.ErrorResponse
	if !errors.As(err, &glErr) || len(glErr.Body) == 0 {
		return ""
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(glErr.Body, &body) != nil || len(body) != 2 {
		return ""
	}
	var code string
	var message *string
	if json.Unmarshal(body["code"], &code) != nil || json.Unmarshal(body["message"], &message) != nil {
		return ""
	}
	if !namedQueryRefusalCodes[code] || message == nil {
		return ""
	}
	return toolutil.BoundedGitLabMessage(code + ": " + *message)
}

// namedQueryExample is the named query orbit.named_query offers as its
// example: its parameter guidance's example binding and the one example its
// input schema carries, which the dynamic surface fills the example call it
// offers with. GitLab.com answered it 200 on 2026-10-29, with no parameters.
const namedQueryExample = "recent_merges"

// namedQueryInput is what orbit.named_query says about its parameters: its
// guidance, and an override of the name property whose one example is a
// template, which runs with nothing else given.
func namedQueryInput() orbitInput {
	return orbitInput{
		guidance: namedQueryGuidance(),
		overrides: []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("name", map[string]any{"examples": []any{namedQueryExample}}),
		},
	}
}

// namedQueryGuidance is the parameter guidance of orbit.named_query. The
// templates GitLab lists take no parameters; the twelve below take the ones
// GitLab.com named for each on 2026-10-29, when an unknown parameter made each
// list its own, and nothing GitLab serves lists them.
func namedQueryGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		"name": {
			SemanticRole: "named_query",
			ValueSource:  "A template name from orbit.query_templates, or one of the named queries that take parameters, which the description of parameters lists.",
			CommonConfusions: []string{
				"A template's name runs the template's text with nothing to fill in. To run a query of your own, use orbit.query.",
			},
			ExampleBinding: namedQueryExample,
		},
		"parameters": {
			SemanticRole: "named_query_parameters",
			ValueSource: "An object of the named query's parameters. Templates take none. The others: " +
				"context_for_issue, context_for_job, context_for_review, expand_merge_request and expand_work_item take node_ids (a list of ids) and limit, both required; " +
				"expand_neighbors takes entity, node_ids and limit; definition_callees and definition_references take node_id, a string; " +
				"file_definitions and file_definition_callers take project_id and file_path; list_nodes takes entity; search_nodes takes entity, field and text.",
			CommonConfusions: []string{
				"The ids in node_ids are numbers or decimal strings, [14831871432, \"662267755689139244\"], a string being sent as the integer it spells. An id of 2^53 (9007199254740992) or beyond, which a definition's or a file's is, has to be a string: as a number it is read with its last digits changed, so it is refused before the request.",
				"node_id of definition_callees and definition_references is a string, \"662267755689139244\", and GitLab refuses a number.",
				"A parameter the named query does not take is refused with the list of the ones it takes, and a template takes none, so send no parameters for one.",
			},
			ExampleBinding: `{"node_ids": [14831871432], "limit": 100}`,
		},
	}
}
