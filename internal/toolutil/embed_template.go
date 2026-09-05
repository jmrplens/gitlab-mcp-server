package toolutil

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// resourceURIScheme is the scheme every canonical resource URI carries; the
// resource registry serves nothing else, so a template with another scheme
// is a typo the spec validator refuses.
const resourceURIScheme = "gitlab://"

// ResourceTemplateVariables returns the names of the {variables} in a URI
// template, in order of appearance, without the RFC 6570 "+" reserved-expansion
// prefix. An unterminated brace ends the scan.
func ResourceTemplateVariables(template string) []string {
	var names []string
	rest := template
	for {
		open := strings.IndexByte(rest, '{')
		if open == -1 {
			return names
		}
		closing := strings.IndexByte(rest[open:], '}')
		if closing == -1 {
			return names
		}
		name := strings.TrimPrefix(rest[open+1:open+closing], "+")
		if name != "" {
			names = append(names, name)
		}
		rest = rest[open+closing+1:]
	}
}

// ExpandResourceURI fills a canonical resource URI template from an action's
// parameters. Each {name} is replaced by the path-escaped value of the parameter
// of that name, so a project given as "group/project" lands in the URI as
// group%2Fproject, which is the form the resource templates accept; {+name}
// keeps the slashes of a path-valued parameter, escaping each segment. It
// reports false when the template is empty or any variable is absent or empty,
// so a result whose identifier the caller never supplied gets no resource block
// instead of a URI with a hole in it.
func ExpandResourceURI(template string, params map[string]any) (string, bool) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", false
	}
	var b strings.Builder
	rest := template
	for {
		open := strings.IndexByte(rest, '{')
		if open == -1 {
			b.WriteString(rest)
			return b.String(), true
		}
		closing := strings.IndexByte(rest[open:], '}')
		if closing == -1 {
			return "", false
		}
		b.WriteString(rest[:open])
		name := rest[open+1 : open+closing]
		reserved := strings.HasPrefix(name, "+")
		name = strings.TrimPrefix(name, "+")
		value, ok := resourceParamValue(params[name])
		if !ok {
			return "", false
		}
		if reserved {
			segments := strings.Split(value, "/")
			for i, segment := range segments {
				segments[i] = url.PathEscape(segment)
			}
			b.WriteString(strings.Join(segments, "/"))
		} else {
			b.WriteString(url.PathEscape(value))
		}
		rest = rest[open+closing+1:]
	}
}

// resourceParamValue renders one parameter for a URI. JSON numbers arrive as
// float64, and an identifier must never be written as 4.2e+01 or 42.0, so
// integral values are formatted as integers; anything empty is reported as
// absent.
func resourceParamValue(value any) (string, bool) {
	switch v := value.(type) {
	case nil:
		return "", false
	case string:
		v = strings.TrimSpace(v)
		return v, v != ""
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1e15 {
			return strconv.FormatInt(int64(v), 10), true
		}
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case bool:
		return "", false
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		return s, s != ""
	}
}

// EmbedCanonicalResource appends the canonical resource of a successful action
// result: the template is the one the action's spec declares, expanded from
// the parameters the call carried, and the payload is the JSON form of the
// output. It is a no-op without a template, on an error result, when a
// variable is missing, or when embedding is disabled, so every dispatcher can
// call it unconditionally after formatting.
func EmbedCanonicalResource(result *mcp.CallToolResult, template string, params map[string]any, value any) {
	if result == nil || result.IsError || template == "" {
		return
	}
	uri, ok := ExpandResourceURI(template, params)
	if !ok {
		return
	}
	EmbedResourceJSON(result, uri, value)
}

// WithEmbeddedResource returns the spec with its canonical resource declared:
// the given gitlab:// URI template, expanded from the call's parameters, is
// embedded in every successful result. Declared at the spec site so the
// action's owner package states which resource a get returns, and the catalog
// validator holds the template to the action's parameters.
func (spec ActionSpec) WithEmbeddedResource(template string) ActionSpec {
	spec.EmbeddedResourcePolicy = ActionSpecEmbeddedAlways
	spec.EmbeddedResource = strings.TrimSpace(template)
	return spec
}

// validateEmbeddedResource checks that a spec's embedded-resource declaration
// can work at run time: a template needs a policy that embeds, an "always"
// policy needs a template, the template must be a gitlab:// URI, and every
// variable in it must be a parameter the action accepts, since the URI is
// expanded from the call's parameters and a name the schema does not know
// would silently leave the result without its resource.
func validateEmbeddedResource(spec ActionSpec) error {
	policy := strings.TrimSpace(spec.EmbeddedResourcePolicy)
	template := strings.TrimSpace(spec.EmbeddedResource)
	if template == "" {
		if policy == ActionSpecEmbeddedAlways {
			return fmt.Errorf("action spec %q has embedded resource policy %q but no embedded resource template", spec.Name, policy)
		}
		return nil
	}
	if policy == "" || policy == ActionSpecEmbeddedNone {
		return fmt.Errorf("action spec %q declares embedded resource %q but its policy %q never embeds", spec.Name, template, policy)
	}
	if !strings.HasPrefix(template, resourceURIScheme) {
		return fmt.Errorf("action spec %q embedded resource %q is not a %s URI", spec.Name, template, resourceURIScheme)
	}
	variables := ResourceTemplateVariables(template)
	if len(variables) == 0 {
		return nil
	}
	properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
	if properties == nil {
		return nil
	}
	for _, name := range variables {
		if _, ok := properties[name]; !ok {
			return fmt.Errorf("action spec %q embedded resource %q names parameter %q, which the action does not accept", spec.Name, template, name)
		}
	}
	return nil
}
