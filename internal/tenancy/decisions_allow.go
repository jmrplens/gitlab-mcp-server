package tenancy

// allowDecisions are the rows that answer "what may it hold or spend?" (spec:
// The five questions): the token buckets and what each method is charged to,
// the listen and watcher ceilings with their process partners, the watch
// lease, the pool's size, eviction and idle rules, the upstream retry policy,
// the telemetry identity policy and the lifetime of multi-round-trip request
// state.
//
// Three of them are class D: keyed on the entry while their own reason is
// about a GitLab user, so one tenant holding N credentials holds N units
// (RTC-001, RTC-005, HLD-003). Each carries the finding that records it;
// moving any of them to the tenant would be a change of policy, and would
// still leave it dividable by bots (spec: Two axes). RTC-003 was the fourth,
// its reason about the process, until RTC-007 gave it a process partner
// (issue 951).
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: its length is the number of decisions it declares.
func allowDecisions() []Decision {
	// heldRefusalPrefix is what every refusal of HLD-010 and HLD-011 begins
	// with, heldRefusal where HLD-011's in-band refusals are built, and
	// busyFailure the gate refusal both write.
	const heldRefusalPrefix = "This server is busy."
	heldRefusal := refuse(pkgServer, "heldRequestsRefusal")
	busyFailure := refuse(pkgServer, "processBusyFailure")
	busy := refuse(pkgServer, "listenLimits.busy")
	listenRefusal := Refusal{
		Methods: []string{"subscriptions/listen"}, Era: EraModern, Channel: RPC, Code: CodeServerBusyLegacy,
		Prefix: "too many open subscriptions/listen streams (", Answer: RetryLater, At: busy,
	}
	tooMany := refuse(pkgSubscriptions, "ErrTooManySubscriptions")
	wire := refuse(pkgServer, "wireSubscribeError")
	watcherRefusal := Refusal{
		Methods: subscribeMethods(), Channel: RPC, Code: CodeServerBusyLegacy,
		Prefix: "subscriptions: too many active subscriptions", Answer: RetryLater, At: tooMany, Via: wire,
	}
	rateLimitedError := refuse(pkgToolutil, "rateLimitedError")

	return []Decision{
		{
			ID: "RTC-001", Question: Allow, Kind: Rate, Class: ClassD, Disposition: Valued,
			Resource: "tools/call, resources/read, resources/subscribe, subscriptions/listen and prompts/get",
			Key:      KeyEntry, StdioKey: KeyProcess,
			// The reason names a per-token limit, and GitLab keeps the limit it
			// cites per user: the unit is misstated as well as disagreeing
			// with the key (F-02, issue 955).
			StatedUnit: KeyCredential, ReasonUnit: KeyTenant,
			Reason:   "the primary defense remains GitLab's own per-token rate limits",
			ReasonAt: reasonAt(pkgToolutil, "RateLimiter"),
			Values: []string{
				"ToolCallRateHTTP", "ToolCallRateEnvDefault", "ToolCallBurst", "ToolCallRateMax", "ToolCallBurstMax",
			},
			// Which methods draw on the bucket is MeterFor's answer.
			Functions: []string{"MeterFor"},
			Source:    Configurable, Flags: []string{"--rate-limit-rps", "--rate-limit-burst"},
			Envs: []string{"GITLAB_MCP_RATE_LIMIT_RPS", "GITLAB_MCP_RATE_LIMIT_BURST"},
			// A zero rate switches the bucket off. A zero burst beside a positive
			// rate is refused at startup by both validators rather than meaning
			// off, the INV-015 departure F-34 records (issue 958).
			Config: []string{"RateLimitRPS", "RateLimitBurst"}, Malformed: RefuseStartup, Zero: ZeroOff,
			// This row is how the server meets MCP's one mandatory limit,
			// "Rate limit tool invocations", and issue 959 decided where it
			// stands on it (F-19): on by default in HTTP mode, off by default
			// on stdio, where the process serves one caller and has no
			// co-tenant to protect, and switched on there by the variable,
			// since stdio accepts the flag and ignores it.
			Decided:  []string{"issue 959"},
			Findings: []string{"F-01", "F-02", "F-20", "F-21", "F-32", "F-34"},
			Refusals: []Refusal{
				{
					Methods: []string{"tools/call"}, Channel: ToolError, Prefix: "rate limit exceeded for ",
					Answer: RetryLater, At: refuse(pkgToolutil, "rateLimitedResult"),
				},
				{
					Methods: toolBucketRPCMethods(), Channel: RPC, Code: CodeTooManyRequests,
					Prefix: "rate limit exceeded for ", Answer: RetryLater, At: rateLimitedError,
				},
			},
			Sites: []Site{
				alias(pkgConfig, "DefaultHTTPRateLimitRPS", "ToolCallRateHTTP"),
				alias(pkgConfig, "DefaultRateLimitBurst", "ToolCallBurst"),
				alias(pkgConfig, "MaxRateLimitRPS", "ToolCallRateMax"),
				alias(pkgConfig, "MaxRateLimitBurst", "ToolCallBurstMax"),
				arg(pkgConfig, "Load", "parseFloatNonNegative", 1, 1, "ToolCallRateEnvDefault"),
				arg(pkgConfig, "loadOverlayAuthAndRate", "parseFloatNonNegative", 1, 1, "ToolCallRateEnvDefault"),
				alias(pkgToolutil, "rateLimitedErrorCode", "CodeTooManyRequests"),
				enforce(pkgToolutil, "NewRateLimiter"),
				enforce(pkgToolutil, "attachRateLimitFunc"),
				enforce(pkgToolutil, "ValidateRateLimit"),
				enforce(pkgConfig, "Config.validateDurationsAndRates"),
				enforce(pkgServer, "serverShell.newCredentialState"),
				refuse(pkgToolutil, "RateLimitRefusalPrefix"),
				refuse(pkgToolutil, "rateLimitRetrySuffix"),
				refuse(pkgToolutil, "rateLimitedResult"),
				rateLimitedError,
			},
		},
		{
			ID: "RTC-002", Question: Allow, Kind: Rate, Class: ClassU, Disposition: Valued,
			Resource: "completion/complete",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Reason:   "The specification asks for both",
			ReasonAt: reasonAt(pkgToolutil, "completionBurstFactor"),
			Values:   []string{"CompletionFactor"}, Source: Derived, Zero: ZeroNotApplicable,
			Functions: []string{"MeterFor"},
			Refusals: []Refusal{
				{
					Methods: []string{"completion/complete"}, Channel: EmptyCompletion, Answer: RetryLater,
					At: refuse(pkgToolutil, "attachRateLimitFunc"),
				},
			},
			// The middleware enforces it as well as writing its refusal: it is
			// where MeterFor sends completion/complete to this bucket.
			Sites: []Site{
				alias(pkgToolutil, "completionBurstFactor", "CompletionFactor"),
				enforce(pkgToolutil, "RateLimiter.scaled"),
				enforce(pkgToolutil, "attachRateLimitFunc"),
				refuse(pkgToolutil, "attachRateLimitFunc"),
			},
		},
		{
			ID: "RTC-003", Question: Allow, Kind: Rate, Class: ClassR, Disposition: Valued,
			Resource: "tools/list",
			Key:      KeyEntry, StdioKey: KeyProcess,
			// The reason is the processor every tenant shares, which RTC-007
			// bounds on the process beside this bucket (F-03, answered by
			// issue 951): what is left keyed on the entry is a ceiling on what
			// one credential lists, so the row is class R, as HLD-001 is.
			StatedUnit: KeyProcess, ReasonUnit: KeyProcess, ProtectsProcess: true, Partner: "RTC-007",
			Reason:   "spends instead the processor every tenant of this process is waiting for",
			ReasonAt: reasonAt(pkgToolutil, "methodToolsList"),
			Values:   []string{"CatalogDivisor"}, Source: Derived, Zero: ZeroNotApplicable,
			Functions: []string{"MeterFor"},
			Refusals: []Refusal{
				{
					Methods: []string{"tools/list"}, Channel: RPC, Code: CodeTooManyRequests,
					Prefix: "rate limit exceeded for ", Answer: RetryLater, At: rateLimitedError,
				},
			},
			// The middleware is where MeterFor sends tools/list to this bucket,
			// and where the server's own listings are exempted from it; the
			// listing's own charge is where the bucket is spent, after the
			// process's.
			Sites: []Site{
				alias(pkgToolutil, "catalogDivisor", "CatalogDivisor"),
				enforce(pkgToolutil, "RateLimiter.slowed"),
				enforce(pkgToolutil, "attachRateLimitFunc"),
				enforce(pkgToolutil, "catalogListing.serve"),
				rateLimitedError,
			},
		},
		{
			// Promoted: which method is charged to which bucket, and so which
			// is charged to none, is MeterFor's answer, and the middleware
			// switches on it.
			//
			// Issue 951 decided that initialize stays charged to none once
			// HLD-010 bounded the stateful sessions it opens: a price in the
			// opener's own rate would sit on a key a caller can mint (INV-003)
			// and multiply with every token it mints, so it would bound
			// nothing the process ceiling does not.
			ID: "RTC-004", Question: Allow, Kind: Rule, Class: ClassP, Disposition: Promoted,
			Resource: "initialize, resources/list, prompts/list and every other unmetered method",
			Key:      KeyRequest, StdioKey: KeyRequest,
			Functions: []string{"MeterFor"},
			Decided:   []string{"issue 951"},
			Sites:     []Site{enforce(pkgToolutil, "attachRateLimitFunc")},
		},
		{
			ID: "RTC-005", Question: Allow, Kind: Rate, Class: ClassD, Disposition: Valued,
			Resource: "polling after GitLab answered a watcher's read with 429",
			Key:      KeyEntry, StdioKey: KeyProcess,
			StatedUnit: KeyTenant, ReasonUnit: KeyTenant,
			Reason:   "the limit is enforced per user",
			ReasonAt: reasonAt(pkgSubscriptions, "ErrRateLimited"),
			Values:   []string{"WatchRateLimitPause", "WatchRateLimitPauseMax", "WatchRateLimitJitter"},
			Source:   Constant, Zero: ZeroNotApplicable,
			Findings: []string{"F-06", "F-07", "F-32"},
			Refusals: []Refusal{
				{
					Methods: subscribeMethods(), Channel: RPC, Code: CodeServerBusyLegacy,
					Prefix: "subscriptions: rate limited", Answer: RetryLater,
					At: refuse(pkgSubscriptions, "ErrRateLimited"), Via: wire,
				},
				{
					Methods: []string{"notifications/resources/updated"}, Channel: Silent, Answer: NoAnswer,
					At: refuse(pkgSubscriptions, "Manager.recordRateLimit"),
				},
			},
			Sites: []Site{
				alias(pkgSubscriptions, "rateLimitBackoff", "WatchRateLimitPause"),
				alias(pkgSubscriptions, "maxRateLimitBackoff", "WatchRateLimitPauseMax"),
				alias(pkgSubscriptions, "jitterFraction", "WatchRateLimitJitter"),
				enforce(pkgSubscriptions, "Manager.recordRateLimit"),
				refuse(pkgSubscriptions, "ErrRateLimited"),
				wire,
			},
		},
		{
			ID: "RTC-006", Question: Allow, Kind: Bound, Class: ClassQ, Disposition: Valued,
			Resource: "re-sends of a failed GitLab request, and the wait between them",
			Key:      KeyRequest, StdioKey: KeyRequest,
			Reason:   "without letting one caller multiply itself sixfold",
			ReasonAt: reasonAt(pkgGitLab, "maxRetries"),
			Values:   []string{"UpstreamRetries", "UpstreamRetryWaitMax", "UpstreamRetryStep"},
			Source:   Constant, Zero: ZeroNotApplicable,
			Findings: []string{"F-16", "F-32"},
			Sites: []Site{
				alias(pkgGitLab, "maxRetries", "UpstreamRetries"),
				alias(pkgGitLab, "maxRetryBackoff", "UpstreamRetryWaitMax"),
				alias(pkgGitLab, "retryBackoffStep", "UpstreamRetryStep"),
				enforce(pkgGitLab, "retryOptions"),
				enforce(pkgGitLab, "retryOptionsWithCeiling"),
				enforce(pkgGitLab, "retryBackoff.wait"),
			},
		},
		{
			// RTC-003's process partner (F-03, issue 951): a tools/list bucket
			// keyed on the process, counted in the tools a listing carries, so
			// the processor listings spend is bounded however many credentials
			// list. It is charged first, and hands its tools back when the
			// entry's own bucket refuses the listing, so either refusal costs
			// the other nothing (PAT-003).
			//
			// It follows the row it partners: consulted only where an entry's
			// listing bucket charges a request, and RTC-003's bucket is derived
			// from RTC-001's, so it is off when RTC-001's rate is zero. That is
			// switching one limit off with another (INV-015), and issue 951
			// decided it for a process partner, which is why the row records a
			// decision where a departure would record a finding.
			//
			// Its refusal is RTC-003's, word for word: the next action is the
			// same, and a sentence naming the process would tell a caller that
			// others are listing. A caller whose own bucket still held tools
			// can infer that much from being refused at all, which is the one
			// bit INV-019 accepts for a bound keyed on the process; the wording
			// adds nothing to it. The log line it writes is where an operator
			// tells the two apart.
			ID: "RTC-007", Question: Allow, Kind: Rate, Class: ClassP, Disposition: Valued,
			Resource: "tools/list across every credential, counted in the tools listed",
			Key:      KeyProcess, StdioKey: KeyProcess,
			StatedUnit: KeyProcess, ReasonUnit: KeyProcess, ProtectsProcess: true,
			Reason:   "only a bucket keyed on the process bounds the processor they share",
			ReasonAt: reasonAt(pkgToolutil, "processCatalog"),
			Values:   []string{"CatalogProcessRate", "CatalogProcessBurst"},
			// Which method draws on it is MeterFor's answer, as for RTC-003.
			Functions: []string{"MeterFor"},
			Source:    Constant, Zero: ZeroNotApplicable, OffWith: "RTC-003", OffWithBy: "issue 951",
			AtCapacity: RefuseNewcomer,
			Decided:    []string{"issue 951"},
			Refusals: []Refusal{
				{
					Methods: []string{"tools/list"}, Channel: RPC, Code: CodeTooManyRequests,
					Prefix: "rate limit exceeded for ", Answer: RetryLater, At: rateLimitedError,
				},
			},
			// AttachRateLimitFunc hands the process's bucket to the middleware;
			// the server's own listings teach it what a listing costs, and the
			// listing's charge, refund and settlement are where it is counted.
			Sites: []Site{
				alias(pkgToolutil, "catalogProcessRate", "CatalogProcessRate"),
				alias(pkgToolutil, "catalogProcessBurst", "CatalogProcessBurst"),
				enforce(pkgToolutil, "processCatalog"),
				enforce(pkgToolutil, "newProcessCatalog"),
				enforce(pkgToolutil, "AttachRateLimitFunc"),
				enforce(pkgToolutil, "attachRateLimitFunc"),
				enforce(pkgToolutil, "catalogListing.learn"),
				enforce(pkgToolutil, "catalogListing.serve"),
				enforce(pkgToolutil, "RateLimiter.take"),
				enforce(pkgToolutil, "RateLimiter.debit"),
				enforce(pkgToolutil, "catalogCharge.refund"),
				rateLimitedError,
			},
		},
		{
			ID: "HLD-001", Question: Allow, Kind: Ceiling, Class: ClassR, Disposition: Valued,
			Resource: "open subscriptions/listen streams",
			Key:      KeyEntry, StdioKey: KeyProcess,
			// The stated unit is the entry's own, so the row is class R. The
			// word fairness breaks INV-003's vocabulary rule, which no field
			// can hold; the gate holds it in the comment (F-04, issue 960).
			StatedUnit: KeyEntry, ReasonUnit: KeyEntry, ProtectsProcess: true, Partner: "HLD-002",
			Reason:   "Per server is per pool entry, which is per token and instance, and bounds fairness",
			ReasonAt: reasonAt(pkgServer, "maxListenStreamsPerServer"),
			Values:   []string{"ListenStreamsPerCredential"},
			Source:   EnvOnly, Envs: []string{"GITLAB_MCP_MAX_LISTEN_STREAMS"}, Malformed: WarnKeepDefault,
			Zero: ZeroOffKeepsCount, AtCapacity: RefuseNewcomer,
			Decided:  []string{"issue 540"},
			Findings: []string{"F-04", "F-07", "F-13", "F-21"},
			Refusals: []Refusal{listenRefusal},
			// codeServerBusy is declared here, with the row whose layer moves
			// it: the listen ceilings are what it refuses with first.
			Sites: []Site{
				alias(pkgServer, "maxListenStreamsPerServer", "ListenStreamsPerCredential"),
				alias(pkgServer, "codeServerBusy", "CodeServerBusyLegacy"),
				enforce(pkgServer, "maxListenStreamsEnv"),
				enforce(pkgServer, "listenLimitsFromEnv"),
				enforce(pkgServer, "listenLimits.middleware"),
				busy,
			},
		},
		{
			// It refuses with HLD-001's refusal, naming its scope: a caller
			// counting its own streams learns that the process is at this
			// ceiling whatever the words say, and that one bit is what INV-019
			// accepts for a bound keyed on the process (issue 951).
			ID: "HLD-002", Question: Allow, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "open subscriptions/listen streams across every credential",
			Key:      KeyProcess, StdioKey: KeyProcess,
			StatedUnit: KeyProcess, ReasonUnit: KeyProcess, ProtectsProcess: true,
			Reason:   "so the process-wide one is what bounds the process",
			ReasonAt: reasonAt(pkgServer, "maxListenStreamsPerProcess"),
			Values:   []string{"ListenStreamsPerProcess"}, Source: Constant, Zero: ZeroNotApplicable,
			AtCapacity: RefuseNewcomer,
			Decided:    []string{"issue 540"},
			Findings:   []string{"F-07", "F-21"},
			Refusals:   []Refusal{listenRefusal},
			Sites: []Site{
				alias(pkgServer, "maxListenStreamsPerProcess", "ListenStreamsPerProcess"),
				enforce(pkgServer, "processListenStreams"),
				enforce(pkgServer, "listenLimits.middleware"),
				busy,
			},
		},
		{
			ID: "HLD-003", Question: Allow, Kind: Ceiling, Class: ClassD, Disposition: Valued,
			Resource: "resource watchers one credential's manager holds",
			Key:      KeyEntry, StdioKey: KeyProcess, Table: true, Partner: "HLD-004",
			// Sized so one GitLab user stays inside that user's budget, and
			// keyed per entry (F-05, issue 955).
			StatedUnit: KeyTenant, ReasonUnit: KeyTenant,
			Reason:   "ten watchers at a five-second interval would consume such a user's entire budget",
			ReasonAt: reasonAt(pkgSubscriptions, "DefaultMaxWatchers"),
			Values:   []string{"WatchersPerCredential"}, Source: Constant, Zero: ZeroSelectsDefault,
			AtCapacity: ReclaimOwnThenRefuse,
			Findings:   []string{"F-05", "F-07", "F-21", "F-34"},
			// Reclaiming ends the longest-demoted watch of the same manager,
			// whose listens are told so.
			Refusals: []Refusal{
				watcherRefusal,
				listenEnd("watcher_evicted", StartOver, refuse(pkgServer, "endOfWatcherEviction")),
			},
			// The manager is built per credential in newRuntime, which is what
			// keys the ceiling on the entry.
			Sites: []Site{
				alias(pkgSubscriptions, "DefaultMaxWatchers", "WatchersPerCredential"),
				enforce(pkgSubscriptions, "Manager.Subscribe"),
				enforce(pkgSubscriptions, "Manager.evictDemotedLocked"),
				enforce(pkgSubscriptions, "Options.withDefaults"),
				enforce(pkgServer, "subscriptionShape.newRuntime"),
				tooMany, wire,
			},
		},
		{
			// It refuses with HLD-003's refusal, word for word, so a caller
			// under its own ceiling learns only that the process is at this
			// one, the one bit INV-019 accepts for a bound keyed on the process
			// (issue 951).
			ID: "HLD-004", Question: Allow, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "resource watchers across every credential",
			Key:      KeyProcess, StdioKey: KeyProcess,
			StatedUnit: KeyProcess, ReasonUnit: KeyProcess, ProtectsProcess: true,
			Reason:   "only this one bounds the process",
			ReasonAt: reasonAt(pkgServer, "maxWatchersPerProcess"),
			Values:   []string{"WatchersPerProcess"}, Source: Constant, Zero: ZeroNotApplicable,
			AtCapacity: RefuseNewcomer,
			Decided:    []string{"issue 561"},
			Findings:   []string{"F-07", "F-21"},
			Refusals:   []Refusal{watcherRefusal},
			Sites: []Site{
				alias(pkgServer, "maxWatchersPerProcess", "WatchersPerProcess"),
				enforce(pkgServer, "processWatchers"),
				enforce(pkgSubscriptions, "WatcherGate.acquire"),
				tooMany, wire,
			},
		},
		{
			ID: "HLD-007", Question: Allow, Kind: Lifetime, Class: ClassQ, Disposition: Valued,
			Resource: "a watch's lease, its polling cadence and its lifetime",
			Key:      KeyRequest, StdioKey: KeyRequest,
			Values: []string{
				"WatchLease", "WatchSlowInterval", "WatchMaxLifetime", "WatchBaseInterval", "WatchMinInterval",
			},
			Source: Constant, Zero: ZeroNotApplicable,
			Decided: []string{"ADR-0015"},
			Refusals: []Refusal{
				listenEnd("lifetime_reached", StartOver, refuse(pkgServer, "endOfLifetime")),
			},
			Sites: []Site{
				alias(pkgSubscriptions, "DefaultLease", "WatchLease"),
				alias(pkgSubscriptions, "DefaultSlowInterval", "WatchSlowInterval"),
				alias(pkgSubscriptions, "DefaultMaxLifetime", "WatchMaxLifetime"),
				alias(pkgSubscriptions, "DefaultBaseInterval", "WatchBaseInterval"),
				alias(pkgSubscriptions, "DefaultMinInterval", "WatchMinInterval"),
				enforce(pkgServer, "renewOnActivity"),
			},
		},
		{
			// The ceiling on the stateful sessions the process keeps, across
			// every credential (issue 951, answering the half of F-31 HLD-011
			// left; this row was the decision by absence that recorded it). On
			// --stateless=false the SDK keeps every session a client opens,
			// with the goroutines serving it and its owner record (IDN-010),
			// until the client deletes it, the pool evicts its credential or it
			// has sat idle for --session-timeout, and initialize is metered to
			// no bucket (RTC-004), so a caller could open sessions as fast as
			// it could post. It stands alone, keyed on the process, for
			// HLD-011's reason.
			//
			// Standing alone is what makes it cheap to fill, and that cost is
			// new: one credential's initializes, which spend no rate and hold
			// no connection, can take every slot, and a session nobody deletes
			// then holds its slot for --session-timeout, thirty minutes by
			// default and up to a day, while every other tenant's initialize is
			// refused. Before this ceiling an idle session refused nobody, and
			// what exhausted the process was a thousand held streams. Issue 951
			// decided to leave that cost where it is: no per-credential ceiling
			// stands beside it, as HLD-001 stands beside HLD-002, and initialize
			// stays unmetered (RTC-004), because the credential is a mintable
			// key (INV-003) and a per-credential number, or a price in the
			// opener's own rate, multiplies with every token a caller mints.
			//
			// Its value is a share of HLD-011's, the half issue 951 confirmed:
			// the held-call ceiling divided by SessionHeldDivisor, 96 sessions
			// under a hard limit of 1024, and half of the fallback's figure
			// where the platform has no limit to read. An idle session holds
			// no connection, and the one
			// it can hold open is its standalone stream, whose held slot
			// (HLD-011) the session takes with its own and keeps until it ends,
			// so the sessions take at most half of the held slots, the
			// descriptor budget HLD-011 is sized from holds as it was, and a
			// session the process keeps is never refused its stream. It bounds
			// descriptors, and memory only where the limit is small: an idle
			// session costs 88 to 110 KiB of resident set and four goroutines,
			// which the descriptor limit does not raise, so under the 524288 a
			// systemd service gets, 114560 idle sessions come to about ten to
			// twelve GiB, and there the memory limit the process runs under (a
			// container's, or a systemd unit's MemoryMax) bounds them where one
			// is set; a unit without MemoryMax is bounded only by the host.
			// Issue 951 decided that no fixed cap stands beside the derived
			// one: the memory limit the process runs under bounds that memory
			// where one is set, which is the case the decision accepts, and a
			// cap no flag moves would be sized for one host.
			//
			// It counts a session from the POST that opens one, which is any
			// POST carrying no session id on a deployment that keeps sessions
			// and on a revision that has sessions: the SDK creates a session
			// for every such POST and closes it with the POST unless its
			// initialize completed. A POST on 2026-07-28 or later is left to
			// the SDK, which answers it, a discover included, with the
			// revisions the transport serves, so the client falls back, and
			// closes its session with it; the refusal is of the legacy era
			// alone. The gate takes the slot after
			// admission, the departure from PAT-003 HLD-011 records for the
			// same reason, and it costs more here: at the pool's bound a
			// newcomer's admission evicts another credential's quiet entry
			// (POL-002), an entry holding only stateful sessions is quiet
			// (POL-003), and evicting it ends those sessions, so a newcomer
			// this row then refuses has taken from another key exactly what
			// the row counts, a cost issue 951 accepted with the order. The
			// gate refuses past the ceiling with a 503 before the SDK creates
			// anything, and the first request dispatched on the new session
			// keeps the slot until the session ends. With --session-timeout=0
			// (END-005) that is when a client
			// deletes it or the pool evicts its credential, after
			// --pool-idle-timeout without a request (never with 0) or to make
			// room at --max-http-clients, and startup says so. The refusal is
			// HLD-011's, in words that name no bound, which is INV-019's one
			// bit, as issue 951 confirmed for this row; the log line says
			// which ceiling refused.
			ID: "HLD-010", Question: Allow, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "stateful sessions across every credential",
			Key:      KeyProcess, StdioKey: KeyNone,
			StatedUnit: KeyProcess, ReasonUnit: KeyProcess, ProtectsProcess: true,
			Reason:   "only a ceiling keyed on the process bounds the sessions it keeps",
			ReasonAt: reasonAt(pkgServer, "processStatefulSessions"),
			Values:   []string{"SessionHeldDivisor"},
			Source:   Derived, Zero: ZeroNotApplicable,
			AtCapacity: RefuseNewcomer,
			Decided:    []string{"issue 951"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Era: EraLegacy, Channel: Gate, Code: CodeUnavailable, Status: 503,
					RetryAfter: RetryAfterFixed, Prefix: heldRefusalPrefix, Answer: RetryLater,
					At: busyFailure,
				},
			},
			Sites: []Site{
				alias(pkgServer, "sessionHeldDivisor", "SessionHeldDivisor"),
				enforce(pkgServer, "statefulSessionsFor"),
				enforce(pkgServer, "processStatefulSessions"),
				enforce(pkgServer, "mcpServerGate.opensSession"),
				enforce(pkgServer, "mcpServerGate.takeSessionSlot"),
				enforce(pkgServer, "claimSessionSlot"),
				enforce(pkgServer, "sessionSlot.release"),
				enforce(pkgServer, "statefulSessionsMiddleware"),
				enforce(pkgServer, "newServerShell"),
				enforce(pkgServer, "mcpServerGate.middleware"),
				enforce(pkgServer, "registerOAuthMCPHandlers"),
				enforce(pkgServer, "registerLegacyMCPHandlers"),
				enforce(pkgServer, "streamableHTTPOptions"),
				enforce(pkgServer, "warnStatefulSessions"),
				busyFailure,
			},
		},
		{
			// The ceiling on the calls the process holds open, across every
			// credential (issue 951, answering F-31 for them; HLD-010 answers
			// it for the stateful sessions). It stands alone, keyed on the process
			// with no per-caller number beside it: a per-caller one would
			// multiply by however many credentials a caller mints (INV-003),
			// and would bound nothing INV-018 asks of this row. Issue 951
			// decided so, rather than setting a per-credential ceiling beside
			// it as HLD-001 stands beside HLD-002, and accepted that one
			// credential can fill it where the limit is small. It bounds
			// descriptors and not memory, about 190 KiB a held call, which the
			// memory limit the process runs under (a container's, or a systemd
			// unit's MemoryMax) bounds where one is set: issue 951 also decided
			// that the server keeps no memory cap of its own, and a unit
			// without MemoryMax is bounded only by the host.
			//
			// Its value is derived rather than written: the descriptors the
			// process may open, read once at startup, less an eighth spare and
			// the listen streams HLD-002 reserves, divided by what one held
			// call costs. Where the platform has no limit to read it is sized
			// against FallbackDescriptorLimit, which issue 951 kept rather than
			// no ceiling, so Windows, the platform with none, is not left
			// unbounded by default.
			// Issue 951 decided the derivation too: no operator setting moves
			// the figure, and raising the hard descriptor limit raises it
			// together with what it protects. The runtime raises the soft
			// limit to the hard one before main, so the limit read is the hard
			// one: 192 under 1024, and far above anything a rate-limited
			// caller reaches under a default systemd or container limit.
			//
			// It counts the calls that reach GitLab (MeterFor's tool-call and
			// completion buckets), a listen aside, since HLD-001 and HLD-002
			// count that one. The calls are counted where the SDK dispatches
			// them, so each call of a batch is counted, a response the client
			// sends to a request of the server's own is not, and the method is
			// the one the SDK read out of the body; the gate takes the slot
			// itself only for a POST whose headers the SDK holds to the body
			// (protocol 2026-07-28 or later), and refuses it with a 503 before
			// the SDK reads it. It also counts the one request a stateful
			// session holds open that is not a call, its standalone stream (a
			// GET on --stateless=false), and counts it from the POST that opens
			// the session until the session ends, whether the stream is open or
			// not: that POST takes the stream's slot with the session's
			// (HLD-010) and is refused the same way when none is free, and the
			// GET takes none. A GET refused at a full ceiling would not be
			// asked for again, since the SDK's client gives a refused stream
			// up, and the session would lose every message the server sends it
			// outside a response while it carried on; a refused initialize is
			// a failure the client sees. HLD-010 keeps the sessions to half of
			// this ceiling, so they take at most half of the slots. Either way
			// the slot is taken after admission,
			// which departs from PAT-003 (take the process slot before
			// anything per key): a slot taken before admission would let a
			// caller with no credential hold one for as long as its
			// verification takes, and issue 951 accepted the departure for that
			// reason. What a refused newcomer has spent by then is its
			// admission: in legacy mode the pool entry its first call
			// builds (POL-006's probe of GitLab, the tier, the catalog), which
			// at the pool's bound evicts another credential's quiet entry
			// (POL-002), and in oauth mode one of ADM-014's verification slots.
			// It never spends the credential's rate.
			//
			// Its refusals say only that the process is busy, in words that
			// name no bound, no figure and no caller: with no per-caller
			// ceiling beside it any refusal of it tells its caller that the
			// process is full, and no wording can take that bit back, which is
			// the one INV-019 accepts for a bound keyed on the process, as
			// issue 951 confirmed for this row. The log line is the one place
			// that says which bound refused. Whether a held call makes an entry
			// busy is POL-003's answer and did not change: it does not.
			ID: "HLD-011", Question: Allow, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "calls the process holds open across every credential",
			Key:      KeyProcess, StdioKey: KeyNone,
			StatedUnit: KeyProcess, ReasonUnit: KeyProcess, ProtectsProcess: true,
			Reason:    "only a ceiling keyed on the process bounds the process",
			ReasonAt:  reasonAt(pkgServer, "processHeldRequests"),
			Values:    []string{"HeldRequestDescriptors", "DescriptorSpareDivisor", "FallbackDescriptorLimit"},
			Functions: []string{"MeterFor"},
			Source:    Derived, Zero: ZeroNotApplicable,
			AtCapacity: RefuseNewcomer,
			Decided:    []string{"issue 951"},
			Refusals: []Refusal{
				{
					// A POST on protocol 2026-07-28 or later, and a POST that
					// would open a stateful session, which only earlier
					// revisions do, when no slot is left for its stream.
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnavailable, Status: 503,
					RetryAfter: RetryAfterFixed, Prefix: heldRefusalPrefix, Answer: RetryLater,
					At: busyFailure,
				},
				{
					Methods: []string{"tools/call"}, Channel: ToolError, Prefix: heldRefusalPrefix, Answer: RetryLater,
					At: heldRefusal,
				},
				{
					Methods: []string{"resources/read", "resources/subscribe", "prompts/get"}, Channel: RPC,
					Code: CodeTooManyRequests, Prefix: heldRefusalPrefix, Answer: RetryLater, At: heldRefusal,
				},
				{
					Methods: []string{"completion/complete"}, Channel: EmptyCompletion, Answer: RetryLater,
					At: heldRefusal,
				},
			},
			Sites: []Site{
				alias(pkgServer, "heldRequestDescriptors", "HeldRequestDescriptors"),
				alias(pkgServer, "descriptorSpareDivisor", "DescriptorSpareDivisor"),
				alias(pkgServer, "fallbackDescriptorLimit", "FallbackDescriptorLimit"),
				alias(pkgServer, "heldRefusalCode", "CodeTooManyRequests"),
				enforce(pkgServer, "heldRequestsFor"),
				enforce(pkgServer, "heldRequestsCeiling"),
				enforce(pkgServer, "descriptorLimit"),
				enforce(pkgServer, "descriptorLimitFrom"),
				enforce(pkgServer, "processHeldRequests"),
				enforce(pkgServer, "processSlots.acquire"),
				enforce(pkgServer, "holdsOpen"),
				enforce(pkgServer, "gateCountsRequest"),
				enforce(pkgServer, "claimGateSlot"),
				enforce(pkgServer, "heldRequestsMiddleware"),
				enforce(pkgServer, "mcpServerGate.takeSessionSlot"),
				enforce(pkgServer, "sessionSlot.release"),
				enforce(pkgServer, "newServerShell"),
				enforce(pkgServer, "mcpServerGate.middleware"),
				enforce(pkgServer, "registerOAuthMCPHandlers"),
				enforce(pkgServer, "registerLegacyMCPHandlers"),
				busyFailure,
				heldRefusal,
			},
		},
		{
			ID: "POL-001", Question: Allow, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "pool entries",
			Key:      KeyProcess, StdioKey: KeyNone, Table: true,
			Values: []string{"PoolSize", "PoolSizeMax"},
			Source: Configurable, Flags: []string{"--max-http-clients"}, Envs: []string{"GITLAB_MCP_MAX_HTTP_CLIENTS"},
			Config: []string{"MaxHTTPClients"}, Malformed: RefuseStartup, Zero: ZeroRefused,
			AtCapacity: EvictAcrossKeys,
			Decided:    []string{"ADR-0020", "issue 561"},
			Findings:   []string{"F-16", "F-34"},
			// The pool keeps its own fallback, stated a second time (issue
			// 958); it is held equal rather than aliased.
			Sites: []Site{
				alias(pkgConfig, "DefaultMaxHTTPClients", "PoolSize"),
				alias(pkgConfig, "MaxHTTPClients", "PoolSizeMax"),
				pin(pkgPool, "defaultMaxSize", "PoolSize"),
				enforce(pkgPool, "ServerPool.insertEntry"),
				enforce(pkgServer, "validateHTTPPoolAndRateBounds"),
			},
		},
		{
			ID: "POL-002", Question: Allow, Kind: Rule, Class: ClassR, Disposition: Ruled,
			Resource: "the entry evicted under size pressure, quiet ones first",
			Key:      KeyEntry, StdioKey: KeyNone,
			Decided:  []string{"ADR-0020", "issue 561"},
			Findings: []string{"F-26"},
			Refusals: []Refusal{
				listenEnd("credential_evicted", StartOver, refuse(pkgServer, "endOfCredentialEviction")),
				{
					Methods: []string{MethodEviction}, Era: EraLegacy, Channel: SessionClose, Answer: StartOver,
					At: refuse(pkgServer, "sessionOwners.endSessionsWithoutStreams"),
				},
			},
			Sites: []Site{
				enforce(pkgPool, "ServerPool.lruVictimLocked"),
				enforce(pkgPool, "ServerPool.evictLRU"),
			},
		},
		{
			// Promoted: which holdings make an entry busy is Busy's answer,
			// and the server's per-credential state asks it over what it holds.
			ID: "POL-003", Question: Allow, Kind: Rule, Class: ClassR, Disposition: Promoted,
			Resource: "whether an entry is busy: an open listen stream or a watcher",
			Key:      KeyEntry, StdioKey: KeyNone,
			Functions: []string{"Busy"},
			Sites: []Site{
				enforce(pkgServer, "credentialState.busy"),
				enforce(pkgServer, "credentialStates.inUse"),
				enforce(pkgServer, "newShapedServerPool"),
			},
		},
		{
			ID: "POL-004", Question: Allow, Kind: Lifetime, Class: ClassR, Disposition: Valued,
			Resource: "how long an entry may go unused",
			Key:      KeyEntry, StdioKey: KeyNone,
			Values: []string{"PoolIdleTimeout", "PoolIdleTimeoutMax", "PoolIdleSweepDivisor", "PoolIdleSweepFloor"},
			Source: Configurable, Flags: []string{"--pool-idle-timeout"}, Envs: []string{"GITLAB_MCP_POOL_IDLE_TIMEOUT"},
			Config: []string{"PoolIdleTimeout"}, Malformed: RefuseStartup, Zero: ZeroOff,
			Findings: []string{"F-16"},
			Refusals: []Refusal{
				listenEnd("credential_reset", StartOver, refuse(pkgServer, "endOfCredentialReset")),
			},
			Sites: []Site{
				alias(pkgConfig, "DefaultPoolIdleTimeout", "PoolIdleTimeout"),
				alias(pkgConfig, "MaxPoolIdleTimeout", "PoolIdleTimeoutMax"),
				alias(pkgPool, "idleSweepDivisor", "PoolIdleSweepDivisor"),
				alias(pkgPool, "idleSweepMinInterval", "PoolIdleSweepFloor"),
				pin(pkgPool, "DefaultIdleTimeout", "PoolIdleTimeout"),
				enforce(pkgPool, "ServerPool.StartIdleEviction"),
			},
		},
		{
			ID: "POL-005", Question: Allow, Kind: Rule, Class: ClassR, Disposition: Mechanism,
			Resource: "one build shared by concurrent requests for one entry",
			Key:      KeyEntry, StdioKey: KeyNone,
			Sites: []Site{enforce(pkgPool, "ServerPool.GetOrCreateEntry")},
		},
		{
			ID: "IDN-009", Question: Allow, Kind: Rule, Class: ClassP, Disposition: Ruled,
			Resource: "how telemetry records a caller",
			Key:      KeyProcess, StdioKey: KeyProcess,
			Source: Configurable, Flags: []string{"--telemetry-identity", "--telemetry-identity-rotation"},
			Envs: []string{
				"GITLAB_MCP_TELEMETRY_IDENTITY", "GITLAB_MCP_TELEMETRY_IDENTITY_KEY",
				"GITLAB_MCP_TELEMETRY_IDENTITY_ROTATION",
			},
			Malformed: RefuseStartup,
			Findings:  []string{"F-13"},
			Sites:     []Site{enforce(pkgServer, "telemetryIdentityPolicy")},
		},
		{
			ID: "IDN-011", Question: Allow, Kind: Lifetime, Class: ClassQ, Disposition: Valued,
			Resource: "how long signed multi-round-trip request state stays usable",
			Key:      KeyRequest, StdioKey: KeyRequest,
			Values: []string{"RequestStateTTL"}, Source: Constant, Zero: ZeroNotApplicable,
			Findings: []string{"F-12"},
			Sites: []Site{
				alias(pkgElicitation, "stateTTL", "RequestStateTTL"),
				enforce(pkgElicitation, "decodeState"),
			},
		},
	}
}
