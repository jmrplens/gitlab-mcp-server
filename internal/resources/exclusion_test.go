package resources

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// collectingRegistrar records what registerAll offers without a server.
type collectingRegistrar struct {
	uris []string
}

func (r *collectingRegistrar) AddResource(resource *mcp.Resource, _ mcp.ResourceHandler) {
	r.uris = append(r.uris, resource.URI)
}

func (r *collectingRegistrar) AddResourceTemplate(template *mcp.ResourceTemplate, _ mcp.ResourceHandler) {
	r.uris = append(r.uris, template.URITemplate)
}

// registeredResourceURIs returns every URI and URI template registerAll offers,
// with the options applied.
func registeredResourceURIs(client *gitlabclient.Client, opts ...RegisterOptions) []string {
	collector := &collectingRegistrar{}
	registerAll(registrarFor(collector, opts), client)
	return collector.uris
}

// TestRegister_ExcludingAnActionRemovesTheResourceServingTheSameData verifies
// that an operator's --exclude-tools reaches the resource surface, which is the
// second request path to the same GitLab data with the same credential.
//
// Excluding gitlab_project_get used to leave gitlab://project/{id} serving the
// identical project object, so the narrowing an operator configured was half a
// contract. Each subtest asserts both directions: the resource whose data went
// is gone, and every resource that shares no action with the exclusion is still
// there, because a filter that removes too much is its own defect.
func TestRegister_ExcludingAnActionRemovesTheResourceServingTheSameData(t *testing.T) {
	tests := []struct {
		name    string
		opts    []RegisterOptions
		gone    []string
		present []string
	}{
		{
			name:    "no options registers everything",
			opts:    nil,
			present: []string{"gitlab://project/{project_id}", "gitlab://user/current"},
		},
		{
			name:    "empty exclusion list registers everything",
			opts:    []RegisterOptions{{}},
			present: []string{"gitlab://project/{project_id}", "gitlab://user/current"},
		},
		{
			name:    "excluding a detail action removes its template",
			opts:    []RegisterOptions{{ExcludedActions: []string{"project.get"}}},
			gone:    []string{"gitlab://project/{project_id}"},
			present: []string{"gitlab://project/{project_id}/issues", "gitlab://user/current"},
		},
		{
			name:    "excluding a static resource's action removes the static resource",
			opts:    []RegisterOptions{{ExcludedActions: []string{"user.current"}}},
			gone:    []string{"gitlab://user/current"},
			present: []string{"gitlab://groups"},
		},
		{
			name:    "one of several backing actions is enough",
			opts:    []RegisterOptions{{ExcludedActions: []string{"repository.file_raw"}}},
			gone:    []string{"gitlab://project/{project_id}/file/{ref}/{+path}"},
			present: []string{"gitlab://project/{project_id}/commit/{sha}"},
		},
		{
			name: "several exclusions compose",
			opts: []RegisterOptions{{ExcludedActions: []string{"job.get", "job.list"}}},
			gone: []string{
				"gitlab://project/{project_id}/job/{job_id}",
				"gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs",
			},
			present: []string{"gitlab://project/{project_id}/pipeline/{pipeline_id}"},
		},
		{
			name:    "blank and unknown entries change nothing",
			opts:    []RegisterOptions{{ExcludedActions: []string{"", "   ", "not.an_action"}}},
			present: []string{"gitlab://project/{project_id}", "gitlab://user/current"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registered := registeredResourceURIs(nil, tt.opts...)
			for _, uri := range tt.gone {
				if slices.Contains(registered, uri) {
					t.Errorf("resource %q is still registered after excluding %v", uri, tt.opts)
				}
			}
			for _, uri := range tt.present {
				if !slices.Contains(registered, uri) {
					t.Errorf("resource %q was removed by excluding %v, which does not name it", uri, tt.opts)
				}
			}
		})
	}
}

// TestRegister_ExcludedResourceIsAlsoAbsentFromTheHandlerIndex verifies that an
// excluded resource leaves neither entry point behind.
//
// The handler index is what a subscription re-reads through, so a resource
// dropped from the server but left in the index would still be subscribable and
// would still poll GitLab with the caller's token on a cadence — a worse
// outcome than the read it replaced, because nobody asked for it per poll.
func TestRegister_ExcludedResourceIsAlsoAbsentFromTheHandlerIndex(t *testing.T) {
	const excluded = "gitlab://project/{project_id}"
	opts := RegisterOptions{ExcludedActions: []string{"project.get"}}

	checks := []struct {
		name  string
		index func() HandlerIndex
	}{
		{"Register", func() HandlerIndex {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			return Register(server, nil, opts)
		}},
		{"NewHandlerIndex", func() HandlerIndex { return NewHandlerIndex(nil, opts) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			index := check.index()
			if _, ok := index[excluded]; ok {
				t.Errorf("%s index still carries %q", check.name, excluded)
			}
			if _, ok := index["gitlab://user/current"]; !ok {
				t.Errorf("%s index lost gitlab://user/current, which the exclusion does not name", check.name)
			}
			if _, err := index.Read(context.Background(), excluded, "gitlab://project/1"); err == nil {
				t.Errorf("%s index read %q after it was excluded", check.name, excluded)
			}
		})
	}
}

// TestRegister_ExcludedResourceIsNotListedByTheServer verifies the same thing
// on the wire: a client's resources/list and resources/templates/list must not
// offer what the operator excluded, since a listed resource is one a model will
// try to read.
func TestRegister_ExcludedResourceIsNotListedByTheServer(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	Register(server, nil, RegisterOptions{ExcludedActions: []string{"project.get", "user.current"}})

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "probe", Version: "0.0.1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	resourceList, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources() error = %v", err)
	}
	templateList, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates() error = %v", err)
	}

	tests := []struct {
		name string
		uris []string
		gone string
		kept string
	}{
		{
			name: "static resources",
			uris: func() []string {
				out := make([]string, 0, len(resourceList.Resources))
				for _, resource := range resourceList.Resources {
					out = append(out, resource.URI)
				}
				return out
			}(),
			gone: "gitlab://user/current",
			kept: "gitlab://groups",
		},
		{
			name: "resource templates",
			uris: func() []string {
				out := make([]string, 0, len(templateList.ResourceTemplates))
				for _, template := range templateList.ResourceTemplates {
					out = append(out, template.URITemplate)
				}
				return out
			}(),
			gone: "gitlab://project/{project_id}",
			kept: "gitlab://project/{project_id}/issues",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if slices.Contains(tt.uris, tt.gone) {
				t.Errorf("%s still lists %q after exclusion", tt.name, tt.gone)
			}
			if !slices.Contains(tt.uris, tt.kept) {
				t.Errorf("%s no longer lists %q, which the exclusion does not name", tt.name, tt.kept)
			}
		})
	}
}

// TestResourceBackingActions_StaysAlignedWithBothSurfaces is the drift guard on
// the hand-kept overlap table.
//
// The table is the only thing relating a resource to the tool serving the same
// data, so it fails in two directions: a resource added to registerAll without
// an entry would silently escape every exclusion, and an action ID that no
// longer exists in the catalog would make an entry a dead string nobody
// notices. Neither is visible from either package alone.
func TestResourceBackingActions_StaysAlignedWithBothSurfaces(t *testing.T) {
	catalog, err := gitlabtools.BuildActionCatalog(nil, gitlabtools.ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	t.Run("every registered resource is classified", func(t *testing.T) {
		for _, uri := range registeredResourceURIs(nil) {
			if _, ok := resourceBackingActions[uri]; !ok {
				t.Errorf("resource %q has no entry in resourceBackingActions, so --exclude-tools cannot reach it", uri)
			}
		}
	})

	t.Run("no entry describes a resource that is gone", func(t *testing.T) {
		registered := registeredResourceURIs(nil)
		for uri := range resourceBackingActions {
			if !slices.Contains(registered, uri) {
				t.Errorf("resourceBackingActions names %q, which registerAll no longer registers", uri)
			}
		}
	})

	t.Run("every named action exists in the catalog", func(t *testing.T) {
		for uri, backing := range resourceBackingActions {
			if len(backing) == 0 {
				t.Errorf("resource %q lists no backing action; give it one or state why it has none", uri)
				continue
			}
			for _, id := range backing {
				if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
					t.Errorf("resource %q names action %q, which is not in the action catalog", uri, id)
				}
			}
		}
	})
}
