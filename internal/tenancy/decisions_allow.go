package tenancy

// allowDecisions are the rows that answer "what may it hold or spend?" (spec
// 4.4): the token buckets and what each method is charged to, the listen and
// watcher ceilings with their process partners, the watch lease, the pool's
// size, eviction and idle rules, the upstream retry policy, the telemetry
// identity policy and the lifetime of multi-round-trip request state.
//
// Four of them are class D: keyed on the entry while their own reason is about
// a GitLab user or the process, so one tenant holding N credentials holds N
// units (RTC-001, RTC-003, RTC-005, HLD-003). Each carries the finding that
// records it; moving any of them to the tenant would be a change of policy,
// and would still leave it dividable by bots (TEN-008).
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: its length is the number of decisions it declares.
func allowDecisions() []Decision {
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
			Findings: []string{"F-01", "F-02", "F-19", "F-20", "F-21", "F-32", "F-34"},
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
				enforce(pkgToolutil, "AttachRateLimitFunc"),
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
					At: refuse(pkgToolutil, "AttachRateLimitFunc"),
				},
			},
			// The middleware enforces it as well as writing its refusal: it is
			// where MeterFor sends completion/complete to this bucket.
			Sites: []Site{
				alias(pkgToolutil, "completionBurstFactor", "CompletionFactor"),
				enforce(pkgToolutil, "RateLimiter.scaled"),
				enforce(pkgToolutil, "AttachRateLimitFunc"),
				refuse(pkgToolutil, "AttachRateLimitFunc"),
			},
		},
		{
			ID: "RTC-003", Question: Allow, Kind: Rate, Class: ClassD, Disposition: Valued,
			Resource: "tools/list",
			Key:      KeyEntry, StdioKey: KeyProcess,
			// The reason is the processor every tenant shares, and there is no
			// process partner beside the per-entry bucket (F-03, issue 951).
			StatedUnit: KeyProcess, ReasonUnit: KeyProcess, ProtectsProcess: true,
			Reason:   "spends instead the processor every tenant of this process is waiting for",
			ReasonAt: reasonAt(pkgToolutil, "methodToolsList"),
			Values:   []string{"CatalogDivisor"}, Source: Derived, Zero: ZeroNotApplicable,
			Functions: []string{"MeterFor"},
			Findings:  []string{"F-03"},
			Refusals: []Refusal{
				{
					Methods: []string{"tools/list"}, Channel: RPC, Code: CodeTooManyRequests,
					Prefix: "rate limit exceeded for ", Answer: RetryLater, At: rateLimitedError,
				},
			},
			// The middleware is where MeterFor sends tools/list to this bucket,
			// and where the server's own listings are exempted from it.
			Sites: []Site{
				alias(pkgToolutil, "catalogDivisor", "CatalogDivisor"),
				enforce(pkgToolutil, "RateLimiter.slowed"),
				enforce(pkgToolutil, "AttachRateLimitFunc"),
				rateLimitedError,
			},
		},
		{
			// Promoted: which method is charged to which bucket, and so which
			// is charged to none, is MeterFor's answer, and the middleware
			// switches on it.
			ID: "RTC-004", Question: Allow, Kind: Rule, Class: ClassP, Disposition: Promoted,
			Resource: "initialize, resources/list, prompts/list and every other unmetered method",
			Key:      KeyRequest, StdioKey: KeyRequest,
			Functions: []string{"MeterFor"},
			Sites:     []Site{enforce(pkgToolutil, "AttachRateLimitFunc")},
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
			// A decision by absence, recorded like RTC-004: nothing bounds the
			// requests one credential or the process holds open, nor the
			// stateful sessions, while the reason the listen ceilings give
			// applies to every held connection (F-31, issue 951).
			ID: "HLD-010", Question: Allow, Kind: Ceiling, Class: ClassP, Disposition: Ruled,
			Resource: "held requests and stateful sessions",
			Key:      KeyProcess, StdioKey: KeyNone,
			ReasonUnit: KeyProcess, ProtectsProcess: true,
			Source:   SourceNone,
			Findings: []string{"F-31"},
			Sites: []Site{
				enforce(pkgServer, "streamableHTTPOptions"),
				enforce(pkgServer, "sessionOwners.record"),
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
