//go:build stdioe2e

// token_scope_test.go drives the narrowing a token's scope imposes over the
// real binary on stdio: a read_api token is served what read_api reaches, the
// way the HTTP pool already served it per entry, and the dynamic surface names
// the scope the token lacks as the reason a write action is withheld. Scope detection is
// stdio startup configuration, which is exactly what the in-process suites
// cannot see.
package stdioe2e

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
)

// resultText joins the text blocks of a tools/call result.
func resultText(t *testing.T, got map[string]any) string {
	t.Helper()
	result, ok := got["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/call result missing: %v", got)
	}
	content, _ := result["content"].([]any)
	var b strings.Builder
	for _, raw := range content {
		if block, isMap := raw.(map[string]any); isMap {
			if text, _ := block["text"].(string); text != "" {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

// readAPILogLine is what the process logs when it narrows a token carrying
// read_api and not api.
const readAPILogLine = "token carries read_api without api; serving the actions GitLab accepts from read_api"

// TestTokenScope_ReadAPITokenIsServedWhatReadAPIReaches verifies that stdio
// narrows the catalog to what the token can call, as HTTP mode does per pool
// entry: with read_api the individual surface lists the issue reads and the
// package download, which writes only a local file and which GitLab serves to
// read_api, and neither the issue creation nor the CI lint, a read GitLab
// refuses read_api, nor a guided creation flow; and the log says why. The api
// token beside it is the control that proves the removals come from the scope
// and not from something else.
func TestTokenScope_ReadAPITokenIsServedWhatReadAPIReaches(t *testing.T) {
	tests := []struct {
		name     string
		scopes   []string
		env      map[string]string
		narrowed bool
	}{
		{name: "read_api", scopes: []string{"read_api"}, narrowed: true},
		{name: "api", scopes: []string{"api"}},
		{name: "read_api with scope detection ignored", scopes: []string{"read_api"}, env: map[string]string{"GITLAB_MCP_IGNORE_SCOPES": "true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := startFakeGitLab(t)
			fake.scopes = tt.scopes
			env := baseEnv(fake.URL)
			env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
			maps.Copy(env, tt.env)
			s := startSession(t, env)

			names := toolNames(t, s.call(t, request(1, "tools/list", "")))
			if !contains(names, "gitlab_issue_get") || !contains(names, "gitlab_package_download") {
				t.Errorf("gitlab_issue_get or gitlab_package_download is not listed: what read_api reaches must stay whatever the scope")
			}
			for _, name := range []string{"gitlab_issue_create", "gitlab_ci_lint", "gitlab_interactive_issue_create"} {
				t.Run(name, func(t *testing.T) {
					if listed := contains(names, name); listed == tt.narrowed {
						t.Errorf("%s listed = %v with scopes %v, want %v", name, listed, tt.scopes, !tt.narrowed)
					}
				})
			}
			if logged := strings.Contains(s.stderrText(), readAPILogLine); logged != tt.narrowed {
				t.Errorf("startup log says the token carries read_api without api = %v, want %v\nstderr: %s", logged, tt.narrowed, s.stderrText())
			}
		})
	}
}

// belowMinimumPrefix is the stable text a stdio process refuses every catalog
// method with while its token is below the admission minimum (register row
// ADM-001).
const belowMinimumPrefix = "GitLab accepted the token this server was started with"

// belowMinimumLogLine is what the process logs, once, when it learns that.
const belowMinimumLogLine = "the token carries neither read_api nor api"

// TestTokenScope_TokenBelowTheMinimum_RefusesEveryCatalogMethod verifies, over
// the real binary, what issue 952 decided for stdio: a token GitLab accepted
// that carries neither read_api nor api keeps the process up and its handshake
// answered, and every catalog method is refused in-band with -40300 and the way
// out. A process that exited instead would read to the client as a crash with
// the reason on stderr alone, and one that listed tools would offer calls
// GitLab answers with nothing. The rows with scope detection ignored hold that
// --ignore-scopes does not exempt a token from the minimum: it skips the scope
// filter, and the minimum is not a filter. self_rotate is the token GitLab
// refuses the version too, as 19.4.1 does, which a start must read as an
// instance that answered rather than one it could not reach.
func TestTokenScope_TokenBelowTheMinimum_RefusesEveryCatalogMethod(t *testing.T) {
	ignored := map[string]string{"GITLAB_MCP_IGNORE_SCOPES": "true"}
	tests := []struct {
		name  string
		scope string
		env   map[string]string
	}{
		{name: "read_user", scope: "read_user"},
		{name: "read_user with scope detection ignored", scope: "read_user", env: ignored},
		{name: "self_rotate", scope: "self_rotate"},
		{name: "self_rotate with scope detection ignored", scope: "self_rotate", env: ignored},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := startFakeGitLab(t)
			fake.scopes = []string{tt.scope}
			fake.versionScopeRefused = tt.scope != "read_user"
			env := baseEnv(fake.URL)
			maps.Copy(env, tt.env)
			assertServesNothingBelowTheMinimum(t, startSession(t, env))
		})
	}
}

// assertServesNothingBelowTheMinimum holds a session whose token is below the
// admission minimum to what issue 952 decided for stdio: the handshake and
// ping answered, every catalog method asked refused with -40300 and the
// sentence naming the minimum, the verdict logged as such and not as an
// instance the start could not reach, and the process still up.
func assertServesNothingBelowTheMinimum(t *testing.T, s *session) {
	t.Helper()

	if handshake := s.call(t, legacyInitialize(1)); handshake["error"] != nil {
		t.Fatalf("the handshake was refused: %v", handshake["error"])
	}
	calls := []struct{ method, params string }{
		{method: "tools/list"},
		{method: "tools/call", params: `{"name":"gitlab_find_action","arguments":{"query":"list issues"}}`},
		{method: "resources/list"},
		{method: "resources/read", params: `{"uri":"gitlab://tools"}`},
		{method: "prompts/list"},
	}
	for i, call := range calls {
		t.Run(call.method, func(t *testing.T) {
			got := s.call(t, legacyRequest(2+i, call.method, call.params))
			if code, refused := errorCode(got); !refused || code != tenancy.CodeForbidden {
				t.Fatalf("%s was answered %v, want a refusal with code %d", call.method, got, tenancy.CodeForbidden)
			}
			message, _ := got["error"].(map[string]any)["message"].(string)
			if !strings.HasPrefix(message, belowMinimumPrefix) {
				t.Errorf("%s was refused with %q, want the sentence beginning %q", call.method, message, belowMinimumPrefix)
			}
		})
	}
	if pong := s.call(t, legacyRequest(20, "ping", "")); pong["error"] != nil {
		t.Errorf("ping was refused: %v", pong["error"])
	}
	s.waitForStderr(t, belowMinimumLogLine, 5*time.Second)
	// Nothing on stderr may say the opposite of the refusal: an instance it
	// could not reach, a start that will recover, or a catalog served whole.
	for _, contradiction := range []string{"connectivity check failed", "degraded mode", "all tools will be registered"} {
		if strings.Contains(s.stderrText(), contradiction) {
			t.Errorf("the start logged %q for a token it refuses:\n%s", contradiction, s.stderrText())
		}
	}
	if !s.alive() {
		t.Errorf("the process ended: %s", s.exitStatus())
	}
}

// TestTokenScope_FineGrainedTokenIsNotReadOnly verifies that a fine-grained
// personal access token, whose scope list is the single value granular, is
// served as unknown authority rather than as a token that cannot write: every
// write stays listed, the groups that need admin_mode stay listed, and the log
// never says the token was narrowed to what read_api reaches. GitLab judges
// each call against the permissions the token was granted, and a write it
// lacks is GitLab's own 403.
//
// The api row is the control for the admin tool: a classic token without
// admin_mode loses it, so its presence under the fine-grained token is the scope
// filter reading that list as unknown and not a surface that never had it.
func TestTokenScope_FineGrainedTokenIsNotReadOnly(t *testing.T) {
	tests := []struct {
		name      string
		scopes    []string
		wantAdmin bool
	}{
		{name: "fine-grained", scopes: []string{"granular"}, wantAdmin: true},
		{name: "api without admin_mode", scopes: []string{"api"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := startFakeGitLab(t)
			fake.scopes = tt.scopes
			env := baseEnv(fake.URL)
			env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
			s := startSession(t, env)

			names := toolNames(t, s.call(t, request(1, "tools/list", "")))
			if !contains(names, "gitlab_issue_create") {
				t.Errorf("gitlab_issue_create is not listed with scopes %v: the token was served a read-only surface", tt.scopes)
			}
			if listed := contains(names, "gitlab_get_settings"); listed != tt.wantAdmin {
				t.Errorf("gitlab_get_settings listed = %v, want %v with scopes %v", listed, tt.wantAdmin, tt.scopes)
			}
			if strings.Contains(s.stderrText(), readAPILogLine) {
				t.Errorf("startup log says the token carries read_api without api with scopes %v\nstderr: %s", tt.scopes, s.stderrText())
			}
		})
	}
}

// TestTokenScope_FineGrainedTokenIsListedWhatUnknownScopesAreLessWhatNoGrantReaches
// verifies the reading itself rather than two tools of it: on the individual
// surface a fine-grained token is listed what a token whose scopes could not be
// detected is listed, tool for tool and descriptor for descriptor, less the
// tools whose action no fine-grained token can reach at the recorded GitLab
// version (issue 952, phase A). That listing is the catalog no scope narrowed,
// so a partial narrowing on the strength of the fine-grained list, one write or
// one admin_mode group removed, shows as a missing tool that is not withheld,
// and is named; every tool left out is called, and must answer that its action
// is withheld from a fine-grained token, naming an action the generated table
// denies.
//
// The unknown-scope session is the fake instance with its self endpoint left
// unanswered, which is what DetectToken reads as unknown; the write and the
// admin tool checked on it hold that the reference is the unfiltered listing
// and not one that happens to be narrowed the same way.
func TestTokenScope_FineGrainedTokenIsListedWhatUnknownScopesAreLessWhatNoGrantReaches(t *testing.T) {
	unknown := individualListing(t, nil)
	fineGrained := individualListing(t, []string{"granular"})

	for _, name := range []string{"gitlab_issue_create", "gitlab_get_settings"} {
		t.Run("reference lists "+name, func(t *testing.T) {
			if _, listed := unknown[name]; !listed {
				t.Errorf("%s is not listed with unknown scopes, so the reference listing is not the unfiltered one", name)
			}
		})
	}

	var missing, extra, differ []string
	for name, tool := range unknown {
		other, listed := fineGrained[name]
		switch {
		case !listed:
			missing = append(missing, name)
		case !reflect.DeepEqual(tool, other):
			differ = append(differ, name)
		}
	}
	for name := range fineGrained {
		if _, listed := unknown[name]; !listed {
			extra = append(extra, name)
		}
	}
	if len(extra)+len(differ) > 0 {
		t.Errorf("the fine-grained listing is not the unknown-scope listing of %d tools less some: extra %s, described differently %s",
			len(unknown), firstNames(extra), firstNames(differ))
	}
	if len(missing) == 0 {
		t.Fatal("the fine-grained listing withholds nothing, while the generated table denies actions this surface registers")
	}

	fake := startFakeGitLab(t)
	fake.scopes = []string{"granular"}
	env := baseEnv(fake.URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
	s := startSession(t, env)
	slices.Sort(missing)
	for i, name := range missing {
		t.Run("withheld "+name, func(t *testing.T) {
			text := resultText(t, s.call(t, request(100+i, "tools/call", `{"name":"`+name+`","arguments":{}}`)))
			id, rest, found := strings.Cut(strings.TrimPrefix(text, `action "`), `" `)
			if !found || !strings.HasPrefix(rest, withheldPrefix) {
				t.Fatalf("%s left the listing but its call is not answered as withheld: %q", name, text)
			}
			if row := actiongrants.Requirement(id); row == nil || row.Denied == nil {
				t.Errorf("%s was withheld as %s, which the generated table does not deny", name, id)
			}
		})
	}
}

// withheldPrefix is the stable text a phase A refusal carries after the action
// it names (register row AUT-007).
const withheldPrefix = "exists but is not available to a fine-grained personal access token: "

// TestTokenScope_FineGrainedToken_PhaseAWithholdsWithTheVersionNamed verifies,
// on the default surface, that an action no fine-grained token can reach is
// answered as withheld, with the reason, what GitLab does to it and the GitLab
// version the verdict is recorded at, never as an unknown action; that its
// detail in gitlab://tools carries the same answer as a withheld block instead
// of being not found; and that a token whose scopes are unknown is answered
// nothing of the kind.
func TestTokenScope_FineGrainedToken_PhaseAWithholdsWithTheVersionNamed(t *testing.T) {
	const want = `gitlab_execute_action: action "custom_emoji.list" ` + withheldPrefix +
		`GitLab 19.4.1 declares no fine-grained permission on the GraphQL type CustomEmoji this action reads, ` +
		`and removes the items from such a list.`
	call := `{"name":"gitlab_execute_action","arguments":{"action":"custom_emoji.list","params":{"group_path":"g"}}}`

	fake := startFakeGitLab(t)
	fake.scopes = []string{"granular"}
	s := startSession(t, baseEnv(fake.URL))
	if text := resultText(t, s.call(t, request(1, "tools/call", call))); !strings.HasPrefix(text, want) {
		t.Errorf("custom_emoji.list under a fine-grained token = %q, want it to begin %q", text, want)
	}
	read := s.call(t, request(2, "resources/read", `{"uri":"gitlab://tools/custom_emoji.list"}`))
	result, _ := read["result"].(map[string]any)
	contents, _ := result["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("gitlab://tools/custom_emoji.list answered %v", read)
	}
	first, _ := contents[0].(map[string]any)
	var detail struct {
		Withheld *struct {
			Cause   string `json:"cause"`
			Message string `json:"message"`
		} `json:"withheld"`
	}
	if err := json.Unmarshal([]byte(first["text"].(string)), &detail); err != nil || detail.Withheld == nil {
		t.Fatalf("the detail of custom_emoji.list carries no withheld block: %v %v", err, first["text"])
	}
	if detail.Withheld.Cause != "graphql-type-undeclared" || !strings.Contains(want, detail.Withheld.Message[:40]) {
		t.Errorf("withheld block = %+v", detail.Withheld)
	}

	unknown := startSession(t, baseEnv(startFakeGitLab(t).URL))
	if text := resultText(t, unknown.call(t, request(1, "tools/call", call))); strings.Contains(text, "fine-grained") {
		t.Errorf("a token whose scopes are unknown was answered as a fine-grained one: %q", text)
	}
}

// TestTokenScope_FineGrainedToken_ADegradedStartIsJudgedOnceGitLabAnswers
// starts the binary against an instance that answers nothing yet, so the start
// cannot tell what kind of token it holds and serves it as a classic one. The
// first call once the instance is up recovers the client, and that recovery is
// what has the token's kind asked, at once rather than at the stdio timer's
// next round fifteen minutes on: within moments an action no fine-grained token
// can reach is answered withheld, with the reason, rather than sent to GitLab.
func TestTokenScope_FineGrainedToken_ADegradedStartIsJudgedOnceGitLabAnswers(t *testing.T) {
	fake := startFakeGitLab(t)
	fake.scopes = []string{"granular"}
	fake.down.Store(true)
	env := baseEnv(fake.URL)
	// No request is made at the start, so the first call's is the client's
	// first lazy initialization, which nothing has put into its cooldown.
	env["GITLAB_MCP_IGNORE_SCOPES"] = "true"
	s := startSession(t, env)
	listing := s.call(t, request(1, "tools/list", ""))
	if _, listed := listing["result"]; !listed {
		t.Fatalf("tools/list on a degraded start = %v", listing)
	}

	fake.down.Store(false)
	recovering := `{"name":"gitlab_execute_action","arguments":{"action":"project.get","params":{"project_id":"42"}}}`
	if text := resultText(t, s.call(t, request(2, "tools/call", recovering))); strings.Contains(text, "fine-grained") {
		t.Fatalf("the call that recovers the client was answered as a fine-grained session's: %q", text)
	}

	withheld := `{"name":"gitlab_execute_action","arguments":{"action":"custom_emoji.list","params":{"group_path":"g"}}}`
	deadline := time.Now().Add(10 * time.Second)
	for id := 3; ; id++ {
		text := resultText(t, s.call(t, request(id, "tools/call", withheld)))
		if strings.Contains(text, withheldPrefix) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("custom_emoji.list was still not withheld %s after the instance answered: %q", 10*time.Second, text)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestTokenScope_FineGrainedToken_TheFirstListingIsAlreadyNarrowed verifies
// that a tools/list sent while the catalog is still being prepared, which the
// server holds until it is ready, is answered narrowed for a fine-grained
// token: the authority is attached before the server opens, since a client may
// keep that first listing for the listing's cache lifetime. The instance holds
// the version the startup asks first until the listing has been sent, so the
// listing is in the server before the catalog is.
func TestTokenScope_FineGrainedToken_TheFirstListingIsAlreadyNarrowed(t *testing.T) {
	fake := startFakeGitLab(t)
	fake.scopes = []string{"granular"}
	fake.versionHold = make(chan struct{})
	env := baseEnv(fake.URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
	s := startSession(t, env)

	s.send(t, request(1, "tools/list", ""))
	close(fake.versionHold)
	got := s.readMessage(t, registrationTimeout)
	result, ok := got["result"].(map[string]any)
	if !ok {
		t.Fatalf("the first tools/list answered no result: %v", got)
	}
	tools, _ := result["tools"].([]any)
	for _, entry := range tools {
		tool, _ := entry.(map[string]any)
		if tool["name"] == "gitlab_list_custom_emoji" {
			t.Fatal("the first listing, held while the catalog was prepared, lists gitlab_list_custom_emoji, which no fine-grained token can run")
		}
	}
	if len(tools) == 0 {
		t.Fatal("the first listing lists nothing")
	}
}

// TestTokenScope_FineGrainedToken_AnExcludedStandaloneToolStaysAbsent verifies
// that an operator's exclusion of a standalone utility still removes it for a
// fine-grained token: the exclusion pass runs over the whole registered
// surface at registration, which the fine-grained listing filter never narrows.
func TestTokenScope_FineGrainedToken_AnExcludedStandaloneToolStaysAbsent(t *testing.T) {
	fake := startFakeGitLab(t)
	fake.scopes = []string{"granular"}
	env := baseEnv(fake.URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = "meta"
	env["GITLAB_MCP_EXCLUDE_TOOLS"] = "gitlab_discover_project"
	s := startSession(t, env)

	names := toolNames(t, s.call(t, request(1, "tools/list", "")))
	if contains(names, "gitlab_discover_project") {
		t.Error("gitlab_discover_project is listed to a fine-grained token although the operator excluded it")
	}
	if !contains(names, "gitlab_interactive_issue_create") {
		t.Error("gitlab_interactive_issue_create is missing, so the listing is not the meta surface less the exclusion")
	}
}

// metadataReadRefusal is GitLab's sentence for a fine-grained token not
// granted Metadata: Read, which GET /api/v4/version declares at the instance
// boundary (lib/api/metadata.rb at v19.4.1-ee).
const metadataReadRefusal = "Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [Metadata: Read]."

// TestTokenScope_FineGrainedTokenWithoutMetadataRead_StartsWhole verifies the
// start of a fine-grained token GitLab refuses the instance version and
// allows the rest: the server is not degraded, says once at WARN which
// permission GitLab named, asks the version endpoint once over twenty calls
// (it used to re-ask once per thirty seconds of activity for the life of the
// process), and still detects the tier from the namespace plan GitLab.com
// reports, which it never reached before because a refused version was read
// as an unreachable instance. The row without a plan is the control: the
// Premium tool is listed because the plan was read, and not because the
// surface always lists it.
func TestTokenScope_FineGrainedTokenWithoutMetadataRead_StartsWhole(t *testing.T) {
	tests := []struct {
		name        string
		plan        string
		wantPremium bool
	}{
		{name: "a paid namespace plan", plan: "ultimate", wantPremium: true},
		{name: "no namespace plan", plan: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := startFakeGitLab(t)
			fake.scopes = []string{"granular"}
			fake.versionRefusal = metadataReadRefusal
			fake.namespacePlan = tt.plan
			env := baseEnv(fake.URL)
			env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
			s := startSession(t, env)

			names := toolNames(t, s.call(t, request(1, "tools/list", "")))
			if listed := contains(names, "gitlab_list_vulnerabilities"); listed != tt.wantPremium {
				t.Errorf("gitlab_list_vulnerabilities listed = %v, want %v with the namespace plan %q", listed, tt.wantPremium, tt.plan)
			}
			if !contains(names, "gitlab_issue_create") {
				t.Error("gitlab_issue_create is not listed: the fine-grained token was served a read-only surface")
			}
			assertVersionAskedOnceAndWarnedOnce(t, s, fake)
		})
	}
}

// assertVersionAskedOnceAndWarnedOnce makes twenty calls that reach GitLab and
// holds the session to one request of the version endpoint over all of them,
// and to one warning, naming Metadata: Read, with no degraded start.
func assertVersionAskedOnceAndWarnedOnce(t *testing.T, s *session, fake *fakeGitLab) {
	t.Helper()
	for id := 2; id <= 21; id++ {
		called := s.call(t, request(id, "tools/call", `{"name":"gitlab_user_current","arguments":{}}`))
		if called["error"] != nil {
			t.Fatalf("call %d failed: %v", id, called["error"])
		}
	}
	if got := fake.versionReads.Load(); got != 1 {
		t.Errorf("the version endpoint was asked %d times over twenty calls, want once", got)
	}

	stderr := s.stderrText()
	if got := strings.Count(stderr, "GitLab refused this token GET /api/v4/version"); got != 1 {
		t.Errorf("the refused version was warned about %d times, want once\nstderr: %s", got, stderr)
	}
	if !strings.Contains(stderr, "Metadata: Read") {
		t.Errorf("the warning does not name Metadata: Read\nstderr: %s", stderr)
	}
	if strings.Contains(stderr, "degraded mode") {
		t.Errorf("a reachable instance was called degraded\nstderr: %s", stderr)
	}
}

// individualListing starts a server on the individual surface against a fake
// instance whose self endpoint reports scopes, or is left unanswered when
// scopes is nil, and returns every tool it lists keyed by name, following the
// cursor to the last page so that no comparison is made on a prefix.
func individualListing(t *testing.T, scopes []string) map[string]any {
	t.Helper()
	fake := startFakeGitLab(t)
	fake.scopes = scopes
	env := baseEnv(fake.URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
	s := startSession(t, env)

	const maxPages = 50
	tools := map[string]any{}
	params := ""
	for id := 1; id <= maxPages; id++ {
		got := s.call(t, request(id, "tools/list", params))
		result, ok := got["result"].(map[string]any)
		if !ok {
			t.Fatalf("tools/list with scopes %v answered no result: %v", scopes, got)
		}
		page, _ := result["tools"].([]any)
		for _, entry := range page {
			tool, _ := entry.(map[string]any)
			name, _ := tool["name"].(string)
			tools[name] = tool
		}
		next, _ := result["nextCursor"].(string)
		if next == "" {
			return tools
		}
		cursor, err := json.Marshal(next)
		if err != nil {
			t.Fatalf("encoding the cursor %q: %v", next, err)
		}
		params = `{"cursor":` + string(cursor) + `}`
	}
	t.Fatalf("tools/list with scopes %v still had a next page after %d pages", scopes, maxPages)
	return nil
}

// firstNames renders a list of tool names for a failure message: its length
// and, sorted, at most the first ten, so a wholesale difference stays readable.
func firstNames(names []string) string {
	slices.Sort(names)
	return fmt.Sprintf("%d %v", len(names), names[:min(len(names), 10)])
}

// TestTokenScope_FineGrainedWriteReachesGitLab verifies the same on the default
// surface: a write asked for under a fine-grained token is dispatched to GitLab
// rather than withheld by the credential, so the answer comes from the instance
// and not from a scope the token never had to carry.
func TestTokenScope_FineGrainedWriteReachesGitLab(t *testing.T) {
	fake := startFakeGitLab(t)
	fake.scopes = []string{"granular"}
	s := startSession(t, baseEnv(fake.URL))

	got := s.call(t, request(1, "tools/call", `{"name":"gitlab_execute_action","arguments":{"action":"issue.create","params":{"project_id":"42","title":"sent to GitLab"}}}`))
	text := resultText(t, got)
	if strings.Contains(text, "and the credential in use does not carry") {
		t.Fatalf("issue.create under a fine-grained token was withheld by scope: %q", text)
	}
	if !served(got) {
		t.Fatalf("issue.create under a fine-grained token was not served: %q", text)
	}
	if created := fake.issuesCreated.Load(); created != 1 {
		t.Errorf("the instance was asked to create %d issues, want exactly 1", created)
	}
}

// TestTokenScope_DynamicSurfaceNamesTheTokenAsTheReason verifies the other
// half of the contract: on the default surface a write action asked for under
// a read_api token is reported as withheld by the credential, not as an action
// the server does not have, so a model reports the scope instead of a missing
// capability.
func TestTokenScope_DynamicSurfaceNamesTheTokenAsTheReason(t *testing.T) {
	fake := startFakeGitLab(t)
	fake.scopes = []string{"read_api"}
	s := startSession(t, baseEnv(fake.URL))

	got := s.call(t, request(1, "tools/call", `{"name":"gitlab_execute_action","arguments":{"action":"issue.create","params":{"project_id":"42","title":"never created"}}}`))
	text := resultText(t, got)
	if !strings.Contains(text, "GitLab requires the api scope for it and the credential in use does not carry it") {
		t.Fatalf("issue.create under read_api answered %q, want the scope named as the reason", text)
	}
	if strings.Contains(text, "unknown action") {
		t.Fatalf("issue.create under read_api was reported as unknown: %q", text)
	}
}

// workItemCreateGrant is a grant of Work Item: Create, which expands to
// create_issue, on every membership: it reaches issue.create and does not
// reach branch.create, which needs Branch: Create.
const workItemCreateGrant = `[{"access":"all_memberships","permissions":["create_work_item"]}]`

// phaseBBranchRefusal is how branch.create is refused to that grant, as the
// recorded release declares what it needs.
const phaseBBranchRefusal = `action "branch.create" exists but this fine-grained personal access token was not granted ` +
	`what it needs: the project permission [Branch: Create], as GitLab 19.4.1 declares it.`

// TestTokenScope_FineGrainedToken_PhaseBServesWhatTheGrantReaches verifies a
// fine-grained token that may read its grant, against an instance reporting
// the release the table records, is served what that grant reaches and
// nothing else: the individual listing holds the issue creation tool and not
// the branch creation one, a call to the latter is answered with the
// permission it needs in the words the token creation page offers, and a call
// to the former reaches GitLab. The grant is read once.
func TestTokenScope_FineGrainedToken_PhaseBServesWhatTheGrantReaches(t *testing.T) {
	fake := startFakeGitLab(t)
	fake.scopes = []string{"granular"}
	fake.version = "19.4.1-ee"
	fake.grantScopes = workItemCreateGrant
	env := baseEnv(fake.URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
	s := startSession(t, env)

	names := toolNames(t, s.call(t, request(1, "tools/list", "")))
	if !contains(names, "gitlab_issue_create") || contains(names, "gitlab_branch_create") {
		t.Fatalf("listing holds gitlab_issue_create %v and gitlab_branch_create %v; want only the first",
			contains(names, "gitlab_issue_create"), contains(names, "gitlab_branch_create"))
	}
	refused := resultText(t, s.call(t, request(2, "tools/call",
		`{"name":"gitlab_branch_create","arguments":{"project_id":"42","branch_name":"b","ref":"main"}}`)))
	if !strings.HasPrefix(refused, phaseBBranchRefusal) {
		t.Errorf("gitlab_branch_create = %q, want it to begin %q", refused, phaseBBranchRefusal)
	}
	served := s.call(t, request(3, "tools/call", `{"name":"gitlab_issue_create","arguments":{"project_id":"42","title":"sent"}}`))
	if text := resultText(t, served); strings.Contains(text, "fine-grained") || fake.issuesCreated.Load() != 1 {
		t.Errorf("gitlab_issue_create = %q with %d issues created; want it sent to GitLab once", text, fake.issuesCreated.Load())
	}
	if reads := fake.grantReads.Load(); reads != 1 {
		t.Errorf("the grant was read %d times, want once", reads)
	}
}

// TestTokenScope_FineGrainedToken_PhaseBTheFirstListingIsAlreadyNarrowed
// verifies a tools/list sent while the grant is still being read, which the
// server holds until the catalog is ready, is answered narrowed by the grant:
// it leaves out a tool phase A would have listed, so it is phase B's listing
// and not one taken before the grant was judged.
func TestTokenScope_FineGrainedToken_PhaseBTheFirstListingIsAlreadyNarrowed(t *testing.T) {
	fake := startFakeGitLab(t)
	fake.scopes = []string{"granular"}
	fake.version = "19.4.1-ee"
	fake.grantScopes = workItemCreateGrant
	fake.grantHold = make(chan struct{})
	env := baseEnv(fake.URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
	s := startSession(t, env)

	s.send(t, request(1, "tools/list", ""))
	close(fake.grantHold)
	names := toolNames(t, s.readMessage(t, registrationTimeout))
	if contains(names, "gitlab_branch_create") || !contains(names, "gitlab_issue_create") {
		t.Errorf("the first listing holds gitlab_branch_create %v and gitlab_issue_create %v; want only the second",
			contains(names, "gitlab_branch_create"), contains(names, "gitlab_issue_create"))
	}
}
