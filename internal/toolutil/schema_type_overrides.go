// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"reflect"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

// schemaForOptions is the one reflection configuration every input and output
// schema in this server is built with.
//
// It exists for one reason today, and the reason is a divergence between
// surfaces rather than a preference about JSON Schema. [StringOrInt] accepts a
// JSON number as well as a string, because models send numeric IDs as numbers,
// and two of the three surfaces honor that: on dynamic and meta the SDK
// validates gitlab_execute_action's own schema, which declares params as a
// plain object, so the action's parameters reach [UnmarshalParams] and its
// coercion chain. The individual surface publishes the action's schema as the
// tool's own, so the SDK validates project_id against it before any of that
// runs, and a number is refused in a millisecond with a message about types.
//
// The same call therefore worked on the default surface and failed on
// individual. Declaring both types is what makes the published schema say what
// every path already does.
//
// A [Nullable] needs an entry per instantiation, since reflection would
// otherwise publish it as the struct it is. Each publishes what a pointer to
// the type it holds published before it, so moving a field from the pointer
// to the [Nullable] changes what the handler is told and not what the caller
// is offered.
var schemaForOptions = sync.OnceValue(func() *jsonschema.ForOptions {
	return &jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{
			reflect.TypeFor[StringOrInt]():     {Types: []string{"string", "integer"}},
			reflect.TypeFor[Nullable[int64]](): {Types: []string{"null", "integer"}},
		},
	}
})
