//go:build httpe2e

// exclusion_test.go pins that --exclude-tools removes an action from every
// request path that serves it, not only from the tool surface.
//
// The tool surface is one of three ways to the same GitLab object with the
// same credential. A resource template returns the identical project, and a
// subscription polls for it on a schedule. internal/resources has carried the
// narrowing mechanism since the security work and cmd/server passed it nothing
// for a while, which is the worst of the three possible states: the package's
// own tests passed and the guard was absent. Nothing inside either package can
// see that, because each is correct on its own and the defect is entirely in
// what connects them.
//
// Exclusion is also the mitigation this project recommends when an operator
// wants an action gone. A mitigation that covers one of the ways to reach it
// is worse than none, because it is believed.
package httpe2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The two resources the cases below use. The first is the one an exclusion of
// project.get must take away; the second is its sibling, which must survive,
// because a narrowing that removed the whole resource surface would satisfy
// every other assertion here.
const (
	exclusionProjectURI  = "gitlab://project/42"
	exclusionIssuesURI   = "gitlab://project/42/issues"
	exclusionProjectTmpl = "gitlab://project/{project_id}"
	// exclusionProjectQuoted is the template with its closing quote, because
	// every sibling template starts with the same prefix: a substring search
	// for the bare form matches gitlab://project/{project_id}/issues too, and
	// would report the exclusion as ineffective whatever it did.
	exclusionProjectQuoted = exclusionProjectTmpl + `"`
)

// exclusionGitLab is a fake instance that records which project endpoints a
// resource read actually reached.
//
// Arrival is half the finding. An excluded resource that is still registered
// does not merely answer the client: it sends this deployment's credential to
// GitLab for an object the operator believed they had retired, and a
// subscription would go on doing so every few minutes.
type exclusionGitLab struct {
	url string

	mu       sync.Mutex
	requests []string
}

// startExclusionGitLab serves the startup probes, one project and its issues.
func startExclusionGitLab(t *testing.T) *exclusionGitLab {
	t.Helper()

	fake := &exclusionGitLab{}
	record := func(r *http.Request) {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		fake.requests = append(fake.requests, r.URL.Path)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"someone","name":"Some One","state":"active"}`))
	})
	mux.HandleFunc("/api/v4/projects/42/issues", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"title":"an issue","state":"opened"}]`))
	})
	mux.HandleFunc("/api/v4/projects/42", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"name":"widgets","path_with_namespace":"acme/widgets"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	fake.url = srv.URL
	return fake
}

// seen returns the project endpoints that were asked for.
func (f *exclusionGitLab) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

// exclusionRead issues one resources/read and reports whether the server
// served the resource, along with the answer for a failure message.
//
// The status is part of the answer rather than a precondition: an unregistered
// resource is refused with a 400 carrying a JSON-RPC error, and a helper that
// insisted on 200 would fail the case for the very behavior it is asserting.
func exclusionRead(t *testing.T, srv *server, uri string) (served bool, answer string) {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"` + uri +
		`","_meta":{"io.modelcontextprotocol/protocolVersion":"` + protocolVersion +
		`","io.modelcontextprotocol/clientCapabilities":{}}}}`
	got := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: body,
		headers: map[string]string{
			"PRIVATE-TOKEN": "glpat-whatever",
			// Protocol 2026-07-28 makes Mcp-Name required for resources/read
			// as well as for tools/call, and carries the URI in it. Without
			// it the transport answers -32020 before any handler runs, which
			// looks like a refusal and is not one.
			"Mcp-Name": uri,
		},
	})
	payload := jsonRPCPayload(t, got.body)
	answer = fmt.Sprintf("HTTP %d: %s", got.status, truncate(payload))
	if got.status != http.StatusOK {
		return false, answer
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("resources/read %s answered something that is not JSON-RPC: %v: %s", uri, err, truncate(payload))
	}
	return decoded["error"] == nil, answer
}

// exclusionTemplates returns the body of a resources/templates/list call.
//
// Templates are listed by their own method rather than by resources/list: the
// project resource is a template, so a test that looked only at resources/list
// would find it absent whether it was registered or not.
func exclusionTemplates(t *testing.T, srv *server) string {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":1,"method":"resources/templates/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"` +
		protocolVersion + `","io.modelcontextprotocol/clientCapabilities":{}}}}`
	got := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: body,
		headers: map[string]string{"PRIVATE-TOKEN": "glpat-whatever"},
	})
	if got.status != http.StatusOK {
		t.Fatalf("resources/templates/list = %d, want 200: %s", got.status, truncate(got.body))
	}
	return jsonRPCPayload(t, got.body)
}

// TestExclusion_AnExcludedActionIsUnreachableThroughResources excludes one
// canonical action and asserts the resource serving the same object goes with
// it, while its siblings do not.
//
// project.get is named by its canonical ID rather than by a group, so what is
// removed is exactly one action. That makes the sibling assertion meaningful:
// gitlab://project/42/issues is served by issue.list, which nobody excluded,
// and it must still answer. A narrowing that took the resource surface down
// wholesale, or one that took nothing, both fail here.
func TestExclusion_AnExcludedActionIsUnreachableThroughResources(t *testing.T) {
	gitlab := startExclusionGitLab(t)
	srv := startServer(t,
		map[string]string{"GITLAB_MCP_EXCLUDE_TOOLS": "project.get"},
		"--gitlab-url="+gitlab.url)

	t.Run("the template is no longer offered", func(t *testing.T) {
		if templates := exclusionTemplates(t, srv); strings.Contains(templates, exclusionProjectQuoted) {
			t.Errorf("%s is still advertised despite the exclusion: %s", exclusionProjectTmpl, truncate(templates))
		}
	})

	t.Run("reading it is refused and reaches no GitLab", func(t *testing.T) {
		before := len(gitlab.seen())

		served, answer := exclusionRead(t, srv, exclusionProjectURI)
		if served {
			t.Fatalf("an excluded resource was still readable: %s", answer)
		}
		if after := gitlab.seen(); len(after) != before {
			t.Errorf("the refused read still spent the deployment's credential on GitLab: %v", after[before:])
		}
	})

	t.Run("a sibling resource is untouched", func(t *testing.T) {
		if served, answer := exclusionRead(t, srv, exclusionIssuesURI); !served {
			t.Fatalf("excluding project.get also removed %s, which no operator asked for: %s",
				exclusionIssuesURI, answer)
		}
	})
}

// TestExclusion_WithoutItTheSameResourceIsServed is the control for the case
// above, and it is not optional.
//
// Every assertion there is satisfied by a server that serves no resources at
// all, by one whose fake GitLab answers nothing, and by a URI nobody ever
// registered. Reading the identical URI from an identical deployment that
// excluded nothing is what tells those apart from the narrowing actually
// working.
func TestExclusion_WithoutItTheSameResourceIsServed(t *testing.T) {
	gitlab := startExclusionGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	if templates := exclusionTemplates(t, srv); !strings.Contains(templates, exclusionProjectQuoted) {
		t.Fatalf("%s is not offered even without an exclusion, so the case above proves nothing: %s",
			exclusionProjectTmpl, truncate(templates))
	}

	if served, answer := exclusionRead(t, srv, exclusionProjectURI); !served {
		t.Fatalf("reading %s failed without any exclusion: %s", exclusionProjectURI, answer)
	}
	if seen := gitlab.seen(); len(seen) == 0 {
		t.Error("the successful read never reached GitLab, so the arrival assertion above is vacuous")
	}
}
