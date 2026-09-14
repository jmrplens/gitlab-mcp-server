//go:build e2e

// mcp_capability_surface_test.go holds the two capability surfaces to what
// each one actually serves, through the real binary.
//
// GITLAB_MCP_CAPABILITY_SURFACE is the one switch that changes what exists
// rather than what is allowed: minimal registers no prompt at all and one
// resource, while full registers the whole catalog of both. Every other
// scenario runs on full and would pass identically if minimal served the same
// thing, so nothing else in the suite can tell the switch works.
//
// It also pins the session's own account of itself. A record names a session by
// its label and reports its surfaces, and a reader of a failure has nothing but
// those to tell one session from another, so a label or an accessor that
// disagreed with the server would misattribute every finding in the report.

package common

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestCapabilitySurface_Minimal_ServesNoPromptAndOneResource holds the minimal
// surface to what it is for.
//
// The single resource is gitlab://tools, which is registered unconditionally
// because it is how a client on any surface discovers the call shapes. That it
// is the *only* one is the assertion: minimal exists so a client pays for the
// tool surface and nothing else.
func TestCapabilitySurface_Minimal_ServesNoPromptAndOneResource(t *testing.T) {
	e := harness.New(t)
	s := e.Session(harness.ServerConfig{
		Surface:      harness.SurfaceDynamic,
		Capabilities: harness.CapabilitiesMinimal,
	})

	if got := s.Capabilities(); got != harness.CapabilitiesMinimal {
		t.Errorf("the session reports capability surface %q, want %q", got, harness.CapabilitiesMinimal)
	}
	if prompts := s.Prompts(); len(prompts) != 0 {
		t.Errorf("the minimal surface served %d prompts: %v", len(prompts), prompts)
	}
	if resources := s.Resources(); len(resources) != 1 || resources[0] != "gitlab://tools" {
		t.Errorf("the minimal surface served resources %v, want only gitlab://tools", resources)
	}
}

// TestCapabilitySurface_Full_ServesThePromptAndResourceCatalogs is the other
// half, and the comparison is the point: a switch that changed nothing would
// pass the minimal test above on its own.
func TestCapabilitySurface_Full_ServesThePromptAndResourceCatalogs(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)

	if got := s.Capabilities(); got != harness.CapabilitiesFull {
		t.Errorf("the session reports capability surface %q, want %q", got, harness.CapabilitiesFull)
	}
	prompts := s.Prompts()
	if len(prompts) == 0 {
		t.Error("the full surface served no prompt, which is what the minimal one is for")
	}
	if resources := s.Resources(); len(resources) <= 1 {
		t.Errorf("the full surface served %d resources, want the catalog rather than the minimal one", len(resources))
	}
	if templates := s.ResourceTemplates(); len(templates) == 0 {
		t.Error("the full surface served no resource template, so no parameterized resource can be read")
	}
}

// TestPrompts_ProjectHealthCheck_RendersAgainstTheWorld renders one prompt and
// asserts what came back.
//
// The sweep in mcp_prompts_test.go renders every prompt it can bind and logs
// whatever answers an error, deliberately: a prompt whose data the read-only
// World does not carry is not a defect, and one prompt failing must not stop
// the other thirty-six from being exercised. The cost is that a prompt could
// fail for every run and the sweep would stay green.
//
// This is the other half: one prompt whose arguments the World always binds,
// rendered through the strict verb, with its messages asserted. A prompt that
// answers an error here fails the suite, and a prompt that answers an empty
// body fails it too, since a rendered prompt with no content is what a client
// would paste into a model as nothing at all.
func TestPrompts_ProjectHealthCheck_RendersAgainstTheWorld(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)
	s := e.On(harness.SurfaceDynamic)

	projectID, bound := world.BindPromptArgument("project_id")
	if !bound {
		t.Skip("the World binds no project_id, so there is no prompt argument to render with")
	}

	result := s.GetPrompt("project_health_check", map[string]string{"project_id": projectID})
	if result == nil || len(result.Messages) == 0 {
		t.Fatal("the prompt rendered no message, which is nothing for a client to send a model")
	}
	var text strings.Builder
	for _, message := range result.Messages {
		if content, ok := message.Content.(*mcp.TextContent); ok {
			text.WriteString(content.Text)
		}
	}
	if strings.TrimSpace(text.String()) == "" {
		t.Error("the prompt's messages carry no text at all")
	}
}

// TestSession_DescribesItself_AsTheRecordNamesIt checks the accessors a report
// is written from.
//
// A failure in this suite is read through the record, which names the session
// by its label and its surfaces. If those disagreed with the server the session
// is actually talking to, every finding attributed to it would name the wrong
// configuration, and no assertion about GitLab could catch that.
func TestSession_DescribesItself_AsTheRecordNamesIt(t *testing.T) {
	e := harness.New(t)
	s := e.Session(harness.ServerConfig{Surface: harness.SurfaceMeta, Mode: harness.ModeReadOnly})

	if got := s.Surface(); got != harness.SurfaceMeta {
		t.Errorf("Surface() = %q, want %q", got, harness.SurfaceMeta)
	}
	if got := s.Mode(); got != harness.ModeReadOnly {
		t.Errorf("Mode() = %q, want %q", got, harness.ModeReadOnly)
	}
	// Stdio is the only transport wired, and saying so here is what will fail
	// the day a session is given another one without this being revisited.
	if got := s.Transport(); got != harness.TransportStdio {
		t.Errorf("Transport() = %q, want %q", got, harness.TransportStdio)
	}
	label := s.Label()
	for _, part := range []string{string(harness.SurfaceMeta), string(harness.ModeReadOnly)} {
		t.Run("the label names "+part, func(t *testing.T) {
			if !strings.Contains(label, part) {
				t.Errorf("the session label %q does not name %q, so a record cannot be read back to this shape", label, part)
			}
		})
	}
}
