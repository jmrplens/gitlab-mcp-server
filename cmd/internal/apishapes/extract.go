package apishapes

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// ErrNotAnAPIDocument is returned when what was parsed is not the document this
// package extracts from. It is separate from a parse failure because the two
// have different causes: a parse failure is a truncated or corrupted download,
// and this is the wrong file entirely.
var ErrNotAnAPIDocument = errors.New("the document declares no OpenAPI version and no paths, so it is not GitLab's API specification")

// spec is only the parts of the OpenAPI document this extraction reads. The
// document is 3.7 MB and describes far more than three lists of names; decoding
// into a narrow type rather than a generic map is what keeps a change in the
// parts we ignore from becoming a change here.
type spec struct {
	OpenAPI string `yaml:"openapi"`
	Info    struct {
		Version string `yaml:"version"`
	} `yaml:"info"`
	// Paths holds each path item undecoded. A path item may carry keys that are
	// not operations, "summary" and a path-level "parameters" among them, which
	// OpenAPI allows and a narrower type refuses to unmarshal: decoding the
	// whole document would then fail over a key this extraction does not read.
	Paths      map[string]map[string]yaml.Node `yaml:"paths"`
	Components struct {
		Schemas map[string]schema `yaml:"schemas"`
	} `yaml:"components"`
}

type operation struct {
	Parameters []struct {
		Name string `yaml:"name"`
		In   string `yaml:"in"`
	} `yaml:"parameters"`
	RequestBody struct {
		Content map[string]struct {
			Schema schema `yaml:"schema"`
		} `yaml:"content"`
	} `yaml:"requestBody"`
	Responses map[string]struct {
		Content map[string]struct {
			Schema schema `yaml:"schema"`
		} `yaml:"content"`
	} `yaml:"responses"`
}

type schema struct {
	Ref        string            `yaml:"$ref"`
	Type       string            `yaml:"type"`
	Properties map[string]schema `yaml:"properties"`
	Items      *schema           `yaml:"items"`
}

// httpMethods are the keys under a path that are operations. A path item also
// carries "parameters" and "summary", which are not.
var httpMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true, "delete": true, "head": true, "options": true,
}

// successCodes are read in this order, so an operation that documents both a
// 200 and a 201 is described by the one a caller of it would get first.
var successCodes = []string{"200", "201", "202"}

// bodyContentTypes are read in this order. GitLab declares most bodies as JSON
// and the file uploads as multipart, and an operation carrying both describes
// the same parameters in each.
var bodyContentTypes = []string{"application/json", "multipart/form-data", "application/x-www-form-urlencoded"}

// Extract reads GitLab's OpenAPI document and returns the three lists of names
// per operation.
func Extract(document []byte) (operations map[string]Operation, openAPIVersion, apiVersion string, err error) {
	var parsed spec
	if parseErr := yaml.Unmarshal(document, &parsed); parseErr != nil {
		return nil, "", "", fmt.Errorf("parse the OpenAPI document: %w", parseErr)
	}
	if parsed.OpenAPI == "" && len(parsed.Paths) == 0 {
		return nil, "", "", ErrNotAnAPIDocument
	}

	resolver := &resolver{schemas: parsed.Components.Schemas}
	operations = map[string]Operation{}
	for path, item := range parsed.Paths {
		for method, node := range item {
			if !httpMethods[strings.ToLower(method)] {
				continue
			}
			var op operation
			if decodeErr := node.Decode(&op); decodeErr != nil {
				return nil, "", "", fmt.Errorf("read %s %s: %w", strings.ToUpper(method), path, decodeErr)
			}
			extracted := Operation{
				Params: parameterNames(op),
				Body:   resolver.bodyProperties(op),
			}
			if object, entity, ok := resolver.responseObject(op); ok {
				extracted.Response = propertyNames(object)
				extracted.Entity = entity
				extracted.Nested, extracted.NestedEntity = resolver.nested(object)
			}
			operations[Key(method, path)] = extracted
		}
	}
	return operations, parsed.OpenAPI, parsed.Info.Version, nil
}

// resolver follows $ref into the component schemas.
type resolver struct{ schemas map[string]schema }

// maxSchemaDepth stops a schema that refers to itself, which several of
// GitLab's do (a namespace holds a parent namespace), from walking forever.
const maxSchemaDepth = 4

// object unwraps a schema to the object at its core, following a reference and
// looking through a list, so a collection endpoint is described by the element
// it returns rather than by the array around it. The name is that of the
// component the object was reached through, the innermost when a reference
// leads to another, and "" for an object the document describes inline. The
// last result is false for a scalar, an unresolvable reference, and a walk
// that ran out of depth.
func (r *resolver) object(s schema, depth int) (object schema, name string, ok bool) {
	if depth > maxSchemaDepth {
		return schema{}, "", false
	}
	switch {
	case s.Ref != "":
		referenced := s.Ref[strings.LastIndexByte(s.Ref, '/')+1:]
		target, found := r.schemas[referenced]
		if !found {
			return schema{}, "", false
		}
		object, name, ok = r.object(target, depth+1)
		if name == "" {
			name = referenced
		}
		return object, name, ok
	case s.Items != nil:
		return r.object(*s.Items, depth+1)
	case len(s.Properties) > 0:
		return s, "", true
	default:
		return schema{}, "", false
	}
}

// properties returns the property names one schema describes.
func (r *resolver) properties(s schema, depth int) []string {
	object, _, ok := r.object(s, depth)
	if !ok {
		return nil
	}
	return propertyNames(object)
}

// propertyNames returns the names an already-resolved object declares, sorted.
func propertyNames(object schema) []string {
	names := make([]string, 0, len(object.Properties))
	for name := range object.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// nested returns, per property of one response object, the property names of
// the object that property carries, and the component that object was reached
// through when the document names one. A property carrying a scalar, or an
// object the document does not describe, is left out rather than recorded
// empty: the two mean different things to a reader and only one of them is
// knowable here.
//
// It takes the response object already resolved, rather than the schema naming
// it, so that the one question "does the document describe an object here" is
// asked once, by [resolver.responseObject], instead of once per caller with a
// second answer nothing could reach.
func (r *resolver) nested(object schema) (properties map[string][]string, entities map[string]string) {
	properties = map[string][]string{}
	entities = map[string]string{}
	for name, property := range object.Properties {
		inner, entity, ok := r.object(property, 1)
		if !ok {
			continue
		}
		if names := propertyNames(inner); len(names) > 0 {
			properties[name] = names
		}
		if entity != "" {
			entities[name] = entity
		}
	}
	if len(properties) == 0 {
		properties = nil
	}
	if len(entities) == 0 {
		entities = nil
	}
	return properties, entities
}

// responseObject resolves the success response to the object it describes,
// reading the codes in the order a caller would meet them, and names the
// component it was reached through.
func (r *resolver) responseObject(op operation) (object schema, entity string, ok bool) {
	for _, code := range successCodes {
		response, found := op.Responses[code]
		if !found {
			continue
		}
		body, hasJSON := response.Content["application/json"]
		if !hasJSON {
			continue
		}
		if object, entity, ok = r.object(body.Schema, 0); ok {
			return object, entity, true
		}
	}
	return schema{}, "", false
}

func (r *resolver) bodyProperties(op operation) []string {
	for _, contentType := range bodyContentTypes {
		body, ok := op.RequestBody.Content[contentType]
		if !ok {
			continue
		}
		if names := r.properties(body.Schema, 0); len(names) > 0 {
			return names
		}
	}
	return nil
}

// parameterNames returns the path and query parameter names. A header
// parameter is left out: it is not something an action's input struct carries.
func parameterNames(op operation) []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(op.Parameters))
	for _, parameter := range op.Parameters {
		if parameter.Name == "" || (parameter.In != "query" && parameter.In != "path") {
			continue
		}
		if seen[parameter.Name] {
			continue
		}
		seen[parameter.Name] = true
		names = append(names, parameter.Name)
	}
	sort.Strings(names)
	return names
}
