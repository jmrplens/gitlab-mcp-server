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
			operations[Key(method, path)] = Operation{
				Response: resolver.responseProperties(op),
				Params:   parameterNames(op),
				Body:     resolver.bodyProperties(op),
			}
		}
	}
	return operations, parsed.OpenAPI, parsed.Info.Version, nil
}

// resolver follows $ref into the component schemas.
type resolver struct{ schemas map[string]schema }

// properties returns the property names one schema describes, following a
// reference and unwrapping a list, so a collection endpoint is described by the
// element it returns rather than by the array around it.
//
// depth stops a schema that refers to itself, which several of GitLab's do
// (a namespace holds a parent namespace), from walking forever.
func (r *resolver) properties(s schema, depth int) []string {
	if depth > 4 {
		return nil
	}
	switch {
	case s.Ref != "":
		name := s.Ref[strings.LastIndexByte(s.Ref, '/')+1:]
		target, ok := r.schemas[name]
		if !ok {
			return nil
		}
		return r.properties(target, depth+1)
	case s.Items != nil:
		return r.properties(*s.Items, depth+1)
	case len(s.Properties) > 0:
		names := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			names = append(names, name)
		}
		sort.Strings(names)
		return names
	default:
		return nil
	}
}

func (r *resolver) responseProperties(op operation) []string {
	for _, code := range successCodes {
		response, ok := op.Responses[code]
		if !ok {
			continue
		}
		if body, hasJSON := response.Content["application/json"]; hasJSON {
			if names := r.properties(body.Schema, 0); len(names) > 0 {
				return names
			}
		}
	}
	return nil
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
