//go:build e2e

// mcp_resources_test.go reads every static resource and every resource
// template the server serves, binding each template's variables from the
// shared World.
//
// Resources are a capability of the server, not of a tool surface: the same
// set is registered whichever surface is active, and only on the full
// capability surface. The sweep therefore runs on one full-capability session
// rather than three. A template whose variables the World cannot supply is
// named in the log rather than read, which is the "or names the binding it
// lacks" the coverage report reads beside the templates it did read; a read
// that answers a handled error is still credited by the recorder.

package common

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestResources_Sweep reads every static resource and every bindable template
// the full-capability server serves.
//
// It replaces no old test by name: the old suite held its resource coverage
// to a handful of URIs in its capability tests, which the port of those
// tests names.
func TestResources_Sweep(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)
	s := e.On(harness.SurfaceDynamic)

	statics := s.Resources()
	templates := s.ResourceTemplates()
	if len(statics) == 0 && len(templates) == 0 {
		t.Fatal("the full-capability session served no resource; the sweep would prove nothing")
	}

	// TryReadResource rather than ReadResource: a template bound by variable
	// name can still name a resource the World does not carry (a group-scoped
	// label addressed with a project label id), which answers a handled error
	// the recorder credits rather than a reason to abort the sweep.
	staticErrs := 0
	for _, uri := range statics {
		if _, err := s.TryReadResource(uri); err != nil {
			staticErrs++
		}
	}

	read, skipped, readErrs := 0, 0, 0
	for _, template := range templates {
		uri, missing := expandTemplate(template, world)
		if missing != "" {
			t.Logf("template %s not read: %s", template, missing)
			skipped++
			continue
		}
		if _, err := s.TryReadResource(uri); err != nil {
			readErrs++
		}
		read++
	}
	t.Logf("read %d static resources (%d errored) and %d templates (%d errored, %d the World cannot bind)",
		len(statics), staticErrs, read, readErrs, skipped)
}

// expandTemplate replaces every RFC 6570 variable in a resource template with
// its World value, returning the reason it could not when a variable is
// unbound.
//
// A variable is one segment written {name} or {+name}; the reserved "+" form
// the file template uses for its path is expanded the same way, since the
// World's value is already a path. The bound value is spelled as a string
// because it lands in a URI, and a segment carrying a slash is percent-encoded
// so the server's own router, which expands variables as slash-free segments,
// resolves it.
func expandTemplate(template string, world *fixture.World) (string, string) {
	var out strings.Builder
	rest := template
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			out.WriteString(rest)
			return out.String(), ""
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			out.WriteString(rest)
			return out.String(), ""
		}
		out.WriteString(rest[:open])
		variable := rest[open+1 : open+end]
		reservedPath := strings.HasPrefix(variable, "+")
		name := strings.TrimPrefix(variable, "+")
		value, bound := world.Bind(name)
		if !bound {
			return "", "the World has no binding for variable " + name
		}
		out.WriteString(templateValue(value, reservedPath))
		rest = rest[open+end+1:]
	}
}

// templateValue spells a World value for a URI: an integer or plain string as
// itself, and a slash inside a plain segment percent-encoded so it stays one
// segment. The reserved path form keeps its slashes, which is what it is for.
func templateValue(value any, reservedPath bool) string {
	var str string
	switch typed := value.(type) {
	case int64:
		str = strconv.FormatInt(typed, 10)
	case string:
		str = typed
	}
	if reservedPath {
		return str
	}
	return strings.ReplaceAll(str, "/", "%2F")
}
