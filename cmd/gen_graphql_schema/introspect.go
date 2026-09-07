package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// introspectionQuery asks for everything the SDL needs and nothing it does
// not. Descriptions and deprecation reasons are left out because validation
// never consults them and they would triple the file.
//
// None of that shape currently decides anything, and the comment would be
// misleading without saying so: every GitLab instance answers any
// introspection-shaped operation with its own canned full __schema payload and
// ignores the selection set entirely. Asking gitlab.com for
// `{ metadata { version } __type(name: "Vulnerability") { name } }` returns the
// whole schema, 4331 types, carrying description, isDeprecated and
// deprecationReason keys nothing here requested; gitlab.gnome.org,
// invent.kde.org and salsa.debian.org all behave the same way. The query is
// written correctly anyway, because the day an instance starts honoring it is
// not a day anybody will be watching this file.
//
// includeDeprecated is asked for on fields and enum values because a
// deprecated field is still a field GitLab serves, and our documents select
// several. It is deliberately not asked for on arguments or input fields:
// deprecating those arrived later in graphql-ruby, and a self-managed instance
// that does not accept the argument would refuse the whole introspection
// rather than answer without it. The risk points both ways and only one
// direction is obvious. An old instance refusing an unknown argument loses the
// whole pin loudly; the specification defaulting args(includeDeprecated:) to
// false loses the deprecated arguments silently, and the gate would then refuse
// documents GitLab still accepts.
//
// The ofType chain is nine deep, which covers every wrapper GitLab nests
// (a non-null list of non-null lists reaches four) with room to spare.
const introspectionQuery = `query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      kind
      name
      fields(includeDeprecated: true) {
        name
        args { ...InputValue }
        type { ...TypeRef }
      }
      inputFields { ...InputValue }
      interfaces { name }
      enumValues(includeDeprecated: true) { name }
      possibleTypes { name }
    }
  }
}

fragment InputValue on __InputValue {
  name
  type { ...TypeRef }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
                ofType { kind name }
              }
            }
          }
        }
      }
    }
  }
}`

// metadataQuery asks the instance what version it runs. GitLab answers it with
// null to an anonymous caller, so the generator treats a null as "unknown"
// rather than as a failure.
const metadataQuery = `query { metadata { version revision } }`

// schemaIntrospection is the __schema payload, holding only what the SDL
// renderer reads.
type schemaIntrospection struct {
	QueryType        *typeName    `json:"queryType"`
	MutationType     *typeName    `json:"mutationType"`
	SubscriptionType *typeName    `json:"subscriptionType"`
	Types            []schemaType `json:"types"`
}

// typeName is a bare reference to a named type.
type typeName struct {
	Name string `json:"name"`
}

// schemaType is one introspected type in any of the six kinds.
type schemaType struct {
	Kind          string       `json:"kind"`
	Name          string       `json:"name"`
	Fields        []schemaItem `json:"fields"`
	InputFields   []inputValue `json:"inputFields"`
	Interfaces    []typeName   `json:"interfaces"`
	EnumValues    []typeName   `json:"enumValues"`
	PossibleTypes []typeName   `json:"possibleTypes"`
}

// schemaItem is one output field with the arguments it accepts.
type schemaItem struct {
	Name string       `json:"name"`
	Args []inputValue `json:"args"`
	Type *typeRef     `json:"type"`
}

// inputValue is one argument or input-object field. DefaultValue is a pointer
// because introspection reports the absence of a default as null, which is not
// the same as a default whose literal happens to be empty.
type inputValue struct {
	Name         string   `json:"name"`
	Type         *typeRef `json:"type"`
	DefaultValue *string  `json:"defaultValue"`
}

// typeRef is a type reference, wrapped in NON_NULL and LIST nodes.
type typeRef struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	OfType *typeRef `json:"ofType"`
}

// graphQLResponse is the envelope both queries come back in.
type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// introspectData is the shape of a successful introspection response.
type introspectData struct {
	Schema *schemaIntrospection `json:"__schema"`
}

// metadataData is the shape of a successful metadata response.
type metadataData struct {
	Metadata *struct {
		Version  string `json:"version"`
		Revision string `json:"revision"`
	} `json:"metadata"`
}

// introspect asks the instance for its whole schema.
func introspect(ctx context.Context, cfg genRun) (*schemaIntrospection, error) {
	raw, err := post(ctx, cfg, introspectionQuery)
	if err != nil {
		return nil, err
	}
	var data introspectData
	if unmarshalErr := json.Unmarshal(raw, &data); unmarshalErr != nil {
		return nil, fmt.Errorf("decode the introspection payload: %w", unmarshalErr)
	}
	if data.Schema == nil || len(data.Schema.Types) == 0 {
		return nil, fmt.Errorf("%s answered introspection with no types", cfg.endpoint)
	}
	return data.Schema, nil
}

// instanceVersion asks the instance what it runs. Failure is not fatal: the
// schema is the artifact and the version is provenance, so an anonymous run
// records unknownVersion and carries on.
func instanceVersion(ctx context.Context, cfg genRun) (version, revision string) {
	raw, err := post(ctx, cfg, metadataQuery)
	if err != nil {
		return unknownVersion, ""
	}
	var data metadataData
	if unmarshalErr := json.Unmarshal(raw, &data); unmarshalErr != nil || data.Metadata == nil {
		return unknownVersion, ""
	}
	return data.Metadata.Version, data.Metadata.Revision
}

// post sends one GraphQL document and returns the data member of the answer.
func post(ctx context.Context, cfg genRun, document string) (json.RawMessage, error) {
	// A map of strings always marshals, and a caller could do nothing with the
	// error if it did not.
	body := cmdutil.Must(json.Marshal(map[string]string{"query": document}))

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build the request for %s: %w", cfg.endpoint, err)
	}
	request.Header.Set("Content-Type", "application/json")
	if cfg.token != "" {
		request.Header.Set("Authorization", "Bearer "+cfg.token)
	}

	response, err := cfg.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("ask %s: %w", cfg.endpoint, err)
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read the answer from %s: %w", cfg.endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s: %s", cfg.endpoint, response.Status, snippet(payload))
	}

	var envelope graphQLResponse
	if unmarshalErr := json.Unmarshal(payload, &envelope); unmarshalErr != nil {
		return nil, fmt.Errorf("decode the answer from %s: %w", cfg.endpoint, unmarshalErr)
	}
	if len(envelope.Errors) > 0 {
		messages := make([]string, 0, len(envelope.Errors))
		for _, item := range envelope.Errors {
			messages = append(messages, item.Message)
		}
		return nil, fmt.Errorf("%s refused the query: %s", cfg.endpoint, strings.Join(messages, "; "))
	}
	return envelope.Data, nil
}

// snippet shortens a response body so an error line stays readable when the
// instance answered with an HTML error page.
func snippet(payload []byte) string {
	const limit = 200
	text := strings.TrimSpace(string(payload))
	if len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}
