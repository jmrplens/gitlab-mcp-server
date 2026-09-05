package toolutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"github.com/google/jsonschema-go/jsonschema"
)

// Process-lived schemas and the transforms derived from them.
//
// A JSON Schema reaches the wire through a chain of small rewrites: the
// canonical enums and overrides an ActionSpec applies, the tier pruning, the
// destructive-confirmation property, the parameter guidance, the individual
// surface's required-field and lockdown pass, and the two tools/list
// middlewares. Each used to run on a private deep copy, once per MCP server,
// and the HTTP pool builds one server per credential; the copies were half of
// the heap at a hundred credentials (see docs/development/resource-hot-spots.md).
//
// The rewrites are pure functions of their input, so their results can be
// shared as long as the input has a stable identity. A map or a compiled
// schema that lives for the process has one: its address. [ShareSchema]
// registers such a schema, keeping it reachable so the address can never be
// reused, and [DeriveSchema] memoizes a named transform of a registered
// schema by that address. A schema nobody registered is derived privately, as
// before, so a test that builds its own catalog and a tool registered with an
// ad hoc map keep their old behavior and leak nothing into the memo.
//
// Every transform registered here is idempotent, and that is relied upon: the
// output of a transform is itself registered as shared and recorded as that
// transform's output, so applying the same transform to it again returns it
// unchanged instead of building a chain of identical maps. The catalog build
// clones a spec several times, and each clone reapplies the spec transform.
var (
	// sharedSchemas holds every registered schema, keyed by address. The
	// value is the schema itself, which is what keeps it alive: an address
	// is only an identity while the object at it cannot be collected.
	sharedSchemas sync.Map // unsafe.Pointer -> any
	// derivedSchemas memoizes transform outputs by input identity and
	// transform name.
	derivedSchemas sync.Map // derivedSchemaKey -> any
	// derivedOrigin records which transform produced a shared schema, so the
	// idempotent reapplication of that transform returns the schema itself.
	derivedOrigin sync.Map // unsafe.Pointer -> string
)

type derivedSchemaKey struct {
	identity  unsafe.Pointer
	transform string
}

// schemaIdentity returns the address that identifies a schema value: the map
// header of a map[string]any or the pointer of a compiled *jsonschema.Schema.
// Any other value, and a nil of either kind, has no identity.
func schemaIdentity(schema any) (unsafe.Pointer, bool) {
	switch typed := schema.(type) {
	case map[string]any:
		if typed == nil {
			return nil, false
		}
		return reflect.ValueOf(typed).UnsafePointer(), true
	case *jsonschema.Schema:
		if typed == nil {
			return nil, false
		}
		return unsafe.Pointer(typed), true
	default:
		return nil, false
	}
}

// ShareSchema registers a schema as process-lived so that transforms of it
// are memoized by [DeriveSchema]. Registering keeps the schema reachable for
// the rest of the process, which is the point: only a schema that can never
// be collected has an address that can serve as its identity.
//
// Register what the process builds once and serves from then on: the
// reflected type schemas, the compiled tool schemas, and the routes of a
// catalog cached per configuration. Never register a per-server map.
func ShareSchema(schema any) {
	if identity, ok := schemaIdentity(schema); ok {
		sharedSchemas.LoadOrStore(identity, schema)
	}
}

// SchemaShared reports whether schema was registered with [ShareSchema], or
// produced by [DeriveSchema] from a registered schema.
func SchemaShared(schema any) bool {
	identity, ok := schemaIdentity(schema)
	if !ok {
		return false
	}
	_, shared := sharedSchemas.Load(identity)
	return shared
}

// SharedSchemaIdentity returns a string naming a registered schema's
// identity, for callers that build a composite cache key out of several
// schemas. It reports false for a schema that is not shared, because the
// address of a map that can be collected names nothing durable.
func SharedSchemaIdentity(schema any) (string, bool) {
	if !SchemaShared(schema) {
		return "", false
	}
	identity, _ := schemaIdentity(schema)
	return fmt.Sprintf("%p", identity), true
}

// ParameterGuidanceIdentity names a guidance map by its content, for a
// transform key whose output depends on the guidance as well as on the
// schema. Nil and empty maps name the same thing, since a transform treats
// them alike.
//
// By content and not by address, which was the other way to close the same
// hole. [DeriveSchema] memoizes on the identity of the input schema, and every
// typed route's input schema is shared, so a route from a catalog nobody
// retained still writes a memo entry that lives for the process. An address in
// that key would be the address of a guidance map that does not: once the
// catalog is collected the allocator may hand the address out again, and the
// next map at it would be served the previous route's guidance. Pinning the
// guidance of every catalog marked shared would close it for the routes
// somebody remembered to pin and leave the rest keyed on a transient address;
// a digest cannot be forgotten, holds nothing alive, and lets two routes that
// spell the same guidance share one derivation.
//
// The cost is a hash of a few short strings per lookup, against the schema
// clone the memo saves.
func ParameterGuidanceIdentity(guidance map[string]ParameterGuidance) string {
	if len(guidance) == 0 {
		return ""
	}
	names := make([]string, 0, len(guidance))
	for name := range guidance {
		names = append(names, name)
	}
	slices.Sort(names)
	var canonical strings.Builder
	for _, name := range names {
		entry := guidance[name]
		// Every field quoted, so that no shifting of characters from one
		// field into the next can spell the same canonical form twice.
		fmt.Fprintf(&canonical, "%q=%q|%q|%q|%q;",
			name, entry.SemanticRole, entry.ValueSource, entry.ExampleBinding, entry.CommonConfusions)
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(digest[:])
}

// DeriveSchema returns transform applied to schema, where derive builds the
// result and transform names the rewrite it performs. The name must identify
// the rewrite completely: two calls with the same name on the same schema are
// assumed to want the same output, so any parameter the rewrite depends on
// belongs in the name.
//
// For a schema registered with [ShareSchema] the result is built once per
// process, registered as shared in turn, and returned to every later caller;
// derive must therefore never mutate its input. For any other schema the
// result is built privately on every call, exactly as before this memo
// existed. A transform reapplied to its own output returns that output.
func DeriveSchema(schema any, transform string, derive func() any) any {
	identity, ok := schemaIdentity(schema)
	if !ok {
		return derive()
	}
	if _, shared := sharedSchemas.Load(identity); !shared {
		return derive()
	}
	if origin, produced := derivedOrigin.Load(identity); produced && origin == transform {
		return schema
	}
	key := derivedSchemaKey{identity: identity, transform: transform}
	if cached, hit := derivedSchemas.Load(key); hit {
		return cached
	}
	out := derive()
	actual, loaded := derivedSchemas.LoadOrStore(key, out)
	if !loaded {
		ShareSchema(out)
		if outIdentity, hasIdentity := schemaIdentity(out); hasIdentity {
			derivedOrigin.LoadOrStore(outIdentity, transform)
		}
	}
	return actual
}

// CloneSchemaMap returns a deep copy of a JSON Schema map, for a caller that
// has a reason to mutate one: every schema reachable from a shared catalog is
// shared with every server in the process and must be copied before it is
// changed.
func CloneSchemaMap(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	return cloneSchemaMap(schema)
}
