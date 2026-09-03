package resources

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/subscriptions"
)

// registrar is the subset of *mcp.Server that resource registration uses.
//
// The registration helpers take this interface rather than the concrete
// server so the same call can be observed as well as performed. That is
// what lets a resource subscription re-read a URI through the very handler
// the MCP router would dispatch to, instead of a second implementation that
// could drift from it: "the content changed" then means exactly "what
// resources/read returns changed", which is the only definition a
// subscriber can act on.
type registrar interface {
	AddResource(resource *mcp.Resource, handler mcp.ResourceHandler)
	AddResourceTemplate(template *mcp.ResourceTemplate, handler mcp.ResourceHandler)
}

// HandlerIndex maps a resource's URI template — or, for a static resource,
// its literal URI — to the handler registered for it.
type HandlerIndex map[string]mcp.ResourceHandler

// recorder registers resources onto a server while indexing their handlers.
type recorder struct {
	server *mcp.Server
	index  HandlerIndex
}

func (r *recorder) AddResource(resource *mcp.Resource, handler mcp.ResourceHandler) {
	r.server.AddResource(resource, handler)
	r.index[resource.URI] = handler
}

// subscribableMarker is the sentence every subscribable template's
// description ends with. Appended mechanically from the subscription
// whitelist rather than written into the 26 literals, so the prose can
// never disagree with the table that enforces it — the hand-written
// variant of this sentence covered 3 of 26 templates and named only the
// legacy method, which the default stateless HTTP deployment refuses.
const subscribableMarker = "Subscribable: subscriptions/listen (protocol 2026-07-28). Resources/subscribe on stateful sessions."

// subscribableMetaKey is the vendor-namespaced `_meta` key stating the same
// fact for machines. The description marker serves the model (models read
// descriptions); this serves generic clients, which can filter subscribable
// templates without knowing this server's manifest. `_meta` with a
// reverse-DNS key is the spec's sanctioned per-object extension point —
// the standard surface itself has no per-resource subscribable field, only
// the server-wide resources.subscribe capability.
const subscribableMetaKey = "io.github.jmrplens/subscribable"

// AddResourceTemplate registers a resource template on the server and
// records its handler in the index. Templates on the subscriptions
// whitelist are annotated first — the subscribable marker is appended to
// the description and the reverse-DNS _meta key is set — on a copy, so
// the shared registration literals are never mutated.
func (r *recorder) AddResourceTemplate(template *mcp.ResourceTemplate, handler mcp.ResourceHandler) {
	if slices.Contains(subscriptions.Templates(), template.URITemplate) {
		// Copy before annotating: the registration literals are shared
		// package state, and Register can run once per pooled server.
		annotated := *template
		annotated.Description = template.Description + " " + subscribableMarker
		meta := make(mcp.Meta, len(template.Meta)+1)
		maps.Copy(meta, template.Meta)
		meta[subscribableMetaKey] = true
		annotated.Meta = meta
		template = &annotated
	}
	r.server.AddResourceTemplate(template, handler)
	r.index[template.URITemplate] = handler
}

// indexRegistrar captures handlers without registering them anywhere.
type indexRegistrar struct {
	index HandlerIndex
}

func (r *indexRegistrar) AddResource(resource *mcp.Resource, handler mcp.ResourceHandler) {
	r.index[resource.URI] = handler
}

func (r *indexRegistrar) AddResourceTemplate(template *mcp.ResourceTemplate, handler mcp.ResourceHandler) {
	r.index[template.URITemplate] = handler
}

// NewHandlerIndex builds the same index [Register] returns, without
// registering anything on a server.
//
// This exists to break an ordering problem rather than to duplicate
// Register: mcp.ServerOptions — which carries the resource-subscription
// handlers — has to be built before mcp.NewServer returns a server to
// register onto. Resource handlers only close over the GitLab client, never
// the server, so the index can be built first and the real registration can
// happen later against the same client.
// It takes the same [RegisterOptions] as [Register] and must be given the same
// ones: this index is what a subscription re-reads through, so an excluded
// resource left in it would still be subscribable and still poll GitLab.
func NewHandlerIndex(client *gitlabclient.Client, opts ...RegisterOptions) HandlerIndex {
	r := &indexRegistrar{index: make(HandlerIndex)}
	registerAll(registrarFor(r, opts), client)
	return r.index
}

// Read invokes the handler registered for uriTemplate and returns the
// resource's content as the client would receive it.
//
// The SDK keeps its own URI-to-handler lookup unexported, so a server
// cannot read its own resources; this index is how that gap is closed
// without duplicating any read logic. Callers resolve uriTemplate from the
// concrete URI themselves — the subscription whitelist already parses these
// URIs and knows which template each one belongs to.
func (index HandlerIndex) Read(ctx context.Context, uriTemplate, uri string) ([]byte, error) {
	handler, ok := index[uriTemplate]
	if !ok {
		return nil, fmt.Errorf("resources: no handler registered for %q", uriTemplate)
	}
	result, err := handler(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: uri}})
	if err != nil {
		return nil, err
	}
	if result == nil || len(result.Contents) == 0 {
		return nil, fmt.Errorf("resources: handler for %q returned no content", uri)
	}
	// Resource handlers in this package all marshal JSON into Text; the
	// Blob branch is here so a future binary resource cannot silently read
	// as empty.
	first := result.Contents[0]
	if first.Text != "" {
		return []byte(first.Text), nil
	}
	return first.Blob, nil
}
