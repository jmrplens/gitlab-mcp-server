package main

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// rules is everything the gate matches the code against that is not the
// register itself: which packages it leaves out, which calls and literals
// read as a limit, which words read as a share, and how a refusal, a charge
// and a setting are spelled in this program. A test replaces parts of it to
// point a rule at a fixture.
type rules struct {
	// leave are the loaded packages no rule reads, module-relative; a
	// trailing "/..." takes the tree below.
	leave []string
	// lexicon are the words that make a package-level name read as a limit
	// (G10(c)), compared with each camel-case word of the name.
	lexicon []string
	// constructors are the functions that build a limit, by full name
	// (G10(a)). A make of a chan struct{} with a capacity that is not the
	// literal 0 or 1 is one too, without being listed. limitTypes are the
	// options types a literal of which sets a limit the same way.
	constructors []string
	limitTypes   []string
	// statusWriters are the calls that write an HTTP status outside a gate
	// literal, each with the index of its status argument (G10(b)).
	statusWriters map[string]int
	// refusalTypes are the literal types a refusal is built from, and
	// gateType the one the gate writes before the SDK sees a request.
	refusalTypes []refusalType
	gateType     string
	// toolResult is the result type a tool-error refusal returns, and
	// isErrorField its error flag.
	toolResult   string
	isErrorField string
	// formatters are the calls whose first argument is a format string,
	// which a prefix is read against only up to its first verb.
	formatters []string
	// policyCodes are the codes that make a refusal literal read as a
	// limit's, and limitStatuses the statuses that do (G10(b)).
	policyCodes   []int
	limitStatuses []int
	// headerRetryAfter and headerChallenge are the two headers G8 reads, in
	// canonical form, and retryAfterReads the names a Retry-After value must
	// read for each source a row can declare.
	headerRetryAfter string
	headerChallenge  string
	retryAfterReads  map[tenancy.RetryAfter][]string
	// lowerCharges are the calls that spend an authentication budget, and
	// chargeCallers the only declarations allowed to make them (G7).
	lowerCharges  []string
	chargeCallers []string
	// shareWords are the words that describe a number as a share, and
	// shareInvariant the invariant whose finding excuses one (G13).
	shareWords     []string
	shareInvariant string
	// envPrefix, envNames, envReaders, envExpanders and configFinding are
	// G14's: the prefix every setting carries, the list of names the
	// configuration package reads, the calls that read a variable directly,
	// those that read every variable a template names, and the finding a row
	// reading one that way carries.
	envPrefix     string
	envNames      tenancy.Site
	envReaders    []string
	envExpanders  []string
	configFinding string
	// leafImports are the only packages the register may import, and server
	// the package that must already import each of them (G12).
	leafImports []string
	server      string
	// valueFiles and ruleFiles are the register files whose exported
	// constants and functions G6 holds to a reader.
	valueFiles []string
	ruleFiles  []string
}

// Import paths the rules name more than once.
const (
	serverPath     = goprogram.ModulePath + "/cmd/server"
	serverpoolPath = goprogram.ModulePath + "/internal/serverpool"
	mcpPath        = "github.com/modelcontextprotocol/go-sdk/mcp"
)

// productionRules are the rules `make check-tenancy` applies.
func productionRules() rules {
	subscriptionsPath := goprogram.ModulePath + "/internal/subscriptions"
	oauthPath := goprogram.ModulePath + "/internal/oauth"
	return rules{
		// The packages the server never links, which
		// TestDependencies_TestSupport_NeverReachesTheServerBinary holds out
		// of the binary: a refusal-shaped literal there answers no caller.
		leave: []string{"internal/testutil/...", "internal/graphqlschema", "internal/freshness"},
		lexicon: []string{
			"max", "min", "limit", "ceiling", "cap", "capacity", "budget", "burst", "window", "timeout", "ttl",
			"interval", "backoff", "divisor", "factor", "cooldown", "lease", "lifetime", "age", "size", "probes",
			"retry", "retries", "jitter", "fraction", "ladder",
		},
		constructors: []string{
			"golang.org/x/time/rate.NewLimiter",
			goprogram.ToolutilPath + ".NewRateLimiter",
			subscriptionsPath + ".NewWatcherGate",
			serverpoolPath + ".NewAuthRateLimiter",
			serverpoolPath + ".NewDistinctTokenBudget",
			oauthPath + ".NewRejectedTokens",
			serverpoolPath + ".WithMaxSize",
			serverpoolPath + ".WithIdleTimeout",
			serverpoolPath + ".WithRevalidateInterval",
			serverpoolPath + ".WithMaxCredentialAge",
			subscriptionsPath + ".New",
			oauthPath + ".NewGitLabVerifier",
			oauthPath + ".NewGitLabVerifierFor",
		},
		limitTypes: []string{subscriptionsPath + ".Options"},
		statusWriters: map[string]int{
			"net/http.Error":                      2,
			"net/http.ResponseWriter.WriteHeader": 0,
		},
		refusalTypes: []refusalType{
			// jsonrpc.Error is an alias of this type, which is what a
			// literal of it carries once the alias is resolved.
			{name: "github.com/modelcontextprotocol/go-sdk/internal/jsonrpc2.WireError", code: "Code", message: "Message"},
			{name: goprogram.ToolutilPath + ".CodedError", code: "code"},
			{name: serverPath + ".gateFailure", code: "code", status: "status", message: "message", header: "header"},
		},
		gateType:      serverPath + ".gateFailure",
		toolResult:    mcpPath + ".CallToolResult",
		isErrorField:  "IsError",
		formatters:    []string{"fmt.Sprintf", "fmt.Errorf"},
		policyCodes:   []int{tenancy.CodeTooManyRequests, tenancy.CodeUnauthorized, tenancy.CodeForbidden, tenancy.CodeUnavailable, tenancy.CodeServerBusyLegacy},
		limitStatuses: []int{429, 503},

		headerRetryAfter: "Retry-After",
		headerChallenge:  "WWW-Authenticate",
		retryAfterReads: map[tenancy.RetryAfter][]string{
			tenancy.RetryAfterLongestBlock:    {"retryAfterSeconds"},
			tenancy.RetryAfterFixed:           {"upstreamRetryAfter"},
			tenancy.RetryAfterUpstreamOrFixed: {"upstreamRetryAfter", "RetryAfter"},
		},

		lowerCharges: []string{
			serverpoolPath + ".AuthRateLimiter.RecordFailure",
			serverPath + ".transportBudget.charge",
			serverpoolPath + ".DistinctTokenBudget.Charge",
		},
		chargeCallers: []string{
			"cmd/server:mcpServerGate.chargeFailure",
			"cmd/server:bearerGuard.recordFailure",
			"cmd/server:transportBudget.charge",
		},

		shareWords:     []string{"fair", "fairness", "quota", "entitlement"},
		shareInvariant: "INV-003",

		envPrefix:     "GITLAB_MCP_",
		envNames:      tenancy.Site{Pkg: "internal/config", Name: "prefixedNames"},
		envReaders:    []string{"os.Getenv", "os.LookupEnv", "syscall.Getenv", "syscall.LookupEnv"},
		envExpanders:  []string{"os.ExpandEnv"},
		configFinding: "F-13",

		leafImports: []string{"errors", "fmt", "strings", "time"},
		server:      "cmd/server",
		valueFiles:  []string{"values.go", "codes.go"},
		ruleFiles:   []string{"meter.go", "busy.go", "budget.go"},
	}
}

// The categories an exemption may give, each with what it covers.
const (
	categoryTransport     = "transport"
	categoryShutdown      = "shutdown"
	categoryVocabulary    = "vocabulary"
	categoryLogging       = "logging"
	categoryToolArgument  = "tool-argument"
	categoryParsing       = "parsing"
	categoryRoundTrip     = "round-trip"
	categoryServerState   = "server-state"
	categoryProtocol      = "protocol"
	categoryUninventoried = "uninventoried"
	categoryTestSupport   = "test-support"
)

// categories are the reasons an exemption may give. An exemption naming any
// other category is reported rather than trusted, since a category nobody
// defined is an excuse nobody reviewed.
var categories = map[string]string{
	categoryTransport:    "HTTP server timeouts, socket dials and header sizes",
	categoryShutdown:     "shutdown and drain waits",
	categoryVocabulary:   "ending and cause names, error sentinels",
	categoryLogging:      "log and refusal-log throttles",
	categoryToolArgument: "the polling and GraphQL page bounds a tool call carries",
	categoryParsing:      "probe bytes, the dotenv caps, float bounds",
	categoryRoundTrip:    "how long one request to GitLab may take, and how often one client re-tries its initialization",
	categoryServerState:  "refusals about the process, not a caller",
	categoryProtocol: "protocol vocabulary: a revision the MCP specification names, or a helper carrying a " +
		"protocol code its callers choose, where every caller passes a protocol code",
	categoryUninventoried: "a decision the gate surfaced that no row of the register declares, " +
		"held here until the specification decides whether it is a row",
	categoryTestSupport: "a name a package the server links declares for its own tests, which read a row's value or build its limit through it; the running server reads that value, or builds that limit, through a site a row declares",
}

// exemption is one declaration shaped like a limit that decides nothing about
// a caller: why, in a category and in words.
type exemption struct {
	category string
	reason   string
}

// notADecision is the exemption table: the declarations G10 would read as a
// limit and that are not tenant decisions, keyed `package:Name` the way the
// register names a site.
//
// Each was judged, and the judgement is the entry: an exemption is a claim
// that the number bounds the transport, the process or one request's parsing
// rather than what a caller may hold or spend. An entry that answers nothing
// is reported, and so is one naming a declaration the register also names as
// a site, since a declaration is one or the other.
var notADecision = map[string]exemption{
	// The server's own listener and the connections it holds.
	"cmd/server:baseHTTPMaxHeaderBytes":    {categoryTransport, "the largest request header the HTTP listener reads, whoever sends it"},
	"cmd/server:baseHTTPReadHeaderTimeout": {categoryTransport, "how long the listener waits for a request's headers"},
	"cmd/server:baseHTTPReadTimeout":       {categoryTransport, "how long the listener waits for a whole request"},
	"cmd/server:baseHTTPWriteTimeout":      {categoryTransport, "how long the listener takes to write a response"},
	"cmd/server:defaultHTTPIdleTimeout":    {categoryTransport, "how long an idle keep-alive connection is kept"},
	"cmd/server:idleTimeoutDisabled":       {categoryTransport, "the sentinel standing for no idle timeout, which Go would otherwise read as the read timeout"},
	"cmd/server:sseKeepAliveInterval":      {categoryTransport, "how often an open event stream is sent a comment so an idle proxy does not cut it"},
	"cmd/server:corsMaxAge":                {categoryTransport, "how long a browser may cache a preflight answer, a header value"},
	"cmd/server:maxUnixPathLen":            {categoryTransport, "the longest unix socket path the kernel's address can hold"},
	"cmd/server:staleSocketDialTimeout":    {categoryTransport, "how long startup waits to learn whether a leftover socket still has a listener"},
	"cmd/server:pprofReadHeaderTimeout":    {categoryTransport, "the loopback-only profiling listener's header timeout"},
	"cmd/server:probeTimeout":              {categoryTransport, "one attempt of the --probe health check, a separate process asking this one"},
	"cmd/server:probeBudget":               {categoryTransport, "the whole --probe run, bounded below the image's HEALTHCHECK"},

	// Stopping.
	"cmd/server:httpShutdownTimeout":      {categoryShutdown, "how long an HTTP shutdown waits for requests in flight"},
	"cmd/server:telemetryShutdownTimeout": {categoryShutdown, "how long the exporters get to flush at exit"},
	"cmd/server:stdioStartupDrainTimeout": {categoryShutdown, "how long a stdio shutdown waits for startup work it has already cancelled"},
	"internal/config:MaxDrainDelay":       {categoryShutdown, "the longest drain announcement an operator may configure before a stop"},

	// Names that read as a limit and are words.
	"cmd/server:endLifetimeReached":              {categoryVocabulary, "the watch-end reason a listen is given, whose rows are END-002 and HLD-007"},
	"cmd/server:headerRetryAfter":                {categoryVocabulary, "the name of the header a refusal's retry delay travels in"},
	"internal/gitlab:rateLimitResetHeader":       {categoryVocabulary, "the name of a header GitLab sends"},
	"internal/serverpool:CauseSizePressure":      {categoryVocabulary, "the name telemetry gives an eviction under size pressure"},
	"internal/subscriptions:ErrLifetimeExceeded": {categoryVocabulary, "the sentinel a watch ends with at its lifetime, whose row is HLD-007"},
	"internal/toolutil:ErrInvalidRateLimit":      {categoryVocabulary, "the sentinel a malformed rate-limit setting is refused with"},

	// Throttles on what the server writes to its own log.
	"cmd/server:refusalLogWindow":             {categoryLogging, "how often one refusal message is logged at most"},
	"internal/toolutil:defaultThrottleWindow": {categoryLogging, "how often the rate limiter reports its refusals"},

	// Bounds a tool call's own arguments carry.
	"internal/toolutil:GraphQLMaxFirst":     {categoryToolArgument, "the largest GraphQL page a tool asks for"},
	"internal/toolutil:PollMinInterval":     {categoryToolArgument, "the shortest wait a polling tool accepts between reads"},
	"internal/toolutil:PollMaxInterval":     {categoryToolArgument, "the longest wait a polling tool accepts between reads"},
	"internal/toolutil:PollDefaultInterval": {categoryToolArgument, "the wait a polling tool uses when the call names none"},
	"internal/toolutil:PollMinTimeout":      {categoryToolArgument, "the shortest total wait a polling tool accepts"},
	"internal/toolutil:PollMaxTimeout":      {categoryToolArgument, "the longest total wait a polling tool accepts"},
	"internal/toolutil:PollDefaultTimeout":  {categoryToolArgument, "the total wait a polling tool uses when the call names none"},

	// Parsing untrusted bytes, and numeric edges.
	"cmd/server:maxIDProbeBytes":                 {categoryParsing, "how much of a refused body is read to recover its request id"},
	"internal/config:maxDotenvBytes":             {categoryParsing, "how much of an untrusted dotenv file is read to name its keys"},
	"internal/config:maxAnnouncedKeys":           {categoryParsing, "how many of those keys the warning names"},
	"internal/config:maxAnnouncedKeyRunes":       {categoryParsing, "how long a named key may be in the warning"},
	"internal/oauth:insufficientScopeLimit":      {categoryParsing, "how much of a 403 body is read to name its error code"},
	"internal/oauth:verificationBodyLimit":       {categoryParsing, "how much of a verification response is read"},
	"internal/toolutil:minInt64AsFloat":          {categoryParsing, "the lowest float that converts to an int64 exactly"},
	"internal/toolutil:maxInt64AsFloatExclusive": {categoryParsing, "the first float above the int64 range"},

	// One request to GitLab, and one client's re-initialization.
	"internal/gitlab:healthTimeout":              {categoryRoundTrip, "one direct health request made while a client initializes"},
	"internal/gitlab:initCooldown":               {categoryRoundTrip, "how often one client re-tries its lazy initialization against a recovering instance"},
	"internal/gitlab:responseHeaderTimeout":      {categoryRoundTrip, "how long one request waits for GitLab's headers"},
	"internal/gitlab:instanceLookupTimeout":      {categoryRoundTrip, "one DNS lookup of the instance the destination guard asks about"},
	"internal/gitlab:maxRedirects":               {categoryRoundTrip, "the redirects one request to GitLab may follow"},
	"internal/oauth:maxVerificationRedirects":    {categoryRoundTrip, "the redirects one token verification may follow"},
	"internal/serverpool:credentialCheckTimeout": {categoryRoundTrip, "one credential probe's round trip, which does not retry"},

	// Refusals about the process rather than a caller.
	"cmd/server:readinessGate.abandoned": {categoryServerState, "answers -32000 to a request that ended while the tool catalog was still building, a fact about the process"},
	"cmd/server:writeCardUnavailable":    {categoryServerState, "answers 503 while the server card is still being built, a fact about the process"},
	"cmd/server:healthHandler":           {categoryServerState, "answers /health with 503 while the process drains, which is what a balancer is asked to read"},

	// Names only their own package's tests reach.
	"cmd/server:authFailureLimit":      {categoryTestSupport, "the tests build guards directly rather than through a Config and read AUB-001's limit here; the server reads it through config.DefaultAuthFailureLimit, the row's alias, as the --auth-failure-limit default"},
	"internal/oauth:NewGitLabVerifier": {categoryTestSupport, "a wrapper of NewGitLabVerifierFor for one fixed instance that only the package's tests call, with a cache TTL of their own; the server builds its verifier with NewGitLabVerifierFor in registerOAuthMCPHandlers, from the configured TTL, and both are Enforce sites (ADM-002 and ADM-006)"},

	// Protocol vocabulary.
	"internal/elicitation:minMRTRProtocolVersion": {categoryProtocol, "the first MCP revision that requires the multi-round-trip flow, a date the specification names"},
	"internal/toolutil:CodedError.Unwrap":         {categoryProtocol, "exposes the code a CodedError carries, which only InvalidParams and InternalError set, to -32602 and -32603"},
	"internal/toolutil:coded":                     {categoryProtocol, "builds the CodedError InvalidParams and InternalError return, with -32602 and -32603"},

	// A decision no row declares.
	"internal/subscriptions:settledFactor": {categoryUninventoried, "how much slower a settled resource is polled than the base cadence; " +
		"it belongs with the cadence HLD-007 holds, and when the register landed only the base and minimum intervals were made that row's values"},
}

// checkExemptions holds the exemption table to what it answered: an entry
// that excused nothing this run, or that names a category nobody defined, is
// a finding.
func (g *gate) checkExemptions() []Finding {
	var found []Finding
	for _, key := range sortedKeys(g.exempt) {
		entry := g.exempt[key]
		if _, known := categories[entry.category]; !known {
			found = append(found, Finding{Rule: "G10", Subject: key, Message: fmt.Sprintf("is exempted under the category %q, which is not one of the defined ones", entry.category)})
		}
		if _, used := g.used[key]; !used {
			found = append(found, Finding{Rule: "G10", Subject: key, Message: "is exempted, and this run found nothing limit-shaped there for it to answer"})
		}
	}
	return found
}
