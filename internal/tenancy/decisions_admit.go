package tenancy

// admitDecisions are the rows that answer "may this credential enter, and for
// how long?" (spec 4.4): verification in both modes, the caches that remember
// a verdict, the lifetimes that force a credential to be checked again, the
// Origin and Host guards, the authentication failure budgets that run before a
// tenant exists, and the destination rules at the door.
//
// Every gate refusal here is written by the gate or the bearer guard in front
// of the SDK, which is the only place a status other than 400 or 404 can be
// chosen (PAT-004). Which of them charge the failure budgets is [Failures]'s
// to say, one row per return. The admission rows also end the listens of an
// entry they evict or revoke (ADM-008 to ADM-010), refuse to start a
// deployment whose escape hatch is reachable (DST-002), and stop counting a
// new address silently once a tracking table is full (AUB-004).
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: its length is the number of decisions it declares.
func admitDecisions() []Decision {
	resolve := refuse(pkgServer, "mcpServerGate.resolve")
	check := refuse(pkgServer, "bearerGuard.check")
	classify := refuse(pkgServer, "bearerGuard.classify")
	invalidToken := refuse(pkgServer, "bearerGuard.invalidTokenFailure")
	unaccepted := refuse(pkgServer, "bearerGuard.unacceptedRecipientFailure")
	rejectedCharges := []string{"AUB-001", "AUB-002", "AUB-003"}
	blocked := func(at Site) Refusal {
		return Refusal{
			Methods: []string{MethodGate}, Channel: Gate, Code: CodeTooManyRequests, Status: 429,
			RetryAfter: RetryAfterLongestBlock, Prefix: blockedPrefix, Answer: RetryLater, At: at,
		}
	}
	blockedRefusals := []Refusal{blocked(resolve), blocked(check)}

	return []Decision{
		{
			ID: "ADM-001", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "admission of a legacy credential GitLab did not refuse",
			Key:      KeyEntry, StdioKey: KeyNone,
			Findings: []string{"F-08", "F-17"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnauthorized, Status: 401,
					Challenge: true, Prefix: rejectedPrefix, Answer: Reauthorize, Charged: rejectedCharges, At: resolve,
				},
				gateRefusal(503, CodeUnavailable, "Could not initialize a GitLab session for this token.", RetryLater, resolve),
			},
			Sites: []Site{enforce(pkgPool, "verifyCredential"), resolve},
		},
		{
			ID: "ADM-002", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Valued,
			Resource: "admission of an OAuth token at the read_api minimum",
			Key:      KeyVerified, StdioKey: KeyNone,
			Values: []string{"UpstreamRetryAfter"}, Source: Constant, Zero: ZeroNotApplicable,
			Decided:  []string{"ADR-0018"},
			Findings: []string{"F-08", "F-17", "F-30"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnauthorized, Status: 401,
					Challenge: true, Prefix: rejectedPrefix, Answer: Reauthorize, Charged: rejectedCharges,
					At: classify, Via: invalidToken,
				},
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeForbidden, Status: 403,
					Challenge: true, Answer: WidenScope, At: check,
				},
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeForbidden, Status: 403,
					Challenge: true, Prefix: "GitLab rejected this token for lacking the scope", Answer: WidenScope, At: classify,
				},
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnavailable, Status: 503,
					RetryAfter: RetryAfterUpstreamOrFixed, Prefix: "GitLab could not verify this token right now;",
					Answer: RetryLater, At: classify,
				},
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnavailable, Status: 503,
					RetryAfter: RetryAfterFixed, Prefix: "GitLab could not verify this token right now.",
					Answer: RetryLater, At: classify,
				},
			},
			Sites: []Site{
				alias(pkgServer, "upstreamRetryAfter", "UpstreamRetryAfter"),
				enforce(pkgOAuth, "NewGitLabVerifierFor"),
				classify, invalidToken, check,
			},
		},
		{
			ID: "ADM-003", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the scopes assumed when introspection cannot answer",
			Key:      KeyVerified, StdioKey: KeyNone,
			Findings: []string{"F-09"},
			Sites:    []Site{enforce(pkgOAuth, "introspectToken")},
		},
		{
			ID: "ADM-004", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the OAuth applications whose tokens are admitted",
			Key:      KeyApplication, StdioKey: KeyNone,
			Source: Configurable, Flags: []string{"--oauth-client-uid"}, Envs: []string{"GITLAB_MCP_OAUTH_CLIENT_UID"},
			Config: []string{"OAuthClientUIDs"}, Malformed: AcceptsAny,
			Decided: []string{"ADR-0019"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnauthorized, Status: 401,
					Challenge: true, Prefix: recipientText, Answer: Reauthorize, At: unaccepted,
				},
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnavailable, Status: 503,
					RetryAfter: RetryAfterFixed,
					Prefix:     "This deployment admits only tokens issued to specific OAuth applications",
					Answer:     RetryLater, At: classify,
				},
			},
			Sites: []Site{enforce(pkgOAuth, "acceptedRecipient"), unaccepted, classify},
		},
		{
			ID: "ADM-005", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "how long a verified OAuth token is reused",
			Key:      KeyVerified, StdioKey: KeyNone, Table: true,
			Values: []string{
				"OAuthCacheTTL", "OAuthCacheTTLFloor", "OAuthCacheTTLMax", "OAuthCacheSweepDivisor", "OAuthCacheSweepFloor",
			},
			Source: Configurable, Flags: []string{"--oauth-cache-ttl"}, Envs: []string{"GITLAB_MCP_OAUTH_CACHE_TTL"},
			Config: []string{"OAuthCacheTTL"}, Malformed: RefuseStartup, Zero: ZeroSelectsDefault,
			AtCapacity: CapacityNone,
			Findings:   []string{"F-29", "F-34"},
			Sites: []Site{
				alias(pkgConfig, "DefaultOAuthCacheTTL", "OAuthCacheTTL"),
				alias(pkgConfig, "MinOAuthCacheTTL", "OAuthCacheTTLFloor"),
				alias(pkgConfig, "MaxOAuthCacheTTL", "OAuthCacheTTLMax"),
				alias(pkgServer, "tokenCacheSweepDivisor", "OAuthCacheSweepDivisor"),
				alias(pkgServer, "tokenCacheSweepMinInterval", "OAuthCacheSweepFloor"),
				enforce(pkgOAuth, "effectiveCacheTTL"),
				enforce(pkgServer, "oauthCacheTTL"),
				enforce(pkgOAuth, "TokenCache.Put"),
			},
		},
		{
			ID: "ADM-006", Question: Admit, Kind: Lifetime, Class: ClassA, Disposition: Valued,
			Resource: "how long, and how many, rejected tokens are remembered",
			Key:      KeyRefused, StdioKey: KeyNone, Table: true,
			Values: []string{"RejectedTokenTTL", "RejectedTokenCapacity"}, Source: Constant, Zero: ZeroNotApplicable,
			AtCapacity: EvictOldest,
			Findings:   []string{"F-11", "F-16"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnauthorized, Status: 401,
					Challenge: true, Prefix: rejectedPrefix, Answer: Reauthorize, Charged: rejectedCharges,
					At: check, Via: invalidToken,
				},
			},
			Sites: []Site{
				alias(pkgServer, "rejectedTokenTTL", "RejectedTokenTTL"),
				alias(pkgServer, "rejectedTokenMaxSize", "RejectedTokenCapacity"),
				enforce(pkgServer, "registerOAuthMCPHandlers"),
				check, invalidToken,
			},
		},
		{
			ID: "ADM-007", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "a session presented by a credential other than its owner's",
			Key:      KeySession, StdioKey: KeyNone,
			Findings: []string{"F-22"},
			Refusals: []Refusal{
				gateRefusal(404, codeInvalidRequest, "This session does not belong to the presented credential.", StartOver,
					refuse(pkgServer, "sessionOwnershipFailure")),
			},
			Sites: []Site{
				enforce(pkgServer, "mcpServerGate.checkSessionOwnership"),
				refuse(pkgServer, "sessionOwnershipFailure"),
			},
		},
		{
			ID: "ADM-008", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "the longest an entry serves on one credential check",
			Key:      KeyEntry, StdioKey: KeyNone,
			Values: []string{"CredentialMaxAge", "CredentialMaxAgeCeiling"}, Source: OptionOnly, Zero: ZeroSelectsDefault,
			Findings: []string{"F-16", "F-34"},
			// An entry past the age is evicted on its next request, and its
			// open listens are told the credential was reset.
			Refusals: []Refusal{
				listenEnd("credential_reset", StartOver, refuse(pkgServer, "endOfCredentialReset")),
			},
			Sites: []Site{
				alias(pkgPool, "DefaultMaxCredentialAge", "CredentialMaxAge"),
				alias(pkgPool, "maxCredentialAgeCeiling", "CredentialMaxAgeCeiling"),
				enforce(pkgPool, "WithMaxCredentialAge"),
				enforce(pkgPool, "ServerPool.evictStaleCredential"),
			},
		},
		{
			ID: "ADM-009", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "how often every entry's credential is re-probed",
			Key:      KeyEntry, StdioKey: KeyNone,
			Values: []string{"RevalidateInterval", "RevalidateIntervalMax"},
			Source: Configurable, Flags: []string{"--revalidate-interval"},
			Envs: []string{"GITLAB_MCP_SESSION_REVALIDATE_INTERVAL"}, Config: []string{"RevalidateInterval"},
			Malformed: RefuseStartup, Zero: ZeroOff,
			Findings: []string{"F-16"},
			Refusals: []Refusal{
				listenEnd("credential_revoked", Reauthorize, refuse(pkgServer, "endOfCredentialRevocation")),
			},
			Sites: []Site{
				alias(pkgConfig, "DefaultRevalidateInterval", "RevalidateInterval"),
				alias(pkgConfig, "MaxRevalidateInterval", "RevalidateIntervalMax"),
				pin(pkgPool, "DefaultRevalidateInterval", "RevalidateInterval"),
				enforce(pkgPool, "ServerPool.revalidateAll"),
			},
		},
		{
			ID: "ADM-010", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "how often a 401 that named no cause is confirmed with a probe",
			Key:      KeyEntry, StdioKey: KeyNone,
			Values: []string{"UnexplainedRefusalCooldown"}, Source: Constant, Zero: ZeroNotApplicable,
			Findings: []string{"F-16"},
			Refusals: []Refusal{
				listenEnd("credential_revoked", Reauthorize, refuse(pkgServer, "endOfCredentialRevocation")),
			},
			Sites: []Site{
				alias(pkgPool, "unauthorizedConfirmCooldown", "UnexplainedRefusalCooldown"),
				enforce(pkgPool, "ServerPool.handleUnauthorized"),
			},
		},
		{
			ID: "ADM-012", Question: Admit, Kind: Rule, Class: ClassQ, Disposition: Ruled,
			Resource: "the origins a browser may reach the server from",
			Key:      KeyRequest, StdioKey: KeyNone,
			Source: Configurable, Flags: []string{"--trusted-origins"}, Envs: []string{"GITLAB_MCP_TRUSTED_ORIGINS"},
			Config: []string{"TrustedOrigins"}, Malformed: RefuseStartup,
			Refusals: []Refusal{
				gateRefusal(403, CodeForbidden, "Cross-origin request refused:", AskOperator,
					refuse(pkgServer, "writeUntrustedOrigin")),
			},
			Sites: []Site{
				enforce(pkgServer, "mcpOriginMiddleware"),
				enforce(pkgServer, "crossOriginProtectionMiddleware"),
				enforce(pkgServer, "buildTrustedOrigins"),
				refuse(pkgServer, "writeUntrustedOrigin"),
			},
		},
		{
			ID: "ADM-013", Question: Admit, Kind: Rule, Class: ClassQ, Disposition: Ruled,
			Resource: "the Host names the deployment answers",
			Key:      KeyRequest, StdioKey: KeyNone,
			Refusals: []Refusal{
				gateRefusal(403, CodeForbidden, "Request refused: the Host header names a host", AskOperator,
					refuse(pkgServer, "hostValidationMiddleware")),
			},
			Sites: []Site{
				enforce(pkgServer, "newHostGuard"),
				enforce(pkgServer, "hostGuard.permits"),
				enforce(pkgServer, "allowedHosts"),
				refuse(pkgServer, "hostValidationMiddleware"),
			},
		},
		{
			ID: "AUB-001", Question: Admit, Kind: Budget, Class: ClassA, Disposition: Valued,
			Resource: "failed authentications one address may produce in a window",
			Key:      KeyAddress, StdioKey: KeyNone,
			Values: []string{"AuthFailureLimit", "AuthFailureWindow", "AuthFailureLimitMax", "AuthFailureWindowMax"},
			// Which settings mean off is BudgetOn's answer.
			Functions: []string{"BudgetOn"},
			Source:    Configurable, Flags: []string{"--auth-failure-limit", "--auth-failure-window"},
			Envs:   []string{"GITLAB_MCP_AUTH_FAILURE_LIMIT", "GITLAB_MCP_AUTH_FAILURE_WINDOW"},
			Config: []string{"AuthFailureLimit", "AuthFailureWindow"}, Malformed: RefuseStartup, Zero: ZeroOff,
			Decided:  []string{"issue 790"},
			Findings: []string{"F-14", "F-25", "F-34"},
			Refusals: blockedRefusals,
			// The gate's own codes are declared here, with the budget row whose
			// layer moves them: they mirror the statuses every gate refusal
			// above and in the admission rows carries.
			Sites: []Site{
				alias(pkgConfig, "DefaultAuthFailureLimit", "AuthFailureLimit"),
				alias(pkgConfig, "DefaultAuthFailureWindow", "AuthFailureWindow"),
				alias(pkgConfig, "MaxAuthFailureLimit", "AuthFailureLimitMax"),
				alias(pkgConfig, "MaxAuthFailureWindow", "AuthFailureWindowMax"),
				alias(pkgServer, "authFailureWindow", "AuthFailureWindow"),
				alias(pkgServer, "errCodeUnauthorized", "CodeUnauthorized"),
				alias(pkgServer, "errCodeForbidden", "CodeForbidden"),
				alias(pkgServer, "errCodeTooManyRequests", "CodeTooManyRequests"),
				alias(pkgServer, "errCodeUpstreamUnavailable", "CodeUnavailable"),
				enforce(pkgServer, "authFailureLimiter"),
				resolve, check,
			},
		},
		{
			ID: "AUB-002", Question: Admit, Kind: Budget, Class: ClassA, Disposition: Valued,
			Resource: "distinct failing primary keys one transport source may produce in a window",
			Key:      KeySource, StdioKey: KeyNone,
			// The pairs already charged are remembered in a map keyed on the
			// source and the primary key, both of which a caller mints. A
			// source stops adding pairs once it is blocked, but a source that
			// arrives after the limiter's table is full is never blocked, so
			// nothing but the sweep bounds the map, which F-35 records
			// (issue 982). It is kept in both authentication modes, so it is
			// not F-29's, whose issue is about OAuth verification.
			Table:  true,
			Values: []string{"TransportSourceDistinctKeys"}, Source: Constant, Zero: ZeroNotApplicable,
			// Whether the budget exists is TransportSourceBudgetOn's answer.
			Functions: []string{"TransportSourceBudgetOn"},
			Findings:  []string{"F-16", "F-35"},
			Refusals:  blockedRefusals,
			// The window falls back to the default when AUB-001's is zero, in
			// three places kept in step by hand (issue 958). It is declared by
			// all three rather than moved to one.
			Sites: []Site{
				alias(pkgServer, "transportFailureLimit", "TransportSourceDistinctKeys"),
				enforce(pkgServer, "transportFailureBudget"),
				enforce(pkgServer, "newTransportBudget"),
				enforce(pkgServer, "transportBudget.window"),
				enforce(pkgServer, "transportBudget.charge"),
				resolve, check,
			},
		},
		{
			ID: "AUB-003", Question: Admit, Kind: Budget, Class: ClassA, Disposition: Valued,
			Resource: "distinct refused credentials one address may have in a window",
			Key:      KeyAddress, StdioKey: KeyNone,
			Values: []string{
				"AuthDistinctTokenLimit", "AuthDistinctTokenWindow", "AuthDistinctTokenLimitMax",
				"AuthDistinctTokenWindowMax", "AuthEscalationFirst", "AuthEscalationSecond", "AuthEscalationThird",
			},
			// Which settings mean off, the escalation step among them, is
			// EscalationOn's answer.
			Functions: []string{"EscalationOn"},
			Source:    Configurable, Flags: []string{"--auth-distinct-token-limit", "--auth-distinct-token-window"},
			Envs:   []string{"GITLAB_MCP_AUTH_DISTINCT_TOKEN_LIMIT", "GITLAB_MCP_AUTH_DISTINCT_TOKEN_WINDOW"},
			Config: []string{"AuthDistinctTokenLimit", "AuthDistinctWindow"}, Malformed: RefuseStartup,
			Zero: ZeroOff, OffWith: "AUB-001",
			Decided:  []string{"issue 790"},
			Findings: []string{"F-14", "F-25", "F-34"},
			Refusals: blockedRefusals,
			Sites: []Site{
				alias(pkgConfig, "DefaultAuthDistinctTokenLimit", "AuthDistinctTokenLimit"),
				alias(pkgConfig, "DefaultAuthDistinctWindow", "AuthDistinctTokenWindow"),
				alias(pkgConfig, "MaxAuthDistinctTokenLimit", "AuthDistinctTokenLimitMax"),
				alias(pkgConfig, "MaxAuthDistinctWindow", "AuthDistinctTokenWindowMax"),
				ladderStep(0, "AuthEscalationFirst"),
				ladderStep(1, "AuthEscalationSecond"),
				ladderStep(2, "AuthEscalationThird"),
				enforce(pkgPool, "NewDistinctTokenBudget"),
				enforce(pkgServer, "authSprayBudget"),
				resolve, check,
			},
		},
		{
			ID: "AUB-004", Question: Admit, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "addresses the authentication failure table and the distinct-credential address table each track",
			Key:      KeyProcess, StdioKey: KeyNone,
			ReasonUnit: KeyProcess, ProtectsProcess: true,
			Values: []string{"AuthTrackedSources"}, Source: Constant, Zero: ZeroNotApplicable,
			AtCapacity: StopCounting,
			Findings:   []string{"F-11"},
			// The one value caps both tables, and each stops counting a new
			// address once it is full.
			Refusals: []Refusal{
				{Methods: []string{MethodGate}, Channel: Silent, Answer: NoAnswer, At: refuse(pkgPool, "AuthRateLimiter.RecordFailure")},
				{Methods: []string{MethodGate}, Channel: Silent, Answer: NoAnswer, At: refuse(pkgPool, "DistinctTokenBudget.roomForNewKeyLocked")},
			},
			Sites: []Site{
				alias(pkgPool, "maxTrackedAuthSources", "AuthTrackedSources"),
				enforce(pkgPool, "DistinctTokenBudget.roomForNewKeyLocked"),
				refuse(pkgPool, "AuthRateLimiter.RecordFailure"),
				refuse(pkgPool, "DistinctTokenBudget.roomForNewKeyLocked"),
			},
		},
		{
			ID: "AUB-005", Question: Admit, Kind: Lifetime, Class: ClassP, Disposition: Valued,
			Resource: "how often the authentication tables are swept",
			Key:      KeyProcess, StdioKey: KeyNone,
			Values: []string{"AuthSweepInterval"}, Source: Constant, Zero: ZeroNotApplicable,
			// A var rather than a const, so a test can make the tick arrive;
			// nothing at runtime writes it.
			Sites: []Site{alias(pkgServer, "periodicCleanupInterval", "AuthSweepInterval")},
		},
		{
			ID: "POL-006", Question: Admit, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "credential probes the pool runs at once",
			Key:      KeyProcess, StdioKey: KeyNone,
			Reason:     "counting work rather than callers",
			ReasonAt:   reasonAt(pkgPool, "maxConcurrentCredentialProbes"),
			ReasonUnit: KeyProcess, ProtectsProcess: true,
			Values: []string{"CredentialProbes", "CredentialProbeWait"}, Source: Constant, Zero: ZeroNotApplicable,
			AtCapacity: WaitThenRefuse,
			Findings:   []string{"F-16", "F-30"},
			Refusals: []Refusal{
				gateRefusal(503, CodeUnavailable, "Could not initialize a GitLab session for this token.", RetryLater, resolve),
			},
			Sites: []Site{
				alias(pkgPool, "maxConcurrentCredentialProbes", "CredentialProbes"),
				alias(pkgPool, "credentialProbeQueueTimeout", "CredentialProbeWait"),
				enforce(pkgPool, "New"),
				resolve,
			},
		},
		{
			ID: "POL-009", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Mechanism,
			Resource: "an entry marked rejected, rebuilt rather than served",
			Key:      KeyEntry, StdioKey: KeyNone,
			Sites: []Site{enforce(pkgPool, "ServerPool.dropRejectedEntry")},
		},
		{
			ID: "DST-001", Question: Admit, Kind: Rule, Class: ClassQ, Disposition: Ruled,
			Resource: "a caller-named instance the dialer would refuse, refused at the door",
			Key:      KeyRequest, StdioKey: KeyNone,
			Decided: []string{"ADR-0022"},
			Refusals: []Refusal{
				gateRefusal(400, codeInvalidRequest, "", AskOperator, resolve),
			},
			Sites: []Site{enforce(pkgGitLab, "CheckCallerNamedInstance"), resolve},
		},
		{
			ID: "DST-002", Question: Admit, Kind: Rule, Class: ClassP, Disposition: Ruled,
			Resource: "the escape hatch that lets a caller name any instance",
			Key:      KeyDeployment, StdioKey: KeyNone,
			Flags:   []string{"--allow-any-gitlab-url"},
			Decided: []string{"ADR-0022"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodStartup}, Channel: Startup,
					Prefix: "--allow-any-gitlab-url names no instance and --http-addr ", Answer: AskOperator,
					At: refuse(pkgServer, "requireInstanceAllowList"),
				},
			},
			Sites: []Site{
				enforce(pkgServer, "requireInstanceAllowList"),
				enforce(pkgServer, "listenerIsHostLocal"),
			},
		},
	}
}

// ladderStep is one element of the distinct-credential budget's escalation
// ladder, a composite literal whose every element aliases a register constant.
func ladderStep(index int, reads string) Site {
	return Site{Pkg: pkgPool, Name: "escalationLadder", Role: Alias, Reads: reads, Arg: index}
}
