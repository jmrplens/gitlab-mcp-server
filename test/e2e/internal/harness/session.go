//go:build e2e

// session.go starts one server per configuration and hands every test that
// asks for that configuration the same one.
//
// A configuration here is only what the released binary reads: a tool surface,
// a protective mode, a capability surface, a credential, an exclusion list, a
// transport. There is no hook that registers something extra and no way to
// hand the server a catalog of the test's own, because the suite this replaces
// had both and consequently tested an assembly no deployment runs.
//
// Sessions are shared because starting a server costs a process and a catalog
// build, and because nothing about one test's calls changes what the next test
// is served. A test that needs a server nobody else touches asks for a private
// one, which is the same shape with a key nothing else can name.

package harness

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// Mode is the protective mode a server runs in.
type Mode string

// The three protective modes, named as the reader of a report would name them
// rather than as the two booleans that produce them.
const (
	// ModeDefault runs the server with neither protection on.
	ModeDefault Mode = "default"
	// ModeReadOnly removes every mutating operation.
	ModeReadOnly Mode = "read-only"
	// ModeSafe answers a mutating operation with a preview of it.
	ModeSafe Mode = "safe"
)

// String returns the mode as a report names it.
func (m Mode) String() string { return string(m) }

// CapabilitySurface is the resource and prompt catalog a server serves.
type CapabilitySurface string

// The two capability surfaces, spelled as GITLAB_MCP_CAPABILITY_SURFACE takes
// them.
const (
	// CapabilitiesFull serves every resource and prompt.
	CapabilitiesFull CapabilitySurface = config.CapabilitySurfaceFull
	// CapabilitiesMinimal serves only what dynamic use needs.
	CapabilitiesMinimal CapabilitySurface = config.CapabilitySurfaceMinimal
)

// String returns the capability surface as the environment variable spells it.
func (c CapabilitySurface) String() string { return string(c) }

// Elicitation is what the harness's client does with an elicitation request.
type Elicitation string

// The three elicitation policies. The default is None, and deliberately: a
// client that advertises no elicitation makes the server fail closed on an
// unconfirmed destructive call, which is the behavior a test asserting that
// refusal needs. A flow that exists to be elicited asks for one of the others.
const (
	// ElicitationNone advertises no elicitation capability at all.
	ElicitationNone Elicitation = "none"
	// ElicitationAutoAccept accepts every request with an empty answer, which
	// is what a confirmation prompt needs and nothing more.
	ElicitationAutoAccept Elicitation = "auto-accept"
	// ElicitationScripted answers with the responder the configuration
	// carries. A scripted session is always private, since its answers belong
	// to one test.
	ElicitationScripted Elicitation = "scripted"
)

// String returns the policy's name.
func (e Elicitation) String() string { return string(e) }

// TransportKind is how the harness reaches the server.
type TransportKind string

// The two transports a deployment can use. Only stdio is wired here; see
// [ServerConfig.Transport].
const (
	// TransportStdio drives the binary over its standard streams, the way
	// every MCP client that launches a local server does.
	TransportStdio TransportKind = "stdio"
	// TransportHTTP drives it over a loopback HTTP listener.
	TransportHTTP TransportKind = "http"
)

// String returns the transport's name.
func (t TransportKind) String() string { return string(t) }

// TierPin is a licensing tier a session asks its server to serve instead of
// detecting one.
//
// It is spelled as GITLAB_MCP_TIER takes it rather than as an [edition.Tier],
// because that type is an ordering whose zero value is Free: a field of it
// could not tell "serve the Free catalog" from "say nothing and let the child
// detect", which is the whole of what this field is for.
type TierPin string

// The tiers a session can pin, and the empty value that pins none.
const (
	// TierDetect leaves GITLAB_MCP_TIER unset, so the child reads the license
	// itself. This is what every ordinary session wants and what a deployment
	// does.
	TierDetect TierPin = ""
	// TierFree pins the Free/CE catalog.
	TierFree TierPin = "free"
	// TierPremium pins the Premium catalog.
	TierPremium TierPin = "premium"
	// TierUltimate pins the Ultimate catalog.
	TierUltimate TierPin = "ultimate"
)

// String returns the pin as the environment variable spells it.
func (t TierPin) String() string { return string(t) }

// AllTierPins returns every tier a session can pin, in the order Free,
// Premium, Ultimate. TierDetect is not one of them: it is the absence of a
// pin, so a scenario sweeping the pinned catalogs never wants it.
func AllTierPins() []TierPin { return []TierPin{TierFree, TierPremium, TierUltimate} }

// ServerConfig is one shape of server, in the terms the released binary reads.
//
// Every field maps onto a variable or a flag cmd/server itself consults. There
// is deliberately no field for anything else: a configuration that could only
// be produced by assembling a server in this process would describe a program
// nobody deploys.
type ServerConfig struct {
	// Surface selects the tool catalog: dynamic, meta or individual. Empty is
	// dynamic, which is what the binary serves with nothing set.
	Surface Surface
	// Mode selects the protective mode. Empty is ModeDefault.
	Mode Mode
	// Capabilities selects the resource and prompt catalog. Empty is
	// CapabilitiesFull, the binary's own default.
	Capabilities CapabilitySurface
	// Token is the credential the server runs with. Empty is the run's own
	// token; a fixture-minted one here is how a narrowed surface is tested.
	Token string
	// ExcludeTools is the operator's removal list, as --exclude-tools takes it.
	ExcludeTools []string
	// Elicitation is what this session's client does with an elicitation
	// request. Empty is ElicitationNone.
	Elicitation Elicitation
	// Responder answers an elicitation request under ElicitationScripted, and
	// is ignored under the other two policies.
	Responder func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
	// Tier pins the licensing tier instead of letting the child detect it.
	//
	// TierDetect, the zero value, is detection: the child reads GET /license
	// and falls back to Free, which is what every ordinary session wants and
	// what a deployment does. Pinning is how a scenario reaches the catalog of
	// a tier this runtime is not, and how the one product fact the suite knows
	// about detection is asserted, that a non-administrator token on a
	// licensed instance is served the Free catalog because that endpoint
	// answers administrators only.
	Tier TierPin
	// Private asks for a server no other test shares, for a test that will
	// leave the process in a state the next one should not inherit.
	Private bool
	// Transport is how the harness reaches the server. Empty is TransportStdio,
	// which is what a client launching a local server uses and what almost
	// every scenario wants.
	//
	// TransportHTTP starts the binary on a loopback listener and carries the
	// credential in a header, which is the deployment shape a shared server is
	// run as. It is worth asking for where the carriage itself is what a
	// scenario is about: nothing else reaches the pool entry, the per-request
	// client binding or the header, and the transport module that covers the
	// HTTP handler chain does it without a GitLab at all.
	Transport TransportKind
}

// normalized returns the configuration with every empty field replaced by the
// default the binary itself applies, so a key and a child environment are
// built from what the server will actually be rather than from what was
// written down.
func (c ServerConfig) normalized() ServerConfig {
	if c.Surface == "" {
		c.Surface = SurfaceDynamic
	}
	if c.Mode == "" {
		c.Mode = ModeDefault
	}
	if c.Capabilities == "" {
		c.Capabilities = CapabilitiesFull
	}
	if c.Elicitation == "" {
		c.Elicitation = ElicitationNone
	}
	if c.Transport == "" {
		c.Transport = TransportStdio
	}
	c.ExcludeTools = slices.Clone(c.ExcludeTools)
	slices.Sort(c.ExcludeTools)
	return c
}

// validate reports a configuration the harness cannot start.
func (c ServerConfig) validate() error {
	if !slices.Contains(AllSurfaces(), c.Surface) {
		return fmt.Errorf("unknown tool surface %q", c.Surface)
	}
	if c.Mode != ModeDefault && c.Mode != ModeReadOnly && c.Mode != ModeSafe {
		return fmt.Errorf("unknown protective mode %q", c.Mode)
	}
	if c.Capabilities != CapabilitiesFull && c.Capabilities != CapabilitiesMinimal {
		return fmt.Errorf("unknown capability surface %q", c.Capabilities)
	}
	if c.Elicitation == ElicitationScripted && c.Responder == nil {
		return errors.New("an elicitation policy of scripted needs a Responder to answer with")
	}
	// A misspelled pin would otherwise be served as detection, which is the
	// one outcome a test that pinned a tier cannot notice: the catalog would
	// be the runtime's own and every assertion about it would pass.
	if _, ok := edition.ParseTier(string(c.Tier)); c.Tier != TierDetect && !ok {
		return fmt.Errorf("unknown tier pin %q", c.Tier)
	}
	if c.Transport != TransportStdio && c.Transport != TransportHTTP {
		return fmt.Errorf("unknown transport %q", c.Transport)
	}
	return nil
}

// resolvedTier is the tier this session's server actually serves: the pinned
// one when there is one, and otherwise what the credential could read off the
// license. Both the catalog the child registers and the expectation the
// harness compares it against are built from this one answer.
//
// A pin that does not parse cannot reach here: [ServerConfig.validate] refuses
// it, with the same parser.
func (c ServerConfig) resolvedTier(detected edition.Tier) edition.Tier {
	if tier, ok := edition.ParseTier(string(c.Tier)); ok {
		return tier
	}
	return detected
}

// privateSessions numbers the private sessions one process hands out, so each
// gets a key nothing else can name.
var privateSessions atomic.Int64

// key names one server shape. Two tests naming the same key share a process;
// two naming different keys get one each.
//
// The instance is part of it because the same shape against another GitLab is
// another server, and the token because the credential decides the catalog:
// the scopes it carries narrow the surface before anything is registered. The
// token is hashed rather than carried, since the key is printed.
func (c ServerConfig) key(instance, token string) string {
	// The pinned tier is part of the key for the same reason the token is: it
	// decides the catalog. A session that pinned one and a session that lets
	// the child detect it are two different servers, and sharing one process
	// between them would serve the second whatever the first registered.
	parts := []string{
		string(c.Surface), string(c.Mode), string(c.Capabilities), string(c.Elicitation), string(c.Transport),
		"tier=" + string(c.Tier),
		"exclude=" + strings.Join(c.ExcludeTools, ","),
		"instance=" + shortStableHash(instance),
		"token=" + shortStableHash(token),
	}
	if c.Private || c.Elicitation == ElicitationScripted {
		parts = append(parts, "private="+strconv.FormatInt(privateSessions.Add(1), 10))
	}
	return strings.Join(parts, "|")
}

// label names a session in a record and in a log file. It is the key without
// the hashes, which are noise to a reader and identical across one run.
func (c ServerConfig) label(private int64) string {
	label := strings.Join([]string{string(c.Surface), string(c.Mode), string(c.Capabilities)}, "-")
	// A pinned tier names the session too: the label is the session's
	// directory, and two sessions differing only in it would write one
	// server's log over the other's.
	if c.Tier != "" {
		label += "-" + string(c.Tier)
	}
	if len(c.ExcludeTools) > 0 {
		label += "-excluded"
	}
	if c.Elicitation != ElicitationNone {
		label += "-" + string(c.Elicitation)
	}
	if private > 0 {
		label += "-private" + strconv.FormatInt(private, 10)
	}
	return label
}

// childVariables returns the environment variables this configuration sets on
// the server child, which is the whole of what it configures.
//
// The telemetry variables are merged in here rather than at the launcher,
// because this is the one place a session's child environment is built and
// every child runs with telemetry on: a session whose server exported no spans
// could only ever record what its tests asked for, never what ran.
func (c ServerConfig) childVariables() map[string]string {
	vars := map[string]string{
		"GITLAB_MCP_TOOL_SURFACE":       string(c.Surface),
		"GITLAB_MCP_CAPABILITY_SURFACE": string(c.Capabilities),
		"GITLAB_MCP_READ_ONLY":          strconv.FormatBool(c.Mode == ModeReadOnly),
		"GITLAB_MCP_SAFE_MODE":          strconv.FormatBool(c.Mode == ModeSafe),
		"GITLAB_MCP_LOG_LEVEL":          serverLogLevel,
	}
	if len(c.ExcludeTools) > 0 {
		vars["GITLAB_MCP_EXCLUDE_TOOLS"] = strings.Join(c.ExcludeTools, ",")
	}
	// Set only when pinned: the variable's absence is what asks the child to
	// detect the tier, and an empty value would be a configuration error
	// rather than the default.
	if c.Tier != "" {
		vars["GITLAB_MCP_TIER"] = string(c.Tier)
	}
	maps.Copy(vars, telemetryVariables())
	return vars
}

// serverLogLevel is what every child logs at. Info rather than the default, so
// a child's log says which catalog it registered and what it refused, which is
// the first thing a failed call wants to quote.
const serverLogLevel = "info"

// sessionStartTimeout bounds starting one child and listing what it serves.
// The individual surface builds roughly a thousand tools and marshals several
// megabytes into its first tools/list, and does it slower under the race
// detector.
const sessionStartTimeout = 3 * time.Minute

// Session is one test's handle on a server.
//
// The server behind it may be shared with other tests; the handle is not, and
// carries the Env whose test the calls made through it are attributed to.
type Session struct {
	env  *Env
	conn *sessionConn
}

// Label returns the name this session is recorded under.
func (s *Session) Label() string { return s.conn.label }

// Surface returns the tool surface this session serves.
func (s *Session) Surface() Surface { return s.conn.cfg.Surface }

// Mode returns the protective mode this session runs in.
func (s *Session) Mode() Mode { return s.conn.cfg.Mode }

// Capabilities returns the resource and prompt surface this session serves.
func (s *Session) Capabilities() CapabilitySurface { return s.conn.cfg.Capabilities }

// Tier returns the licensing tier this session's server serves, which decides
// which actions exist in its catalog.
//
// It is the pinned tier for a session that asked for one, and otherwise what
// the credential could read: the run's tier for a session on the run's own
// token, and for a session given another token what that token could read,
// since the license endpoint answers administrators only and any other user's
// token is served the Free catalog on a licensed instance.
func (s *Session) Tier() edition.Tier { return s.conn.tier }

// Transport returns how the harness reaches this session's server.
func (s *Session) Transport() TransportKind { return s.conn.cfg.Transport }

// Tools returns the tool names the session listed when it started.
func (s *Session) Tools() []string { return slices.Clone(s.conn.served.tools) }

// ToolDefinitions returns the tools the session serves as the server published
// them, each with its input and output schema and annotations, for a test of
// what the served surface says about a tool rather than only its name.
//
// It lists afresh rather than reading what the session start kept, because that
// kept only the names. The listing is not attributed and so is not recorded as
// coverage: it is a question about the surface, not a call of an action.
func (s *Session) ToolDefinitions() []*mcp.Tool {
	s.env.T.Helper()

	ctx, cancel := context.WithTimeout(s.env.Ctx, sessionStartTimeout)
	defer cancel()

	var tools []*mcp.Tool
	for tool, err := range s.conn.client().Tools(ctx, nil) {
		if err != nil {
			s.env.T.Fatalf("tools/list on the %s session: %v%s", s.Surface(), err, s.conn.failureContext())
			return nil
		}
		tools = append(tools, tool)
	}
	return tools
}

// Resources returns the static resource URIs the session listed.
func (s *Session) Resources() []string { return slices.Clone(s.conn.served.resources) }

// ResourceTemplates returns the resource URI templates the session listed.
func (s *Session) ResourceTemplates() []string { return slices.Clone(s.conn.served.templates) }

// Prompts returns the prompt names the session listed.
func (s *Session) Prompts() []string { return slices.Clone(s.conn.served.prompts) }

// PromptSpecs returns each served prompt with the arguments it declares, so a
// sweep can bind the required ones and skip a prompt it cannot satisfy. The
// slices are copied, so a caller cannot change what the session listed.
func (s *Session) PromptSpecs() []PromptSpec {
	specs := make([]PromptSpec, len(s.conn.served.promptSpecs))
	for i, spec := range s.conn.served.promptSpecs {
		specs[i] = PromptSpec{
			Name:     spec.Name,
			Required: slices.Clone(spec.Required),
			Optional: slices.Clone(spec.Optional),
		}
	}
	return specs
}

// Serves reports whether this session can reach the given action at all.
//
// It answers from the catalog the server built rather than from the base
// catalog, so an action removed by read-only mode, by the credential's scopes
// or by the operator's exclusions is not served, and neither is one whose
// individual tool name another action owns.
func (s *Session) Serves(id ActionID) bool {
	_, ok := s.conn.served.actions[id]
	return ok
}

// Actions returns every action this session can reach, sorted, on the same
// terms as Serves. It is what a test compares a listing the server published
// against, and what a sweep walks.
func (s *Session) Actions() []ActionID {
	actions := make([]ActionID, 0, len(s.conn.served.actions))
	for id := range s.conn.served.actions {
		actions = append(actions, id)
	}
	slices.Sort(actions)
	return actions
}

// sessionConn is one running server and the client speaking to it.
type sessionConn struct {
	label string
	cfg   ServerConfig
	// tier is the tier the server detected with this session's credential,
	// which is the run's own unless the session was given another token.
	tier edition.Tier
	proc *serverProcess
	inst *instance

	// served is what the session listed when it started, and what the
	// served-set check was run against.
	served servedSets

	mu      sync.Mutex
	session *mcp.ClientSession
	// notifier fans resource-updated notifications out to the subscriptions
	// tests opened on this session.
	notifier *updateNotifier
	// subscribers says whose record a resource-updated notification belongs
	// in, which the notifier cannot answer: it wakes channels, not tests.
	subscribers *subscriberIndex
	// progress collects the progress notifications this session's calls asked
	// for, keyed by the token each call minted.
	progress *progressCollector

	// dispatchObserved is set the first time a span of this session's own
	// arrives. While it is false every call of the session is a claim about
	// what was asked for and not about what ran, and the session line says so.
	dispatchObserved atomic.Bool

	// inFlight holds the attribution of every call this session is serving
	// right now, so an elicitation the server sends back mid-call can be
	// recorded against the test that provoked it.
	inFlightMu   sync.Mutex
	inFlight     map[int64]callAttribution
	inFlightNext atomic.Int64
}

// sessionEntry is one pooled session, started by the first test that asks for
// its key while every concurrent asker waits.
type sessionEntry struct {
	once sync.Once
	conn *sessionConn
	err  error
}

// sessions holds one entry per server shape for the life of the test binary.
var sessions sync.Map // string -> *sessionEntry

// sessionContext is what every pooled child's life is bounded by, and
// sessionCancel is what [closeSessions] ends the run's servers with.
//
// It belongs to the package rather than to the first caller because the
// children outlive whichever test started them: a context created inside one
// test's call would end with that test and take every later test's server with
// it.
var sessionContext, sessionCancel = context.WithCancel(context.Background())

// sessionLifetime returns the context every child of this run is started
// under.
func sessionLifetime() context.Context { return sessionContext }

// sessionRoot is the one directory every child calls home, and the only
// directory any of them may read a local file from or write a download into.
var (
	sessionRootOnce sync.Once
	sessionRootDir  string
	errSessionRoot  error
)

// harnessRoot returns the directory the children run in, creating it once.
//
// Not a t.TempDir: children outlive the test that started them, and a
// directory removed when that test ended would take the home of every later
// test's server with it.
func harnessRoot() (string, error) {
	sessionRootOnce.Do(func() {
		sessionRootDir, errSessionRoot = os.MkdirTemp("", "gitlab-mcp-e2e-root")
	})
	return sessionRootDir, errSessionRoot
}

// closeSessions stops every server this process started and removes what they
// ran in.
//
// Main calls it before it deletes the binary it built, and the order matters
// on Windows, where an executable that is still running cannot be removed.
//
// Close first and cancel second, which is the reverse of what this did until
// the run's Go coverage was measured. Canceling first killed every child
// through exec's default Cancel, and a killed process runs no exit hook: an
// instrumented child wrote its meta-data at startup and never its counters, so
// everything it executed was lost. A session's Close ends a stdio child the
// way a client does, by closing its stdin and waiting; the cancel is what is
// left for an HTTP child, whose client holds no pipe of the process at all.
//
// Then the wait, before anything is deleted. The cancel only asks, and what
// follows assumes the answer: the session root is each child's home and
// working directory, the built binary is the file it is executing, and on
// Windows a running executable cannot be removed at all.
func closeSessions() {
	var procs []*serverProcess
	sessions.Range(func(_, value any) bool {
		if entry, ok := value.(*sessionEntry); ok && entry.conn != nil {
			entry.conn.close()
			procs = append(procs, entry.conn.proc)
		}
		return true
	})
	sessionCancel()
	waitForChildren(procs)

	if sessionRootDir != "" {
		_ = os.RemoveAll(sessionRootDir)
	}
}

// waitForChildren waits for the run's children to be collected under one
// budget shared by all of them, and says which ones outlasted it.
//
// Saying so rather than failing: the run's verdict belongs to the tests, and a
// child that would not stop is a fact about the shutdown that the next reader
// of the log needs and that nothing else would record.
func waitForChildren(procs []*serverProcess) {
	deadline := time.Now().Add(childrenExitBudget)
	var running []string
	for _, proc := range procs {
		if !proc.waitForExit(deadline) {
			running = append(running, proc.label)
		}
	}
	if len(running) > 0 {
		log.Printf("e2e: %d server children were still running %s after being asked to stop: %s",
			len(running), childrenExitBudget, strings.Join(running, ", "))
	}
}

// Session returns this test's handle on the server for cfg, starting it if no
// test has asked for that shape yet.
//
// A configuration that cannot be started, or a server whose served set does
// not match what the shared assemblers say it should be, fails the test that
// asked for it. Every later test asking for the same shape fails the same way,
// with the same message: a session that started wrong is wrong for everybody,
// and letting the second test through would report the difference as a
// scattering of unrelated failures.
func (e *Env) Session(cfg ServerConfig) *Session {
	e.T.Helper()

	conn, err := e.session(cfg)
	if err != nil {
		e.T.Fatalf("starting the %s session: %v", cfg.normalized().Surface, err)
	}
	if conn.cfg.Private {
		e.T.Cleanup(conn.close)
	}
	return &Session{env: e, conn: conn}
}

// On returns this test's handle on the default session for one surface: no
// protective mode, the full capability surface, the run's own credential.
func (e *Env) On(surface Surface) *Session {
	e.T.Helper()
	return e.Session(ServerConfig{Surface: surface})
}

// session resolves the pooled connection for cfg, starting it once.
func (e *Env) session(cfg ServerConfig) (*sessionConn, error) {
	normalized := cfg.normalized()
	if err := normalized.validate(); err != nil {
		return nil, err
	}
	token := normalized.Token
	if token == "" {
		token = e.inst.settings.get(envGitLabToken)
	}

	key := normalized.key(e.inst.facts.URL, token)
	loaded, _ := sessions.LoadOrStore(key, &sessionEntry{})
	entry, _ := loaded.(*sessionEntry)
	entry.once.Do(func() {
		entry.conn, entry.err = startSession(e.inst, normalized, token, key)
	})
	return entry.conn, entry.err
}

// startSession launches one server and checks what it serves.
//
// The credential's scopes are resolved before the child is launched, because
// they decide what the session is: a token that cannot write is served a
// read-only surface by the binary whatever the configuration asked for, and
// the session is recorded as read-only so that the refusals its tests see are
// filed under the mode that produced them rather than under the default one.
func startSession(inst *instance, cfg ServerConfig, token, key string) (*sessionConn, error) {
	lifetime := sessionLifetime()
	ctx, cancel := context.WithTimeout(lifetime, sessionStartTimeout)
	defer cancel()

	root, err := harnessRoot()
	if err != nil {
		return nil, fmt.Errorf("creating the directory the server runs in: %w", err)
	}
	bin, err := serverBinary(inst.settings.get(binaryEnv))
	if err != nil {
		return nil, err
	}

	cred, err := sessionCredential(ctx, inst, token)
	if err != nil {
		return nil, err
	}
	serverCfg := serverConfigFor(inst, cfg, cred)
	expectation, err := expectedSurface(inst, cfg.Surface, serverCfg)
	if err != nil {
		return nil, err
	}
	// The child is given what was asked for and narrows itself from the
	// same scopes; only what the session is recorded as moves.
	recorded := cfg.narrowedBy(serverCfg)

	label := recorded.label(privateSessionNumber(key))
	dir := filepath.Join(root, sanitizeNamePart(label, 60))
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating the session directory %s: %w", dir, err)
	}

	settingsForChild := inst.settings
	if token != inst.settings.get(envGitLabToken) {
		settingsForChild = inst.settings.with(envGitLabToken, token)
	}

	// Merged here rather than in childVariables, which is a method on the
	// configuration and knows nothing about the run's settings, and which
	// cannot report a directory that could not be created. The map it returns
	// is a fresh one per call, so writing into it changes nobody else's child.
	childVars := cfg.childVariables()
	coverage, err := coverageVariables(inst.settings)
	if err != nil {
		return nil, err
	}
	maps.Copy(childVars, coverage)

	conn := &sessionConn{
		label:       label,
		cfg:         recorded,
		tier:        recorded.resolvedTier(cred.tier),
		inst:        inst,
		proc:        newServerProcess(label, bin, newChildEnv(settingsForChild, dir, childVars)),
		notifier:    newUpdateNotifier(),
		progress:    newProgressCollector(),
		subscribers: newSubscriberIndex(),
	}
	if connectErr := conn.connect(); connectErr != nil {
		return nil, connectErr
	}

	if conn.served, err = listServed(ctx, conn.client()); err != nil {
		conn.close()
		return nil, fmt.Errorf("listing what the %s session serves: %w\nserver stderr:\n%s", label, err, conn.proc.stderrTail())
	}
	if err = checkServedTools(cfg.Surface, conn.served.tools, expectation); err != nil {
		conn.close()
		return nil, err
	}
	conn.served.actions = expectation.actions
	return conn, nil
}

// narrowedBy returns the configuration as the binary will actually serve it:
// read-only when the credential's scopes made it so, whatever mode was asked
// for. It is applied to what the session is recorded as and never to the
// child's environment, so the narrowing under test stays the binary's own.
func (c ServerConfig) narrowedBy(serverCfg *config.ServerConfig) ServerConfig {
	if serverCfg.ReadOnlyFromTokenScope && c.Mode == ModeDefault {
		c.Mode = ModeReadOnly
	}
	return c
}

// privateSessionNumber reads back the number a private key carries, so the
// label a reader sees matches the key the pool holds. It returns zero for a
// shared session, which carries no number.
func privateSessionNumber(key string) int64 {
	_, suffix, found := strings.Cut(key, "private=")
	if !found {
		return 0
	}
	number, err := strconv.ParseInt(suffix, 10, 64)
	if err != nil {
		return 0
	}
	return number
}

// sessionCredential returns what the binary learns from the credential a
// session runs with: the token's scopes, and the tier it can read.
//
// The run's own token was probed at bootstrap; a session given another one is
// probed here, with the two calls the binary makes at startup, because both
// answers decide the catalog. The scopes decide which groups are registered
// at all, and the tier decides which exist: the license endpoint answers
// administrators only, so a token belonging to anyone else reads no license
// and is served the Free catalog on a licensed instance, exactly as the
// binary serves it.
func sessionCredential(ctx context.Context, inst *instance, token string) (credentialFacts, error) {
	if token == inst.settings.get(envGitLabToken) {
		return inst.credential(), nil
	}
	client, err := gitlabclient.NewClientWithToken(inst.facts.URL, token,
		strings.EqualFold(inst.settings.get(envSkipTLSVerify), "true"))
	if err != nil {
		return credentialFacts{}, fmt.Errorf("building a client for the session's own credential: %w", err)
	}
	return credentialFacts{
		scopes: gitlabclient.DetectScopes(ctx, client.GL()),
		tier:   client.DetectTier(ctx),
	}, nil
}

// connect starts the child and opens an MCP session to it.
func (c *sessionConn) connect() error {
	lifetime := sessionLifetime()
	ctx, cancel := context.WithTimeout(lifetime, sessionStartTimeout)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "gitlab-mcp-e2e-harness", Version: "1"}, c.clientOptions())
	// The recorder sits on the client rather than on the session, because that
	// is where the SDK keeps its middleware, and it is installed before
	// Connect so the handshake is inside it rather than beside it.
	client.AddSendingMiddleware(c.recordSending())
	client.AddReceivingMiddleware(c.recordReceiving())

	transport, err := c.transport(lifetime)
	if err != nil {
		return err
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connecting to the %s server: %w\nserver stderr:\n%s", c.label, err, c.proc.stderrTail())
	}

	c.mu.Lock()
	c.session = session
	c.mu.Unlock()
	return nil
}

// transport returns the transport this session's configuration asks for.
//
// The two differ in who starts the child. Over stdio the transport does, since
// the pipes it speaks over are the process's own; over HTTP the process has to
// be listening before a client can connect, so it is started and waited for
// here and the transport is a client pointed at the address.
func (c *sessionConn) transport(lifetime context.Context) (mcp.Transport, error) {
	if c.cfg.Transport != TransportHTTP {
		return c.proc.transport(lifetime), nil
	}
	addr, err := freeLoopbackAddr(lifetime)
	if err != nil {
		return nil, fmt.Errorf("starting the %s server: %w", c.label, err)
	}
	transport, err := c.proc.httpTransport(lifetime, addr)
	if err != nil {
		return nil, fmt.Errorf("starting the %s server: %w", c.label, err)
	}
	return transport, nil
}

// clientOptions builds the client for this session: what it answers an
// elicitation request with, and where a resource-updated notification goes.
func (c *sessionConn) clientOptions() *mcp.ClientOptions {
	options := &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			if req != nil && req.Params != nil {
				c.notifier.deliver(req.Params.URI)
			}
		},
		// Progress arrives on the SDK's own goroutine while the call that
		// asked for it is still in flight, so it is collected against the
		// token rather than handed to whoever is waiting: the caller reads it
		// once the call returns.
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			if req != nil && req.Params != nil {
				c.progress.deliver(req.Params)
			}
		},
	}
	switch c.cfg.Elicitation {
	case ElicitationAutoAccept:
		options.ElicitationHandler = c.recordingElicitationHandler(acceptElicitation)
	case ElicitationScripted:
		options.ElicitationHandler = c.recordingElicitationHandler(c.cfg.Responder)
	default:
		// ElicitationNone: no handler, so the client advertises no elicitation
		// capability and the server fails closed rather than prompting.
	}
	return options
}

// recordingElicitationHandler wraps an elicitation handler so the request is
// written down before it is answered.
//
// The receiving middleware cannot do this. The SDK delivers an elicitation to
// the handler these options carry, not as an inbound request through the
// method chain, so the middleware's elicitation/create case never fired and
// every interactive flow ran uncounted: the coverage report read zero
// elicitations while the flows themselves were passing. Recording here is the
// one place that sees every one of them, whatever the policy answered.
func (c *sessionConn) recordingElicitationHandler(
	next func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error),
) func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	if next == nil {
		return nil
	}
	return func(ctx context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		c.recordElicitation(req)
		return next(ctx, req)
	}
}

// acceptElicitation answers every request by accepting it, filling in the
// properties the requested schema demands.
//
// It used to answer with empty content, on the reasoning that a confirmation
// prompt reads no field back. That was wrong twice over, and the policy could
// never have worked: the SDK validates the accepted content against the
// requested schema before handing it to the server and before applying the
// schema's defaults (go-sdk mcp/client.go:591-598), and every schema this
// server sends marks its one property required (internal/elicitation/schemas.go
// lines 50, 78, 107, 150, 202 and 258). An empty accept therefore failed the
// whole call with InvalidParams rather than approving anything.
//
// The value chosen for each type is the one that means "yes, go ahead": true
// for the confirmation boolean, the first option of an enum, and the empty
// string for free text, which is the only answer a policy that asks nobody can
// honestly give. A flow that needs particular values scripts them instead.
func acceptElicitation(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	content := map[string]any{}
	if req != nil && req.Params != nil {
		content = acceptedContent(req.Params.RequestedSchema)
	}
	return &mcp.ElicitResult{Action: "accept", Content: content}, nil
}

// acceptedContent builds the answer an auto-accepting client gives one
// requested schema: a value for every property the schema requires, and
// nothing else.
//
// The schema arrives as whatever the server marshaled, which on this side of
// the wire is a map. Anything it cannot read it answers nothing for, leaving
// the SDK's own validation to report a shape this policy cannot satisfy rather
// than guessing at one.
func acceptedContent(schema any) map[string]any {
	content := map[string]any{}
	object, isObject := schema.(map[string]any)
	if !isObject {
		return content
	}
	properties, _ := object["properties"].(map[string]any)
	for _, name := range requiredNames(object["required"]) {
		property, _ := properties[name].(map[string]any)
		content[name] = acceptedValue(property)
	}
	return content
}

// requiredNames reads the required property names off a schema, which arrive
// as a JSON array of strings.
func requiredNames(required any) []string {
	items, isList := required.([]any)
	if !isList {
		return nil
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name, isString := item.(string); isString {
			names = append(names, name)
		}
	}
	return names
}

// acceptedValue is the value this policy gives one property.
func acceptedValue(property map[string]any) any {
	kind, _ := property["type"].(string)
	options, _ := property["enum"].([]any)
	switch kind {
	case "boolean":
		return true
	case "integer", "number":
		return 0
	case "array":
		if choices := itemOptions(property, options); len(choices) > 0 {
			return []any{choices[0]}
		}
		return []any{}
	default:
		if len(options) > 0 {
			return options[0]
		}
		return ""
	}
}

// itemOptions reads the values a multi-select property admits.
//
// The specification puts them on the item schema, so a server that follows it
// sends `items.enum` and nothing on the array itself; reading only the array's
// own enum answered such a property with an empty selection, which a required
// multi-select refuses. The array's enum is still accepted, since a schema
// this policy can read is better answered than declined.
func itemOptions(property map[string]any, own []any) []any {
	items, isObject := property["items"].(map[string]any)
	if !isObject {
		return own
	}
	options, isList := items["enum"].([]any)
	if !isList || len(options) == 0 {
		return own
	}
	return options
}

// client returns the live MCP session.
func (c *sessionConn) client() *mcp.ClientSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session
}

// restart replaces a child that has died with a fresh one of the same shape.
//
// It is what keeps one panicking handler from failing every later test of the
// run: the process is gone, the session with it, and the next call would
// otherwise find a closed pipe forever. The served set is not re-checked,
// because it was checked when this shape first started and the child is
// started from the same environment.
func (c *sessionConn) restart() error {
	c.mu.Lock()
	if c.session != nil {
		_ = c.session.Close()
		c.session = nil
	}
	c.mu.Unlock()
	return c.connect()
}

// close ends the session and the child behind it.
//
// The SDK's Close reports the error the reaper already collected, since both
// wait on the same process; it is discarded for that reason and not out of
// indifference.
func (c *sessionConn) close() {
	c.mu.Lock()
	session := c.session
	c.session = nil
	c.mu.Unlock()

	if session != nil {
		_ = session.Close()
	}
}

// alive reports whether the child behind this session is still running.
func (c *sessionConn) alive() bool { return c.proc.alive() }

// failureContext describes a dead child, for a call that failed because of it.
func (c *sessionConn) failureContext() string {
	if c.alive() {
		return ""
	}
	return fmt.Sprintf("\nthe %s server is no longer running (%s)\nserver stderr:\n%s",
		c.label, c.proc.exitStatus(), c.proc.stderrTail())
}
