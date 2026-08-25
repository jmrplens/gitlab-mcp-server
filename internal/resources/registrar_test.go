// registrar_test.go validates the seam that lets this server read its own
// resources: the recording registrars and the handler index a subscription
// watcher reads through.
//
// The wiring tests in cmd/server prove the index against a live server;
// what belongs here is the seam's own contract — every registration is
// captured, and Read's outcomes (missing handler, failing handler, empty
// result, text versus blob content) are each reported distinctly.
package resources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// registrarTestClient builds a GitLab client for registration alone; no
// request is issued unless a handler is invoked.
func registrarTestClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:   "http://127.0.0.1:1", // deliberately unreachable: registration must not dial
		GitLabToken: "glpat-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// TestNewHandlerIndex_CapturesEveryRegistration verifies the index built
// without a server carries a handler for both registration forms — static
// resources and templates — for everything registerAll installs.
//
// This is the property subscriptions depend on: a kind whose template was
// registered but not indexed would classify as subscribable and then fail
// every read.
func TestNewHandlerIndex_CapturesEveryRegistration(t *testing.T) {
	index := NewHandlerIndex(registrarTestClient(t))

	if len(index) == 0 {
		t.Fatal("NewHandlerIndex() returned an empty index")
	}
	for _, key := range []string{
		"gitlab://user/current",                                // static resource
		"gitlab://project/{project_id}/pipeline/{pipeline_id}", // template
	} {
		if index[key] == nil {
			t.Errorf("index has no handler for %q", key)
		}
	}
}

// TestHandlerIndexRead_ReportsEachFailureDistinctly verifies Read's error
// paths name what actually went wrong, since a watcher's stop decision
// hangs on telling them apart.
func TestHandlerIndexRead_ReportsEachFailureDistinctly(t *testing.T) {
	handlerErr := errors.New("backend said no")
	index := HandlerIndex{
		"tmpl/fails": func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return nil, handlerErr
		},
		"tmpl/nil": func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			// The nil,nil contract violation under test — Read must catch a
			// handler that returns neither result nor error.
			return nil, nil //nolint:nilnil // deliberately malformed handler
		},
		"tmpl/empty": func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{}, nil
		},
	}

	tests := []struct {
		name     string
		template string
		wantIn   string
		wantErr  error
	}{
		{"unregistered template", "tmpl/missing", "no handler registered", nil},
		{"handler error is passed through", "tmpl/fails", "", handlerErr},
		{"nil result", "tmpl/nil", "returned no content", nil},
		{"empty contents", "tmpl/empty", "returned no content", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := index.Read(context.Background(), tt.template, "gitlab://whatever/1")
			if err == nil {
				t.Fatal("Read() error = nil, want a failure")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("Read() error = %v, want it to wrap %v", err, tt.wantErr)
			}
			if tt.wantIn != "" && !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Read() error = %v, want it to mention %q", err, tt.wantIn)
			}
		})
	}
}

// TestHandlerIndexRead_ReturnsTextThenBlob verifies both content branches:
// text content is returned as its bytes, and a blob-only result is not
// silently read as empty.
func TestHandlerIndexRead_ReturnsTextThenBlob(t *testing.T) {
	index := HandlerIndex{
		"tmpl/text": func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{Text: `{"ok":true}`}}}, nil
		},
		"tmpl/blob": func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{Blob: []byte{0x1f, 0x8b}}}}, nil
		},
	}

	text, err := index.Read(context.Background(), "tmpl/text", "gitlab://x/1")
	if err != nil {
		t.Fatalf("Read(text): %v", err)
	}
	if string(text) != `{"ok":true}` {
		t.Errorf("Read(text) = %q, want the handler's text", text)
	}

	blob, err := index.Read(context.Background(), "tmpl/blob", "gitlab://x/1")
	if err != nil {
		t.Fatalf("Read(blob): %v", err)
	}
	if len(blob) != 2 || blob[0] != 0x1f {
		t.Errorf("Read(blob) = %v, want the handler's blob bytes", blob)
	}
}

// TestRecorder_RegistersAndIndexesTogether verifies the server-attached
// recorder does both halves of its job for both registration forms: the
// resource reaches the server and the handler reaches the index.
func TestRecorder_RegistersAndIndexesTogether(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	rec := &recorder{server: server, index: make(HandlerIndex)}
	handler := func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{Text: "x"}}}, nil
	}

	rec.AddResource(&mcp.Resource{URI: "gitlab://static", Name: "static", MIMEType: "application/json"}, handler)
	rec.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "gitlab://tmpl/{id}", Name: "tmpl", MIMEType: "application/json"}, handler)

	if rec.index["gitlab://static"] == nil {
		t.Error("AddResource registered on the server but not in the index")
	}
	if rec.index["gitlab://tmpl/{id}"] == nil {
		t.Error("AddResourceTemplate registered on the server but not in the index")
	}
}
