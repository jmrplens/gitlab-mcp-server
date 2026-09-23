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
// prefix. A brace that never closes, or a pair with nothing between them, is an
// error rather than the end of the scan: ExpandResourceURI answers such a
// template by embedding nothing, and a declaration that validation had let
// through would fail that quietly on every call.
func ResourceTemplateVariables(template string) ([]string, error) {
	var names []string
	rest := template
	for {
		_, afterOpen, opened := strings.Cut(rest, "{")
		if !opened {
			return names, nil
		}
		name, afterClose, closed := strings.Cut(afterOpen, "}")
		if !closed {
			return nil, fmt.Errorf("resource template %q has an unterminated variable at %q", template, "{"+afterOpen)
		}
		name = strings.TrimPrefix(name, "+")
		if name == "" {
			return nil, fmt.Errorf("resource template %q has a variable with no name", template)
		}
		names = append(names, name)
		rest = afterClose
	}
}

// ExpandResourceURI fills a canonical resource URI template from an action's
// parameters. Each {name} is replaced by the value of the parameter of that name
// escaped as RFC 6570 simple expansion escapes it ([escapeSimpleExpansion]), so
// a project given as "group/project" lands in the URI as group%2Fproject and a
// scoped label priority::high as priority%3A%3Ahigh, which is the form the
// resource templates accept and the resource handlers decode once; {+name}
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
			b.WriteString(escapeSimpleExpansion(value))
		}
		rest = rest[open+closing+1:]
	}
}

// upperHex is the digit set RFC 3986 section 2.1 asks a producer to write a
// percent-encoding in.
const upperHex = "0123456789ABCDEF"

// escapeSimpleExpansion percent-encodes value the way an RFC 6570 simple
// expansion does: every byte outside ALPHA, DIGIT and "-" "." "_" "~" becomes
// %XX, a multi-byte character one escape per byte.
//
// Go's url.PathEscape is not that. It encodes a path segment, which is allowed
// to carry "$" "&" "+" ":" "=" "@" raw, and the go-sdk router matches a simple
// variable against the unreserved set and escapes alone, so a URI that
// PathEscape wrote for a scoped label (priority::high) or a tag with build
// metadata (v1.0.0+build) matched no template and the resource block a tool
// result carried could not be read back.
func escapeSimpleExpansion(value string) string {
	var b strings.Builder
	for i := range len(value) {
		c := value[i]
		if isUnreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperHex[c>>4])
		b.WriteByte(upperHex[c&0x0F])
	}
	return b.String()
}

// unreservedBytes is RFC 3986's unreserved set, the only bytes a simple
// expansion writes as themselves.
const unreservedBytes = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"

// isUnreserved reports whether c is in [unreservedBytes].
func isUnreserved(c byte) bool {
	return strings.IndexByte(unreservedBytes, c) >= 0
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
//
// [FinishToolResult] calls it last, after the hints have been set on the
// output, so the JSON the block carries and the structured output of the
// same result are one serialization: an embed run before the hints carried
// a body that lacked the next_steps the structured output had.
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
	variables, err := ResourceTemplateVariables(template)
	if err != nil {
		return fmt.Errorf("action spec %q: %w", spec.Name, err)
	}
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
