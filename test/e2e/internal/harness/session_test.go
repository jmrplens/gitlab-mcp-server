//go:build e2e

// session_test.go covers what a server shape is and what starting one proves.
//
// The configuration half is offline: a shape is a key, a label and a set of
// environment variables, and each of those is what decides whether two tests
// share a process. The last test is the whole thing: the real binary, started
// on each of the three surfaces against a stub GitLab, serving exactly what
// the shared assemblers say it should and answering a call made by canonical
// action ID.

package harness

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// stubToken is the credential the harness's own tests run with. It reaches
// only the stub GitLab, which authenticates nobody.
const stubToken = "glpat-harness-stub"

// stubInstance builds the instance a harness test drives: a stub GitLab, a
// client for it, and the facts a probe of it reports.
//
// It exists because the package-wide bootstrap needs a real GitLab and these
// tests must not. Everything below the bootstrap is the same code a real run
// uses, including the probe.
func stubInstance(t *testing.T) *instance {
	t.Helper()

	stub := startStubGitLab(t)
	client, err := gitlabclient.NewClientWithToken(stub.URL, stubToken, false)
	if err != nil {
		t.Fatalf("building a client for the stub: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	facts, err := probe(ctx, client, stub.URL, false, stubToken)
	if err != nil {
		t.Fatalf("probing the stub: %v", err)
	}

	return &instance{
		settings:    testSettings(map[string]string{envGitLabURL: stub.URL, envGitLabToken: stubToken}),
		requirement: Any,
		pkg:         "harness",
		runID:       configuredRunID(time.Now(), "", "harness"),
		facts:       facts,
		client:      client,
	}
}

// TestServerConfig_EmptyFields_TakeTheBinarysOwnDefaults checks that a
// configuration nobody filled in describes the server a client gets with
// nothing set.
//
// It matters because the default surface is the one a deployment serves and
// the one the suite should exercise most: a harness whose empty configuration
// meant meta would have every unqualified test running on a surface almost
// nobody uses.
func TestServerConfig_EmptyFields_TakeTheBinarysOwnDefaults(t *testing.T) {
	normalized := ServerConfig{}.normalized()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "surface", got: normalized.Surface.String(), want: SurfaceDynamic.String()},
		{name: "mode", got: normalized.Mode.String(), want: ModeDefault.String()},
		{name: "capabilities", got: normalized.Capabilities.String(), want: CapabilitiesFull.String()},
		{name: "elicitation", got: normalized.Elicitation.String(), want: ElicitationNone.String()},
		{name: "transport", got: normalized.Transport.String(), want: TransportStdio.String()},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("%s = %q, want %q", testCase.name, testCase.got, testCase.want)
			}
		})
	}
}

// TestServerConfig_Invalid_IsRefusedWithAReason checks that a configuration
// the harness cannot start says so rather than starting something else.
func TestServerConfig_Invalid_IsRefusedWithAReason(t *testing.T) {
	cases := []struct {
		name   string
		config ServerConfig
		want   string
	}{
		{name: "unknown surface", config: ServerConfig{Surface: Surface("everything")}, want: "unknown tool surface"},
		{name: "unknown mode", config: ServerConfig{Mode: Mode("paranoid")}, want: "unknown protective mode"},
		{name: "unknown capability surface", config: ServerConfig{Capabilities: CapabilitySurface("some")}, want: "unknown capability surface"},
		{name: "scripted with no responder", config: ServerConfig{Elicitation: ElicitationScripted}, want: "needs a Responder"},
		{name: "http transport", config: ServerConfig{Transport: TransportHTTP}, want: "not wired yet"},
		{name: "unknown transport", config: ServerConfig{Transport: TransportKind("carrier pigeon")}, want: "unknown transport"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.config.normalized().validate()
			if err == nil {
				t.Fatalf("%#v was accepted", testCase.config)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("the refusal is %q, want it to mention %q", err, testCase.want)
			}
		})
	}
}

// TestServerConfig_Key_SeparatesWhatMakesADifferentServer pins which
// differences get a process of their own.
//
// The credential is in the key because the scopes it carries narrow the
// catalog before anything is registered, and the instance because the same
// shape against another GitLab is another server. A key that ignored either
// would hand a test a session serving a catalog built for somebody else.
func TestServerConfig_Key_SeparatesWhatMakesADifferentServer(t *testing.T) {
	base := ServerConfig{}.normalized()
	baseKey := base.key("https://gitlab.test", "token-a")

	cases := []struct {
		name   string
		config ServerConfig
		url    string
		token  string
	}{
		{name: "surface", config: ServerConfig{Surface: SurfaceMeta}, url: "https://gitlab.test", token: "token-a"},
		{name: "mode", config: ServerConfig{Mode: ModeReadOnly}, url: "https://gitlab.test", token: "token-a"},
		{name: "capabilities", config: ServerConfig{Capabilities: CapabilitiesMinimal}, url: "https://gitlab.test", token: "token-a"},
		{name: "exclusions", config: ServerConfig{ExcludeTools: []string{"gitlab_issue"}}, url: "https://gitlab.test", token: "token-a"},
		{name: "elicitation", config: ServerConfig{Elicitation: ElicitationAutoAccept}, url: "https://gitlab.test", token: "token-a"},
		{name: "instance", config: ServerConfig{}, url: "https://other.test", token: "token-a"},
		{name: "credential", config: ServerConfig{}, url: "https://gitlab.test", token: "token-b"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			key := testCase.config.normalized().key(testCase.url, testCase.token)
			if key == baseKey {
				t.Errorf("%s does not change the session key: both are %q", testCase.name, key)
			}
		})
	}

	if again := base.key("https://gitlab.test", "token-a"); again != baseKey {
		t.Errorf("the same configuration produced two keys: %q and %q", baseKey, again)
	}
}

// TestServerConfig_Key_CarriesNoCredential checks that the key a failure
// message and a log line carry does not carry the token.
func TestServerConfig_Key_CarriesNoCredential(t *testing.T) {
	key := ServerConfig{}.normalized().key("https://gitlab.test", "glpat-secret-value")

	if strings.Contains(key, "glpat-secret-value") {
		t.Errorf("the session key carries the credential: %q", key)
	}
}

// TestServerConfig_Private_GetsAServerOfItsOwn checks that two private
// sessions of one shape never share a process.
func TestServerConfig_Private_GetsAServerOfItsOwn(t *testing.T) {
	private := ServerConfig{Private: true}.normalized()

	first := private.key("https://gitlab.test", "token")
	second := private.key("https://gitlab.test", "token")

	if first == second {
		t.Fatalf("two private sessions share the key %q", first)
	}
}

// TestServerConfig_ChildVariables_AreTheOnesTheBinaryReads pins the mapping
// from a shape to the environment the server is started with.
//
// Every entry here is a variable cmd/server itself consults. That is the whole
// contract of this type: a field with no variable behind it would describe a
// server the harness could produce and a deployment could not.
func TestServerConfig_ChildVariables_AreTheOnesTheBinaryReads(t *testing.T) {
	cases := []struct {
		name   string
		config ServerConfig
		want   map[string]string
	}{
		{
			name:   "default",
			config: ServerConfig{},
			want: map[string]string{
				"GITLAB_MCP_TOOL_SURFACE":       "dynamic",
				"GITLAB_MCP_CAPABILITY_SURFACE": "full",
				"GITLAB_MCP_READ_ONLY":          "false",
				"GITLAB_MCP_SAFE_MODE":          "false",
			},
		},
		{
			name:   "read-only meta",
			config: ServerConfig{Surface: SurfaceMeta, Mode: ModeReadOnly},
			want: map[string]string{
				"GITLAB_MCP_TOOL_SURFACE": "meta",
				"GITLAB_MCP_READ_ONLY":    "true",
				"GITLAB_MCP_SAFE_MODE":    "false",
			},
		},
		{
			name:   "safe individual with exclusions",
			config: ServerConfig{Surface: SurfaceIndividual, Mode: ModeSafe, ExcludeTools: []string{"gitlab_project_delete", "gitlab_issue"}},
			want: map[string]string{
				"GITLAB_MCP_TOOL_SURFACE":  "individual",
				"GITLAB_MCP_READ_ONLY":     "false",
				"GITLAB_MCP_SAFE_MODE":     "true",
				"GITLAB_MCP_EXCLUDE_TOOLS": "gitlab_issue,gitlab_project_delete",
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			vars := testCase.config.normalized().childVariables()
			for key, want := range testCase.want {
				if vars[key] != want {
					t.Errorf("%s = %q, want %q", key, vars[key], want)
				}
			}
		})
	}
}

// TestServerConfig_Label_NamesTheShapeWithoutHashes checks that the name a
// record and a server log carry reads as the configuration it describes.
func TestServerConfig_Label_NamesTheShapeWithoutHashes(t *testing.T) {
	cases := []struct {
		name    string
		config  ServerConfig
		private int64
		want    string
	}{
		{name: "default", config: ServerConfig{}, want: "dynamic-default-full"},
		{name: "read-only meta", config: ServerConfig{Surface: SurfaceMeta, Mode: ModeReadOnly}, want: "meta-read-only-full"},
		{name: "excluded", config: ServerConfig{ExcludeTools: []string{"gitlab_issue"}}, want: "dynamic-default-full-excluded"},
		{name: "auto-accepting", config: ServerConfig{Elicitation: ElicitationAutoAccept}, want: "dynamic-default-full-auto-accept"},
		{name: "private", config: ServerConfig{Private: true}, private: 3, want: "dynamic-default-full-private3"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if label := testCase.config.normalized().label(testCase.private); label != testCase.want {
				t.Errorf("label = %q, want %q", label, testCase.want)
			}
		})
	}
}

// TestPrivateSessionNumber_ReadsTheNumberOffTheKey checks that a private
// session's label and its key agree, so a reader holding one can find the
// other.
func TestPrivateSessionNumber_ReadsTheNumberOffTheKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want int64
	}{
		{name: "shared", key: "dynamic|default|full|none|stdio|exclude=", want: 0},
		{name: "private", key: "dynamic|default|full|none|stdio|exclude=|private=7", want: 7},
		{name: "unparseable", key: "dynamic|default|full|none|stdio|exclude=|private=many", want: 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if number := privateSessionNumber(testCase.key); number != testCase.want {
				t.Errorf("privateSessionNumber(%q) = %d, want %d", testCase.key, number, testCase.want)
			}
		})
	}
}

// TestAcceptElicitation_AnswersWithAnEmptyAcceptance pins what the
// auto-accepting policy sends back.
//
// Empty content is deliberate: the only request this policy exists for is the
// confirmation a destructive action raises, which reads no field back. A flow
// that needs values gets a scripted responder instead, and a policy that
// invented values here would answer it with the wrong ones silently.
func TestAcceptElicitation_AnswersWithAnEmptyAcceptance(t *testing.T) {
	result, err := acceptElicitation(context.Background(), &mcp.ElicitRequest{})
	if err != nil {
		t.Fatalf("acceptElicitation() error = %v, want nil", err)
	}
	if result.Action != "accept" {
		t.Errorf("action = %q, want accept", result.Action)
	}
	if len(result.Content) != 0 {
		t.Errorf("content = %v, want it empty", result.Content)
	}
}

// TestSettingsWith_ChangesOneValueAndLeavesTheOriginal checks that a session
// given its own credential does not change the credential every other session
// inherits.
func TestSettingsWith_ChangesOneValueAndLeavesTheOriginal(t *testing.T) {
	original := testSettings(map[string]string{envGitLabURL: "https://gitlab.test", envGitLabToken: "first"})

	changed := original.with(envGitLabToken, "second")

	if original.get(envGitLabToken) != "first" {
		t.Errorf("the original settings now carry %q", original.get(envGitLabToken))
	}
	if changed.get(envGitLabToken) != "second" {
		t.Errorf("the copy carries %q, want second", changed.get(envGitLabToken))
	}
	if changed.get(envGitLabURL) != "https://gitlab.test" {
		t.Errorf("the copy lost the instance: %q", changed.get(envGitLabURL))
	}
}

// TestSession_UnsupportedTransport_FailsTheTestThatAskedForIt checks that a
// configuration the harness cannot start stops one test rather than being
// silently downgraded to the one it can.
func TestSession_UnsupportedTransport_FailsTheTestThatAskedForIt(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)

	_, err := env.session(ServerConfig{Transport: TransportHTTP})

	if err == nil {
		t.Fatal("an HTTP session was started, and the launcher serves stdio only")
	}
	if !strings.Contains(err.Error(), "not wired yet") {
		t.Errorf("the refusal is %q, want it to say the transport is not wired", err)
	}
}

// statusAnswer is the part of the server.status answer these tests read.
type statusAnswer struct {
	// Status is the diagnostic verdict.
	Status string `json:"status"`
	// GitLabURL is the instance the server was configured with.
	GitLabURL string `json:"gitlab_url"`
	// Authenticated says the server reached GitLab with its credential.
	Authenticated bool `json:"authenticated"`
}

// TestSession_RealBinaryOnEverySurface_ServesTheAssemblersCatalogAndAnswers is
// the whole of what this step delivers, exercised end to end.
//
// For each of the three surfaces it starts the real binary, compares what the
// session serves with what tools.SharedMetaCatalog, tools.SharedIndividualCatalog
// and dynamiccatalog.Build say it should, and then makes one call by canonical
// action ID. The call goes through the projection, so each surface sends its
// own spelling: gitlab_execute_action with server.status, gitlab_server with
// status, and gitlab_server_status with nothing.
//
// The GitLab behind it is the stub, which answers only what the server asks at
// startup and what the status action reads, so the test needs no instance and
// no credential.
func TestSession_RealBinaryOnEverySurface_ServesTheAssemblersCatalogAndAnswers(t *testing.T) {
	inst := stubInstance(t)

	for _, surface := range AllSurfaces() {
		t.Run(string(surface), func(t *testing.T) {
			env := newEnv(t, inst)
			// Private, because the stub GitLab behind this instance is closed
			// when the test ends and a pooled session would outlive it.
			session := env.Session(ServerConfig{Surface: surface, Private: true})

			if len(session.Tools()) == 0 {
				t.Fatal("the session lists no tools at all")
			}
			if !session.Serves("server.status") {
				t.Fatalf("the %s session does not serve server.status", surface)
			}

			answer := Do[statusAnswer](session, "server.status", nil)
			if !answer.Authenticated {
				t.Errorf("the server reports it is not authenticated: %+v", answer)
			}
			// A prefix, because the server reports the API base it built from
			// the instance rather than the instance itself.
			if !strings.HasPrefix(answer.GitLabURL, inst.facts.URL) {
				t.Errorf("gitlab_url = %q, want it to name the instance the harness pointed it at, %q", answer.GitLabURL, inst.facts.URL)
			}
			if answer.Status == "" {
				t.Errorf("the status answer carries no status: %+v", answer)
			}
		})
	}
}

// TestSession_UnservableAction_IsRefusedBeforeItIsSent checks the three
// pre-flight refusals, which exist because each of them produces a failure
// that reads as something else once the call has gone out.
func TestSession_UnservableAction_IsRefusedBeforeItIsSent(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)
	session := env.Session(ServerConfig{Surface: SurfaceIndividual, Private: true})

	cases := []struct {
		name string
		id   ActionID
		want string
	}{
		{name: "not in the catalog", id: "not_a_domain.not_an_action", want: "no action"},
		{name: "shadowed individual name", id: "repository.file_history", want: "not servable on the individual surface"},
		{name: "no individual tool", id: "server.health_check", want: "not servable on the individual surface"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := session.resolve(testCase.id, nil, false)
			if err == nil {
				t.Fatalf("%s resolved to a call on the individual surface", testCase.id)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("the refusal is %q, want it to mention %q", err, testCase.want)
			}
		})
	}
}

// TestSession_ReadOnlyMode_WithholdsTheWritesAndSaysSo checks a protective
// mode end to end: the server registers a narrowed surface, the session's
// served-set check accepts it, and a call to something the mode removed is
// refused before it is sent, naming the mode.
func TestSession_ReadOnlyMode_WithholdsTheWritesAndSaysSo(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)
	session := env.Session(ServerConfig{Surface: SurfaceIndividual, Mode: ModeReadOnly, Private: true})

	if !session.Serves("project.get") {
		t.Error("the read-only session does not serve project.get, which reads")
	}
	if session.Serves("project.delete") {
		t.Fatal("the read-only session serves project.delete")
	}
	if slices.Contains(session.Tools(), "gitlab_project_delete") {
		t.Error("the read-only session lists gitlab_project_delete")
	}

	_, err := session.resolve("project.delete", map[string]any{"project_id": "group/project"}, true)
	if err == nil {
		t.Fatal("project.delete resolved on a read-only session")
	}
	if !strings.Contains(err.Error(), "read-only mode") {
		t.Errorf("the refusal is %q, want it to name the mode that removed the action", err)
	}
}

// TestSession_DestructiveWithoutConfirmation_IsRefusedByTheServer checks the
// refusal verb against the real binary.
//
// The confirmation guard runs before the handler, so the call never reaches
// GitLab and the stub behind this test is never asked to delete anything. That
// is also why this is the one end-to-end assertion about a mutating action
// that a test with no GitLab can make.
func TestSession_DestructiveWithoutConfirmation_IsRefusedByTheServer(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)
	// ElicitationNone, so the client advertises no way to prompt and the
	// server fails closed rather than asking.
	session := env.Session(ServerConfig{Surface: SurfaceIndividual, Elicitation: ElicitationNone, Private: true})

	text := Refused(session, "project.delete", map[string]any{"project_id": "group/project"},
		FailureNeedsConfirmation, WithoutConfirmation())

	if !strings.Contains(text, "confirm=true") {
		t.Errorf("the refusal does not tell the caller how to proceed: %q", text)
	}
}

// TestSession_ActionAboveThePackagesRequirement_IsRefused checks the tier
// ceiling, which is the runtime half of the rule the static gate enforces.
//
// A package that declared no license may call Free actions only. Without this
// the same test would pass on a licensed runtime and fail on an unlicensed
// one, and a suite whose verdict depends on which runtime ran it is not a
// gate.
func TestSession_ActionAboveThePackagesRequirement_IsRefused(t *testing.T) {
	inst := stubInstance(t)
	// Pretend the instance is licensed while the package still is not: that is
	// exactly the case the ceiling exists for, and the one a Docker run on an
	// EE image produces.
	inst.facts.Tier = edition.Ultimate
	env := newEnv(t, inst)
	session := env.Session(ServerConfig{Private: true})

	licensed := licensedActionFor(t, inst)
	if _, err := session.resolve(licensed, nil, false); err == nil {
		t.Fatalf("%s resolved for a package that declared no license", licensed)
	} else if !strings.Contains(err.Error(), "runs on free") {
		t.Errorf("the refusal is %q, want it to name the ceiling the package runs under", err)
	}
}

// licensedActionFor returns one action the Ultimate catalog has and the Free
// one does not, so the ceiling test names a real action rather than a made-up
// one.
func licensedActionFor(t *testing.T, inst *instance) ActionID {
	t.Helper()

	ultimate, err := newProjection(edition.Ultimate, inst.client.IsGitLabDotCom())
	if err != nil {
		t.Fatalf("building the Ultimate projection: %v", err)
	}
	free, err := newProjection(edition.Free, inst.client.IsGitLabDotCom())
	if err != nil {
		t.Fatalf("building the Free projection: %v", err)
	}
	for id, action := range ultimate.actions {
		if action.minimumTier == edition.Free {
			continue
		}
		if _, inFree := free.lookup(id); !inFree {
			return id
		}
	}
	t.Fatal("the Ultimate catalog carries no action the Free one lacks")
	return ""
}
