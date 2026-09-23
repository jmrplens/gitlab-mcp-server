//go:build e2e

// mcp_resources_test.go reads every static resource and every resource
// template the server serves, binding each template's variables from the
// shared World.
//
// Resources are a capability of the server, not of a tool surface: the same
// set is registered whichever surface is active, and only on the full
// capability surface. The sweep therefore runs on one full-capability session
// rather than three, and the coverage command counts each resource once per
// capability surface for the same reason. A template whose variables the
// World cannot supply is named in the log rather than read, which is the "or
// names the binding it lacks" the coverage report reads beside the templates
// it did read; a read that answers a handled error is still credited by the
// recorder.
//
// The tool manifest, gitlab://tools and gitlab://tools/{id}, is the exception
// and is left to mcp_manifest_test.go: its content is what the session's tool
// surface registered in its mode, so it is counted per surface x mode x
// capability surface and a read here would fill one of those cells and say
// nothing of the others.

package common

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/resources"
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
	// the recorder credits rather than a reason to abort the sweep. Each read
	// says it is a sweep's, since it asserts nothing about what came back,
	// and the coverage command credits it as sweep-only rather than asserted.
	sweep := harness.For(harness.PurposeSweep)
	manifest := resources.ToolSurfaceResourceURIs()
	staticsRead, staticErrs := 0, 0
	for _, uri := range statics {
		if slices.Contains(manifest, uri) {
			t.Logf("resource %s not read here: TestManifest_EveryShape_ReadsTheIndexAndAnEntry reads it on every shape the suite holds sessions for", uri)
			continue
		}
		staticsRead++
		if _, err := s.TryReadResource(uri, sweep); err != nil {
			staticErrs++
		}
	}

	read, skipped, readErrs := 0, 0, 0
	for _, template := range templates {
		if slices.Contains(manifest, template) {
			t.Logf("template %s not read here: TestManifest_EveryShape_ReadsTheIndexAndAnEntry reads it on every shape the suite holds sessions for", template)
			continue
		}
		uri, missing := expandTemplate(template, world)
		if missing != "" {
			t.Logf("template %s not read: %s", template, missing)
			skipped++
			continue
		}
		if _, err := s.TryReadResource(uri, sweep); err != nil {
			readErrs++
		}
		read++
	}
	t.Logf("read %d static resources (%d errored) and %d templates (%d errored, %d the World cannot bind)",
		staticsRead, staticErrs, read, readErrs, skipped)
}

// expandTemplate replaces every RFC 6570 variable in a resource template with
// its World value, returning the reason it could not when a variable is
// unbound.
//
// The value is the World's for that template ([fixture.World.BindTemplate]),
// since a variable's name does not always mean one object: a group template's
// label_id is the group's label, not the project's. An unbound variable comes
// back with the World's own reason, which names the object it could not make
// and why.
//
// A variable is one segment written {name} or {+name}; the reserved "+" form
// the file template uses for its path is expanded the same way, since the
// World's value is already a path. The bound value is spelled as a string
// because it lands in a URI, and it is spelled the way a conforming client
// spells it ([templateValue]), so the sweep reads what any RFC 6570 client
// would send. The branch template is bound to the World's feature branch,
// slash and all, on purpose: it arrives as feature%2Fworld and the server has
// to decode it once before GitLab is asked (issue 912), and binding a branch
// without a slash would leave that decode unexercised.
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
		value, bound, reason := world.BindTemplate(template, name)
		if !bound {
			return "", reason
		}
		out.WriteString(templateValue(value, reservedPath))
		rest = rest[open+end+1:]
	}
}

// templateValue spells a World value for a URI the way RFC 6570 expands it: a
// simple variable has every byte outside the unreserved set percent-encoded, so
// a slash, a colon or a space stays inside its one segment, and the reserved
// path form keeps its slashes, which is what it is for. It is written here
// rather than borrowed from the server's own expansion, because a sweep that
// spelled its URIs with the code under test would agree with it by
// construction.
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
	var out strings.Builder
	for i := range len(str) {
		c := str[i]
		if strings.IndexByte(uriUnreserved, c) >= 0 {
			out.WriteByte(c)
			continue
		}
		fmt.Fprintf(&out, "%%%02X", c)
	}
	return out.String()
}

// uriUnreserved is RFC 3986's unreserved set, the bytes a simple expansion
// writes as themselves.
const uriUnreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"

// TestResources_Branch_NameWithASlash_ReadsTheBranch reads the World's feature
// branch through the branch template, with the project given as its path and
// both variables percent-encoded the way RFC 6570 simple expansion encodes
// them, and asserts that the branch which comes back is the one the URI names.
//
// It is the read issue 912 was about. The resource handed the encoded segments
// to client-go, which escaped them again, so GitLab was asked for a branch
// named feature%2Fworld in a project named with a %2F of its own, and
// answered 404. The sweep reads this template too, but a sweep read proves
// only that the template answered; this one holds the answer to the object,
// against a real GitLab.
func TestResources_Branch_NameWithASlash_ReadsTheBranch(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)
	if !strings.Contains(world.Branch.Name, "/") {
		t.Fatalf("the World's branch %q carries no slash, so reading it proves nothing about decoding one", world.Branch.Name)
	}
	s := e.On(harness.SurfaceDynamic)

	uri := "gitlab://project/" + templateValue(world.Project.Path, false) + "/branch/" + templateValue(world.Branch.Name, false)
	result := s.ReadResource(uri)
	if len(result.Contents) == 0 {
		t.Fatalf("%s answered with no contents", uri)
	}
	var branch resources.BranchResourceOutput
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &branch); err != nil {
		t.Fatalf("decoding %s: %v", uri, err)
	}
	if branch.Name != world.Branch.Name {
		t.Errorf("%s read branch %q, want %q", uri, branch.Name, world.Branch.Name)
	}
}
