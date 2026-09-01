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
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/subscriptions"
)

// registrarTestClient builds a GitLab client for registration alone; no
// request is issued unless a handler is invoked.
func registrarTestClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:      "http://127.0.0.1:1", // deliberately unreachable: registration must not dial
		GitLabToken:    "glpat-test",
		DisableRetries: true,
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
		t.Run(key, func(t *testing.T) {
			if index[key] == nil {
				t.Errorf("index has no handler for %q", key)
			}
		})
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

	tests := []struct {
		name     string
		template string
		want     []byte
	}{
		{"text content", "tmpl/text", []byte(`{"ok":true}`)},
		{"blob content", "tmpl/blob", []byte{0x1f, 0x8b}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := index.Read(context.Background(), tt.template, "gitlab://x/1")
			if err != nil {
				t.Fatalf("Read(%s): %v", tt.template, err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("Read(%s) = %q, want %q", tt.template, got, tt.want)
			}
		})
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

// TestAddResourceTemplate_SubscribableTemplates_MatchWhitelistExactly
// verifies the recorder annotates exactly the subscribable templates: every
// template in the subscription whitelist ends with the marker sentence and
// carries the vendor _meta key, and no other template does. Both markers
// are appended mechanically from subscriptions.Templates(), so this is the
// drift guard between what a client reads and the whitelist the
// SubscribeHandler enforces — the hand-written predecessor of the sentence
// covered 3 of 26 templates.
func TestAddResourceTemplate_SubscribableTemplates_MatchWhitelistExactly(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	result, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(result.ResourceTemplates) == 0 {
		t.Fatal("no resource templates registered")
	}

	subscribable := make(map[string]bool)
	for _, tmpl := range subscriptions.Templates() {
		subscribable[tmpl] = true
	}

	seen := 0
	for _, tmpl := range result.ResourceTemplates {
		want := subscribable[tmpl.URITemplate]
		if want {
			seen++
		}
		t.Run(tmpl.URITemplate, func(t *testing.T) {
			if marked := strings.HasSuffix(tmpl.Description, subscribableMarker); marked != want {
				t.Errorf("description marker = %t, want %t", marked, want)
			}
			if metaMarked, _ := tmpl.Meta[subscribableMetaKey].(bool); metaMarked != want {
				t.Errorf("%s meta key = %t, want %t", subscribableMetaKey, metaMarked, want)
			}
		})
	}
	if seen != len(subscribable) {
		t.Errorf("registered %d subscribable templates, whitelist has %d", seen, len(subscribable))
	}
}
