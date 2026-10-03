//go:build httpe2e

// fine_grained_test.go drives one process serving a classic token and a
// fine-grained one on one configuration shape (issue 952, phase A): the shape
// server is shared, so what the fine-grained credential is withheld has to be
// decided per request from the authority its own pool entry carries, and the
// classic credential beside it must be served the whole surface. A unit test
// over either middleware sees a correct middleware; only the wire shows the two
// credentials being told apart on one server.
package httpe2e

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// The two credentials, and the tools the assertions name: custom_emoji.list
// reads a GraphQL type GitLab 19.4.1 declares no fine-grained permission on,
// so no fine-grained token runs it, and issue.create is a REST write GitLab
// judges against the grant.
const (
	phaseAClassicToken = "glpat-classic-unknown-scopes"
	phaseAFineToken    = "glpat-fine-grained-phase-a"
	withheldTool       = "gitlab_list_custom_emoji"
	withheldAction     = "custom_emoji.list"
	servedWriteTool    = "gitlab_issue_create"
	phaseAWithheldFor  = `action "custom_emoji.list" exists but is not available to a fine-grained personal access token: ` +
		`GitLab 19.4.1 declares no fine-grained permission on the GraphQL type CustomEmoji this action reads`
)

// phaseAGitLab is a stand-in that knows one classic token, whose scopes it
// does not describe, and one fine-grained token, which it describes as GitLab
// does, and counts the issues each asked it to create.
type phaseAGitLab struct {
	URL     string
	created map[string]*atomic.Int32
}

func startPhaseAGitLab(t *testing.T) *phaseAGitLab {
	t.Helper()
	fake := &phaseAGitLab{created: map[string]*atomic.Int32{phaseAClassicToken: {}, phaseAFineToken: {}}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := 7
		if r.Header.Get("PRIVATE-TOKEN") == phaseAFineToken {
			id = 8
		}
		_, _ = fmt.Fprintf(w, `{"id":%d,"username":"user%d"}`, id, id)
	})
	mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != phaseAFineToken {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"name":"fine","active":true,"scopes":["granular"]}`))
	})
	mux.HandleFunc("POST /api/v4/projects/42/issues", func(w http.ResponseWriter, r *http.Request) {
		if counter, known := fake.created[r.Header.Get("PRIVATE-TOKEN")]; known {
			counter.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"project_id":42,"title":"sent","state":"opened","web_url":"http://example.invalid/g/p/-/issues/1"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	fake.URL = srv.URL
	return fake
}

// listedAs returns the names of the tools the server lists to token.
func listedAs(t *testing.T, srv *server, token string) []string {
	t.Helper()
	got := srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": token}))
	if got.status != http.StatusOK {
		t.Fatalf("tools/list as %s = %d: %s", token, got.status, got.body)
	}
	var msg struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(jsonRPCPayload(t, got.body)), &msg); err != nil {
		t.Fatalf("tools/list as %s is not JSON-RPC: %v", token, err)
	}
	names := make([]string, 0, len(msg.Result.Tools))
	for _, tool := range msg.Result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// callAs calls an individual tool as token and returns the result's text.
func callAs(t *testing.T, srv *server, token, tool, arguments string) string {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{%s,"name":%q,"arguments":%s}}`,
		protocolMeta, tool, arguments)
	got := srv.do(t, request{body: body, headers: map[string]string{"PRIVATE-TOKEN": token}})
	if got.status != http.StatusOK {
		t.Fatalf("tools/call %s as %s = %d: %s", tool, token, got.status, got.body)
	}
	text, _ := toolResultsText(t, jsonRPCPayload(t, got.body))
	return text
}

// legacyRevision is the revision before 2026-07-28, whose results carry no
// resultType.
const legacyRevision = "2025-11-25"

// callResultAs calls a tool as token the way a client of revision sends it and
// returns the JSON-RPC result decoded as a map, so an absent member can be told
// from an empty one. At 2026-07-28 the call carries the protocol's _meta and
// the extra headers that revision asks of its arguments; at 2025-11-25 it names
// its revision in the header alone and sends none of the headers 2026-07-28
// added, as callToolAtLegacyRevision does.
func callResultAs(t *testing.T, srv *server, revision, token, tool, arguments string, extra map[string]string) map[string]any {
	t.Helper()
	meta := protocolMeta + ","
	headers := map[string]string{"PRIVATE-TOKEN": token}
	maps.Copy(headers, extra)
	if revision == legacyRevision {
		meta = ""
		maps.Copy(headers, map[string]string{"MCP-Protocol-Version": legacyRevision, "Mcp-Method": "", "Mcp-Name": ""})
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{%s"name":%q,"arguments":%s}}`,
		meta, tool, arguments)
	return decodeToolCall(t, srv.do(t, request{body: body, headers: headers})).Result
}

// manifestEntriesAs reads gitlab://tools as token and returns its entry IDs.
func manifestEntriesAs(t *testing.T, srv *server, token string) []string {
	t.Helper()
	read := readResource(3, "gitlab://tools")
	read.headers["PRIVATE-TOKEN"] = token
	got := srv.do(t, read)
	if got.status != http.StatusOK {
		t.Fatalf("resources/read gitlab://tools as %s = %d: %s", token, got.status, got.body)
	}
	var msg struct {
		Result struct {
			Contents []struct {
				Text string `json:"text"`
			} `json:"contents"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(jsonRPCPayload(t, got.body)), &msg); err != nil || len(msg.Result.Contents) != 1 {
		t.Fatalf("gitlab://tools as %s: %v: %s", token, err, got.body)
	}
	var manifest struct {
		Entries []struct {
			ID string `json:"id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(msg.Result.Contents[0].Text), &manifest); err != nil {
		t.Fatalf("the manifest as %s is not JSON: %v", token, err)
	}
	ids := make([]string, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

// TestFineGrained_TwoCredentialsOnOneShape_PhaseAIsDecidedPerRequest verifies,
// on one process and one individual-surface shape, that a fine-grained
// credential is listed and shown in gitlab://tools only what some fine-grained
// token can reach while the classic credential beside it is served the whole
// surface; that the fine-grained one's call to a withheld tool is answered with
// the reason and the GitLab version it is recorded at, where the classic one's
// is not; and that the fine-grained one's call to a tool it may run reaches
// GitLab, which judges it against the grant.
func TestFineGrained_TwoCredentialsOnOneShape_PhaseAIsDecidedPerRequest(t *testing.T) {
	gitlab := startPhaseAGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--tool-surface=individual", "--capability-surface=full")

	classic := listedAs(t, srv, phaseAClassicToken)
	fine := listedAs(t, srv, phaseAFineToken)
	if !slices.Contains(classic, withheldTool) || !slices.Contains(classic, servedWriteTool) {
		t.Fatalf("the classic listing lacks %s or %s, so it is not the whole surface:\n%s", withheldTool, servedWriteTool, srv.logs())
	}
	if slices.Contains(fine, withheldTool) {
		t.Errorf("the fine-grained listing lists %s, which no fine-grained token can run", withheldTool)
	}
	if !slices.Contains(fine, servedWriteTool) {
		t.Errorf("the fine-grained listing lacks %s, which GitLab judges against the grant", servedWriteTool)
	}
	for _, name := range fine {
		if !slices.Contains(classic, name) {
			t.Errorf("the fine-grained listing lists %s, which the classic one does not", name)
		}
	}

	classicEntries := manifestEntriesAs(t, srv, phaseAClassicToken)
	fineEntries := manifestEntriesAs(t, srv, phaseAFineToken)
	if !slices.Contains(classicEntries, withheldTool) || slices.Contains(fineEntries, withheldTool) {
		t.Errorf("gitlab://tools lists %s to the classic credential %v and to the fine-grained one %v; want only the first",
			withheldTool, slices.Contains(classicEntries, withheldTool), slices.Contains(fineEntries, withheldTool))
	}

	if text := callAs(t, srv, phaseAFineToken, withheldTool, `{"group_path":"g"}`); !strings.Contains(text, phaseAWithheldFor) {
		t.Errorf("the fine-grained call to %s = %q, want the withheld answer naming %s", withheldTool, text, withheldAction)
	}
	if text := callAs(t, srv, phaseAClassicToken, withheldTool, `{"group_path":"g"}`); strings.Contains(text, "fine-grained") {
		t.Errorf("the classic call to %s was answered as a fine-grained one: %q", withheldTool, text)
	}

	callAs(t, srv, phaseAFineToken, servedWriteTool, `{"project_id":"42","title":"sent"}`)
	if got := gitlab.created[phaseAFineToken].Load(); got != 1 {
		t.Errorf("GitLab was asked for %d issues by the fine-grained credential, want 1: the write was not let through to it", got)
	}
}

// TestFineGrained_WithheldCall_EachRevision_CarriesTheResultTypeTheServedCallDoes
// holds the refusal phase A's call middleware makes of a fine-grained
// session's call to a withheld tool to the resultType the call the same
// credential is served just before it carries: "complete" at 2026-07-28,
// which that revision requires on every result, and none at 2025-11-25, which
// is what the SDK sends a client of that revision from its own dispatcher.
//
// The middleware answers before the SDK's dispatcher, which labels only what
// it answers, so the refusal goes out through toolutil.LabelForRevision, as the
// rate limiter's does (row 66 of docs/development/upstream-bugs.md). The
// served call is the control: it is what makes the refusal's label about who
// built the result rather than about the revision, the transport or the
// credential.
func TestFineGrained_WithheldCall_EachRevision_CarriesTheResultTypeTheServedCallDoes(t *testing.T) {
	cases := []struct {
		name string
		want any
	}{
		{name: protocolVersion, want: "complete"},
		{name: legacyRevision, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gitlab := startPhaseAGitLab(t)
			srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--tool-surface=individual")

			served := callResultAs(t, srv, tc.name, phaseAFineToken, servedWriteTool, `{"project_id":"42","title":"sent"}`, nil)
			if isError, _ := served["isError"].(bool); isError {
				t.Fatalf("the served call %s was refused: %v", servedWriteTool, served)
			}
			if got := served["resultType"]; got != tc.want {
				t.Fatalf("a served tools/call at %s carried resultType %v, want %v: the control is broken, so this case cannot say anything about the refusal", tc.name, got, tc.want)
			}

			refused := callResultAs(t, srv, tc.name, phaseAFineToken, withheldTool, `{"group_path":"g"}`, nil)
			if isError, _ := refused["isError"].(bool); !isError || !strings.Contains(refusalText(refused), phaseAWithheldFor) {
				t.Fatalf("the call to %s was not phase A's refusal, so this case is not about a middleware-made result: %v", withheldTool, refused)
			}
			if got := refused["resultType"]; got != tc.want {
				t.Errorf("phase A's refusal at %s carried resultType %v, want %v, the same as the call the dispatcher served before it",
					tc.name, got, tc.want)
			}
		})
	}
}

// TestFineGrained_WithheldExecute_ModernRevision_IsLabeledLikeAServedCall
// verifies the same refusal on the default surface, where gitlab_execute_action
// makes it inside its handler, which the SDK's dispatcher labels like any call
// it served, so at 2026-07-28 it carries resultType "complete", and its text is
// the withheld sentence after the tool's own name, as every refusal execute
// writes is.
func TestFineGrained_WithheldExecute_ModernRevision_IsLabeledLikeAServedCall(t *testing.T) {
	gitlab := startPhaseAGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.URL)

	arguments := fmt.Sprintf(`{"action":%q,"params":{"group_path":"g"}}`, withheldAction)
	refused := callResultAs(t, srv, protocolVersion, phaseAFineToken, "gitlab_execute_action", arguments,
		map[string]string{"Mcp-Param-Action": withheldAction})
	if isError, _ := refused["isError"].(bool); !isError || !strings.HasPrefix(refusalText(refused), "gitlab_execute_action: "+phaseAWithheldFor) {
		t.Fatalf("execute of %s = %v, want the withheld answer after the tool's name", withheldAction, refused)
	}
	if got, ok := refused["resultType"]; !ok || got != "complete" {
		t.Errorf("execute's refusal carried resultType %v (present: %t), want \"complete\": a refusal made in a handler goes out labeled", got, ok)
	}
}
